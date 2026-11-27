// Package desktop is what the program does on the Windows, macOS and Linux
// desktops: the icon by the clock or in the menu bar, message boxes, the
// clipboard, opening at sign-in and the desktop shortcut. On other systems
// it keeps to the console.
package desktop

// Tray is the icon by the clock (in the menu bar on a Mac) and its menu.
type Tray struct {
	Icon    []byte // .ico on Windows, PNG elsewhere
	Tooltip string
	Panel   func()          // left click and "Abrir painel"
	Overlay string          // what "Copiar endereço do overlay" copies
	Status  func() string   // the grey line in the menu
	Changed <-chan struct{} // Status may have changed
	// ConfirmQuit may stop "Sair" by returning false; nil always quits.
	ConfirmQuit func() bool
	Quit        func() // cleans up; runs once, before Run returns
	// Stop, once closed, quits like "Sair" without asking.
	Stop <-chan struct{}
	Log  func(string)
}

// logLine is Tray.Log once Run starts, for problems Ask can only log.
var logLine = func(string) {}
