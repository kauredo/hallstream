package obsws

import (
	"slices"
	"strings"
	"sync"
	"testing"
)

// model is a small OBS for the setup: scenes, inputs, devices, the canvas
// and the outputs. writes lists every request that changes something.
type model struct {
	mu        sync.Mutex
	scenes    []string
	items     map[string][]*mItem // bottom first
	program   string
	inputs    []*mInput
	kinds     []string
	devices   []map[string]any // video_device_id items
	phones    []map[string]any // DroidCam plugin device_list items
	baseW     int
	baseH     int
	outW      int
	outH      int
	streaming bool
	next      int
	writes    []string
	pressed   []string
}

type mItem struct {
	id      int
	source  string
	enabled bool
}

type mInput struct {
	name     string
	kind     string
	settings map[string]any
}

func newModel() *model {
	return &model{scenes: []string{"Cena"}, items: map[string][]*mItem{"Cena": nil}, program: "Cena",
		kinds: []string{"browser_source", "dshow_input", "image_source", "wasapi_input_capture"},
		baseW: 1920, baseH: 1080, outW: 1920, outH: 1080}
}

func (m *model) input(name string) *mInput {
	for _, in := range m.inputs {
		if in.name == name {
			return in
		}
	}
	return nil
}

// put adds an input made by the user to a scene (made if missing).
func (m *model) put(scene, name, kind string, settings map[string]any) {
	if !slices.Contains(m.scenes, scene) {
		m.scenes = append(m.scenes, scene)
	}
	if m.input(name) == nil {
		m.inputs = append(m.inputs, &mInput{name, kind, settings})
	}
	m.next++
	m.items[scene] = append(m.items[scene], &mItem{m.next, name, true})
}

func (m *model) item(scene string, id int) (int, *mItem) {
	for i, it := range m.items[scene] {
		if it.id == id {
			return i, it
		}
	}
	return -1, nil
}

func copyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *model) answer(typ string, d map[string]any) (bool, int, any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !strings.HasPrefix(typ, "Get") {
		m.writes = append(m.writes, typ)
	}
	sname, _ := d["sceneName"].(string)
	iname, _ := d["inputName"].(string)
	id := 0
	if f, ok := d["sceneItemId"].(float64); ok {
		id = int(f)
	}
	switch typ {
	case "GetStreamStatus":
		return true, 100, map[string]any{"outputActive": m.streaming, "outputDuration": 0}
	case "GetRecordStatus":
		return true, 100, map[string]any{"outputActive": false}
	case "GetSceneList":
		var list []map[string]any
		for _, s := range m.scenes {
			list = append(list, map[string]any{"sceneName": s})
		}
		return true, 100, map[string]any{"scenes": list}
	case "CreateScene":
		if slices.Contains(m.scenes, sname) {
			return false, 601, nil
		}
		m.scenes = append(m.scenes, sname)
		m.items[sname] = nil
		return true, 100, nil
	case "GetCurrentProgramScene":
		return true, 100, map[string]any{"sceneName": m.program}
	case "SetCurrentProgramScene":
		m.program = sname
		return true, 100, nil
	case "GetInputList":
		var list []map[string]any
		for _, in := range m.inputs {
			list = append(list, map[string]any{"inputName": in.name, "inputKind": in.kind, "unversionedInputKind": in.kind})
		}
		return true, 100, map[string]any{"inputs": list}
	case "GetInputSettings":
		in := m.input(iname)
		if in == nil {
			return false, NotFound, nil
		}
		return true, 100, map[string]any{"inputSettings": copyMap(in.settings), "inputKind": in.kind}
	case "GetInputKindList":
		return true, 100, map[string]any{"inputKinds": m.kinds}
	case "GetSceneItemList":
		if !slices.Contains(m.scenes, sname) {
			return false, NotFound, nil
		}
		var list []map[string]any
		for i, it := range m.items[sname] {
			list = append(list, map[string]any{"sceneItemId": it.id, "sceneItemIndex": i, "sourceName": it.source,
				"inputKind": m.input(it.source).kind, "sceneItemEnabled": it.enabled})
		}
		return true, 100, map[string]any{"sceneItems": list}
	case "CreateInput":
		kind, _ := d["inputKind"].(string)
		if m.input(iname) != nil {
			return false, 601, nil
		}
		if !slices.Contains(m.kinds, kind) {
			return false, 605, nil
		}
		set, _ := d["inputSettings"].(map[string]any)
		m.inputs = append(m.inputs, &mInput{iname, kind, copyMap(set)})
		m.next++
		on, ok := d["sceneItemEnabled"].(bool)
		m.items[sname] = append(m.items[sname], &mItem{m.next, iname, on || !ok})
		return true, 100, map[string]any{"sceneItemId": m.next}
	case "RemoveInput":
		m.inputs = slices.DeleteFunc(m.inputs, func(in *mInput) bool { return in.name == iname })
		for s := range m.items {
			m.items[s] = slices.DeleteFunc(m.items[s], func(it *mItem) bool { return it.source == iname })
		}
		return true, 100, nil
	case "SetInputSettings":
		in := m.input(iname)
		set, _ := d["inputSettings"].(map[string]any)
		if overlay, ok := d["overlay"].(bool); ok && !overlay {
			in.settings = map[string]any{}
		}
		for k, v := range set {
			in.settings[k] = v
		}
		return true, 100, nil
	case "GetInputPropertiesListPropertyItems":
		in := m.input(iname)
		switch {
		case in == nil:
			return false, NotFound, nil
		case in.kind == "dshow_input" && d["propertyName"] == "video_device_id":
			return true, 100, map[string]any{"propertyItems": m.devices}
		case in.kind == PluginKind && d["propertyName"] == "device_list":
			return true, 100, map[string]any{"propertyItems": m.phones}
		}
		return false, 602, nil
	case "PressInputPropertiesButton":
		m.pressed = append(m.pressed, iname+"/"+d["propertyName"].(string))
		return true, 100, nil
	case "CreateSceneItem":
		src, _ := d["sourceName"].(string)
		m.next++
		m.items[sname] = append(m.items[sname], &mItem{m.next, src, true})
		return true, 100, map[string]any{"sceneItemId": m.next}
	case "SetSceneItemEnabled":
		_, it := m.item(sname, id)
		it.enabled = d["sceneItemEnabled"].(bool)
		return true, 100, nil
	case "SetSceneItemIndex":
		i, it := m.item(sname, id)
		list := slices.Delete(m.items[sname], i, i+1)
		to := int(d["sceneItemIndex"].(float64))
		m.items[sname] = slices.Insert(list, min(to, len(list)), it)
		return true, 100, nil
	case "GetVideoSettings":
		return true, 100, map[string]any{"baseWidth": m.baseW, "baseHeight": m.baseH, "outputWidth": m.outW, "outputHeight": m.outH,
			"fpsNumerator": 30, "fpsDenominator": 1}
	case "SetVideoSettings":
		if m.streaming {
			return false, 500, nil
		}
		m.baseW, m.baseH = int(d["baseWidth"].(float64)), int(d["baseHeight"].(float64))
		m.outW, m.outH = int(d["outputWidth"].(float64)), int(d["outputHeight"].(float64))
		return true, 100, nil
	}
	return false, 204, nil
}

func (m *model) wrote() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.writes
	m.writes = nil
	return w
}

// order is the scene's sources, bottom first, hidden ones in brackets.
func (m *model) order(scene string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, it := range m.items[scene] {
		if it.enabled {
			out = append(out, it.source)
		} else {
			out = append(out, "("+it.source+")")
		}
	}
	return out
}

const overlayURL = "http://127.0.0.1:8765/overlay"

func device(name string) map[string]any {
	return map[string]any{"itemName": name, "itemEnabled": true, "itemValue": name + `:\\?\usb#vid_0001#` + strings.ToLower(strings.ReplaceAll(name, " ", ""))}
}

type run struct {
	t      *testing.T
	m      *model
	cfg    Config
	c      Client
	fitted []string
}

func newRun(t *testing.T, m *model) *run {
	return &run{t: t, m: m, cfg: fakeOBS(t, "", m)}
}

func (r *run) setup(camera, old string) Report {
	r.t.Helper()
	rep, err := r.c.Setup(r.cfg, Want{OverlayURL: overlayURL, OverlayPorts: [2]int{8765, 8775}, Camera: camera, Old: old}, true,
		func(cam string) error { r.fitted = append(r.fitted, cam); return nil })
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func (r *run) health(camera string) Report {
	r.t.Helper()
	rep, err := r.c.Setup(r.cfg, Want{OverlayURL: overlayURL, OverlayPorts: [2]int{8765, 8775}, Camera: camera}, false, nil)
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func results(rep Report) map[string]string {
	out := map[string]string{}
	for _, s := range rep.Steps {
		out[s.ID] = s.Result
	}
	return out
}

func detail(rep Report, id string) string {
	for _, s := range rep.Steps {
		if s.ID == id {
			return s.Detail
		}
	}
	return ""
}

func wantResults(t *testing.T, rep Report, want map[string]string) {
	t.Helper()
	got := results(rep)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s (%s), want %s", k, got[k], detail(rep, k), v)
		}
	}
}

// Again, nothing changes and the health check passes.
func (r *run) again(camera string) {
	r.t.Helper()
	r.m.wrote()
	rep := r.setup(camera, "")
	for _, s := range rep.Steps {
		if s.Result != "ok" {
			r.t.Errorf("second run: %s %s (%s)", s.ID, s.Result, s.Detail)
		}
	}
	if w := r.m.wrote(); len(w) != 0 {
		r.t.Errorf("second run wrote %v", w)
	}
	if h := r.health(camera); !h.Ready {
		r.t.Errorf("health %v", h.Problems())
	}
	if w := r.m.wrote(); len(w) != 0 {
		r.t.Errorf("health wrote %v", w)
	}
}

func TestSetupEmptyWithVirtualWebcam(t *testing.T) {
	m := newModel()
	m.baseW, m.baseH, m.outW, m.outH = 1280, 720, 1280, 720
	m.devices = []map[string]any{device("DroidCam Video")}
	r := newRun(t, m)
	if h := r.health(""); h.Ready || len(h.Problems()) == 0 {
		t.Fatalf("health of an empty OBS %+v", h)
	}
	rep := r.setup("", "")
	wantResults(t, rep, map[string]string{"scene": "created", "overlay": "created", "camera": "created",
		"order": "fixed", "video": "fixed", "program": "fixed", "fit": "ok"})
	if !rep.Ready || rep.Camera != "DroidCam Video" || !slices.Equal(r.fitted, []string{"DroidCam Video"}) {
		t.Errorf("ready %v camera %q fitted %v", rep.Ready, rep.Camera, r.fitted)
	}
	if got := m.order(SceneName); !slices.Equal(got, []string{"DroidCam Video", OverlayName}) {
		t.Errorf("scene %v", got)
	}
	ov := m.input(OverlayName).settings
	if ov["url"] != overlayURL || ov["width"] != 1920.0 || ov["height"] != 1080.0 || ov["reroute_audio"] != true {
		t.Errorf("overlay %v", ov)
	}
	if m.input(probeName) != nil || m.program != SceneName || m.baseW != 1920 || m.outH != 1080 {
		t.Errorf("probe left, program %q, canvas %dx%d", m.program, m.baseW, m.baseH)
	}
	if id := m.input("DroidCam Video").settings["video_device_id"]; id != device("DroidCam Video")["itemValue"] {
		t.Errorf("device id %v", id)
	}
	r.again("source:DroidCam Video")
}

func TestSetupExistingUserSetup(t *testing.T) {
	m := newModel()
	m.put("Jogo", "Tablet", "dshow_input", map[string]any{"video_device_id": device("DroidCam Source 3")["itemValue"]})
	m.put("Jogo", "Grafismo", "browser_source", map[string]any{"url": overlayURL, "width": 1920.0, "height": 1080.0, "reroute_audio": true})
	m.put("Jogo", "Logo", "image_source", map[string]any{})
	m.program = "Jogo"
	r := newRun(t, m)
	rep := r.setup("", "")
	wantResults(t, rep, map[string]string{"scene": "created", "overlay": "fixed", "camera": "fixed", "order": "fixed", "program": "fixed"})
	if rep.Camera != "Tablet" {
		t.Errorf("camera %q", rep.Camera)
	}
	if w := m.wrote(); slices.Contains(w, "CreateInput") || slices.Contains(w, "SetInputSettings") || slices.Contains(w, "RemoveInput") {
		t.Errorf("touched the user's sources: %v", w)
	}
	if got := m.order("Jogo"); !slices.Equal(got, []string{"Tablet", "Grafismo", "Logo"}) {
		t.Errorf("user scene changed: %v", got)
	}
	if got := m.order(SceneName); !slices.Equal(got, []string{"Tablet", "Grafismo"}) {
		t.Errorf("scene %v", got)
	}
	r.again("source:Tablet")
}

func TestSetupPlugin(t *testing.T) {
	m := newModel()
	m.kinds = []string{"browser_source", PluginKind}
	m.phones = []map[string]any{{"itemName": "Galaxy Tab A8", "itemEnabled": true, "itemValue": "R52T"},
		{"itemName": "Use WiFi IP", "itemEnabled": true, "itemValue": "dev_id_wifi"}}
	r := newRun(t, m)
	rep := r.setup("", "")
	wantResults(t, rep, map[string]string{"camera": "created", "overlay": "created"})
	cam := m.input("DroidCam")
	if rep.Camera != "DroidCam" || cam == nil || cam.kind != PluginKind || cam.settings["device_list"] != "R52T" ||
		!slices.Equal(m.pressed, []string{"DroidCam/connect"}) {
		t.Errorf("camera %q %+v pressed %v", rep.Camera, cam, m.pressed)
	}
	r.again("source:DroidCam")
}

func TestSetupNoCamera(t *testing.T) {
	m := newModel()
	r := newRun(t, m)
	rep := r.setup("", "")
	wantResults(t, rep, map[string]string{"camera": "failed", "overlay": "created"})
	if rep.Ready || !strings.Contains(detail(rep, "camera"), "DroidCam") || len(r.fitted) != 0 || m.input(probeName) != nil {
		t.Errorf("ready %v, %q, fitted %v", rep.Ready, detail(rep, "camera"), r.fitted)
	}
}

func TestSetupWebcamOnly(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("Logitech C920")}
	r := newRun(t, m)
	rep := r.setup("", "")
	if results(rep)["camera"] != "created" || rep.Camera != "Logitech C920" {
		t.Errorf("camera %q %s", rep.Camera, detail(rep, "camera"))
	}
	r.again("source:Logitech C920")
}

func TestSetupTwoDevicesPick(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("HD Webcam"), device("DroidCam Video")}
	m.put("Outra", "Cartão HDMI", "decklink-input", map[string]any{})
	r := newRun(t, m)
	rep := r.setup("", "")
	if results(rep)["camera"] != "choose" || rep.Ready || rep.Camera != "" || len(r.fitted) != 0 {
		t.Fatalf("camera %s %q", results(rep)["camera"], rep.Camera)
	}
	var labels []string
	for _, c := range rep.Choices {
		labels = append(labels, c.Type+":"+c.Label)
	}
	if !slices.Equal(labels, []string{"device:DroidCam Video", "source:Cartão HDMI", "device:HD Webcam"}) {
		t.Errorf("choices %v", labels)
	}
	rep = r.setup(rep.Choices[2].Value, "")
	if results(rep)["camera"] != "created" || rep.Camera != "HD Webcam" || !rep.Ready {
		t.Errorf("picked %q %s", rep.Camera, detail(rep, "camera"))
	}
	r.again("source:HD Webcam")
}

func TestSetupSwapCamera(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("DroidCam Video")}
	r := newRun(t, m)
	r.setup("", "")
	m.put("Outra", "Câmara HDMI", "dshow_input", map[string]any{"video_device_id": "Cam Link 4K:x"})
	rep := r.setup("source:Câmara HDMI", "DroidCam Video")
	wantResults(t, rep, map[string]string{"camera": "fixed", "old-camera": "fixed", "order": "fixed"})
	if got := m.order(SceneName); !slices.Equal(got, []string{"(DroidCam Video)", "Câmara HDMI", OverlayName}) {
		t.Errorf("scene %v", got)
	}
	if m.input("DroidCam Video") == nil || r.fitted[len(r.fitted)-1] != "Câmara HDMI" {
		t.Errorf("old camera removed or new not fitted: %v", r.fitted)
	}
	r.again("source:Câmara HDMI")
}

func TestSetupStreamingWrongSize(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("DroidCam Video")}
	m.baseW, m.baseH, m.outW, m.outH = 1920, 1080, 1280, 720
	m.streaming = true
	r := newRun(t, m)
	rep := r.setup("", "")
	if results(rep)["video"] != "later" || !strings.Contains(detail(rep, "video"), "transmissão") || !rep.Ready || results(rep)["program"] != "later" {
		t.Errorf("video %s: %s", results(rep)["video"], detail(rep, "video"))
	}
	if w := m.wrote(); slices.Contains(w, "SetVideoSettings") || slices.Contains(w, "SetCurrentProgramScene") || m.outW != 1280 {
		t.Error("changed the canvas or the scene while streaming")
	}
	if h := r.health(""); !h.Ready || results(h)["video"] != "later" {
		t.Errorf("health while live %v", h.Problems())
	}
}

func TestSetupWrongOverlayPort(t *testing.T) {
	m := newModel()
	m.put(SceneName, "Marcador", "browser_source", map[string]any{"url": "http://127.0.0.1:8766/overlay", "width": 1920.0, "height": 1080.0})
	m.put(SceneName, "Outro site", "browser_source", map[string]any{"url": "http://localhost:3000/overlay"})
	m.devices = []map[string]any{device("DroidCam Video")}
	r := newRun(t, m)
	if h := r.health(""); !strings.Contains(detail(h, "overlay"), "8766") {
		t.Errorf("health %v", h.Problems())
	}
	rep := r.setup("", "")
	if results(rep)["overlay"] != "fixed" || !strings.Contains(detail(rep, "overlay"), "8766") {
		t.Errorf("overlay %s: %s", results(rep)["overlay"], detail(rep, "overlay"))
	}
	if s := m.input("Marcador").settings; s["url"] != overlayURL || s["reroute_audio"] != true {
		t.Errorf("not fixed: %v", s)
	}
	if m.input("Outro site").settings["url"] != "http://localhost:3000/overlay" || m.input(OverlayName) != nil {
		t.Error("touched another program's browser source, or added a second overlay")
	}
	r.again("source:DroidCam Video")
}

func TestHealthCameraGone(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("DroidCam Video")}
	r := newRun(t, m)
	r.setup("", "")
	m.answer("RemoveInput", map[string]any{"inputName": "DroidCam Video"})
	h := r.health("source:DroidCam Video")
	if h.Ready || results(h)["camera"] != "failed" || !strings.Contains(detail(h, "camera"), "já não existe") {
		t.Errorf("health %v", h.Problems())
	}
}

// An NDI camera is no capture kind: only Old says it was the camera.
func TestSetupSwapFromNDI(t *testing.T) {
	m := newModel()
	m.put("Outra", "NDI Bancada", "ndi_source", map[string]any{})
	m.put("Outra", "Webcam", "dshow_input", map[string]any{"video_device_id": "Webcam:x"})
	r := newRun(t, m)
	r.setup("source:NDI Bancada", "")
	r.setup("source:Webcam", "NDI Bancada")
	if got := m.order(SceneName); !slices.Equal(got, []string{"(NDI Bancada)", "Webcam", OverlayName}) {
		t.Errorf("scene %v", got)
	}
}
