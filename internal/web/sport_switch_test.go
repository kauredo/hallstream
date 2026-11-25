package web

import (
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// withScorer wires a manual scorer to the server the way main does: its
// rules are the game's escalão's sport and it saves to the store.
func withScorer(s *Server) *source.Manual {
	m := source.NewManual(&source.Manual{
		Save:  func(sc source.Score) error { return s.Store.SaveManual(sc) },
		Rules: func() sport.Profile { return sport.Get(s.Store.GameSport()) },
	})
	s.Manual = m
	return m
}

// gameIn puts a game of a new escalão with the sport on the store.
func gameIn(t *testing.T, s *Server, name, sp string) string {
	t.Helper()
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": name}, &g)
	setSport(t, s, g.ID, sp)
	return g.ID
}

func playIn(t *testing.T, s *Server, id string) {
	t.Helper()
	gm := s.Store.Game()
	gm.GroupID = id
	if err := s.Store.SaveGame(gm); err != nil {
		t.Fatal(err)
	}
	s.SyncGame()
}

func TestSportSwitchMovesThePeriodLength(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	fut := gameIn(t, s, "Futsal", "futsal")
	playIn(t, s, bb)
	if err := m.Do(source.Action{Do: "period_length", Tenths: 4800}); err != nil {
		t.Fatal(err)
	}
	if sc := m.Score(); sc.Length != 4800 || sc.Clock != 4800 {
		t.Fatalf("setup: %+v", sc)
	}

	playIn(t, s, fut)
	if sc := m.Score(); sc.Length != 12000 || sc.Clock != 12000 {
		t.Errorf("to futsal: %+v, want length and clock 12000", sc)
	}
	playIn(t, s, bb)
	if sc := m.Score(); sc.Length != 6000 || sc.Clock != 6000 {
		t.Errorf("back to basketball: %+v, want 6000", sc)
	}

	// The same sport again changes nothing.
	m.Do(source.Action{Do: "period_length", Tenths: 4800})
	s.SyncGame()
	if sc := m.Score(); sc.Length != 4800 {
		t.Errorf("a sync with the same sport moved the length to %d", sc.Length)
	}
}

func TestSportSwitchLeavesARunningClock(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	fut := gameIn(t, s, "Futsal", "futsal")
	playIn(t, s, bb)
	m.Do(source.Action{Do: "clock_start"})
	playIn(t, s, fut)
	if sc := m.Score(); sc.Length != 6000 || !sc.Running {
		t.Errorf("a running clock was switched: %+v", sc)
	}
}

func TestSportSwitchDoesNotConnectTheScorer(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	s.State.SetInput("manual")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		m.Run(stop, s.State.ApplyConsole, s.State.SetBox)
		close(done)
	}()
	t.Cleanup(func() { close(stop); <-done })
	bb := gameIn(t, s, "Sub 16", "basketball")
	fut := gameIn(t, s, "Futsal", "futsal")
	playIn(t, s, bb)
	playIn(t, s, fut)
	if sc := m.Score(); sc.Length != 12000 {
		t.Fatalf("the switch did not happen: %+v", sc)
	}
	if s.State.Snapshot().Box.Connected {
		t.Error("a sport switch marked the scorer connected")
	}
	// Control: a tap from the phone does connect it, so the false above means something.
	if err := m.Do(source.Action{Do: "point", Team: "home", N: 1}); err != nil {
		t.Fatal(err)
	}
	if !s.State.Snapshot().Box.Connected {
		t.Error("a tap did not mark the scorer connected")
	}
}

// A game under way keeps its length and clock when the sport changes: the
// volunteer starts a new game.
func TestSportSwitchLeavesAGameUnderWay(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	fut := gameIn(t, s, "Futsal", "futsal")
	playIn(t, s, bb)
	for _, a := range []source.Action{{Do: "period_next"}, {Do: "period_next"}, {Do: "clock_set", Tenths: 2520}} {
		if err := m.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	playIn(t, s, fut)
	if sc := m.Score(); sc.Length != 6000 || sc.Clock != 2520 || sc.Period != 3 {
		t.Errorf("a game in the 3rd quarter was switched: %+v", sc)
	}
}

// An escalão with no sport of its own and a club with none reads as
// basketball, so the first switch away from it still counts.
func TestSportSwitchFromAnEmptySport(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	if _, err := s.Store.UpdateSettings(func(v *store.Settings) { v.Club.Sport = "" }); err != nil {
		t.Fatal(err)
	}
	fut := gameIn(t, s, "Futsal", "futsal")
	s.SyncGame()
	playIn(t, s, fut)
	if sc := m.Score(); sc.Length != 12000 || sc.Clock != 12000 {
		t.Errorf("from an empty sport to futsal: %+v, want 12000", sc)
	}
}

// Volleyball has no clock and no period length, so a hop through it leaves
// the length alone.
func TestSportSwitchThroughVolleyball(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	vb := gameIn(t, s, "Voley", "volleyball")
	playIn(t, s, bb)
	if err := m.Do(source.Action{Do: "period_length", Tenths: 4800}); err != nil {
		t.Fatal(err)
	}
	playIn(t, s, vb)
	if sc := m.Score(); sc.Length != 4800 || sc.Clock != 4800 {
		t.Errorf("the volleyball hop changed the length: %+v", sc)
	}
	playIn(t, s, bb)
	if sc := m.Score(); sc.Length != 6000 || sc.Clock != 6000 {
		t.Errorf("back to basketball: %+v, want 6000", sc)
	}
}

// A restart starts with no sport recorded, so a length the volunteer set
// survives it, even when the escalão is not basketball's.
func TestSportSwitchKeepsACustomLengthAcrossARestart(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	playIn(t, s, bb)
	if err := m.Do(source.Action{Do: "period_length", Tenths: 4800}); err != nil {
		t.Fatal(err)
	}

	var saved source.Score
	if err := s.Store.LoadManual(&saved); err != nil {
		t.Fatal(err)
	}
	s2 := New(&Server{Store: s.Store, State: state.New(time.Now), Files: s.Files, Version: "test", Port: 8765})
	m2 := withScorer(s2)
	m2.Restore(saved)
	s2.SyncGame()
	if sc := m2.Score(); sc.Length != 4800 || sc.Clock != 4800 {
		t.Errorf("after a restart: %+v, want 4800", sc)
	}
}

// A fresh basketball game switched to handball starts its clock at 0 with the
// handball half; switching back gives basketball's clock again.
func TestSportSwitchToAndFromHandball(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	hb := gameIn(t, s, "Andebol", "handball")
	playIn(t, s, bb)
	if err := m.Do(source.Action{Do: "clock_set", Tenths: 6000}); err != nil {
		t.Fatal(err)
	}
	playIn(t, s, hb)
	if sc := m.Score(); sc.Length != 18000 || sc.Clock != 0 {
		t.Errorf("to handball: %+v, want length 18000 and clock 0", sc)
	}
	playIn(t, s, bb)
	if sc := m.Score(); sc.Length != 6000 || sc.Clock != 6000 {
		t.Errorf("back to basketball: %+v, want 6000", sc)
	}
}

// A fresh score has its suspensions cleared by the switch; a game under way
// keeps its clock and its suspensions, and a running clock is never touched.
func TestSportSwitchHandballLeavesAGameUnderWay(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	hb := gameIn(t, s, "Andebol", "handball")
	playIn(t, s, hb)
	playIn(t, s, bb)
	playIn(t, s, hb)
	for _, a := range []source.Action{{Do: "period_next"}, {Do: "clock_set", Tenths: 24770}, {Do: "suspension", Team: "home"}} {
		if err := m.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	playIn(t, s, bb)
	if sc := m.Score(); sc.Clock != 24770 || sc.Period != 2 || sc.HomeSusp[0].ID != 1 {
		t.Errorf("a handball game in the 2nd half was switched: %+v", sc)
	}

	playIn(t, s, hb)
	if err := m.Do(source.Action{Do: "clock_start"}); err != nil {
		t.Fatal(err)
	}
	playIn(t, s, bb)
	if sc := m.Score(); !sc.Running || sc.Length != 18000 {
		t.Errorf("a running clock was switched: %+v", sc)
	}
}

func TestSportSwitchClearsSuspensionsOnAFreshScore(t *testing.T) {
	s := newServer(t)
	m := withScorer(s)
	bb := gameIn(t, s, "Sub 16", "basketball")
	hb := gameIn(t, s, "Andebol", "handball")
	playIn(t, s, hb)
	playIn(t, s, bb)
	playIn(t, s, hb)
	if err := m.Do(source.Action{Do: "suspension", Team: "away"}); err != nil {
		t.Fatal(err)
	}
	playIn(t, s, bb)
	if sc := m.Score(); sc.AwaySusp[0].ID != 0 {
		t.Errorf("suspensions survived a switch on a fresh score: %+v", sc)
	}
}
