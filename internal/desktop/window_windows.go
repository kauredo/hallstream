package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
)

// HasWindow is false: the panel's Edge window is a process of its own, so it
// outlives an update's restart and reloads onto the new version by itself.
var HasWindow = false

// ShowPanel opens the panel as an Edge (else Chrome) app window. False when
// neither is installed, and the caller opens the default browser.
func ShowPanel(url string) bool {
	base := filepath.Join(os.Getenv("LOCALAPPDATA"), "Hallstream")
	if os.Getenv("LOCALAPPDATA") == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			return false
		}
		base = filepath.Join(dir, "Hallstream")
	}
	for _, b := range appBrowsers(os.Getenv, base) {
		if _, err := os.Stat(b.exe); err != nil {
			continue
		}
		cmd := exec.Command(b.exe, appArgs(url, b.profile)...)
		if cmd.Start() == nil {
			go cmd.Wait()
			return true
		}
	}
	return false
}
