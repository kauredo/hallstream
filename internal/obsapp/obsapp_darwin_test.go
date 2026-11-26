package obsapp

import "testing"

func TestPids(t *testing.T) {
	if _, err := pids(); err != nil {
		t.Errorf("pgrep: %v", err)
	}
}
