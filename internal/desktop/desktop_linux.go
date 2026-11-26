package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Console is true: a terminal may be attached, and printing is harmless
// when none is.
const Console = true

// Shortcut is nil: Linux gets no desktop shortcut yet.
var Shortcut func() (string, error)

// Fatal shows why the program can't start: in a zenity box when there is
// one, else on the console, waiting for Enter when it is a terminal.
func Fatal(msg string) {
	fmt.Println(msg)
	if zenity("--error", msg) == nil {
		return
	}
	if _, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS); err == nil {
		fmt.Println(i18n.T("app.enter_fecha"))
		fmt.Scanln()
	}
}

// Warn shows a problem the program can carry on with.
func Warn(msg string) {
	fmt.Println(msg)
	zenity("--warning", msg)
}

// Ask shows a yes/no question; true is yes. No is the default button.
// Without zenity there is no one to ask (the console closes with Ctrl+C),
// so it says yes.
func Ask(msg string) bool {
	fmt.Println(msg)
	if _, err := exec.LookPath("zenity"); err != nil {
		return true
	}
	return zenity("--question", msg, "--ok-label="+i18n.T("app.sim"), "--cancel-label="+i18n.T("app.nao"), "--default-cancel") == nil
}

// zenity shows a box; the error is also how it answers no.
func zenity(kind, msg string, extra ...string) error {
	path, err := exec.LookPath("zenity")
	if err != nil {
		return err
	}
	return exec.Command(path, append([]string{kind, "--title=Hallstream", "--text=" + msg}, extra...)...).Run()
}

// CopyText puts s on the clipboard with the first tool it finds.
func CopyText(s string) error {
	for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}} {
		path, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c[1:]...)
		cmd.Stdin = strings.NewReader(s)
		return cmd.Run()
	}
	return i18n.Err("app.sem_area_transferencia")
}

// Autostart puts hallstream.desktop in the autostart folder, so the
// desktop opens the program at sign-in, or takes it out.
var Autostart = func(on bool) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "autostart")
	file := filepath.Join(dir, "hallstream.desktop")
	if !on {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, autostartEntry(exe), 0o644)
}
