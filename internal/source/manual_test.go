package source

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/sport"
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

// newVolley is newTest for a volleyball game.
func newVolley() (*Manual, *state.Machine, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	m := state.New(c.now)
	m.SetInput("manual")
	m.SetGame(state.Game{Sport: "volleyball"}, false)
	mn := NewManual(&Manual{Now: c.now, Rules: func() sport.Profile { return sport.Volleyball }})
	mn.attach(m.ApplyConsole, m.SetBox)
	return mn, m, c
}

func points(t *testing.T, mn *Manual, team string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		do(t, mn, Action{Do: "point", Team: team, N: 1})
	}
}

func TestVolleyballPointsAndServe(t *testing.T) {
	mn, m, _ := newVolley()
	do(t, mn, Action{Do: "point", Team: "home", N: 1})
	if s := mn.Score(); s.Home != 1 || s.Serve != "home" {
		t.Errorf("after +1 home: %+v", s)
	}
	do(t, mn, Action{Do: "point", Team: "away", N: 1})
	if s := mn.Score(); s.Serve != "away" {
		t.Errorf("the serve did not follow the point: %+v", s)
	}
	do(t, mn, Action{Do: "point", Team: "home", N: -1})
	if s := mn.Score(); s.Home != 0 || s.Serve != "away" {
		t.Errorf("a correction moved the serve: %+v", s)
	}
	do(t, mn, Action{Do: "serve", Team: "home"})
	if s := mn.Score(); s.Serve != "home" {
		t.Errorf("serve action: %+v", s)
	}
	if mn.Do(Action{Do: "serve", Team: "nobody"}) == nil {
		t.Error("serve for nobody")
	}
	for _, a := range []Action{
		{Do: "point", Team: "home", N: 2}, {Do: "foul", Team: "home", N: 1},
		{Do: "clock_start"}, {Do: "clock_set", Tenths: 100}, {Do: "period_next"}, {Do: "period_back"}, {Do: "period_length", Tenths: 100},
	} {
		if mn.Do(a) == nil {
			t.Errorf("%+v: want an error", a)
		}
	}
	live := m.Snapshot().Live
	if live.Serve != "home" {
		t.Errorf("serve never reached the state: %+v", live)
	}
}

func TestVolleyballSetNext(t *testing.T) {
	mn, m, _ := newVolley()
	points(t, mn, "home", 24)
	points(t, mn, "away", 23)
	if err := mn.Do(Action{Do: "set_next"}); !errors.Is(err, errSetOpen) {
		t.Fatalf("set_next at 24-23: %v", err)
	}
	do(t, mn, Action{Do: "point", Team: "home", N: 1}, Action{Do: "timeout", Team: "home", N: 1})
	do(t, mn, Action{Do: "set_next"})
	s := mn.Score()
	if s.Sets[0] != (state.SetScore{Set: 1, Home: 25, Away: 23, Winner: "home"}) || s.Sets[1].Winner != "" || s.Home != 0 || s.Away != 0 ||
		s.Serve != "" || s.HomeTimeouts != 0 || s.Period != 2 {
		t.Errorf("after set_next: %+v", s)
	}
	if st := m.Snapshot(); len(st.Sets) != 1 || st.Sets[0].Home != 25 || st.Live.HomeSets != 1 {
		t.Errorf("state after set_next: sets %v live %+v", st.Sets, st.Live)
	}
	// Undo brings the set back.
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.Home != 25 || s.Away != 23 || s.Sets[0].Winner != "" {
		t.Errorf("after undo: %+v", s)
	}
	if st := m.Snapshot(); len(st.Sets) != 0 {
		t.Errorf("state kept the set after undo: %v", st.Sets)
	}
}

func TestVolleyballMatchEnds(t *testing.T) {
	mn, m, _ := newVolley()
	for i := 0; i < 3; i++ {
		points(t, mn, "home", 25)
		do(t, mn, Action{Do: "set_next"})
	}
	if err := mn.Do(Action{Do: "set_next"}); !errors.Is(err, errGameOver) {
		t.Errorf("set_next after the match: %v", err)
	}
	if m.Snapshot().Screen.Current != "final" {
		t.Errorf("screen %q, want final", m.Snapshot().Screen.Current)
	}
	do(t, mn, Action{Do: "undo"})
	if m.Snapshot().Screen.Current != "marcador" {
		t.Errorf("screen %q after the undo, want marcador", m.Snapshot().Screen.Current)
	}
}

func TestVolleyballDeciderTarget(t *testing.T) {
	mn, _, _ := newVolley()
	for i := 0; i < 4; i++ { // two sets each
		w := "home"
		if i%2 == 1 {
			w = "away"
		}
		points(t, mn, w, 25)
		do(t, mn, Action{Do: "set_next"})
	}
	points(t, mn, "home", 15)
	points(t, mn, "away", 14)
	if !errors.Is(mn.Do(Action{Do: "set_next"}), errSetOpen) {
		t.Error("15-14 ended the deciding set")
	}
	points(t, mn, "home", 1)
	do(t, mn, Action{Do: "set_next"})
}

func TestVolleyballEmit(t *testing.T) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	mn := NewManual(&Manual{Now: c.now, Rules: func() sport.Profile { return sport.Volleyball }})
	var got map[string]string
	mn.attach(func(v map[string]string) { got = v }, func(bool) {})
	points(t, mn, "home", 25)
	points(t, mn, "away", 23)
	do(t, mn, Action{Do: "set_next"})
	if got["sets"] != "25-23h" || got["home_sets"] != "1" || got["away_sets"] != "0" || got["clock"] != "" || got["period"] != "2" {
		t.Errorf("emit %v", got)
	}
}

// A set the console ended has no points, and an away win must stay one.
func TestVolleyballSeededAwayWinStaysAway(t *testing.T) {
	mn, m, _ := newVolley()
	mn.Seed(state.Live{HomeSets: 1, AwaySets: 1, Home: 3, Away: 2})
	got := m.Snapshot().Sets
	if len(got) != 2 || got[0].Winner != "home" || got[1].Winner != "away" {
		t.Errorf("recorded sets %v, want home then away", got)
	}
	if s := m.Snapshot().Live; s.HomeSets != 1 || s.AwaySets != 1 {
		t.Errorf("live %+v", s)
	}
}

func TestVolleyballSeedAndRestore(t *testing.T) {
	mn, _, _ := newVolley()
	mn.Seed(state.Live{HomeSets: 1, AwaySets: 1, Home: 12, Away: 9, Serve: "away"})
	s := mn.Score()
	if s.Period != 3 || s.Home != 12 || s.Serve != "away" || s.Sets[0].Winner != "home" || s.Sets[1].Winner != "away" || s.Sets[2].Winner != "" {
		t.Errorf("seeded %+v", s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Score
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	again, _, _ := newVolley()
	again.Restore(back)
	if got := again.Score(); got != s {
		t.Errorf("restored %+v, want %+v", got, s)
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

// The scorer nudges the clock to the hall's: a second or ten at a time,
// running or not, never below 0, and undo takes the nudge back.
func TestClockAdd(t *testing.T) {
	mn, _, c := newTest()
	do(t, mn, Action{Do: "clock_add", Tenths: -10})
	if s := mn.Score(); s.Clock != DefaultLength-10 || s.Running {
		t.Errorf("-1 s stopped: %+v", s)
	}
	do(t, mn, Action{Do: "clock_start"})
	c.add(2 * time.Second)
	do(t, mn, Action{Do: "clock_add", Tenths: 100})
	if s := mn.Score(); s.Clock != DefaultLength-10-20+100 || !s.Running {
		t.Errorf("+10 s running: %+v", s)
	}
	do(t, mn, Action{Do: "clock_stop"}, Action{Do: "clock_add", Tenths: -MaxClock})
	if s := mn.Score(); s.Clock != 0 {
		t.Errorf("below 0: %+v", s)
	}
	if err := mn.Do(Action{Do: "clock_add"}); err == nil {
		t.Error("0 s accepted")
	}
	if a, ok := mn.Undo(); !ok || a.Do != "clock_add" || a.Tenths != -MaxClock {
		t.Errorf("next undo %+v %v", a, ok)
	}
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.Clock != DefaultLength-30+100 {
		t.Errorf("after undo %+v", s)
	}
}

// FIBA timeouts: the count goes back to 0 for the second half and each overtime only.
func TestTimeoutsPerHalf(t *testing.T) {
	mn, _, _ := newTest()
	want := map[int]int{2: 1, 3: 0, 4: 1, 5: 0, 6: 0}
	for p := 2; p <= 6; p++ {
		do(t, mn, Action{Do: "timeout", Team: "home", N: 1}, Action{Do: "period_next"})
		if got := mn.Score(); got.Period != p || got.HomeTimeouts != want[p] {
			t.Errorf("period %d: %d timeouts, want %d", got.Period, got.HomeTimeouts, want[p])
		}
	}
	if _, ok := NewManual(&Manual{}).Undo(); ok {
		t.Error("a new game has something to undo")
	}
}
