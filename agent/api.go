package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) serveAPI(ctx context.Context) {
	ln, err := a.srv.Listen("tcp", fmt.Sprintf(":%d", APIPort))
	if err != nil {
		log.Printf("api listen: %v", err)
		return
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"name": myName(), "version": Version})
	})

	mux.HandleFunc("/clipboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "PUT", 405)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.applyRemoteClipboard(r.URL.Query().Get("fmt"), b)
	})

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "PUT", 405)
			return
		}
		name := safeName(r.URL.Query().Get("name"))
		if name == "" {
			http.Error(w, "nume lipsă", 400)
			return
		}
		dst, err := saveIncoming(name, r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("primit fișier: %s", dst)
		notifyUser("Fișier primit", filepath.Base(dst))
	})

	// Doar pe PC.
	if isHost() {
		mux.HandleFunc("/monitors", func(w http.ResponseWriter, r *http.Request) {
			mons, err := listMonitors()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			cur := currentMonitor()
			for i := range mons {
				mons[i].Current = mons[i].ID == cur
			}
			writeJSON(w, mons)
		})
		mux.HandleFunc("/monitor", func(w http.ResponseWriter, r *http.Request) {
			if err := setMonitor(r.URL.Query().Get("id")); err != nil {
				http.Error(w, err.Error(), 500)
			}
		})
		mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
			if err := sunshinePin(r.URL.Query().Get("pin"), MacName); err != nil {
				http.Error(w, err.Error(), 500)
			}
		})
		mux.HandleFunc("/power", func(w http.ResponseWriter, r *http.Request) {
			if err := powerAction(r.URL.Query().Get("action")); err != nil {
				http.Error(w, err.Error(), 500)
			}
		})
	}

	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); hs.Close() }()
	if err := hs.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Printf("api: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// client HTTP care merge prin rețeaua privată.
func (a *App) httpClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return a.srv.Dial(ctx, network, addr)
			},
		},
	}
}

// call trimite o cerere la celălalt calculator.
func (a *App) call(method, path string, q url.Values, body io.Reader, timeout time.Duration) ([]byte, error) {
	addr, err := a.peerAddr(APIPort)
	if err != nil {
		return nil, err
	}
	u := "http://" + addr + path
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// ---------- fișiere ----------

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	if n == "." || n == "/" || n == ".." || strings.HasPrefix(n, ".") && len(strings.Trim(n, ".")) == 0 {
		return ""
	}
	return n
}

// saveIncoming salvează fără să suprascrie: "poza.jpg", "poza (1).jpg"...
func saveIncoming(name string, r io.Reader) (string, error) {
	dir := inboxDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	dst := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	return dst, os.Rename(tmp, dst)
}

// sendFile trimite un fișier la celălalt calculator.
func (a *App) sendFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s e folder (trimite fișiere, sau fă-l zip)", filepath.Base(path))
	}
	q := url.Values{"name": {filepath.Base(path)}}
	_, err = a.call(http.MethodPut, "/upload", q, f, 0)
	return err
}

func (a *App) putClipboard(format string, b []byte) error {
	_, err := a.call(http.MethodPut, "/clipboard", url.Values{"fmt": {format}}, bytes.NewReader(b), 15*time.Second)
	return err
}

// outboxLoop: tot ce pui în folderul "Trimite pe ..." pleacă automat la celălalt.
func (a *App) outboxLoop(ctx context.Context) {
	dir := outboxDir()
	sent := filepath.Join(dir, "trimise")
	os.MkdirAll(sent, 0o755)
	sizes := map[string]int64{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		_, _, online := a.snapshot()
		if !online {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasSuffix(e.Name(), ".part") || strings.EqualFold(e.Name(), "desktop.ini") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			info, err := e.Info()
			if err != nil {
				continue
			}
			seen[p] = true
			// trimite doar când mărimea nu s-a mai schimbat (copierea s-a terminat)
			if prev, ok := sizes[p]; !ok || prev != info.Size() {
				sizes[p] = info.Size()
				continue
			}
			if err := a.sendFile(p); err != nil {
				log.Printf("outbox %s: %v", e.Name(), err)
				continue
			}
			delete(sizes, p)
			dst := filepath.Join(sent, e.Name())
			if _, err := os.Stat(dst); err == nil {
				dst = filepath.Join(sent, fmt.Sprintf("%d-%s", time.Now().Unix(), e.Name()))
			}
			os.Rename(p, dst)
			log.Printf("trimis: %s", e.Name())
		}
		for p := range sizes {
			if !seen[p] {
				delete(sizes, p)
			}
		}
	}
}
