package main

import (
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/web"
)

func TestRestartArgs(t *testing.T) {
	for _, c := range []struct{ in, want []string }{
		{nil, []string{"-nobrowser", "-restarted"}},
		{[]string{"-demo"}, []string{"-demo", "-nobrowser", "-restarted"}},
		{[]string{"-demo", "--nobrowser", "-restarted=true", "-replay", "x.capture"}, []string{"-demo", "-replay", "x.capture", "-nobrowser", "-restarted"}},
	} {
		if got := restartArgs(c.in, false); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q", c.in, got)
		}
	}
	// With a window of its own, the new copy opens it: the old one closed with the old copy.
	if got := restartArgs([]string{"-demo", "-nobrowser"}, true); !slices.Equal(got, []string{"-demo", "-restarted"}) {
		t.Errorf("window: %q", got)
	}
}

func TestOpenAtStart(t *testing.T) {
	for _, c := range []struct{ noBrowser, restarted, window, want bool }{
		{false, false, false, true},
		{false, false, true, true},
		{true, false, true, false},
		{true, false, false, false},
		{true, true, true, true}, // v0.1.6's updater starting the new Mac app
		{true, true, false, false},
		{false, true, false, true},
		{false, true, true, true},
	} {
		if got := openAtStart(c.noBrowser, c.restarted, c.window); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

func TestWaitForOldCopy(t *testing.T) {
	// Only the last port answers, three times: the wait must look that far.
	var asks atomic.Int32
	last := fmt.Sprintf("127.0.0.1:%d", web.LastPort)
	start := time.Now()
	waitForOldCopy(func(addr string) bool { return addr == last && asks.Add(1) <= 3 }, 5*time.Second)
	if time.Since(start) > 2*time.Second || asks.Load() != 4 {
		t.Errorf("waited %v, %d asks of the last port", time.Since(start), asks.Load())
	}
	start = time.Now()
	waitForOldCopy(func(string) bool { return true }, 600*time.Millisecond)
	if d := time.Since(start); d < 500*time.Millisecond || d > 2*time.Second {
		t.Errorf("gave up after %v", d)
	}
}
