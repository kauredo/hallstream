package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/net/websocket"

	"github.com/kauredo/tribuna/internal/obsapp"
	"github.com/kauredo/tribuna/internal/obsws"
	"github.com/kauredo/tribuna/internal/store"
)

type wsMsg struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

// fakeOBS is an obs-websocket server with password; answer gets each request.
func fakeOBS(t *testing.T, password string, answer func(typ string) (bool, int, any)) (string, int) {
	return fakeOBSData(t, password, func(typ string, _ json.RawMessage) (bool, int, any) { return answer(typ) })
}

// fakeOBSData is fakeOBS with each request's requestData.
func fakeOBSData(t *testing.T, password string, answer func(typ string, data json.RawMessage) (bool, int, any)) (string, int) {
	var mu sync.Mutex
	srv := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		hello, _ := json.Marshal(map[string]any{"rpcVersion": 1, "authentication": map[string]string{"challenge": "c", "salt": "s"}})
		websocket.JSON.Send(ws, wsMsg{0, hello})
		var m wsMsg
		if websocket.JSON.Receive(ws, &m) != nil {
			return
		}
		var id struct {
			Authentication string `json:"authentication"`
		}
		json.Unmarshal(m.D, &id)
		if id.Authentication != obsws.Auth(password, "s", "c") {
			return
		}
		websocket.JSON.Send(ws, wsMsg{2, json.RawMessage(`{"negotiatedRpcVersion":1}`)})
		for {
			if websocket.JSON.Receive(ws, &m) != nil {
				return
			}
			var req struct {
				Type string          `json:"requestType"`
				ID   string          `json:"requestId"`
				Data json.RawMessage `json:"requestData"`
			}
			json.Unmarshal(m.D, &req)
			mu.Lock()
			ok, code, data := answer(req.Type, req.Data)
			mu.Unlock()
			resp := map[string]any{"requestType": req.Type, "requestId": req.ID,
				"requestStatus": map[string]any{"result": ok, "code": code, "comment": "fake"}}
			if data != nil {
				resp["responseData"] = data
			}
			b, _ := json.Marshal(resp)
			websocket.JSON.Send(ws, wsMsg{7, b})
		}
	}))
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return host, p
}

type fakeApp struct {
	running bool
	starts  int
}

func (f *fakeApp) app(path string) obsapp.App {
	return obsapp.App{ConfigPath: path,
		Running: func() (bool, error) { return f.running, nil },
		Start:   func() error { f.starts++; return errors.New("sem OBS no teste") },
		Close:   func(bool) error { return nil },
	}
}

func obsOff(t *testing.T, s *Server) {
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Port = 1 }) // nothing listens there
}

func TestReachOBSRunningLeavesFile(t *testing.T) {
	s := newServer(t)
	obsOff(t, s)
	path := filepath.Join(t.TempDir(), "config.json")
	f := &fakeApp{running: true}
	s.OBSApp = f.app(path)
	_, st, ok := s.reachOBS(true)
	if ok || st.Result != "failed" || f.starts != 0 {
		t.Fatalf("%+v starts %d", st, f.starts)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("wrote the config while OBS runs")
	}
}

func TestReachOBSClosedTurnsServerOn(t *testing.T) {
	s := newServer(t)
	obsOff(t, s)
	path := filepath.Join(t.TempDir(), "obs-websocket", "config.json")
	f := &fakeApp{}
	s.OBSApp = f.app(path)
	_, st, ok := s.reachOBS(true)
	if ok || f.starts != 1 || st.Detail != "sem OBS no teste" {
		t.Fatalf("%+v starts %d", st, f.starts)
	}
	ws, err := obsapp.ReadWebSocket(path)
	o := s.Store.Settings().OBS
	if err != nil || !ws.Enabled || ws.Password == "" || o.Password != ws.Password || o.Port != 4455 || !o.Enabled {
		t.Errorf("file %+v, settings %+v", ws, o)
	}
	// Again: the server is on now, the password stays.
	s.reachOBS(true)
	if again, _ := obsapp.ReadWebSocket(path); again.Password != ws.Password {
		t.Error("changed the password")
	}
}

func TestReachOBSReadsPassword(t *testing.T) {
	s := newServer(t)
	obsOff(t, s)
	host, port := fakeOBS(t, "do-ficheiro", func(string) (bool, int, any) { return true, 100, nil })
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"server_enabled":true,"server_port":`+strconv.Itoa(port)+`,"auth_required":true,"server_password":"do-ficheiro"}`), 0o644)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host = host })
	s.OBSApp = (&fakeApp{running: true}).app(path)
	_, st, ok := s.reachOBS(true)
	if o := s.Store.Settings().OBS; !ok || o.Password != "do-ficheiro" || o.Port != port {
		t.Errorf("%+v settings %+v", st, o)
	}
}

func TestReachOBSFromPhone(t *testing.T) {
	s := newServer(t)
	obsOff(t, s)
	path := filepath.Join(t.TempDir(), "config.json")
	f := &fakeApp{}
	s.OBSApp = f.app(path)
	if _, _, ok := s.reachOBS(false); ok || f.starts != 0 {
		t.Error("a phone started OBS")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a phone wrote the config")
	}
}

func TestReachOBSOtherPC(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = "192.0.2.1", 1 })
	path := filepath.Join(t.TempDir(), "config.json")
	f := &fakeApp{}
	s.OBSApp = f.app(path)
	if _, _, ok := s.reachOBS(true); ok || f.starts != 0 {
		t.Error("touched OBS on another PC")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("wrote the config for another PC")
	}
}

func TestStreamNoKey(t *testing.T) {
	s := newServer(t)
	started := false
	host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
		switch typ {
		case "GetStreamServiceSettings":
			return true, 100, map[string]any{"streamServiceType": "rtmp_custom", "streamServiceSettings": map[string]any{"server": "rtmps://a.rtmps.youtube.com/live2"}}
		case "StartStream":
			started = true
		}
		return true, 100, nil
	})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
	var resp map[string]string
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &resp); code != 409 || resp["error"] != obsws.ErrNoStreamKey.Error() || started {
		t.Errorf("%d %v started %v", code, resp, started)
	}
}

func TestStreamStartStop(t *testing.T) {
	s := newServer(t)
	live := false
	host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
		switch typ {
		case "GetStreamServiceSettings":
			return true, 100, map[string]any{"streamServiceType": "rtmp_common", "streamServiceSettings": map[string]any{"key": "abcd"}}
		case "StartStream":
			live = true
		case "StopStream":
			live = false
		case "GetStreamStatus":
			return true, 100, map[string]any{"outputActive": live, "outputDuration": 61000}
		}
		return true, 100, nil
	})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
	var resp struct {
		Stream *obsws.Stream `json:"stream"`
	}
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &resp); code != 200 || resp.Stream == nil || !resp.Stream.Active || resp.Stream.DurationMS != 61000 {
		t.Fatalf("%d %+v", code, resp.Stream)
	}
	if !s.Streaming() {
		t.Error("Streaming")
	}
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, &resp)
	if resp.Stream == nil || resp.Stream.Active || live {
		t.Errorf("still live %+v", resp.Stream)
	}
}

// OBS can take StartStream and still not send, as when a YouTube account
// linked in OBS has no broadcast: the start is then an error, and OBS stops.
func TestStreamStartNeverLive(t *testing.T) {
	s := newServer(t)
	defer func(n int) { startPolls = n }(startPolls)
	startPolls = 3
	var stopped atomic.Bool
	var reads atomic.Int32
	liveAt := int32(0) // the status read that first says active; 0 = never
	host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
		switch typ {
		case "GetStreamServiceSettings":
			return true, 100, map[string]any{"streamServiceType": "rtmp_common", "streamServiceSettings": map[string]any{}}
		case "StopStream":
			stopped.Store(true)
		case "GetStreamStatus":
			n := reads.Add(1)
			if n == 1 {
				return false, 702, nil
			}
			return true, 100, map[string]any{"outputActive": liveAt != 0 && n >= liveAt}
		}
		return true, 100, nil
	})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
	var resp map[string]string
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &resp); code != 502 || resp["error"] != errNotLive.Error() || !stopped.Load() {
		t.Errorf("%d %v stopped %v", code, resp, stopped.Load())
	}

	// A failed read while OBS connects is not a failed start.
	stopped.Store(false)
	reads.Store(0)
	liveAt = 2
	var ok streamReply
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &ok); code != 200 || ok.Stream == nil || !ok.Stream.Active || stopped.Load() {
		t.Errorf("after a failed read: %d %+v stopped %v", code, ok, stopped.Load())
	}
}

func TestAutostart(t *testing.T) {
	s := newServer(t)
	var resp map[string]any
	if code := call(t, s, "POST", "/api/system/autostart", map[string]bool{"on": true}, &resp); code != 501 {
		t.Errorf("no Autostart here: %d", code)
	}
	var got []bool
	s.Autostart = func(on bool) error { got = append(got, on); return nil }
	call(t, s, "POST", "/api/system/autostart", map[string]bool{"on": true}, &resp)
	if !s.Store.Settings().StartWithWindows {
		t.Error("not saved")
	}
	// The panel's settings form never changes it.
	set := s.Store.Settings()
	set.StartWithWindows = false
	call(t, s, "PUT", "/api/settings", set, nil)
	call(t, s, "POST", "/api/system/autostart", map[string]bool{"on": false}, &resp)
	if len(got) != 2 || got[0] != true || got[1] != false || s.Store.Settings().StartWithWindows {
		t.Errorf("calls %v, saved %v", got, s.Store.Settings().StartWithWindows)
	}
}

// A phone with the PIN never sees the OBS password, and can't change what
// reaches OBS or starts programs on the PC.
func TestSettingsFromPhone(t *testing.T) {
	s := newServer(t)
	lanIP(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.PhoneAccess, set.PIN = true, "1234"
		set.OBS.Password, set.OBS.Launch = "segredo", true
	})
	phone := func(method string, body any) store.Settings {
		var b bytes.Buffer
		json.NewEncoder(&b).Encode(body)
		req := httptest.NewRequest(method, "/api/settings", &b)
		req.RemoteAddr, req.Host = "192.168.1.20:5000", "192.168.1.40:8765"
		req.Header.Set("X-PIN", "1234")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %d %s", method, w.Code, w.Body)
		}
		var got store.Settings
		json.Unmarshal(w.Body.Bytes(), &got)
		return got
	}
	got := phone("GET", nil)
	if got.OBS.Password != "" {
		t.Error("the phone got the OBS password")
	}
	got.OBS.Host, got.OBS.Launch, got.OBS.CloseOnExit, got.Instagram = "192.0.2.9", false, true, "@outro"
	phone("PUT", got)
	o := s.Store.Settings()
	if o.OBS.Password != "segredo" || o.OBS.Host != "127.0.0.1" || !o.OBS.Launch || o.OBS.CloseOnExit || o.Instagram != "@outro" {
		t.Errorf("after the phone's PUT: %+v, instagram %q", o.OBS, o.Instagram)
	}
}
