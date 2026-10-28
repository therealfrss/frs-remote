package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

type State int

const (
	StateStarting State = iota
	StateNeedsLogin
	StateRunning
)

type App struct {
	srv *tsnet.Server
	lc  *local.Client

	mu        sync.Mutex
	state     State
	authURL   string
	peerIP    netip.Addr
	peerOn    bool
	onChange  func()
	clipSync  bool
	ready     chan struct{}
	readyOnce sync.Once
}

func newApp() *App {
	return &App{clipSync: true, ready: make(chan struct{})}
}

func (a *App) setOnChange(f func()) {
	a.mu.Lock()
	a.onChange = f
	a.mu.Unlock()
}

func (a *App) notify() {
	a.mu.Lock()
	f := a.onChange
	a.mu.Unlock()
	if f != nil {
		f()
	}
}

func (a *App) snapshot() (State, string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state, a.authURL, a.peerOn
}

func (a *App) run(ctx context.Context) {
	a.srv = &tsnet.Server{
		Hostname: myName(),
		Dir:      filepath.Join(appDir(), "net"),
		Logf:     func(string, ...any) {}, // prea vorbăreț
		UserLogf: log.Printf,
	}
	if err := a.srv.Start(); err != nil {
		log.Printf("tsnet start: %v", err)
		return
	}
	lc, err := a.srv.LocalClient()
	if err != nil {
		log.Printf("local client: %v", err)
		return
	}
	a.lc = lc

	openedLogin := false
	// Așteaptă să fim logați în rețea.
	waitFor(ctx, time.Second, func() bool {
		st, err := lc.StatusWithoutPeers(ctx)
		if err != nil {
			return false
		}
		switch st.BackendState {
		case "Running":
			a.mu.Lock()
			a.state = StateRunning
			a.authURL = ""
			a.mu.Unlock()
			a.notify()
			return true
		case "NeedsLogin":
			a.mu.Lock()
			changed := a.state != StateNeedsLogin || a.authURL != st.AuthURL
			a.state = StateNeedsLogin
			a.authURL = st.AuthURL
			a.mu.Unlock()
			if changed {
				a.notify()
			}
			if st.AuthURL != "" && !openedLogin {
				openedLogin = true
				openURL(st.AuthURL)
			}
		}
		return false
	})
	log.Printf("conectat la rețea ca %s", myName())

	// API de control, pe rețeaua privată.
	go a.serveAPI(ctx)

	// Forward porturi stream.
	if isHost() {
		go a.hostForwarders(ctx)
	} else {
		go a.clientForwarders(ctx)
	}

	go a.watchPeer(ctx)
	go a.clipboardLoop(ctx)
	go a.outboxLoop(ctx)

	a.readyOnce.Do(func() { close(a.ready) })
}

// watchPeer ține la zi IP-ul și starea celuilalt calculator.
func (a *App) watchPeer(ctx context.Context) {
	for {
		ip, online := a.findPeer(ctx)
		a.mu.Lock()
		changed := a.peerOn != online || a.peerIP != ip
		a.peerIP, a.peerOn = ip, online
		a.mu.Unlock()
		if changed {
			log.Printf("peer %s: ip=%v online=%v", peerName(), ip, online)
			a.notify()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *App) findPeer(ctx context.Context) (netip.Addr, bool) {
	st, err := a.lc.Status(ctx)
	if err != nil {
		return netip.Addr{}, false
	}
	want := peerName()
	for _, p := range st.Peer {
		dns := strings.SplitN(p.DNSName, ".", 2)[0]
		if strings.EqualFold(p.HostName, want) || strings.EqualFold(dns, want) {
			for _, ip := range p.TailscaleIPs {
				if ip.Is4() {
					return ip, p.Online
				}
			}
		}
	}
	return netip.Addr{}, false
}

func (a *App) peerAddr(port int) (string, error) {
	a.mu.Lock()
	ip := a.peerIP
	a.mu.Unlock()
	if !ip.IsValid() {
		return "", fmt.Errorf("%s nu e găsit în rețea", peerName())
	}
	return net.JoinHostPort(ip.String(), fmt.Sprint(port)), nil
}

func (a *App) myIP4() netip.Addr {
	ip4, _ := a.srv.TailscaleIPs()
	return ip4
}
