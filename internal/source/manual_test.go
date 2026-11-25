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
	if s := mn.Score(); !sameScore(s, newScoreAt(DefaultLength, DefaultLength)) {
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
	if got := again.Score(); !sameScore(got, want) {
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
	if got := again.Score(); !sameScore(got, s) {
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
	if s := mn.Score(); !sameScore(s, want) {
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

// newSport is newTest for a sport's profile.
func newSport(p sport.Profile) (*Manual, *state.Machine, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	m := state.New(c.now)
	m.SetInput("manual")
	mn := NewManual(&Manual{Now: c.now, Rules: func() sport.Profile { return p }})
	mn.attach(m.ApplyConsole, m.SetBox)
	return mn, m, c
}

func foulsTo(t *testing.T, mn *Manual, home, away int) {
	t.Helper()
	for i := 0; i < home; i++ {
		do(t, mn, Action{Do: "foul", Team: "home", N: 1})
	}
	for i := 0; i < away; i++ {
		do(t, mn, Action{Do: "foul", Team: "away", N: 1})
	}
}

func TestFutsalPeriods(t *testing.T) {
	mn, _, _ := newSport(sport.Futsal)
	do(t, mn, Action{Do: "period_length", Tenths: sport.Futsal.Length})
	foulsTo(t, mn, 4, 3)
	do(t, mn, Action{Do: "timeout", Team: "home", N: 1}, Action{Do: "period_next"})
	if s := mn.Score(); s.Period != 2 || s.HomeFouls != 0 || s.AwayFouls != 0 || s.HomeTimeouts != 0 || s.Clock != 12000 {
		t.Errorf("half 2 starts with %+v", s)
	}
	foulsTo(t, mn, 5, 2)
	do(t, mn, Action{Do: "timeout", Team: "away", N: 1}, Action{Do: "period_next"})
	s := mn.Score()
	if s.Period != 3 || s.HomeFouls != 5 || s.AwayFouls != 2 {
		t.Errorf("extra time loses the fouls: %+v", s)
	}
	if s.Clock != 3000 {
		t.Errorf("extra time starts at %d, want 3000", s.Clock)
	}
	if s.AwayTimeouts != 0 {
		t.Errorf("extra time keeps the timeouts: %+v", s)
	}
	do(t, mn, Action{Do: "foul", Team: "home", N: 1}, Action{Do: "period_next"})
	if s := mn.Score(); s.Period != 4 || s.HomeFouls != 6 || s.Clock != 3000 {
		t.Errorf("the second extra period: %+v", s)
	}
}

func TestBasketballOvertimeIsFiveMinutes(t *testing.T) {
	mn, _, _ := newSport(sport.Basketball)
	for i := 0; i < 3; i++ {
		do(t, mn, Action{Do: "period_next"})
	}
	if s := mn.Score(); s.Period != 4 || s.Clock != DefaultLength {
		t.Fatalf("the 4th quarter starts at %d", s.Clock)
	}
	foulsTo(t, mn, 3, 0)
	do(t, mn, Action{Do: "period_next"})
	if s := mn.Score(); s.Period != 5 || s.Clock != 3000 || s.HomeFouls != 0 {
		t.Errorf("overtime starts with %+v", s)
	}
}

func TestSetLength(t *testing.T) {
	mn, m, c := newSport(sport.Futsal)
	if err := mn.SetLength(12000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 12000 || s.Clock != 12000 {
		t.Errorf("a stopped clock at the old length: %+v", s)
	}
	if m.Snapshot().Box.Connected {
		t.Error("SetLength counted as the phone calling in")
	}
	if _, ok := mn.Undo(); ok {
		t.Error("SetLength went on the undo list")
	}
	if got := m.Snapshot().Live.Clock; got != "20:00" {
		t.Errorf("the live clock shows %q", got)
	}

	do(t, mn, Action{Do: "clock_set", Tenths: 9000})
	if err := mn.SetLength(6000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 6000 || s.Clock != 9000 {
		t.Errorf("a clock elsewhere moved: %+v", s)
	}
	if a, ok := mn.Undo(); !ok || a.Do != "clock_set" {
		t.Errorf("SetLength went on the undo list: %+v", a)
	}

	do(t, mn, Action{Do: "clock_set", Tenths: 6000}, Action{Do: "clock_start"})
	c.add(10 * time.Second)
	if err := mn.SetLength(4800); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); !s.Running || s.Clock != 5900 || s.Length != 6000 {
		t.Errorf("a running clock was touched: %+v", s)
	}
	do(t, mn, Action{Do: "clock_stop"}, Action{Do: "point", Team: "home", N: 1})
	if err := mn.SetLength(4800); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 6000 || s.Clock != 5900 {
		t.Errorf("a game under way changed length: %+v", s)
	}
	do(t, mn, Action{Do: "new_game"})
	if err := mn.SetLength(4800); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 4800 || s.Clock != 4800 {
		t.Errorf("a new game kept the old length: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"})
	if err := mn.SetLength(3000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 4800 || s.Period != 2 {
		t.Errorf("period 2 changed length: %+v", s)
	}

	for _, n := range []int{0, -1, MaxClock + 1} {
		if mn.SetLength(n) == nil {
			t.Errorf("SetLength(%d): want an error", n)
		}
	}
}

// The scorer takes a timeout tap in futsal extra time, where the rules give
// none: only the page, which draws no marks for it, shows the limit.
func TestFutsalExtraTimeTimeoutIsNotRefused(t *testing.T) {
	mn, _, _ := newSport(sport.Futsal)
	do(t, mn, Action{Do: "period_next"}, Action{Do: "period_next"})
	if got := sport.Futsal.Timeouts(mn.Score().Period); got != 0 {
		t.Fatalf("extra time has %d timeouts, want 0", got)
	}
	do(t, mn, Action{Do: "timeout", Team: "home", N: 1})
	if s := mn.Score(); s.Period != 3 || s.HomeTimeouts != 1 {
		t.Errorf("the tap was not counted: %+v", s)
	}
}

// How much one score action can add follows the profile: basketball takes up to
// 3, a goal in futsal is 1, and volleyball's +1 only stays as it was.
func TestMaxPointByProfile(t *testing.T) {
	for _, c := range []struct {
		p      sport.Profile
		n      int
		accept bool
	}{
		{sport.Basketball, 3, true}, {sport.Basketball, -3, true}, {sport.Basketball, 4, false},
		{sport.Futsal, 1, true}, {sport.Futsal, -1, true}, {sport.Futsal, 2, false}, {sport.Futsal, 3, false}, {sport.Futsal, 0, false},
		{sport.Volleyball, 1, true}, {sport.Volleyball, 2, false}, {sport.Volleyball, -2, false},
		{sport.Handball, 1, true}, {sport.Handball, 2, false}, {sport.Handball, 3, false},
	} {
		mn, _, _ := newSport(c.p)
		if c.n < 0 {
			do(t, mn, Action{Do: "point", Team: "home", N: 1}, Action{Do: "point", Team: "home", N: 1}, Action{Do: "point", Team: "home", N: 1})
			if c.p.Sets() {
				continue
			}
		}
		err := mn.Do(Action{Do: "point", Team: "home", N: c.n})
		if (err == nil) != c.accept {
			t.Errorf("%s point %d: err %v, want accepted %v", c.p.ID, c.n, err, c.accept)
		}
	}
	mn, _, _ := newSport(sport.Futsal)
	foulsTo(t, mn, 2, 0) // futsal keeps its fouls
	if s := mn.Score(); s.HomeFouls != 2 {
		t.Errorf("futsal fouls %d, want 2", s.HomeFouls)
	}
	before := mn.Score()
	if mn.Do(Action{Do: "point", Team: "home", N: 2}) == nil || !sameScore(mn.Score(), before) {
		t.Error("a refused +2 changed the score")
	}
}

// newHandball is a scorer for handball in a new game: 2 x 30:00, the clock at 0.
func newHandball() (*Manual, *state.Machine, *fakeClock) {
	mn, m, c := newSport(sport.Handball)
	if err := mn.Do(Action{Do: "period_length", Tenths: 18000}); err != nil {
		panic(err)
	}
	if err := mn.Do(Action{Do: "new_game"}); err != nil {
		panic(err)
	}
	return mn, m, c
}

func TestCountUpClock(t *testing.T) {
	mn, m, c := newHandball()
	if s := mn.Score(); s.Clock != 0 || s.Length != 18000 || s.Period != 1 {
		t.Fatalf("a new game: %+v", s)
	}
	do(t, mn, Action{Do: "clock_start"})
	c.add(65 * time.Second)
	if s := mn.Score(); s.Clock != 650 || !s.Running {
		t.Errorf("65 s after the start: %+v", s)
	}
	mn.Tick()
	if got := m.Snapshot().Live.Clock; got != "01:05" {
		t.Errorf("live clock %q, want 01:05", got)
	}
	if got := m.Snapshot().Live.PeriodEnd; got != 18000 {
		t.Errorf("live period_end %d, want 18000", got)
	}
	// It stops at the end of the period.
	c.add(40 * time.Minute)
	if s := mn.Score(); s.Clock != 18000 || s.Running {
		t.Errorf("after the half: %+v, want stopped at 18000", s)
	}
	do(t, mn, Action{Do: "clock_start"})
	if s := mn.Score(); s.Running {
		t.Errorf("the clock started at the end of the half: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"})
	if s := mn.Score(); s.Period != 2 || s.Clock != 18000 || s.Running {
		t.Errorf("second half: %+v", s)
	}
	do(t, mn, Action{Do: "clock_start"})
	if s := mn.Score(); !s.Running {
		t.Errorf("the second half did not start: %+v", s)
	}
	c.add(40 * time.Minute)
	if s := mn.Score(); s.Clock != 36000 || s.Running {
		t.Errorf("end of the game: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"})
	if s := mn.Score(); s.Period != 3 || s.Clock != 36000 {
		t.Errorf("extra time starts at the time played: %+v", s)
	}
}

func TestCountUpClockEdits(t *testing.T) {
	mn, _, c := newHandball()
	do(t, mn, Action{Do: "clock_set", Tenths: 50}, Action{Do: "clock_start"})
	c.add(0)
	do(t, mn, Action{Do: "clock_add", Tenths: 100})
	if s := mn.Score(); s.Clock != 150 || !s.Running {
		t.Errorf("clock_add on a running clock: %+v, want 150 and running", s)
	}
	do(t, mn, Action{Do: "clock_set", Tenths: 50000})
	if s := mn.Score(); s.Clock != 18000 || s.Running {
		t.Errorf("clock_set past the end: %+v, want 18000 and stopped", s)
	}
	do(t, mn, Action{Do: "clock_add", Tenths: -50000})
	if s := mn.Score(); s.Clock != 0 {
		t.Errorf("clock_add below the start: %+v, want 0", s)
	}
	do(t, mn, Action{Do: "period_next"}, Action{Do: "clock_set", Tenths: 10})
	if s := mn.Score(); s.Clock != 18000 {
		t.Errorf("second half below its start: %+v, want 18000", s)
	}
	do(t, mn, Action{Do: "period_back"})
	if s := mn.Score(); s.Period != 1 || s.Clock != 18000 || s.Running {
		t.Errorf("period_back: %+v, want 18000 stopped", s)
	}
}

func TestCountUpPeriodLength(t *testing.T) {
	mn, _, _ := newHandball()
	do(t, mn, Action{Do: "period_length", Tenths: 15000})
	if s := mn.Score(); s.Clock != 0 || s.Length != 15000 {
		t.Errorf("period 1 at 0: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"})
	if s := mn.Score(); s.Clock != 15000 {
		t.Errorf("second half with 25 minutes: %+v, want 15000", s)
	}
	do(t, mn, Action{Do: "new_game"})
	if s := mn.Score(); s.Clock != 0 || s.Length != 15000 {
		t.Errorf("new_game: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"}, Action{Do: "period_length", Tenths: 18000})
	if s := mn.Score(); s.Clock != 18000 || s.Length != 18000 {
		t.Errorf("a stopped clock at the start of period 2 follows the length: %+v", s)
	}
	do(t, mn, Action{Do: "clock_set", Tenths: 20000}, Action{Do: "period_length", Tenths: 12000})
	if s := mn.Score(); s.Clock != 20000 {
		t.Errorf("a clock elsewhere moved: %+v", s)
	}
}

func TestCountUpRunningClockKeepsItsLength(t *testing.T) {
	mn, _, c := newHandball()
	do(t, mn, Action{Do: "clock_start"})
	c.add(10 * time.Second)
	do(t, mn, Action{Do: "period_length", Tenths: 12000})
	if s := mn.Score(); s.Clock != 100 || !s.Running {
		t.Errorf("a running clock moved: %+v", s)
	}
}

func TestSuspensions(t *testing.T) {
	mn, m, c := newHandball()
	do(t, mn, Action{Do: "clock_set", Tenths: 5000}, Action{Do: "suspension", Team: "home"})
	if s := mn.Score(); s.HomeSusp[0] != (Suspension{ID: 1, End: 6200}) {
		t.Fatalf("a tap at 5000: %+v", s.HomeSusp)
	}
	for i := 0; i < 2; i++ {
		do(t, mn, Action{Do: "suspension", Team: "home"})
	}
	before := mn.Score()
	if err := mn.Do(Action{Do: "suspension", Team: "home"}); err == nil {
		t.Error("a fourth suspension was taken")
	}
	if !sameScore(mn.Score(), before) {
		t.Error("a refused suspension changed the score")
	}
	if err := mn.Do(Action{Do: "suspension", Team: "away"}); err != nil {
		t.Errorf("the other team is full too: %v", err)
	}

	// Time left follows the game clock, not the wall.
	do(t, mn, Action{Do: "clock_start"})
	c.add(30 * time.Second)
	if s := mn.Score(); s.HomeSusp[0].End-s.Clock != 900 {
		t.Errorf("left after 30 s: %+v", s)
	}
	do(t, mn, Action{Do: "clock_stop"})
	c.add(10 * time.Minute)
	if s := mn.Score(); s.HomeSusp[0].End-s.Clock != 900 {
		t.Errorf("left moved while the clock was stopped: %+v", s)
	}
	mn.Tick()
	var live []state.Suspension
	for _, x := range m.Snapshot().Live.Suspensions {
		if x.Team == "home" {
			live = append(live, x)
		}
	}
	if len(live) != 3 || live[0].Left != "01:30" || live[0].ID != 1 {
		t.Errorf("live suspensions %+v", live)
	}

	do(t, mn, Action{Do: "suspension_number", ID: 1, Number: "7"})
	if s := mn.Score(); s.HomeSusp[0].Number != "7" {
		t.Errorf("number not set: %+v", s.HomeSusp)
	}
	for _, n := range []string{"1234", "7a", "-1"} {
		if mn.Do(Action{Do: "suspension_number", ID: 1, Number: n}) == nil {
			t.Errorf("number %q accepted", n)
		}
	}
	if mn.Do(Action{Do: "suspension_number", ID: 99, Number: "1"}) == nil {
		t.Error("an unknown ID was accepted")
	}
	do(t, mn, Action{Do: "suspension_number", ID: 1, Number: ""})
	if s := mn.Score(); s.HomeSusp[0].Number != "" {
		t.Errorf("number not cleared: %+v", s.HomeSusp)
	}
	do(t, mn, Action{Do: "suspension_end", ID: 2})
	if s := mn.Score(); s.HomeSusp[1].ID != 0 || s.HomeSusp[0].ID != 1 || s.HomeSusp[2].ID != 3 {
		t.Errorf("suspension_end: %+v", s.HomeSusp)
	}
	do(t, mn, Action{Do: "suspension", Team: "home"})
	if s := mn.Score(); s.HomeSusp[1].ID != 5 {
		t.Errorf("a freed slot is reused with a new ID: %+v", s.HomeSusp)
	}
}

func TestSuspensionsEndOnTheGameClock(t *testing.T) {
	mn, _, c := newHandball()
	do(t, mn, Action{Do: "clock_set", Tenths: 17000}, Action{Do: "suspension", Team: "away"})
	do(t, mn, Action{Do: "clock_start"})
	c.add(100 * time.Second)
	if s := mn.Score(); s.AwaySusp[0].ID != 1 || s.Clock != 18000 || s.Running {
		t.Fatalf("at the half: %+v", s)
	}
	do(t, mn, Action{Do: "period_next"}, Action{Do: "clock_start"})
	c.add(19 * time.Second)
	if s := mn.Score(); s.AwaySusp[0].ID != 1 {
		t.Errorf("the suspension did not carry into the second half: %+v", s)
	}
	c.add(2 * time.Second)
	if s := mn.Score(); s.AwaySusp[0].ID != 0 {
		t.Errorf("the suspension did not end at 31:00: %+v", s)
	}
}

func TestSuspensionUndo(t *testing.T) {
	mn, _, c := newHandball()
	do(t, mn, Action{Do: "clock_start"}, Action{Do: "suspension", Team: "home"})
	c.add(10 * time.Second)
	do(t, mn, Action{Do: "point", Team: "away", N: 1})
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.HomeSusp[0].End-s.Clock != 1100 || s.Away != 0 {
		t.Errorf("undo of a point changed the suspension: %+v", s)
	}
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.HomeSusp[0].ID != 0 {
		t.Errorf("undo of the suspension left it: %+v", s)
	}
	do(t, mn, Action{Do: "suspension", Team: "home"}, Action{Do: "suspension_end", ID: 1})
	do(t, mn, Action{Do: "undo"})
	if s := mn.Score(); s.HomeSusp[0].ID != 1 {
		t.Errorf("undo of an early end did not bring the chip back: %+v", s)
	}
}

func TestSuspensionOnlyChangeIsKept(t *testing.T) {
	mn, m, _ := newHandball()
	var saved []Score
	mn.Save = func(s Score) error { saved = append(saved, s); return nil }
	do(t, mn, Action{Do: "suspension", Team: "home"})
	if len(saved) != 1 || saved[0].HomeSusp[0].ID != 1 {
		t.Errorf("saved %d scores: %+v", len(saved), saved)
	}
	if a, ok := mn.Undo(); !ok || a.Do != "suspension" {
		t.Errorf("not on the undo list: %+v", a)
	}
	if got := m.Snapshot().Live.Suspensions; len(got) != 1 || got[0].Left != "02:00" {
		t.Errorf("live %+v", got)
	}
}

func TestNoSuspensionsOutsideHandball(t *testing.T) {
	for _, p := range []sport.Profile{sport.Basketball, sport.Futsal, sport.Volleyball} {
		mn, _, _ := newSport(p)
		for _, a := range []Action{{Do: "suspension", Team: "home"}, {Do: "suspension_number", ID: 1}, {Do: "suspension_end", ID: 1}} {
			if mn.Do(a) == nil {
				t.Errorf("%s accepted %s", p.ID, a.Do)
			}
		}
	}
	if err := (func() error { mn, _, _ := newHandball(); return mn.Do(Action{Do: "suspension", Team: "x"}) })(); err == nil {
		t.Error("a bad team was accepted")
	}
}

func TestSuspensionsExpireInTheFrame(t *testing.T) {
	mn, _, _ := newHandball()
	do(t, mn, Action{Do: "suspension", Team: "home"}, Action{Do: "clock_set", Tenths: 1200})
	if got := suspensionsJSON(mn.s); got != "[]" {
		t.Errorf("an ended suspension is still sent: %s", got)
	}
	if s := mn.Score(); s.HomeSusp[0].ID != 0 {
		t.Errorf("slot not emptied: %+v", s.HomeSusp)
	}
}

func TestSetLengthAcrossACountUpSwitch(t *testing.T) {
	prof := sport.Basketball
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	mn := NewManual(&Manual{Now: c.now, Rules: func() sport.Profile { return prof }})
	prof = sport.Handball
	do(t, mn, Action{Do: "suspension", Team: "home"})
	if err := mn.SetLengthFrom(sport.Basketball, 18000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 18000 || s.Clock != 0 || s.HomeSusp[0].ID != 0 {
		t.Errorf("basketball to handball: %+v", s)
	}
	prof = sport.Basketball
	if err := mn.SetLengthFrom(sport.Handball, 6000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 6000 || s.Clock != 6000 {
		t.Errorf("handball to basketball: %+v", s)
	}
	// A game under way is left alone, suspensions included.
	prof = sport.Handball
	do(t, mn, Action{Do: "new_game"}, Action{Do: "suspension", Team: "away"}, Action{Do: "point", Team: "home", N: 1})
	prof = sport.Basketball
	if err := mn.SetLengthFrom(sport.Handball, 6000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Clock != 0 || s.AwaySusp[0].ID != 1 {
		t.Errorf("a game under way was touched: %+v", s)
	}
}

// A countdown at 0:00 in a fresh game is not "at the old start": the switch to
// another countdown leaves it there. A fresh game with the clock moved by hand
// keeps its suspensions, since only a clock at the old start is a new game.
func TestSetLengthOnlyMovesAClockAtTheOldStart(t *testing.T) {
	mn, _, _ := newSport(sport.Futsal)
	do(t, mn, Action{Do: "clock_set", Tenths: 0})
	if err := mn.SetLengthFrom(sport.Basketball, 12000); err != nil {
		t.Fatal(err)
	}
	if s := mn.Score(); s.Length != 12000 || s.Clock != 0 {
		t.Errorf("countdown to countdown with the clock at 0: %+v, want the clock left at 0", s)
	}

	hb, _, _ := newHandball()
	do(t, hb, Action{Do: "clock_set", Tenths: 5000}, Action{Do: "suspension", Team: "home"})
	if err := hb.SetLengthFrom(sport.Basketball, 15000); err != nil {
		t.Fatal(err)
	}
	if s := hb.Score(); s.Length != 15000 || s.Clock != 5000 || s.HomeSusp[0].ID != 1 {
		t.Errorf("a clock set by hand lost its suspension or moved: %+v", s)
	}
}

// The time left rounds up: the last second reads 00:01, never 00:00.
func TestSuspensionLeftRoundsUp(t *testing.T) {
	mn, _, _ := newHandball()
	do(t, mn, Action{Do: "suspension", Team: "home"})
	for _, c := range []struct {
		clock int
		want  string
	}{{0, "02:00"}, {600, "01:00"}, {1100, "00:10"}, {1191, "00:01"}, {1195, "00:01"}} {
		s := mn.s
		s.Clock = c.clock
		var list []struct{ Left string }
		if err := json.Unmarshal([]byte(suspensionsJSON(s)), &list); err != nil || len(list) != 1 || list[0].Left != c.want {
			t.Errorf("clock %d: %s, want left %s", c.clock, suspensionsJSON(s), c.want)
		}
	}
	s := mn.s
	s.Clock = 1200
	if got := suspensionsJSON(s); got != "[]" {
		t.Errorf("a suspension at its end is still sent: %s", got)
	}
}

// A console-less take-over in the second half of handball puts the clock at
// the start of that half, not the length.
func TestSeedHandballWithNoClock(t *testing.T) {
	mn, _, _ := newHandball()
	mn.Seed(state.Live{Period: "2", Home: 14, Away: 12})
	if s := mn.Score(); s.Period != 2 || s.Clock != 18000 || s.Home != 14 {
		t.Errorf("seeded %+v, want period 2 at 18000", s)
	}
	mn.Seed(state.Live{Period: "3"})
	if s := mn.Score(); s.Period != 3 || s.Clock != 36000 {
		t.Errorf("seeded %+v, want extra time at 36000", s)
	}
}

func TestRestoreSanitisesSuspensions(t *testing.T) {
	mn, _, _ := newHandball()
	var s Score
	s.Period, s.Length = 1, 18000
	s.HomeSusp[0] = Suspension{ID: 4, Number: "abc", End: 5000}
	s.HomeSusp[1] = Suspension{ID: -2, Number: "1", End: 5000}
	s.AwaySusp[0] = Suspension{ID: 2, Number: "77", End: -9}
	s.NextSusp = 0
	mn.Restore(s)
	got := mn.s
	if got.HomeSusp[0].Number != "" || got.HomeSusp[0].ID != 4 {
		t.Errorf("a bad number stayed: %+v", got.HomeSusp[0])
	}
	if got.HomeSusp[1].ID != 0 {
		t.Errorf("a negative ID stayed: %+v", got.HomeSusp[1])
	}
	if got.AwaySusp[0].End != 0 || got.AwaySusp[0].Number != "77" {
		t.Errorf("end or number wrong: %+v", got.AwaySusp[0])
	}
	if got.NextSusp != 4 {
		t.Errorf("NextSusp %d, want the highest restored ID 4", got.NextSusp)
	}
	s.NextSusp = 1 << 40
	mn.Restore(s)
	if mn.s.NextSusp != maxSuspID {
		t.Errorf("NextSusp %d, want it capped at %d", mn.s.NextSusp, maxSuspID)
	}
}

func TestRestoreKeepsSuspensions(t *testing.T) {
	mn, _, _ := newHandball()
	do(t, mn, Action{Do: "suspension", Team: "home"}, Action{Do: "suspension_number", ID: 1, Number: "9"})
	saved := mn.Score()
	b, _ := json.Marshal(saved)
	var back Score
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	mn2, _, _ := newHandball()
	mn2.Restore(back)
	if got := mn2.Score(); got.HomeSusp[0] != saved.HomeSusp[0] || got.NextSusp != 1 {
		t.Errorf("restored %+v", got)
	}
}

// A sport with no foul marks (handball) takes no foul; futsal and basketball do.
func TestFoulRefusedWhereASportHasNone(t *testing.T) {
	mn, _, _ := newHandball()
	before := mn.Score()
	if err := mn.Do(Action{Do: "foul", Team: "home", N: 1}); err == nil || !sameScore(mn.Score(), before) {
		t.Errorf("handball took a foul: %v", err)
	}
	for _, p := range []sport.Profile{sport.Basketball, sport.Futsal} {
		mn, _, _ := newSport(p)
		if err := mn.Do(Action{Do: "foul", Team: "home", N: 1}); err != nil {
			t.Errorf("%s refused a foul: %v", p.ID, err)
		}
	}
}

func TestSameScoreNilLists(t *testing.T) {
	a, b := newScoreAt(DefaultLength, DefaultLength), newScoreAt(DefaultLength, DefaultLength)
	if !sameScore(a, b) {
		t.Error("two new games differ")
	}
	b.Cards = []Card{}
	if sameScore(a, b) {
		t.Error("a nil list and an empty one count as the same; lists must stay nil until used")
	}
}

func newSoccer() (*Manual, *state.Machine, *fakeClock) {
	mn, m, c := newSport(sport.Soccer)
	do2 := func(a Action) {
		if err := mn.Do(a); err != nil {
			panic(err)
		}
	}
	do2(Action{Do: "period_length", Tenths: 27000})
	do2(Action{Do: "new_game"})
	return mn, m, c
}

func card(team, number, kind string) Action {
	return Action{Do: "card", Team: team, Number: number, Kind: kind}
}

func TestSoccerCards(t *testing.T) {
	mn, _, _ := newSoccer()
	do(t, mn, card("home", "7", "yellow"), card("home", "7", "yellow"))
	cs := mn.Score().Cards
	if len(cs) != 2 || cs[0].Kind != "yellow" || cs[1].Kind != "second_yellow" {
		t.Fatalf("two yellows: %+v", cs)
	}
	if err := mn.Do(card("home", "7", "red")); !errors.Is(err, errSentOff) {
		t.Errorf("a card after a second yellow: %v", err)
	}
	if err := mn.Do(card("away", "7", "yellow")); err != nil {
		t.Errorf("the same number on the other team is another player: %v", err)
	}
	do(t, mn, card("home", "08", "red"), card("home", "9", "red"))
	if err := mn.Do(card("home", "9", "yellow")); !errors.Is(err, errSentOff) {
		t.Errorf("a card after a red: %v", err)
	}
	if err := mn.Do(card("home", "008", "yellow")); !errors.Is(err, errSentOff) {
		t.Errorf("008 is shirt 8: %v", err)
	}
	if got := mn.Score().Cards[3]; got.Number != "8" {
		t.Errorf("number not normalised: %+v", got)
	}
	for _, a := range []Action{card("home", "", "red"), card("home", "1234", "red"), card("home", "x", "red"),
		card("home", "5", "blue"), card("home", "5", "second_yellow"), card("casa", "5", "red")} {
		if err := mn.Do(a); !errors.Is(err, errCard) {
			t.Errorf("%+v: %v", a, err)
		}
	}
}

func TestSoccerSubs(t *testing.T) {
	mn, _, _ := newSoccer()
	for _, a := range []Action{
		{Do: "sub", Team: "home", Out: "7", In: "7"}, {Do: "sub", Team: "home", Out: "0", In: "7"},
		{Do: "sub", Team: "home", Out: "100", In: "7"}, {Do: "sub", Team: "home", Out: "7", In: ""},
		{Do: "sub", Team: "home", Out: "7", In: "100"}, {Do: "sub", Team: "x", Out: "7", In: "8"},
	} {
		if err := mn.Do(a); !errors.Is(err, errSub) {
			t.Errorf("%+v: %v", a, err)
		}
	}
	do(t, mn, Action{Do: "sub", Team: "away", Out: "9", In: "99"})
	if got := mn.Score().Subs; len(got) != 1 || got[0].Out != "9" || got[0].In != "99" {
		t.Errorf("subs %+v", got)
	}
}

func TestSoccerUndoKeepsListsApart(t *testing.T) {
	mn, _, _ := newSoccer()
	before := mn.Score()
	do(t, mn, card("home", "4", "yellow"))
	if sameScore(before, mn.Score()) {
		t.Error("a card-only change is dropped as no change")
	}
	do(t, mn, Action{Do: "undo"})
	if mn.Score().Cards != nil || !sameScore(before, mn.Score()) {
		t.Errorf("after the undo: %+v", mn.Score().Cards)
	}
	// Older snapshots keep their lists when the live one grows.
	do(t, mn, card("home", "4", "yellow"), card("home", "5", "yellow"), card("home", "6", "yellow"))
	do(t, mn, Action{Do: "undo"}, card("away", "8", "red"))
	do(t, mn, Action{Do: "undo"})
	if got := mn.Score().Cards; len(got) != 2 || got[0].Number != "4" || got[1].Number != "5" {
		t.Errorf("cards %+v", got)
	}
	do(t, mn, Action{Do: "undo"}, Action{Do: "undo"})
	if mn.Score().Cards != nil {
		t.Errorf("undo to the start leaves %+v, want nil", mn.Score().Cards)
	}
	// Subs only differing is a change too.
	a, b := before, before
	b.Subs = []Sub{{ID: 1}}
	c := before
	c.Cards = []Card{{ID: 1}}
	if sameScore(a, b) || sameScore(a, c) {
		t.Error("sameScore misses a list")
	}
}

func TestSoccerCardIDsGoUpEvenAfterUndo(t *testing.T) {
	mn, _, _ := newSoccer()
	do(t, mn, card("home", "4", "yellow"), card("home", "5", "yellow"), Action{Do: "undo"}, card("home", "6", "yellow"))
	cs := mn.Score().Cards
	if len(cs) != 2 || cs[0].ID != 1 || cs[1].ID != 3 {
		t.Errorf("ids %+v", cs)
	}
}

func TestSoccerStoppageAndPeriods(t *testing.T) {
	mn, _, _ := newSoccer()
	do(t, mn, Action{Do: "stoppage", N: 4})
	if mn.Score().Stoppage != 4 {
		t.Error("stoppage not set")
	}
	for _, n := range []int{-1, 16} {
		if err := mn.Do(Action{Do: "stoppage", N: n}); !errors.Is(err, errStoppage) {
			t.Errorf("stoppage %d: %v", n, err)
		}
	}
	do(t, mn, Action{Do: "period_next"})
	if s := mn.Score(); s.Stoppage != 0 || s.Clock != 27000 || s.Period != 2 {
		t.Errorf("after period_next: %+v", s)
	}
	if err := mn.Do(Action{Do: "period_next"}); !errors.Is(err, errNoExtra) {
		t.Errorf("period_next past the last half: %v", err)
	}
	if mn.Score().Period != 2 {
		t.Error("period moved")
	}
}

func TestSoccerGameEnd(t *testing.T) {
	mn, _, c := newSoccer()
	if err := mn.Do(Action{Do: "game_end"}); !errors.Is(err, errEndEarly) {
		t.Errorf("game_end in period 1: %v", err)
	}
	do(t, mn, Action{Do: "clock_start"})
	c.add(47*time.Minute + 12*time.Second)
	if mn.Score().Clock != 28320 {
		t.Fatalf("clock %d", mn.Score().Clock)
	}
	do(t, mn, Action{Do: "period_next"}, Action{Do: "clock_start"})
	c.add(time.Minute)
	do(t, mn, Action{Do: "game_end"})
	if s := mn.Score(); !s.Over || s.Running {
		t.Errorf("after game_end: %+v", s)
	}
	do(t, mn, Action{Do: "undo"})
	if mn.Score().Over {
		t.Error("undo leaves Over")
	}
	do(t, mn, Action{Do: "game_end"}, Action{Do: "clock_start"})
	if s := mn.Score(); s.Over || !s.Running {
		t.Errorf("clock_start after game_end: %+v", s)
	}
	do(t, mn, Action{Do: "clock_stop"}, Action{Do: "game_end"}, Action{Do: "period_back"})
	if s := mn.Score(); s.Over || s.Period != 1 {
		t.Errorf("period_back after game_end: %+v", s)
	}
}

func TestSoccerClockRunsPastTheHalf(t *testing.T) {
	mn, _, c := newSoccer()
	do(t, mn, Action{Do: "clock_start"})
	c.add(47*time.Minute + 12*time.Second)
	if s := mn.Score(); s.Clock != 28320 || !s.Running {
		t.Errorf("clock %d running %v", s.Clock, s.Running)
	}
	do(t, mn, Action{Do: "period_next"})
	if mn.Score().Clock != 27000 {
		t.Errorf("second half starts at %d", mn.Score().Clock)
	}
	do(t, mn, Action{Do: "period_back"}, Action{Do: "period_length", Tenths: 21000}, Action{Do: "period_next"})
	if mn.Score().Clock != 21000 {
		t.Errorf("35 minute second half starts at %d", mn.Score().Clock)
	}
}

func TestOtherSportsRefuseCards(t *testing.T) {
	for _, p := range []sport.Profile{sport.Basketball, sport.Handball, sport.Volleyball} {
		mn, _, _ := newSport(p)
		for _, a := range []Action{card("home", "4", "red"), {Do: "sub", Team: "home", Out: "1", In: "2"}, {Do: "stoppage", N: 2}, {Do: "game_end"}} {
			if err := mn.Do(a); !errors.Is(err, errNoCards) {
				t.Errorf("%s %s: %v", p.ID, a.Do, err)
			}
		}
		if _, ok := frame(p, mn.Score())["cards"]; ok {
			t.Errorf("%s frame carries cards", p.ID)
		}
	}
}

func TestMinute(t *testing.T) {
	r := sport.Soccer
	for _, c := range []struct {
		period, clock int
		want          string
	}{
		{1, 0, "1'"}, {1, 22*600 + 100, "23'"}, {1, 27000, "45'"}, {1, 27001, "45+1'"}, {1, 28320, "45+3'"},
		{2, 27000, "46'"}, {2, 54000, "90'"}, {2, 54001, "90+1'"}, {2, 54000 + 4*600, "90+4'"},
	} {
		if got := minute(r, 27000, c.period, c.clock); got != c.want {
			t.Errorf("minute(%d, %d) = %q, want %q", c.period, c.clock, got, c.want)
		}
	}
	if got := minute(r, 21000, 2, 42001); got != "70+1'" {
		t.Errorf("35 minute halves: %q", got)
	}
}

func TestSoccerFrame(t *testing.T) {
	mn, _, _ := newSoccer()
	f := frame(sport.Soccer, mn.Score())
	if f["cards"] != "[]" || f["subs"] != "[]" || f["over"] != "" || f["stoppage"] != "0" || f["period_end"] != "27000" {
		t.Errorf("empty frame %v", f)
	}
	do(t, mn, card("home", "4", "yellow"), Action{Do: "sub", Team: "away", Out: "9", In: "12"}, Action{Do: "stoppage", N: 3})
	f = frame(sport.Soccer, mn.Score())
	if f["cards"] != `[{"id":1,"team":"home","number":"4","kind":"yellow","minute":"1'"}]` ||
		f["subs"] != `[{"id":2,"team":"away","out":"9","in":"12","minute":"1'"}]` || f["stoppage"] != "3" {
		t.Errorf("frame %v", f)
	}
}

func TestSoccerRestore(t *testing.T) {
	mn, _, _ := newSoccer()
	do(t, mn, card("home", "4", "yellow"), Action{Do: "sub", Team: "home", Out: "1", In: "2"})
	saved := mn.Score()
	saved.Cards = append(saved.Cards, Card{ID: 0, Team: "home", Number: "9", Kind: "red"}, Card{ID: 7, Team: "x", Number: "9", Kind: "red"})
	saved.Stoppage = 99
	back, _, _ := newSoccer()
	back.Restore(saved)
	got := back.Score()
	if len(got.Cards) != 1 || len(got.Subs) != 1 || got.Stoppage != maxStoppage {
		t.Errorf("restored %+v", got)
	}
	do(t, back, card("home", "5", "yellow"))
	if cs := back.Score().Cards; cs[len(cs)-1].ID != 3 {
		t.Errorf("next id %+v", cs)
	}
	empty := newScoreAt(27000, 0)
	back.Restore(empty)
	if s := back.Score(); s.Cards != nil || s.Subs != nil {
		t.Error("restore turned a nil list into an empty one")
	}
}
