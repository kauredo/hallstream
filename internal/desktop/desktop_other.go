//go:build !windows && !darwin && !linux

package desktop

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Console is true where the program has a console window for its messages.
const Console = true

// Shortcut and Autostart are nil: these systems have no desktop to open on.
var (
	Shortcut  func() (string, error)
	Autostart func(on bool) error
)

// Ask has no one to ask here (the console closes with Ctrl+C): it says yes.
func Ask(msg string) bool {
	fmt.Println(msg)
	return true
}

// Fatal shows why the program can't start and waits for Enter.
func Fatal(msg string) {
	fmt.Println(msg)
	fmt.Println(i18n.T("app.enter_fecha"))
	fmt.Scanln()
}

// Warn shows a problem the program can carry on with.
func Warn(msg string) { fmt.Println(msg) }

// Run has no icon here: it waits for Ctrl+C or Stop.
func (t Tray) Run() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case <-t.Stop:
	}
	t.Quit()
}
