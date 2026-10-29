//go:build windows

// Package ui provides the Windows tray icon, toast notifications and auto-start toggle.
package ui

import (
	"context"
	_ "embed"
	"log"
	"os/exec"

	"github.com/getlantern/systray"
)

var (
	//go:embed icons/idle.ico
	iconIdle []byte
	//go:embed icons/call.ico
	iconCall []byte
)

type Tray struct {
	status *systray.MenuItem
}

// RunTray blocks on the calling (main) goroutine until Exit is clicked or ctx is
// cancelled, then calls onExit.
// onReady receives the Tray once the icon exists; it is the place to start polling.
// "View logs" opens logDir in Explorer (the folder, not a file: logs can be big).
func RunTray(ctx context.Context, onReady func(*Tray), onExit func(), logDir string) {
	go func() { <-ctx.Done(); systray.Quit() }()
	systray.Run(func() {
		systray.SetIcon(iconIdle)
		systray.SetTooltip("Teams Monitor: Idle")
		t := &Tray{status: systray.AddMenuItem("Status: Idle", "")}
		t.status.Disable()
		auto := systray.AddMenuItemCheckbox("Start with Windows", "", AutoStartEnabled())
		logs := systray.AddMenuItem("View logs", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Exit", "")

		go func() {
			for {
				select {
				case <-auto.ClickedCh:
					if err := SetAutoStart(!auto.Checked()); err != nil {
						log.Printf("ui: auto-start: %v", err)
					}
					if AutoStartEnabled() {
						auto.Check()
					} else {
						auto.Uncheck()
					}
				case <-logs.ClickedCh:
					// explorer.exe exits 1 even on success, so only a failure to launch counts
					if err := exec.Command("explorer.exe", logDir).Start(); err != nil {
						log.Printf("ui: view logs: %v", err)
					}
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
		onReady(t)
	}, onExit)
}

func (t *Tray) SetInCall(inCall bool) {
	if inCall {
		systray.SetIcon(iconCall)
		systray.SetTooltip("Teams Monitor: In Call")
		t.status.SetTitle("Status: In Call")
	} else {
		systray.SetIcon(iconIdle)
		systray.SetTooltip("Teams Monitor: Idle")
		t.status.SetTitle("Status: Idle")
	}
}
