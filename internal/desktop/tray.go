//go:build windows || darwin || linux

package desktop

import (
	"runtime"
	"sync"

	"fyne.io/systray"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Run shows the icon and blocks until "Sair" (or Windows logging off) has
// cleaned up. Call it from main: the tray needs the main thread.
func (t Tray) Run() {
	var once sync.Once
	quit := func() { once.Do(t.Quit) }
	var asking sync.Mutex
	sair := func() bool {
		return confirmThenQuit(&asking, t.ConfirmQuit, func() {
			quit()
			systray.Quit()
		})
	}
	logLine = t.Log
	if !onReopen(t.Panel) {
		t.Log(i18n.T("app.dock_falhou", "evento", "reopen"))
	}
	// macOS ends the app itself once the reply is yes, so this path does
	// not call systray.Quit: that waits on the main thread, which AppKit
	// holds while it waits for the reply.
	if !onQuit(func(reply func(bool)) { terminate(&asking, t.ConfirmQuit, quit, reply, t.Log) }) {
		t.Log(i18n.T("app.dock_falhou", "evento", "terminate"))
	}
	systray.Run(func() {
		watchSignals(func() {
			quit()
			systray.Quit()
		})
		systray.SetIcon(t.Icon)
		systray.SetTooltip(t.Tooltip)
		// On a Mac a click on the menu bar icon shows the menu.
		if runtime.GOOS != "darwin" {
			systray.SetOnTapped(t.Panel)
		}
		open := systray.AddMenuItem(i18n.T("app.bandeja.abrir"), "")
		cp := systray.AddMenuItem(i18n.T("app.bandeja.copiar"), t.Overlay)
		systray.AddSeparator()
		status := systray.AddMenuItem(t.Status(), "")
		status.Disable()
		systray.AddSeparator()
		exit := systray.AddMenuItem(i18n.T("app.bandeja.sair"), i18n.T("app.bandeja.sair_dica"))
		go func() {
			last := ""
			for range t.Changed {
				if s := t.Status(); s != last {
					status.SetTitle(s)
					last = s
				}
			}
		}()
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					t.Panel()
				case <-cp.ClickedCh:
					if err := CopyText(t.Overlay); err != nil {
						t.Log(i18n.T("app.bandeja.nao_copiei", "err", err))
					}
				case <-exit.ClickedCh:
					if sair() {
						return
					}
				}
			}
		}()
	}, quit)
}

// confirmThenQuit is "Sair": it runs stop unless confirm says no. While one
// question is open, another request to quit is dropped.
func confirmThenQuit(asking *sync.Mutex, confirm func() bool, stop func()) bool {
	if !asking.TryLock() {
		return false
	}
	defer asking.Unlock()
	if confirm != nil && !confirm() {
		return false
	}
	stop()
	return true
}

// terminate answers the system's request to end the app: it runs cleanup
// and replies yes, unless confirm says no or a question is already open.
// It replies exactly once, after a panic too (a no, logged).
func terminate(asking *sync.Mutex, confirm func() bool, cleanup func(), reply func(bool), log func(string)) {
	yes := false
	defer func() {
		if r := recover(); r != nil {
			yes = false
			log(i18n.T("app.sair_falhou", "err", r))
		}
		reply(yes)
	}()
	yes = confirmThenQuit(asking, confirm, cleanup)
}
