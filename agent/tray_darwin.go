//go:build darwin

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"fyne.io/systray"
)

const maxMonitors = 6

func runTray(a *App) {
	systray.Run(func() { trayReady(a) }, func() { os.Exit(0) })
}

func trayReady(a *App) {
	systray.SetTemplateIcon(iconMac, iconMac)
	systray.SetTooltip("FRS Remote")

	mStatus := systray.AddMenuItem("Pornește…", "")
	mStatus.Disable()
	mLogin := systray.AddMenuItem("Conectează-te la rețea (prima dată)", "")
	mLogin.Hide()
	systray.AddSeparator()

	mConnect := systray.AddMenuItem("Conectează la PC", "Deschide ecranul PC-ului")
	mMon := systray.AddMenuItem("Monitor", "Alege ce monitor vezi")
	mMonAuto := mMon.AddSubMenuItemCheckbox("Principal (automat)", "", false)
	var monItems []*systray.MenuItem
	for i := 0; i < maxMonitors; i++ {
		it := mMon.AddSubMenuItemCheckbox("", "", false)
		it.Hide()
		monItems = append(monItems, it)
	}
	systray.AddSeparator()

	mSend := systray.AddMenuItem("Trimite fișiere pe PC…", "")
	mInbox := systray.AddMenuItem("Fișiere primite de pe PC", "")
	mOutbox := systray.AddMenuItem("Folderul „Trimite pe PC”", "Ce pui aici pleacă singur pe PC")
	mClip := systray.AddMenuItemCheckbox("Clipboard comun", "Copy pe un calculator, paste pe celălalt", true)
	systray.AddSeparator()

	mPC := systray.AddMenuItem("PC", "")
	mLock := mPC.AddSubMenuItem("Blochează PC-ul", "")
	mRestartStream := mPC.AddSubMenuItem("Repornește serverul de stream", "")
	mRestartPC := mPC.AddSubMenuItem("Repornește PC-ul…", "")
	mPair := systray.AddMenuItem("Împerechează cu PC-ul (prima dată)", "")
	mLoginItem := systray.AddMenuItemCheckbox("Pornește odată cu Mac-ul", "", isLoginItem())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Ieșire", "")

	monIDs := make([]string, maxMonitors)

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
			mStatus.SetTitle("● PC online")
			mLogin.Hide()
		default:
			mStatus.SetTitle("○ PC offline")
			mLogin.Hide()
		}
		for _, m := range []*systray.MenuItem{mConnect, mMon, mSend, mPC, mPair} {
			if online {
				m.Enable()
			} else {
				m.Disable()
			}
		}
	}

	loadMonitors := func() {
		b, err := a.call("GET", "/monitors", nil, nil, 10*time.Second)
		if err != nil {
			return
		}
		var mons []Monitor
		if json.Unmarshal(b, &mons) != nil {
			return
		}
		anyCur := false
		for i, it := range monItems {
			if i < len(mons) {
				m := mons[i]
				title := m.Name
				if m.Primary {
					title += " (principal)"
				}
				it.SetTitle(title)
				monIDs[i] = m.ID
				if m.Current {
					it.Check()
					anyCur = true
				} else {
					it.Uncheck()
				}
				it.Show()
			} else {
				it.Hide()
				monIDs[i] = ""
			}
		}
		if anyCur {
			mMonAuto.Uncheck()
		} else {
			mMonAuto.Check()
		}
	}

	wasOnline := false
	a.setOnChange(func() {
		refresh()
		_, _, online := a.snapshot()
		if online && !wasOnline {
			go loadMonitors()
		}
		wasOnline = online
	})
	refresh()

	selectMonitor := func(id string) {
		if _, err := a.call("POST", "/monitor", url.Values{"id": {id}}, nil, 20*time.Second); err != nil {
			alertUser("Nu am putut schimba monitorul: " + err.Error())
			return
		}
		notifyUser("Monitor schimbat", "Reconectează stream-ul dacă era deschis.")
		time.Sleep(2 * time.Second)
		loadMonitors()
	}

	go func() {
		for {
			select {
			case <-mLogin.ClickedCh:
				if _, u, _ := a.snapshot(); u != "" {
					openURL(u)
				}
			case <-mConnect.ClickedCh:
				if err := startStream(); err != nil {
					alertUser("Nu pot porni stream-ul: " + err.Error())
				}
			case <-mMonAuto.ClickedCh:
				go selectMonitor("")
			case <-mSend.ClickedCh:
				go func() {
					files := pickFiles()
					ok := 0
					for _, f := range files {
						if err := a.sendFile(f); err != nil {
							log.Printf("send %s: %v", f, err)
							alertUser(fmt.Sprintf("Eroare la %s: %v", f, err))
							continue
						}
						ok++
					}
					if ok > 0 {
						notifyUser("Trimis pe PC", fmt.Sprintf("%d fișier(e) în „De pe Mac” de pe Desktop", ok))
					}
				}()
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
			case <-mLock.ClickedCh:
				go a.call("POST", "/power", url.Values{"action": {"lock"}}, nil, 10*time.Second)
			case <-mRestartStream.ClickedCh:
				go a.call("POST", "/power", url.Values{"action": {"restart-stream"}}, nil, 10*time.Second)
			case <-mRestartPC.ClickedCh:
				go func() {
					if confirm("Repornești PC-ul de acasă? Revine singur în 1–2 minute.", "Repornește") {
						if _, err := a.call("POST", "/power", url.Values{"action": {"restart"}}, nil, 10*time.Second); err != nil {
							alertUser("Eroare: " + err.Error())
						}
					}
				}()
			case <-mPair.ClickedCh:
				go func() {
					if err := a.pair(); err != nil {
						alertUser("Împerecherea n-a mers: " + err.Error())
						return
					}
					notifyUser("Gata", "Mac-ul e împerecheat cu PC-ul.")
				}()
			case <-mLoginItem.ClickedCh:
				if mLoginItem.Checked() {
					setLoginItem(false)
					mLoginItem.Uncheck()
				} else {
					setLoginItem(true)
					mLoginItem.Check()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	for i, it := range monItems {
		i, it := i, it
		go func() {
			for range it.ClickedCh {
				if id := monIDs[i]; id != "" {
					selectMonitor(id)
				}
			}
		}()
	}
}
