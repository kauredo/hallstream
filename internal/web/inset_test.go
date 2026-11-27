package web

import (
	"testing"

	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

var insetCams = []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2", Label: "Banco"}, {Source: "C", Scene: "Câmara 3"}}

// insetOBS is camOBS with each camera scene also holding the other cameras,
// hidden, as Configurar OBS leaves them.
func insetOBS() *camOBS {
	f := newCamOBS("A", "B", "C")
	for i, scene := range []string{"Câmara 1", "Câmara 2", "Câmara 3"} {
		id := 3
		for k, src := range []string{"A", "B", "C"} {
			if k == i {
				continue
			}
			f.scenes[scene] = append(f.scenes[scene], map[string]any{"sceneItemId": id, "sourceName": src, "inputKind": "dshow_input", "sceneItemEnabled": false,
				"sceneItemTransform": map[string]any{"sourceWidth": 1280.0, "sourceHeight": 720.0, "scaleX": 1.0, "scaleY": 1.0, "alignment": 5.0}})
			id++
		}
	}
	return f
}

// shown lists "scene/source" of each shown item that is not the scene's
// camera or the graphics.
func (f *camOBS) shown() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, scene := range []string{"Câmara 1", "Câmara 2", "Câmara 3"} {
		for _, it := range f.scenes[scene] {
			if id := it["sceneItemId"]; id != 1 && id != 2 && it["sceneItemEnabled"] == true {
				out = append(out, scene+"/"+it["sourceName"].(string))
			}
		}
	}
	return out
}

func setInset(t *testing.T, s *Server, n int) (int, map[string]any) {
	t.Helper()
	var resp map[string]any
	code := call(t, s, "POST", "/api/obs/inset", map[string]any{"camera": n}, &resp)
	return code, resp
}

func insetState(s *Server) (int, *state.Inset) {
	o := s.State.Snapshot().OBS
	return o.InsetCamera, o.Inset
}

func TestInsetRouteRefuses(t *testing.T) {
	s := camServer(t, insetOBS(), insetCams)
	for _, n := range []int{-1, 4} {
		if code, resp := setInset(t, s, n); code != 400 || resp["error"] == nil {
			t.Errorf("camera %d: %d %v", n, code, resp)
		}
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
	if code, _ := setInset(t, s, 2); code != 409 {
		t.Errorf("OBS off: %d", code)
	}
	if n, in := insetState(s); n != 0 || in != nil {
		t.Errorf("state %d %+v", n, in)
	}
}

func TestInsetUnreachable(t *testing.T) {
	s := camServer(t, insetOBS(), insetCams)
	s.programScene("Câmara 1")
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = "127.0.0.1", 1 })
	if code, resp := setInset(t, s, 2); code != 502 || resp["error"] == nil {
		t.Errorf("%d %v", code, resp)
	}
	if n, in := insetState(s); n != 0 || in != nil {
		t.Errorf("state %d %+v", n, in)
	}
}

// The inset shows in the live camera's scene only, bottom-right under the
// default top-left bug and left lower third.
func TestInsetShowsInLiveScene(t *testing.T) {
	f := insetOBS()
	s := camServer(t, f, insetCams)
	s.programScene("Câmara 1")
	s.bg.Wait()
	f.scenes["Câmara 3"][2]["sceneItemEnabled"] = true // left shown by an earlier run
	if code, resp := setInset(t, s, 2); code != 200 || resp["camera"] != 2.0 {
		t.Fatalf("%d %v", code, resp)
	}
	if got := f.shown(); len(got) != 1 || got[0] != "Câmara 1/B" {
		t.Errorf("shown %v", got)
	}
	n, in := insetState(s)
	if want := (state.Inset{Camera: 2, Label: "Banco", Rect: state.Rect{X: 1376, Y: 746, W: 480, H: 270}}); n != 2 || in == nil || *in != want {
		t.Errorf("state %d %+v", n, in)
	}
	if code, _ := setInset(t, s, 0); code != 200 || len(f.shown()) != 0 {
		t.Errorf("off: %d %v", code, f.shown())
	}
	if n, in := insetState(s); n != 0 || in != nil {
		t.Errorf("off: state %d %+v", n, in)
	}
}

// Off marcador and nada the inset hides; back on either it shows again.
func TestInsetFollowsScreen(t *testing.T) {
	f := insetOBS()
	s := camServer(t, f, insetCams)
	s.programScene("Câmara 1")
	setInset(t, s, 3)
	for _, c := range []struct {
		screen string
		shown  bool
	}{{"intervalo", false}, {"nada", true}, {"desconto", false}, {"marcador", true}} {
		call(t, s, "POST", "/api/screen", map[string]any{"screen": c.screen}, nil)
		s.Camera.Stop()
		got := f.shown()
		_, in := insetState(s)
		if c.shown && (len(got) != 1 || got[0] != "Câmara 1/C" || in == nil) || !c.shown && (len(got) != 0 || in != nil) {
			t.Errorf("%s: shown %v state %+v", c.screen, got, in)
		}
	}
	if n, _ := insetState(s); n != 3 {
		t.Errorf("pick lost: %d", n)
	}
}

// A switch moves the inset to the new scene; a switch to the inset's own
// camera hides it, before the switch.
func TestInsetFollowsSwitch(t *testing.T) {
	f := insetOBS()
	s := camServer(t, f, insetCams)
	s.programScene("Câmara 1")
	setInset(t, s, 2)
	if code, resp := switchCamera(t, s, 3); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	if got := f.shown(); len(got) != 1 || got[0] != "Câmara 3/B" {
		t.Errorf("after the switch to 3: %v", got)
	}
	if code, _ := switchCamera(t, s, 2); code != 200 || len(f.shown()) != 0 {
		t.Errorf("the inset camera went live: %v", f.shown())
	}
	if _, in := insetState(s); in != nil {
		t.Errorf("state %+v", in)
	}
	// A switch made in OBS: the camera loop follows it.
	s.programScene("Câmara 2")
	s.programScene("Câmara 1")
	s.followCamera()
	if got := f.shown(); len(got) != 1 || got[0] != "Câmara 1/B" {
		t.Errorf("after OBS switched to 1: %v", got)
	}
}

// The pick follows its camera in a new list, and a removed camera turns it off.
func TestInsetMovesWithList(t *testing.T) {
	s := camServer(t, insetOBS(), insetCams)
	s.inset = 2
	s.insetMoved([]setupCamera{{Choice: "source:B", Was: 2}, {Choice: "source:A", Was: 1}})
	if s.inset != 1 {
		t.Errorf("moved to %d", s.inset)
	}
	s.insetMoved([]setupCamera{{Choice: "source:A", Was: 2}})
	if s.inset != 0 {
		t.Errorf("removed, still %d", s.inset)
	}
}

func TestInsetBox(t *testing.T) {
	for _, c := range []struct {
		bug, lt string
		margin  int
		x, y    float64
	}{
		{"top-left", "left", 64, 1376, 746},
		{"top-right", "center", 64, 1376, 746},
		{"top-center", "right", 32, 32, 778},
		{"bottom-left", "left", 64, 1376, 64},
		{"bottom-right", "left", 96, 96, 96},
		{"bottom-center", "right", 64, 64, 64},
	} {
		r := insetBox(store.Overlay{BugPosition: c.bug, LowerThird: c.lt, Margin: c.margin})
		if r.X != c.x || r.Y != c.y || r.W != 480 || r.H != 270 {
			t.Errorf("%s %s %d: %+v", c.bug, c.lt, c.margin, r)
		}
	}
}
