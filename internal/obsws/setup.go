package obsws

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
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
// ("source:<name>"), a video capture device to add ("device:<id>") or the
// DroidCam OBS plugin ("plugin").
type Choice struct {
	Value    string `json:"value"`
	Type     string `json:"type"` // source, device, plugin
	Label    string `json:"label"`
	DroidCam bool   `json:"droidcam"`
}

// Want is what the setup aims for.
type Want struct {
	OverlayURL   string
	OverlayPorts [2]int // the ports the program may run on: a browser source at /overlay on one of them is ours
	// Camera is a Choice value, or "" to find one: a DroidCam source first.
	Camera string
	// Old is the camera before; it is hidden in the scene when another replaces it.
	Old string
}

// Report is the checklist and the camera it ended with.
type Report struct {
	Steps   []Step   `json:"steps"`
	Camera  string   `json:"camera"`
	Choices []Choice `json:"choices,omitempty"`
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
// scene, the overlay browser source, the camera, their order, the canvas
// size and the program scene, then fit(camera). It never deletes or changes
// a source the user made, other than the overlay's own settings. Without
// apply it only reports (the health check). The error is for a lost
// connection; whatever OBS refuses becomes a failed step.
func (c *Client) Setup(cfg Config, w Want, apply bool, fit func(camera string) error) (Report, error) {
	s := &setup{c: c, cfg: cfg, apply: apply, want: w}
	s.run(fit)
	if s.err != nil {
		return Report{}, s.err
	}
	s.rep.Ready = true
	for _, st := range s.rep.Steps {
		if (st.Result == "failed" || st.Result == "choose") && st.ID != "fit" {
			s.rep.Ready = false
		}
	}
	return s.rep, nil
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
	c     *Client
	cfg   Config
	apply bool
	want  Want
	rep   Report
	err   error  // the connection failed: stop
	scene string // where new sources go
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

func (s *setup) run(fit func(string) error) {
	busy := s.busy()
	if !s.ensureScene() || s.err != nil {
		return
	}
	s.scene = SceneName
	in := s.inputs()
	overlay := s.overlay(in)
	cam := s.camera(in)
	if s.err != nil {
		return
	}
	s.rep.Camera = cam
	s.hideOld(cam, overlay)
	s.order(overlay, cam)
	s.video(busy)
	s.program(busy)
	if s.apply && cam != "" && fit != nil && s.err == nil {
		st := Step{ID: "fit", Label: "Câmara ajustada ao ecrã", Result: "ok", Detail: "A câmara ocupa o ecrã inteiro."}
		if err := fit(cam); err != nil {
			st.Result, st.Detail = "failed", err.Error()
		}
		s.add(st)
	}
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

func (s *setup) ensureScene() bool {
	st := Step{ID: "scene", Label: "Cena " + SceneName, Result: "ok", Detail: "A cena já existe."}
	defer func() { s.add(st) }()
	var r struct {
		Scenes []struct {
			Name string `json:"sceneName"`
		} `json:"scenes"`
	}
	if re := s.call("GetSceneList", nil, &r); re != nil {
		st.Result, st.Detail = "failed", "O OBS não deu a lista de cenas: "+re.Comment
		return false
	}
	for _, sc := range r.Scenes {
		if sc.Name == SceneName {
			return true
		}
	}
	if !s.apply {
		st.Result, st.Detail = "failed", "Falta a cena "+SceneName+" no OBS."
		return true
	}
	if re := s.call("CreateScene", map[string]string{"sceneName": SceneName}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não criou a cena: "+re.Comment
		return false
	}
	st.Result, st.Detail = "created", "Criei a cena "+SceneName+"."
	return true
}

func (s *setup) inputs() []input {
	var r struct {
		Inputs []input `json:"inputs"`
	}
	if re := s.call("GetInputList", nil, &r); re != nil {
		if s.err == nil {
			s.err = errors.New("o OBS não deu a lista de fontes: " + re.Comment)
		}
		return nil
	}
	for i := range r.Inputs {
		if k := r.Inputs[i].Kind; k == "browser_source" || k == dshowKind {
			var st struct {
				Settings map[string]any `json:"inputSettings"`
			}
			s.call("GetInputSettings", map[string]string{"inputName": r.Inputs[i].Name}, &st)
			r.Inputs[i].Settings = st.Settings
		}
	}
	return r.Inputs
}

// items is the scene's items; none when the scene is missing.
func (s *setup) items() []sceneItem {
	var r struct {
		Items []sceneItem `json:"sceneItems"`
	}
	if s.call("GetSceneItemList", map[string]string{"sceneName": SceneName}, &r) != nil {
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
	st := Step{ID: "overlay", Label: "Grafismo (fonte Navegador)", Result: "ok", Detail: "Aponta para " + s.want.OverlayURL + "."}
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
			st.Result, st.Detail = "failed", "Falta a fonte do grafismo no OBS."
			return ""
		}
		name := uniqueName(in, OverlayName)
		if re := s.call("CreateInput", map[string]any{"sceneName": SceneName, "inputName": name,
			"inputKind": "browser_source", "inputSettings": settings}, nil); re != nil {
			st.Result, st.Detail = "failed", "O OBS não criou a fonte do grafismo: "+re.Comment
			return ""
		}
		st.Result, st.Detail = "created", "Criei a fonte "+name+", 1920x1080, com o som pelo OBS."
		return name
	}
	// GetInputSettings leaves out what is at obs-browser's defaults.
	cur := found.Settings
	var fixes []string
	if str(cur["url"]) != s.want.OverlayURL || flag(cur["is_local_file"]) {
		fixes = append(fixes, "o endereço era "+str(cur["url"]))
	}
	if num(cur["width"], 800) != 1920 || num(cur["height"], 600) != 1080 {
		fixes = append(fixes, fmt.Sprintf("o tamanho era %.0fx%.0f", num(cur["width"], 800), num(cur["height"], 600)))
	}
	if !flag(cur["reroute_audio"]) {
		fixes = append(fixes, "o som não passava pelo OBS")
	}
	wrong := len(fixes) > 0
	it := find(items, found.Name)
	if it == nil {
		fixes = append(fixes, "não estava na cena")
	} else if !it.Enabled {
		fixes = append(fixes, "estava escondida")
	}
	if len(fixes) == 0 {
		return found.Name
	}
	what := strings.Join(fixes, ", ")
	if !s.apply {
		st.Result, st.Detail = "failed", "A fonte "+found.Name+" está mal: "+what+"."
		return found.Name
	}
	if wrong {
		if re := s.call("SetInputSettings", map[string]any{"inputName": found.Name, "inputSettings": settings, "overlay": true}, nil); re != nil {
			st.Result, st.Detail = "failed", "O OBS não corrigiu a fonte do grafismo: "+re.Comment
			return found.Name
		}
	}
	if re := s.show(it, found.Name); re != nil {
		st.Result, st.Detail = "failed", "O OBS não pôs o grafismo na cena: "+re.Comment
		return found.Name
	}
	st.Result, st.Detail = "fixed", "Corrigi a fonte "+found.Name+": "+what+"."
	return found.Name
}

// show puts the source in the scene (it == nil) or unhides its item.
func (s *setup) show(it *sceneItem, name string) *RequestError {
	if it == nil {
		return s.call("CreateSceneItem", map[string]any{"sceneName": SceneName, "sourceName": name, "sceneItemEnabled": true}, nil)
	}
	if !it.Enabled {
		return s.call("SetSceneItemEnabled", map[string]any{"sceneName": SceneName, "sceneItemId": it.ID, "sceneItemEnabled": true}, nil)
	}
	return nil
}

func uniqueName(in []input, name string) string {
	taken := func(n string) bool {
		return slices.ContainsFunc(in, func(x input) bool { return x.Name == n })
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
	st := Step{ID: "camera", Label: "Câmara", Result: "ok"}
	defer func() { s.add(st) }()
	w := s.want.Camera
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
	case strings.HasPrefix(w, "device:"):
		id := strings.TrimPrefix(w, "device:")
		for _, x := range in {
			if x.Kind == dshowKind && str(x.Settings["video_device_id"]) == id {
				name = x.Name
			}
		}
		if name == "" {
			if !s.apply {
				st.Result, st.Detail = "failed", "Carregue em Configurar OBS para juntar a câmara "+deviceName(id)+"."
				return ""
			}
			return s.addDevice(&st, in, id)
		}
	case w == "plugin":
		for _, x := range in {
			if x.Kind == PluginKind {
				name = x.Name
			}
		}
		if name == "" {
			if !s.apply {
				st.Result, st.Detail = "failed", "Carregue em Configurar OBS para juntar a fonte DroidCam OBS."
				return ""
			}
			return s.addPlugin(&st, in)
		}
	case strings.HasPrefix(w, "source:"):
		name = strings.TrimPrefix(w, "source:")
		if byName(name) == nil {
			if !s.apply {
				st.Result, st.Detail = "failed", "A câmara "+name+" já não existe no OBS. Escolha outra em Definições > Câmara e carregue em Configurar OBS."
				return ""
			}
			note = "A câmara " + name + " já não existe no OBS. "
			name = ""
		}
	}
	if name == "" {
		name = s.auto(&st, in, note)
		if name == "" || st.Result != "ok" {
			return name
		}
	}
	it := find(s.items(), name)
	if it != nil && it.Enabled {
		if st.Detail == "" {
			st.Detail = "Usa a fonte " + name + "."
		}
		return name
	}
	if !s.apply {
		st.Result, st.Detail = "failed", "A câmara "+name+" não aparece na cena "+SceneName+"."
		return name
	}
	if re := s.show(it, name); re != nil {
		st.Result, st.Detail = "failed", "O OBS não pôs a câmara na cena: "+re.Comment
		return name
	}
	st.Result, st.Detail = "fixed", note+"Pus a fonte "+name+" na cena."
	return name
}

// auto finds the camera with none chosen: a DroidCam source, else a camera
// already in the scene, else the only choice. With several, the volunteer
// picks.
func (s *setup) auto(st *Step, in []input, note string) string {
	items := s.items()
	var best string
	for _, x := range in {
		if isDroidCam(x) && (best == "" || find(items, x.Name) != nil) {
			best = x.Name
		}
	}
	if best != "" {
		return best
	}
	for i := len(items) - 1; i >= 0; i-- {
		if isVideo(items[i].Kind) && items[i].Name != probeName {
			return items[i].Name
		}
	}
	if !s.apply {
		st.Result, st.Detail = "failed", "Falta a câmara na cena "+SceneName+". Carregue em Configurar OBS."
		return ""
	}
	list := s.choices(in)
	switch len(list) {
	case 0:
		st.Result, st.Detail = "failed", note+"Não encontrei nenhuma câmara. Instale o DroidCam no PC e no tablet (droidcam.app), ou ligue uma câmara USB, e carregue outra vez em Configurar OBS."
		return ""
	case 1:
		return s.use(st, in, list[0])
	}
	st.Result, st.Detail = "choose", note+"O OBS tem várias câmaras. Escolha uma e carregue em Usar esta câmara."
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
// and the DroidCam plugin when it has no source, DroidCam first.
func (s *setup) choices(in []input) []Choice {
	var list []Choice
	used := map[string]bool{}
	plugin := false
	for _, x := range in {
		if x.Kind == dshowKind {
			used[str(x.Settings["video_device_id"])] = true
		}
		plugin = plugin || x.Kind == PluginKind
		if isVideo(x.Kind) && x.Name != probeName {
			list = append(list, Choice{"source:" + x.Name, "source", x.Name, isDroidCam(x)})
		}
	}
	var kinds struct {
		Kinds []string `json:"inputKinds"`
	}
	s.call("GetInputKindList", map[string]bool{"unversioned": true}, &kinds)
	if slices.Contains(kinds.Kinds, PluginKind) && !plugin {
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
	name := uniqueName(in, deviceName(id))
	if re := s.call("CreateInput", map[string]any{"sceneName": SceneName, "inputName": name, "inputKind": dshowKind,
		"inputSettings": map[string]any{"video_device_id": id}}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não juntou a câmara "+name+": "+re.Comment
		return ""
	}
	st.Result, st.Detail = "created", "Juntei a câmara "+name+" à cena."
	return name
}

// addPlugin adds a DroidCam OBS source and connects it to the first phone
// or tablet it sees by cable; over Wi-Fi it needs its IP, set once in OBS.
func (s *setup) addPlugin(st *Step, in []input) string {
	name := uniqueName(in, "DroidCam")
	if re := s.call("CreateInput", map[string]any{"sceneName": SceneName, "inputName": name, "inputKind": PluginKind,
		"inputSettings": map[string]any{}}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não juntou a fonte DroidCam: "+re.Comment
		return ""
	}
	st.Result = "created"
	for _, d := range s.listItems(name, "device_list") {
		if d.Value == "dev_id_wifi" {
			continue
		}
		if s.call("SetInputSettings", map[string]any{"inputName": name, "inputSettings": map[string]any{"device_list": d.Value}}, nil) == nil &&
			s.call("PressInputPropertiesButton", map[string]string{"inputName": name, "propertyName": "connect"}, nil) == nil {
			st.Detail = "Juntei a fonte " + name + " e liguei-a a " + d.Name + "."
			return name
		}
	}
	st.Detail = "Juntei a fonte " + name + ". Para a ligar ao tablet por Wi-Fi, abra-a no OBS uma vez, escreva o IP do tablet e carregue em Activate."
	return name
}

// hideOld hides other cameras in the scene, so the new one is the only one
// shown. They stay in OBS.
func (s *setup) hideOld(cam, overlay string) {
	if !s.apply || cam == "" {
		return
	}
	for _, it := range s.items() {
		if it.Name == cam || it.Name == overlay || !it.Enabled || !(it.Name == s.want.Old || isCapture(it.Kind)) {
			continue
		}
		if s.call("SetSceneItemEnabled", map[string]any{"sceneName": SceneName, "sceneItemId": it.ID, "sceneItemEnabled": false}, nil) == nil {
			s.add(Step{ID: "old-camera", Label: "Câmara antiga", Result: "fixed",
				Detail: "Escondi " + it.Name + " na cena. Continua no OBS, se precisar dela outra vez."})
		}
	}
}

func (s *setup) order(overlay, cam string) {
	items := s.items()
	o, c := find(items, overlay), find(items, cam)
	if o == nil || c == nil {
		return
	}
	st := Step{ID: "order", Label: "Grafismo por cima da câmara", Result: "ok", Detail: "O grafismo está por cima da câmara."}
	defer func() { s.add(st) }()
	if o.Index > c.Index {
		return
	}
	if !s.apply {
		st.Result, st.Detail = "failed", "O grafismo está por baixo da câmara."
		return
	}
	if re := s.call("SetSceneItemIndex", map[string]any{"sceneName": SceneName, "sceneItemId": o.ID, "sceneItemIndex": len(items) - 1}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não mudou a ordem: "+re.Comment
		return
	}
	st.Result, st.Detail = "fixed", "Pus o grafismo por cima da câmara."
}

func (s *setup) video(busy bool) {
	st := Step{ID: "video", Label: "Resolução 1920x1080", Result: "ok", Detail: "O OBS já está em 1920x1080."}
	defer func() { s.add(st) }()
	var v struct {
		BaseW float64 `json:"baseWidth"`
		BaseH float64 `json:"baseHeight"`
		OutW  float64 `json:"outputWidth"`
		OutH  float64 `json:"outputHeight"`
	}
	if s.call("GetVideoSettings", nil, &v) != nil {
		st.Result, st.Detail = "failed", "Não consegui ler a resolução do OBS."
		return
	}
	if v.BaseW == 1920 && v.BaseH == 1080 && v.OutW == 1920 && v.OutH == 1080 {
		return
	}
	was := fmt.Sprintf("%.0fx%.0f", v.BaseW, v.BaseH)
	if v.OutW != v.BaseW || v.OutH != v.BaseH {
		was += fmt.Sprintf(" (saída %.0fx%.0f)", v.OutW, v.OutH)
	}
	switch {
	case busy:
		st.Result, st.Detail = "later", "A resolução é "+was+". Não dá para a mudar com a transmissão ou a gravação ligadas. Carregue em Configurar OBS depois de as parar."
		return
	case !s.apply:
		st.Result, st.Detail = "failed", "A resolução do OBS é "+was+"."
		return
	}
	if re := s.call("SetVideoSettings", map[string]any{"baseWidth": 1920, "baseHeight": 1080, "outputWidth": 1920, "outputHeight": 1080}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não mudou a resolução ("+was+"): "+re.Comment
		return
	}
	st.Result, st.Detail = "fixed", "Mudei a resolução de "+was+" para 1920x1080."
}

func (s *setup) program(busy bool) {
	st := Step{ID: "program", Label: "Cena no ar", Result: "ok", Detail: "O OBS mostra a cena " + SceneName + "."}
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
	if cur == SceneName {
		return
	}
	if busy {
		st.Result, st.Detail = "later", "O OBS está na cena "+cur+". Não mudei de cena com a transmissão no ar."
		return
	}
	if !s.apply {
		st.Result, st.Detail = "failed", "O OBS está na cena "+cur+"."
		return
	}
	if re := s.call("SetCurrentProgramScene", map[string]string{"sceneName": SceneName}, nil); re != nil {
		st.Result, st.Detail = "failed", "O OBS não mudou de cena: "+re.Comment
		return
	}
	st.Result, st.Detail = "fixed", "Mudei o OBS da cena "+cur+" para "+SceneName+"."
}

// OverlayURL is the overlay's address on port.
func OverlayURL(port int) string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/overlay"
}
