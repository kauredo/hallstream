package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

// clean moves with its camera; a row without it (an older panel) keeps the
// saved camera's, and a new camera without it shows the graphics.
func TestCameraListKeepsClean(t *testing.T) {
	s := threeCameras(t, 0)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras[2].Clean = true })
	on, off := true, false
	if _, err := s.listCameras([]setupCamera{
		{Choice: "source:C", Was: 3},
		{Choice: "source:A", Was: 1, Clean: &on},
		{Choice: "vdo:abc", Was: 2},
		{Choice: "device:x"},
	}); err != nil {
		t.Fatal(err)
	}
	got := func() []bool {
		var b []bool
		for _, c := range s.Store.Settings().OBS.Cameras {
			b = append(b, c.Clean)
		}
		return b
	}
	if b := got(); len(b) != 4 || !b[0] || !b[1] || b[2] || b[3] {
		t.Errorf("reordered: %v", b)
	}
	if _, err := s.listCameras([]setupCamera{{Choice: "source:C", Was: 1, Clean: &off}, {Choice: "source:A", Was: 2}}); err != nil {
		t.Fatal(err)
	}
	if b := got(); len(b) != 2 || b[0] || !b[1] {
		t.Errorf("switched off: %v", b)
	}
}

// obs.clean is the live camera's clean, and follows a switch and the settings.
func TestLiveCameraClean(t *testing.T) {
	f := newCamOBS("A", "B")
	cams := []store.Camera{twoCams[0], {Source: "B", Scene: "Câmara 2", Clean: true}}
	s := camServer(t, f, cams)
	clean := func() bool { return s.State.Snapshot().OBS.Clean }
	s.programScene("Câmara 1")
	if clean() {
		t.Error("camera 1 is not clean")
	}
	if code, resp := switchCamera(t, s, 2); code != 200 || !clean() {
		t.Errorf("switched to camera 2: %d %v clean %v", code, resp, clean())
	}
	if switchCamera(t, s, 1); clean() {
		t.Error("back on camera 1, still clean")
	}
	s.programScene("Câmara 1")
	s.programScene("Câmara 2") // a switch made in OBS
	if !clean() {
		t.Error("camera 2 from OBS not clean")
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = []store.Camera{cams[1], cams[0]} })
	s.liveAgain()
	if o := s.State.Snapshot().OBS; o.LiveCamera != 1 || !o.Clean {
		t.Errorf("reordered: %+v", o)
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras[0].Clean = false })
	s.liveAgain()
	if clean() {
		t.Error("switched off in the settings, still clean")
	}
	s.programScene("Cena") // no camera's scene
	if o := s.State.Snapshot().OBS; o.LiveCamera != 0 || o.Clean {
		t.Errorf("another scene: %+v", o)
	}
}

// Saving the list from the panel sets obs.clean at once, even when the
// setup that follows fails.
func TestSetupListSetsClean(t *testing.T) {
	s := threeCameras(t, 0)
	host, port := fakeOBSData(t, "", func(typ string, data json.RawMessage) (bool, int, any) {
		switch typ {
		case "GetStreamStatus", "GetRecordStatus":
			return true, 100, map[string]any{"outputActive": false}
		case "GetInputList":
			return false, hangUp, nil
		}
		return true, 100, nil
	})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = host, port })
	s.sceneSeen("Câmara 3", false)
	list := []map[string]any{{"choice": "source:C", "was": 3, "clean": true}, {"choice": "source:A", "was": 1}}
	var resp map[string]any
	if code := call(t, s, "POST", "/api/obs/setup", map[string]any{"cameras": list}, &resp); code != http.StatusBadGateway {
		t.Fatalf("%d %v", code, resp)
	}
	if o := s.State.Snapshot().OBS; o.LiveCamera != 1 || !o.Clean {
		t.Errorf("live %+v", o)
	}
}
