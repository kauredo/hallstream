package state

import (
	"reflect"
	"testing"
)

func TestParseSets(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []SetScore
	}{
		{"", nil},
		{"nonsense", nil},
		{"25-", nil},
		{"-20,x-y", nil},
		{"25-20", []SetScore{{1, 25, 20, "home"}}},
		{"18-25", []SetScore{{1, 18, 25, "away"}}},
		{"25-20h,18-25a", []SetScore{{1, 25, 20, "home"}, {2, 18, 25, "away"}}},
		{"0-0a", []SetScore{{1, 0, 0, "away"}}},
		{"0-0", []SetScore{{1, 0, 0, ""}}}, // a tie: setSets takes the winner from the counts
		{"25-20,junk,18-25", []SetScore{{1, 25, 20, "home"}, {2, 18, 25, "away"}}},
	} {
		if got := parseSets(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseSets(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// The scorer says who won a set it has no points for; the counts decide when
// it does not.
func TestSeededSetsKeepTheirWinner(t *testing.T) {
	m, _ := volleyball()
	vframe(m, 0, 0, 1, 1, "0-0h,0-0a")
	want := []SetScore{{1, 0, 0, "home"}, {2, 0, 0, "away"}}
	if got := m.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Errorf("sets %v, want %v", got, want)
	}
	m, _ = volleyball()
	vframe(m, 0, 0, 0, 2, "0-0,0-0") // no letters: the counts say away twice
	want = []SetScore{{1, 0, 0, "away"}, {2, 0, 0, "away"}}
	if got := m.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Errorf("sets %v, want %v", got, want)
	}
}

// A console frame that carries no set counts (the console has not sent them,
// or the decoder has no volleyball frame yet) must not wipe the sets already
// recorded, nor end the match.
func TestFrameWithoutSetCountsLeavesTheSets(t *testing.T) {
	m, _ := volleyball()
	vframe(m, 0, 0, 1, 0, "25-20")
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	m.ApplyConsole(map[string]string{"home": "7", "away": "4", "period": "4"})
	if got := m.Snapshot(); len(got.Sets) != 3 || got.Screen.Current != "final" {
		t.Errorf("a frame with no counts left %d sets on screen %q", len(got.Sets), got.Screen.Current)
	}
	vframe(m, 7, 4, 1, 0, "25-20") // a frame with counts that fell is an undo
	if got := m.Snapshot().Sets; len(got) != 1 {
		t.Errorf("a falling count left %v", got)
	}
}

func TestSetCountsAreClamped(t *testing.T) {
	m, _ := volleyball()
	m.ApplyConsole(map[string]string{"home_sets": "99", "away_sets": "-4", "period": "1"})
	st := m.Snapshot()
	if len(st.Sets) > 5 || st.Live.HomeSets != 5 || st.Live.AwaySets != 0 {
		t.Errorf("sets %d, live %d-%d", len(st.Sets), st.Live.HomeSets, st.Live.AwaySets)
	}
}

func TestDemoGivesTheSetsBack(t *testing.T) {
	m, _ := volleyball()
	vframe(m, 0, 0, 1, 0, "25-20")
	m.SetDemo(true)
	m.SetDemo(false)
	if got := m.Snapshot().Sets; len(got) != 1 || got[0].Home != 25 {
		t.Errorf("after the demo: %v", got)
	}
}

// A new game clears the sets with the periods and lets the match end again.
func TestResetPeriodsClearsSets(t *testing.T) {
	m, events := volleyball()
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	m.ResetPeriods()
	if got := m.Snapshot().Sets; len(got) != 0 {
		t.Errorf("after the reset: %v", got)
	}
	if err := m.SetScreen("marcador", true); err != nil {
		t.Fatal(err)
	}
	vframe(m, 0, 0, 3, 0, "25-20,25-18,25-22")
	finals := 0
	for _, e := range *events {
		if e == "screen:final" {
			finals++
		}
	}
	if finals != 2 {
		t.Errorf("final fired %d times, want once per match: %v", finals, *events)
	}
}
