//go:build windows

package main

import (
	"os"

	"fyne.io/systray"
)

func runTray(a *App) {
	systray.Run(func() { trayReady(a) }, func() { os.Exit(0) })
}

func trayReady(a *App) {
	systray.SetIcon(iconWin)
	systray.SetTitle("FRS Remote")
	systray.SetTooltip("FRS Remote")

	mStatus := systray.AddMenuItem("Pornește…", "")
	mStatus.Disable()
	mLogin := systray.AddMenuItem("Conectează-te la rețea (prima dată)", "")
	mLogin.Hide()
	systray.AddSeparator()
	mInbox := systray.AddMenuItem("Fișiere primite de pe Mac", "")
	mOutbox := systray.AddMenuItem("Folderul „Trimite pe Mac”", "Ce pui aici pleacă singur pe Mac")
	mClip := systray.AddMenuItemCheckbox("Clipboard comun", "", true)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Oprește FRS Remote", "")

	refresh := func() {
		st, authURL, online := a.snapshot()
		switch {
		case st == StateNeedsLogin:
			mStatus.SetTitle("Nu ești conectat la rețea")
			if authURL != "" {
				mLogin.Show()
			}
		case st == StateStarting:
			mStatus.SetTitle("Pornește…")
		case online:
			mStatus.SetTitle("● Mac online")
			mLogin.Hide()
		default:
			mStatus.SetTitle("○ Mac offline — PC-ul te așteaptă")
			mLogin.Hide()
		}
	}
	a.setOnChange(refresh)
	refresh()

	go func() {
		for {
			select {
			case <-mLogin.ClickedCh:
				if _, u, _ := a.snapshot(); u != "" {
					openURL(u)
				}
			case <-mInbox.ClickedCh:
				openFolder(inboxDir())
			case <-mOutbox.ClickedCh:
				openFolder(outboxDir())
			case <-mClip.ClickedCh:
				if mClip.Checked() {
					mClip.Uncheck()
					a.setClipEnabled(false)
				} else {
					mClip.Check()
					a.setClipEnabled(true)
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
