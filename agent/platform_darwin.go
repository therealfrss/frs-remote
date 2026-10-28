//go:build darwin

package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func homeDir() string { h, _ := os.UserHomeDir(); return h }

func inboxDir() string  { return filepath.Join(homeDir(), "Downloads", "De pe PC") }
func outboxDir() string { return filepath.Join(homeDir(), "Desktop", "Trimite pe PC") }

func openURL(u string) { exec.Command("open", u).Start() }

func openFolder(p string) {
	os.MkdirAll(p, 0o755)
	exec.Command("open", p).Start()
}

func osaQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func notifyUser(title, msg string) {
	exec.Command("osascript", "-e",
		"display notification "+osaQuote(msg)+" with title "+osaQuote("FRS Remote")+" subtitle "+osaQuote(title)).Start()
}

func alertUser(msg string) {
	exec.Command("osascript", "-e",
		"display dialog "+osaQuote(msg)+" with title "+osaQuote("FRS Remote")+` buttons {"OK"} default button "OK"`).Start()
}

// confirm întoarce true dacă utilizatorul apasă butonul de confirmare.
func confirm(msg, okButton string) bool {
	script := "display dialog " + osaQuote(msg) + " with title " + osaQuote("FRS Remote") +
		" buttons {\"Anulează\", " + osaQuote(okButton) + "} default button \"Anulează\""
	out, err := exec.Command("osascript", "-e", script).Output()
	return err == nil && strings.Contains(string(out), okButton)
}

// pickFiles deschide fereastra de alegere fișiere (mai multe odată).
func pickFiles() []string {
	script := `set fs to choose file with prompt "Alege fișierele pentru PC" with multiple selections allowed
set out to ""
repeat with f in fs
set out to out & POSIX path of f & linefeed
end repeat
return out`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
}

// ---------- Moonlight inclus în app ----------

func moonlightExe() string {
	exe, err := os.Executable()
	if err == nil {
		// FRS Remote.app/Contents/MacOS/frsremote -> Contents/Resources/Moonlight.app
		p := filepath.Join(filepath.Dir(exe), "..", "Resources", "Moonlight.app", "Contents", "MacOS", "Moonlight")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/Applications/Moonlight.app/Contents/MacOS/Moonlight"
}

func startStream() error {
	cmd := exec.Command(moonlightExe(), "stream", "127.0.0.1", "Desktop")
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func randomPin() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%04d", n.Int64())
}

// pair împerechează Moonlight cu Sunshine, fără să mai tastezi PIN-ul.
func (a *App) pair() error {
	pin := randomPin()
	cmd := exec.Command(moonlightExe(), "pair", "127.0.0.1", "--pin", pin)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Moonlight are nevoie de o secundă să inițieze cererea.
	var lastErr error
	for i := 0; i < 10; i++ {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("moonlight pair: %w", err)
			}
			return nil
		default:
		}
		_, lastErr = a.call("POST", "/pair", map[string][]string{"pin": {pin}}, nil, 0)
		if lastErr == nil {
			break
		}
		log.Printf("pair: %v", lastErr)
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("moonlight pair: %w", err)
			}
			return nil
		case <-sleepCh(1500):
		}
	}
	if lastErr != nil {
		cmd.Process.Kill()
		return lastErr
	}
	return <-done
}

// ---------- pornire la login ----------

func appBundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	if strings.HasSuffix(p, ".app") {
		return p
	}
	return ""
}

func setLoginItem(on bool) {
	app := appBundlePath()
	if app == "" {
		return
	}
	var script string
	if on {
		script = `tell application "System Events" to make login item at end with properties {path:` + osaQuote(app) + `, hidden:true}`
	} else {
		script = `tell application "System Events" to delete login item "FRS Remote"`
	}
	exec.Command("osascript", "-e", script).Run()
}

func isLoginItem() bool {
	out, err := exec.Command("osascript", "-e", `tell application "System Events" to get the name of every login item`).Output()
	return err == nil && strings.Contains(string(out), "FRS Remote")
}

// ---------- funcții doar pentru PC ----------

func listMonitors() ([]Monitor, error) { return nil, fmt.Errorf("doar pe PC") }
func currentMonitor() string           { return "" }
func setMonitor(string) error          { return fmt.Errorf("doar pe PC") }
func sunshinePin(string, string) error { return fmt.Errorf("doar pe PC") }
func powerAction(string) error         { return fmt.Errorf("doar pe PC") }
func setupSunshineCreds() error        { return fmt.Errorf("doar pe PC") }
