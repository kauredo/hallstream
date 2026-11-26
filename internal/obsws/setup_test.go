package obsws

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kauredo/hallstream/internal/i18n"
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
	wroteIn   []string // the sceneName of each write that had one
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

// nest puts scene child in scene.
func (m *model) nest(scene, child string) {
	m.next++
	m.items[scene] = append(m.items[scene], &mItem{m.next, child, true})
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
	sname, _ := d["sceneName"].(string)
	if !strings.HasPrefix(typ, "Get") {
		m.writes = append(m.writes, typ)
		if sname != "" {
			m.wroteIn = append(m.wroteIn, sname)
		}
	}
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
		if slices.Contains(m.scenes, sname) || m.input(sname) != nil {
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
			item := map[string]any{"sceneItemId": it.id, "sceneItemIndex": i, "sourceName": it.source,
				"inputKind": nil, "sourceType": "OBS_SOURCE_TYPE_SCENE", "sceneItemEnabled": it.enabled}
			if in := m.input(it.source); in != nil {
				item["inputKind"], item["sourceType"] = in.kind, "OBS_SOURCE_TYPE_INPUT"
			}
			list = append(list, item)
		}
		return true, 100, map[string]any{"sceneItems": list}
	case "CreateInput":
		kind, _ := d["inputKind"].(string)
		if m.input(iname) != nil || slices.Contains(m.scenes, iname) {
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
		if m.input(src) == nil && !slices.Contains(m.scenes, src) {
			return false, NotFound, nil
		}
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
	t        *testing.T
	m        *model
	cfg      Config
	c        Client
	fitted   []string // the sources fitted
	fits     []string // "n scene source" of each fit
	graphics string   // the saved scenes, as the server keeps them
	scenes   []string
	extra    []string // the choices of cameras 2 and 3
}

func newRun(t *testing.T, m *model) *run {
	return &run{t: t, m: m, cfg: fakeOBS(t, "", m)}
}

func (r *run) want(camera, old string) Want {
	w := Want{OverlayURL: overlayURL, OverlayPorts: [2]int{8765, 8775}, Graphics: r.graphics}
	for i, choice := range append([]string{camera}, r.extra...) {
		c := WantCamera{Choice: choice}
		if i == 0 {
			c.Old = old
		}
		if i < len(r.scenes) {
			c.Scene = r.scenes[i]
		}
		w.Cameras = append(w.Cameras, c)
	}
	return w
}

func (r *run) setup(camera, old string) Report {
	r.t.Helper()
	rep, err := r.c.Setup(r.cfg, r.want(camera, old), true, func(n int, scene, cam string) error {
		r.fitted = append(r.fitted, cam)
		r.fits = append(r.fits, fmt.Sprintf("%d %s %s", n, scene, cam))
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
	if rep.Graphics != "" {
		r.graphics = rep.Graphics
	}
	for i, c := range rep.Cameras {
		if i == len(r.scenes) {
			r.scenes = append(r.scenes, "")
		}
		if c.Scene != "" {
			r.scenes[i] = c.Scene
		}
	}
	return rep
}

func (r *run) health(camera string) Report {
	r.t.Helper()
	rep, err := r.c.Setup(r.cfg, r.want(camera, ""), false, nil)
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
	wantResults(t, rep, map[string]string{"graphics": "created", "scene": "created", "overlay": "created", "camera": "created",
		"order": "fixed", "video": "fixed", "program": "fixed", "fit": "ok"})
	if !rep.Ready || rep.Camera != "DroidCam Video" || !slices.Equal(r.fitted, []string{"DroidCam Video"}) {
		t.Errorf("ready %v camera %q fitted %v", rep.Ready, rep.Camera, r.fitted)
	}
	if got := m.order(r.scenes[0]); !slices.Equal(got, []string{"DroidCam Video", r.graphics}) {
		t.Errorf("scene %v", got)
	}
	if got := m.order(r.graphics); !slices.Equal(got, []string{OverlayName}) {
		t.Errorf("graphics %v", got)
	}
	ov := m.input(OverlayName).settings
	if ov["url"] != overlayURL || ov["width"] != 1920.0 || ov["height"] != 1080.0 || ov["reroute_audio"] != true {
		t.Errorf("overlay %v", ov)
	}
	if m.input(probeName) != nil || m.program != r.scenes[0] || m.baseW != 1920 || m.outH != 1080 {
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
	wantResults(t, rep, map[string]string{"graphics": "created", "scene": "created", "overlay": "fixed", "camera": "fixed", "order": "fixed", "program": "fixed"})
	if rep.Camera != "Tablet" {
		t.Errorf("camera %q", rep.Camera)
	}
	if w := m.wrote(); slices.Contains(w, "CreateInput") || slices.Contains(w, "SetInputSettings") || slices.Contains(w, "RemoveInput") {
		t.Errorf("touched the user's sources: %v", w)
	}
	if got := m.order("Jogo"); !slices.Equal(got, []string{"Tablet", "Grafismo", "Logo"}) {
		t.Errorf("user scene changed: %v", got)
	}
	if got := m.order(r.scenes[0]); !slices.Equal(got, []string{"Tablet", r.graphics}) {
		t.Errorf("scene %v", got)
	}
	if got := m.order(r.graphics); !slices.Equal(got, []string{"Grafismo"}) {
		t.Errorf("graphics %v", got)
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
	if got := m.order(r.scenes[0]); !slices.Equal(got, []string{"(DroidCam Video)", "Câmara HDMI", r.graphics}) {
		t.Errorf("scene %v", got)
	}
	if got := m.order(r.graphics); !slices.Equal(got, []string{OverlayName}) {
		t.Errorf("graphics %v", got)
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
	if results(rep)["video"] != "later" || detail(rep, "video") != i18n.T("obs.setup.resolucao_depois", "resolucao", i18n.T("obs.setup.resolucao_saida", "base", "1920x1080", "saida", "1280x720")) ||
		!rep.Ready || results(rep)["program"] != "later" {
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
	if h.Ready || results(h)["camera"] != "failed" || detail(h, "camera") != i18n.T("obs.setup.camara_sumiu_escolha", "camara", "DroidCam Video") {
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
	if got := m.order(r.scenes[0]); !slices.Equal(got, []string{"(NDI Bancada)", "Webcam", r.graphics}) {
		t.Errorf("scene %v", got)
	}
	if got := m.order(r.graphics); !slices.Equal(got, []string{OverlayName}) {
		t.Errorf("graphics %v", got)
	}
}

func TestSetupBuildsGraphicsAndCameraScene(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("Webcam")}
	r := newRun(t, m)
	rep := r.setup("", "")
	if !rep.Ready || !slices.Contains(m.scenes, "Grafismo") || !slices.Contains(m.scenes, "Câmara 1") {
		t.Fatalf("ready %v scenes %v %v", rep.Ready, m.scenes, rep.Problems())
	}
	if got := m.order("Grafismo"); !slices.Equal(got, []string{OverlayName}) {
		t.Errorf("Grafismo %v", got)
	}
	if got := m.order("Câmara 1"); !slices.Equal(got, []string{"Webcam", "Grafismo"}) {
		t.Errorf("Câmara 1 %v", got)
	}
	if m.program != "Câmara 1" || rep.Graphics != "Grafismo" || !slices.Equal(rep.Cameras, []ReportCamera{{"Webcam", "Câmara 1"}}) {
		t.Errorf("program %q graphics %q cameras %v", m.program, rep.Graphics, rep.Cameras)
	}
	r.again("source:Webcam")
}

// oldLayout is OBS as 0.1 left it: Hallstream with the camera and the overlay, on program.
func oldLayout() *model {
	m := newModel()
	m.put(SceneName, "Tablet", "dshow_input", map[string]any{"video_device_id": "Tablet:x"})
	m.put(SceneName, OverlayName, "browser_source", map[string]any{"url": overlayURL, "width": 1920.0, "height": 1080.0, "reroute_audio": true})
	m.program = SceneName
	return m
}

func TestSetupFromOldLayout(t *testing.T) {
	m := oldLayout()
	r := newRun(t, m)
	rep := r.setup("source:Tablet", "")
	if !rep.Ready {
		t.Errorf("not ready: %v", rep.Problems())
	}
	if got := m.order(SceneName); !slices.Equal(got, []string{"Tablet", OverlayName}) {
		t.Errorf("Hallstream changed: %v", got)
	}
	if slices.Contains(m.wroteIn, SceneName) {
		t.Errorf("wrote to Hallstream: %v %v", m.writes, m.wroteIn)
	}
	if got := m.order("Câmara 1"); !slices.Equal(got, []string{"Tablet", "Grafismo"}) {
		t.Errorf("Câmara 1 %v", got)
	}
	if got := m.order("Grafismo"); !slices.Equal(got, []string{OverlayName}) {
		t.Errorf("Grafismo %v", got)
	}
	if m.program != "Câmara 1" {
		t.Errorf("program %q", m.program)
	}
	r.again("source:Tablet")
}

func TestSetupThreeCameras(t *testing.T) {
	m := newModel()
	for _, n := range []string{"A", "B", "C"} {
		m.put("Cena", n, "dshow_input", map[string]any{"video_device_id": n + ":x"})
	}
	r := newRun(t, m)
	r.extra = []string{"source:B", "source:C"}
	rep := r.setup("source:A", "")
	if !rep.Ready {
		t.Errorf("not ready: %v", rep.Problems())
	}
	for i, n := range []string{"A", "B", "C"} {
		scene := fmt.Sprintf("Câmara %d", i+1)
		if got := m.order(scene); !slices.Equal(got, []string{n, "Grafismo"}) {
			t.Errorf("%s %v", scene, got)
		}
	}
	if want := []string{"1 Câmara 1 A", "2 Câmara 2 B", "3 Câmara 3 C"}; !slices.Equal(r.fits, want) {
		t.Errorf("fits %v", r.fits)
	}
	wantResults(t, rep, map[string]string{"camera-2": "fixed", "order-3": "fixed", "fit-3": "ok"})
	r.again("source:A")
}

func TestSetupCameraTwoGone(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	r := newRun(t, m)
	r.extra = []string{"source:B"}
	rep := r.setup("source:A", "")
	if results(rep)["camera-2"] != "failed" || detail(rep, "camera-2") != i18n.T("obs.setup.camara_n_sumiu", "n", 2, "camara", "B") || rep.Ready {
		t.Errorf("camera-2 %s %q ready %v", results(rep)["camera-2"], detail(rep, "camera-2"), rep.Ready)
	}
	if results(rep)["camera"] != "fixed" || !slices.Equal(m.order("Câmara 1"), []string{"A", "Grafismo"}) || !slices.Equal(r.fits, []string{"1 Câmara 1 A"}) {
		t.Errorf("camera 1 %s %v fits %v", results(rep)["camera"], m.order("Câmara 1"), r.fits)
	}
}

func TestSetupSameSourceTwice(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	r := newRun(t, m)
	r.extra = []string{"source:A"}
	rep := r.setup("source:A", "")
	if results(rep)["camera-2"] != "failed" || detail(rep, "camera-2") != i18n.T("obs.setup.camara_repetida", "n", 2, "outra", 1) || rep.Ready {
		t.Errorf("camera-2 %s %q", results(rep)["camera-2"], detail(rep, "camera-2"))
	}
	if len(m.order("Câmara 2")) != 0 {
		t.Errorf("Câmara 2 %v", m.order("Câmara 2"))
	}
}

func TestSetupNameTaken(t *testing.T) {
	m := newModel()
	m.put("Jogo", "Tablet", "dshow_input", map[string]any{"video_device_id": "Tablet:x"})
	m.put("Jogo", "Grafismo", "browser_source", map[string]any{"url": overlayURL, "width": 1920.0, "height": 1080.0, "reroute_audio": true})
	m.put("Câmara 1", "Logo", "image_source", map[string]any{})
	r := newRun(t, m)
	rep := r.setup("", "")
	if rep.Graphics != "Grafismo 2" || len(rep.Cameras) != 1 || rep.Cameras[0].Scene != "Câmara 1 2" || !rep.Ready {
		t.Fatalf("graphics %q cameras %v %v", rep.Graphics, rep.Cameras, rep.Problems())
	}
	if got := m.order("Câmara 1"); !slices.Equal(got, []string{"Logo"}) || slices.Contains(m.wroteIn, "Câmara 1") {
		t.Errorf("the user's Câmara 1 changed: %v", got)
	}
	if got := m.order("Câmara 1 2"); !slices.Equal(got, []string{"Tablet", "Grafismo 2"}) {
		t.Errorf("Câmara 1 2 %v", got)
	}
	r.again("source:Tablet")
}

func TestSetupSavedNamesWin(t *testing.T) {
	m := newModel()
	m.put("Graphics", OverlayName, "browser_source", map[string]any{"url": overlayURL, "width": 1920.0, "height": 1080.0, "reroute_audio": true})
	m.put("Camera 1", "Webcam", "dshow_input", map[string]any{"video_device_id": "Webcam:x"})
	m.nest("Camera 1", "Graphics")
	m.program = "Camera 1"
	r := newRun(t, m)
	r.graphics, r.scenes = "Graphics", []string{"Camera 1"}
	r.again("source:Webcam")
	if slices.Contains(m.scenes, "Grafismo") || slices.Contains(m.scenes, "Câmara 1") {
		t.Errorf("made new scenes: %v", m.scenes)
	}
}

func TestSetupKeepsCameraTwoLive(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	m.put("Cena", "B", "dshow_input", map[string]any{"video_device_id": "B:x"})
	r := newRun(t, m)
	r.extra = []string{"source:B"}
	r.setup("source:A", "")
	m.program = r.scenes[1]
	r.again("source:A")
	if m.program != "Câmara 2" {
		t.Errorf("program %q", m.program)
	}
}

func TestSetupStreamingKeepsProgram(t *testing.T) {
	m := oldLayout()
	m.streaming = true
	r := newRun(t, m)
	rep := r.setup("source:Tablet", "")
	if results(rep)["program"] != "later" || !rep.Ready {
		t.Errorf("program %s ready %v %v", results(rep)["program"], rep.Ready, rep.Problems())
	}
	if slices.Contains(m.writes, "SetCurrentProgramScene") || m.program != SceneName {
		t.Errorf("switched scene while streaming: %q", m.program)
	}
	if !slices.Equal(m.order("Câmara 1"), []string{"Tablet", "Grafismo"}) || !slices.Equal(m.order("Grafismo"), []string{OverlayName}) {
		t.Errorf("Câmara 1 %v Grafismo %v", m.order("Câmara 1"), m.order("Grafismo"))
	}
}

func TestHealthOldLayoutReady(t *testing.T) {
	m := oldLayout()
	r := newRun(t, m)
	if h := r.health("source:Tablet"); !h.Ready {
		t.Errorf("health %v", h.Problems())
	}
	if w := m.wrote(); len(w) != 0 {
		t.Errorf("health wrote %v", w)
	}
}

func TestHealthOldLayoutTwoCameras(t *testing.T) {
	m := oldLayout()
	m.put("Cena", "B", "dshow_input", map[string]any{"video_device_id": "B:x"})
	r := newRun(t, m)
	r.extra = []string{"source:B"}
	if h := r.health("source:Tablet"); h.Ready {
		t.Error("two cameras: the old layout counted as ready")
	}
}

func TestHealthOldLayoutBroken(t *testing.T) {
	m := newModel()
	m.put(SceneName, "Tablet", "dshow_input", map[string]any{"video_device_id": "Tablet:x"})
	m.program = SceneName
	r := newRun(t, m)
	h := r.health("source:Tablet")
	if h.Ready || !strings.Contains(strings.Join(h.Problems(), " "), "Grafismo") {
		t.Errorf("health %v", h.Problems())
	}
}

func TestHealthAfterStreamingSetup(t *testing.T) {
	m := oldLayout()
	m.streaming = true
	r := newRun(t, m)
	r.setup("source:Tablet", "")
	if h := r.health("source:Tablet"); !h.Ready {
		t.Errorf("health %v", h.Problems())
	}
}

// dropAfter answers from m until it has made scene, then hangs up on every
// request, as OBS closing half way through.
type dropAfter struct {
	m     *model
	scene string
	mu    sync.Mutex
	gone  bool
}

func (d *dropAfter) answer(typ string, data map[string]any) (bool, int, any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gone {
		return false, hangUp, nil
	}
	ok, code, out := d.m.answer(typ, data)
	d.gone = typ == "CreateScene" && data["sceneName"] == d.scene
	return ok, code, out
}

// The scenes made before the connection dropped come back with the error,
// so the server can keep their names for the next run.
func TestSetupLostKeepsScenes(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	var c Client
	cfg := fakeOBS(t, "", &dropAfter{m: m, scene: "Câmara 1"})
	w := Want{OverlayURL: overlayURL, OverlayPorts: [2]int{8765, 8775}, Cameras: []WantCamera{{Choice: "source:A"}}}
	rep, err := c.Setup(cfg, w, true, nil)
	if err == nil {
		t.Fatal("want the lost connection's error")
	}
	if rep.Graphics != "Grafismo" || len(rep.Cameras) != 1 || rep.Cameras[0].Scene != "Câmara 1" {
		t.Errorf("graphics %q cameras %v", rep.Graphics, rep.Cameras)
	}
}

// With camera 1's scene refused, camera 2's goes on program.
func TestSetupProgramWithoutCameraOneScene(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	m.put("Cena", "B", "dshow_input", map[string]any{"video_device_id": "B:x"})
	r := newRun(t, m)
	r.scenes, r.extra = []string{"A"}, []string{"source:B"} // a saved name OBS now has as an input
	rep := r.setup("source:A", "")
	if results(rep)["scene"] != "failed" || m.program != "Câmara 2" {
		t.Errorf("scene %s, program %q", results(rep)["scene"], m.program)
	}
}

// OBS names are unique across scenes and inputs, so a device named like a
// scene gets a number.
func TestSetupDeviceNamedLikeScene(t *testing.T) {
	m := newModel()
	m.put("HD Webcam", "Logo", "image_source", map[string]any{})
	m.devices = []map[string]any{device("HD Webcam")}
	r := newRun(t, m)
	rep := r.setup("", "")
	if results(rep)["camera"] != "created" || rep.Camera != "HD Webcam 2" {
		t.Errorf("camera %q %s: %s", rep.Camera, results(rep)["camera"], detail(rep, "camera"))
	}
}

// A camera scene the user deleted comes back under its saved name.
func TestSetupRemakesSavedScene(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	r := newRun(t, m)
	r.scenes = []string{"Câmara Norte"}
	rep := r.setup("source:A", "")
	if results(rep)["scene"] != "created" || !slices.Equal(m.order("Câmara Norte"), []string{"A", "Grafismo"}) ||
		slices.Contains(m.scenes, "Câmara 1") || rep.Cameras[0].Scene != "Câmara Norte" {
		t.Errorf("scene %s, scenes %v, cameras %v", results(rep)["scene"], m.scenes, rep.Cameras)
	}
}

// Camera 2 saved with no source fails its own step and says so; camera 1
// is set up and fitted.
func TestSetupCameraTwoWithoutSource(t *testing.T) {
	m := newModel()
	m.put("Cena", "A", "dshow_input", map[string]any{"video_device_id": "A:x"})
	r := newRun(t, m)
	r.extra = []string{"source:"}
	rep := r.setup("source:A", "")
	if results(rep)["camera-2"] != "failed" || detail(rep, "camera-2") != i18n.T("obs.setup.camara_n_sem_fonte", "n", 2) {
		t.Errorf("camera-2 %s %q", results(rep)["camera-2"], detail(rep, "camera-2"))
	}
	if !slices.Equal(r.fits, []string{"1 Câmara 1 A"}) || results(rep)["fit"] != "ok" {
		t.Errorf("fits %v", r.fits)
	}
}

// setupWant runs Configurar OBS (apply) or the health check on w.
func (r *run) setupWant(w Want, apply bool) Report {
	r.t.Helper()
	w.OverlayURL, w.OverlayPorts = overlayURL, [2]int{8765, 8775}
	var fit func(int, string, string) error
	if apply {
		fit = func(int, string, string) error { return nil }
	}
	rep, err := r.c.Setup(r.cfg, w, apply, fit)
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

// A phone camera gets a browser source on its VDO.Ninja view link, kept
// loaded off air; the next runs find it by its link and put the link back.
func TestSetupPhone(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("Logitech C920")}
	r := newRun(t, m)
	rep := r.setupWant(Want{Cameras: []WantCamera{{Choice: "device:" + device("Logitech C920")["itemValue"].(string)}, {Choice: "vdo"}}}, true)
	wantResults(t, rep, map[string]string{"camera": "created", "camera-2": "created", "order-2": "fixed"})
	if len(rep.Cameras) != 2 || rep.Cameras[1].Source != "Telemóvel 1" || len(rep.Phones) != 1 {
		t.Fatalf("cameras %+v phones %v", rep.Cameras, rep.Phones)
	}
	id, scene := rep.Phones["Telemóvel 1"], rep.Cameras[1].Scene
	in := m.input("Telemóvel 1")
	if in == nil || in.kind != "browser_source" {
		t.Fatalf("phone source %+v", in)
	}
	if id == "" || str(in.settings["url"]) != VDOView(id) || in.settings["width"] != 1920.0 || in.settings["height"] != 1080.0 ||
		in.settings["shutdown"] != false || in.settings["restart_when_active"] != false || in.settings["is_local_file"] != false {
		t.Errorf("phone settings %v, id %q", in.settings, id)
	}
	if got := m.order(scene); !slices.Equal(got, []string{"Telemóvel 1", rep.Graphics}) {
		t.Errorf("camera 2's scene %v", got)
	}

	saved := Want{Graphics: rep.Graphics, Cameras: []WantCamera{{Choice: "source:Logitech C920", Scene: rep.Cameras[0].Scene},
		{Choice: "source:Telemóvel 1", Scene: scene, Vdo: id}}}
	m.wrote()
	if rep := r.setupWant(saved, true); !rep.Ready || rep.Phones["Telemóvel 1"] != id || rep.Cameras[1].Source != "Telemóvel 1" {
		t.Errorf("again: ready %v cameras %+v phones %v %v", rep.Ready, rep.Cameras, rep.Phones, rep.Problems())
	}
	if w := m.wrote(); slices.Contains(w, "CreateInput") {
		t.Errorf("again wrote %v", w)
	}
	if h := r.setupWant(saved, false); !h.Ready {
		t.Errorf("health %v", h.Problems())
	}

	in.settings["url"] = "https://vdo.ninja/?view=someoneelse"
	h := r.setupWant(saved, false)
	if h.Ready || results(h)["camera-2"] != "failed" || detail(h, "camera-2") != i18n.T("obs.setup.telemovel_endereco", "fonte", "Telemóvel 1") {
		t.Errorf("health of a moved link: %s %q", results(h)["camera-2"], detail(h, "camera-2"))
	}
	rep = r.setupWant(saved, true)
	if results(rep)["camera-2"] != "fixed" || str(in.settings["url"]) != VDOView(id) || in.settings["width"] != 1920.0 {
		t.Errorf("fix: %s %q settings %v", results(rep)["camera-2"], detail(rep, "camera-2"), in.settings)
	}
	if h := r.setupWant(saved, false); !h.Ready {
		t.Errorf("health after the fix %v", h.Problems())
	}
}

// A second phone through the DroidCam plugin: the plugin is offered with a
// source already there, and the new source takes the phone no source uses.
func TestSetupSecondDroidCam(t *testing.T) {
	m := newModel()
	m.kinds = []string{"browser_source", PluginKind}
	m.put("Cena", "DroidCam", PluginKind, map[string]any{"device_list": "A"})
	m.phones = []map[string]any{{"itemName": "Phone A", "itemEnabled": true, "itemValue": "A"},
		{"itemName": "Phone B", "itemEnabled": true, "itemValue": "B"},
		{"itemName": "Use WiFi IP", "itemEnabled": true, "itemValue": "dev_id_wifi"}}
	r := newRun(t, m)
	list, err := r.c.Cameras(r.cfg)
	if err != nil || !slices.ContainsFunc(list, func(c Choice) bool { return c.Value == "plugin" }) {
		t.Fatalf("choices %v %v", list, err)
	}
	rep := r.setupWant(Want{Cameras: []WantCamera{{Choice: "source:DroidCam"}, {Choice: "plugin"}}}, true)
	cam := m.input("DroidCam 2")
	if results(rep)["camera-2"] != "created" || rep.Cameras[1].Source != "DroidCam 2" || cam == nil || cam.settings["device_list"] != "B" ||
		!slices.Equal(m.pressed, []string{"DroidCam 2/connect"}) {
		t.Errorf("camera 2 %s %q %+v pressed %v", results(rep)["camera-2"], detail(rep, "camera-2"), cam, m.pressed)
	}
	if m.input("DroidCam").settings["device_list"] != "A" {
		t.Errorf("first source moved: %v", m.input("DroidCam").settings)
	}
}

// Two new phones in one run each get their own source and id.
func TestSetupTwoNewPhones(t *testing.T) {
	m := newModel()
	m.devices = []map[string]any{device("Logitech C920")}
	r := newRun(t, m)
	rep := r.setupWant(Want{Cameras: []WantCamera{{Choice: "device:" + device("Logitech C920")["itemValue"].(string)}, {Choice: "vdo"}, {Choice: "vdo"}}}, true)
	wantResults(t, rep, map[string]string{"camera": "created", "camera-2": "created", "camera-3": "created"})
	if len(rep.Cameras) != 3 || rep.Cameras[1].Source != "Telemóvel 1" || rep.Cameras[2].Source != "Telemóvel 2" {
		t.Fatalf("cameras %+v", rep.Cameras)
	}
	a, b := rep.Phones["Telemóvel 1"], rep.Phones["Telemóvel 2"]
	if a == "" || b == "" || a == b || str(m.input("Telemóvel 2").settings["url"]) != VDOView(b) {
		t.Errorf("phones %v", rep.Phones)
	}
}

// Two new DroidCam plugin sources in one run take different phones.
func TestSetupTwoNewPlugins(t *testing.T) {
	m := newModel()
	m.kinds = []string{"browser_source", PluginKind}
	m.phones = []map[string]any{{"itemName": "Phone A", "itemEnabled": true, "itemValue": "A"},
		{"itemName": "Phone B", "itemEnabled": true, "itemValue": "B"},
		{"itemName": "Use WiFi IP", "itemEnabled": true, "itemValue": "dev_id_wifi"}}
	r := newRun(t, m)
	rep := r.setupWant(Want{Cameras: []WantCamera{{Choice: "plugin"}, {Choice: "plugin"}}}, true)
	wantResults(t, rep, map[string]string{"camera": "created", "camera-2": "created"})
	one, two := m.input("DroidCam"), m.input("DroidCam 2")
	if rep.Cameras[1].Source != "DroidCam 2" || one == nil || two == nil || one.settings["device_list"] != "A" || two.settings["device_list"] != "B" {
		t.Errorf("cameras %+v, %+v %+v", rep.Cameras, one, two)
	}
}
