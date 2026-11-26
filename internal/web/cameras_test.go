package web

import (
	"encoding/json"
	"math"
	"sync"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

// camOBS is an OBS with one scene per camera, each holding its camera
// (1280x720, at full screen) below the Grafismo scene.
type camOBS struct {
	mu      sync.Mutex
	program string
	refuse  bool                        // SetCurrentProgramScene fails
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
	if s.expect != "" || s.program != "Câmara 1" {
		t.Errorf("expect %q program %q", s.expect, s.program)
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
