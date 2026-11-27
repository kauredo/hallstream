//go:build darwin || linux

package obsapp

import (
	"os/exec"
	"testing"
	"time"
)

func TestStopSendsSIGINT(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	pids := func() ([]int, error) {
		select {
		case <-done:
			return nil, nil
		default:
			return []int{cmd.Process.Pid}, nil
		}
	}
	running := func() (bool, error) {
		ids, err := pids()
		return len(ids) > 0, err
	}
	start := time.Now()
	if err := stop(pids, running, false); err != nil {
		t.Fatal(err)
	}
	if !cmd.ProcessState.Exited() && cmd.ProcessState.String() != "signal: interrupt" {
		t.Errorf("ended by %s", cmd.ProcessState)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
	if err := stop(pids, running, false); err != nil {
		t.Errorf("nothing to stop: %v", err)
	}
}
