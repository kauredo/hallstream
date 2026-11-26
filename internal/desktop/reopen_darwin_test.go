package desktop

import (
	"testing"
	"time"
)

func TestReopenHook(t *testing.T) {
	opened := make(chan struct{}, 1)
	if !onReopen(func() { opened <- struct{}{} }) {
		t.Fatal("could not add the reopen handler to systray's app delegate")
	}
	hallstreamReopen()
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Error("a reopen event did not open the panel")
	}
}
