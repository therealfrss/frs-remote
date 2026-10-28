package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// ---------- PC (host): rețeaua privată -> Sunshine pe 127.0.0.1 ----------

func (a *App) hostForwarders(ctx context.Context) {
	ip := a.myIP4()
	for _, p := range streamTCPPorts {
		p := p
		ln, err := a.srv.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			log.Printf("listen tcp %d: %v", p, err)
			continue
		}
		go acceptLoop(ctx, ln, func() (net.Conn, error) {
			return net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 5*time.Second)
		})
	}
	for _, p := range streamUDPPorts {
		p := p
		pc, err := a.srv.ListenPacket("udp", fmt.Sprintf("%s:%d", ip, p))
		if err != nil {
			log.Printf("listen udp %d: %v", p, err)
			continue
		}
		go hostUDP(ctx, pc, fmt.Sprintf("127.0.0.1:%d", p))
	}
	log.Printf("host: porturile de stream sunt deschise pe %s", ip)
}

// hostUDP: pentru fiecare adresă care trimite pachete, un socket local spre Sunshine.
func hostUDP(ctx context.Context, pc net.PacketConn, target string) {
	type sess struct {
		c    net.Conn
		last time.Time
	}
	var mu sync.Mutex
	sessions := map[string]*sess{}

	go func() { // curățenie
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			mu.Lock()
			for k, s := range sessions {
				if time.Since(s.last) > 2*time.Minute {
					s.c.Close()
					delete(sessions, k)
				}
			}
			mu.Unlock()
		}
	}()

	buf := make([]byte, 65535)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			log.Printf("udp read %s: %v", target, err)
			return
		}
		key := from.String()
		mu.Lock()
		s := sessions[key]
		if s == nil {
			c, err := net.Dial("udp", target)
			if err != nil {
				mu.Unlock()
				continue
			}
			s = &sess{c: c}
			sessions[key] = s
			go func(c net.Conn, to net.Addr) {
				b := make([]byte, 65535)
				for {
					m, err := c.Read(b)
					if err != nil {
						return
					}
					pc.WriteTo(b[:m], to)
				}
			}(c, from)
		}
		s.last = time.Now()
		mu.Unlock()
		s.c.Write(buf[:n])
	}
}

// ---------- Mac (client): 127.0.0.1 -> PC prin rețeaua privată ----------

func (a *App) clientForwarders(ctx context.Context) {
	for _, p := range streamTCPPorts {
		p := p
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			log.Printf("listen local tcp %d: %v", p, err)
			continue
		}
		go acceptLoop(ctx, ln, func() (net.Conn, error) {
			addr, err := a.peerAddr(p)
			if err != nil {
				return nil, err
			}
			dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return a.srv.Dial(dctx, "tcp", addr)
		})
	}
	for _, p := range streamUDPPorts {
		p := p
		lc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			log.Printf("listen local udp %d: %v", p, err)
			continue
		}
		go a.clientUDP(ctx, lc, p)
	}
	log.Printf("client: porturile de stream ascultă pe 127.0.0.1")
}

func (a *App) clientUDP(ctx context.Context, local net.PacketConn, port int) {
	var mu sync.Mutex
	var remote net.Conn
	var lastFrom net.Addr

	buf := make([]byte, 65535)
	for {
		n, from, err := local.ReadFrom(buf)
		if err != nil {
			return
		}
		mu.Lock()
		lastFrom = from
		r := remote
		mu.Unlock()
		if r == nil {
			addr, err := a.peerAddr(port)
			if err != nil {
				continue
			}
			dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			r, err = a.srv.Dial(dctx, "udp", addr)
			cancel()
			if err != nil {
				log.Printf("dial udp %d: %v", port, err)
				continue
			}
			mu.Lock()
			remote = r
			mu.Unlock()
			go func(r net.Conn) {
				b := make([]byte, 65535)
				for {
					m, err := r.Read(b)
					if err != nil {
						mu.Lock()
						if remote == r {
							remote = nil
						}
						mu.Unlock()
						r.Close()
						return
					}
					mu.Lock()
					to := lastFrom
					mu.Unlock()
					if to != nil {
						local.WriteTo(b[:m], to)
					}
				}
			}(r)
		}
		if _, err := r.Write(buf[:n]); err != nil {
			mu.Lock()
			if remote == r {
				remote = nil
			}
			mu.Unlock()
			r.Close()
		}
	}
}

// ---------- comun ----------

func acceptLoop(ctx context.Context, ln net.Listener, dial func() (net.Conn, error)) {
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			d, err := dial()
			if err != nil {
				log.Printf("forward: %v", err)
				return
			}
			defer d.Close()
			pipe(c, d)
		}(c)
	}
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
