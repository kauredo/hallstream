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
