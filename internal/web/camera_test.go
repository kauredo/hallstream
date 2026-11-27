package web

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// The camera and the overlay read the time-out window from the same file.
func TestSqueezeTokens(t *testing.T) {
	tok, err := readSqueeze(string(overlayCSS(t)))
	if err != nil {
		t.Fatal(err)
	}
	if tok.win != (obsws.Rect{X: 416, Y: 64, W: 1440, H: 810}) || tok.dur != 900*time.Millisecond {
		t.Errorf("window %v, %v", tok.win, tok.dur)
	}
	if e := tok.ease(0.5); e < 0.49 || e > 0.51 || tok.ease(0.2) > 0.1 {
		t.Errorf("ease %v %v", tok.ease(0.5), tok.ease(0.2))
	}
	if _, err := readSqueeze("--win-x: 1px;"); err == nil {
		t.Error("want an error for a CSS without the tokens")
	}
}

func TestSettingsKeepCamera(t *testing.T) {
	s := newServer(t)
	full := &obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraFull = full })

	set := s.Store.Settings()
	set.OBS.CameraFull = nil // a panel page from before the fit
	set.OBS.CameraFit = ""
	var got store.Settings
	if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
		t.Fatal(code)
	}
	if got.OBS.CameraFull == nil || got.OBS.CameraFit != "contain" {
		t.Errorf("camera %+v fit %q", got.OBS.CameraFull, got.OBS.CameraFit)
	}
	set.OBS.CameraSource = "DroidCam"
	call(t, s, "PUT", "/api/settings", set, &got)
	if got.OBS.CameraFull != nil {
		t.Error("another camera must be fitted again")
	}
}

func TestSettingsKeepCameras(t *testing.T) {
	s := newServer(t)
	full := &obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	saved := []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}}
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource, set.OBS.Cameras, set.OBS.GraphicsScene = "A", saved, "Grafismo"
		set.OBS.CameraFulls = map[string]*obsws.Full{"B": full}
	})
	keeps := func(got store.Settings, first string) {
		t.Helper()
		o := got.OBS
		if len(o.Cameras) != 2 || o.Cameras[0] != (store.Camera{Source: first, Scene: "Câmara 1"}) || o.Cameras[1] != saved[1] ||
			o.GraphicsScene != "Grafismo" || o.CameraFulls["B"] == nil || len(o.CameraFulls) != 1 {
			t.Errorf("cameras %+v graphics %q fulls %v", o.Cameras, o.GraphicsScene, o.CameraFulls)
		}
	}

	set := s.Store.Settings()
	set.OBS.Cameras, set.OBS.GraphicsScene, set.OBS.CameraFulls = nil, "", nil // a panel page from 0.1.2
	var got store.Settings
	if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
		t.Fatal(code)
	}
	keeps(got, "A")
	set.OBS.Cameras = []store.Camera{{Source: "X", Scene: "Y"}}
	set.OBS.GraphicsScene, set.OBS.CameraFulls = "Outra", map[string]*obsws.Full{"X": full}
	call(t, s, "PUT", "/api/settings", set, &got)
	keeps(got, "A")
	set.OBS.CameraSource = "DroidCam"
	call(t, s, "PUT", "/api/settings", set, &got)
	keeps(got, "DroidCam")
}

func TestSaveFullByCamera(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource = "A"
		set.OBS.Cameras = []store.Camera{{Source: "A"}, {Source: "B"}}
	})
	f := obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	s.Camera.Save("B", f)
	o := s.Store.Settings().OBS
	if o.CameraFulls["B"] == nil || *o.CameraFulls["B"] != f || o.CameraFull != nil {
		t.Errorf("camera 2: fulls %v full %v", o.CameraFulls, o.CameraFull)
	}
	g := obsws.Full{SourceWidth: 1920, SourceHeight: 1080, Fit: "fill"}
	s.Camera.Save("A", g)
	o = s.Store.Settings().OBS
	if o.CameraFulls["A"] == nil || *o.CameraFulls["A"] != g || o.CameraFull == nil || *o.CameraFull != g || *o.CameraFulls["B"] != f {
		t.Errorf("camera 1: fulls %v full %v", o.CameraFulls, o.CameraFull)
	}
}

// An automatic camera 1 (camera_source "") keeps camera_full; a second
// source never writes it.
func TestSaveFullAutoCamera(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Cameras = []store.Camera{{Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}}
	})
	f := obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	s.Camera.Save("DroidCam", f)
	g := obsws.Full{SourceWidth: 1920, SourceHeight: 1080, Fit: "contain"}
	s.Camera.Save("B", g)
	o := s.Store.Settings().OBS
	if o.CameraFull == nil || *o.CameraFull != f || o.CameraFulls["B"] == nil || *o.CameraFulls["B"] != g {
		t.Errorf("full %v fulls %v", o.CameraFull, o.CameraFulls)
	}
}

// The camera reads each source's own placement.
func TestCameraSettingsFulls(t *testing.T) {
	s := newServer(t)
	f := &obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource = "A"
		set.OBS.CameraFulls = map[string]*obsws.Full{"B": f}
	})
	if got := s.Camera.Settings().Fulls["B"]; got == nil || *got != *f {
		t.Errorf("camera 2's placement %v", got)
	}
}

func TestSettingsNewCameraWithoutList(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource, set.OBS.CameraFull = "A", &obsws.Full{Fit: "contain"}
	})
	set := s.Store.Settings()
	set.OBS.CameraSource = "B"
	var got store.Settings
	if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
		t.Fatal(code)
	}
	if got.OBS.Cameras != nil || got.OBS.CameraFull != nil || got.OBS.CameraSource != "B" {
		t.Errorf("cameras %v full %v source %q", got.OBS.Cameras, got.OBS.CameraFull, got.OBS.CameraSource)
	}
}

func TestSettingsTimeoutCamera(t *testing.T) {
	s := newServer(t)
	if n := len(s.Store.Settings().OBS.CameraList()); n >= 3 {
		t.Fatalf("%d cameras; 3 must be past the list", n) // a camera since removed stays chosen
	}
	for _, c := range []struct{ put, want int }{{2, 2}, {7, 0}, {3, 3}, {4, 4}, {5, 0}, {-1, 0}} {
		set := s.Store.Settings()
		set.OBS.TimeoutCamera = c.put
		var got store.Settings
		if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 || got.OBS.TimeoutCamera != c.want {
			t.Errorf("put %d: %d, saved %d", c.put, code, got.OBS.TimeoutCamera)
		}
	}
	// A panel page from 0.1.2 sends no timeout_camera.
	b, _ := json.Marshal(s.Store.Settings())
	var old map[string]any
	json.Unmarshal(b, &old)
	delete(old["obs"].(map[string]any), "timeout_camera")
	var got store.Settings
	if call(t, s, "PUT", "/api/settings", old, &got); got.OBS.TimeoutCamera != 0 {
		t.Errorf("without the key: %d", got.OBS.TimeoutCamera)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"obs":{"enabled":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := st.Settings().OBS.TimeoutCamera; n != 0 {
		t.Errorf("settings.json without it: %d", n)
	}
}

func TestSettingsScreenCameras(t *testing.T) {
	s := newServer(t)
	put := func(sc map[string]int, tcam int) store.OBS {
		t.Helper()
		set := s.Store.Settings()
		set.OBS.ScreenCameras, set.OBS.TimeoutCamera = sc, tcam
		var got store.Settings
		if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
			t.Fatalf("put %v: %d", sc, code)
		}
		return got.OBS
	}
	o := put(map[string]int{"nada": 2, "desconto": 3, "marcador": 1, "intervalo": 2}, 3)
	if !maps.Equal(o.ScreenCameras, map[string]int{"nada": 2, "desconto": 3}) || o.TimeoutCamera != 3 {
		t.Errorf("good values: %v timeout %d", o.ScreenCameras, o.TimeoutCamera)
	}
	o = put(map[string]int{"nada": 9, "desconto": -1}, 3)
	if o.ScreenCameras["nada"] != 0 || o.ScreenCameras["desconto"] != 0 || o.TimeoutCamera != 0 {
		t.Errorf("bad values kept: %v timeout %d", o.ScreenCameras, o.TimeoutCamera)
	}
	put(map[string]int{"nada": 1, "desconto": 0}, 0)
	// A page from before screen_cameras sends timeout_camera alone.
	if o = put(nil, 4); !maps.Equal(o.ScreenCameras, map[string]int{"nada": 1, "desconto": 4}) || o.TimeoutCamera != 4 {
		t.Errorf("timeout_camera alone: %v timeout %d", o.ScreenCameras, o.TimeoutCamera)
	}
}

// timeout_camera mirrors screen_cameras.desconto, so an older version keeps
// the time-out camera.
func TestScreenCamerasMirrorTimeout(t *testing.T) {
	open := func(js string) *store.Store {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	if o := open(`{"obs":{"timeout_camera":2}}`).Settings().OBS; o.ScreenCameras["desconto"] != 2 {
		t.Errorf("older settings: %v", o.ScreenCameras)
	}
	st := open(`{"obs":{"timeout_camera":2,"screen_cameras":{"desconto":3,"nada":1}}}`)
	if o := st.Settings().OBS; o.TimeoutCamera != 3 || o.ScreenCameras["nada"] != 1 {
		t.Errorf("the map must win: timeout %d %v", o.TimeoutCamera, o.ScreenCameras)
	}
	st.UpdateSettings(func(set *store.Settings) { set.OBS.TimeoutCamera = 4 })
	if o := st.Settings().OBS; o.ScreenCameras["desconto"] != 4 || o.ScreenCameras["nada"] != 1 {
		t.Errorf("timeout_camera saved: %v", o.ScreenCameras)
	}
	st.UpdateSettings(func(set *store.Settings) { set.OBS.ScreenCameras = map[string]int{"desconto": 1, "nada": 2} })
	if o := st.Settings().OBS; o.TimeoutCamera != 1 {
		t.Errorf("screen_cameras saved: timeout %d", o.TimeoutCamera)
	}
	b, err := os.ReadFile(filepath.Join(st.Dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		OBS struct {
			TimeoutCamera int            `json:"timeout_camera"`
			ScreenCameras map[string]int `json:"screen_cameras"`
		} `json:"obs"`
	}
	json.Unmarshal(b, &file)
	if file.OBS.TimeoutCamera != 1 || file.OBS.ScreenCameras["desconto"] != 1 || file.OBS.ScreenCameras["nada"] != 2 {
		t.Errorf("file %s", b)
	}
}

func TestSaveCamerasThree(t *testing.T) {
	s := newServer(t)
	saved := []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}, {Source: "C", Scene: "Câmara 3", Label: "Bancada"}}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraSource, set.OBS.Cameras = "A", saved })
	rep := obsws.Report{Cameras: []obsws.ReportCamera{{Source: "A", Scene: "Câmara 1"}, {Source: "B"},
		{Source: "D", Scene: "Câmara 3"}, {Source: "E", Scene: "Câmara 4"}}}
	if err := s.saveCameras(rep); err != nil {
		t.Fatal(err)
	}
	want := []store.Camera{saved[0], saved[1], {Source: "D", Scene: "Câmara 3", Label: "Bancada"}}
	if o := s.Store.Settings().OBS; !slices.Equal(o.Cameras, want) {
		t.Errorf("cameras %+v", o.Cameras)
	}
}

// fitCamera keeps camera n alone, whatever n is.
func TestFitCameraKeepsOnlyItsCamera(t *testing.T) {
	s := newServer(t)
	s.camErr = errors.New("sem janela")
	saved := []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraSource, set.OBS.Cameras = "A", saved })
	s.fitCamera(2, "Câmara 2b", "B2")
	want := []store.Camera{saved[0], {Source: "B2", Scene: "Câmara 2b"}}
	if o := s.Store.Settings().OBS; !slices.Equal(o.Cameras, want) || o.CameraSource != "A" {
		t.Errorf("cameras %+v source %q", o.Cameras, o.CameraSource)
	}
	s.fitCamera(3, "Câmara 3", "C") // not saved: no panic, no change
	if o := s.Store.Settings().OBS; !slices.Equal(o.Cameras, want) {
		t.Errorf("camera 3 beyond the list: %+v", o.Cameras)
	}
}

// A fit that fails leaves no placement behind, so the next check fits the
// camera in its new scene.
func TestFitCameraFailedForgetsPlacement(t *testing.T) {
	defer func(w time.Duration) { fitWait = w }(fitWait)
	fitWait = 0
	full := &obsws.Full{SourceWidth: 1280, SourceHeight: 720, CanvasWidth: 1920, CanvasHeight: 1080, Fit: "contain"}
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Port = 1 // nothing listens there
		set.OBS.CameraSource, set.OBS.CameraFull = "A", full
		set.OBS.Cameras = []store.Camera{{Source: "A"}, {Source: "B"}}
		set.OBS.CameraFulls = map[string]*obsws.Full{"A": full, "B": full}
	})
	if err := s.fitCamera(1, "Câmara 1", "A"); err == nil {
		t.Fatal("want the fit's error")
	}
	o := s.Store.Settings().OBS
	if o.CameraFull != nil || o.CameraFulls["A"] != nil || o.CameraFulls["B"] == nil {
		t.Errorf("camera 1: full %v fulls %v", o.CameraFull, o.CameraFulls)
	}
	s.camErr = errors.New("sem janela") // the fit is skipped
	s.fitCamera(2, "Câmara 2", "B")
	o = s.Store.Settings().OBS
	if o.CameraFulls["B"] != nil {
		t.Errorf("camera 2: fulls %v", o.CameraFulls)
	}
}

func TestSaveCamerasFromSetup(t *testing.T) {
	s := newServer(t)
	full := &obsws.Full{Fit: "contain"}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraSource, set.OBS.CameraFull = "Tablet", full })
	if err := s.saveCameras(obsws.Report{Graphics: "Grafismo", Cameras: []obsws.ReportCamera{{Source: "Tablet", Scene: "Câmara 1"}}}); err != nil {
		t.Fatal(err)
	}
	o := s.Store.Settings().OBS
	if o.GraphicsScene != "Grafismo" || len(o.Cameras) != 1 || o.Cameras[0] != (store.Camera{Source: "Tablet", Scene: "Câmara 1"}) || o.CameraFull != full {
		t.Errorf("%+v", o)
	}
	s.saveCameras(obsws.Report{Cameras: []obsws.ReportCamera{{Source: "Webcam"}}})
	o = s.Store.Settings().OBS
	if o.CameraSource != "Webcam" || o.Cameras[0].Scene != "Câmara 1" || o.CameraFull != nil {
		t.Errorf("new camera 1: %+v", o)
	}
}

func TestSetupWantsSavedScenes(t *testing.T) {
	s := newServer(t)
	o := store.OBS{CameraSource: "A", GraphicsScene: "Grafismo",
		Cameras: []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 2"}}}
	w := s.want(o, "", "")
	want := []obsws.WantCamera{{Choice: "source:A", Scene: "Câmara 1", Old: "A"}, {Choice: "source:B", Scene: "Câmara 2"}}
	if w.Graphics != "Grafismo" || !slices.Equal(w.Cameras, want) {
		t.Errorf("%+v", w)
	}
	if w := s.want(store.OBS{}, "", ""); len(w.Cameras) != 1 || w.Cameras[0] != (obsws.WantCamera{}) {
		t.Errorf("automatic camera: %+v", w.Cameras)
	}
	if w := s.want(o, "device:x", "Old"); w.Cameras[0].Choice != "device:x" || w.Cameras[0].Old != "Old" {
		t.Errorf("picked: %+v", w.Cameras[0])
	}
}

func TestScreenWithoutOBS(t *testing.T) {
	s := newServer(t)
	var resp map[string]any
	if code := call(t, s, "POST", "/api/screen", map[string]any{"screen": "plantel"}, &resp); code != 200 || resp["obs_error"] != nil {
		t.Fatalf("%d %v", code, resp)
	}
	// OBS on but not running: the overlay changes screen and the panel gets the error.
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled, set.OBS.Port = true, 1 })
	resp = nil
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, &resp)
	if resp["obs_error"] == nil || s.camErr != nil || s.State.Snapshot().Screen.Current != "desconto" {
		t.Errorf("%v %q", resp, s.State.Snapshot().Screen.Current)
	}
}

func overlayCSS(t *testing.T) []byte {
	css, err := os.ReadFile("../../web/overlay/overlay.css")
	if err != nil {
		t.Fatal(err)
	}
	return css
}
