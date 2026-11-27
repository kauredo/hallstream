package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/update"
)

func TestUpdateCheck(t *testing.T) {
	tag := "v1.1.0"
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://github.com/kauredo/hallstream/releases/tag/` + tag + `","assets":[` +
			`{"name":"SHA256SUMS","browser_download_url":"https://github.com/kauredo/hallstream/releases/download/` + tag + `/SHA256SUMS"}]}`))
	}))
	defer gh.Close()
	s := newServer(t)
	s.Version, s.UpdateURL = "1.0.0", gh.URL

	var st struct {
		Update *struct{ Version, URL string } `json:"update"`
	}
	s.checkUpdate() // the check is off: what it finds stays hidden
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update != nil {
		t.Fatalf("check off: %+v", st.Update)
	}

	set := s.Store.Settings()
	set.UpdateCheck = true
	call(t, s, "PUT", "/api/settings", set, nil)
	select {
	case <-s.updateNow:
	default:
		t.Error("switching the check on did not wake RunUpdates")
	}
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update == nil || st.Update.Version != "1.1.0" {
		t.Fatalf("newer release: %+v", st.Update)
	}
	if rel := s.newer.Load(); rel == nil || rel.Assets["SHA256SUMS"] == "" {
		t.Errorf("assets not kept: %+v", rel)
	}

	tag = "v1.0.0"
	s.checkUpdate()
	st.Update = nil
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update != nil {
		t.Errorf("same version: %+v", st.Update)
	}
}

// fakeInstall stands in for update.Install: it runs the hooks as the real
// one does and fails at the step named in fail ("download", "prepare",
// "swap", "panic"). undone says whether it would have put the old program
// back, which the real Install does whenever it returns an error after a swap.
type fakeInstall struct {
	fail     string
	liveAt   string // the step after which OffAir says the stream went live
	steps    []string
	swapped  bool
	restarts []string
}

func (f *fakeInstall) run(ctx context.Context, c *http.Client, rel update.Release, h update.Hooks) error {
	step := func(name string) error {
		f.steps = append(f.steps, name)
		if f.fail == "panic" && name == "swap" {
			panic("boom")
		}
		if f.fail == name {
			return errors.New(name + " failed")
		}
		return nil
	}
	offAir := func(after string) bool { return f.liveAt != after && h.OffAir() }
	if err := step("download"); err != nil {
		return err
	}
	h.Progress(50)
	h.Status("installing")
	if err := step("prepare"); err != nil {
		return err
	}
	if !offAir("prepare") {
		return errors.New("live")
	}
	if err := step("swap"); err != nil {
		return err
	}
	f.swapped = true
	if !offAir("swap") {
		return errors.New("live")
	}
	h.Status("restarting")
	if err := h.Restart("/x/Hallstream"); err != nil {
		return err
	}
	return nil
}

// updateServer offers 9.9.9 and installs it with f.
func updateServer(t *testing.T, f *fakeInstall) *Server {
	t.Helper()
	s := newServer(t)
	s.Install = f.run
	s.CanInstall = func() error { return nil }
	s.Restart = func(target string) error { f.restarts = append(f.restarts, target); return nil }
	s.Store.UpdateSettings(func(set *store.Settings) { set.UpdateCheck = true })
	s.newer.Store(&update.Release{Version: "9.9.9", URL: "https://github.com/kauredo/hallstream/releases/tag/v9.9.9"})
	return s
}

func updateState(t *testing.T, s *Server) map[string]any {
	t.Helper()
	var st struct {
		Update map[string]any `json:"update"`
	}
	call(t, s, "GET", "/api/status", nil, &st)
	return st.Update
}

// fromPhone sends a request from a phone with the PIN and decodes the answer.
func fromPhone(t *testing.T, s *Server, method, path string, body, out any) int {
	t.Helper()
	lanIP(t)
	phoneAccess(s)
	var rd bytes.Buffer
	if body != nil {
		json.NewEncoder(&rd).Encode(body)
	}
	req := httptest.NewRequest(method, path, &rd)
	req.RemoteAddr, req.Host = "192.168.1.20:5000", "192.168.1.40:8765"
	req.Header.Set("X-PIN", "1234")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if out != nil {
		json.Unmarshal(w.Body.Bytes(), out)
	}
	return w.Code
}

func TestUpdateInstalls(t *testing.T) {
	f := &fakeInstall{}
	s := updateServer(t, f)
	if u := updateState(t, s); u["install"] != true || u["version"] != "9.9.9" || u["state"] != "" {
		t.Fatalf("offer %v", u)
	}
	var resp map[string]any
	if code := call(t, s, "POST", "/api/update", map[string]any{}, &resp); code != 200 || resp["version"] != "9.9.9" {
		t.Fatalf("%d %v", code, resp)
	}
	if len(f.restarts) != 1 || f.restarts[0] != "/x/Hallstream" {
		t.Errorf("restarts %v", f.restarts)
	}
	if u := updateState(t, s); u["state"] != "restarting" || u["progress"] != 50.0 || u["on_pc"] != true {
		t.Errorf("state %v", u)
	}
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("a second install: %d", code)
	}
	// The install keeps showing while the old copy closes, even with the check off.
	s.Store.UpdateSettings(func(set *store.Settings) { set.UpdateCheck = false })
	if u := updateState(t, s); u == nil || u["state"] != "restarting" {
		t.Errorf("hidden mid-install: %v", u)
	}
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("stream started during an install: %d", code)
	}
}

func TestUpdateFailures(t *testing.T) {
	for _, step := range []string{"download", "prepare", "swap", "restart", "panic"} {
		f := &fakeInstall{fail: step}
		s := updateServer(t, f)
		if step == "restart" {
			s.Restart = func(string) error { return errors.New("restart failed") }
		}
		call(t, s, "POST", "/api/update", map[string]any{}, nil)
		if u := updateState(t, s); u["state"] != "failed" || u["error"] == "" {
			t.Errorf("%s: %v", step, u)
		}
		f.fail = ""
		if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != 200 {
			t.Errorf("%s: no second try: %d", step, code)
		}
	}
}

// A stream that goes live during the install stops it, before or after the
// swap, and nothing restarts.
func TestUpdateStopsWhenLive(t *testing.T) {
	var live atomic.Bool
	host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
		if typ == "GetStreamStatus" {
			return true, 100, map[string]any{"outputActive": live.Load()}
		}
		return true, 100, nil
	})
	for _, at := range []string{"prepare", "swap"} {
		f := &fakeInstall{liveAt: at}
		s := updateServer(t, f)
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
		call(t, s, "POST", "/api/update", map[string]any{}, nil)
		if len(f.restarts) != 0 || f.swapped != (at == "swap") || updateState(t, s)["state"] != "failed" {
			t.Errorf("live after %s: restarts %v, swapped %v", at, f.restarts, f.swapped)
		}
	}
	live.Store(true)
	s := updateServer(t, &fakeInstall{})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("live: %d", code)
	}
}

func TestUpdateRefusals(t *testing.T) {
	s := updateServer(t, &fakeInstall{})
	var calls int
	s.CanInstall = func() error { calls++; return errors.New("read-only folder") }
	for range 3 {
		if u := updateState(t, s); u["install"] != false {
			t.Errorf("install offered: %v", u)
		}
	}
	if calls != 1 {
		t.Errorf("worked out %d times for the status", calls)
	}
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("refused copy: %d", code)
	}

	s = updateServer(t, &fakeInstall{})
	s.newer.Store(nil)
	if code := call(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusConflict {
		t.Errorf("nothing newer: %d", code)
	}

	s = updateServer(t, &fakeInstall{})
	s.Restart = nil
	if u := updateState(t, s); u["install"] != false {
		t.Errorf("no Restart, install offered: %v", u)
	}

	s = updateServer(t, &fakeInstall{fail: "download"})
	call(t, s, "POST", "/api/update", map[string]any{}, nil)
	if code := fromPhone(t, s, "POST", "/api/update", map[string]any{}, nil); code != http.StatusForbidden {
		t.Errorf("phone: %d", code)
	}
	var st struct {
		Update map[string]any `json:"update"`
		Ask    bool           `json:"update_check_ask"`
	}
	fromPhone(t, s, "GET", "/api/status", nil, &st)
	if st.Update["install"] != false || st.Update["state"] != "" || st.Update["error"] != "" || st.Ask {
		t.Errorf("a phone sees %v, ask %v", st.Update, st.Ask)
	}
}

func TestUpdateAsk(t *testing.T) {
	s := newServer(t)
	ask := func() bool {
		var st struct {
			Ask bool `json:"update_check_ask"`
		}
		call(t, s, "GET", "/api/status", nil, &st)
		return st.Ask
	}
	if !ask() {
		t.Fatal("a new install isn't asked")
	}
	if code := fromPhone(t, s, "POST", "/api/update/ask", map[string]bool{"check": true}, nil); code != http.StatusForbidden {
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
	select {
	case <-s.updateNow:
	default:
		t.Error("Sim did not wake RunUpdates")
	}
	call(t, s, "POST", "/api/update/ask", map[string]bool{"check": false}, &set)
	if !set.UpdateCheck {
		t.Error("a later Não turned the check off")
	}
	// Someone who turned the check on before this version isn't asked.
	s2 := newServer(t)
	s2.Store.UpdateSettings(func(set *store.Settings) { set.UpdateCheck = true })
	var st struct {
		Ask bool `json:"update_check_ask"`
	}
	call(t, s2, "GET", "/api/status", nil, &st)
	if st.Ask {
		t.Error("asked with the check already on")
	}
}
