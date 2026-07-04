// Package tray puts a small icon in the Windows notification area so a
// streamer running the app as a windowed-GUI (no console) process still
// has a way to reopen the dashboard, pause/resume, or quit — otherwise
// closing the browser tab would leave no way to interact with a
// no-console background process at all.
package tray

import (
	_ "embed"

	"fyne.io/systray"
)

//go:embed icon.ico
var iconBytes []byte

// Callbacks are invoked from the menu. All are called on the systray
// library's own goroutine; callers should keep them fast/non-blocking
// (e.g. sending on a channel) and not assume any particular goroutine.
type Callbacks struct {
	OnOpenDashboard func()
	OnTogglePause   func() (nowPaused bool)
	OnQuit          func()
}

// Run blocks until Quit is called (from the menu or via ForceQuit). Call
// it from its own goroutine; systray needs the OS message loop, which on
// Windows it manages internally.
func Run(cb Callbacks) {
	systray.Run(func() { onReady(cb) }, func() {})
}

// ForceQuit programmatically triggers the same shutdown path as clicking
// Quit — used when the app is stopped some other way (e.g. Ctrl+C in a
// console build) and needs to tear down the tray icon too.
func ForceQuit() {
	systray.Quit()
}

func onReady(cb Callbacks) {
	systray.SetIcon(iconBytes)
	systray.SetTitle("Streamer Remote")
	systray.SetTooltip("Streamer Remote")

	open := systray.AddMenuItem("Open Dashboard", "Open the dashboard in your browser")
	toggle := systray.AddMenuItem("Pause", "Pause or resume the remote")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop Streamer Remote")

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				if cb.OnOpenDashboard != nil {
					cb.OnOpenDashboard()
				}
			case <-toggle.ClickedCh:
				if cb.OnTogglePause != nil {
					if cb.OnTogglePause() {
						toggle.SetTitle("Resume")
					} else {
						toggle.SetTitle("Pause")
					}
				}
			case <-quit.ClickedCh:
				if cb.OnQuit != nil {
					cb.OnQuit()
				}
				systray.Quit()
				return
			}
		}
	}()
}
