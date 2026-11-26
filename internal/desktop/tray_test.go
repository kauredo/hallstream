//go:build windows || darwin || linux

package desktop

import (
	"sync"
	"testing"
)

func TestConfirmThenQuit(t *testing.T) {
	var asking sync.Mutex
	stopped := 0
	stop := func() { stopped++ }
	if confirmThenQuit(&asking, func() bool { return false }, stop) || stopped != 0 {
		t.Error("a no still quit")
	}
	if !confirmThenQuit(&asking, func() bool { return true }, stop) || stopped != 1 {
		t.Error("a yes did not quit")
	}
	if !confirmThenQuit(&asking, nil, stop) || stopped != 2 {
		t.Error("with nothing to ask it did not quit")
	}
	asking.Lock()
	if confirmThenQuit(&asking, nil, stop) || stopped != 2 {
		t.Error("a second quit while the question is open went through")
	}
	asking.Unlock()
}

func TestTerminate(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	cases := []struct {
		name      string
		confirm   func() bool
		busy      bool // a question is already open
		cleanup   func()
		reply     bool
		cleanedUp bool
		logged    bool
	}{
		{"live, no", no, false, nil, false, false, false},
		{"live, yes", yes, false, nil, true, true, false},
		{"off air", nil, false, nil, true, true, false},
		{"question open", yes, true, nil, false, false, false},
		{"question panics", func() bool { panic("boom") }, false, nil, false, false, true},
		{"clean-up panics", yes, false, func() { panic("boom") }, false, true, true},
	}
	for _, c := range cases {
		var asking sync.Mutex
		if c.busy {
			asking.Lock()
		}
		cleanedUp := false
		cleanup := func() {
			cleanedUp = true
			if c.cleanup != nil {
				c.cleanup()
			}
		}
		var replies []bool
		logged := false
		terminate(&asking, c.confirm, cleanup, func(y bool) { replies = append(replies, y) }, func(string) { logged = true })
		if len(replies) != 1 || replies[0] != c.reply {
			t.Errorf("%s: replies %v, want one %v", c.name, replies, c.reply)
		}
		if cleanedUp != c.cleanedUp || logged != c.logged {
			t.Errorf("%s: cleaned up %v, logged %v", c.name, cleanedUp, logged)
		}
		if !c.busy && !asking.TryLock() {
			t.Errorf("%s: the question stayed open", c.name)
		}
	}
}
