package source

import (
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/sport"
)

func TestDefaultLengthIsBasketballs(t *testing.T) {
	if DefaultLength != sport.Basketball.Length {
		t.Errorf("DefaultLength %d, basketball %d", DefaultLength, sport.Basketball.Length)
	}
}

func TestPeriodNextReadsTheProfile(t *testing.T) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	p := sport.Basketball
	p.Half, p.FoulsReset = 1, false
	mn := NewManual(&Manual{Now: c.now, Rules: func() sport.Profile { return p }})
	do(t, mn,
		Action{Do: "foul", Team: "home", N: 1},
		Action{Do: "timeout", Team: "home", N: 1},
		Action{Do: "period_next"},
	)
	s := mn.Score()
	if s.Period != 2 || s.HomeFouls != 1 || s.HomeTimeouts != 0 {
		t.Errorf("score %+v", s)
	}
}
