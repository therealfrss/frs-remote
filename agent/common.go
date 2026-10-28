package main

import (
	_ "embed"
	"time"
)

//go:embed icons/tray_mac.png
var iconMac []byte

//go:embed icons/tray_win.ico
var iconWin []byte

func sleepCh(ms int) <-chan time.Time { return time.After(time.Duration(ms) * time.Millisecond) }
