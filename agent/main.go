// FRS Remote — agent personal Mac <-> PC Windows.
// Pe Windows rulează ca "host" (lângă Sunshine), pe Mac ca "client" (lângă Moonlight).
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Version e setat la build (-X main.Version=...).
var Version = "dev"

const (
	PCName     = "frs-pc"
	MacName    = "frs-mac"
	APIPort    = 8765
	appDirName = "FRS Remote"
)

// Porturile Sunshine/Moonlight (port de bază 47989).
var (
	streamTCPPorts = []int{47984, 47989, 48010}
	streamUDPPorts = []int{47998, 47999, 48000, 48002, 48010}
)

func isHost() bool { return runtime.GOOS == "windows" }

func myName() string {
	if isHost() {
		return PCName
	}
	return MacName
}

func peerName() string {
	if isHost() {
		return MacName
	}
	return PCName
}

// appDir = folderul de config/stare al aplicației.
func appDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	d := filepath.Join(base, appDirName)
	_ = os.MkdirAll(d, 0o755)
	return d
}

func setupLog() {
	f, err := os.OpenFile(filepath.Join(appDir(), "frsremote.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	// Sub-comenzi folosite de installerul Windows.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup-sunshine":
			setupLog()
			if err := setupSunshineCreds(); err != nil {
				log.Printf("setup-sunshine: %v", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Println(Version)
			return
		}
	}

	setupLog()
	log.Printf("FRS Remote %s pornit ca %s (%s)", Version, myName(), runtime.GOOS)

	app := newApp()
	go app.run(context.Background())
	runTray(app) // blochează pe main thread (obligatoriu pe macOS)
}

// waitFor repetă f până întoarce true sau expiră ctx.
func waitFor(ctx context.Context, every time.Duration, f func() bool) bool {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if f() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
		}
	}
}
