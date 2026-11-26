package obsapp

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

// System is OBS on this Mac.
func System() App {
	return App{ConfigPath: configPath(), Running: running, Start: start, Close: closeOBS}
}

// Path is the first OBS.app found.
func Path() (string, error) {
	home, _ := os.UserHomeDir()
	for _, p := range macCandidates(home) {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, nil
		}
	}
	return "", i18n.Err("obs.nao_instalado")
}

// pids are the processes named OBS.
func pids() ([]int, error) {
	out, err := exec.Command("/usr/bin/pgrep", "-x", "OBS").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func running() (bool, error) {
	ids, err := pids()
	return len(ids) > 0, err
}

// start opens OBS with its window showing. open returns at once, and OBS
// runs on its own, not as this program's child.
func start() error {
	app, err := Path()
	if err != nil {
		return err
	}
	if err := exec.Command("/usr/bin/open", append([]string{"-a", app, "--args"}, startArgs()...)...).Run(); err != nil {
		return i18n.Err("obs.nao_abri", "err", err)
	}
	return nil
}

// closeOBS sends SIGINT, which OBS handles like its Quit: it saves its
// settings and exits. If it is still running after 15 s and force is true,
// it is killed.
func closeOBS(force bool) error {
	return stop(pids, running, force)
}
