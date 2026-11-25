package state

import (
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/sport"
)

func TestFinalHoldIsBasketballs(t *testing.T) {
	if want := time.Duration(sport.Basketball.FinalHold) * 100 * time.Millisecond; FinalHold != want {
		t.Errorf("FinalHold %v, basketball %v", FinalHold, want)
	}
}
