package main

import (
	"context"
	"crypto/sha256"
	"log"
	"sync"

	"golang.design/x/clipboard"
)

var (
	clipMu       sync.Mutex
	lastClipHash [32]byte // ultimul conținut venit de la celălalt; nu-l trimitem înapoi
)

func (a *App) clipEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.clipSync
}

func (a *App) setClipEnabled(v bool) {
	a.mu.Lock()
	a.clipSync = v
	a.mu.Unlock()
}

func (a *App) clipboardLoop(ctx context.Context) {
	if err := clipboard.Init(); err != nil {
		log.Printf("clipboard indisponibil: %v", err)
		return
	}
	text := clipboard.Watch(ctx, clipboard.FmtText)
	img := clipboard.Watch(ctx, clipboard.FmtImage)
	for {
		var format string
		var b []byte
		select {
		case <-ctx.Done():
			return
		case b = <-text:
			format = "text"
		case b = <-img:
			format = "image"
		}
		if len(b) == 0 || !a.clipEnabled() {
			continue
		}
		h := sha256.Sum256(b)
		clipMu.Lock()
		dup := h == lastClipHash
		clipMu.Unlock()
		if dup {
			continue
		}
		if _, _, online := a.snapshot(); !online {
			continue
		}
		if err := a.putClipboard(format, b); err != nil {
			log.Printf("clipboard -> %s: %v", peerName(), err)
		}
	}
}

func (a *App) applyRemoteClipboard(format string, b []byte) {
	if !a.clipEnabled() || len(b) == 0 {
		return
	}
	clipMu.Lock()
	lastClipHash = sha256.Sum256(b)
	clipMu.Unlock()
	f := clipboard.FmtText
	if format == "image" {
		f = clipboard.FmtImage
	}
	clipboard.Write(f, b)
}
