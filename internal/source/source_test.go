package source

import (
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/state"
)

// scoreFrame is a basketball frame from the box.
func scoreFrame(mm, ss string, home, away byte, period byte) []byte {
	f := make([]byte, 20)
	for i := range f {
		f[i] = ' '
	}
	f[0] = '3'
	copy(f[1:3], mm)
	copy(f[3:5], ss)
	f[5], f[6], f[7] = home, away, period
	f[9], f[10] = 0, 0
	return f
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("never: %s", what)
}

func TestSwitchSwapsAtRunTime(t *testing.T) {
	m := state.New(time.Now)
	var mu sync.Mutex
	files := map[string]string{} // what score/*.txt would get
	console := NewConsole(&Console{})
	manual := NewManual(&Manual{})
	w := &Switch{Sources: []Source{console, manual}, State: m, Values: func(v map[string]string) {
		mu.Lock()
		defer mu.Unlock()
		for k, x := range v {
			files[k] = x
		}
	}}
	live := func() state.Live { return m.Snapshot().Live }

	w.Use("console")
	console.Connected(true)
	eventually(t, "the box connects", func() bool { return m.Snapshot().Box.Connected })
	eventually(t, "the box frame shows", func() bool {
		console.Frame(scoreFrame("07", "45", 41, 38, '2'))
		return live().Home == 41
	})

	// The scorer takes over from the score on air.
	w.Use("manual")
	if s := m.Snapshot(); s.Input != "manual" || s.Box.Connected {
		t.Fatalf("after the swap: input %q, connected %t", s.Input, s.Box.Connected)
	}
	if err := manual.Do(Action{Do: "point", Team: "away", N: 3}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the scorer's basket shows", func() bool { l := live(); return l.Home == 41 && l.Away == 41 && l.Clock == "07:45" })
	if !m.Snapshot().Box.Connected {
		t.Error("a tap does not connect the scorer")
	}
	console.Frame(scoreFrame("07", "40", 50, 38, '2'))
	console.Connected(false) // the box goes; the scorer stays
	time.Sleep(50 * time.Millisecond)
	if l, s := live(), m.Snapshot(); l.Home != 41 || !s.Box.Connected {
		t.Errorf("the console still counts while manual runs: home %d, connected %t", l.Home, s.Box.Connected)
	}
	mu.Lock()
	if files["away"] != "41" {
		t.Errorf("score files have away %q", files["away"])
	}
	mu.Unlock()

	// Back to the box: it is not linked any more, and the next frame wins.
	w.Use("console")
	if s := m.Snapshot(); s.Input != "console" || s.Box.Connected {
		t.Fatalf("back on the console: input %q, connected %t", s.Input, s.Box.Connected)
	}
	manual.Do(Action{Do: "point", Team: "away", N: 3})
	// The run drops frames queued before it starts; the box sends them all the time.
	eventually(t, "the box frame shows again", func() bool {
		console.Frame(scoreFrame("07", "30", 52, 40, '2'))
		l := live()
		return l.Home == 52 && l.Away == 40
	})
	if !m.Snapshot().Box.Connected {
		t.Error("a box frame does not connect the console")
	}

	w.Use("serial") // unknown: the console
	if in := m.Snapshot().Input; in != "console" {
		t.Errorf("unknown input runs %q", in)
	}
}

func TestSwitchSkipsTheSeedInTheDemo(t *testing.T) {
	m := state.New(time.Now)
	manual := NewManual(&Manual{})
	w := &Switch{Sources: []Source{NewConsole(&Console{}), manual}, State: m}
	w.Use("console")
	m.SetDemo(true)
	defer m.SetDemo(false)
	eventually(t, "the demo plays", func() bool { return m.Snapshot().Live.Period != "" })
	w.Use("manual")
	if s := manual.Score(); s != newScoreAt(DefaultLength, DefaultLength) {
		t.Errorf("the scorer took the demo's score: %+v", s)
	}
}

func TestStartedManualKeepsItsOwnGame(t *testing.T) {
	m := state.New(time.Now)
	manual := NewManual(&Manual{})
	manual.Restore(Score{Home: 12, Period: 3, Clock: 3000, Length: DefaultLength})
	w := &Switch{Sources: []Source{NewConsole(&Console{}), manual}, State: m}
	w.Use("manual") // at start, with no source before it
	eventually(t, "the saved game shows", func() bool { return m.Snapshot().Live.Home == 12 })
	if p := m.Snapshot().Live.Period; p != "3" {
		t.Errorf("period %q", p)
	}
}
