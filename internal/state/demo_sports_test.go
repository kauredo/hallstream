package state

import (
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
