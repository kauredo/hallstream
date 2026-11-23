package state

import (
	"math/rand/v2"
	"strconv"
	"testing"
)

// The demo is shown to the club: Casa wins every game, in regulation or
// in overtime, and every game is played to the final buzzer.
func TestDemoClubAlwaysWins(t *testing.T) {
	for seed := range uint64(3000) {
		r := rand.New(rand.NewPCG(seed, 7))
		var last Live
		games := 0
		playDemo(r, func(l Live) { last = l }, func(int) bool { return games <= 3 }, func() {
			if games > 0 {
				if p, _ := strconv.Atoi(last.Period); last.Home <= last.Away || p < 4 || last.Clock != "0.0" {
					t.Fatalf("seed %d: game ended %d-%d in period %s at %s", seed, last.Home, last.Away, last.Period, last.Clock)
				}
			}
			games++
		})
	}
}
