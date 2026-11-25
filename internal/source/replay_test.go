package source

import (
	"maps"
	"os"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
)

func fixtureFrames(t *testing.T) []ble.Frame {
	t.Helper()
	f, err := os.Open("../ble/testdata/basketball-synthetic.capture")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	frames, err := ble.ReadCapture(f)
	if err != nil {
		t.Fatal(err)
	}
	return frames
}

// The console fed by a replay yields the values Decode gives for the same
// frames, one set per frame, in order.
func TestReplayMatchesDecode(t *testing.T) {
	frames := fixtureFrames(t)
	var want []map[string]string
	vals := map[string]string{}
	for _, f := range frames {
		ble.Decode(f.Data, vals)
		want = append(want, maps.Clone(vals))
	}

	c := NewConsole(&Console{})
	got := make(chan map[string]string, len(frames))
	stop := make(chan struct{})
	defer close(stop)
	go c.Run(stop, func(m map[string]string) { got <- maps.Clone(m) }, func(bool) {})
	for !c.running() {
		time.Sleep(time.Millisecond)
	}

	// Real pace is for people: here the waits are skipped, one frame at a time.
	next := make(chan struct{}, 1)
	go ble.Play(frames, c.Frame, func(time.Duration) bool { <-next; return true })
	for i := range frames {
		if i > 0 {
			next <- struct{}{}
		}
		select {
		case m := <-got:
			if !maps.Equal(m, want[i]) {
				t.Fatalf("frame %d: %v, want %v", i, m, want[i])
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("frame %d never reached the console", i)
		}
	}
}

func TestReplayLinksTheConsoleAndStops(t *testing.T) {
	c := NewConsole(&Console{})
	var on bool
	stop := make(chan struct{})
	go c.Run(stop, func(map[string]string) {}, func(b bool) { on = b })
	defer close(stop)
	for !c.running() {
		time.Sleep(time.Millisecond)
	}
	frames := []ble.Frame{{At: 0, Data: make([]byte, 20)}, {At: time.Hour, Data: make([]byte, 20)}}
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() { c.Replay(quit, frames); close(done) }()
	time.Sleep(50 * time.Millisecond)
	close(quit)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Replay ignored stop")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.linked || !on {
		t.Errorf("linked=%v on=%v, want both", c.linked, on)
	}
}
