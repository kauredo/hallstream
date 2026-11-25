package state

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/sport"
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
	m.ResetPeriods()
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

// Futsal has two halves: the break falls after period 1, the final after
// period 2 once the clock has sat at zero for the hold.
func TestFutsalPeriodsAndScreens(t *testing.T) {
	m, c := newTest()
	m.SetGame(Game{Sport: "futsal", Rules: sport.Futsal}, true)
	run(t, m, c, []step{
		{0, "1", "20:00", 0, 0, "marcador", 0},
		{1, "1", "0.0", 3, 2, "marcador", 0},
		{1, "2", "20:00", 3, 2, "intervalo", 1},
		{1, "2", "19:59", 3, 2, "marcador", 1},
		{1, "2", "0.0", 5, 4, "marcador", 1},
		{59, "2", "0.0", 5, 4, "marcador", 1},
		{1, "2", "0.0", 5, 4, "final", 2},
	})
	if got := m.Snapshot().Periods; len(got) != 2 || got[0].Period != 1 || got[1].Period != 2 {
		t.Errorf("periods %v", got)
	}
}

func TestClockTextUp(t *testing.T) {
	for _, c := range []struct {
		t    int
		want string
	}{{0, "00:00"}, {125, "00:12"}, {129, "00:12"}, {18000, "30:00"}, {35999, "59:59"}, {36000, "60:00"}, {-5, "00:00"}} {
		if got := ClockTextUp(c.t); got != c.want {
			t.Errorf("ClockTextUp(%d) = %q, want %q", c.t, got, c.want)
		}
	}
	if Tenths(ClockTextUp(18000)) != 18000 {
		t.Error("Tenths does not read ClockTextUp")
	}
}

// Handball counts up: the break falls after period 1, the first frame of the
// second half at 30:00 is not yet the clock running, and the final comes once
// the clock has sat at 60:00 for the hold.
func TestHandballPeriodsAndScreens(t *testing.T) {
	m, c := newTest()
	m.SetGame(Game{Sport: "handball", Rules: sport.Handball}, true)
	run(t, m, c, []step{
		{0, "1", "00:00", 0, 0, "marcador", 0},
		{1, "1", "00:01", 0, 0, "marcador", 0},
		{1, "1", "30:00", 14, 12, "marcador", 0},
		{1, "2", "30:00", 14, 12, "intervalo", 1},
		{1, "2", "30:00", 14, 12, "intervalo", 1},
		{1, "2", "30:01", 14, 12, "marcador", 1},
		{1, "2", "59:59", 27, 25, "marcador", 1},
		{1, "2", "60:00", 27, 25, "marcador", 1},
		{59, "2", "60:00", 27, 25, "marcador", 1},
		{1, "2", "60:00", 27, 25, "final", 2},
	})
	if got := m.Snapshot().Periods; len(got) != 2 || got[1].Period != 2 {
		t.Errorf("periods %v", got)
	}
}

// A frame from the scorer says where its period ends, which holds for a half
// of any length; the clock at the start of period 1 never holds.
func TestHandballHoldUsesPeriodEnd(t *testing.T) {
	m, c := newTest()
	m.SetGame(Game{Sport: "handball", Rules: sport.Handball}, true)
	frame := func(p, clock, end string) {
		c.t = c.t.Add(time.Second)
		m.ApplyConsole(map[string]string{"period": p, "clock": clock, "period_end": end})
	}
	frame("1", "00:00", "15000")
	frame("2", "25:00", "30000")
	frame("2", "25:01", "30000")
	c.t = c.t.Add(time.Minute)
	frame("2", "50:00", "30000")
	c.t = c.t.Add(time.Minute)
	m.Tick()
	if got := m.Snapshot().Screen.Current; got != "final" {
		t.Errorf("a 25 minute half ended at 50:00 but the screen is %q", got)
	}
}

// A count-up that runs on (soccer) never holds at the end of the game; one
// that stops at the end of its period (handball) does, for final_hold.
func TestCountUpHoldDependsOnStopAtEnd(t *testing.T) {
	for _, stop := range []bool{false, true} {
		p := sport.Handball
		p.StopAtEnd = stop
		m, c := newTest()
		m.rulesFor = func(string) sport.Profile { return p }
		m.SetGame(Game{Sport: "handball", Rules: p}, true)
		for _, clock := range []string{"00:00", "30:00"} {
			m.ApplyConsole(map[string]string{"period": "2", "clock": clock})
		}
		m.ApplyConsole(map[string]string{"period": "2", "clock": "60:00"})
		c.t = c.t.Add(time.Minute)
		m.Tick()
		got := m.Snapshot().Screen.Current
		if stop && got != "final" {
			t.Errorf("StopAtEnd: screen %q at 60:00 after the hold, want final", got)
		}
		if !stop && got == "final" {
			t.Error("a clock that runs on ended the game")
		}
	}
}

func TestSuspensionsFromAFrame(t *testing.T) {
	m, _ := newTest()
	m.ApplyConsole(map[string]string{"period": "1", "clock": "05:00",
		"suspensions": `[{"team":"home","id":1,"number":"7","left":"01:23"},{"team":"away","id":2,"number":"","left":"00:10"}]`})
	got := m.Snapshot().Live.Suspensions
	want := []Suspension{{"home", 1, "7", "01:23"}, {"away", 2, "", "00:10"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("suspensions %+v", got)
	}
	m.ApplyConsole(map[string]string{"period": "1", "clock": "05:01", "home": "3", "suspensions": "not json"})
	l := m.Snapshot().Live
	if len(l.Suspensions) != 0 || l.Home != 3 {
		t.Errorf("a bad list: %d suspensions, home %d", len(l.Suspensions), l.Home)
	}
	m.ApplyConsole(map[string]string{"period": "1", "clock": "05:02", "home": "4"})
	if l := m.Snapshot().Live; len(l.Suspensions) != 0 || l.Home != 4 {
		t.Errorf("a frame with none: %+v", l)
	}
}

// events collects what the timeline is told.
func events(m *Machine) *[]string {
	var got []string
	m.OnEvent = func(e string, f map[string]any) {
		if e == "suspension" {
			got = append(got, f["team"].(string)+":"+f["number"].(string))
		}
	}
	return &got
}

func suspFrame(ids ...int) map[string]string {
	list := "["
	for i, id := range ids {
		if i > 0 {
			list += ","
		}
		list += `{"team":"home","id":` + strconv.Itoa(id) + `,"number":"` + strconv.Itoa(id) + `","left":"01:00"}`
	}
	return map[string]string{"period": "1", "clock": "05:00", "suspensions": list + "]"}
}

func TestSuspensionEvent(t *testing.T) {
	m, _ := newTest()
	got := events(m)
	m.ApplyConsole(suspFrame())
	m.ApplyConsole(suspFrame(1))
	m.ApplyConsole(suspFrame(1))
	if !reflect.DeepEqual(*got, []string{"home:1"}) {
		t.Fatalf("events %v, want one", *got)
	}
	m.ApplyConsole(suspFrame(1, 2))
	if len(*got) != 2 || (*got)[1] != "home:2" {
		t.Errorf("events %v", *got)
	}
}

func TestSuspensionEventMutedInTheDemo(t *testing.T) {
	m, _ := newTest()
	got := events(m)
	m.mu.Lock()
	m.s.Demo = true
	m.mu.Unlock()
	m.update(Live{Period: "1", Clock: "05:00", Suspensions: []Suspension{{"home", 1, "", "01:00"}}}, true)
	if len(*got) != 0 {
		t.Errorf("the demo told %v", *got)
	}
}

// After a restart the scorer holds suspension 3, already told. The first sync
// (SetGame) runs before its first frame, and must not make 3 news again.
func TestPrimeKeepsRestoredSuspensionsQuiet(t *testing.T) {
	m, _ := newTest()
	got := events(m)
	m.Prime(LiveOf(suspFrame(3)))
	m.SetGame(Game{Sport: "handball", Rules: sport.Handball}, true)
	m.ApplyConsole(suspFrame(3))
	if len(*got) != 0 {
		t.Fatalf("a restored suspension was told as new: %v", *got)
	}
	m.ApplyConsole(suspFrame(3, 4))
	if !reflect.DeepEqual(*got, []string{"home:4"}) {
		t.Errorf("events %v, want one for ID 4", *got)
	}
}

func TestPrimeLoadsTheLiveValues(t *testing.T) {
	m, _ := newTest()
	f := suspFrame(3)
	f["home"] = "7"
	m.Prime(LiveOf(f))
	if l := m.Snapshot().Live; l.Home != 7 || len(l.Suspensions) != 1 {
		t.Errorf("live after Prime: %+v", l)
	}
}

func TestResetPeriodsNumbersSuspensionsFromOneAgain(t *testing.T) {
	m, _ := newTest()
	got := events(m)
	m.Prime(LiveOf(suspFrame(3)))
	m.ResetPeriods()
	m.ApplyConsole(suspFrame(1))
	if !reflect.DeepEqual(*got, []string{"home:1"}) {
		t.Errorf("a new game's first suspension was not told: %v", *got)
	}
}

// An ID is told once: ending ID 2 early and undoing it brings the chip back
// but is no news.
func TestSuspensionIsToldOnce(t *testing.T) {
	m, _ := newTest()
	got := events(m)
	m.ApplyConsole(suspFrame(1, 2))
	m.ApplyConsole(suspFrame(1))
	m.ApplyConsole(suspFrame(1, 2))
	if !reflect.DeepEqual(*got, []string{"home:1", "home:2"}) {
		t.Errorf("events %v, want one each for IDs 1 and 2", *got)
	}
}

func TestLiveOfBoundsTheSuspensionList(t *testing.T) {
	v := suspFrame(1, 2, 3, 4, 5, 6, 7, 8)
	if got := LiveOf(v).Suspensions; len(got) != 6 {
		t.Errorf("%d suspensions, want 6", len(got))
	}
	v["suspensions"] = `[{"team":"cat","id":1,"left":"01:00"},{"team":"away","id":2,"left":"01:00"}]`
	if got := LiveOf(v).Suspensions; len(got) != 1 || got[0].Team != "away" {
		t.Errorf("a team that is neither home nor away got through: %+v", got)
	}
}

func soccer() (*Machine, *fakeClock) {
	m, c := newTest()
	m.SetGame(Game{Sport: "soccer", Rules: sport.Soccer}, true)
	return m, c
}

func sframe(period, clock string, home, away int, over bool) map[string]string {
	f := map[string]string{"period": period, "clock": clock, "period_end": "54000",
		"home": strconv.Itoa(home), "away": strconv.Itoa(away), "over": ""}
	if over {
		f["over"] = "1"
	}
	return f
}

// The clock runs past 90:00 and nothing but "Fim do jogo" ends the game.
func TestSoccerNoHoldPastTheEnd(t *testing.T) {
	m, c := soccer()
	m.ApplyConsole(sframe("1", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "45:01", 1, 0, false))
	m.ApplyConsole(sframe("2", "92:30", 1, 0, false))
	c.t = c.t.Add(5 * time.Minute)
	m.Tick()
	m.ApplyConsole(sframe("2", "92:30", 1, 0, false))
	c.t = c.t.Add(5 * time.Minute)
	m.Tick()
	if s := m.Snapshot(); s.Screen.Current == "final" || len(s.Periods) != 1 {
		t.Errorf("screen %q, periods %v: a stopped clock past 90:00 ended the game", s.Screen.Current, s.Periods)
	}
}

func TestSoccerOverBothWays(t *testing.T) {
	m, _ := soccer()
	var saved [][]Period
	m.SavePeriods = func(ps []Period) { saved = append(saved, ps) }
	m.ApplyConsole(sframe("1", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "46:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "90:05", 2, 1, true))
	m.ApplyConsole(sframe("2", "90:05", 2, 1, true))
	s := m.Snapshot()
	if s.Screen.Current != "final" || len(s.Periods) != 2 || s.Periods[1].Period != 2 || s.Periods[1].Home != 2 {
		t.Fatalf("after Over: screen %q periods %+v", s.Screen.Current, s.Periods)
	}
	n := len(saved)
	m.ApplyConsole(sframe("2", "90:05", 2, 1, false))
	s = m.Snapshot()
	if s.Screen.Current != "marcador" || len(s.Periods) != 1 {
		t.Fatalf("after taking it back: screen %q periods %+v", s.Screen.Current, s.Periods)
	}
	if len(saved) <= n || len(saved[len(saved)-1]) != 1 {
		t.Errorf("SavePeriods did not get the list without period 2: %+v", saved)
	}
	restored, _ := soccer()
	restored.RestorePeriods(saved[len(saved)-1])
	if len(restored.Snapshot().Periods) != 1 {
		t.Error("a restart brings the dropped period back")
	}
	m.ApplyConsole(sframe("2", "90:05", 2, 1, true))
	m.ApplyConsole(sframe("2", "90:05", 2, 1, true))
	if s := m.Snapshot(); s.Screen.Current != "final" || len(s.Periods) != 2 {
		t.Errorf("ending again: screen %q periods %+v", s.Screen.Current, s.Periods)
	}
}

// Going back a half after the end takes the final back too.
func TestSoccerPeriodBackAfterOver(t *testing.T) {
	m, _ := soccer()
	m.ApplyConsole(sframe("1", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "45:00", 1, 0, false))
	m.ApplyConsole(sframe("2", "90:00", 1, 0, true))
	m.ApplyConsole(sframe("1", "45:00", 1, 0, false))
	if s := m.Snapshot(); s.Screen.Current == "final" || len(s.Periods) != 1 || s.Periods[0].Period != 1 {
		t.Errorf("screen %q periods %+v", s.Screen.Current, s.Periods)
	}
}

// Over never touches the other sports: a basketball final stays recorded.
func TestOverIsIgnoredWithoutCards(t *testing.T) {
	m, c := newTest()
	m.SetGame(Game{Sport: "basketball", Rules: sport.Basketball}, true)
	run(t, m, c, []step{
		{0, "4", "0.0", 5, 4, "marcador", 0},
		{1, "4", "0.0", 5, 4, "marcador", 0},
		{60, "4", "0.0", 5, 4, "final", 1},
		{1, "4", "0.0", 5, 4, "final", 1},
	})
}

func cframe(period string, cards, subs string) map[string]string {
	f := sframe(period, "10:00", 0, 0, false)
	f["cards"], f["subs"] = cards, subs
	return f
}

const (
	c1 = `{"id":1,"team":"home","number":"4","kind":"yellow","minute":"23'"}`
	c2 = `{"id":2,"team":"home","number":"4","kind":"second_yellow","minute":"45+2'"}`
	c3 = `{"id":3,"team":"away","number":"9","kind":"red","minute":"60'"}`
	s1 = `{"id":4,"team":"home","out":"7","in":"14","minute":"60'"}`
)

func TestCardsAndSubsFromAFrame(t *testing.T) {
	m, _ := soccer()
	m.ApplyConsole(cframe("1", "["+c1+","+c3+"]", "["+s1+"]"))
	l := m.Snapshot().Live
	if len(l.Cards) != 2 || l.Cards[0] != (Card{1, "home", "4", "yellow", "23'"}) || len(l.Subs) != 1 || l.Subs[0].In != "14" {
		t.Errorf("live %+v", l)
	}
	m.ApplyConsole(cframe("1", "not json", "{"))
	if l := m.Snapshot().Live; len(l.Cards) != 0 || len(l.Subs) != 0 {
		t.Errorf("a bad list: %+v", l)
	}
	m.ApplyConsole(cframe("1", `[{"id":9,"team":"cat","number":"1","kind":"red","minute":"1'"}]`, "[]"))
	if l := m.Snapshot().Live; len(l.Cards) != 0 {
		t.Errorf("a card for another team: %+v", l.Cards)
	}
	m.ApplyConsole(sframe("1", "10:01", 0, 0, false))
	if l := m.Snapshot().Live; len(l.Cards) != 0 || l.Over {
		t.Errorf("a frame with none: %+v", l)
	}
}

func cardEvents(m *Machine) *[]string {
	var got []string
	m.OnEvent = func(e string, f map[string]any) {
		switch e {
		case "card":
			got = append(got, "card:"+f["team"].(string)+f["number"].(string)+f["kind"].(string)+f["minute"].(string))
		case "sub":
			got = append(got, "sub:"+f["out"].(string)+">"+f["in"].(string)+f["minute"].(string))
		}
	}
	return &got
}

func TestCardAndSubEvents(t *testing.T) {
	m, _ := soccer()
	got := cardEvents(m)
	m.ApplyConsole(cframe("1", "[]", "[]"))
	m.ApplyConsole(cframe("1", "["+c1+"]", "[]"))
	m.ApplyConsole(cframe("1", "["+c1+"]", "[]"))
	m.ApplyConsole(cframe("1", "["+c1+","+c2+"]", "["+s1+"]"))
	want := []string{"card:home4yellow23'", "card:home4second_yellow45+2'", "sub:7>1460'"}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("events %v, want %v", *got, want)
	}
	// An undo shrinks the list and tells nothing; the next card has a higher ID.
	m.ApplyConsole(cframe("1", "["+c1+"]", "[]"))
	m.ApplyConsole(cframe("1", "["+c1+","+c3+"]", "[]"))
	if len(*got) != 4 || (*got)[3] != "card:away9red60'" {
		t.Errorf("events %v", *got)
	}
	// An ID lower than one already told is never news again.
	m.ApplyConsole(cframe("1", "["+c1+","+c2+","+c3+"]", "["+s1+"]"))
	if len(*got) != 4 {
		t.Errorf("an ID told before came back as news: %v", *got)
	}
}

func TestCardEventsMutedInTheDemo(t *testing.T) {
	m, _ := soccer()
	got := cardEvents(m)
	m.mu.Lock()
	m.s.Demo = true
	m.mu.Unlock()
	m.update(Live{Period: "1", Clock: "05:00", Cards: []Card{{1, "home", "4", "red", "5'"}}}, true)
	if len(*got) != 0 {
		t.Errorf("the demo told %v", *got)
	}
}

// After a restart the scorer holds 2 cards and a sub, already told: Prime and
// the SetGame that runs before its first frame must not make them news.
func TestPrimeKeepsRestoredCardsQuiet(t *testing.T) {
	m, _ := newTest()
	got := cardEvents(m)
	m.Prime(LiveOf(cframe("1", "["+c1+","+c2+"]", "["+s1+"]")))
	m.SetGame(Game{Sport: "soccer", Rules: sport.Soccer}, true)
	m.ApplyConsole(cframe("1", "["+c1+","+c2+"]", "["+s1+"]"))
	if len(*got) != 0 {
		t.Fatalf("restored cards were told as new: %v", *got)
	}
	m.ApplyConsole(cframe("1", "["+c1+","+c2+","+c3+"]", "["+s1+"]"))
	if len(*got) != 1 {
		t.Errorf("events %v, want one for the card after the restart", *got)
	}
}

// A sport with FinalHold 0 never ends on a hold, even one whose clock sits at 0
// (the guard in checkFinal, apart from soccer's clock that never holds).
func TestNoHoldWhenFinalHoldIsZero(t *testing.T) {
	m, c := newTest()
	m.rulesFor = func(string) sport.Profile {
		return sport.Profile{ID: "x", Periods: 1, Length: 100, ClockDown: true, Scoring: "points", FinalHold: 0}
	}
	m.ApplyConsole(map[string]string{"period": "1", "clock": "0.5"})
	m.ApplyConsole(map[string]string{"period": "1", "clock": "0.0"})
	c.t = c.t.Add(10 * time.Minute)
	m.Tick()
	if s := m.Snapshot(); s.Screen.Current == "final" || len(s.Periods) != 0 {
		t.Errorf("screen %q periods %v: a hold ended a sport with no hold", s.Screen.Current, s.Periods)
	}
}

// A new game numbers from 1 again: once the last game's entries are gone from the
// live state, a card with ID 1 is news again.
func TestResetPeriodsNumbersCardsAndSubsFromOneAgain(t *testing.T) {
	m, _ := soccer()
	got := cardEvents(m)
	m.ApplyConsole(cframe("1", "["+c1+","+c2+","+c3+"]", "["+s1+"]"))
	if len(*got) != 4 {
		t.Fatalf("events %v", *got)
	}
	m.ResetPeriods()
	m.ApplyConsole(cframe("1", "[]", "[]"))
	one := `{"id":1,"team":"away","number":"3","kind":"red","minute":"2'"}`
	m.ApplyConsole(cframe("1", "["+one+"]", `[{"id":1,"team":"away","out":"1","in":"2","minute":"3'"}]`))
	if len(*got) != 6 || (*got)[4] != "card:away3red2'" || (*got)[5] != "sub:1>23'" {
		t.Errorf("events %v: the new game's first card and sub were not told", *got)
	}
	// Until the old entries are gone, an ID below the mark is not news.
	m.ApplyConsole(cframe("1", "["+one+"]", "[]"))
	if len(*got) != 6 {
		t.Errorf("events %v", *got)
	}
}

func TestLiveOfBoundsTheCardAndSubLists(t *testing.T) {
	var cards, subs []string
	for i := 1; i <= 150; i++ {
		cards = append(cards, `{"id":`+strconv.Itoa(i)+`,"team":"home","number":"4","kind":"yellow","minute":"1'"}`)
		subs = append(subs, `{"id":`+strconv.Itoa(i)+`,"team":"home","out":"1","in":"2","minute":"1'"}`)
	}
	l := LiveOf(map[string]string{"cards": "[" + strings.Join(cards, ",") + "]", "subs": "[" + strings.Join(subs, ",") + "]"})
	if len(l.Cards) != MaxLiveList || len(l.Subs) != MaxLiveList {
		t.Errorf("%d cards, %d subs, want %d", len(l.Cards), len(l.Subs), MaxLiveList)
	}
}

func TestDecodeListAndNews(t *testing.T) {
	team := func(x Card) string { return x.Team }
	got := decodeList(`[{"id":1,"team":"home"},{"id":2,"team":"cat"},{"id":3,"team":"away"}]`, team, 5)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("decodeList %+v", got)
	}
	if decodeList(`nope`, team, 5) != nil || len(decodeList(`[{"id":1,"team":"home"},{"id":2,"team":"home"}]`, team, 1)) != 1 {
		t.Error("decodeList does not read bad JSON as none or cap the list")
	}
	var k told
	live := []Card{{ID: 4}, {ID: 5}}
	if f := news(&k, live, cardID); len(f) != 2 || k.high != 5 {
		t.Errorf("first: %+v mark %d", f, k.high)
	}
	if f := news(&k, live[:1], cardID); len(f) != 0 || k.high != 5 {
		t.Errorf("after an undo: %+v mark %d", f, k.high)
	}
	if f := news(&k, []Card{{ID: 6}}, cardID); len(f) != 1 || k.high != 6 {
		t.Errorf("a new one: %+v mark %d", f, k.high)
	}
}
