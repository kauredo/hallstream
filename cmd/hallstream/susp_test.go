package main

import (
	"testing"

	"github.com/kauredo/hallstream/internal/source"
)

func TestHighestSuspension(t *testing.T) {
	var sc source.Score
	if got := highestSuspension(sc); got != 0 {
		t.Errorf("no suspensions: %d", got)
	}
	sc.HomeSusp[1] = source.Suspension{ID: 2}
	sc.AwaySusp[0] = source.Suspension{ID: 5}
	sc.NextSusp = 6
	if got := highestSuspension(sc); got != 5 {
		t.Errorf("highest %d, want 5", got)
	}
}
