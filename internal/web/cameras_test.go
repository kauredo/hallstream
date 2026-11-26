package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// camOBS is an OBS with one scene per camera, each holding its camera
// (1280x720, at full screen) below the Grafismo scene.
type camOBS struct {
	mu      sync.Mutex
	program string
	refuse  bool                        // SetCurrentProgramScene fails
	noList  bool                        // GetSceneList fails
	scenes  map[string][]map[string]any // GetSceneItemList by scene
	sent    []camSent                   // transforms and switches, in order
}

type camSent struct {
	switchTo string // the scene of a SetCurrentProgramScene; "" for a transform
	scene    string
	x, scale float64
}

func newCamOBS(sources ...string) *camOBS {
	f := &camOBS{program: "Câmara 1", scenes: map[string][]map[string]any{}}
	for i, src := range sources {
		tr := map[string]any{"positionX": 0.0, "positionY": 0.0, "scaleX": 1.5, "scaleY": 1.5,
			"sourceWidth": 1280.0, "sourceHeight": 720.0, "alignment": 5.0, "boundsType": "OBS_BOUNDS_NONE"}
		f.scenes["Câmara "+string(rune('1'+i))] = []map[string]any{
			{"sceneItemId": 1, "sceneItemIndex": 0, "sourceName": src, "inputKind": "dshow_input", "sceneItemEnabled": true, "sceneItemTransform": tr},
			{"sceneItemId": 2, "sceneItemIndex": 1, "sourceName": "Grafismo", "inputKind": nil, "sceneItemEnabled": true,
				"sceneItemTransform": map[string]any{"sourceWidth": 1920.0, "sourceHeight": 1080.0, "scaleX": 1.0, "scaleY": 1.0, "alignment": 5.0}},
		}
	}
	return f
}

func (f *camOBS) answer(typ string, raw json.RawMessage) (bool, int, any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var d struct {
		Scene     string         `json:"sceneName"`
		ID        int            `json:"sceneItemId"`
		Transform map[string]any `json:"sceneItemTransform"`
	}
	json.Unmarshal(raw, &d)
	switch typ {
	case "GetVideoSettings":
		return true, 100, map[string]any{"baseWidth": 1920, "baseHeight": 1080}
	case "GetCurrentProgramScene":
		return true, 100, map[string]any{"sceneName": f.program}
	case "GetSceneList":
		if f.noList {
			return false, 500, nil
		}
		list := []map[string]string{}
		for name := range f.scenes {
			list = append(list, map[string]string{"sceneName": name})
		}
		return true, 100, map[string]any{"scenes": list}
	case "GetSceneItemList":
		items, ok := f.scenes[d.Scene]
		if !ok {
			return false, 600, nil
		}
		return true, 100, map[string]any{"sceneItems": items}
	case "SetSceneItemTransform":
		for _, it := range f.scenes[d.Scene] {
			if it["sceneItemId"] == d.ID {
				cur := it["sceneItemTransform"].(map[string]any)
				for k, v := range d.Transform {
					cur[k] = v
				}
			}
		}
		x, _ := d.Transform["positionX"].(float64)
		sc, _ := d.Transform["scaleX"].(float64)
		f.sent = append(f.sent, camSent{scene: d.Scene, x: x, scale: sc})
		return true, 100, nil
	case "SetCurrentProgramScene":
		if f.refuse {
			return false, 600, nil
		}
		f.program = d.Scene
		f.sent = append(f.sent, camSent{switchTo: d.Scene})
		return true, 100, nil
	}
	return true, 100, nil
}

func (f *camOBS) log() []camSent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]camSent{}, f.sent...)
}

func (f *camOBS) clear() {
	f.mu.Lock()
	f.sent = nil
	f.mu.Unlock()
}

func (f *camOBS) scene() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.program
}

func camServer(t *testing.T, f *camOBS, cams []store.Camera) *Server {
	t.Helper()
	s := newServer(t)
	host, port := fakeOBSData(t, "", f.answer)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true
		set.OBS.CameraSource, set.OBS.Cameras, set.OBS.GraphicsScene = cams[0].Source, cams, "Grafismo"
	})
	return s
}

var twoCams = []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}}

const (
	fullX, fullScale = 0.0, 1.5     // 1280x720 on 1920x1080
	winX, winScale   = 416.0, 1.125 // in the 1440x810 window at 416,64
)

func near2(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// beforeSwitch is the last transform sent before the first switch.
func beforeSwitch(t *testing.T, sent []camSent) camSent {
	t.Helper()
	var last *camSent
	for i := range sent {
		if sent[i].switchTo != "" {
			if last == nil {
				t.Fatalf("no transform before the switch: %+v", sent)
			}
			return *last
		}
		last = &sent[i]
	}
	t.Fatalf("no switch: %+v", sent)
	return camSent{}
}

func switchCamera(t *testing.T, s *Server, index int) (int, map[string]any) {
	t.Helper()
	var resp map[string]any
	code := call(t, s, "POST", "/api/obs/camera/live", map[string]any{"index": index}, &resp)
	return code, resp
}

func TestCameraLiveSwitches(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	code, resp := switchCamera(t, s, 2)
	if code != 200 || resp["live_camera"] != 2.0 || resp["warning"] != "" {
		t.Fatalf("%d %v", code, resp)
	}
	if p := f.scene(); p != "Câmara 2" {
		t.Errorf("program %q", p)
	}
	if b := beforeSwitch(t, f.log()); b.scene != "Câmara 2" || !near2(b.x, fullX) || !near2(b.scale, fullScale) {
		t.Errorf("placed %+v", b)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
}

func TestCameraLiveDuringTimeout(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, nil)
	s.Camera.Stop()
	f.clear()
	if code, resp := switchCamera(t, s, 2); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	if b := beforeSwitch(t, f.log()); b.scene != "Câmara 2" || !near2(b.x, winX) || !near2(b.scale, winScale) {
		t.Errorf("placed %+v", b)
	}
	f.clear()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "marcador"}, nil)
	s.Camera.Stop()
	sent := f.log()
	if len(sent) == 0 {
		t.Fatal("no tween on marcador")
	}
	for _, m := range sent {
		if m.scene != "Câmara 2" {
			t.Fatalf("tween sent to %+v", m)
		}
	}
	if end := sent[len(sent)-1]; !near2(end.x, fullX) || !near2(end.scale, fullScale) {
		t.Errorf("ended at %+v", end)
	}
}

func TestCameraLiveRefused(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B"}})
	for _, c := range []struct {
		index, code int
	}{{0, 400}, {3, 400}, {2, 409}} {
		if code, resp := switchCamera(t, s, c.index); code != c.code || resp["error"] == nil {
			t.Errorf("camera %d: %d %v", c.index, code, resp)
		}
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
	if code, _ := switchCamera(t, s, 1); code != 409 {
		t.Errorf("OBS off: %d", code)
	}
	for _, m := range f.log() {
		if m.switchTo != "" {
			t.Errorf("switched to %q", m.switchTo)
		}
	}
}

func TestCameraLiveOldModeTimeout(t *testing.T) {
	f := newCamOBS("A", "B")
	f.scenes["Desconto"] = nil
	s := camServer(t, f, twoCams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.TimeoutScene, set.OBS.GameScene = "Desconto", "Câmara 1" })
	s.State.SetScreen("desconto", false)
	if code, _ := switchCamera(t, s, 2); code != 409 {
		t.Errorf("during the time-out: %d", code)
	}
	// OBS can't say whether its time-out scene is there: no switch either.
	f.mu.Lock()
	f.noList = true
	f.mu.Unlock()
	if code, _ := switchCamera(t, s, 2); code != 502 {
		t.Errorf("scene list refused: %d", code)
	}
	for _, m := range f.log() {
		if m.switchTo != "" {
			t.Fatalf("switched to %q during the time-out", m.switchTo)
		}
	}
	f.mu.Lock()
	f.noList = false
	f.mu.Unlock()
	s.State.SetScreen("marcador", false)
	if code, resp := switchCamera(t, s, 2); code != 200 {
		t.Errorf("after: %d %v", code, resp)
	}
}

func TestCameraLiveOBSRefuses(t *testing.T) {
	f := newCamOBS("A", "B")
	f.refuse = true
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	code, resp := switchCamera(t, s, 2)
	if code != 502 || resp["error"] == nil {
		t.Errorf("%d %v", code, resp)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if len(s.expect) != 0 || s.program != "Câmara 1" {
		t.Errorf("expect %v program %q", s.expect, s.program)
	}
}

func TestProgramSceneMapping(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false }) // nothing to place
	for _, c := range []struct {
		scene string
		live  int
	}{{"Câmara 2", 2}, {"Outra", 0}, {"Hallstream", 0}, {"Câmara 1", 1}} {
		s.programScene(c.scene)
		if n := s.State.Snapshot().OBS.LiveCamera; n != c.live {
			t.Errorf("%q: live camera %d", c.scene, n)
		}
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = twoCams[:1] })
	s.programScene("Hallstream")
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("one camera, Hallstream: live camera %d", n)
	}
}

// With OBS control on, a scene change from OBS places the camera only for a
// camera's own scene and with the camera able to move.
func TestProgramScenePlacing(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams[:1])
	s.programScene("Hallstream") // the old scene: the camera follows the program scene
	s.bg.Wait()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("old scene: sent %+v", sent)
	}

	s = camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	f.clear()
	s.programScene("Câmara 2") // a click in OBS on marcador
	s.bg.Wait()
	if sent := f.log(); len(sent) != 1 || sent[0].scene != "Câmara 2" || !near2(sent[0].x, fullX) || !near2(sent[0].scale, fullScale) {
		t.Errorf("click on marcador: sent %+v", sent)
	}

	f.clear()
	s.camErr = errors.New("no window")
	s.programScene("Câmara 1")
	s.bg.Wait()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("camErr: sent %+v", sent)
	}
}

func TestProgramSceneFromOBSPlaces(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, nil)
	s.Camera.Stop()
	f.clear()
	s.programScene("Câmara 2") // a click in OBS
	s.bg.Wait()
	sent := f.log()
	if len(sent) != 1 || sent[0].scene != "Câmara 2" || !near2(sent[0].x, winX) {
		t.Errorf("placed %+v", sent)
	}
	switchCamera(t, s, 1)
	f.clear()
	s.programScene("Câmara 1") // the event for the panel's own switch
	s.bg.Wait()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("the panel's switch placed again: %+v", sent)
	}
}

// A click in OBS that the watch saw makes that camera the one the squeeze
// moves, whatever GetCurrentProgramScene says.
func TestSqueezeFollowsTheWatchedScene(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	s.programScene("Câmara 2") // the fake's program stays Câmara 1
	s.bg.Wait()
	f.clear()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, nil)
	s.Camera.Stop()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "marcador"}, nil)
	s.Camera.Stop()
	sent := f.log()
	if len(sent) == 0 {
		t.Fatal("no squeeze")
	}
	for _, m := range sent {
		if m.scene != "Câmara 2" {
			t.Fatalf("moved %+v, not camera 2", m)
		}
	}
}

// A camera that can't be placed still goes on air, with the reason.
func TestCameraLivePlaceFails(t *testing.T) {
	f := newCamOBS("A", "B")
	f.scenes["Câmara 2"] = f.scenes["Câmara 2"][1:] // B is gone from its scene
	s := camServer(t, f, twoCams)
	code, resp := switchCamera(t, s, 2)
	if code != 200 || resp["warning"] == "" || f.scene() != "Câmara 2" {
		t.Errorf("%d %v, program %q", code, resp, f.scene())
	}
}

// The camera on air is where the screen has it: a switch to it moves nothing.
func TestCameraLiveOnAirDuringTimeout(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, nil)
	s.Camera.Stop()
	f.clear()
	if code, resp := switchCamera(t, s, 1); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	for _, m := range f.log() {
		if m.switchTo == "" {
			t.Errorf("transform %+v", m)
		}
	}
}

// Without the time-out window the camera never moves, and switches still work.
func TestCameraLiveWithoutWindow(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.camErr = errors.New("no window")
	if code, resp := switchCamera(t, s, 2); code != 200 || f.scene() != "Câmara 2" {
		t.Fatalf("%d %v", code, resp)
	}
	for _, m := range f.log() {
		if m.switchTo == "" {
			t.Errorf("transform %+v", m)
		}
	}
}

// Camera 1 saved before camera_fulls keeps its placement (camera_full).
func TestCameraLiveOldFull(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	old := &obsws.Full{Place: obsws.Place{Rect: obsws.Rect{X: 10, Y: 0, W: 1900, H: 1068.75}},
		SourceWidth: 1280, SourceHeight: 720, CanvasWidth: 1920, CanvasHeight: 1080, Fit: "contain"}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraFull, set.OBS.CameraFulls = old, nil })
	s.programScene("Câmara 2")
	s.bg.Wait()
	f.clear()
	if code, resp := switchCamera(t, s, 1); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	if b := beforeSwitch(t, f.log()); b.scene != "Câmara 1" || !near2(b.x, 10) {
		t.Errorf("placed %+v, not camera_full", b)
	}
}

func TestCameraLiveBadIndex(t *testing.T) {
	f := newCamOBS("A")
	s := camServer(t, f, []store.Camera{{Source: "A"}})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = nil })
	for _, n := range []int{-1, 2} {
		if code, resp := switchCamera(t, s, n); code != 400 || resp["error"] == nil {
			t.Errorf("index %d: %d %v", n, code, resp)
		}
	}
}

// Two quick switches: the first one's late event neither counts as a click
// in OBS nor puts the first camera back as the program.
func TestCameraLiveQuickSwitches(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	switchCamera(t, s, 2)
	switchCamera(t, s, 1)
	f.clear()
	s.programScene("Câmara 2")
	s.bg.Wait()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("after the first event: live camera %d", n)
	}
	s.programScene("Câmara 1")
	s.bg.Wait()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("the panel's switches placed again: %+v", sent)
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if len(s.expect) != 0 || s.program != "Câmara 1" {
		t.Errorf("expect %v program %q", s.expect, s.program)
	}
}

// The watch's answers place a camera only when the program changed, and
// wait while a switch from the panel is on its way.
func TestSceneAnswers(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.sceneSeen("Câmara 1", false)
	s.bg.Wait()
	f.clear()
	s.sceneSeen("Câmara 1", false)
	s.bg.Wait()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("same scene placed: %+v", sent)
	}
	switchCamera(t, s, 2)
	f.clear()
	s.sceneSeen("Câmara 1", false) // asked before the switch
	s.bg.Wait()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 || len(f.log()) != 0 {
		t.Errorf("an answer during the switch: live camera %d, sent %+v", n, f.log())
	}
	s.sceneSeen("Renamed", false) // the scene renamed in OBS, once the event came
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("answer before the event: live camera %d", n)
	}
	s.programScene("Câmara 2")
	s.sceneSeen("Renamed", false)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 0 {
		t.Errorf("renamed: live camera %d", n)
	}
}

// Settings that change with no scene event still move obs.live_camera.
func TestLiveAgainAfterSettings(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
	s.programScene("Câmara 2")
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Cameras = []store.Camera{twoCams[1], twoCams[0]}
	})
	s.liveAgain()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("reordered: live camera %d", n)
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = twoCams[:1] })
	s.liveAgain()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 0 {
		t.Errorf("removed: live camera %d", n)
	}
}

// Switches sent at once run one by one: each placement is followed by its
// own switch, and the last one is what OBS shows.
func TestCameraLiveConcurrent(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/obs/camera/live", strings.NewReader(fmt.Sprintf(`{"index":%d}`, i%2+1)))
			r.RemoteAddr, r.Host = "127.0.0.1:5000", "127.0.0.1:8765"
			r.Header.Set("Content-Type", "application/json")
			s.ServeHTTP(httptest.NewRecorder(), r)
		}()
	}
	wg.Wait()
	s.bg.Wait()
	var pending []camSent
	for _, m := range f.log() {
		if m.switchTo == "" {
			pending = append(pending, m)
			continue
		}
		for _, p := range pending {
			if p.scene != m.switchTo {
				t.Errorf("placed %q, then switched to %q", p.scene, m.switchTo)
			}
		}
		pending = nil
	}
	want := map[string]int{"Câmara 1": 1, "Câmara 2": 2}[f.scene()]
	if n := s.State.Snapshot().OBS.LiveCamera; n != want {
		t.Errorf("live camera %d, OBS shows %q", n, f.scene())
	}
}

// The switch goes through the same gate as every route.
func TestCameraLiveGate(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.PhoneAccess, set.PIN = true, "1234" })
	for name, c := range map[string]struct {
		mod  func(r *http.Request)
		want int
	}{
		"other origin":  {func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, 403},
		"form post":     {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		"phone, no PIN": {func(r *http.Request) { r.RemoteAddr = "192.168.1.20:5000" }, 401},
	} {
		r := httptest.NewRequest("POST", "/api/obs/camera/live", strings.NewReader(`{"index":2}`))
		r.RemoteAddr, r.Host = "127.0.0.1:5000", "127.0.0.1:8765"
		r.Header.Set("Content-Type", "application/json")
		c.mod(r)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: %d, want %d", name, w.Code, c.want)
		}
	}
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("sent %+v", sent)
	}
}

// The keys the panel reads: the reply's and obs.live_camera in the state.
func TestCameraLiveKeys(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	_, resp := switchCamera(t, s, 2)
	keys := slices.Sorted(maps.Keys(resp))
	if !slices.Equal(keys, []string{"live_camera", "ok", "warning"}) || resp["ok"] != true || resp["live_camera"] != 2.0 {
		t.Errorf("reply %v", resp)
	}
	var st struct {
		OBS map[string]any `json:"obs"`
	}
	call(t, s, "GET", "/api/state", nil, &st)
	if st.OBS["live_camera"] != 2.0 {
		t.Errorf("state obs %v", st.OBS)
	}
}

// fakeWatch puts body in as the scene watch.
func fakeWatch(s *Server, body func(stop <-chan struct{}, scene func(string, bool)) error) *int {
	calls := 0
	s.watch = func(_ obsws.Config, stop <-chan struct{}, scene func(string, bool)) error {
		calls++
		return body(stop, scene)
	}
	return &calls
}

func shortScenes(t *testing.T) {
	p, st := scenesPoll, scenesSteady
	scenesPoll = 10 * time.Millisecond
	t.Cleanup(func() { scenesPoll, scenesSteady = p, st })
}

func TestScenesDropResetsLive(t *testing.T) {
	shortScenes(t)
	s := camServer(t, newCamOBS("A", "B"), twoCams)
	during := 0
	fakeWatch(s, func(_ <-chan struct{}, scene func(string, bool)) error {
		scene("Câmara 2", false)
		during = s.State.Snapshot().OBS.LiveCamera
		return errors.New("gone")
	})
	pause, next := s.scenesOnce(scenesWait)
	if during != 2 || s.State.Snapshot().OBS.LiveCamera != 0 {
		t.Errorf("live camera %d during, %d after", during, s.State.Snapshot().OBS.LiveCamera)
	}
	if pause != scenesWait || next != 2*scenesWait {
		t.Errorf("pause %v, next %v", pause, next)
	}
}

func TestScenesDisablingStops(t *testing.T) {
	shortScenes(t)
	s := camServer(t, newCamOBS("A", "B"), twoCams)
	fakeWatch(s, func(stop <-chan struct{}, _ func(string, bool)) error {
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
		select {
		case <-stop:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("never stopped")
		}
	})
	if pause, _ := s.scenesOnce(scenesWait); pause != 0 {
		t.Errorf("pause %v", pause)
	}
}

func TestScenesBackoff(t *testing.T) {
	shortScenes(t)
	s := camServer(t, newCamOBS("A", "B"), twoCams)
	fakeWatch(s, func(<-chan struct{}, func(string, bool)) error { return errors.New("no OBS") })
	var pauses []time.Duration
	wait := scenesWait
	for range 6 {
		var pause time.Duration
		pause, wait = s.scenesOnce(wait)
		pauses = append(pauses, pause)
	}
	if want := []time.Duration{1, 2, 4, 8, 10, 10}; !slices.Equal(pauses, scaled(want, time.Second)) {
		t.Errorf("pauses %v", pauses)
	}
	scenesSteady = -1 // the next watch counts as one that lasted
	if pause, _ := s.scenesOnce(wait); pause != scenesWait {
		t.Errorf("after a steady watch: pause %v", pause)
	}
}

func scaled(n []time.Duration, unit time.Duration) []time.Duration {
	out := make([]time.Duration, len(n))
	for i, v := range n {
		out[i] = v * unit
	}
	return out
}

func TestScenesDisabledNeverWatches(t *testing.T) {
	shortScenes(t)
	s := camServer(t, newCamOBS("A", "B"), twoCams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
	s.State.SetLiveCamera(2, false)
	calls := fakeWatch(s, func(<-chan struct{}, func(string, bool)) error { return nil })
	if pause, _ := s.scenesOnce(scenesWait); pause != scenesPoll || *calls != 0 {
		t.Errorf("pause %v, %d watches", pause, *calls)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 0 {
		t.Errorf("live camera %d", n)
	}
}

// A switch whose event never comes stops holding the program once its
// deadline passes: the next answer from the watch counts again.
func TestCameraLiveEventNeverComes(t *testing.T) {
	f := newCamOBS("A", "B")
	s := camServer(t, f, twoCams)
	old := expectFor
	expectFor = 50 * time.Millisecond
	t.Cleanup(func() { expectFor = old })
	s.programScene("Câmara 1")
	s.bg.Wait()
	switchCamera(t, s, 2)
	s.sceneSeen("Câmara 1", false) // within the deadline: ignored
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Fatalf("answer during the switch: live camera %d", n)
	}
	time.Sleep(100 * time.Millisecond)
	s.sceneSeen("Câmara 1", false)
	s.bg.Wait()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("after the deadline: live camera %d", n)
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if len(s.expect) != 0 {
		t.Errorf("expect %v", s.expect)
	}
}

var threeCams = []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}, {Source: "C", Scene: "Câmara 3"}}

// timeoutServer has camera 1 live and camera tcam for the time-out.
func timeoutServer(t *testing.T, f *camOBS, cams []store.Camera, tcam int) *Server {
	t.Helper()
	s := camServer(t, f, cams)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.TimeoutCamera = tcam })
	s.programScene("Câmara 1")
	s.bg.Wait()
	f.clear()
	return s
}

// screenTo changes the screen and lets the squeeze end.
func screenTo(t *testing.T, s *Server, screen string) map[string]any {
	t.Helper()
	var resp map[string]any
	if code := call(t, s, "POST", "/api/screen", map[string]any{"screen": screen}, &resp); code != 200 {
		t.Fatalf("%s: %d %v", screen, code, resp)
	}
	s.Camera.Stop()
	return resp
}

func switches(sent []camSent) []string {
	var to []string
	for _, m := range sent {
		if m.switchTo != "" {
			to = append(to, m.switchTo)
		}
	}
	return to
}

// tweenOn checks that every transform went to scene and the last one ended at x, scale.
func tweenOn(t *testing.T, sent []camSent, scene string, x, scale float64) {
	t.Helper()
	if len(sent) == 0 {
		t.Fatal("no tween")
	}
	for _, m := range sent {
		if m.switchTo != "" || m.scene != scene {
			t.Fatalf("want only transforms on %q: %+v", scene, sent)
		}
	}
	if end := sent[len(sent)-1]; !near2(end.x, x) || !near2(end.scale, scale) {
		t.Errorf("ended at %+v", end)
	}
}

// placedSwitched checks a placing of scene's camera at x, scale, the switch
// to scene, then a tween on scene ending at endX, endScale.
func placedSwitched(t *testing.T, sent []camSent, scene string, x, scale, endX, endScale float64) {
	t.Helper()
	if len(sent) < 3 {
		t.Fatalf("too few: %+v", sent)
	}
	if p := sent[0]; p.switchTo != "" || p.scene != scene || !near2(p.x, x) || !near2(p.scale, scale) {
		t.Errorf("placed %+v", p)
	}
	if sent[1].switchTo != scene {
		t.Errorf("switched %+v", sent[1])
	}
	tweenOn(t, sent[2:], scene, endX, endScale)
}

func TestTimeoutCameraInAndOut(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	if resp := screenTo(t, s, "desconto"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	placedSwitched(t, f.log(), "Câmara 2", fullX, fullScale, winX, winScale)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
	f.clear()
	if resp := screenTo(t, s, "marcador"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	placedSwitched(t, f.log(), "Câmara 1", winX, winScale, fullX, fullScale)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
}

func TestTimeoutCameraOff(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 0)
	screenTo(t, s, "desconto")
	tweenOn(t, f.log(), "Câmara 1", winX, winScale)
	f.clear()
	screenTo(t, s, "marcador")
	tweenOn(t, f.log(), "Câmara 1", fullX, fullScale)
}

func TestTimeoutCameraSkipped(t *testing.T) {
	noScene := []store.Camera{threeCams[0], {Source: "B"}, threeCams[2]}
	for _, c := range []struct {
		name    string
		cams    []store.Camera
		tcam    int
		program string // "" keeps Câmara 1
		legacy  bool
	}{
		{name: "out of range", cams: twoCams, tcam: 3},
		{name: "no scene", cams: noScene, tcam: 2},
		{name: "already live", cams: threeCams, tcam: 1},
		{name: "live camera unknown", cams: threeCams, tcam: 2, program: "Outra"},
		{name: "old two-scene mode", cams: threeCams, tcam: 2, legacy: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newCamOBS("A", "B", "C")
			s := timeoutServer(t, f, c.cams, c.tcam)
			if c.program != "" {
				s.programScene(c.program)
				s.bg.Wait()
			}
			if c.legacy {
				f.scenes["Desconto"] = nil
				s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.TimeoutScene, set.OBS.GameScene = "Desconto", "Câmara 1" })
			}
			screenTo(t, s, "desconto")
			screenTo(t, s, "marcador")
			want := []string(nil)
			if c.legacy {
				want = []string{"Desconto", "Câmara 1"} // the old mode's own switch
			}
			if got := switches(f.log()); !slices.Equal(got, want) {
				t.Errorf("switches %v", got)
			}
			for _, m := range f.log() {
				if m.switchTo == "" && m.scene != "Câmara 1" {
					t.Errorf("moved %+v", m)
				}
			}
		})
	}
}

func TestTimeoutCameraManualSwitchWins(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	if code, resp := switchCamera(t, s, 3); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	f.clear()
	screenTo(t, s, "marcador")
	tweenOn(t, f.log(), "Câmara 3", fullX, fullScale)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 3 {
		t.Errorf("live camera %d", n)
	}
}

func TestTimeoutCameraOBSSwitchForgets(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	s.programScene("Câmara 3") // a click in OBS
	s.bg.Wait()
	f.clear()
	screenTo(t, s, "marcador")
	tweenOn(t, f.log(), "Câmara 3", fullX, fullScale)
}

func TestTimeoutCameraRefused(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	f.refuse = true
	s := timeoutServer(t, f, threeCams, 2)
	resp := screenTo(t, s, "desconto")
	msg, _ := resp["obs_error"].(string)
	if want := i18n.T("log.camara.desconto_falhou", "err", ""); !strings.HasPrefix(msg, want) || len(msg) == len(want) {
		t.Errorf("obs_error %q", msg)
	}
	if cur := s.State.Snapshot().Screen.Current; cur != "desconto" {
		t.Errorf("screen %q", cur)
	}
	sent := f.log()
	if len(sent) < 2 || sent[0].scene != "Câmara 2" {
		t.Fatalf("want camera 2 placed first: %+v", sent)
	}
	tweenOn(t, sent[1:], "Câmara 1", winX, winScale)
	f.clear()
	f.refuse = false
	screenTo(t, s, "marcador")
	tweenOn(t, f.log(), "Câmara 1", fullX, fullScale)
}

func TestTimeoutCameraPromo(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	s.State.ShowPromo(state.PromoClip{ID: "a", URL: "/promo/a.mp4"})
	s.followCamera() // as RunCamera does on the change
	s.Camera.Stop()
	screenTo(t, s, "desconto")
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 2", "Câmara 1", "Câmara 2"}) {
		t.Errorf("switches %v", got)
	}
}

// RunCamera and POST /api/screen both follow the same change.
func TestTimeoutCameraFollowedTwice(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	s.State.SetScreen("desconto", false)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); s.followCamera() }()
	}
	wg.Wait()
	s.Camera.Stop()
	placedSwitched(t, f.log(), "Câmara 2", fullX, fullScale, winX, winScale)
}

// wantError checks that msg holds want.
func wantError(t *testing.T, msg any, want string) {
	t.Helper()
	if s, _ := msg.(string); !strings.Contains(s, want) {
		t.Errorf("error %v, want %q", msg, want)
	}
}

// backToOne leaves the time-out and checks camera 1 came back, in the window first.
func backToOne(t *testing.T, s *Server, f *camOBS) {
	t.Helper()
	f.clear()
	if resp := screenTo(t, s, "marcador"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	placedSwitched(t, f.log(), "Câmara 1", winX, winScale, fullX, fullScale)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
}

func TestTimeoutCameraEventThenBack(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	s.programScene("Câmara 2") // the event for the time-out switch
	s.bg.Wait()
	backToOne(t, s, f)
}

// OBS reports each switch about 315 ms late, so a quick toggle's events
// arrive after all three switches.
func TestTimeoutCameraLateEventsQuickToggle(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	screenTo(t, s, "marcador")
	screenTo(t, s, "desconto")
	for _, scene := range []string{"Câmara 2", "Câmara 1", "Câmara 2"} {
		s.programScene(scene)
	}
	s.bg.Wait()
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
	backToOne(t, s, f)
}

// A transition longer than expectFor: the event names the scene the program
// put on air, so it is no switch made in OBS.
func TestTimeoutCameraEventAfterDeadline(t *testing.T) {
	old := expectFor
	expectFor = time.Millisecond
	t.Cleanup(func() { expectFor = old })
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	time.Sleep(5 * time.Millisecond)
	s.programScene("Câmara 2")
	s.bg.Wait()
	backToOne(t, s, f)
}

func TestTimeoutCameraManualSwitchThenDesconto(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	if code, resp := switchCamera(t, s, 3); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	screenTo(t, s, "desconto") // before OBS reports the manual switch
	s.programScene("Câmara 3")
	s.programScene("Câmara 2")
	s.bg.Wait()
	f.clear()
	screenTo(t, s, "marcador")
	placedSwitched(t, f.log(), "Câmara 3", winX, winScale, fullX, fullScale)
}

func TestTimeoutCameraReconnectKeeps(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	calls := fakeWatch(s, func(_ <-chan struct{}, scene func(string, bool)) error {
		return errors.New("gone") // the connection drops, and the pending switch with it
	})
	s.scenesOnce(scenesWait)
	fakeWatch(s, func(_ <-chan struct{}, scene func(string, bool)) error {
		scene("Câmara 2", false) // the first report after connecting again
		scene("Câmara 2", true)  // a change event naming the scene the program put on air
		return errors.New("gone")
	})
	s.scenesOnce(scenesWait)
	s.bg.Wait() // the report placed camera 2, in the window, as for any report
	if *calls != 1 {
		t.Fatalf("first watch ran %d times", *calls)
	}
	backToOne(t, s, f)
}

func TestTimeoutCameraSettingsChanged(t *testing.T) {
	t.Run("camera list shrunk", func(t *testing.T) {
		f := newCamOBS("A", "B", "C")
		s := timeoutServer(t, f, threeCams, 2)
		s.programScene("Câmara 3")
		s.bg.Wait()
		screenTo(t, s, "desconto")
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = twoCams })
		f.clear()
		resp := screenTo(t, s, "marcador")
		wantError(t, resp["obs_error"], i18n.T("log.camara.desconto_sem_volta", "n", 3))
		tweenOn(t, f.log(), "Câmara 2", fullX, fullScale)
	})
	t.Run("scene cleared", func(t *testing.T) {
		f := newCamOBS("A", "B", "C")
		s := timeoutServer(t, f, threeCams, 2)
		screenTo(t, s, "desconto")
		s.Store.UpdateSettings(func(set *store.Settings) {
			set.OBS.Cameras = []store.Camera{{Source: "A"}, threeCams[1], threeCams[2]}
		})
		f.clear()
		resp := screenTo(t, s, "marcador")
		wantError(t, resp["obs_error"], i18n.T("log.camara.desconto_sem_volta", "n", 1))
		tweenOn(t, f.log(), "Câmara 2", fullX, fullScale)
	})
	t.Run("timeout_camera changed", func(t *testing.T) {
		f := newCamOBS("A", "B", "C")
		s := timeoutServer(t, f, threeCams, 2)
		screenTo(t, s, "desconto")
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.TimeoutCamera = 3 })
		backToOne(t, s, f)
	})
	t.Run("OBS control off", func(t *testing.T) {
		f := newCamOBS("A", "B", "C")
		s := timeoutServer(t, f, threeCams, 2)
		screenTo(t, s, "desconto")
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
		f.clear()
		s.State.SetScreen("marcador", false)
		err := s.followCamera() // as RunCamera does; the API follows only with OBS on
		s.Camera.Stop()
		var msg any
		if err != nil {
			msg = err.Error()
		}
		wantError(t, msg, i18n.T("log.camara.desconto_obs_desligado", "n", 1))
		if sent := f.log(); len(sent) != 0 {
			t.Errorf("sent %+v", sent)
		}
	})
}

// A follow that waited for switchMu acts on the screen as it is then, not
// on the one it was woken for.
func TestTimeoutCameraScreenChangesWhileWaiting(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	s.switchMu.Lock()
	s.State.SetScreen("desconto", false)
	done := make(chan struct{})
	go func() { s.followCamera(); close(done) }()
	time.Sleep(50 * time.Millisecond)
	s.State.SetScreen("marcador", false)
	s.switchMu.Unlock()
	<-done
	s.Camera.Stop()
	if sent := f.log(); len(sent) != 0 {
		t.Errorf("acted on a screen already left: %+v", sent)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
}

func TestTimeoutCameraBackRefused(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	f.clear()
	f.refuse = true
	resp := screenTo(t, s, "marcador")
	msg, _ := resp["obs_error"].(string)
	if want := i18n.T("log.camara.desconto_falhou", "err", ""); !strings.HasPrefix(msg, want) {
		t.Errorf("obs_error %q", msg)
	}
	sent := f.log()
	if len(sent) < 2 || sent[0].scene != "Câmara 1" {
		t.Fatalf("want camera 1 placed first: %+v", sent)
	}
	tweenOn(t, sent[1:], "Câmara 2", fullX, fullScale)
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
}

func TestTimeoutCameraManualRefusedKeeps(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	screenTo(t, s, "desconto")
	f.refuse = true
	if code, resp := switchCamera(t, s, 3); code != 502 {
		t.Fatalf("%d %v", code, resp)
	}
	f.refuse = false
	backToOne(t, s, f)
}

func TestTimeoutCameraLiveUnknownThenReported(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := timeoutServer(t, f, threeCams, 2)
	s.programScene("Outra")
	s.bg.Wait()
	f.clear()
	resp := screenTo(t, s, "desconto")
	wantError(t, resp["obs_error"], i18n.T("log.camara.desconto_sem_live", "n", 2))
	s.followCamera() // a state change while OBS still has not said
	if got := switches(f.log()); got != nil {
		t.Fatalf("switched with the live camera unknown: %v", got)
	}
	s.programScene("Câmara 1")
	s.bg.Wait()
	if err := s.followCamera(); err != nil { // as RunCamera does on obs.live_camera
		t.Fatal(err)
	}
	s.Camera.Stop()
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 2"}) {
		t.Fatalf("switches %v", got)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
	backToOne(t, s, f)
}

// nadaServer has camera 1 live, camera ncam on nada and tcam on desconto.
func nadaServer(t *testing.T, f *camOBS, ncam, tcam int) *Server {
	t.Helper()
	s := timeoutServer(t, f, threeCams, tcam)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.ScreenCameras = map[string]int{"nada": ncam, "desconto": tcam} })
	return s
}

func TestNadaCameraInAndOut(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 0)
	if resp := screenTo(t, s, "nada"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 3"}) {
		t.Errorf("into nada: %v", got)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 3 {
		t.Errorf("live camera %d", n)
	}
	f.clear()
	if resp := screenTo(t, s, "marcador"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 1"}) {
		t.Errorf("out of nada: %v", got)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
}

// nada → desconto → marcador goes back to the camera live before nada.
func TestNadaThenTimeoutCamera(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 2)
	screenTo(t, s, "nada")
	f.clear()
	if resp := screenTo(t, s, "desconto"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	placedSwitched(t, f.log(), "Câmara 2", fullX, fullScale, winX, winScale)
	backToOne(t, s, f)
}

func TestNadaCameraManualSwitchWins(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 0)
	screenTo(t, s, "nada")
	if code, resp := switchCamera(t, s, 2); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); got != nil {
		t.Errorf("switched back: %v", got)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 2 {
		t.Errorf("live camera %d", n)
	}
}

func remembered(s *Server) int {
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	return s.returnTo
}

func TestNadaCameraLiveUnknownThenReported(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 0)
	s.programScene("Outra")
	s.bg.Wait()
	f.clear()
	resp := screenTo(t, s, "nada")
	wantError(t, resp["obs_error"], i18n.T("log.camara.nada_sem_live", "n", 3))
	s.programScene("Câmara 1")
	s.bg.Wait()
	if err := s.followCamera(); err != nil {
		t.Fatal(err)
	}
	s.Camera.Stop()
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 3"}) {
		t.Fatalf("switches %v", got)
	}
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 1"}) {
		t.Errorf("out of nada: %v", got)
	}
}

func TestNadaCameraRefusedForgets(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	f.refuse = true
	s := nadaServer(t, f, 3, 0)
	resp := screenTo(t, s, "nada")
	wantError(t, resp["obs_error"], i18n.T("log.camara.nada_falhou", "err", ""))
	if n := remembered(s); n != 0 {
		t.Errorf("remembered camera %d after a refused switch", n)
	}
	f.refuse = false
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); got != nil {
		t.Errorf("switched back: %v", got)
	}
}

func TestNadaCameraAlreadyLive(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 1, 0)
	screenTo(t, s, "nada")
	if got := switches(f.log()); got != nil {
		t.Errorf("into nada: %v", got)
	}
	if n := remembered(s); n != 0 {
		t.Errorf("remembered camera %d", n)
	}
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); got != nil {
		t.Errorf("out of nada: %v", got)
	}
}

// desconto without a camera of its own puts back the camera live before nada.
func TestNadaThenDescontoWithoutCamera(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 0)
	screenTo(t, s, "nada")
	f.clear()
	if resp := screenTo(t, s, "desconto"); resp["obs_error"] != nil {
		t.Fatal(resp)
	}
	if got := switches(f.log()); !slices.Equal(got, []string{"Câmara 1"}) {
		t.Errorf("into desconto: %v", got)
	}
	if n := s.State.Snapshot().OBS.LiveCamera; n != 1 {
		t.Errorf("live camera %d", n)
	}
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); got != nil {
		t.Errorf("out of desconto: %v", got)
	}
}

// A reordered list forgets the camera to put back: its index is the old list's.
func TestNadaCameraListReordered(t *testing.T) {
	f := newCamOBS("A", "B", "C")
	s := nadaServer(t, f, 3, 0)
	screenTo(t, s, "nada")
	if _, err := s.listCameras([]setupCamera{{Choice: "source:B", Was: 2}, {Choice: "source:A", Was: 1}, {Choice: "source:C", Was: 3}}); err != nil {
		t.Fatal(err)
	}
	f.clear()
	screenTo(t, s, "marcador")
	if got := switches(f.log()); got != nil {
		t.Errorf("switched back to %v", got)
	}
}
