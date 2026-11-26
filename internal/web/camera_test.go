package web

import (
	"os"
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
