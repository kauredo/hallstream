package source

import (
	"errors"
	"io"
	"maps"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/cable"
	"github.com/kauredo/hallstream/internal/i18n"
)

// OpenError is an Open that failed in a way the panel can name: Key is what
// it shows, Err the system's own words, which go to the log only.
type OpenError struct {
	Key i18n.Key
	Err error
}

func (e *OpenError) Error() string { return e.Err.Error() }
func (e *OpenError) Unwrap() error { return e.Err }

// SerialStatus is what GET /api/serial says about the cable.
type SerialStatus struct {
	Linked   bool   `json:"linked"` // good frames are coming
	Open     bool   `json:"open"`   // the port is open
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Error    string `json:"error"` // why it is not open, in words; "" when fine
}

// Serial is a console on a serial cable: it opens the port while it runs,
// cuts what comes into frames and decodes them with the settings' protocol.
// The decoded values outlive a run, since a Daktronics console sends only
// what changed.
type Serial struct {
	// Open opens a port at a baud rate, 8N1; an *OpenError names the cause.
	Open     func(port string, baud int) (io.ReadCloser, error)
	Settings func() (port, protocol string)
	Sport    func() string // the game's sport, which picks the layout
	Raw      func([]byte)  // every good frame, for raw.log
	Capture  *ble.Recorder // Gravar a consola, for the cable
	Log      func(string)
	Now      func() time.Time
	Retry    time.Duration // between opens; 0 is 3 s
	Quiet    time.Duration // with no byte at all, the console is gone; 0 is 30 s

	run sync.Mutex // one Run at a time, so a restart waits for the port to close

	mu       sync.Mutex
	proto    cable.Protocol
	vals     map[string]string
	open     bool
	linked   bool
	port     string
	errKey   i18n.Key // why it is not open; "" when it is
	lastErr  string   // last error logged
	noLayout string   // last sport logged as having no layout
	sport    string   // the sport vals were decoded for
}

// NewSerial fills in the defaults.
func NewSerial(s *Serial) *Serial {
	if s.Settings == nil {
		s.Settings = func() (string, string) { return "", "" }
	}
	if s.Raw == nil {
		s.Raw = func([]byte) {}
	}
	if s.Log == nil {
		s.Log = func(string) {}
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.Retry == 0 {
		s.Retry = 3 * time.Second
	}
	if s.Quiet == 0 {
		s.Quiet = 30 * time.Second
	}
	if s.Sport == nil {
		s.Sport = func() string { return "" }
	}
	s.vals = map[string]string{}
	return s
}

func (s *Serial) Name() string { return "serial" }

// Status is the cable's state for the panel.
func (s *Serial) Status() SerialStatus {
	port, proto := s.Settings()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SerialStatus{Linked: s.linked, Open: s.open, Port: port, Protocol: cable.Known(proto)}
	if s.errKey != "" {
		st.Error = s.errKey.Error()
	}
	return st
}

// Linked is whether frames are coming, and the port's name.
func (s *Serial) Linked() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linked, s.port
}

func (s *Serial) Run(stop <-chan struct{}, out func(map[string]string), connected func(bool)) {
	s.run.Lock()
	defer s.run.Unlock()
	select {
	case <-stop: // a restart came while this run waited for the last one to close the port
		return
	default:
	}
	for {
		if s.once(stop, out, connected) {
			return
		}
		t := time.NewTimer(s.Retry)
		select {
		case <-stop:
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// once opens the port and reads it until it fails or stop closes; it
// reports whether stop closed.
func (s *Serial) once(stop <-chan struct{}, out func(map[string]string), connected func(bool)) bool {
	port, name := s.Settings()
	s.mu.Lock()
	// Another console, or the same type on another port: start from a blank score.
	if s.proto == nil || s.proto.Name() != cable.Known(name) || (s.port != "" && s.port != port) {
		s.proto, s.vals = cable.Get(name), map[string]string{}
	}
	proto := s.proto
	s.mu.Unlock()
	connected(false)
	if port == "" {
		s.fail(i18n.Key("api.serial.sem_porta"), nil, port)
		return false
	}
	rc, err := s.Open(port, proto.Baud())
	if err != nil {
		key := i18n.Key("api.serial.nao_abri")
		var oe *OpenError
		if errors.As(err, &oe) {
			key = oe.Key
		}
		s.fail(key, err, port)
		return false
	}
	s.mu.Lock()
	s.open, s.port, s.errKey, s.lastErr = true, port, "", ""
	s.mu.Unlock()
	s.Log(i18n.T("log.serial.aberta", "porta", port, "consola", proto.Name(), "baud", proto.Baud()))
	stopped := func() bool {
		defer rc.Close() // a panic in a decoder must not leave the port held
		return s.read(stop, rc, proto, out, connected)
	}()
	s.mu.Lock()
	s.open = false
	s.mu.Unlock()
	s.setLinked(false, connected)
	s.Capture.StopNoWait("box")
	return stopped
}

// fail records why the port is not open, and logs it when the reason is new.
func (s *Serial) fail(key i18n.Key, err error, port string) {
	text := key.Error()
	if err != nil {
		text = err.Error()
	}
	s.mu.Lock()
	s.errKey = key
	logIt := text != s.lastErr
	s.lastErr = text
	s.mu.Unlock()
	switch {
	case !logIt:
	case err != nil:
		s.Log(i18n.T("log.serial.nao_abri", "porta", port, "err", err))
	default:
		s.Log(text)
	}
}

func (s *Serial) setLinked(on bool, connected func(bool)) {
	s.mu.Lock()
	changed := s.linked != on
	s.linked = on
	s.mu.Unlock()
	if changed {
		connected(on)
	}
}

// read hands what the port sends to the protocol until the port fails or
// stop closes; it reports whether stop closed.
func (s *Serial) read(stop <-chan struct{}, rc io.Reader, proto cable.Protocol, out func(map[string]string), connected func(bool)) bool {
	chunks, failed, done := make(chan []byte, 16), make(chan error, 1), make(chan struct{})
	defer close(done)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				failed <- io.ErrUnexpectedEOF
			}
		}()
		b := make([]byte, 512)
		for {
			n, err := rc.Read(b)
			if n > 0 {
				select {
				case chunks <- append([]byte(nil), b[:n]...):
				case <-done:
					return
				}
			}
			if err != nil {
				failed <- err
				return
			}
		}
	}()
	// A Daktronics console sends idle bytes between frames; nothing says a
	// Scorepad does, so a stopped clock on one must not read as a lost cable.
	quiet := s.Quiet
	if proto.Name() == "bodet" {
		quiet *= 10
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	last := s.Now()
	var buf []byte
	take := func(b []byte) {
		last = s.Now()
		var frames [][]byte
		frames, buf = proto.Split(append(buf, b...))
		for _, f := range frames {
			s.frame(f, proto, out, connected)
		}
	}
	for {
		select {
		case <-stop:
			return true
		case err := <-failed:
			for drained := false; !drained; { // the frames that came before the error
				select {
				case b := <-chunks:
					take(b)
				default:
					drained = true
				}
			}
			s.Log(i18n.T("log.serial.falhou", "err", err))
			return false
		case b := <-chunks:
			take(b)
		case <-tick.C:
			s.mu.Lock()
			gone := s.linked && s.Now().Sub(last) > quiet
			s.mu.Unlock()
			if gone {
				s.Log(i18n.T("log.serial.calada"))
				s.setLinked(false, connected)
			}
		}
	}
}

func (s *Serial) frame(f []byte, proto cable.Protocol, out func(map[string]string), connected func(bool)) {
	s.Raw(f)
	s.Capture.Offer(f)
	s.setLinked(true, connected)
	sport := s.Sport()
	s.mu.Lock()
	if sport != s.sport { // another sport's keys (sets, shot clock) must not linger
		s.sport, s.vals = sport, map[string]string{}
	}
	if !proto.Decode(f, sport, s.vals) {
		first := s.noLayout != sport
		s.noLayout = sport
		s.mu.Unlock()
		if first {
			s.Log(i18n.T("log.serial.sem_modalidade", "consola", proto.Name(), "modalidade", sport))
		}
		return
	}
	s.noLayout = ""
	vals := maps.Clone(s.vals) // out writes score/*.txt: not under s.mu
	s.mu.Unlock()
	out(vals)
}
