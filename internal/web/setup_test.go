package web

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/kauredo/tribuna/internal/store"
)

type setupState struct {
	PC    bool              `json:"pc"`
	Done  string            `json:"done"`
	Later bool              `json:"later"`
	Steps map[string]string `json:"steps"`
	Error string            `json:"error"`
}

func TestSetupSteps(t *testing.T) {
	s := newServer(t)
	var st setupState
	if code := call(t, s, "GET", "/api/setup", nil, &st); code != 200 || !st.PC || st.Done != "" || st.Later || st.Steps == nil || len(st.Steps) != 0 {
		t.Fatalf("fresh: %d %+v", code, st)
	}
	call(t, s, "PUT", "/api/setup/steps/canal-diretos", map[string]string{"state": "done"}, nil)
	if code := call(t, s, "PUT", "/api/setup/steps/youtube", map[string]string{"state": "skipped"}, &st); code != 200 ||
		st.Steps["canal-diretos"] != "done" || st.Steps["youtube"] != "skipped" {
		t.Fatalf("ticked: %d %+v", code, st)
	}
	st = setupState{} // Unmarshal adds to a map it is given
	call(t, s, "PUT", "/api/setup/steps/canal-diretos", map[string]string{"state": ""}, &st)
	if _, ok := st.Steps["canal-diretos"]; ok || st.Steps["youtube"] != "skipped" {
		t.Errorf("untick: %+v", st)
	}
	if code := call(t, s, "PUT", "/api/setup/steps/obs", map[string]string{"state": "done"}, &st); code != 404 {
		t.Errorf("a step the program checks: %d", code)
	}
	if code := call(t, s, "PUT", "/api/setup/steps/youtube", map[string]string{"state": "talvez"}, &st); code != 400 {
		t.Errorf("bad state: %d", code)
	}

	call(t, s, "POST", "/api/setup/later", map[string]any{}, &st)
	if !st.Later || st.Done != "" {
		t.Errorf("later: %+v", st)
	}
	call(t, s, "POST", "/api/setup/done", map[string]any{}, &st)
	if st.Later || st.Done == "" || st.Steps["youtube"] != "skipped" {
		t.Errorf("done: %+v", st)
	}

	// Saving the settings form keeps the setup, whatever the form sends.
	for _, sent := range []store.FirstSetup{{}, {Done: "", Steps: map[string]string{"youtube": "done", "ensaio-demo": "done"}}} {
		set := s.Store.Settings()
		set.FirstSetup = sent
		call(t, s, "PUT", "/api/settings", set, nil)
		if f := s.Store.Settings().FirstSetup; f.Done != st.Done || len(f.Steps) != 1 || f.Steps["youtube"] != "skipped" {
			t.Errorf("after PUT /api/settings with %+v: %+v", sent, f)
		}
	}
}

func TestSetupOnPCOnly(t *testing.T) {
	s := newServer(t)
	lanIP(t)
	set := s.Store.Settings()
	set.PhoneAccess, set.PIN = true, "1234"
	s.Store.SaveSettings(set)
	phone := func(method, path string) *httptest.ResponseRecorder {
		var body *bytes.Reader
		if method != "GET" {
			body = bytes.NewReader([]byte(`{"state":"done"}`))
		} else {
			body = bytes.NewReader(nil)
		}
		r := httptest.NewRequest(method, path, body)
		r.RemoteAddr, r.Host = "192.168.1.20:5000", "192.168.1.40:8765"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-PIN", "1234")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := phone("GET", "/api/setup"); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"pc":false`)) {
		t.Errorf("phone GET: %d %s", w.Code, w.Body)
	}
	for _, p := range []string{"/api/setup/steps/youtube", "/api/setup/done", "/api/setup/later", "/api/setup/channel"} {
		m := "POST"
		if p == "/api/setup/steps/youtube" {
			m = "PUT"
		}
		if w := phone(m, p); w.Code != 403 {
			t.Errorf("phone %s %s: %d", m, p, w.Code)
		}
	}
	if f := s.Store.Settings().FirstSetup; f.Done != "" || f.Later || len(f.Steps) != 0 {
		t.Errorf("phone changed the setup: %+v", f)
	}
}

func TestSetupChannelCheck(t *testing.T) {
	f := newFakeYT(t)
	s := newServer(t)
	var e setupState
	if code := call(t, s, "POST", "/api/setup/channel", map[string]any{}, &e); code != 409 || e.Error == "" {
		t.Errorf("not connected: %d %+v", code, e)
	}
	connect(s)
	type check struct{ Live, Verified string }
	for _, c := range []struct {
		long    string
		liveOff bool
		want    check
	}{
		{"allowed", false, check{"on", "yes"}},
		{"eligible", true, check{"off", "no"}},
		{"disallowed", false, check{"on", "blocked"}},
		{"", false, check{"on", ""}},
	} {
		f.set(func(f *fakeYT) { f.longUploads, f.liveOff = c.long, c.liveOff })
		var got check
		if code := call(t, s, "POST", "/api/setup/channel", map[string]any{}, &got); code != 200 || got != c.want {
			t.Errorf("%q live off %v: %d %+v, want %+v", c.long, c.liveOff, code, got, c.want)
		}
	}
	if n := f.count("GET /channels"); n != 4 {
		t.Errorf("channels.list %d times", n)
	}

	// A refused login marks YouTube expired, as everywhere else.
	f.set(func(f *fakeYT) { f.refreshFail = true })
	connect(s)
	if code := call(t, s, "POST", "/api/setup/channel", map[string]any{}, &e); code != 502 || !s.ytGet().Expired {
		t.Errorf("refused: %d %+v expired %v", code, e, s.ytGet().Expired)
	}
}
