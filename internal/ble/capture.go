package ble

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

// A capture is a text file with the raw notifications the box sent, so a
// session recorded at a game can be sent to the project and replayed:
//
//	# hallstream capture 1 app=0.1.1 box="Livestream Stramatel" sport=futsal start=2026-11-25T20:31:05Z
//	0 77004d004f004e00...
//	1042 33303734352...
//
// The first line is the header: the program's version, the box's Bluetooth
// name, the club's sport and the start time. The frames carry the team names
// typed into the console, which are often the club's name. Every other line is the time offset in ms since
// the start, a space, and the frame in hex. Lines starting with # are
// comments.

// CaptureMagic starts the header line of every capture file.
const CaptureMagic = "# hallstream capture 1"

// DefaultCaptureMax is the size at which a recording stops.
const DefaultCaptureMax = 50 << 20

// DefaultFolderMax is the size of the capture folder past which a new
// recording is refused.
const DefaultFolderMax = 300 << 20

// ErrFolderFull is Start's answer when the folder is over its limit.
var ErrFolderFull error = i18n.Key("api.captura.pasta_cheia")

// What ReadCapture accepts: a file no bigger than a recording plus its
// header, a bounded number of frames, and offsets under a week.
var maxCaptureFrames = 2_000_000

const (
	maxCaptureOffset = 7 * 24 * time.Hour
	headerSlack      = 4096
)

// Frame is one recorded notification.
type Frame struct {
	At   time.Duration // since the recording started
	Data []byte
}

// ReadCapture parses a capture file. It rejects a file without the header
// and any line that is not "<ms> <hex>".
func ReadCapture(r io.Reader) ([]Frame, error) {
	_, frames, err := ReadCaptureFile(r)
	return frames, err
}

// ReadCaptureFile is ReadCapture with the header's fields too (app, box,
// sport, start, and link and protocol for a cable recording).
func ReadCaptureFile(r io.Reader) (map[string]string, []Frame, error) {
	lr := &io.LimitedReader{R: r, N: DefaultCaptureMax + headerSlack + 1}
	sc := bufio.NewScanner(lr)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	var out []Frame
	head := map[string]string{}
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			if !strings.HasPrefix(line, CaptureMagic) {
				return nil, nil, errors.New("not a Hallstream capture: the first line is not the header")
			}
			head = headerFields(line[len(CaptureMagic):])
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ms, data, ok := strings.Cut(line, " ")
		at, err := strconv.ParseInt(ms, 10, 64)
		if !ok || err != nil || at < 0 {
			return nil, nil, fmt.Errorf("line %d: want \"<ms> <hex>\", got %q", n, line)
		}
		b, err := hex.DecodeString(strings.TrimSpace(data))
		if err != nil || len(b) == 0 {
			return nil, nil, fmt.Errorf("line %d: the frame is not hex", n)
		}
		if at > maxCaptureOffset.Milliseconds() {
			return nil, nil, fmt.Errorf("line %d: offset %d ms is over 7 days", n, at)
		}
		if len(out) >= maxCaptureFrames {
			return nil, nil, fmt.Errorf("more than %d frames", maxCaptureFrames)
		}
		out = append(out, Frame{At: time.Duration(at) * time.Millisecond, Data: b})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	if lr.N <= 0 {
		return nil, nil, errors.New("the file is bigger than any recording the program makes")
	}
	return head, out, nil
}

// headerFields reads the header's key="value" pairs; a pair it cannot
// read ends the list.
func headerFields(s string) map[string]string {
	out := map[string]string{}
	for {
		s = strings.TrimLeft(s, " ")
		key, rest, ok := strings.Cut(s, "=")
		if !ok || key == "" || strings.Contains(key, " ") {
			return out
		}
		q, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return out
		}
		out[key], _ = strconv.Unquote(q)
		s = rest[len(q):]
	}
}

// Play sends each frame to emit at its recorded offset. sleep waits for a
// gap and returns false to stop the replay early.
func Play(frames []Frame, emit func([]byte), sleep func(time.Duration) bool) {
	var at time.Duration
	for _, f := range frames {
		if gap := f.At - at; gap > 0 {
			if !sleep(gap) {
				return
			}
		}
		at = max(at, f.At)
		emit(f.Data)
	}
}

// CaptureStatus is what the panel shows about the recording.
type CaptureStatus struct {
	On      bool   `json:"on"`
	File    string `json:"file"`    // the last recording's name, "" before the first
	Bytes   int64  `json:"bytes"`   // written so far
	Frames  int64  `json:"frames"`  // written so far
	Dropped int64  `json:"dropped"` // frames the writer was too slow for
	Max     int64  `json:"max"`     // size that stops a recording
	// Stopped says why the last recording ended: "" while on or before the
	// first, then "off", "cap", "box" (it disconnected) or "error".
	// The disk's own message is in the log: it names folders.
	Stopped string `json:"stopped"`
}

// Recorder writes the raw frames to a file without ever holding the reader
// up. Offer puts a frame on a buffered channel and drops it, counted, when
// the channel is full; a goroutine writes the file.
type Recorder struct {
	Dir   string        // dados/captura
	App   string        // the program's version, for the header
	Sport func() string // the club's sport, for the header; may be nil
	// Link and Protocol name a cable recording in the header (link="serial");
	// empty for the Bluetooth box.
	Link     string
	Protocol func() string
	Max      int64 // bytes at which it stops; 0 is DefaultCaptureMax
	// FolderMax is the size of Dir past which Start refuses; 0 is DefaultFolderMax.
	FolderMax int64
	Slots     int // frames waiting for the writer; 0 is 4096
	Now       func() time.Time
	Log       func(string)
	// Create opens a new file; nil makes it in Dir, and the folder too.
	Create func(path string) (io.WriteCloser, error)

	cur atomic.Pointer[session]
	mu  sync.Mutex // last
	// last is the newest recording, running or over.
	last *session
}

type session struct {
	ch      chan []byte   // each frame already carries its line
	quit    chan struct{} // closed once, by finish
	done    chan struct{} // closed when the writer is out
	once    sync.Once
	started time.Time
	name    string

	frames, bytes, dropped atomic.Int64
	mu                     sync.Mutex
	stopped                string
}

func (r *Recorder) max() int64 {
	if r.Max > 0 {
		return r.Max
	}
	return DefaultCaptureMax
}

// Offer hands the writer one raw frame. It never blocks and is safe on a nil
// Recorder or with nothing recording.
func (r *Recorder) Offer(f []byte) {
	if r == nil {
		return
	}
	s := r.cur.Load()
	if s == nil {
		return
	}
	line := fmt.Sprintf("%d %s\n", time.Since(s.started).Milliseconds(), hex.EncodeToString(f))
	select {
	case s.ch <- []byte(line):
	default:
		s.dropped.Add(1)
	}
}

func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Recorder) log(msg string) {
	if r.Log != nil {
		r.Log(msg)
	}
}

// Start opens a new file and begins recording. box is the device's name.
// run starts the writer in the background (Server.once or safely).
func (r *Recorder) Start(box string, run func(name string, fn func())) (CaptureStatus, error) {
	if r == nil {
		return CaptureStatus{}, i18n.Err("api.ble.sem_leitor")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur.Load() != nil {
		return r.statusLocked(), nil
	}
	if r.folderSize() > r.folderMax() {
		return r.statusLocked(), ErrFolderFull
	}
	utc := r.now().UTC()
	name := "captura-" + r.now().Format("2006-01-02_15-04-05") + ".txt"
	w, name, err := r.create(name)
	if err != nil {
		return r.statusLocked(), err
	}
	sport := ""
	if r.Sport != nil {
		sport = r.Sport()
	}
	header := fmt.Sprintf("%s app=%s box=%s sport=%s start=%s", CaptureMagic,
		strconv.Quote(r.App), strconv.Quote(box), strconv.Quote(sport), strconv.Quote(utc.Format(time.RFC3339)))
	if r.Link != "" {
		proto := ""
		if r.Protocol != nil {
			proto = r.Protocol()
		}
		header += fmt.Sprintf(" link=%s protocol=%s", strconv.Quote(r.Link), strconv.Quote(proto))
	}
	header += "\n"
	s := &session{
		ch: make(chan []byte, cmpSlots(r.Slots)), quit: make(chan struct{}), done: make(chan struct{}),
		started: time.Now(), name: name,
	}
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(header); err != nil {
		w.Close()
		return r.statusLocked(), err
	}
	if err := bw.Flush(); err != nil {
		w.Close()
		return r.statusLocked(), err
	}
	s.bytes.Store(int64(len(header)))
	r.last = s
	r.cur.Store(s)
	r.log(i18n.T("log.captura.comecou", "ficheiro", name))
	run(i18n.T("log.tarefa.captura"), func() { r.write(s, w, bw) })
	return r.statusLocked(), nil
}

func (r *Recorder) folderMax() int64 {
	if r.FolderMax > 0 {
		return r.FolderMax
	}
	return DefaultFolderMax
}

// folderSize is what the recordings in Dir take; a missing folder is empty.
func (r *Recorder) folderSize() int64 {
	var n int64
	files, _ := os.ReadDir(r.Dir)
	for _, f := range files {
		if info, err := f.Info(); err == nil && !f.IsDir() {
			n += info.Size()
		}
	}
	return n
}

func cmpSlots(n int) int {
	if n > 0 {
		return n
	}
	return 4096
}

// create opens the file, adding -2, -3… when the name is taken.
func (r *Recorder) create(name string) (io.WriteCloser, string, error) {
	create := r.Create
	if create == nil {
		create = func(path string) (io.WriteCloser, error) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		}
	}
	stem := strings.TrimSuffix(name, ".txt")
	try := name
	for i := 2; ; i++ {
		w, err := create(filepath.Join(r.Dir, try))
		if err == nil {
			return w, try, nil
		}
		if !errors.Is(err, os.ErrExist) || i > 9 {
			return nil, "", err
		}
		try = fmt.Sprintf("%s-%d.txt", stem, i)
	}
}

// write is the writer goroutine: it takes lines off the channel until the
// recording is stopped, the file is full or the disk fails.
func (r *Recorder) write(s *session, w io.WriteCloser, bw *bufio.Writer) {
	defer close(s.done) // last: the status is final before anyone hears it
	defer func() {
		if p := recover(); p != nil { // a panic in the file must not leave the panel on
			r.finish(s, "error", fmt.Errorf("writer panic: %v", p))
		}
		// Whatever is buffered reaches the file; a writer that panics once may again.
		func() { defer func() { recover() }(); bw.Flush() }()
		func() { defer func() { recover() }(); w.Close() }()
	}()
	limit := r.max()
	put := func(line []byte) bool {
		if s.bytes.Load()+int64(len(line)) > limit {
			bw.Flush() // the file is complete before the panel hears it stopped
			r.finish(s, "cap", nil)
			return false
		}
		if _, err := bw.Write(line); err != nil {
			r.finish(s, "error", err)
			return false
		}
		s.bytes.Add(int64(len(line)))
		s.frames.Add(1)
		return true
	}
	for {
		select {
		case line := <-s.ch:
			if !put(line) {
				return
			}
			if len(s.ch) == 0 {
				if err := bw.Flush(); err != nil {
					r.finish(s, "error", err)
					return
				}
			}
		case <-s.quit:
			for { // what the reader handed over before the stop
				select {
				case line := <-s.ch:
					if !put(line) {
						return
					}
				default:
					if err := bw.Flush(); err != nil {
						r.finish(s, "error", err)
					}
					return
				}
			}
		}
	}
}

// finish records why a session ended and stops taking frames; the first
// reason wins.
func (r *Recorder) finish(s *session, why string, err error) {
	s.mu.Lock()
	if s.stopped != "" {
		s.mu.Unlock()
		return
	}
	s.stopped = why
	s.mu.Unlock()
	r.cur.CompareAndSwap(s, nil)
	s.once.Do(func() { close(s.quit) })
	switch why {
	case "cap":
		r.log(i18n.T("log.captura.limite", "mb", r.max()>>20, "ficheiro", s.name))
	case "box":
		r.log(i18n.T("log.captura.caixa", "ficheiro", s.name))
	case "error":
		r.log(i18n.T("log.captura.erro", "err", err))
	default:
		r.log(i18n.T("log.captura.parou", "ficheiro", s.name, "frames", s.frames.Load()))
	}
	if n := s.dropped.Load(); n > 0 {
		r.log(i18n.T("log.captura.perdidos", "n", n))
	}
}

// StopNoWait ends the recording without waiting for the file: the writer
// closes it in its own time. The Bluetooth reader uses this one.
func (r *Recorder) StopNoWait(why string) {
	if r == nil {
		return
	}
	if s := r.cur.Load(); s != nil {
		r.finish(s, why, nil)
	}
}

// Stop ends the recording and waits up to 2 s for the file to close. why is
// "off" for the switch and "box" for a lost link. Nothing recording is a
// no-op. It holds no lock while it waits, so Status and Start never sit
// behind a stuck disk.
func (r *Recorder) Stop(why string) CaptureStatus {
	if r == nil {
		return CaptureStatus{}
	}
	if s := r.cur.Load(); s != nil {
		r.finish(s, why, nil)
		select {
		case <-s.done:
		case <-time.After(2 * time.Second): // a stuck disk must not hold the caller
		}
	}
	return r.Status()
}

// Status is the panel's view of the recording.
func (r *Recorder) Status() CaptureStatus {
	if r == nil {
		return CaptureStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

func (r *Recorder) statusLocked() CaptureStatus {
	st := CaptureStatus{Max: r.max()}
	s := r.last
	if s == nil {
		return st
	}
	st.File, st.Bytes, st.Frames, st.Dropped = s.name, s.bytes.Load(), s.frames.Load(), s.dropped.Load()
	s.mu.Lock()
	st.Stopped = s.stopped
	s.mu.Unlock()
	st.On = r.cur.Load() == s
	return st
}
