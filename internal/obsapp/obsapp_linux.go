package obsapp

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/kauredo/hallstream/internal/i18n"
)

// System is OBS on this Linux PC, native or from Flathub. Close is nil
// until someone checks on a Linux PC that SIGINT closes OBS cleanly there;
// closeOBS is ready for it.
func System() App {
	path := configPath()
	dir, derr := os.UserConfigDir()
	home, herr := os.UserHomeDir()
	if derr == nil && herr == nil {
		path = linuxConfigPath(dir, home, dirExists)
	}
	return App{ConfigPath: path, Running: running, Start: start}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// launcher is obs on the PATH, else the Flatpak.
func launcher() (string, []string, error) {
	if p, err := exec.LookPath("obs"); err == nil {
		return p, startArgs(), nil
	}
	if p, err := exec.LookPath("flatpak"); err == nil && exec.Command(p, "info", flatpakID).Run() == nil {
		return p, append([]string{"run", flatpakID}, startArgs()...), nil
	}
	return "", nil, i18n.Err("obs.nao_instalado")
}

// pids are this user's processes named obs; the Flatpak's has that name
// too.
func pids() ([]int, error) { return pidsIn("/proc", "obs", os.Getuid()) }

func running() (bool, error) {
	ids, err := pids()
	return len(ids) > 0, err
}

// start runs OBS with its window showing, in its own session, so closing a
// terminal does not close OBS.
func start() error {
	name, args, err := launcher()
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return i18n.Err("obs.nao_abri", "err", err)
	}
	go cmd.Wait()
	return nil
}

// closeOBS sends SIGINT, as on macOS, and kills OBS after 15 s when force
// is true.
func closeOBS(force bool) error {
	return stop(pids, running, force)
}
