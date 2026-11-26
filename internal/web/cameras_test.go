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

	"github.com/kauredo/hallstream/internal/obsws"
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
	s.State.SetLiveCamera(2)
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
