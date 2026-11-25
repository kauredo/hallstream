package state

import (
	"testing"

	"github.com/kauredo/hallstream/internal/sport"
)

// shortGame is a two-period sport with the half after period 1 and a 30 s
// final hold, so a machine that still used basketball's literals fails.
func shortGame(string) sport.Profile {
	p := sport.Basketball
	p.Periods, p.Half, p.FinalHold = 2, 1, 300
	return p
}

func TestMachineReadsTheProfile(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
	}{
		{"the half falls after the profile's half", []step{
			{0, "1", "10:00", 0, 0, "marcador", 0},
			{1, "1", "0.0", 10, 8, "marcador", 0},
			{1, "2", "10:00", 10, 8, "intervalo", 1},
		}},
		{"the scoreboard returns when the next period's clock starts", []step{
			{0, "1", "0.0", 10, 8, "marcador", 0},
			{1, "2", "10:00", 10, 8, "intervalo", 1},
			{1, "2", "09:59", 10, 8, "marcador", 1},
		}},
		{"final after the profile's hold in its last period", []step{
			{0, "2", "0.0", 30, 28, "marcador", 0},
			{29, "2", "0.0", 30, 28, "marcador", 0},
			{1, "2", "0.0", 30, 28, "final", 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, c := newTest()
			m.rulesFor = shortGame
			run(t, m, c, tt.steps)
		})
	}
}

func TestSnapshotCarriesRules(t *testing.T) {
	m, _ := newTest()
	if got := m.Snapshot().Game.Rules; got != sport.Basketball {
		t.Errorf("rules %+v, want basketball", got)
	}
}
