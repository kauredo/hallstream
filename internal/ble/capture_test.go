package ble

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fixture = "testdata/basketball-synthetic.capture"

func loadFixture(t *testing.T) []Frame {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	frames, err := ReadCapture(f)
	if err != nil {
		t.Fatal(err)
	}
	return frames
}

// decoded lists, per frame, the fields the frame changed.
func decoded(frames []Frame) string {
	var b strings.Builder
	state := map[string]string{}
	for i, f := range frames {
		before := map[string]string{}
		for k, v := range state {
			before[k] = v
		}
		Decode(f.Data, state)
		var changes []string
		for k, v := range state {
			if before[k] != v {
				changes = append(changes, fmt.Sprintf("%s=%q", k, v))
			}
		}
		sort.Strings(changes)
		fmt.Fprintf(&b, "%d %dms %s\n", i, f.At.Milliseconds(), strings.Join(changes, " "))
	}
	return b.String()
}

func TestReplayFixtureGolden(t *testing.T) {
	got := decoded(loadFixture(t))
	golden := "testdata/basketball-synthetic.golden"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("decoded sequence changed (go test -update to accept):\n%s\nwant:\n%s", got, want)
	}
}

func TestReadCaptureRejects(t *testing.T) {
	head := CaptureMagic + " app=\"x\"\n"
	for name, in := range map[string]string{
		"no header":       "0 3300\n",
		"no space":        head + "12\n",
		"ms not a number": head + "soon 3300\n",
		"negative ms":     head + "-5 3300\n",
		"not hex":         head + "10 zz\n",
		"empty frame":     head + "10 \n",
	} {
		if _, err := ReadCapture(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	frames, err := ReadCapture(strings.NewReader(head + "# note\n\n0 3300\n7 41\n"))
	if err != nil || len(frames) != 2 || frames[1].At != 7*time.Millisecond || frames[1].Data[0] != 'A' {
		t.Errorf("good file: %v %v", frames, err)
	}
}

func TestPlayKeepsThePace(t *testing.T) {
	frames := loadFixture(t)
	var gaps []time.Duration
	var got [][]byte
	Play(frames, func(b []byte) { got = append(got, b) }, func(d time.Duration) bool { gaps = append(gaps, d); return true })
	if len(got) != len(frames) {
		t.Fatalf("emitted %d of %d", len(got), len(frames))
	}
	var total time.Duration
	for _, g := range gaps {
		total += g
	}
	if total != frames[len(frames)-1].At {
		t.Errorf("waited %v, want %v", total, frames[len(frames)-1].At)
	}
	n := 0
	Play(frames, func([]byte) { n++ }, func(time.Duration) bool { return n < 3 })
	if n != 3 {
		t.Errorf("a stopped replay emitted %d frames, want 3", n)
	}
}

func inline(_ string, fn func()) { go fn() }

func newRecorder(t *testing.T) *Recorder {
	return &Recorder{Dir: filepath.Join(t.TempDir(), "captura"), App: "1.2.3", Sport: func() string { return "futsal" }}
}

func TestRecordOnOff(t *testing.T) {
	r := newRecorder(t)
	r.Offer([]byte("lost")) // nothing recording
	st, err := r.Start("Livestream Stramatel", inline)
	if err != nil || !st.On {
		t.Fatalf("start: %+v %v", st, err)
	}
	frames := loadFixture(t)
	for _, f := range frames {
		r.Offer(f.Data)
	}
	st = r.Stop("off")
	if st.On || st.Stopped != "off" || st.Frames != int64(len(frames)) || st.Dropped != 0 {
		t.Fatalf("stop: %+v", st)
	}
	path := filepath.Join(r.Dir, st.File)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(b)) != st.Bytes {
		t.Errorf("file is %d bytes, status says %d", len(b), st.Bytes)
	}
	head, _, _ := strings.Cut(string(b), "\n")
	if !strings.HasPrefix(head, CaptureMagic+` app="1.2.3" box="Livestream Stramatel" sport="futsal" start="`) || !strings.HasSuffix(head, `Z"`) {
		t.Errorf("header = %q", head)
	}
	got, err := ReadCapture(bytes.NewReader(b))
	if err != nil || len(got) != len(frames) {
		t.Fatalf("read back: %d frames, %v", len(got), err)
	}
	for i := range got {
		if !bytes.Equal(got[i].Data, frames[i].Data) {
			t.Errorf("frame %d differs", i)
		}
	}
	r.Offer([]byte("late")) // after stop: ignored
	if r.Status().Frames != st.Frames {
		t.Error("a frame after the stop was counted")
	}
	// A second recording gets its own file.
	st2, err := r.Start("box", inline)
	if err != nil {
		t.Fatal(err)
	}
	r.Stop("box")
	if st2.File == st.File && st2.File != "" {
		t.Errorf("both recordings are %s", st2.File)
	}
}

func TestRecordStopsAtTheCap(t *testing.T) {
	r := newRecorder(t)
	r.Max = 300
	if _, err := r.Start("box", inline); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		r.Offer(make([]byte, 20))
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.Status().On && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	st := r.Status()
	if st.On || st.Stopped != "cap" {
		t.Fatalf("status = %+v", st)
	}
	info, err := os.Stat(filepath.Join(r.Dir, st.File))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 300 || info.Size() != st.Bytes || st.Frames == 0 || st.Frames >= 100 {
		t.Errorf("file %d bytes, status %+v", info.Size(), st)
	}
}

// gate lets the header through and then blocks every write until released.
type gate struct {
	mu      sync.Mutex
	writes  int
	release chan struct{}
}

func (g *gate) Write(p []byte) (int, error) {
	g.mu.Lock()
	g.writes++
	first := g.writes == 1
	g.mu.Unlock()
	if !first {
		<-g.release
	}
	return len(p), nil
}
func (g *gate) Close() error { return nil }

func TestSlowWriterNeverBlocksTheReader(t *testing.T) {
	g := &gate{release: make(chan struct{})}
	r := &Recorder{Slots: 8, Create: func(string) (io.WriteCloser, error) { return g, nil }}
	if _, err := r.Start("box", inline); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			r.Offer(make([]byte, 20))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Offer blocked on a stuck writer")
	}
	st := r.Status()
	if st.Dropped < 900 {
		t.Errorf("dropped %d of 1000 with 8 slots", st.Dropped)
	}
	close(g.release)
	st = r.Stop("off")
	if st.Frames+st.Dropped != 1000 {
		t.Errorf("frames %d + dropped %d != 1000", st.Frames, st.Dropped)
	}
}

func TestStopDoesNotHangOnAStuckDisk(t *testing.T) {
	g := &gate{release: make(chan struct{})}
	defer close(g.release)
	r := &Recorder{Create: func(string) (io.WriteCloser, error) { return g, nil }}
	r.Start("box", inline)
	r.Offer(make([]byte, 20))
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	r.Stop("off")
	if time.Since(start) > 4*time.Second {
		t.Error("Stop waited for the stuck writer")
	}
}

func TestReaderOffersRawFrames(t *testing.T) {
	r := newRecorder(t)
	var seen int
	rd := New(&Reader{Capture: r, Frame: func([]byte) { seen++ }})
	rd.onFrame(make([]byte, 20)) // not recording yet
	if _, err := r.Start("box", inline); err != nil {
		t.Fatal(err)
	}
	rd.onFrame(make([]byte, 20))
	if st := r.Stop("off"); st.Frames != 1 || seen != 2 {
		t.Errorf("recorded %d, reader saw %d", st.Frames, seen)
	}
	New(&Reader{}).onFrame(make([]byte, 20)) // no recorder at all
}
