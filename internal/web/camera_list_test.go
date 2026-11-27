package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

func threeCameras(t *testing.T, tcam int) *Server {
	t.Helper()
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource, set.OBS.GraphicsScene, set.OBS.TimeoutCamera = "A", "Grafismo", tcam
		set.OBS.Cameras = []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "Telemóvel 1", Scene: "Câmara 2", Label: "Bancada", Vdo: "abc"},
			{Source: "C", Scene: "Câmara 3"}}
	})
	return s
}

// Only the server writes a phone's stream id: a PUT that sends another
// keeps the saved one, and a new camera 1 from a PUT has none.
func TestSettingsKeepVdo(t *testing.T) {
	s := threeCameras(t, 0)
	set := s.Store.Settings()
	set.OBS.Cameras = []store.Camera{{Source: "A", Vdo: "evil"}, {Source: "Telemóvel 1", Vdo: "evil"}}
	var got store.Settings
	if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
		t.Fatal(code)
	}
	if c := got.OBS.Cameras; len(c) != 3 || c[0].Vdo != "" || c[1].Vdo != "abc" || c[2].Vdo != "" {
		t.Errorf("cameras %+v", c)
	}
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.CameraSource = "Telemóvel 1"
		set.OBS.Cameras = []store.Camera{{Source: "Telemóvel 1", Vdo: "abc"}}
	})
	set = s.Store.Settings()
	set.OBS.CameraSource = "Webcam"
	call(t, s, "PUT", "/api/settings", set, &got)
	if c := got.OBS.Cameras; c[0].Source != "Webcam" || c[0].Vdo != "" {
		t.Errorf("new camera 1 kept the phone's id: %+v", c)
	}
}

func TestCameraListTimeoutFollows(t *testing.T) {
	keep := func(n int) setupCamera {
		return setupCamera{Choice: []string{"", "source:A", "vdo:abc", "source:C"}[n], Was: n}
	}
	for _, c := range []struct {
		name       string
		tcam, want int
		list       []setupCamera
	}{
		{"camera 2 of 3 removed, camera 3 moves down", 3, 2, []setupCamera{keep(1), keep(3)}},
		{"the time-out camera removed", 2, 0, []setupCamera{keep(1), keep(3)}},
		{"a camera before it removed", 2, 1, []setupCamera{keep(2), keep(3)}},
		{"a camera after it removed", 1, 1, []setupCamera{keep(1), keep(2)}},
		{"reordered", 1, 3, []setupCamera{keep(3), keep(2), keep(1)}},
		{"none stays none", 0, 0, []setupCamera{keep(1)}},
		{"a camera replaced keeps its place", 3, 3, []setupCamera{keep(1), keep(2), {Choice: "device:x", Was: 3}}},
	} {
		s := threeCameras(t, c.tcam)
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.ScreenCameras = map[string]int{"nada": c.tcam, "desconto": c.tcam} })
		if _, err := s.listCameras(c.list); err != nil {
			t.Fatal(err)
		}
		if o := s.Store.Settings().OBS; o.TimeoutCamera != c.want || o.ScreenCameras["desconto"] != c.want || o.ScreenCameras["nada"] != c.want {
			t.Errorf("%s: timeout_camera %d, screen_cameras %v, want %d", c.name, o.TimeoutCamera, o.ScreenCameras, c.want)
		}
	}
}

func TestCameraListSaves(t *testing.T) {
	s := threeCameras(t, 0)
	w, err := s.listCameras([]setupCamera{
		{Choice: "vdo:abc", Label: "  Ao   alto  ", Was: 2},
		{Choice: "device:HD Webcam:x", Label: "Bancos", Was: 1},
		{Choice: "vdo:evil", Label: "Nova"},
	})
	if err != nil {
		t.Fatal(err)
	}
	o := s.Store.Settings().OBS
	// Camera 2 keeps its source until the setup adds the new device.
	want := []store.Camera{{Source: "Telemóvel 1", Scene: "Câmara 2", Label: "Ao alto", Vdo: "abc"}, {Source: "A", Scene: "Câmara 1", Label: "Bancos"}, {Label: "Nova"}}
	if !slices.Equal(o.Cameras, want) || o.CameraSource != "Telemóvel 1" || o.CameraFull != nil {
		t.Errorf("cameras %+v source %q", o.Cameras, o.CameraSource)
	}
	aim := []obsws.WantCamera{{Choice: "source:Telemóvel 1", Scene: "Câmara 2", Old: "Telemóvel 1", Vdo: "abc"},
		{Choice: "device:HD Webcam:x", Scene: "Câmara 1", Old: "A"}, {Choice: "vdo"}}
	if w.Graphics != "Grafismo" || !slices.Equal(w.Cameras, aim) {
		t.Errorf("want %+v", w.Cameras)
	}

	// The setup reports the new phone's id, which is saved with its source.
	rep := obsws.Report{Cameras: []obsws.ReportCamera{{Source: "Telemóvel 1", Scene: "Câmara 2"}, {Source: "HD Webcam", Scene: "Câmara 1"},
		{Source: "Telemóvel 2", Scene: "Câmara 3"}}, Phones: map[string]string{"Telemóvel 1": "abc", "Telemóvel 2": "newid"}}
	if err := s.saveCameras(rep); err != nil {
		t.Fatal(err)
	}
	o = s.Store.Settings().OBS
	if c := o.Cameras; c[0].Vdo != "abc" || c[1].Vdo != "" || c[2] != (store.Camera{Source: "Telemóvel 2", Scene: "Câmara 3", Label: "Nova", Vdo: "newid"}) {
		t.Errorf("after the setup %+v", c)
	}
	// A phone replaced by another camera loses its id; a fit's report keeps it.
	s.saveCameras(obsws.Report{Cameras: []obsws.ReportCamera{{Source: "Webcam"}}})
	s.camErr = errors.New("sem janela")
	s.fitCamera(3, "Câmara 3", "Telemóvel 2")
	if c := s.Store.Settings().OBS.Cameras; c[0].Vdo != "" || c[2].Vdo != "newid" {
		t.Errorf("replaced %+v", c)
	}
}

func TestCameraListRefused(t *testing.T) {
	s := threeCameras(t, 0)
	for _, list := range [][]setupCamera{{}, {{Was: 1}, {Was: 1}}, {{Was: 4}}, {{Was: -1}}, {{}, {}, {}, {}}} {
		var resp map[string]any
		if code := call(t, s, "POST", "/api/obs/setup", map[string]any{"cameras": list}, &resp); code != http.StatusBadRequest {
			t.Errorf("%v: %d %v", list, code, resp)
		}
	}
	if o := s.Store.Settings().OBS; len(o.Cameras) != 3 {
		t.Errorf("cameras changed: %+v", o.Cameras)
	}
}

// Configurar OBS with a list saves it before it sets OBS up, so a lost
// connection still keeps the names and the time-out camera.
func TestSetupSavesTheList(t *testing.T) {
	s := threeCameras(t, 3)
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
	list := []setupCamera{{Choice: "source:A", Label: "Campo", Was: 1}, {Choice: "source:C", Was: 3}}
	var resp map[string]any
	if code := call(t, s, "POST", "/api/obs/setup", map[string]any{"cameras": list}, &resp); code != http.StatusBadGateway {
		t.Fatalf("%d %v", code, resp)
	}
	o := s.Store.Settings().OBS
	want := []store.Camera{{Source: "A", Scene: "Câmara 1", Label: "Campo"}, {Source: "C", Scene: "Câmara 3"}}
	if !slices.Equal(o.Cameras, want) || o.TimeoutCamera != 2 {
		t.Errorf("cameras %+v timeout_camera %d", o.Cameras, o.TimeoutCamera)
	}
}

// The QR code holds the phone's push link, a secret: other devices need the
// PIN, and nothing keeps a copy.
func TestPhoneQR(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.PhoneAccess, set.PIN = true, "1234"
		set.OBS.Cameras = []store.Camera{{Source: "DroidCam"}, {Source: "Telemóvel 1", Vdo: "abc"}}
	})
	get := func(path, from, host, pin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr, req.Host = from, host
		if pin != "" {
			req.Header.Set("X-PIN", pin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w
	}
	const qr2, phone, pc, here = "/api/obs/cameras/2/qr.svg", "192.168.1.20:5000", "127.0.0.1:5000", "127.0.0.1:8765"
	if w := get(qr2, phone, here, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("phone without the PIN: %d", w.Code)
	}
	w := get(qr2, phone, here, "1234")
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<svg") || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Errorf("phone with the PIN: %d %q %v", w.Code, w.Body.String()[:min(20, w.Body.Len())], w.Header())
	}
	if w := get(qr2, pc, here, ""); w.Code != 200 {
		t.Errorf("the PC: %d", w.Code)
	}
	for _, p := range []string{"/api/obs/cameras/1/qr.svg", "/api/obs/cameras/4/qr.svg", "/api/obs/cameras/0/qr.svg", "/api/obs/cameras/x/qr.svg"} {
		if w := get(p, pc, here, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := get(qr2, pc, "evil.example", ""); w.Code != http.StatusForbidden {
		t.Errorf("another host: %d", w.Code)
	}
}
