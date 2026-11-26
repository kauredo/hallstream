package obsws

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

// What "Configurar OBS" makes in OBS.
const (
	SceneName   = "Hallstream"
	OverlayName = "Hallstream overlay"
	// PluginKind is the DroidCam OBS plugin's source (droidcam-obs-plugin, src/plugin.cc).
	PluginKind = "droidcam_obs"
	dshowKind  = "dshow_input"
	probeName  = "Hallstream (à procura de câmaras)"
)

// Step is one line of the checklist. Result is "ok", "created", "fixed",
// "failed", "choose" (the camera needs picking) or "later" (it can't change
// while OBS streams or records).
type Step struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Result string `json:"result"`
	Detail string `json:"detail"`
}

// Choice is a camera the volunteer can pick: a source already in OBS
// ("source:<name>"), a video capture device to add ("device:<id>"), the
// DroidCam OBS plugin ("plugin") or a new phone through VDO.Ninja ("vdo",
// which the panel offers itself).
type Choice struct {
	Value    string `json:"value"`
	Type     string `json:"type"` // source, device, plugin, phone
	Label    string `json:"label"`
	DroidCam bool   `json:"droidcam"`
}

// WantCamera is one camera the setup aims for.
type WantCamera struct {
	// Choice is a Choice value, or "" to find one (camera 1 only): a
	// DroidCam source first.
	Choice string
	// Scene is the saved scene name; "" before the first setup.
	Scene string
	// Old is the camera before; it is hidden in the scene when another replaces it.
	Old string
	// Vdo is the stream id of a saved phone camera, whose Choice is its source.
	Vdo string
}

// Want is what the setup aims for.
type Want struct {
	OverlayURL   string
	OverlayPorts [2]int // the ports the program may run on: a browser source at /overlay on one of them is ours
	// Graphics is the saved overlay scene name; "" before the first setup.
	Graphics string
	// Cameras is 1 to 3 cameras, camera 1 first.
	Cameras []WantCamera
}

// ReportCamera is the scene and source a camera ended with.
type ReportCamera struct {
	Source string `json:"source"`
	Scene  string `json:"scene"`
}

// Report is the checklist and the scenes and cameras it ended with.
type Report struct {
	Steps    []Step         `json:"steps"`
	Camera   string         `json:"camera"` // camera 1's source
	Graphics string         `json:"graphics"`
	Cameras  []ReportCamera `json:"cameras"`
	Choices  []Choice       `json:"choices,omitempty"`
	// Phones is each phone camera's stream id, by source.
	Phones map[string]string `json:"phones,omitempty"`
	// Ready: nothing is missing or wrong (the camera fit aside).
	Ready bool `json:"ready"`
}

// Problems are the details of the failed steps.
func (r Report) Problems() []string {
	p := []string{}
	for _, s := range r.Steps {
		if s.Result == "failed" || s.Result == "choose" {
			p = append(p, s.Detail)
		}
	}
	return p
}

// Setup checks OBS against w and, with apply, fixes what it can: the
// graphics scene with the overlay browser source, one scene per camera
// with the camera and the graphics scene above it, the canvas size and the
// program scene, then fit(n, scene, camera) for each camera. It never
// deletes or changes a source or scene the user made, other than the
// overlay's own settings. Without apply it only reports (the health
// check), and with one camera it also accepts the old Hallstream scene.
// The error is for a lost connection, and comes with the report so far,
// whose scenes exist; whatever OBS refuses becomes a failed step.
func (c *Client) Setup(cfg Config, w Want, apply bool, fit func(n int, scene, camera string) error) (Report, error) {
	s := &setup{c: c, cfg: cfg, apply: apply, want: w}
	s.run(fit)
	if s.err != nil {
		return s.rep, s.err
	}
	s.ready()
	if !apply && !s.rep.Ready && len(w.Cameras) <= 1 {
		old := &setup{c: c, cfg: cfg, want: w}
		old.legacy(s.rep.Cameras)
		if old.err != nil {
			return Report{}, old.err
		}
		if old.ready(); old.rep.Ready {
			return old.rep, nil
		}
	}
	return s.rep, nil
}

func (s *setup) ready() {
	s.rep.Ready = true
	for _, st := range s.rep.Steps {
		if (st.Result == "failed" || st.Result == "choose") && !strings.HasPrefix(st.ID, "fit") {
			s.rep.Ready = false
		}
	}
}

// Cameras lists what "Câmara" can offer: video sources in OBS, the video
// capture devices OBS sees and the DroidCam plugin, DroidCam first.
func (c *Client) Cameras(cfg Config) ([]Choice, error) {
	s := &setup{c: c, cfg: cfg}
	var sc struct {
		Name string `json:"sceneName"`
		Old  string `json:"currentProgramSceneName"`
	}
	if s.call("GetCurrentProgramScene", nil, &sc); s.err != nil {
		return nil, s.err
	}
	s.scene = sc.Name
	if s.scene == "" {
		s.scene = sc.Old
	}
	in := s.inputs()
	list := s.choices(in)
	return list, s.err
}

type setup struct {
	c      *Client
	cfg    Config
	apply  bool
	want   Want
	rep    Report
	err    error    // the connection failed: stop
	scene  string   // where new sources go
	at     string   // the scene being checked
	scenes []string // every scene in OBS
	n      int      // the camera being set up, from 1
	cam    WantCamera
	vdo    string // the stream id of the camera being set up, when it is a phone
}

type input struct {
	Name     string         `json:"inputName"`
	Kind     string         `json:"unversionedInputKind"`
	Settings map[string]any `json:"-"`
}

// call sends one request. A refusal comes back as the error; a lost
// connection is kept in s.err and every later call does nothing.
func (s *setup) call(typ string, data, into any) *RequestError {
	if s.err != nil {
		return &RequestError{Type: typ, Comment: s.err.Error()}
	}
	err := s.c.Request(s.cfg, typ, data, into)
	if re, ok := err.(*RequestError); ok {
		return re
	}
	if err != nil {
		s.err = err
		return &RequestError{Type: typ, Comment: err.Error()}
	}
	return nil
}

func (s *setup) add(st Step) { s.rep.Steps = append(s.rep.Steps, st) }

func (s *setup) run(fit func(int, string, string) error) {
	busy := s.busy()
	in := s.inputs()
	if s.err != nil {
		return
	}
	graphics := s.want.Graphics
	if graphics == "" {
		graphics = i18n.T("obs.cena_grafismo")
	}
	if !s.listScenes("graphics", graphics) {
		return
	}
	if s.want.Graphics == "" {
		graphics = uniqueSourceName(in, s.scenes, graphics)
	}
	ok := s.ensureScene("graphics", graphics)
	if slices.Contains(s.scenes, graphics) {
		s.rep.Graphics = graphics
	}
	if !ok && s.apply || s.err != nil {
		return
	}
	s.at = graphics
	s.overlay(in)
	for i, w := range s.want.Cameras {
		s.n, s.cam, s.vdo = i+1, w, ""
		scene := w.Scene
		if scene == "" {
			scene = uniqueSourceName(in, s.scenes, i18n.T("obs.cena_camara", "n", s.n))
		}
		var rc ReportCamera
		if s.ensureScene(s.id("scene"), scene) || !s.apply {
			s.at, s.scene = scene, scene
			cam := s.camera(in)
			s.hideOld(cam, graphics)
			s.order(graphics, cam)
			rc.Source = cam
			if cam != "" && s.vdo != "" {
				if s.rep.Phones == nil {
					s.rep.Phones = map[string]string{}
				}
				s.rep.Phones[cam] = s.vdo
			}
			if slices.Contains(s.scenes, scene) {
				rc.Scene = scene
			}
		}
		if i == 0 {
			s.rep.Camera = rc.Source
		}
		s.rep.Cameras = append(s.rep.Cameras, rc)
		if s.err != nil {
			return
		}
	}
	s.video(busy)
	var live []string
	for _, c := range s.rep.Cameras {
		if c.Scene != "" {
			live = append(live, c.Scene)
		}
	}
	first := ""
	if len(s.rep.Cameras) > 0 {
		first = s.rep.Cameras[0].Scene
	}
	if first == "" && len(live) > 0 {
		first = live[0]
	}
	s.program(busy, live, first)
	if !s.apply || fit == nil {
		return
	}
	for i, c := range s.rep.Cameras {
		if c.Source == "" || c.Scene == "" || s.err != nil {
			continue
		}
		s.n = i + 1
		st := Step{ID: s.id("fit"), Label: i18n.T("obs.setup.ajuste"), Result: "ok", Detail: i18n.T("obs.setup.ajuste_ok")}
		if err := fit(s.n, c.Scene, c.Source); err != nil {
			st.Result, st.Detail = "failed", err.Error()
		}
		s.add(st)
	}
}

// legacy checks the layout before several cameras: the camera and the
// overlay in the one Hallstream scene. On program it also accepts the
// camera scenes the new layout has.
func (s *setup) legacy(cams []ReportCamera) {
	busy := s.busy()
	s.n, s.at = 1, SceneName
	if len(s.want.Cameras) > 0 {
		s.cam = s.want.Cameras[0]
	}
	if !s.listScenes("scene", SceneName) || !s.ensureScene("scene", SceneName) {
		return
	}
	in := s.inputs()
	overlay := s.overlay(in)
	cam := s.camera(in)
	if s.err != nil {
		return
	}
	s.rep.Camera = cam
	s.order(overlay, cam)
	s.video(busy)
	live := []string{SceneName}
	for _, c := range cams {
		if c.Scene != "" {
			live = append(live, c.Scene)
		}
	}
	s.program(busy, live, SceneName)
}

// id is a step's id for the camera being set up: "camera" for camera 1,
// "camera-2" for camera 2.
func (s *setup) id(base string) string {
	if s.n <= 1 {
		return base
	}
	return base + "-" + strconv.Itoa(s.n)
}

// busy reports whether OBS is streaming or recording: the canvas can't change then.
func (s *setup) busy() bool {
	var st, rec struct {
		Active bool `json:"outputActive"`
	}
	s.call("GetStreamStatus", nil, &st)
	s.call("GetRecordStatus", nil, &rec)
	return st.Active || rec.Active
}

func sceneLabel(id, name string) string {
	if id == "graphics" {
		return i18n.T("obs.setup.cena_grafismo", "cena", name)
	}
	return i18n.T("obs.setup.cena", "cena", name)
}

// listScenes reads the scenes once; when OBS refuses, the step id fails.
func (s *setup) listScenes(id, name string) bool {
	var r struct {
		Scenes []struct {
			Name string `json:"sceneName"`
		} `json:"scenes"`
	}
	if re := s.call("GetSceneList", nil, &r); re != nil {
		s.add(Step{ID: id, Label: sceneLabel(id, name), Result: "failed", Detail: i18n.T("obs.setup.sem_lista_cenas", "erro", re.Comment)})
		return false
	}
	s.scenes = nil
	for _, sc := range r.Scenes {
		s.scenes = append(s.scenes, sc.Name)
	}
	return true
}

// ensureScene reports whether the scene exists, making it with apply.
func (s *setup) ensureScene(id, name string) bool {
	st := Step{ID: id, Label: sceneLabel(id, name), Result: "ok", Detail: i18n.T("obs.setup.cena_existe")}
	defer func() { s.add(st) }()
	if slices.Contains(s.scenes, name) {
		return true
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.falta_cena", "cena", name)
		return false
	}
	if re := s.call("CreateScene", map[string]string{"sceneName": name}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_criou_cena", "erro", re.Comment)
		return false
	}
	s.scenes = append(s.scenes, name)
	st.Result, st.Detail = "created", i18n.T("obs.setup.criei_cena", "cena", name)
	return true
}

func (s *setup) inputs() []input {
	var r struct {
		Inputs []input `json:"inputs"`
	}
	if re := s.call("GetInputList", nil, &r); re != nil {
		if s.err == nil {
			s.err = i18n.Err("obs.setup.sem_lista_fontes", "erro", re.Comment)
		}
		return nil
	}
	for i := range r.Inputs {
		if k := r.Inputs[i].Kind; k == "browser_source" || k == dshowKind || k == PluginKind {
			var st struct {
				Settings map[string]any `json:"inputSettings"`
			}
			s.call("GetInputSettings", map[string]string{"inputName": r.Inputs[i].Name}, &st)
			r.Inputs[i].Settings = st.Settings
		}
	}
	return r.Inputs
}

// items is the checked scene's items; none when the scene is missing.
func (s *setup) items() []sceneItem { return s.itemsIn(s.at) }

func (s *setup) itemsIn(scene string) []sceneItem {
	var r struct {
		Items []sceneItem `json:"sceneItems"`
	}
	if s.call("GetSceneItemList", map[string]string{"sceneName": scene}, &r) != nil {
		return nil
	}
	return r.Items
}

func find(items []sceneItem, name string) *sceneItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

// isOverlay: a browser source at /overlay on one of the program's ports.
func (s *setup) isOverlay(in input) bool {
	if in.Kind != "browser_source" {
		return false
	}
	u, err := url.Parse(str(in.Settings["url"]))
	if err != nil || strings.TrimSuffix(u.Path, "/") != "/overlay" {
		return false
	}
	p, _ := strconv.Atoi(u.Port())
	return p >= s.want.OverlayPorts[0] && p <= s.want.OverlayPorts[1]
}

func str(v any) string { s, _ := v.(string); return s }

func num(v any, def float64) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return def
}

func flag(v any) bool { b, _ := v.(bool); return b }

func (s *setup) overlay(in []input) string {
	st := Step{ID: "overlay", Label: i18n.T("obs.setup.grafismo"), Result: "ok", Detail: i18n.T("obs.setup.grafismo_aponta", "url", s.want.OverlayURL)}
	defer func() { s.add(st) }()
	items := s.items()
	// Ours by name first, then one already in the scene.
	var found *input
	best := -1
	for i := range in {
		if !s.isOverlay(in[i]) {
			continue
		}
		score := 0
		if in[i].Name == OverlayName {
			score = 2
		} else if find(items, in[i].Name) != nil {
			score = 1
		}
		if score > best {
			found, best = &in[i], score
		}
	}
	settings := map[string]any{"url": s.want.OverlayURL, "is_local_file": false,
		"width": 1920, "height": 1080, "reroute_audio": true}
	if found == nil {
		if !s.apply {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.falta_grafismo")
			return ""
		}
		name := uniqueSourceName(in, s.scenes, OverlayName)
		if re := s.call("CreateInput", map[string]any{"sceneName": s.at, "inputName": name,
			"inputKind": "browser_source", "inputSettings": settings}, nil); re != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_criou_grafismo", "erro", re.Comment)
			return ""
		}
		st.Result, st.Detail = "created", i18n.T("obs.setup.criei_grafismo", "fonte", name)
		return name
	}
	// GetInputSettings leaves out what is at obs-browser's defaults.
	cur := found.Settings
	var fixes []string
	if str(cur["url"]) != s.want.OverlayURL || flag(cur["is_local_file"]) {
		fixes = append(fixes, i18n.T("obs.setup.mal_endereco", "url", str(cur["url"])))
	}
	if num(cur["width"], 800) != 1920 || num(cur["height"], 600) != 1080 {
		fixes = append(fixes, i18n.T("obs.setup.mal_tamanho", "tamanho", fmt.Sprintf("%.0fx%.0f", num(cur["width"], 800), num(cur["height"], 600))))
	}
	if !flag(cur["reroute_audio"]) {
		fixes = append(fixes, i18n.T("obs.setup.mal_som"))
	}
	wrong := len(fixes) > 0
	it := find(items, found.Name)
	if it == nil {
		fixes = append(fixes, i18n.T("obs.setup.mal_fora_cena"))
	} else if !it.Enabled {
		fixes = append(fixes, i18n.T("obs.setup.mal_escondida"))
	}
	if len(fixes) == 0 {
		return found.Name
	}
	what := strings.Join(fixes, ", ")
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.fonte_mal", "fonte", found.Name, "o_que", what)
		return found.Name
	}
	if wrong {
		if re := s.call("SetInputSettings", map[string]any{"inputName": found.Name, "inputSettings": settings, "overlay": true}, nil); re != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_corrigiu_grafismo", "erro", re.Comment)
			return found.Name
		}
	}
	if re := s.show(it, found.Name); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_pos_grafismo", "erro", re.Comment)
		return found.Name
	}
	st.Result, st.Detail = "fixed", i18n.T("obs.setup.corrigi_fonte", "fonte", found.Name, "o_que", what)
	return found.Name
}

// show puts the source in the scene (it == nil) or unhides its item.
func (s *setup) show(it *sceneItem, name string) *RequestError {
	if it == nil {
		return s.call("CreateSceneItem", map[string]any{"sceneName": s.at, "sourceName": name, "sceneItemEnabled": true}, nil)
	}
	if !it.Enabled {
		return s.call("SetSceneItemEnabled", map[string]any{"sceneName": s.at, "sceneItemId": it.ID, "sceneItemEnabled": true}, nil)
	}
	return nil
}

// uniqueSourceName is name, numbered when an input or a scene has it: OBS
// names are unique across inputs and scenes.
func uniqueSourceName(in []input, scenes []string, name string) string {
	taken := func(n string) bool {
		return slices.Contains(scenes, n) || slices.ContainsFunc(in, func(x input) bool { return x.Name == n })
	}
	out := name
	for i := 2; taken(out); i++ {
		out = name + " " + strconv.Itoa(i)
	}
	return out
}

// isCapture is a camera kind the program knows.
func isCapture(kind string) bool {
	k := strings.ToLower(kind)
	return k == dshowKind || k == "v4l2_input" || strings.HasPrefix(k, "av_capture_input") ||
		k == "macos-avcapture" || strings.Contains(k, "droidcam")
}

// notCamera are the source kinds that can't be a camera.
var notCamera = []string{"browser_source", "image_source", "color_source", "color_source_v2", "color_source_v3",
	"slideshow", "slideshow_v2", "text_gdiplus", "text_gdiplus_v2", "text_gdiplus_v3", "text_ft2_source", "text_ft2_source_v2",
	"monitor_capture", "window_capture", "game_capture", "display_capture", "screen_capture", "xshm_input", "xcomposite_input",
	"pipewire-desktop-capture-source", "pipewire-window-capture-source"}

// isVideo is any source that may be a camera: capture devices, NDI,
// network streams and the like, but not audio, text, images or screens.
func isVideo(kind string) bool {
	k := strings.ToLower(kind)
	if isCapture(k) {
		return true
	}
	return k != "" && !slices.Contains(notCamera, k) && !strings.Contains(k, "audio") &&
		!strings.HasPrefix(k, "wasapi") && !strings.HasPrefix(k, "pulse") && !strings.HasPrefix(k, "alsa") && !strings.HasPrefix(k, "jack")
}

func isDroidCam(in input) bool {
	return strings.Contains(strings.ToLower(in.Kind), "droidcam") || strings.Contains(strings.ToLower(in.Name), "droidcam") ||
		in.Kind == dshowKind && strings.Contains(strings.ToLower(str(in.Settings["video_device_id"])), "droidcam")
}

// deviceName is the friendly name in a win-dshow device id, which is the
// name and the path, each with # and : escaped as #22 and #3A, joined by ":".
func deviceName(id string) string {
	name, _, _ := strings.Cut(id, ":")
	return strings.NewReplacer("#3A", ":", "#22", "#").Replace(name)
}

func (s *setup) camera(in []input) string {
	st := Step{ID: "camera", Label: i18n.T("obs.setup.camara"), Result: "ok"}
	if s.n > 1 {
		st.ID, st.Label = s.id("camera"), i18n.T("obs.setup.camara_n", "n", s.n)
	}
	defer func() { s.add(st) }()
	w := s.cam.Choice
	byName := func(n string) *input {
		for i := range in {
			if in[i].Name == n {
				return &in[i]
			}
		}
		return nil
	}
	var name, note string
	switch {
	case w == "vdo" || s.cam.Vdo != "":
		saved := ""
		if w != "vdo" {
			saved = strings.TrimPrefix(w, "source:")
		}
		var done bool
		if name, note, done = s.phone(&st, in, saved); done {
			return name
		}
	case s.n > 1 && w != "plugin" && !strings.HasPrefix(w, "device:"):
		// Cameras 2 and 3 come from the settings and are never picked here.
		name = strings.TrimPrefix(w, "source:")
		if name == "" {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.camara_n_sem_fonte", "n", s.n)
			return ""
		}
		if !strings.HasPrefix(w, "source:") || byName(name) == nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.camara_n_sumiu", "n", s.n, "camara", name)
			return ""
		}
	case strings.HasPrefix(w, "device:"):
		id := strings.TrimPrefix(w, "device:")
		for _, x := range in {
			if x.Kind == dshowKind && str(x.Settings["video_device_id"]) == id {
				name = x.Name
			}
		}
		if name == "" {
			if !s.apply {
				st.Result, st.Detail = "failed", i18n.T("obs.setup.juntar_camara", "camara", deviceName(id))
				return ""
			}
			return s.addDevice(&st, in, id)
		}
	case w == "plugin":
		others := s.others()
		for _, x := range in {
			if x.Kind == PluginKind && !slices.Contains(others, x.Name) {
				name = x.Name
			}
		}
		if name == "" {
			if !s.apply {
				st.Result, st.Detail = "failed", i18n.T("obs.setup.juntar_droidcam")
				return ""
			}
			return s.addPlugin(&st, in)
		}
	case strings.HasPrefix(w, "source:"):
		name = strings.TrimPrefix(w, "source:")
		if byName(name) == nil {
			if !s.apply {
				st.Result, st.Detail = "failed", i18n.T("obs.setup.camara_sumiu_escolha", "camara", name)
				return ""
			}
			note = i18n.T("obs.setup.camara_sumiu", "camara", name) + " "
			name = ""
		}
	}
	if name == "" {
		name = s.auto(&st, in, note)
		if name == "" || st.Result != "ok" {
			return name
		}
	}
	for i, c := range s.rep.Cameras {
		if c.Source == name {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.camara_repetida", "n", s.n, "outra", i+1)
			return ""
		}
	}
	it := find(s.items(), name)
	if it != nil && it.Enabled {
		if st.Detail == "" {
			st.Detail = i18n.T("obs.setup.usa_fonte", "fonte", name)
		}
		return name
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.camara_fora_cena", "camara", name, "cena", s.at)
		return name
	}
	if re := s.show(it, name); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_pos_camara", "erro", re.Comment)
		return name
	}
	st.Result, st.Detail = "fixed", note+i18n.T("obs.setup.pus_fonte", "fonte", name)
	return name
}

// phone finds the phone camera's browser source: the one on its view link,
// else the saved source (its link put back), else a new one. With done the
// step is decided.
func (s *setup) phone(st *Step, in []input, saved string) (name, note string, done bool) {
	id := s.cam.Vdo
	if id == "" {
		var err error
		if id, err = NewVDOID(); err != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_juntou_telemovel", "erro", err)
			return "", "", true
		}
	}
	s.vdo = id
	for _, x := range in {
		if x.Kind == "browser_source" && str(x.Settings["url"]) == VDOView(id) {
			return x.Name, "", false
		}
	}
	for _, x := range in {
		if saved == "" || x.Name != saved || x.Kind != "browser_source" {
			continue
		}
		if !s.apply {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.telemovel_endereco", "fonte", x.Name)
			return x.Name, "", true
		}
		if re := s.call("SetInputSettings", map[string]any{"inputName": x.Name, "inputSettings": map[string]any{"url": VDOView(id)}, "overlay": true}, nil); re != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_corrigiu_telemovel", "fonte", x.Name, "erro", re.Comment)
			return x.Name, "", true
		}
		note = i18n.T("obs.setup.telemovel_corrigido", "fonte", x.Name)
		st.Result, st.Detail = "fixed", note
		return x.Name, note + " ", false
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.falta_telemovel", "n", s.n)
		return "", "", true
	}
	return s.addPhone(st, in, id), "", true
}

// addPhone adds a browser source on the phone's view link, named
// "Telemóvel N". It stays loaded off air, so the phone never reconnects.
func (s *setup) addPhone(st *Step, in []input, id string) string {
	var name string
	for k := 1; ; k++ {
		if name = i18n.T("obs.setup.telemovel_nome", "n", k); uniqueSourceName(in, s.scenes, name) == name {
			break
		}
	}
	settings := map[string]any{"url": VDOView(id), "width": 1920, "height": 1080, "is_local_file": false,
		"shutdown": false, "restart_when_active": false}
	if re := s.call("CreateInput", map[string]any{"sceneName": s.at, "inputName": name, "inputKind": "browser_source",
		"inputSettings": settings}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_juntou_telemovel", "erro", re.Comment)
		return ""
	}
	st.Result, st.Detail = "created", i18n.T("obs.setup.juntei_telemovel", "fonte", name)
	return name
}

// others are the sources the other cameras ended with or ask for.
func (s *setup) others() []string {
	var out []string
	for _, c := range s.rep.Cameras {
		out = append(out, c.Source)
	}
	for i, c := range s.want.Cameras {
		if name, ok := strings.CutPrefix(c.Choice, "source:"); ok && i+1 != s.n {
			out = append(out, name)
		}
	}
	return out
}

// auto finds the camera with none chosen: a DroidCam source, else a camera
// already in the scene or in the old Hallstream scene, else the only
// choice. With several, the volunteer picks.
func (s *setup) auto(st *Step, in []input, note string) string {
	lists := [][]sceneItem{s.items()}
	if s.at != SceneName && slices.Contains(s.scenes, SceneName) {
		lists = append(lists, s.itemsIn(SceneName))
	}
	shown := func(name string) bool {
		return slices.ContainsFunc(lists, func(items []sceneItem) bool { return find(items, name) != nil })
	}
	var best string
	for _, x := range in {
		if isDroidCam(x) && (best == "" || shown(x.Name)) {
			best = x.Name
		}
	}
	if best != "" {
		return best
	}
	for _, items := range lists {
		for i := len(items) - 1; i >= 0; i-- {
			if isVideo(items[i].Kind) && items[i].Name != probeName {
				return items[i].Name
			}
		}
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.falta_camara", "cena", s.at)
		return ""
	}
	list := s.choices(in)
	switch len(list) {
	case 0:
		st.Result, st.Detail = "failed", note+i18n.T("obs.setup.nenhuma_camara")
		return ""
	case 1:
		return s.use(st, in, list[0])
	}
	st.Result, st.Detail = "choose", note+i18n.T("obs.setup.varias_camaras")
	s.rep.Choices = list
	return ""
}

func (s *setup) use(st *Step, in []input, c Choice) string {
	switch c.Type {
	case "device":
		return s.addDevice(st, in, strings.TrimPrefix(c.Value, "device:"))
	case "plugin":
		return s.addPlugin(st, in)
	}
	return strings.TrimPrefix(c.Value, "source:")
}

// choices: the video sources in OBS, the capture devices no source uses yet
// and the DroidCam plugin, for one more phone, DroidCam first.
func (s *setup) choices(in []input) []Choice {
	var list []Choice
	used := map[string]bool{}
	for _, x := range in {
		if x.Kind == dshowKind {
			used[str(x.Settings["video_device_id"])] = true
		}
		if isVideo(x.Kind) && x.Name != probeName {
			list = append(list, Choice{"source:" + x.Name, "source", x.Name, isDroidCam(x)})
		}
	}
	var kinds struct {
		Kinds []string `json:"inputKinds"`
	}
	s.call("GetInputKindList", map[string]bool{"unversioned": true}, &kinds)
	if slices.Contains(kinds.Kinds, PluginKind) {
		list = append(list, Choice{"plugin", "plugin", "DroidCam OBS (plugin)", true})
	}
	if slices.Contains(kinds.Kinds, dshowKind) {
		for _, d := range s.devices(in) {
			if !used[d.Value] {
				list = append(list, Choice{"device:" + d.Value, "device", d.Name, strings.Contains(strings.ToLower(d.Name), "droidcam")})
			}
		}
	}
	slices.SortStableFunc(list, func(a, b Choice) int {
		switch {
		case a.DroidCam == b.DroidCam:
			return 0
		case a.DroidCam:
			return -1
		}
		return 1
	})
	return list
}

type propItem struct {
	Name    string `json:"itemName"`
	Enabled bool   `json:"itemEnabled"`
	Value   any    `json:"itemValue"`
}

type listItem struct{ Name, Value string }

// listItems reads a list property of a source; disabled and empty entries are left out.
func (s *setup) listItems(source, prop string) []listItem {
	var r struct {
		Items []propItem `json:"propertyItems"`
	}
	s.call("GetInputPropertiesListPropertyItems", map[string]string{"inputName": source, "propertyName": prop}, &r)
	var out []listItem
	for _, it := range r.Items {
		if v := str(it.Value); it.Enabled && v != "" {
			out = append(out, listItem{it.Name, v})
		}
	}
	return out
}

// devices are the video capture devices OBS sees. OBS lists them only on a
// source, so with none to ask this adds a hidden one and removes it after.
func (s *setup) devices(in []input) []listItem {
	for _, x := range in {
		if x.Kind == dshowKind {
			return s.listItems(x.Name, "video_device_id")
		}
	}
	if s.scene == "" || s.call("CreateInput", map[string]any{"sceneName": s.scene, "inputName": probeName,
		"inputKind": dshowKind, "inputSettings": map[string]any{}, "sceneItemEnabled": false}, nil) != nil {
		return nil
	}
	// Straight to the client: the probe goes even when the connection had trouble.
	defer s.c.Request(s.cfg, "RemoveInput", map[string]string{"inputName": probeName}, nil)
	return s.listItems(probeName, "video_device_id")
}

func (s *setup) addDevice(st *Step, in []input, id string) string {
	name := uniqueSourceName(in, s.scenes, deviceName(id))
	if re := s.call("CreateInput", map[string]any{"sceneName": s.at, "inputName": name, "inputKind": dshowKind,
		"inputSettings": map[string]any{"video_device_id": id}}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_juntou_camara", "camara", name, "erro", re.Comment)
		return ""
	}
	st.Result, st.Detail = "created", i18n.T("obs.setup.juntei_camara", "camara", name)
	return name
}

// addPlugin adds a DroidCam OBS source and connects it to the first phone
// or tablet it sees by cable that no other DroidCam source uses; over Wi-Fi
// it needs its IP, set once in OBS.
func (s *setup) addPlugin(st *Step, in []input) string {
	used := []string{"dev_id_wifi"}
	for _, x := range in {
		if x.Kind == PluginKind {
			used = append(used, str(x.Settings["device_list"]))
		}
	}
	name := uniqueSourceName(in, s.scenes, "DroidCam")
	if re := s.call("CreateInput", map[string]any{"sceneName": s.at, "inputName": name, "inputKind": PluginKind,
		"inputSettings": map[string]any{}}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_juntou_droidcam", "erro", re.Comment)
		return ""
	}
	st.Result = "created"
	for _, d := range s.listItems(name, "device_list") {
		if slices.Contains(used, d.Value) {
			continue
		}
		if s.call("SetInputSettings", map[string]any{"inputName": name, "inputSettings": map[string]any{"device_list": d.Value}}, nil) == nil &&
			s.call("PressInputPropertiesButton", map[string]string{"inputName": name, "propertyName": "connect"}, nil) == nil {
			st.Detail = i18n.T("obs.setup.juntei_ligada", "fonte", name, "aparelho", d.Name)
			return name
		}
	}
	st.Detail = i18n.T("obs.setup.juntei_wifi", "fonte", name)
	return name
}

// hideOld hides other cameras in the scene, so the new one is the only one
// shown. They stay in OBS.
func (s *setup) hideOld(cam, graphics string) {
	if !s.apply || cam == "" {
		return
	}
	for _, it := range s.items() {
		if it.Name == cam || it.Name == graphics || !it.Enabled || !(it.Name == s.cam.Old || isCapture(it.Kind)) {
			continue
		}
		if s.call("SetSceneItemEnabled", map[string]any{"sceneName": s.at, "sceneItemId": it.ID, "sceneItemEnabled": false}, nil) == nil {
			s.add(Step{ID: s.id("old-camera"), Label: i18n.T("obs.setup.camara_antiga"), Result: "fixed",
				Detail: i18n.T("obs.setup.escondi", "fonte", it.Name)})
		}
	}
}

// order puts above (the graphics scene, or the overlay in the old layout)
// in the scene, shown, above the camera.
func (s *setup) order(above, cam string) {
	items := s.items()
	c := find(items, cam)
	if c == nil {
		return
	}
	st := Step{ID: s.id("order"), Label: i18n.T("obs.setup.ordem"), Result: "ok", Detail: i18n.T("obs.setup.ordem_ok")}
	defer func() { s.add(st) }()
	o := find(items, above)
	if o != nil && o.Enabled && o.Index > c.Index {
		return
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.grafismo_na_cena", "cena", s.at, "grafismo", above)
		return
	}
	if re := s.show(o, above); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_mudou_ordem", "erro", re.Comment)
		return
	}
	items = s.items()
	if o, c = find(items, above), find(items, cam); o != nil && c != nil && o.Index < c.Index {
		if re := s.call("SetSceneItemIndex", map[string]any{"sceneName": s.at, "sceneItemId": o.ID, "sceneItemIndex": len(items) - 1}, nil); re != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_mudou_ordem", "erro", re.Comment)
			return
		}
	}
	st.Result, st.Detail = "fixed", i18n.T("obs.setup.pus_grafismo", "grafismo", above, "cena", s.at)
}

func (s *setup) video(busy bool) {
	st := Step{ID: "video", Label: i18n.T("obs.setup.resolucao"), Result: "ok", Detail: i18n.T("obs.setup.resolucao_ok")}
	defer func() { s.add(st) }()
	var v struct {
		BaseW float64 `json:"baseWidth"`
		BaseH float64 `json:"baseHeight"`
		OutW  float64 `json:"outputWidth"`
		OutH  float64 `json:"outputHeight"`
	}
	if s.call("GetVideoSettings", nil, &v) != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.resolucao_nao_li")
		return
	}
	if v.BaseW == 1920 && v.BaseH == 1080 && v.OutW == 1920 && v.OutH == 1080 {
		return
	}
	was := fmt.Sprintf("%.0fx%.0f", v.BaseW, v.BaseH)
	if v.OutW != v.BaseW || v.OutH != v.BaseH {
		was = i18n.T("obs.setup.resolucao_saida", "base", was, "saida", fmt.Sprintf("%.0fx%.0f", v.OutW, v.OutH))
	}
	switch {
	case busy:
		st.Result, st.Detail = "later", i18n.T("obs.setup.resolucao_depois", "resolucao", was)
		return
	case !s.apply:
		st.Result, st.Detail = "failed", i18n.T("obs.setup.resolucao_mal", "resolucao", was)
		return
	}
	if re := s.call("SetVideoSettings", map[string]any{"baseWidth": 1920, "baseHeight": 1080, "outputWidth": 1920, "outputHeight": 1080}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_mudou_resolucao", "resolucao", was, "erro", re.Comment)
		return
	}
	st.Result, st.Detail = "fixed", i18n.T("obs.setup.mudei_resolucao", "resolucao", was)
}

// program wants one of the live scenes on program, else first.
func (s *setup) program(busy bool, live []string, first string) {
	st := Step{ID: "program", Label: i18n.T("obs.setup.no_ar"), Result: "ok", Detail: i18n.T("obs.setup.no_ar_ok", "cena", first)}
	defer func() { s.add(st) }()
	var sc struct {
		Name string `json:"sceneName"`
		Old  string `json:"currentProgramSceneName"`
	}
	if s.call("GetCurrentProgramScene", nil, &sc) != nil {
		return
	}
	cur := sc.Name
	if cur == "" {
		cur = sc.Old
	}
	if slices.Contains(live, cur) {
		st.Detail = i18n.T("obs.setup.no_ar_ok", "cena", cur)
		return
	}
	if busy {
		st.Result, st.Detail = "later", i18n.T("obs.setup.no_ar_depois", "cena", cur)
		return
	}
	if !s.apply {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.no_ar_mal", "cena", cur)
		return
	}
	if re := s.call("SetCurrentProgramScene", map[string]string{"sceneName": first}, nil); re != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_mudou_cena", "erro", re.Comment)
		return
	}
	st.Result, st.Detail = "fixed", i18n.T("obs.setup.mudei_cena", "antes", cur, "cena", first)
}

// OverlayURL is the overlay's address on port.
func OverlayURL(port int) string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/overlay"
}
