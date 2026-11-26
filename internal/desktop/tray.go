package desktop

import (
	"sync"

	"fyne.io/systray"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Run shows the icon and blocks until "Sair" (or Windows logging off) has
// cleaned up. Call it from main: the tray needs the main thread.
func (t Tray) Run() {
	var once sync.Once
	quit := func() { once.Do(t.Quit) }
	systray.Run(func() {
		systray.SetIcon(t.Icon)
		systray.SetTooltip(t.Tooltip)
		systray.SetOnTapped(t.Panel)
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
					if t.ConfirmQuit != nil && !t.ConfirmQuit() {
						continue
					}
					quit()
					systray.Quit()
					return
				}
			}
		}()
	}, quit)
}
