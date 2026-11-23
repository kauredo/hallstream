// Package desktop is what the program does on the Windows desktop: the icon
// by the clock, message boxes, the clipboard and the desktop shortcut. On
// other systems (development) it keeps to the console.
package desktop

// Tray is the icon by the clock and its menu.
type Tray struct {
	Icon    []byte // .ico
	Tooltip string
	Panel   func()          // left click and "Abrir painel"
	Overlay string          // what "Copiar endereço do overlay" copies
	Status  func() string   // the grey line in the menu
	Changed <-chan struct{} // Status may have changed
	// ConfirmQuit may stop "Sair" by returning false; nil always quits.
	ConfirmQuit func() bool
	Quit        func() // cleans up; runs once, before Run returns
	Log         func(string)
}
