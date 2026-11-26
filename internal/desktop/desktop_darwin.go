package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Console is true outside the app bundle (go run, a scratch build), where a
// terminal shows the messages.
var Console = inConsole()

func inConsole() bool {
	exe, err := os.Executable()
	return err != nil || bundlePath(exe) == ""
}

// Shortcut is nil: Mac users keep apps in Applications and the Dock.
var Shortcut func() (string, error)

// Autostart is nil outside the app bundle, so the panel hides the switch.
var Autostart = autostartFunc()

func autostartFunc() func(bool) error {
	if Console {
		return nil
	}
	return autostart
}

// Fatal shows why the program can't start, in an alert.
func Fatal(msg string) { alert(msg, "critical") }

// Warn shows a problem the program can carry on with.
func Warn(msg string) { alert(msg, "warning") }

// alert passes msg as an argument, never inside the script, so a quote in
// it is only ever data.
func alert(msg, kind string) {
	if Console {
		fmt.Println(msg)
	}
	exec.Command("/usr/bin/osascript", "-e", "on run argv",
		"-e", `display alert "Hallstream" message (item 1 of argv) as `+kind,
		"-e", "end run", msg).Run()
}

// Ask shows a yes/no question; true is yes. No is the default button.
func Ask(msg string) bool {
	if Console {
		fmt.Println(msg)
	}
	yes := i18n.T("app.sim")
	out, err := exec.Command("/usr/bin/osascript", "-e", "on run argv",
		"-e", `display dialog (item 1 of argv) with title "Hallstream" buttons {(item 2 of argv), (item 3 of argv)} default button (item 2 of argv) with icon caution`,
		"-e", "end run", msg, i18n.T("app.nao"), yes).Output()
	return err == nil && strings.Contains(string(out), "button returned:"+yes)
}

// CopyText puts s on the clipboard.
func CopyText(s string) error {
	cmd := exec.Command("/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// autostart writes or removes the LaunchAgent that opens the app at
// sign-in.
func autostart(on bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	plist := filepath.Join(dir, bundleID+".plist")
	if !on {
		if err := os.Remove(plist); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	bundle := bundlePath(exe)
	switch {
	case bundle == "":
		return i18n.Err("app.arranque_sem_app")
	case translocated(exe):
		return i18n.Err("app.arranque_mover")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(plist, launchAgent(bundleID, bundle), 0o644)
}
