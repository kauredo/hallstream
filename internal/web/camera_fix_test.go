package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// tinyOBS is just enough OBS for a whole Configurar OBS: scenes, inputs and
// scene items. refuse makes OBS refuse one request type, hangOn makes it
// drop the connection at one.
type tinyOBS struct {
	mu      sync.Mutex // the test and the fake's connection both touch it
	scenes  []string
	items   map[string][]tinyItem
	inputs  []tinyInput
	next    int
	refuse  string
	hangOn  string
	program string
}

type tinyItem struct {
	id      int
	name    string
	enabled bool
}

type tinyInput struct {
	name, kind string
	settings   map[string]any
}

func newTinyOBS(inputs ...string) *tinyOBS {
	o := &tinyOBS{scenes: []string{"Cena"}, items: map[string][]tinyItem{"Cena": nil}, program: "Cena"}
	for _, n := range inputs {
		o.inputs = append(o.inputs, tinyInput{n, "ndi_source", map[string]any{}})
	}
	return o
}

func (o *tinyOBS) input(name string) *tinyInput {
	for i := range o.inputs {
		if o.inputs[i].name == name {
			return &o.inputs[i]
		}
	}
	return nil
}

// fail sets which request OBS refuses and at which it hangs up.
func (o *tinyOBS) fail(refuse, hangOn string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.refuse, o.hangOn = refuse, hangOn
}

func (o *tinyOBS) answer(typ string, data json.RawMessage) (bool, int, any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var d map[string]any
	json.Unmarshal(data, &d)
	sname, _ := d["sceneName"].(string)
	iname, _ := d["inputName"].(string)
	switch {
	case typ == o.hangOn:
		return false, hangUp, nil
	case typ == o.refuse:
		return false, 600, nil
	}
	switch typ {
	case "GetStreamStatus", "GetRecordStatus":
		return true, 100, map[string]any{"outputActive": false}
	case "GetSceneList":
		var list []map[string]any
		for _, s := range o.scenes {
			list = append(list, map[string]any{"sceneName": s})
		}
		return true, 100, map[string]any{"scenes": list}
	case "CreateScene":
		o.scenes = append(o.scenes, sname)
		return true, 100, nil
	case "GetInputList":
		var list []map[string]any
		for _, in := range o.inputs {
			list = append(list, map[string]any{"inputName": in.name, "unversionedInputKind": in.kind})
		}
		return true, 100, map[string]any{"inputs": list}
	case "GetInputSettings":
		if in := o.input(iname); in != nil {
			return true, 100, map[string]any{"inputSettings": in.settings}
		}
		return false, 600, nil
	case "GetInputKindList":
		return true, 100, map[string]any{"inputKinds": []string{"browser_source"}}
	case "CreateInput":
		kind, _ := d["inputKind"].(string)
		set, _ := d["inputSettings"].(map[string]any)
		o.inputs = append(o.inputs, tinyInput{iname, kind, set})
		o.next++
		o.items[sname] = append(o.items[sname], tinyItem{o.next, iname, true})
		return true, 100, nil
	case "CreateSceneItem":
		src, _ := d["sourceName"].(string)
		o.next++
		o.items[sname] = append(o.items[sname], tinyItem{o.next, src, true})
		return true, 100, nil
	case "GetSceneItemList":
		var list []map[string]any
		for i, it := range o.items[sname] {
			kind := ""
			if in := o.input(it.name); in != nil {
				kind = in.kind
			}
			list = append(list, map[string]any{"sceneItemId": it.id, "sceneItemIndex": i, "sourceName": it.name, "inputKind": kind, "sceneItemEnabled": it.enabled})
		}
		return true, 100, map[string]any{"sceneItems": list}
	case "GetVideoSettings":
		return true, 100, map[string]any{"baseWidth": 1920, "baseHeight": 1080, "outputWidth": 1920, "outputHeight": 1080}
	case "GetCurrentProgramScene":
		return true, 100, map[string]any{"sceneName": o.program}
	case "SetCurrentProgramScene":
		o.program = sname
		return true, 100, nil
	}
	return true, 100, nil
}

// useTiny points s at o; fits fail at once, as with no OBS window.
func useTiny(t *testing.T, s *Server, o *tinyOBS) {
	t.Helper()
	host, port := fakeOBSData(t, "", o.answer)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = host, port })
	s.camErr = errors.New("sem janela")
}

// from sends a JSON request as the PC (127.0.0.1) or as a phone with the PIN 1234.
func from(t *testing.T, s *Server, addr, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.RemoteAddr, req.Host = addr, "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PIN", "1234")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	s.bg.Wait()
	return w
}

const thePhone, thePC = "192.168.1.20:5000", "127.0.0.1:5000"

func phoneAccess(s *Server) {
	s.Store.UpdateSettings(func(set *store.Settings) { set.PhoneAccess, set.PIN = true, "1234" })
}

// A phone camera's stream id is the push link's secret: a phone with the
// PIN never gets it in the settings, only has_vdo; the PC gets both.
func TestSettingsHideVdoFromPhones(t *testing.T) {
	s := threeCameras(t, 0)
	phoneAccess(s)
	for _, c := range []struct{ method, addr string }{{"GET", thePhone}, {"PUT", thePhone}, {"GET", thePC}, {"PUT", thePC}} {
		var body any
		if c.method == "PUT" {
			body = s.Store.Settings()
		}
		w := from(t, s, c.addr, c.method, "/api/settings", body)
		var got store.Settings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
			t.Fatalf("%s from %s: %d %v", c.method, c.addr, w.Code, err)
		}
		cams := got.OBS.Cameras
		if len(cams) != 3 || !cams[1].HasVdo || cams[0].HasVdo {
			t.Errorf("%s from %s: cameras %+v", c.method, c.addr, cams)
		}
		if leaked := strings.Contains(w.Body.String(), `"abc"`); leaked != (c.addr == thePC) {
			t.Errorf("%s from %s: the id in the reply %v", c.method, c.addr, leaked)
		}
	}
	if c := s.Store.Settings().OBS.Cameras[1]; c.Vdo != "abc" || c.HasVdo {
		t.Errorf("the saved camera changed: %+v", c)
	}
}

// Configurar OBS from a phone answers without the new phone's id.
func TestSetupHidesPhonesFromPhones(t *testing.T) {
	for _, addr := range []string{thePhone, thePC} {
		s := newServer(t)
		phoneAccess(s)
		useTiny(t, s, newTinyOBS("A"))
		w := from(t, s, addr, "POST", "/api/obs/setup", map[string]any{"cameras": []setupCamera{{Choice: "source:A", Was: 1}, {Choice: "vdo"}}})
		id := s.Store.Settings().OBS.Cameras[1].Vdo
		if w.Code != 200 || id == "" {
			t.Fatalf("from %s: %d %s, id %q", addr, w.Code, w.Body.String(), id)
		}
		var rep struct {
			Phones map[string]string `json:"phones"`
			Saved  bool              `json:"saved"`
		}
		json.Unmarshal(w.Body.Bytes(), &rep)
		if leaked := strings.Contains(w.Body.String(), id); leaked != (addr == thePC) || !rep.Saved {
			t.Errorf("from %s: id in the reply %v, saved %v: %s", addr, leaked, rep.Saved, w.Body.String())
		}
	}
}

// The push link comes alone, behind the PIN, never cached.
func TestPhoneLink(t *testing.T) {
	s := threeCameras(t, 0)
	phoneAccess(s)
	w := from(t, s, thePhone, "GET", "/api/obs/cameras/2/link", nil)
	var got map[string]string
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got["link"] != obsws.VDOPush("abc") || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("link: %d %v %v", w.Code, got, w.Header())
	}
	if w := from(t, s, thePhone, "GET", "/api/obs/cameras/1/link", nil); w.Code != http.StatusNotFound {
		t.Errorf("camera 1, no phone: %d", w.Code)
	}
	req := httptest.NewRequest("GET", "/api/obs/cameras/2/link", nil)
	req.RemoteAddr, req.Host = thePhone, "127.0.0.1:8765"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no PIN: %d", rec.Code)
	}
}

// A camera whose new pick OBS refused keeps the camera it had, and the
// list the panel rebuilds from the settings goes through again.
func TestSetupFailedPickKeepsSource(t *testing.T) {
	for _, c := range []struct {
		name, refuse, hangOn string
		code                 int
		saved                bool
	}{{"refused", "CreateInput", "", 200, true}, {"connection lost", "", "CreateInput", http.StatusBadGateway, true}} {
		s := threeCameras(t, 0)
		o := newTinyOBS("A", "C")
		useTiny(t, s, o)
		o.fail(c.refuse, c.hangOn)
		w := from(t, s, thePC, "POST", "/api/obs/setup", map[string]any{"cameras": []setupCamera{{Choice: "source:A", Was: 1}, {Choice: "vdo", Was: 3}}})
		var rep struct {
			Saved bool `json:"saved"`
		}
		json.Unmarshal(w.Body.Bytes(), &rep)
		if w.Code != c.code || rep.Saved != c.saved {
			t.Fatalf("%s: %d saved %v %s", c.name, w.Code, rep.Saved, w.Body.String())
		}
		cams := s.Store.Settings().OBS.Cameras
		if len(cams) != 2 || cams[1].Source != "C" || cams[1].Scene != "Câmara 3" || cams[1].Vdo != "" {
			t.Errorf("%s: cameras %+v", c.name, cams)
		}
		// The panel's rows after a re-read of the settings.
		o.fail("", "")
		w = from(t, s, thePC, "POST", "/api/obs/setup", map[string]any{"cameras": []setupCamera{{Choice: "source:A", Was: 1}, {Choice: "source:C", Was: 2}}})
		if cams := s.Store.Settings().OBS.Cameras; w.Code != 200 || cams[1].Source != "C" || cams[1].Scene != "Câmara 3" {
			t.Errorf("%s: retry %d %+v", c.name, w.Code, cams)
		}
	}
}

// With OBS out of reach the list is not saved, and the reply says so.
func TestSetupUnreachableKeepsList(t *testing.T) {
	s := threeCameras(t, 2)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = "127.0.0.1", port })
	before := s.Store.Settings().OBS
	var rep map[string]any
	code := call(t, s, "POST", "/api/obs/setup", map[string]any{"cameras": []setupCamera{{Choice: "source:C", Was: 3}, {Choice: "vdo", Label: "Nova"}}}, &rep)
	if code != 200 || rep["saved"] != false || rep["ok"] != false {
		t.Errorf("%d %v", code, rep)
	}
	if o := s.Store.Settings().OBS; !slices.Equal(o.Cameras, before.Cameras) || o.TimeoutCamera != 2 {
		t.Errorf("cameras %+v timeout %d", o.Cameras, o.TimeoutCamera)
	}
}

// A request without cameras sets OBS up from the saved list and changes no label.
func TestSetupWithoutCamerasKeepsList(t *testing.T) {
	s := threeCameras(t, 0)
	useTiny(t, s, newTinyOBS("A", "Telemóvel 1", "C"))
	var rep map[string]any
	if code := call(t, s, "POST", "/api/obs/setup", map[string]any{}, &rep); code != 200 || rep["saved"] != false {
		t.Fatalf("%d %v", code, rep)
	}
	cams := s.Store.Settings().OBS.Cameras
	if len(cams) != 3 || cams[1].Label != "Bancada" || cams[1].Vdo != "abc" || cams[2].Source != "C" {
		t.Errorf("cameras %+v", cams)
	}
}

// A phone source the setup made before the connection dropped keeps its id.
func TestSetupLostKeepsNewPhone(t *testing.T) {
	s := threeCameras(t, 0)
	o := newTinyOBS("A", "Telemóvel 1", "C")
	useTiny(t, s, o)
	o.fail("", "GetVideoSettings")
	w := from(t, s, thePC, "POST", "/api/obs/setup", map[string]any{"cameras": []setupCamera{{Choice: "source:A", Was: 1}, {Choice: "vdo", Was: 3}}})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	c := s.Store.Settings().OBS.Cameras[1]
	o.mu.Lock()
	in := o.input("Telemóvel 2")
	o.mu.Unlock()
	if in == nil || c.Source != "Telemóvel 2" || c.Vdo == "" || in.settings["url"] != obsws.VDOView(c.Vdo) {
		t.Errorf("camera 2 %+v, source %+v", c, in)
	}
}

func TestCameraListValidation(t *testing.T) {
	s := threeCameras(t, 0)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = "127.0.0.1", 1 })
	for _, c := range []struct {
		name string
		list []setupCamera
		want int
	}{
		{"camera 2 with no choice", []setupCamera{{Was: 1}, {Label: "x"}}, 400},
		{"camera 3 with no choice", []setupCamera{{Was: 1}, {Choice: "source:C", Was: 3}, {}}, 400},
		{"the same source twice", []setupCamera{{Choice: "source:A", Was: 1}, {Choice: "source:A"}}, 400},
		{"the same phone twice", []setupCamera{{Choice: "vdo:abc", Was: 2}, {Choice: "vdo:abc"}}, 400},
		{"a phone and its source", []setupCamera{{Choice: "vdo:abc", Was: 2}, {Choice: "source:Telemóvel 1"}}, 400},
		{"camera 4 with no choice", []setupCamera{{Was: 1}, {Choice: "source:C", Was: 3}, {Choice: "vdo"}, {}}, 400},
		{"five", []setupCamera{{Was: 1}, {Choice: "plugin"}, {Choice: "plugin"}, {Choice: "plugin"}, {Choice: "plugin"}}, 400},
		{"four", []setupCamera{{Was: 1}, {Choice: "source:C", Was: 3}, {Choice: "vdo"}, {Choice: "plugin"}}, 200},
		{"camera 1 automatic, two new phones", []setupCamera{{Was: 1}, {Choice: "vdo"}, {Choice: "vdo"}}, 200},
		{"exactly three", []setupCamera{{Choice: "source:C", Was: 3}, {Choice: "source:Telemóvel 1", Was: 2}, {Choice: "source:A", Was: 1}}, 200},
	} {
		var resp map[string]any
		if code := call(t, s, "POST", "/api/obs/setup", map[string]any{"cameras": c.list}, &resp); code != c.want {
			t.Errorf("%s: %d %v", c.name, code, resp)
		}
	}
	if o := s.Store.Settings().OBS; len(o.Cameras) != 3 || o.Cameras[0].Source != "A" {
		t.Errorf("cameras changed: %+v", o.Cameras)
	}
}

// A reordered list takes each camera's source, scene and phone id with it,
// whether the panel names the phone by its source or by vdo:<id>.
func TestCameraListReorderPhones(t *testing.T) {
	for _, phone := range []string{"source:Telemóvel 1", "vdo:abc"} {
		s := threeCameras(t, 0)
		if _, err := s.listCameras([]setupCamera{{Choice: phone, Was: 2}, {Choice: "source:C", Was: 3}, {Choice: "source:A", Was: 1}}); err != nil {
			t.Fatal(err)
		}
		want := []store.Camera{{Source: "Telemóvel 1", Scene: "Câmara 2", Vdo: "abc"}, {Source: "C", Scene: "Câmara 3"}, {Source: "A", Scene: "Câmara 1"}}
		if o := s.Store.Settings().OBS; !slices.Equal(o.Cameras, want) || o.CameraSource != "Telemóvel 1" {
			t.Errorf("%s: %+v", phone, o.Cameras)
		}
	}
}

// A time-out camera past the old list (a camera that was gone) does not
// land on a new camera.
func TestCameraListTimeoutPastTheList(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Cameras, set.OBS.TimeoutCamera = []store.Camera{{Source: "A", Scene: "Câmara 1"}}, 3
	})
	if _, err := s.listCameras([]setupCamera{{Choice: "source:A", Was: 1}, {Choice: "vdo"}, {Choice: "plugin"}}); err != nil {
		t.Fatal(err)
	}
	if got := s.Store.Settings().OBS.TimeoutCamera; got != 0 {
		t.Errorf("timeout_camera %d", got)
	}
}

// A fit saves the new source; Vdo stays when the source only changes from none.
func TestSaveCamerasKeepsVdoFromEmpty(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Cameras = []store.Camera{{Source: "A"}, {Vdo: "abc"}}
	})
	s.saveCameras(obsws.Report{Cameras: []obsws.ReportCamera{{}, {Source: "Telemóvel 1", Scene: "Câmara 2"}}})
	if c := s.Store.Settings().OBS.Cameras[1]; c.Source != "Telemóvel 1" || c.Vdo != "abc" {
		t.Errorf("camera 2 %+v", c)
	}
}
