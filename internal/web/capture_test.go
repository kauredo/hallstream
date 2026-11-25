package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
)

// boxRadio connects to anything and hands the test the link's frame callback.
type boxRadio struct {
	stop chan struct{}
	link *boxLink
}

func (f *boxRadio) Enable() error               { return nil }
func (f *boxRadio) Scan(func(ble.Advert)) error { <-f.stop; return nil }
func (f *boxRadio) StopScan() error             { f.stop <- struct{}{}; return nil }
func (f *boxRadio) Connect(string) (ble.Link, error) {
	return f.link, nil
}

type boxLink struct {
	mu    sync.Mutex
	frame func([]byte)
	up    bool
}

func (l *boxLink) Listen(f func([]byte)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frame, l.up = f, true
	return nil
}
func (l *boxLink) Connected() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.up }
func (l *boxLink) Disconnect()     { l.mu.Lock(); defer l.mu.Unlock(); l.up = false }
func (l *boxLink) send(b []byte)   { l.mu.Lock(); f := l.frame; l.mu.Unlock(); f(b) }
func (l *boxLink) drop()           { l.mu.Lock(); l.up = false; l.mu.Unlock() }

// post is call without the wait on background work, which a recording is.
func post(s *Server, method, path string, body any, out any) int {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.RemoteAddr, req.Host = "127.0.0.1:5000", "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if out != nil {
		json.Unmarshal(w.Body.Bytes(), out)
	}
	return w.Code
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(6 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestCaptureSwitch(t *testing.T) {
	s := newServer(t)
	var st ble.CaptureStatus
	if code := post(s, "GET", "/api/ble/capture", nil, &st); code != 200 || st.On || st.Max != ble.DefaultCaptureMax {
		t.Fatalf("demo status: %d %+v", code, st)
	}
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, nil); code != 503 {
		t.Fatalf("switch on without a reader: %d, want 503", code)
	}

	link := &boxLink{}
	s.BLE = ble.New(&ble.Reader{Radio: &boxRadio{stop: make(chan struct{}), link: link}})
	s.BLE.Capture = &ble.Recorder{Dir: s.captureDir(), App: "test", Sport: func() string { return "handball" }}
	go s.BLE.Run()

	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, nil); code != 409 {
		t.Fatalf("switch on with no box linked: %d, want 409", code)
	}
	if err := s.BLE.Try("AA:BB"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the box link", func() bool { ok, _ := s.BLE.Linked(); return ok })

	// Whatever fails below, a running recording must not outlive the test:
	// the cleanup waits on the writer goroutine.
	t.Cleanup(func() { s.BLE.Capture.Stop("off") })
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, &st); code != 200 || !st.On || st.File == "" {
		t.Fatalf("switch on: %d %+v", code, st)
	}
	link.send(make([]byte, 20))
	link.send(make([]byte, 20))
	waitUntil(t, "two frames written", func() bool { return s.BLE.Capture.Status().Frames == 2 })
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": false}, &st); code != 200 || st.On || st.Stopped != "off" || st.Frames != 2 {
		t.Fatalf("switch off: %d %+v", code, st)
	}
	f, err := os.Open(filepath.Join(s.captureDir(), st.File))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if frames, err := ble.ReadCapture(f); err != nil || len(frames) != 2 {
		t.Fatalf("file: %d frames, %v", len(frames), err)
	}

	// A lost link ends a recording on its own.
	post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, nil)
	link.drop()
	waitUntil(t, "the recording to stop with the box", func() bool {
		c := s.BLE.Capture.Status()
		return !c.On && c.Stopped == "box"
	})
}

func TestCaptureFolder(t *testing.T) {
	s := newServer(t)
	if code := post(s, "POST", "/api/system/capture-folder", nil, nil); code != 501 {
		t.Fatalf("without a file manager: %d, want 501", code)
	}
	var opened string
	s.OpenFolder = func(dir string) error { opened = dir; return nil }
	if code := post(s, "POST", "/api/system/capture-folder", nil, nil); code != 200 || opened != filepath.Join(s.Store.Dir, "captura") {
		t.Fatalf("open: %d %q", code, opened)
	}
	req := httptest.NewRequest("POST", "/api/system/capture-folder", nil)
	req.RemoteAddr, req.Host = "192.168.1.40:5000", "192.168.1.5:8765"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	opened = ""
	s.ServeHTTP(w, req)
	if w.Code == 200 || opened != "" {
		t.Errorf("another device opened the folder: %d", w.Code)
	}
}

// started builds a server whose box is linked, with the recorder rec.
func started(t *testing.T, rec *ble.Recorder) (*Server, *boxLink) {
	t.Helper()
	s := newServer(t)
	link := &boxLink{}
	s.BLE = ble.New(&ble.Reader{Radio: &boxRadio{stop: make(chan struct{}), link: link}})
	rec.Dir = cmpDir(rec.Dir, s.captureDir())
	s.BLE.Capture = rec
	t.Cleanup(func() { rec.Stop("off") })
	go s.BLE.Run()
	if err := s.BLE.Try("AA:BB"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the box link", func() bool { ok, _ := s.BLE.Linked(); return ok })
	return s, link
}

func cmpDir(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func TestCaptureStopsWhenTheLinkDropsDuringStart(t *testing.T) {
	var link *boxLink
	var s *Server
	rec := &ble.Recorder{}
	rec.Create = func(path string) (io.WriteCloser, error) {
		// The link goes while the file is being made; the reader notices on its next tick.
		link.drop()
		waitUntil(t, "the reader to see the drop", func() bool { ok, _ := s.BLE.Linked(); return !ok })
		return nopCloser{io.Discard}, nil
	}
	s, link = started(t, rec)
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, nil); code != 409 {
		t.Fatalf("switch on as the link drops: %d, want 409", code)
	}
	if st := rec.Status(); st.On || st.Stopped != "box" {
		t.Errorf("status = %+v, want stopped by the box", st)
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func TestCaptureErrorsNameNoFolder(t *testing.T) {
	rec := &ble.Recorder{}
	var s *Server
	rec.Create = func(path string) (io.WriteCloser, error) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
	}
	s, _ = started(t, rec)
	var logged []string
	s.Log = func(m string) { logged = append(logged, m) }
	req := func(path string) string {
		b, _ := json.Marshal(map[string]bool{"on": true})
		r := httptest.NewRequest("POST", path, bytes.NewReader(b))
		r.RemoteAddr, r.Host = "127.0.0.1:5000", "127.0.0.1:8765"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 500 {
			t.Fatalf("%s: %d, want 500", path, w.Code)
		}
		return w.Body.String()
	}
	if body := req("/api/ble/capture"); strings.Contains(body, s.Store.Dir) {
		t.Errorf("the reply names the folder: %s", body)
	}
	if len(logged) == 0 || !strings.Contains(logged[len(logged)-1], s.Store.Dir) {
		t.Errorf("the log lacks the full error: %v", logged)
	}
	s.OpenFolder = func(dir string) error { return &os.PathError{Op: "start", Path: dir, Err: os.ErrNotExist} }
	if body := req("/api/system/capture-folder"); strings.Contains(body, s.Store.Dir) {
		t.Errorf("the reply names the folder: %s", body)
	}
}

func TestCaptureFolderFull(t *testing.T) {
	rec := &ble.Recorder{FolderMax: 500}
	s, _ := started(t, rec)
	if err := os.MkdirAll(s.captureDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.captureDir(), "old.txt"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Error string `json:"error"`
	}
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, &out); code != 409 || !strings.Contains(out.Error, "300 MB") {
		t.Fatalf("switch on with a full folder: %d %q", code, out.Error)
	}
	if rec.Status().On {
		t.Error("recording started in a full folder")
	}
}
