package desktop

import (
	"testing"
	"time"
)

func TestDelegateHooks(t *testing.T) {
	cases := []struct {
		name  string
		hook  func(func()) bool
		event func()
	}{
		{"reopen", onReopen, hallstreamReopen},
		{"quit", onQuit, hallstreamQuit},
	}
	for _, c := range cases {
		ran := make(chan struct{}, 1)
		if !c.hook(func() { ran <- struct{}{} }) {
			t.Fatalf("%s: could not add the handler to systray's app delegate", c.name)
		}
		c.event()
		select {
		case <-ran:
		case <-time.After(time.Second):
			t.Errorf("%s: the event did not reach the program", c.name)
		}
	}
}
