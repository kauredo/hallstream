//go:build darwin || linux

package obsapp

import (
	"syscall"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

// stop sends SIGINT to every pid and waits up to 15 s for OBS to go. Still
// running then, it kills OBS when force is true.
func stop(pids func() ([]int, error), running func() (bool, error), force bool) error {
	ids, err := pids()
	if err != nil || len(ids) == 0 {
		return err
	}
	for _, id := range ids {
		syscall.Kill(id, syscall.SIGINT)
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if on, err := running(); err == nil && !on {
			return nil
		}
	}
	if !force {
		return i18n.Err("obs.nao_fechou")
	}
	ids, err = pids()
	for _, id := range ids {
		syscall.Kill(id, syscall.SIGKILL)
	}
	return err
}
