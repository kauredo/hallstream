package state

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/sport"
)

// playOne plays one game of a sport's script and returns every frame.
func playOne(t *testing.T, id string) []Live {
	t.Helper()
	var frames []Live
	p := sport.Get(id)
	show := func(l Live) { frames = append(frames, l) }
	hold := func(int) bool { return true }
	ok := false
	if p.Sets() {
		ok = playSets(p, show, hold)
	} else {
		ok = playTimed(p, show, hold)
	}
	if !ok || len(frames) == 0 {
		t.Fatalf("%s: the game did not finish", id)
	}
	for i, l := range frames {
		if l.Sport != id {
			t.Fatalf("%s: frame %d carries sport %q", id, i, l.Sport)
		}
	}
	return frames
}

func TestDemoFutsal(t *testing.T) {
	frames := playOne(t, "futsal")
	hot, reset := 0, false
	for _, l := range frames {
		if l.Sport != "futsal" || l.Period == "3" {
			t.Fatalf("frame %+v is not a two-half futsal frame", l)
		}
		if l.Period == "1" && l.AwayFouls >= 6 {
			hot++
		}
		if l.Period == "2" && l.HomeFouls == 0 && l.AwayFouls == 0 {
			reset = true
		}
	}
	if first := frames[0]; first.Clock != "20:00" || first.Period != "1" {
		t.Errorf("first frame %q in %s, want 20:00 in 1", first.Clock, first.Period)
	}
	if hot == 0 || !reset {
		t.Errorf("fouls past 5 shown %d times, reset at half %v", hot, reset)
	}
	if last := frames[len(frames)-1]; last.Clock != "0.0" || last.Period != "2" || last.Home <= last.Away {
		t.Errorf("last frame %+v", last)
	}
}

func TestDemoHandball(t *testing.T) {
	frames := playOne(t, "handball")
	most, both := 0, false
	for _, l := range frames {
		most = max(most, len(l.Suspensions))
		both = both || len(l.Suspensions) == 2
		if l.PeriodEnd == 0 || l.PeriodEnd != 18000*periodOf(l) {
			t.Fatalf("frame %+v has no half end", l)
		}
	}
	if most == 0 || !both {
		t.Errorf("frames carry at most %d suspensions, want two at once", most)
	}
	if last := frames[len(frames)-1]; last.Clock != "60:00" || len(last.Suspensions) != 0 {
		t.Errorf("last frame %+v", last)
	}
}

// periodOf is the half number of a frame, for the tests.
func periodOf(l Live) int { n, _ := strconv.Atoi(l.Period); return n }

func TestDemoSoccer(t *testing.T) {
	frames := playOne(t, "soccer")
	last := frames[len(frames)-1]
	if !last.Over || last.Stoppage != 3 {
		t.Errorf("last frame over %v, stoppage %d", last.Over, last.Stoppage)
	}
	kinds := map[string]bool{}
	for _, c := range last.Cards {
		kinds[c.Kind] = true
	}
	if !kinds["yellow"] || !kinds["red"] || len(last.Subs) != 2 {
		t.Errorf("cards %v, subs %v", last.Cards, last.Subs)
	}
	run := false
	for _, l := range frames {
		run = run || (l.Period == "1" && Tenths(l.Clock) > 27000)
	}
	if !run {
		t.Error("the first half never ran on past 45:00")
	}
}

func TestDemoVolleyball(t *testing.T) {
	frames := playOne(t, "volleyball")
	last := frames[len(frames)-1]
	if last.HomeSets != 3 || last.AwaySets != 1 || last.Sets != "25-21h,19-25a,25-22h,25-23h" {
		t.Errorf("match ended %d-%d sets %q", last.HomeSets, last.AwaySets, last.Sets)
	}
	served := false
	for _, l := range frames {
		served = served || l.Serve == "away"
		if l.Home > 25 || l.Away > 25 {
			t.Fatalf("a set past 25: %+v", l)
		}
	}
	if !served {
		t.Error("the serve never changed side")
	}
}

// Fed to a machine as a demo, each script ends the game the way its sport does.
func TestDemoEndsEachSport(t *testing.T) {
	for _, id := range []string{"futsal", "handball", "soccer", "volleyball"} {
		m, c := newTest()
		m.s.Demo, m.s.Game.Sport = true, id
		p := sport.Get(id)
		show := func(l Live) { m.update(l, true); c.t = c.t.Add(100 * time.Millisecond) }
		hold := func(n int) bool { c.t = c.t.Add(time.Duration(n) * 100 * time.Millisecond); m.Tick(); return true }
		if p.Sets() {
			playSets(p, show, hold)
		} else {
			playTimed(p, show, hold)
		}
		s := m.Snapshot()
		if s.Screen.Current != "final" {
			t.Errorf("%s: screen %q after the game, want final", id, s.Screen.Current)
		}
	}
}

// soccerFullTime is where the demo's last frame sits: 90:00 plus 3 minutes added.
const soccerFullTime = 93 * 600

func TestDemoSoccerLastFrame(t *testing.T) {
	frames := playOne(t, "soccer")
	last := frames[len(frames)-1]
	if last.Period != "2" || Tenths(last.Clock) < soccerFullTime {
		t.Errorf("last frame period %s at %s, want 2 at 93:00 or later", last.Period, last.Clock)
	}
}

func TestDemoVolleyballPointsResetEachSet(t *testing.T) {
	frames := playOne(t, "volleyball")
	sets := 1
	for i := 1; i < len(frames); i++ {
		if frames[i].Period != frames[i-1].Period {
			sets++
			if frames[i].Home != 0 || frames[i].Away != 0 {
				t.Fatalf("set %s starts at %d-%d", frames[i].Period, frames[i].Home, frames[i].Away)
			}
		}
	}
	if sets != len(demoSets) {
		t.Errorf("%d sets played, want %d", sets, len(demoSets))
	}
}

func TestRallies(t *testing.T) {
	for _, set := range demoSets {
		got := rallies(set[0], set[1])
		home := 0
		for _, side := range got {
			if side == "home" {
				home++
			}
		}
		winner := "home"
		if set[1] > set[0] {
			winner = "away"
		}
		if len(got) != set[0]+set[1] || home != set[0] || got[len(got)-1] != winner {
			t.Errorf("rallies(%d, %d): %d points, home %d, last %s", set[0], set[1], len(got), home, got[len(got)-1])
		}
	}
}

func TestDemoClubEndsAhead(t *testing.T) {
	for _, id := range []string{"futsal", "handball", "soccer"} {
		frames := playOne(t, id)
		if last := frames[len(frames)-1]; last.Home <= last.Away {
			t.Errorf("%s ended %d-%d", id, last.Home, last.Away)
		}
	}
}

// demoMachine is a machine in demo mode for sport id whose loop the test
// drives itself, with a clock that moves only when the demo waits.
func demoMachine(id string) (*Machine, *fakeClock, func(int) bool) {
	m, c := newTest()
	m.s.Game.Sport = id
	m.setDemo(true, func(<-chan struct{}) {})
	hold := func(n int) bool {
		c.t = c.t.Add(time.Duration(n) * 100 * time.Millisecond)
		m.Tick()
		return true
	}
	return m, c, hold
}

func play(p sport.Profile, show func(Live), hold func(int) bool) bool {
	if p.Sets() {
		return playSets(p, show, hold)
	}
	return playTimed(p, show, hold)
}

// A second demo game starts on the scoreboard, not on the last game's final.
func TestDemoSecondGameStartsOnScoreboard(t *testing.T) {
	for _, id := range []string{"futsal", "handball", "soccer", "volleyball"} {
		m, c, _ := demoMachine(id)
		seenFinal, calls, back := false, 0, ""
		m.demoLoop(m.demoStop, func(n int) bool {
			c.t = c.t.Add(time.Duration(n) * 100 * time.Millisecond)
			m.Tick()
			calls++
			s := m.Snapshot()
			switch {
			case s.Screen.Current == "final":
				seenFinal = true
			case seenFinal && s.Live.Period == "1" && s.Live.Home == 0 && s.Live.Away == 0:
				back = s.Screen.Current
				m.SetDemo(false)
				return false
			}
			if calls > 20000 {
				t.Errorf("%s: no second game", id)
				m.SetDemo(false)
				return false
			}
			return true
		})
		if !seenFinal || back != "marcador" {
			t.Errorf("%s: final seen %v, second game on %q", id, seenFinal, back)
		}
	}
}

// The demo raises the timeline's marks; the real game's first suspension,
// card and substitution still get told after it.
func TestDemoLeavesTheTimelineMarks(t *testing.T) {
	for _, id := range []string{"handball", "soccer"} {
		m, _, hold := demoMachine(id)
		var told []string
		m.OnEvent = func(ev string, _ map[string]any) { told = append(told, ev) }
		if !play(sport.Get(id), func(l Live) { m.update(l, true) }, hold) {
			t.Fatalf("%s: the game did not finish", id)
		}
		m.SetDemo(false)
		m.s.Game.Sport = id
		m.ApplyConsole(map[string]string{
			"period": "1", "clock": "05:00", "home": "0", "away": "0",
			"suspensions": `[{"team":"home","id":1,"number":"7","left":"01:00"}]`,
			"cards":       `[{"id":1,"team":"home","number":"5","kind":"yellow","minute":"5'"}]`,
			"subs":        `[{"id":1,"team":"home","out":"10","in":"14","minute":"5'"}]`,
		})
		want := map[string]bool{"suspension": true, "card": true, "sub": true}
		for ev, w := range want {
			got := false
			for _, e := range told {
				got = got || e == ev
			}
			if got != w {
				t.Errorf("%s: event %q told %v, want %v (%v)", id, ev, got, w, told)
			}
		}
	}
}

// A demo stopped during its final hold leaves no final behind: the real game
// that follows reaches the end of period 2 and fires its own.
func TestDemoStoppedInFinalHold(t *testing.T) {
	m, c, hold := demoMachine("futsal")
	stopped := func(n int) bool {
		if n == demoFinalHold {
			hold(n)
			return false
		}
		return hold(n)
	}
	if playTimed(sport.Futsal, func(l Live) { m.update(l, true) }, stopped) {
		t.Fatal("the game ran on past a stop")
	}
	if m.Snapshot().Screen.Current != "final" {
		t.Fatal("the demo had not fired its final")
	}
	m.SetDemo(false)
	m.SetScreen("marcador", true)
	for _, f := range [][2]string{{"2", "00:05"}, {"2", "0.0"}} {
		c.t = c.t.Add(time.Second)
		m.ApplyConsole(map[string]string{"period": f[0], "clock": f[1], "home": "3", "away": "2"})
	}
	c.t = c.t.Add(time.Minute + time.Second)
	m.Tick()
	if s := m.Snapshot(); s.Screen.Current != "final" || len(s.Periods) != 1 {
		t.Errorf("real game: screen %q, %d periods", s.Screen.Current, len(s.Periods))
	}
}

// A profile the script does not fit plays nothing from playSport, and the
// loop falls back to the basketball demo.
func TestDemoFallsBackWithoutAScript(t *testing.T) {
	odd := sport.Futsal
	odd.ID = "curling"
	three := sport.Futsal
	three.Periods = 3
	for _, p := range []sport.Profile{odd, three} {
		if scriptable(p) {
			t.Errorf("%s with %d periods is scriptable", p.ID, p.Periods)
		}
		playSport(p, func(Live) { t.Error("a frame for a profile with no script") }, func(int) bool { return true }, func() {})
	}
	m, _, _ := demoMachine("curling")
	m.rulesFor = func(string) sport.Profile { return odd }
	calls, got := 0, ""
	m.demoLoop(m.demoStop, func(int) bool {
		if calls++; calls == 3 {
			got = m.Snapshot().Live.Sport
			m.SetDemo(false)
			return false
		}
		return true
	})
	if got != "basketball" {
		t.Errorf("fallback played sport %q", got)
	}
}

// The loop plays the game's sport, follows a change of it, and stops with the demo.
func TestDemoLoopFollowsTheSport(t *testing.T) {
	m, _, _ := demoMachine("futsal")
	calls, futsal, volley, sets := 0, false, false, false
	m.demoLoop(m.demoStop, func(int) bool {
		calls++
		l := m.Snapshot().Live
		switch {
		case l.Sport == "futsal" && !futsal:
			futsal = true
			m.mu.Lock()
			m.s.Game.Sport = "volleyball"
			m.mu.Unlock()
		case l.Sport == "volleyball":
			volley, sets = true, sets || l.Sets != ""
		}
		if sets || calls > 5000 {
			m.SetDemo(false)
			return false
		}
		return true
	})
	if !futsal || !volley || !sets {
		t.Errorf("futsal %v, volleyball %v, with sets %v after %d waits", futsal, volley, sets, calls)
	}
	if m.Snapshot().Demo {
		t.Error("the demo is still on")
	}
}

// Nothing of a demo game reaches the timeline, the saves or the history, and
// the real game's periods, sets and live lists come back when it ends.
func TestDemoIsMutedAndRestored(t *testing.T) {
	for _, id := range []string{"futsal", "handball", "soccer", "volleyball"} {
		m, _, hold := demoMachine(id)
		m.SetDemo(false) // start from the real game, then give it something to restore
		periods := []Period{{1, 10, 8, 10, 8}, {2, 20, 15, 10, 7}}
		sets := []SetScore{{1, 25, 20, "home"}}
		m.RestorePeriods(periods)
		m.RestoreSets(sets)
		events, saves, history := 0, 0, 0
		m.OnEvent = func(string, map[string]any) { events++ }
		m.SavePeriods = func([]Period) { saves++ }
		m.SaveSets = func([]SetScore) { saves++ }
		m.OnPeriods = func(s State) {
			if !s.Demo {
				history++
			}
		}
		m.SetDemo(true)
		if !play(sport.Get(id), func(l Live) { m.update(l, true) }, hold) {
			t.Fatalf("%s: the game did not finish", id)
		}
		m.SetDemo(false)
		s := m.Snapshot()
		if events != 0 || saves != 0 || history != 0 {
			t.Errorf("%s: %d timeline events, %d saves, %d history saves from the demo", id, events, saves, history)
		}
		if !reflect.DeepEqual(s.Periods, periods) || !reflect.DeepEqual(s.Sets, sets) {
			t.Errorf("%s: periods %v, sets %v after the demo", id, s.Periods, s.Sets)
		}
		if l := s.Live; len(l.Suspensions) != 0 || len(l.Cards) != 0 || len(l.Subs) != 0 || l.Stoppage != 0 {
			t.Errorf("%s: live lists left behind: %+v", id, l)
		}
	}
}
