package state

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTest() (*Machine, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)}
	return New(c.now), c
}

// step is one box frame: period, clock, score, and seconds to wait before it.
type step struct {
	wait        int
	period      string
	clock       string
	home, away  int
	wantScreen  string
	wantPeriods int
}

func run(t *testing.T, m *Machine, c *fakeClock, steps []step) {
	t.Helper()
	for i, s := range steps {
		c.t = c.t.Add(time.Duration(s.wait) * time.Second)
		m.Tick()
		m.ApplyConsole(map[string]string{
			"period": s.period, "clock": s.clock,
			"home": strconv.Itoa(s.home), "away": strconv.Itoa(s.away),
		})
		got := m.Snapshot()
		if s.wantScreen != "" && got.Screen.Current != s.wantScreen {
			t.Errorf("step %d: screen %q, want %q", i, got.Screen.Current, s.wantScreen)
		}
		if len(got.Periods) != s.wantPeriods {
			t.Errorf("step %d: %d periods, want %d", i, len(got.Periods), s.wantPeriods)
		}
	}
}

func TestPeriodsAndScreens(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
		want  []Period
	}{
		{
			name: "half-time and back to the scoreboard",
			steps: []step{
				{0, "1", "10:00", 0, 0, "marcador", 0},
				{1, "1", "00:00", 20, 18, "marcador", 0},
				{1, "2", "10:00", 20, 18, "marcador", 1},
				{1, "2", "0.0", 40, 41, "marcador", 1},
				{1, "3", "00:00", 40, 41, "intervalo", 2},
				{1, "3", "10:00", 40, 41, "intervalo", 2}, // clock reset, not running
				{1, "3", "09:59", 40, 41, "marcador", 2},
			},
			want: []Period{{1, 20, 18, 20, 18}, {2, 40, 41, 20, 23}},
		},
		{
			name: "final after 60 s at zero",
			steps: []step{
				{0, "4", "00:10", 70, 60, "marcador", 0},
				{1, "4", "0.0", 72, 60, "marcador", 0},
				{30, "4", "0.0", 72, 60, "marcador", 0},
				{29, "4", "0.0", 72, 60, "marcador", 0},
				{1, "4", "0.0", 72, 60, "final", 1},
				{30, "4", "0.0", 72, 60, "final", 1},
			},
			want: []Period{{4, 72, 60, 72, 60}},
		},
		{
			name: "overtime starts before the hold ends",
			steps: []step{
				{0, "4", "0.0", 80, 80, "marcador", 0},
				{40, "5", "05:00", 80, 80, "marcador", 1},
				{1, "5", "04:59", 82, 80, "marcador", 1},
			},
			want: []Period{{4, 80, 80, 80, 80}},
		},
		{
			name: "overtime after the final screen goes back to the scoreboard",
			steps: []step{
				{0, "4", "0.0", 80, 80, "marcador", 0},
				{61, "4", "0.0", 80, 80, "final", 1},
				{60, "5", "05:00", 80, 80, "final", 1},
				{1, "5", "04:59", 80, 80, "marcador", 1},
			},
			want: []Period{{4, 80, 80, 80, 80}},
		},
		{
			name: "clock at zero in period 1 is not the end",
			steps: []step{
				{0, "1", "0.0", 10, 10, "marcador", 0},
				{90, "1", "0.0", 10, 10, "marcador", 0},
			},
		},
		{
			name: "period going back does not record",
			steps: []step{
				{0, "3", "05:00", 50, 50, "", 0},
				{1, "1", "10:00", 0, 0, "", 0},
				{1, "", "", 0, 0, "", 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, c := newTest()
			run(t, m, c, tt.steps)
			got := m.Snapshot().Periods
			if tt.want == nil {
				tt.want = []Period{}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("periods %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManualWinsUntilNextTrigger(t *testing.T) {
	m, c := newTest()
	run(t, m, c, []step{
		{0, "2", "0.0", 30, 30, "marcador", 0},
		{1, "3", "10:00", 30, 30, "intervalo", 1},
	})
	if err := m.SetScreen("plantel", true); err != nil {
		t.Fatal(err)
	}
	run(t, m, c, []step{
		{1, "3", "09:59", 30, 30, "plantel", 1}, // the intervalo rule no longer applies
	})
}

func TestAutoOff(t *testing.T) {
	m, c := newTest()
	m.SetScreen("", false)
	run(t, m, c, []step{
		{0, "2", "0.0", 30, 30, "marcador", 0},
		{1, "3", "10:00", 30, 30, "marcador", 1},
	})
}

func TestUnknownScreen(t *testing.T) {
	m, _ := newTest()
	if m.SetScreen("emblema", true) == nil {
		t.Error("want an error")
	}
}

func TestLowerThirdHides(t *testing.T) {
	m, c := newTest()
	m.ShowLowerThird(LowerThird{Type: "jogador", Number: "7"}, 8*time.Second)
	c.t = c.t.Add(7 * time.Second)
	m.Tick()
	if !m.Snapshot().LowerThird.Visible {
		t.Fatal("hidden too early")
	}
	c.t = c.t.Add(time.Second)
	m.Tick()
	if lt := m.Snapshot().LowerThird; lt.Visible || lt.Until != nil {
		t.Fatal("still visible at until")
	}
}

func TestNamesFallBackToConsole(t *testing.T) {
	m, _ := newTest()
	m.SetGame(Game{Home: Team{Name: "CASA", Short: "CAS"}, Away: Team{Name: "VISITANTE"}}, false)
	m.ApplyConsole(map[string]string{"home_name": "CASA", "away_name": "GRUPO DESPORTIVO DA PONTE VELHA"})
	g := m.Snapshot().Game
	if g.Home.Name != "CASA" || g.Away.Name != "GRUPO DESPOR" || g.Away.Initials != "GRU" {
		t.Errorf("got home %q away %q initials %q", g.Home.Name, g.Away.Name, g.Away.Initials)
	}
	m.SetGame(Game{Away: Team{Name: "FARPAS", Short: "FAR"}}, true)
	if g := m.Snapshot().Game; g.Away.Name != "FARPAS" || g.Away.Initials != "FAR" {
		t.Errorf("picked game: away %q initials %q", g.Away.Name, g.Away.Initials)
	}
	// A picked opponent keeps up to 20 characters.
	m.SetGame(Game{Away: Team{Name: "UNIVERSITÁRIO VALBOA NORTE", Short: "CDUL"}}, true)
	if g := m.Snapshot().Game; g.Away.Name != "UNIVERSITÁRIO VALBOA" {
		t.Errorf("picked long name: %q", g.Away.Name)
	}
}

func TestOnPeriodsHook(t *testing.T) {
	m, c := newTest()
	var got []Period
	m.OnPeriods = func(s State) { got = s.Periods }
	run(t, m, c, []step{
		{0, "1", "0.0", 5, 3, "", 0},
		{1, "2", "10:00", 5, 3, "", 1},
	})
	if len(got) != 1 || got[0] != (Period{1, 5, 3, 5, 3}) {
		t.Errorf("hook got %v", got)
	}
}

func TestPeriodPoints(t *testing.T) {
	m, c := newTest()
	var saved [][]Period
	m.SavePeriods = func(ps []Period) { saved = append(saved, ps) }
	run(t, m, c, []step{
		{0, "1", "10:00", 0, 0, "", 0},
		{1, "1", "0.0", 18, 15, "", 0},
		{1, "2", "10:00", 18, 15, "", 1},
		{1, "2", "05:00", 26, 20, "", 1},
		{1, "2", "04:59", 24, 20, "", 1}, // a basket taken back
		{1, "2", "0.0", 32, 29, "", 1},
		{1, "3", "10:00", 32, 29, "", 2},
		{1, "3", "0.0", 32, 29, "", 2}, // nobody scores
		{1, "4", "10:00", 32, 29, "", 3},
		{1, "4", "05:00", 31, 35, "", 3}, // home corrected below the end of period 3
		{1, "4", "0.0", 31, 36, "", 3},
		{1, "5", "05:00", 31, 36, "", 4},
		{1, "5", "0.0", 40, 38, "", 4},
		{1, "6", "05:00", 40, 38, "", 5},
	})
	want := []Period{
		{1, 18, 15, 18, 15},
		{2, 32, 29, 14, 14},
		{3, 32, 29, 0, 0},
		{4, 31, 36, 0, 7},
		{5, 40, 38, 9, 2},
	}
	if got := m.Snapshot().Periods; !reflect.DeepEqual(got, want) {
		t.Errorf("periods %v, want %v", got, want)
	}
	if len(saved) != 5 || !reflect.DeepEqual(saved[4], want) {
		t.Errorf("saved %v", saved)
	}

	m.ResetPeriods()
	if got := m.Snapshot().Periods; len(got) != 0 {
		t.Errorf("after reset %v", got)
	}
	if last := saved[len(saved)-1]; len(last) != 0 {
		t.Errorf("reset saved %v", last)
	}
	run(t, m, c, []step{
		{1, "1", "0.0", 10, 12, "", 0},
		{1, "2", "10:00", 10, 12, "", 1},
	})
	if got := m.Snapshot().Periods; !reflect.DeepEqual(got, []Period{{1, 10, 12, 10, 12}}) {
		t.Errorf("new game %v", got)
	}
}

func TestRestoredPeriodsCarryOn(t *testing.T) {
	m, c := newTest()
	m.RestorePeriods([]Period{{1, 18, 15, 18, 15}, {2, 32, 29, 14, 14}})
	run(t, m, c, []step{
		{0, "3", "04:00", 40, 33, "", 2}, // first frame after the restart records nothing
		{1, "3", "0.0", 44, 37, "", 2},
		{1, "4", "10:00", 44, 37, "", 3},
	})
	if got := m.Snapshot().Periods[2]; got != (Period{3, 44, 37, 12, 8}) {
		t.Errorf("period 3 %v", got)
	}
}

// volleyball is a machine playing a volleyball game, with the events it told.
func volleyball() (m *Machine, events *[]string) {
	m, _ = newTest()
	m.SetGame(Game{Sport: "volleyball"}, false)
	events = &[]string{}
	m.OnEvent = func(ev string, f map[string]any) {
		switch ev {
		case "set":
			*events = append(*events, "set"+strconv.Itoa(f["set"].(int)))
		case "screen":
			*events = append(*events, "screen:"+f["name"].(string))
		}
	}
	return m, events
}

// vframe is one frame of a sets sport: points, sets won and the finished sets.
func vframe(m *Machine, home, away, hs, as int, sets string) {
	m.ApplyConsole(map[string]string{"home": strconv.Itoa(home), "away": strconv.Itoa(away),
		"home_sets": strconv.Itoa(hs), "away_sets": strconv.Itoa(as), "sets": sets, "period": strconv.Itoa(hs + as + 1)})
}

func TestSetsFromThePhoneScorer(t *testing.T) {
	m, events := volleyball()
	vframe(m, 25, 20, 0, 0, "")
	vframe(m, 0, 0, 1, 0, "25-20")
	want := []SetScore{{1, 25, 20, "home"}}
	if got := m.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Errorf("sets %v, want %v", got, want)
	}
	vframe(m, 0, 0, 1, 1, "25-20,18-25")
	if got := m.Snapshot().Sets; len(got) != 2 || got[1] != (SetScore{2, 18, 25, "away"}) {
		t.Errorf("sets %v", got)
	}
	for _, e := range *events {
		if e == "screen:intervalo" {
			t.Error("a set end switched to the intervalo")
		}
	}
	if !reflect.DeepEqual(*events, []string{"set1", "set2"}) {
		t.Errorf("events %v", *events)
	}
}

func TestSetsFromTheConsoleUseTheLastPoints(t *testing.T) {
	m, _ := volleyball()
	m.ApplyConsole(map[string]string{"home": "24", "away": "20", "home_sets": "0", "away_sets": "0"})
	m.ApplyConsole(map[string]string{"home": "25", "away": "20", "home_sets": "0", "away_sets": "0"})
	m.ApplyConsole(map[string]string{"home": "0", "away": "0", "home_sets": "1", "away_sets": "0"})
	want := []SetScore{{1, 25, 20, "home"}}
	if got := m.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Errorf("sets %v, want %v", got, want)
	}
	// Two sets at once (a restart on a console mid-game): winners only.
	m, _ = volleyball()
	m.ApplyConsole(map[string]string{"home": "3", "away": "4", "home_sets": "1", "away_sets": "1"})
	want = []SetScore{{1, 0, 0, "home"}, {2, 0, 0, "away"}}
	if got := m.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Errorf("sets %v, want %v", got, want)
	}
}

func TestSetsDropOnUndo(t *testing.T) {
	m, _ := volleyball()
	vframe(m, 0, 0, 1, 0, "25-20")
	vframe(m, 25, 20, 0, 0, "")
	if got := m.Snapshot().Sets; len(got) != 0 {
		t.Errorf("an undo left %v", got)
	}
}

func TestMatchEndsOnceAndAgainAfterUndo(t *testing.T) {
	m, events := volleyball()
	vframe(m, 0, 0, 1, 0, "25-20")
	vframe(m, 0, 0, 2, 0, "25-20,25-18")
	if m.Snapshot().Screen.Current != "marcador" {
		t.Errorf("screen %q before the match is won", m.Snapshot().Screen.Current)
	}
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	if m.Snapshot().Screen.Current != "final" {
		t.Errorf("screen %q, want final", m.Snapshot().Screen.Current)
	}
	vframe(m, 25, 22, 2, 0, "25-20,25-18") // undo across the match end
	if m.Snapshot().Screen.Current != "marcador" {
		t.Errorf("screen %q after the undo, want marcador", m.Snapshot().Screen.Current)
	}
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	finals := 0
	for _, e := range *events {
		if e == "screen:final" {
			finals++
		}
	}
	if finals != 2 || m.Snapshot().Screen.Current != "final" {
		t.Errorf("final fired %d times (events %v)", finals, *events)
	}
}

func TestRestoredSetsCarryOn(t *testing.T) {
	m, events := volleyball()
	m.RestoreSets([]SetScore{{1, 25, 20, "home"}, {2, 18, 25, "away"}})
	vframe(m, 10, 5, 1, 1, "25-20,18-25") // first frame after the restart
	vframe(m, 0, 0, 2, 1, "25-20,18-25,25-23")
	if !reflect.DeepEqual(*events, []string{"set3"}) {
		t.Errorf("events %v, want only set3", *events)
	}
}

func TestSavedSetsFollowTheMachine(t *testing.T) {
	m, _ := volleyball()
	var saved [][]SetScore
	m.SaveSets = func(ss []SetScore) { saved = append(saved, ss) }
	vframe(m, 0, 0, 1, 0, "25-20")
	m.ResetSets()
	if len(saved) != 2 || len(saved[0]) != 1 || len(saved[1]) != 0 {
		t.Errorf("saved %v", saved)
	}
}

func TestDemoKeepsTheGamesPeriods(t *testing.T) {
	m, _ := newTest()
	saves := 0
	m.SavePeriods = func([]Period) { saves++ }
	game := []Period{{1, 18, 15, 18, 15}}
	m.RestorePeriods(game)
	m.SetDemo(true)
	m.update(Live{Period: "1", Home: 2}, true)
	m.update(Live{Period: "2", Home: 2}, true)
	m.SetDemo(false)
	if got := m.Snapshot().Periods; !reflect.DeepEqual(got, game) {
		t.Errorf("after the demo %v, want %v", got, game)
	}
	if saves != 0 {
		t.Errorf("the demo saved periods %d times", saves)
	}
	// The box's first frame after the demo is not the end of the demo's period.
	m.ApplyConsole(map[string]string{"period": "3", "home": "40", "away": "38"})
	if got := m.Snapshot().Periods; !reflect.DeepEqual(got, game) || saves != 0 {
		t.Errorf("after the first box frame %v, %d saves", got, saves)
	}
}

func TestManualFramesDoNotConnectTheBox(t *testing.T) {
	m, _ := newTest()
	if in := m.Snapshot().Input; in != "console" {
		t.Fatalf("input %q, want console", in)
	}
	m.ApplyConsole(map[string]string{"period": "1", "home": "2"})
	if !m.Snapshot().Box.Connected {
		t.Fatal("a box frame does not connect")
	}
	m.SetInput("manual")
	if m.Snapshot().Box.Connected {
		t.Fatal("still connected after the input changed")
	}
	m.ApplyConsole(map[string]string{"period": "1", "home": "4", "home_timeouts": "1"})
	s := m.Snapshot()
	if s.Box.Connected || s.Live.Home != 4 || s.Live.HomeTimeouts != 1 || s.Input != "manual" {
		t.Errorf("manual frame: connected %t, live %+v, input %q", s.Box.Connected, s.Live, s.Input)
	}
	m.SetBox(true)
	if !m.Snapshot().Box.Connected {
		t.Error("SetBox does not connect the scorer")
	}
}

func TestClockText(t *testing.T) {
	for in, want := range map[int]string{6000: "10:00", 600: "01:00", 599: "59.9", 0: "0.0"} {
		if got := ClockText(in); got != want {
			t.Errorf("ClockText(%d) = %q, want %q", in, got, want)
		}
		if got := Tenths(want); got != in {
			t.Errorf("Tenths(%q) = %d, want %d", want, got, in)
		}
	}
	for in, want := range map[int]string{5999: "10:00", 5991: "10:00", 5990: "09:59", 601: "01:01"} {
		if got := ClockText(in); got != want {
			t.Errorf("ClockText(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestClockRanWithin(t *testing.T) {
	m, c := newTest()
	if m.ClockRanWithin(30 * time.Minute) {
		t.Fatal("clock ran before any frame")
	}
	m.ApplyConsole(map[string]string{"period": "1", "clock": "10:00"})
	m.ApplyConsole(map[string]string{"period": "1", "clock": "10:00"})
	if m.ClockRanWithin(30 * time.Minute) {
		t.Fatal("a still clock counts as running")
	}
	m.ApplyConsole(map[string]string{"period": "1", "clock": "09:59"})
	c.t = c.t.Add(29 * time.Minute)
	if !m.ClockRanWithin(30 * time.Minute) {
		t.Fatal("clock moved 29 minutes ago")
	}
	c.t = c.t.Add(2 * time.Minute)
	if m.ClockRanWithin(30 * time.Minute) {
		t.Fatal("clock moved 31 minutes ago")
	}
}
