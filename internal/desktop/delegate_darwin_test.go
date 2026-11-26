package desktop

import (
	"testing"
	"time"
)

// TestDelegateHooks also passes with -count=2: adding a handler twice is
// fine.
func TestDelegateHooks(t *testing.T) {
	ran := make(chan string, 2)
	if !onReopen(func() { ran <- "reopen" }) {
		t.Fatal("reopen: could not add the handler to systray's app delegate")
	}
	if !onQuit(func(reply func(bool)) { ran <- "quit" }) {
		t.Fatal("quit: could not add the handler to systray's app delegate")
	}
	hallstreamReopen()
	hallstreamQuit()
	for range 2 {
		select {
		case <-ran:
		case <-time.After(time.Second):
			t.Fatal("an event did not reach the program")
		}
	}
	if !hookReopen() || !hookTerminate() {
		t.Error("adding the same handler again failed")
	}
}
