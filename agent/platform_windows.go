//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------- foldere ----------

func desktopDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0); err == nil && p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Desktop")
}

func inboxDir() string  { return filepath.Join(desktopDir(), "De pe Mac") }
func outboxDir() string { return filepath.Join(desktopDir(), "Trimite pe Mac") }

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

func openURL(u string) {
	hidden(exec.Command("rundll32", "url.dll,FileProtocolHandler", u)).Start()
}

func openFolder(p string) {
	os.MkdirAll(p, 0o755)
	exec.Command("explorer", p).Start()
}

func notifyUser(title, msg string) { log.Printf("%s: %s", title, msg) }

// ---------- Sunshine ----------

const sunshineUser = "frs"

func sunshineExe() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	for _, p := range []string{
		filepath.Join(pf, "Sunshine", "sunshine.exe"),
		filepath.Join(pf, "LizardByte", "Sunshine", "sunshine.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(pf, "Sunshine", "sunshine.exe")
}

func credsFile() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "FRS Remote", "sunshine.json")
}

type sunCreds struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

func loadCreds() (sunCreds, error) {
	var c sunCreds
	b, err := os.ReadFile(credsFile())
	if err != nil {
		return c, fmt.Errorf("lipsesc datele Sunshine (reinstalează FRS Remote): %w", err)
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// setupSunshineCreds rulează din installer (ca administrator).
func setupSunshineCreds() error {
	c, err := loadCreds()
	if err != nil || c.Pass == "" {
		buf := make([]byte, 18)
		rand.Read(buf)
		c = sunCreds{User: sunshineUser, Pass: hex.EncodeToString(buf)}
	}
	os.MkdirAll(filepath.Dir(credsFile()), 0o755)
	b, _ := json.Marshal(c)
	if err := os.WriteFile(credsFile(), b, 0o644); err != nil {
		return err
	}
	out, err := hidden(exec.Command(sunshineExe(), "--creds", c.User, c.Pass)).CombinedOutput()
	log.Printf("sunshine --creds: %s", bytes.TrimSpace(out))
	if err != nil {
		return fmt.Errorf("sunshine --creds: %w", err)
	}
	// repornește serviciul ca să ia datele noi
	hidden(exec.Command("sc", "stop", "SunshineService")).Run()
	time.Sleep(2 * time.Second)
	hidden(exec.Command("sc", "start", "SunshineService")).Run()
	return nil
}

func sunshineReq(method, path string, body any) ([]byte, error) {
	c, err := loadCreds()
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	const base = "https://localhost:47990"
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.Pass)
	// Fără Origin/Referer: Sunshine cere token CSRF doar cererilor de browser.
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{
		Timeout: 60 * time.Second, // /api/pin ține cererea deschisă până termină Moonlight
		Transport: &http.Transport{
			// certificatul Sunshine e self-signed și e doar pe localhost
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sunshine %s %s: %s %s", method, path, resp.Status, out)
	}
	return out, nil
}

// sunshinePin: Moonlight a pornit împerecherea; găsim cererea în așteptare și îi dăm PIN-ul.
func sunshinePin(pin, name string) error {
	if len(pin) != 4 {
		return fmt.Errorf("PIN invalid")
	}
	var id string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		b, err := sunshineReq("GET", "/api/pin", nil)
		if err != nil {
			return err
		}
		var r struct {
			Pairings []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Address string `json:"address"`
			} `json:"pairings"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("răspuns Sunshine: %w", err)
		}
		if n := len(r.Pairings); n > 0 {
			id = r.Pairings[n-1].ID
			break
		}
		time.Sleep(time.Second)
	}
	if id == "" {
		return fmt.Errorf("Sunshine nu a primit cererea de la Moonlight (verifică dacă serviciul Sunshine rulează pe PC)")
	}
	b, err := sunshineReq("POST", "/api/pin", map[string]string{"pairing_id": id, "pin": pin, "name": name})
	if err != nil {
		return err
	}
	var st struct {
		Status bool `json:"status"`
	}
	json.Unmarshal(b, &st)
	if !st.Status {
		return fmt.Errorf("Sunshine a refuzat PIN-ul")
	}
	return nil
}

func sunshineConfig() (map[string]any, error) {
	b, err := sunshineReq("GET", "/api/config", nil)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	return m, err
}

func currentMonitor() string {
	m, err := sunshineConfig()
	if err != nil {
		return ""
	}
	s, _ := m["output_name"].(string)
	return s
}

func setMonitor(id string) error {
	m, err := sunshineConfig()
	if err != nil {
		return err
	}
	// păstrăm doar setările (valori text), fără câmpurile informative
	cfg := map[string]string{}
	for k, v := range m {
		switch k {
		case "status", "platform", "version", "restart_supported":
			continue
		}
		if s, ok := v.(string); ok {
			cfg[k] = s
		}
	}
	if id == "" {
		delete(cfg, "output_name")
	} else {
		cfg["output_name"] = id
	}
	if _, err := sunshineReq("POST", "/api/config", cfg); err != nil {
		return err
	}
	_, err = sunshineReq("POST", "/api/restart", nil)
	if err != nil {
		log.Printf("sunshine restart: %v (normal dacă se reconectează)", err)
	}
	return nil
}

// ---------- monitoare ----------

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procEnumDisplayDevs = user32.NewProc("EnumDisplayDevicesW")
	procLockWorkStation = user32.NewProc("LockWorkStation")
)

// Sunshine identifică monitoarele după un ID propriu, pe care îl scrie în log la pornire.
func sunshineLog() string {
	return filepath.Join(filepath.Dir(sunshineExe()), "config", "sunshine.log")
}

func listMonitors() ([]Monitor, error) {
	if mons, err := monitorsFromSunshineLog(); err == nil && len(mons) > 0 {
		return mons, nil
	} else if err != nil {
		log.Printf("monitoare din logul Sunshine: %v", err)
	}
	return listMonitorsWin32()
}

func monitorsFromSunshineLog() ([]Monitor, error) {
	b, err := os.ReadFile(sunshineLog())
	if err != nil {
		return nil, err
	}
	const marker = "Currently available display devices:"
	i := bytes.LastIndex(b, []byte(marker))
	if i < 0 {
		return nil, fmt.Errorf("lista de monitoare lipsește din log")
	}
	rest := b[i+len(marker):]
	j := bytes.IndexByte(rest, '[')
	if j < 0 {
		return nil, fmt.Errorf("format necunoscut")
	}
	var devs []map[string]any
	if err := json.NewDecoder(bytes.NewReader(rest[j:])).Decode(&devs); err != nil {
		return nil, err
	}
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	var out []Monitor
	for _, d := range devs {
		id := str(d, "device_id", "m_device_id")
		if id == "" {
			continue
		}
		info, _ := d["info"].(map[string]any)
		if info == nil {
			info, _ = d["m_info"].(map[string]any)
		}
		if info == nil { // monitor inactiv
			continue
		}
		primary, _ := info["primary"].(bool)
		label := str(d, "friendly_name", "m_friendly_name")
		disp := str(d, "display_name", "m_display_name")
		if label == "" {
			label = disp
		}
		out = append(out, Monitor{
			ID:      id,
			Name:    fmt.Sprintf("Monitor %d – %s", len(out)+1, label),
			Primary: primary,
		})
	}
	return out, nil
}

func listMonitorsWin32() ([]Monitor, error) {
	var out []Monitor
	for i := uint32(0); i < 16; i++ {
		var d displayDevice
		d.Cb = uint32(unsafe.Sizeof(d))
		r, _, _ := procEnumDisplayDevs.Call(0, uintptr(i), uintptr(unsafe.Pointer(&d)), 0)
		if r == 0 {
			break
		}
		if d.StateFlags&0x1 == 0 { // DISPLAY_DEVICE_ATTACHED_TO_DESKTOP
			continue
		}
		name := windows.UTF16ToString(d.DeviceName[:])
		label := ""
		var mon displayDevice
		mon.Cb = uint32(unsafe.Sizeof(mon))
		nameP, _ := windows.UTF16PtrFromString(name)
		if r, _, _ := procEnumDisplayDevs.Call(uintptr(unsafe.Pointer(nameP)), 0, uintptr(unsafe.Pointer(&mon)), 0); r != 0 {
			label = windows.UTF16ToString(mon.DeviceString[:])
		}
		if label == "" {
			label = windows.UTF16ToString(d.DeviceString[:])
		}
		out = append(out, Monitor{
			ID:      name,
			Name:    fmt.Sprintf("Monitor %d – %s", len(out)+1, label),
			Primary: d.StateFlags&0x4 != 0, // DISPLAY_DEVICE_PRIMARY_DEVICE
		})
	}
	return out, nil
}

// ---------- alimentare ----------

func powerAction(action string) error {
	switch action {
	case "lock":
		procLockWorkStation.Call()
		return nil
	case "restart":
		return hidden(exec.Command("shutdown", "/r", "/t", "5", "/c", "FRS Remote: repornire cerută de pe Mac")).Run()
	case "restart-stream":
		_, err := sunshineReq("POST", "/api/restart", nil)
		return err
	}
	return fmt.Errorf("acțiune necunoscută: %s", action)
}
