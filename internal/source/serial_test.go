package source

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/state"
)

// dak is an All Sport 5000 data frame for offset.
func dak(offset int, data string) []byte {
	body := fmt.Sprintf("00000000\x01004210%04d\x02%s\x04", offset, data)
	var sum byte
	for i := 0; i < len(body); i++ {
		sum += body[i]
	}
	return []byte(fmt.Sprintf("\x16%s%02X\x17", body, sum))
}

// cableRig runs a Serial on pipes: each Open hands out a new pipe, whose
// write end lands in ports.
type cableRig struct {
	mu    sync.Mutex
	ports []*io.PipeWriter
	fails int // Opens to fail before one works
	vals  map[string]string
	on    bool
	opens int
	sends int // out calls
}

func (r *cableRig) open(port string, baud int) (io.ReadCloser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opens++
	if r.fails > 0 {
		r.fails--
		return nil, &OpenError{Key: i18n.Key("api.serial.ocupada"), Err: errors.New("busy")}
	}
	pr, pw := io.Pipe()
	r.ports = append(r.ports, pw)
	return pr, nil
}

func (r *cableRig) port() *io.PipeWriter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ports) == 0 {
		return nil
	}
	return r.ports[len(r.ports)-1]
}

func (r *cableRig) outs() int { r.mu.Lock(); defer r.mu.Unlock(); return r.sends }

func (r *cableRig) out(v map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sends++
	r.vals = map[string]string{}
	for k, x := range v {
		r.vals[k] = x
	}
}

func (r *cableRig) connected(on bool) { r.mu.Lock(); r.on = on; r.mu.Unlock() }

func (r *cableRig) get(k string) string { r.mu.Lock(); defer r.mu.Unlock(); return r.vals[k] }
func (r *cableRig) linked() bool        { r.mu.Lock(); defer r.mu.Unlock(); return r.on }

func newCable(r *cableRig, port, sport string) *Serial {
	return NewSerial(&Serial{
		Open:     r.open,
		Settings: func() (string, string) { return port, "daktronics" },
		Sport:    func() string { return sport },
		Retry:    10 * time.Millisecond,
	})
}

func TestSerialReadsTheConsole(t *testing.T) {
	r := &cableRig{}
	var raw [][]byte
	var rawMu sync.Mutex
	s := newCable(r, "COM3", "basketball")
	s.Raw = func(f []byte) { rawMu.Lock(); raw = append(raw, f); rawMu.Unlock() }
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port opened", func() bool { return r.port() != nil })
	if r.linked() {
		t.Error("linked before any frame")
	}
	half := dak(107, "  52  48")
	go func() { r.port().Write(half[:10]); r.port().Write(half[10:]); r.port().Write(dak(0, "12:34")) }()
	eventually(t, "the score", func() bool { return r.get("home") == "52" && r.get("clock") == "12:34" })
	if !r.linked() || !s.Status().Open {
		t.Errorf("linked %v, status %+v", r.linked(), s.Status())
	}
	if on, port := s.Linked(); !on || port != "COM3" {
		t.Errorf("Linked %v %q", on, port)
	}
	rawMu.Lock()
	if len(raw) != 2 {
		t.Errorf("raw.log got %d frames", len(raw))
	}
	rawMu.Unlock()
}

func TestSerialRetriesAFailedOpen(t *testing.T) {
	r := &cableRig{fails: 2}
	var logs []string
	var logMu sync.Mutex
	s := newCable(r, "COM3", "basketball")
	s.Log = func(m string) { logMu.Lock(); logs = append(logs, m); logMu.Unlock() }
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the third open", func() bool { st := s.Status(); return st.Open && st.Error == "" })
	eventually(t, "the log", func() bool { logMu.Lock(); defer logMu.Unlock(); return len(logs) == 2 })
	logMu.Lock()
	defer logMu.Unlock()
	// The same error twice is logged once, then the open.
	if !strings.Contains(logs[0], "busy") || logs[1] != i18n.T("log.serial.aberta", "porta", "COM3", "consola", "daktronics", "baud", 19200) {
		t.Errorf("logs %q", logs)
	}
}

func TestSerialStatusSaysWhy(t *testing.T) {
	r := &cableRig{fails: 1000}
	s := newCable(r, "COM3", "basketball")
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	eventually(t, "the error", func() bool { return s.Status().Error == i18n.T("api.serial.ocupada") })
	close(stop)

	s = newCable(r, "", "basketball")
	stop = make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "no port", func() bool { return s.Status().Error == i18n.T("api.serial.sem_porta") })
}

func TestSerialGoesQuiet(t *testing.T) {
	r := &cableRig{}
	s := newCable(r, "COM3", "basketball")
	s.Quiet = 50 * time.Millisecond
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port", func() bool { return r.port() != nil })
	go r.port().Write(dak(107, "   1"))
	eventually(t, "linked", r.linked)
	eventually(t, "quiet", func() bool { return !r.linked() })
}

func TestSerialReopensAfterTheCableGoes(t *testing.T) {
	r := &cableRig{}
	s := newCable(r, "COM3", "basketball")
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port", func() bool { return r.port() != nil })
	go r.port().Write(dak(107, "   7"))
	eventually(t, "linked", r.linked)
	first := r.port()
	first.CloseWithError(errors.New("unplugged"))
	eventually(t, "unlinked", func() bool { return !r.linked() })
	eventually(t, "a new port", func() bool { return r.port() != first })
	go r.port().Write(dak(111, "   3"))
	eventually(t, "the score kept", func() bool { return r.get("away") == "3" })
	if r.get("home") != "7" {
		t.Errorf("home %q: the buffer was lost on the reopen", r.get("home"))
	}
}

func TestSerialSkipsASportWithNoLayout(t *testing.T) {
	r := &cableRig{}
	var sport atomic.Value
	sport.Store("futsal")
	s := newCable(r, "COM3", "")
	s.Sport = func() string { return sport.Load().(string) }
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port", func() bool { return r.port() != nil })
	r.port().Write(dak(107, "   7"))
	eventually(t, "linked", r.linked)
	// A frame for a layout the console has, after it: once it arrives, the futsal one is long done.
	sport.Store("basketball")
	go r.port().Write(dak(141, " 1"))
	eventually(t, "the basketball frame", func() bool { return r.get("period") == "1" })
	if n := r.outs(); n != 1 {
		t.Errorf("%d sends; the futsal frame sent one", n)
	}
}

// The score starts afresh when the game's sport changes, so a volleyball game's
// sets don't show in the basketball one after it.
func TestSerialForgetsAnotherSportsKeys(t *testing.T) {
	r := &cableRig{}
	var sport atomic.Value
	sport.Store("volleyball")
	s := newCable(r, "COM3", "")
	s.Sport = func() string { return sport.Load().(string) }
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port", func() bool { return r.port() != nil })
	go r.port().Write(dak(214, " 2 1"))
	eventually(t, "the sets", func() bool { return r.get("home_sets") == "2" })
	sport.Store("basketball")
	go r.port().Write(dak(141, " 3"))
	eventually(t, "the period", func() bool { return r.get("period") == "3" })
	if r.get("home_sets") != "" {
		t.Errorf("home_sets %q in basketball", r.get("home_sets"))
	}
}

// Another port is another console: its score does not start from the last one's.
func TestSerialForgetsTheScoreOnAnotherPort(t *testing.T) {
	r := &cableRig{}
	var port atomic.Value
	port.Store("COM3")
	s := NewSerial(&Serial{
		Open:     r.open,
		Settings: func() (string, string) { return port.Load().(string), "daktronics" },
		Sport:    func() string { return "basketball" },
		Retry:    10 * time.Millisecond,
	})
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	eventually(t, "the port", func() bool { return r.port() != nil })
	go r.port().Write(dak(107, "  44"))
	eventually(t, "home", func() bool { return r.get("home") == "44" })
	first := r.port()
	port.Store("COM4")
	close(stop) // what Switch.Restart does: stop, then run again
	stop = make(chan struct{})
	defer close(stop)
	go s.Run(stop, r.out, r.connected)
	eventually(t, "the new port", func() bool { return r.port() != first && s.Status().Open })
	go r.port().Write(dak(111, "   2"))
	eventually(t, "away", func() bool { return r.get("away") == "2" })
	if r.get("home") != "0" {
		t.Errorf("home %q from the other port's console", r.get("home"))
	}
}

// A recording stops with the cable: the port failing ends it.
func TestSerialCaptureStopsWithTheCable(t *testing.T) {
	r := &cableRig{}
	s := newCable(r, "COM3", "basketball")
	s.Capture = &ble.Recorder{Dir: t.TempDir(), App: "test", Link: "serial"}
	stop := make(chan struct{})
	go s.Run(stop, r.out, r.connected)
	defer close(stop)
	eventually(t, "the port", func() bool { return r.port() != nil })
	go r.port().Write(dak(107, "   1"))
	eventually(t, "linked", r.linked)
	if _, err := s.Capture.Start("COM3", func(_ string, fn func()) { go fn() }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Capture.Stop("off") })
	r.port().CloseWithError(errors.New("unplugged"))
	eventually(t, "the recording stopped", func() bool { c := s.Capture.Status(); return !c.On && c.Stopped == "box" })
}

// The hand-built recordings in internal/cable/testdata play through the
// same read loop as a port, the way -replay does it.
func TestSerialReplaysRecordings(t *testing.T) {
	for file, want := range map[string]map[string]string{
		"daktronics-basketball.capture": {"home": "61", "away": "58", "clock": "0.4", "period": "4", "home_fouls": "5", "away_fouls": "3", "shot_clock": "", "sport": "basketball"},
		"bodet-basketball.capture":      {"home": "77", "away": "80", "clock": "5:12", "period": "5", "home_fouls": "2", "away_fouls": "4", "shot_clock": "14", "sport": "basketball"},
	} {
		t.Run(file, func(t *testing.T) {
			f, err := os.Open("../cable/testdata/" + file)
			if err != nil {
				t.Fatal(err)
			}
			head, frames, err := ble.ReadCaptureFile(f)
			f.Close()
			if err != nil || head["link"] != "serial" {
				t.Fatalf("head %v, err %v", head, err)
			}
			r := &cableRig{}
			s := newCable(r, "", "volleyball") // the recording's sport and protocol win, as main's replayCable sets them
			proto, recorded := head["protocol"], head["sport"]
			s.Settings = func() (string, string) { return "replay", proto }
			s.Sport = func() string { return recorded }
			pr, pw := io.Pipe()
			s.Open = func(string, int) (io.ReadCloser, error) { return pr, nil }
			stop := make(chan struct{})
			go s.Run(stop, r.out, r.connected)
			defer close(stop)
			ble.Play(frames, func(b []byte) { pw.Write(b) }, func(time.Duration) bool { return true })
			eventually(t, "the last frame", func() bool {
				for k, v := range want {
					if r.get(k) != v {
						return false
					}
				}
				return true
			})
			r.mu.Lock()
			defer r.mu.Unlock()
			for k, v := range want {
				if r.vals[k] != v {
					t.Errorf("%s = %q, want %q", k, r.vals[k], v)
				}
			}
			if !r.on {
				t.Error("a replay reads as linked")
			}
		})
	}
}

func TestSwitchRestartReopensTheCable(t *testing.T) {
	r := &cableRig{}
	port := "COM3"
	var mu sync.Mutex
	s := NewSerial(&Serial{
		Open:     r.open,
		Settings: func() (string, string) { mu.Lock(); defer mu.Unlock(); return port, "daktronics" },
		Sport:    func() string { return "basketball" },
		Retry:    10 * time.Millisecond,
	})
	w := &Switch{Sources: []Source{NewConsole(&Console{}), s}, State: state.New(time.Now)}
	w.Restart() // nothing running yet: no-op
	w.Use("serial")
	eventually(t, "the first open", func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.opens == 1 })
	mu.Lock()
	port = "COM4"
	mu.Unlock()
	w.Restart()
	eventually(t, "the reopen", func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.opens == 2 })
	eventually(t, "open on COM4", func() bool { return s.Status().Port == "COM4" && s.Status().Open })
}
