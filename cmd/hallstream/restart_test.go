package main

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestartArgs(t *testing.T) {
	for _, c := range []struct{ in, want []string }{
		{nil, []string{"-nobrowser", "-restarted"}},
		{[]string{"-demo"}, []string{"-demo", "-nobrowser", "-restarted"}},
		{[]string{"-demo", "--nobrowser", "-restarted=true", "-replay", "x.capture"}, []string{"-demo", "-replay", "x.capture", "-nobrowser", "-restarted"}},
	} {
		if got := restartArgs(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q", c.in, got)
		}
	}
}

func TestWaitForOldCopy(t *testing.T) {
	var asks atomic.Int32
	start := time.Now()
	waitForOldCopy(func(string) bool { return asks.Add(1) <= 3 }, 5*time.Second)
	if time.Since(start) > 2*time.Second || asks.Load() < 4 {
		t.Errorf("waited %v, %d asks", time.Since(start), asks.Load())
	}
	start = time.Now()
	waitForOldCopy(func(string) bool { return true }, 600*time.Millisecond)
	if d := time.Since(start); d < 500*time.Millisecond || d > 2*time.Second {
		t.Errorf("gave up after %v", d)
	}
}
