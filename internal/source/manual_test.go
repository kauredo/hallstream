package source

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/state"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }

// newTest is a manual scorer and a state machine on one fake clock, the
// scorer's values going straight into the state as if it ran.
func newTest() (*Manual, *state.Machine, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	m := state.New(c.now)
	m.SetInput("manual")
	mn := NewManual(&Manual{Now: c.now})
	mn.attach(m.ApplyConsole, m.SetBox)
	return mn, m, c
}

func do(t *testing.T, mn *Manual, acts ...Action) {
	t.Helper()
	for _, a := range acts {
		if err := mn.Do(a); err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
	}
}

func TestPointsAndUndo(t *testing.T) {
	tests := []struct {
		name       string
		acts       []Action
		home, away int
		hf, at     int // home fouls, away timeouts
	}{
		{"a basket each", []Action{{Do: "point", Team: "home", N: 2}, {Do: "point", Team: "away", N: 3}}, 2, 3, 0, 0},
		{"never below 0", []Action{{Do: "point", Team: "home", N: 1}, {Do: "point", Team: "home", N: -3}}, 0, 0, 0, 0},
		{"undo the last", []Action{{Do: "point", Team: "home", N: 2}, {Do: "point", Team: "home", N: 3}, {Do: "undo"}}, 2, 0, 0, 0},
		{"undo twice", []Action{{Do: "point", Team: "home", N: 2}, {Do: "point", Team: "away", N: 1}, {Do: "undo"}, {Do: "undo"}}, 0, 0, 0, 0},
		{"a no-op is not undone", []Action{{Do: "point", Team: "home", N: 2}, {Do: "point", Team: "away", N: -1}, {Do: "undo"}}, 0, 0, 0, 0},
		{"fouls and timeouts", []Action{{Do: "foul", Team: "home", N: 1}, {Do: "foul", Team: "home", N: 1}, {Do: "foul", Team: "home", N: -1}, {Do: "timeout", Team: "away", N: 1}}, 0, 0, 1, 1},
		{"new game, undone", []Action{{Do: "point", Team: "home", N: 3}, {Do: "new_game"}, {Do: "undo"}}, 3, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mn, m, _ := newTest()
			do(t, mn, tt.acts...)
			s := mn.Score()
			if s.Home != tt.home || s.Away != tt.away || s.HomeFouls != tt.hf || s.AwayTimeouts != tt.at {
				t.Errorf("score %+v", s)
			}
			l := m.Snapshot().Live
			if l.Home != tt.home || l.Away != tt.away || l.HomeFouls != tt.hf || l.AwayTimeouts != tt.at {
				t.Errorf("live %+v", l)
			}
		})
	}
}

func TestBadActions(t *testing.T) {
	mn, _, _ := newTest()
	for _, a := range []Action{
		{Do: "point", Team: "home", N: 4},
		{Do: "point", Team: "home"},
		{Do: "point", Team: "casa", N: 1},
		{Do: "foul", Team: "away", N: 2},
		{Do: "clock_set", Tenths: -1},
		{Do: "clock_set", Tenths: MaxClock + 1},
		{Do: "period_length"},
		{Do: "undo"},
		{Do: "dunk"},
	} {
		if mn.Do(a) == nil {
			t.Errorf("%+v: want an error", a)
		}
	}
	if s := mn.Score(); s != newScore(DefaultLength) {
		t.Errorf("bad actions changed the score: %+v", s)
	}
}

func TestUndoKeepsMax(t *testing.T) {
	mn, _, _ := newTest()
	for range undoMax + 10 {
		do(t, mn, Action{Do: "point", Team: "home", N: 1})
	}
	for range undoMax {
		do(t, mn, Action{Do: "undo"})
	}
	if err := mn.Do(Action{Do: "undo"}); !errors.Is(err, errUndo) {
		t.Errorf("undo past %d: %v", undoMax, err)
	}
	if s := mn.Score(); s.Home != 10 {
		t.Errorf("home %d, want 10", s.Home)
	}
}

func TestClockRunsAndStops(t *testing.T) {
	mn, m, c := newTest()
	if got := m.Snapshot().Live.Clock; got != "10:00" {
		t.Fatalf("clock %q before the start", got)
	}
	do(t, mn, Action{Do: "clock_start"})
	c.add(1500 * time.Millisecond)
	mn.Tick()
	if got := m.Snapshot().Live.Clock; got != "09:59" {
		t.Errorf("clock %q after 1.5 s", got)
	}
	if !m.ClockRanWithin(time.Second) {
		t.Error("the state does not see the clock run")
	}
	// A basket while the clock runs leaves it running, and undo leaves it too.
	do(t, mn, Action{Do: "point", Team: "away", N: 2})
	c.add(time.Second)
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); !s.Running || s.Clock != 5975 || s.Away != 0 {
		t.Errorf("after the undo %+v", s)
	}
	do(t, mn, Action{Do: "clock_stop"})
	c.add(time.Minute)
	mn.Tick()
	if s := mn.Score(); s.Running || s.Clock != 5975 {
		t.Errorf("stopped clock moved: %+v", s)
	}
	do(t, mn, Action{Do: "clock_set", Tenths: 425}, Action{Do: "clock_start"})
	c.add(300 * time.Millisecond)
	mn.Tick()
	if got := m.Snapshot().Live.Clock; got != "42.2" {
		t.Errorf("clock %q under a minute", got)
	}
	c.add(time.Minute)
	mn.Tick()
	if s := mn.Score(); s.Running || s.Clock != 0 || m.Snapshot().Live.Clock != "0.0" {
		t.Errorf("clock past 0: %+v, live %q", s, m.Snapshot().Live.Clock)
	}
	if err := mn.Do(Action{Do: "clock_start"}); err != nil || mn.Score().Running {
		t.Errorf("a clock at 0 started: %v", err)
	}
}

func TestPeriods(t *testing.T) {
	mn, m, _ := newTest()
	do(t, mn, Action{Do: "period_length", Tenths: 4800})
	if s := mn.Score(); s.Clock != 4800 || s.Length != 4800 {
		t.Fatalf("a new length leaves the clock at %d", s.Clock)
	}
	do(t, mn,
		Action{Do: "point", Team: "home", N: 2},
		Action{Do: "foul", Team: "away", N: 1},
		Action{Do: "timeout", Team: "away", N: 1},
		Action{Do: "clock_set", Tenths: 0},
		Action{Do: "period_next"},
	)
	s := mn.Score()
	if s.Period != 2 || s.Clock != 4800 || s.AwayFouls != 0 || s.AwayTimeouts != 1 || s.Running {
		t.Errorf("period 2 starts with %+v", s)
	}
	if ps := m.Snapshot().Periods; len(ps) != 1 || ps[0] != (state.Period{Period: 1, Home: 2, HomePoints: 2}) {
		t.Errorf("periods %v", ps)
	}
	do(t, mn, Action{Do: "clock_set", Tenths: 3000}, Action{Do: "period_back"}, Action{Do: "period_back"})
	if s := mn.Score(); s.Period != 1 || s.Clock != 3000 {
		t.Errorf("period back: %+v, want period 1 with the clock where it was", s)
	}
	do(t, mn, Action{Do: "undo"}) // the second back changed nothing, so this is the first
	if s := mn.Score(); s.Period != 2 || s.Clock != 3000 {
		t.Errorf("undo period back %+v", s)
	}
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.Period != 2 || s.Clock != 4800 {
		t.Errorf("undo clock set %+v", s)
	}
}

func TestHalfTime(t *testing.T) {
	mn, m, c := newTest()
	do(t, mn, Action{Do: "period_next"}, Action{Do: "clock_set", Tenths: 0}, Action{Do: "period_next"})
	if got := m.Snapshot().Screen.Current; got != "intervalo" {
		t.Errorf("screen %q after period 2, want intervalo", got)
	}
	do(t, mn, Action{Do: "clock_start"})
	c.add(time.Second)
	mn.Tick()
	if got := m.Snapshot().Screen.Current; got != "marcador" {
		t.Errorf("screen %q once period 3 runs, want marcador", got)
	}
}

func TestFinal(t *testing.T) {
	mn, m, c := newTest()
	for range 3 {
		do(t, mn, Action{Do: "period_next"})
	}
	do(t, mn, Action{Do: "point", Team: "home", N: 3}, Action{Do: "clock_set", Tenths: 50}, Action{Do: "clock_start"})
	c.add(5 * time.Second)
	mn.Tick()
	m.Tick()
	if got := m.Snapshot().Screen.Current; got != "marcador" {
		t.Fatalf("screen %q at 0", got)
	}
	c.add(state.FinalHold)
	m.Tick()
	s := m.Snapshot()
	if s.Screen.Current != "final" || len(s.Periods) != 4 || s.Periods[3].Home != 3 {
		t.Errorf("after the hold: screen %q, periods %v", s.Screen.Current, s.Periods)
	}
}

func TestHeartbeat(t *testing.T) {
	mn, m, c := newTest()
	if m.Snapshot().Box.Connected {
		t.Fatal("connected before the phone called in")
	}
	mn.Heartbeat()
	if !m.Snapshot().Box.Connected {
		t.Fatal("the heartbeat does not connect")
	}
	c.add(Seen - time.Second)
	mn.refresh()
	do(t, mn, Action{Do: "point", Team: "home", N: 1}) // a tap counts too
	c.add(Seen - time.Second)
	mn.refresh()
	if !m.Snapshot().Box.Connected {
		t.Error("a tap does not keep it connected")
	}
	c.add(time.Second)
	mn.refresh()
	if m.Snapshot().Box.Connected {
		t.Error("still connected 60 s after the phone went quiet")
	}
}

func TestSavedScoreComesBackStopped(t *testing.T) {
	var saved []byte
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	mn := NewManual(&Manual{Now: c.now, Save: func(s Score) error {
		b, err := json.Marshal(s)
		saved = b
		return err
	}})
	mn.attach(func(map[string]string) {}, func(bool) {})
	do(t, mn, Action{Do: "period_next"}, Action{Do: "point", Team: "away", N: 2}, Action{Do: "clock_start"})
	c.add(2 * time.Second)
	mn.Tick()

	var back Score
	if err := json.Unmarshal(saved, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Running || back.Clock != 5980 {
		t.Fatalf("saved %+v, want the running clock at 09:58", back)
	}
	again := NewManual(&Manual{Now: c.now})
	again.Restore(back)
	c.add(time.Hour) // the program was off
	want := Score{Away: 2, Period: 2, Clock: 5980, Length: DefaultLength}
	if got := again.Score(); got != want {
		t.Errorf("restored %+v, want %+v", got, want)
	}
	if again.Do(Action{Do: "undo"}) == nil {
		t.Error("undo crosses a restart")
	}
}

func TestSeed(t *testing.T) {
	mn, _, _ := newTest()
	do(t, mn, Action{Do: "point", Team: "home", N: 3})
	mn.Seed(state.Live{})
	if s := mn.Score(); s.Home != 3 {
		t.Errorf("a console with no period replaced the game: %+v", s)
	}
	mn.Seed(state.Live{Home: 41, Away: 38, Clock: "07:45", Period: "2", HomeFouls: 2, AwayFouls: 4})
	want := Score{Home: 41, Away: 38, Period: 2, Clock: 4650, Length: DefaultLength, HomeFouls: 2, AwayFouls: 4}
	if s := mn.Score(); s != want {
		t.Errorf("seeded %+v, want %+v", s, want)
	}
	if mn.Do(Action{Do: "undo"}) == nil {
		t.Error("undo goes back past the seed")
	}
}
