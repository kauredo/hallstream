package desktop

import (
	"sync"

	"fyne.io/systray"
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
		open := systray.AddMenuItem("Abrir painel", "")
		cp := systray.AddMenuItem("Copiar endereço do overlay", t.Overlay)
		systray.AddSeparator()
		status := systray.AddMenuItem(t.Status(), "")
		status.Disable()
		systray.AddSeparator()
		exit := systray.AddMenuItem("Sair", "Fechar o Hallstream")
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
						t.Log("Não consegui copiar o endereço: " + err.Error())
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
