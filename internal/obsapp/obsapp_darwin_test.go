package obsapp

import (
	"slices"
	"testing"
)

func TestPids(t *testing.T) {
	if _, err := pids(); err != nil {
		t.Errorf("pgrep: %v", err)
	}
	// Nobody signs in as root to run OBS, so root's OBS is never found.
	if ids, err := pidsOf(0); err != nil || ids != nil {
		t.Errorf("root's OBS: %v, %v", ids, err)
	}
}

func TestParsePids(t *testing.T) {
	if ids, err := parsePids(""); err != nil || ids != nil {
		t.Errorf("no match: %v, %v", ids, err)
	}
	if ids, err := parsePids("12\n345\n"); err != nil || !slices.Equal(ids, []int{12, 345}) {
		t.Errorf("two pids: %v, %v", ids, err)
	}
	if _, err := parsePids("12\nabc\n"); err == nil {
		t.Error("a line that is not a pid went through")
	}
}
