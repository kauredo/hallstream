package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/update"
)

// fakeUpdater records each step and fails the one named in fail.
type fakeUpdater struct {
	mu      sync.Mutex
	steps   []string
	fail    string
	undone  bool
	refuse  error
	onSwap  func()
	swapped bool
}

func (f *fakeUpdater) step(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, name)
	if f.fail == name {
		return errors.New(name + " failed")
	}
	return nil
}
func (f *fakeUpdater) Target() (string, error)         { return "/x/Hallstream", f.step("target") }
func (f *fakeUpdater) Installable(string) error        { f.step("installable"); return f.refuse }
func (f *fakeUpdater) StageDir(string) (string, error) { return "", f.step("stage") }
func (f *fakeUpdater) Download(ctx context.Context, c *http.Client, rel update.Release, name, dir string) (string, error) {
	return "/x/a.zip", f.step("download " + name)
}
func (f *fakeUpdater) Prepare(string, string) (string, error) { return "/x/new", f.step("prepare") }
func (f *fakeUpdater) Swap(string, string) (func() error, error) {
	if err := f.step("swap"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.swapped = true
	f.mu.Unlock()
	if f.onSwap != nil {
		f.onSwap()
	}
	return func() error { f.mu.Lock(); f.undone = true; f.mu.Unlock(); return nil }, nil
}

// updateServer offers 9.9.9 with a fake updater; restarts counts Restart.
func updateServer(t *testing.T, f *fakeUpdater) (*Server, *atomic.Int32) {
	t.Helper()
	s := newServer(t)
	var restarts atomic.Int32
	s.Updater = f
	s.Restart = func() error { restarts.Add(1); return nil }
	s.Store.UpdateSettings(func(set *store.Settings) { set.UpdateCheck = true })
	s.newer.Store(&update.Release{Version: "9.9.9", URL: "https://github.com/kauredo/hallstream/releases/tag/v9.9.9"})
	return s, &restarts
}

func updateState(t *testing.T, s *Server) map[string]any {
	t.Helper()
	var st struct {
		Update map[string]any `json:"update"`
	}
	call(t, s, "GET", "/api/status", nil, &st)
	return st.Update
}

func fromPhone(t *testing.T, s *Server, path string, body any) int {
	t.Helper()
	lanIP(t)
	phoneAccess(s)
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.RemoteAddr, req.Host = "192.168.1.20:5000", "192.168.1.40:8765"
	req.Header.Set("X-PIN", "1234")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w.Code
}

func TestUpdateInstalls(t *testing.T) {
	f := &fakeUpdater{}
	s, restarts := updateServer(t, f)
	if u := updateState(t, s); u["install"] != true || u["version"] != "9.9.9" || u["state"] != "" {
		t.Fatalf("offer %v", u)
	}
	var resp map[string]any
	if code := call(t, s, "POST", "/api/update", map[string]any{}, &resp); code != 200 || resp["version"] != "9.9.9" {
		t.Fatalf("%d %v", code, resp)
	}
	if restarts.Load() != 1 || f.undone {
		t.Errorf("restarts %d, undone %v, steps %v", restarts.Load(), f.undone, f.steps)
	}
	want := "download " + update.AssetName(runtime.GOOS, runtime.GOARCH, "9.9.9")
	if len(f.steps) < 5 || f.steps[len(f.steps)-3] != want {
		t.Errorf("steps %v", f.steps)
	}
	if u := updateState(t, s); u["state"] != "restarting" {
		t.Errorf("state %v", u)
	}
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("a second install: %d", code)
	}
}

func TestUpdateFailurePutsTheOldBack(t *testing.T) {
	for _, step := range []string{"download " + update.AssetName(runtime.GOOS, runtime.GOARCH, "9.9.9"), "prepare", "swap", "restart"} {
		f := &fakeUpdater{fail: step}
		s, _ := updateServer(t, f)
		if step == "restart" {
			s.Restart = func() error { return errors.New("restart failed") }
		}
		call(t, s, "POST", "/api/update", map[string]any{}, nil)
		u := updateState(t, s)
		if u["state"] != "failed" || u["error"] == "" {
			t.Errorf("%s: %v", step, u)
		}
		if f.undone != f.swapped {
			t.Errorf("%s: swapped %v, undone %v", step, f.swapped, f.undone)
		}
		f.fail = ""
		if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != 200 {
			t.Errorf("%s: no second try: %d", step, code)
		}
	}
}

// A stream that goes live during the download stops the install before the
// program is touched, or puts it back right after the swap.
func TestUpdateStopsWhenLive(t *testing.T) {
	var live atomic.Bool
	host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
		if typ == "GetStreamStatus" {
			return true, 100, map[string]any{"outputActive": live.Load()}
		}
		return true, 100, nil
	})
	f := &fakeUpdater{}
	s, restarts := updateServer(t, f)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
	live.Store(true)
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("live: %d", code)
	}
	live.Store(false)
	f.onSwap = func() { live.Store(true) }
	call(t, s, "POST", "/api/update", map[string]any{}, nil)
	if restarts.Load() != 0 || !f.undone || updateState(t, s)["state"] != "failed" {
		t.Errorf("restarts %d, undone %v, steps %v", restarts.Load(), f.undone, f.steps)
	}
}

func TestUpdateRefusals(t *testing.T) {
	s, _ := updateServer(t, &fakeUpdater{refuse: errors.New("read-only folder")})
	if u := updateState(t, s); u["install"] != false {
		t.Errorf("install offered: %v", u)
	}
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("refused copy: %d", code)
	}
	s, _ = updateServer(t, &fakeUpdater{})
	s.newer.Store(nil)
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("nothing newer: %d", code)
	}
	s, _ = updateServer(t, &fakeUpdater{})
	if code := fromPhone(t, s, "/api/update", map[string]any{}); code != http.StatusForbidden {
		t.Errorf("phone: %d", code)
	}
	s, _ = updateServer(t, &fakeUpdater{})
	s.Restart = nil
	if u := updateState(t, s); u["install"] != false {
		t.Errorf("no Restart, install offered: %v", u)
	}
}

func TestUpdateAsk(t *testing.T) {
	s := newServer(t)
	ask := func() bool {
		var st struct {
			Ask bool `json:"update_ask"`
		}
		call(t, s, "GET", "/api/status", nil, &st)
		return st.Ask
	}
	if !ask() {
		t.Fatal("a new install isn't asked")
	}
	if code := fromPhone(t, s, "/api/update/ask", map[string]bool{"check": true}); code != http.StatusForbidden {
		t.Errorf("phone: %d", code)
	}
	var set store.Settings
	if code := call(t, s, "POST", "/api/update/ask", map[string]bool{"check": false}, &set); code != 200 || !set.UpdateAsked || set.UpdateCheck {
		t.Errorf("%d %+v", code, set)
	}
	if ask() {
		t.Error("asked again")
	}
	// A page loaded before the answer can't bring the question back.
	set.UpdateAsked = false
	call(t, s, "PUT", "/api/settings", set, nil)
	if !s.Store.Settings().UpdateAsked {
		t.Error("PUT /api/settings reset update_asked")
	}
	call(t, s, "POST", "/api/update/ask", map[string]bool{"check": true}, &set)
	if !set.UpdateCheck {
		t.Error("Sim didn't turn the check on")
	}
}
