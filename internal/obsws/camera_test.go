package obsws

import (
	"math"
	"slices"
	"sync"
	"testing"
	"time"
)

var (
	window = Rect{416, 64, 1440, 810}
	canvas = Rect{0, 0, 1920, 1080}
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func samePlace(a, b Place) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H) &&
		near(a.CropLeft, b.CropLeft) && near(a.CropRight, b.CropRight) && near(a.CropTop, b.CropTop) && near(a.CropBottom, b.CropBottom)
}

func TestFit(t *testing.T) {
	cases := []struct {
		name string
		w, h float64
		fit  string
		box  Rect
		want Place
	}{
		{"16:9 window", 1920, 1080, "contain", window, Place{Rect: window}},
		{"16:9 full fill", 1920, 1080, "fill", canvas, Place{Rect: canvas}},
		{"720p window", 1280, 720, "contain", window, Place{Rect: window}},
		{"720p full", 1280, 720, "contain", canvas, Place{Rect: canvas}},
		{"720p full fill", 1280, 720, "fill", canvas, Place{Rect: canvas}},
		{"4:3 window", 1440, 1080, "contain", window, Place{Rect: Rect{596, 64, 1080, 810}}},
		{"4:3 window fill", 1440, 1080, "fill", window, Place{Rect: window, CropTop: 135, CropBottom: 135}},
		{"4:3 full", 1440, 1080, "contain", canvas, Place{Rect: Rect{240, 0, 1440, 1080}}},
		{"4:3 full fill", 1440, 1080, "fill", canvas, Place{Rect: canvas, CropTop: 135, CropBottom: 135}},
		{"9:16 window", 1080, 1920, "contain", window, Place{Rect: Rect{908.1875, 64, 455.625, 810}}},
		{"9:16 full", 1080, 1920, "contain", canvas, Place{Rect: Rect{656.25, 0, 607.5, 1080}}},
		{"9:16 full fill", 1080, 1920, "fill", canvas, Place{Rect: canvas, CropTop: 656.25, CropBottom: 656.25}},
	}
	for _, c := range cases {
		if got := Fit(c.fit, c.w, c.h, c.box); !samePlace(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestTransform(t *testing.T) {
	tr := Fill(1440, 1080, canvas).transform(1440, 1080)
	if tr["cropTop"] != 135 || tr["cropBottom"] != 135 || !near(tr["scaleY"].(float64), 1080.0/810) || !near(tr["scaleX"].(float64), 1920.0/1440) {
		t.Errorf("4:3 fill %v", tr)
	}
	// Crops round to whole pixels and the scale follows, so the picture still fills the box.
	tr = Fill(1080, 1920, canvas).transform(1080, 1920)
	if tr["cropTop"] != 656 || !near(tr["scaleY"].(float64), 1080.0/(1920-656-656)) {
		t.Errorf("9:16 fill %v", tr)
	}
	if tr["alignment"] != 5 || tr["boundsType"] != "OBS_BOUNDS_NONE" {
		t.Errorf("alignment and bounds %v", tr)
	}
}

func TestBezier(t *testing.T) {
	e := Bezier(0.65, 0, 0.35, 1)
	if !near(e(0), 0) || !near(e(1), 1) || math.Abs(e(0.5)-0.5) > 1e-6 {
		t.Errorf("ends %v %v %v", e(0), e(0.5), e(1))
	}
	if e(0.2) > 0.1 || e(0.8) < 0.9 {
		t.Errorf("not eased: %v %v", e(0.2), e(0.8))
	}
	prev := 0.0
	for x := 0.01; x <= 1; x += 0.01 {
		if v := e(x); v < prev {
			t.Fatalf("goes back at %v", x)
		} else {
			prev = v
		}
	}
}

func item3(srcW, srcH float64) []map[string]any {
	tr := func(w, h float64) map[string]any {
		return map[string]any{"positionX": 0.0, "positionY": 0.0, "scaleX": 1920 / w, "scaleY": 1080 / h,
			"sourceWidth": w, "sourceHeight": h, "alignment": 5.0, "boundsType": "OBS_BOUNDS_NONE",
			"cropLeft": 0.0, "cropRight": 0.0, "cropTop": 0.0, "cropBottom": 0.0}
	}
	return []map[string]any{
		{"sceneItemId": 1, "sceneItemIndex": 0, "sourceName": "Fundo", "inputKind": "image_source", "sceneItemEnabled": true, "sceneItemTransform": tr(1920, 1080)},
		{"sceneItemId": 2, "sceneItemIndex": 1, "sourceName": "Tablet", "inputKind": "dshow_input", "sceneItemEnabled": true, "sceneItemTransform": tr(srcW, srcH)},
		{"sceneItemId": 3, "sceneItemIndex": 2, "sourceName": "Hallstream", "inputKind": "browser_source", "sceneItemEnabled": true, "sceneItemTransform": tr(1920, 1080)},
		{"sceneItemId": 4, "sceneItemIndex": 3, "sourceName": "Webcam por cima", "inputKind": "dshow_input", "sceneItemEnabled": true, "sceneItemTransform": tr(1920, 1080)},
	}
}

type rig struct {
	f     *fake
	c     *Camera
	mu    sync.Mutex
	set   CameraSettings
	saved []Full
	win   *Rect
}

func newRig(t *testing.T, srcW, srcH float64) *rig {
	r := &rig{f: &fake{scenes: []string{"Jogo"}, program: "Jogo", items: item3(srcW, srcH), canvasW: 1920, canvasH: 1080}}
	r.set = CameraSettings{Config: fakeOBS(t, "", r.f), Enabled: true, Fit: "contain"}
	r.c = &Camera{
		OBS:      &Client{},
		Settings: func() CameraSettings { r.mu.Lock(); defer r.mu.Unlock(); return r.set },
		Save:     func(_ string, f Full) { r.mu.Lock(); r.saved = append(r.saved, f); r.set.Full = &f; r.mu.Unlock() },
		Show:     func(_, w *Rect) { r.mu.Lock(); r.win = w; r.mu.Unlock() },
		Window:   window,
		Dur:      150 * time.Millisecond,
		Ease:     Bezier(0.65, 0, 0.35, 1),
		Step:     5 * time.Millisecond,
	}
	return r
}

func (r *rig) wait(t *testing.T) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		r.c.mu.Lock()
		m := r.c.moving
		r.c.mu.Unlock()
		if !m {
			return
		}
	}
	t.Fatal("the tween never ended")
}

func last(t *testing.T, f *fake) map[string]any {
	t.Helper()
	s := f.transforms()
	if len(s) == 0 {
		t.Fatal("no transform sent")
	}
	return s[len(s)-1]
}

func TestSqueezeInAndOut(t *testing.T) {
	r := newRig(t, 1280, 720)
	if err := r.c.Follow("desconto"); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	sets := r.f.transforms()
	if n := len(sets); n < 10 || n > 60 {
		t.Errorf("%d steps for 150 ms at 5 ms", n)
	}
	for i := 1; i < len(sets); i++ {
		if sets[i]["positionX"].(float64) < sets[i-1]["positionX"].(float64) {
			t.Fatalf("step %d goes back", i)
		}
	}
	end := last(t, r.f)
	if !near(end["positionX"].(float64), 416) || !near(end["positionY"].(float64), 64) || !near(end["scaleX"].(float64), 1.125) {
		t.Errorf("window %v", end)
	}
	if r.win == nil || *r.win != window {
		t.Errorf("overlay window %v", r.win)
	}
	if len(r.saved) != 1 || r.saved[0].SourceWidth != 1280 || r.saved[0].Place.Rect != canvas {
		t.Errorf("saved %+v", r.saved)
	}
	// The same screen again does nothing.
	n := len(r.f.transforms())
	r.c.Follow("desconto")
	if len(r.f.transforms()) != n {
		t.Error("a repeated desconto moved the camera")
	}

	r.c.Follow("marcador")
	r.wait(t)
	end = last(t, r.f)
	if !near(end["positionX"].(float64), 0) || !near(end["scaleX"].(float64), 1.5) {
		t.Errorf("full %v", end)
	}
}

func TestPortraitWindow(t *testing.T) {
	r := newRig(t, 1080, 1920)
	r.c.Follow("desconto")
	r.wait(t)
	end := last(t, r.f)
	if !near(end["positionX"].(float64), 908.1875) || !near(end["scaleY"].(float64), 810.0/1920) {
		t.Errorf("portrait %v", end)
	}
	if want := (Rect{908.1875, 64, 455.625, 810}); r.win == nil || *r.win != want {
		t.Errorf("overlay hole %v", r.win)
	}
}

// desconto → marcador → desconto in quick turns: the camera turns round
// where it is and ends in the window, without jumping.
func TestRetarget(t *testing.T) {
	r := newRig(t, 1920, 1080)
	// The tween runs on the wall clock, so a starved CI runner skips frames and the
	// next one lands further on. A slower tween keeps a late tick well under the
	// 60 px bound, while a snap to either end still breaks it.
	r.c.Dur = 600 * time.Millisecond
	r.c.Follow("desconto")
	time.Sleep(240 * time.Millisecond)
	r.c.Follow("marcador")
	time.Sleep(120 * time.Millisecond)
	r.c.Follow("desconto")
	r.wait(t)
	sets := r.f.transforms()
	for i := 1; i < len(sets); i++ {
		if d := math.Abs(sets[i]["positionX"].(float64) - sets[i-1]["positionX"].(float64)); d > 60 {
			t.Fatalf("jump of %.0f px at step %d", d, i)
		}
	}
	end := last(t, r.f)
	if !near(end["positionX"].(float64), 416) || !near(end["scaleX"].(float64), 0.75) {
		t.Errorf("ended at %v", end)
	}
}

func TestLegacyScenes(t *testing.T) {
	r := newRig(t, 1920, 1080)
	r.set.TimeoutScene, r.set.GameScene = "Desconto", "Jogo"
	r.f.scenes = []string{"Jogo", "Desconto"}
	if err := r.c.Follow("desconto"); err != nil {
		t.Fatal(err)
	}
	if r.f.scene() != "Desconto" || len(r.f.transforms()) != 0 {
		t.Errorf("program %q, %d transforms", r.f.scene(), len(r.f.transforms()))
	}
	r.c.Follow("marcador")
	if r.f.scene() != "Jogo" {
		t.Errorf("program %q", r.f.scene())
	}

	// A timeout_scene OBS does not have: the camera moves instead.
	r = newRig(t, 1920, 1080)
	r.set.TimeoutScene = "Desconto"
	r.c.Follow("desconto")
	r.wait(t)
	if r.f.scene() != "Jogo" || len(r.f.transforms()) == 0 {
		t.Errorf("program %q, %d transforms", r.f.scene(), len(r.f.transforms()))
	}
}

func TestPick(t *testing.T) {
	items := []sceneItem{
		{ID: 1, Index: 0, Name: "Câmara velha", Kind: "dshow_input", Enabled: true},
		{ID: 2, Index: 1, Name: "DroidCam", Kind: "droidcam_obs", Enabled: true},
		{ID: 3, Index: 2, Name: "Hallstream", Kind: "browser_source", Enabled: true},
		{ID: 4, Index: 3, Name: "Webcam", Kind: "dshow_input", Enabled: true},
	}
	if p := pick(items, ""); p == nil || p.ID != 2 {
		t.Errorf("auto %+v", p)
	}
	if p := pick(items, "Webcam"); p == nil || p.ID != 4 {
		t.Errorf("named %+v", p)
	}
	if pick(items, "Outra") != nil {
		t.Error("an unknown name must not match")
	}
	if p := pick(items[3:], ""); p == nil || p.ID != 4 {
		t.Errorf("no browser source %+v", p)
	}
	// Automática prefers DroidCam, even below another camera, and skips hidden items.
	cams := []sceneItem{
		{ID: 1, Index: 0, Name: "DroidCam", Kind: "dshow_input", Enabled: true},
		{ID: 2, Index: 1, Name: "Logitech", Kind: "dshow_input", Enabled: true},
		{ID: 3, Index: 2, Name: "Hallstream", Kind: "browser_source", Enabled: true},
	}
	if p := pick(cams, ""); p == nil || p.ID != 1 {
		t.Errorf("droidcam first %+v", p)
	}
	cams[0].Enabled = false
	if p := pick(cams, ""); p == nil || p.ID != 2 {
		t.Errorf("hidden droidcam %+v", p)
	}
}

func TestErrors(t *testing.T) {
	r := newRig(t, 1920, 1080)
	r.set.Source = "Não existe"
	if err := r.c.Follow("desconto"); err == nil {
		t.Error("want an error for a missing source")
	}
	// No OBS at all: an error, nothing hangs.
	r = newRig(t, 1920, 1080)
	r.set.Port = 1
	if err := r.c.Follow("desconto"); err == nil {
		t.Error("want an error without OBS")
	}
	// Disabled: nothing to do.
	r = newRig(t, 1920, 1080)
	r.set.Enabled = false
	if err := r.c.Follow("desconto"); err != nil || len(r.f.asked) != 0 {
		t.Errorf("disabled: %v %v", err, r.f.asked)
	}
}

func TestCheckRefits(t *testing.T) {
	r := newRig(t, 1280, 720)
	if err := r.c.Check(); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	n := len(r.f.transforms())
	if len(r.saved) != 1 || n != 1 {
		t.Fatalf("first check: %d saved, %d transforms", len(r.saved), n)
	}
	r.c.Check()
	r.wait(t)
	if len(r.saved) != 1 || len(r.f.transforms()) != n {
		t.Error("an unchanged source must not refit")
	}
	// The tablet turns.
	r.f.mu.Lock()
	tr := r.f.items[1]["sceneItemTransform"].(map[string]any)
	tr["sourceWidth"], tr["sourceHeight"] = 1080.0, 1920.0
	r.f.mu.Unlock()
	r.c.Check()
	r.wait(t)
	end := last(t, r.f)
	if len(r.saved) != 2 || !near(end["positionX"].(float64), 656.25) || !near(end["scaleX"].(float64), 0.5625) {
		t.Errorf("refit %v, saved %d", end, len(r.saved))
	}
}

func TestFitNowOtherCanvas(t *testing.T) {
	r := newRig(t, 1920, 1080)
	r.f.canvasW, r.f.canvasH = 1280, 720
	rep, err := r.c.FitNow()
	if err != nil || rep.CanvasWidth != 1280 || rep.Source != "Tablet" {
		t.Fatalf("%+v %v", rep, err)
	}
	r.wait(t)
	if end := last(t, r.f); !near(end["scaleX"].(float64), 1280.0/1920) {
		t.Errorf("fit %v", end)
	}
	// The overlay still gets 1920x1080 px.
	if r.win == nil || *r.win != window {
		t.Errorf("overlay window %v", r.win)
	}
}

func TestFitInOtherScene(t *testing.T) {
	r := newRig(t, 1920, 1080)
	other := item3(1280, 720)
	cam, graphics := other[1], other[2]
	cam["sceneItemId"], cam["sceneItemIndex"], cam["sourceName"] = 7, 0, "DroidCam"
	graphics["sceneItemId"], graphics["sceneItemIndex"], graphics["sourceName"], graphics["inputKind"] = 8, 1, "Grafismo", nil
	r.f.byScene = map[string][]map[string]any{"Jogo": r.f.items, "Câmara 2": {cam, graphics}}
	var saved []string
	r.c.Save = func(source string, _ Full) { saved = append(saved, source) }
	shown := 0
	r.c.Show = func(_, _ *Rect) { shown++ }
	rep, err := r.c.FitIn("Câmara 2", "DroidCam")
	if err != nil || rep.Source != "DroidCam" || rep.SourceWidth != 1280 {
		t.Fatalf("%+v %v", rep, err)
	}
	r.wait(t)
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	if len(r.f.setIn) == 0 || slices.ContainsFunc(r.f.setIn, func(s string) bool { return s != "Câmara 2" }) {
		t.Errorf("transforms sent to %v", r.f.setIn)
	}
	if !slices.Equal(saved, []string{"DroidCam"}) || r.f.program != "Jogo" {
		t.Errorf("saved %v, program %q", saved, r.f.program)
	}
	// The overlay draws what is on program, which is another scene.
	if shown != 0 {
		t.Errorf("told the overlay %d times about a scene off program", shown)
	}
}

// otherScene puts camera B, 1280x720, alone in Câmara 2, beside the rig's Jogo.
func otherScene(r *rig) {
	cam := item3(1280, 720)[1]
	cam["sceneItemId"], cam["sceneItemIndex"], cam["sourceName"] = 7, 0, "B"
	r.f.byScene = map[string][]map[string]any{"Jogo": r.f.items, "Câmara 2": {cam}}
}

// Camera 2 is fitted with its own saved placement, never camera 1's.
func TestFitInUsesItsOwnPlacement(t *testing.T) {
	r := newRig(t, 1920, 1080)
	otherScene(r)
	one := Full{Place{Rect: canvas}, 1920, 1080, 1920, 1080, "contain"}
	two := Full{Place{Rect: Rect{0, 12, 1920, 1056}}, 1280, 720, 1920, 1080, "contain"}
	r.set.Source, r.set.Full = "Tablet", &one
	r.set.Fulls = map[string]*Full{"Tablet": &one, "B": &two}
	if _, err := r.c.FitIn("Câmara 2", "B"); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	if len(r.saved) != 0 {
		t.Errorf("saved %+v over camera 2's own placement", r.saved)
	}
	if end := last(t, r.f); !near(end["positionY"].(float64), 12) {
		t.Errorf("placed at %v, not camera 2's placement", end)
	}
}

// A fit while a squeeze runs leaves the squeeze to end where it was going.
func TestFitInWaitsForTheSqueeze(t *testing.T) {
	r := newRig(t, 1920, 1080)
	otherScene(r)
	r.c.Dur = 200 * time.Millisecond
	r.c.Follow("desconto")
	time.Sleep(40 * time.Millisecond)
	if _, err := r.c.FitIn("Câmara 2", "B"); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	var jogo map[string]any
	for i, s := range r.f.setIn {
		if s == "Jogo" {
			jogo = r.f.sets[i]
		}
	}
	if jogo == nil || !near(jogo["positionX"].(float64), 416) {
		t.Errorf("the squeeze stopped at %v", jogo)
	}
	if r.f.setIn[len(r.f.setIn)-1] != "Câmara 2" {
		t.Errorf("camera 2 not fitted: %v", r.f.setIn)
	}
}

func TestSources(t *testing.T) {
	r := newRig(t, 1920, 1080)
	scene, list, auto, err := r.c.Sources()
	if err != nil || scene != "Jogo" || auto != "Tablet" || len(list) != 4 || list[0].Name != "Webcam por cima" {
		t.Errorf("%q %v %q %v", scene, list, auto, err)
	}
}

func TestStopEndsTheTweenAtItsTarget(t *testing.T) {
	r := newRig(t, 1280, 720)
	r.c.Dur = time.Hour
	r.c.Follow("desconto")
	time.Sleep(30 * time.Millisecond)
	r.c.Stop()
	r.c.mu.Lock()
	moving := r.c.moving
	r.c.mu.Unlock()
	if moving {
		t.Fatal("still moving after Stop")
	}
	end := last(t, r.f)
	if !near(end["positionX"].(float64), 416) || !near(end["scaleX"].(float64), 1.125) {
		t.Errorf("stopped at %v, not in the window", end)
	}
	r.c.Stop() // nothing running: returns at once
}

// twoScenes gives the rig two camera scenes, A in Câmara 1 (on program) and
// B in Câmara 2, both 1280x720 below the graphics.
func twoScenes(r *rig) {
	scene := func(id int, source string) []map[string]any {
		it := item3(1280, 720)
		cam, graphics := it[1], it[2]
		cam["sceneItemId"], cam["sceneItemIndex"], cam["sourceName"] = id, 0, source
		graphics["sceneItemId"], graphics["sceneItemIndex"], graphics["sourceName"], graphics["inputKind"] = id+1, 1, "Grafismo", nil
		return []map[string]any{cam, graphics}
	}
	r.f.scenes = []string{"Câmara 1", "Câmara 2"}
	r.f.program = "Câmara 1"
	r.f.byScene = map[string][]map[string]any{"Câmara 1": scene(1, "A"), "Câmara 2": scene(3, "B")}
}

func TestSqueezeFollowsLiveScene(t *testing.T) {
	r := newRig(t, 1280, 720)
	twoScenes(r)
	r.set.Scene, r.set.Source = "Câmara 2", "B"
	if err := r.c.Follow("desconto"); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	if len(r.f.setIn) == 0 || slices.ContainsFunc(r.f.setIn, func(s string) bool { return s != "Câmara 2" }) {
		t.Errorf("transforms sent to %v", r.f.setIn)
	}
}

func TestPlaceWindowAtOnce(t *testing.T) {
	r := newRig(t, 1280, 720)
	twoScenes(r)
	r.c.in = true // the last screen followed was desconto
	if err, _ := r.c.Place("Câmara 2", "B", nil); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	sets := r.f.transforms()
	if len(sets) != 1 || !near(sets[0]["positionX"].(float64), 416) || !near(sets[0]["scaleX"].(float64), 1.125) {
		t.Errorf("transforms %v", sets)
	}
	if r.win == nil || *r.win != window {
		t.Errorf("overlay window %v", r.win)
	}
	if r.f.setIn[0] != "Câmara 2" || r.f.scene() != "Câmara 1" {
		t.Errorf("placed in %q, program %q", r.f.setIn[0], r.f.scene())
	}
}

func TestPlaceUsesItsOwnFull(t *testing.T) {
	r := newRig(t, 1280, 720)
	twoScenes(r)
	own := &Full{Place: Place{Rect: Rect{10, 20, 1900, 1060}}, SourceWidth: 1280, SourceHeight: 720, CanvasWidth: 1920, CanvasHeight: 1080, Fit: "contain"}
	r.set.Full = &Full{Place: Place{Rect: canvas}, SourceWidth: 1280, SourceHeight: 720, CanvasWidth: 1920, CanvasHeight: 1080, Fit: "contain"}
	r.set.Fulls = map[string]*Full{"B": own}
	if err, _ := r.c.Place("Câmara 2", "B", nil); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	if end := last(t, r.f); !near(end["positionX"].(float64), 10) || !near(end["positionY"].(float64), 20) {
		t.Errorf("placed at %v", end)
	}
	if len(r.saved) != 0 {
		t.Errorf("saved %+v", r.saved)
	}
}

// Camera A's placement (set.Full) is never B's: B with none saved gets a new one.
func TestPlaceNeverTakesTheLiveCamerasFull(t *testing.T) {
	r := newRig(t, 1280, 720)
	twoScenes(r)
	r.set.Source = "A"
	r.set.Full = &Full{Place: Place{Rect: Rect{10, 20, 1900, 1060}}, SourceWidth: 1280, SourceHeight: 720, CanvasWidth: 1920, CanvasHeight: 1080, Fit: "contain"}
	var savedFor []string
	r.c.Save = func(source string, f Full) { savedFor = append(savedFor, source) }
	if err, _ := r.c.Place("Câmara 2", "B", nil); err != nil {
		t.Fatal(err)
	}
	r.wait(t)
	if end := last(t, r.f); !near(end["positionX"].(float64), 0) || !near(end["scaleX"].(float64), 1.5) {
		t.Errorf("placed at %v, not B's own fit", end)
	}
	if !slices.Equal(savedFor, []string{"B"}) {
		t.Errorf("saved for %v", savedFor)
	}
}

// With OBS control off or the old two-scene mode, Place moves nothing but
// still runs then.
func TestPlaceNoOps(t *testing.T) {
	for name, set := range map[string]func(r *rig){
		"disabled": func(r *rig) { r.set.Enabled = false },
		"legacy": func(r *rig) {
			r.f.scenes = append(r.f.scenes, "Desconto")
			r.set.TimeoutScene, r.set.GameScene = "Desconto", "Câmara 1"
		},
	} {
		r := newRig(t, 1280, 720)
		twoScenes(r)
		set(r)
		ran := false
		placed, err := r.c.Place("Câmara 2", "B", func() error { ran = true; return nil })
		if placed != nil || err != nil || !ran {
			t.Errorf("%s: placed %v, err %v, ran %v", name, placed, err, ran)
		}
		if n := len(r.f.transforms()); n != 0 {
			t.Errorf("%s: %d transforms", name, n)
		}
	}
}

// A time-out that comes in while then runs (the switch) waits for it, and
// squeezes the camera then switched to.
func TestPlaceHoldsTheSqueezeUntilTheSwitch(t *testing.T) {
	r := newRig(t, 1280, 720)
	twoScenes(r)
	r.set.Scene, r.set.Source = "Câmara 1", "A"
	followed := make(chan struct{})
	early := false
	_, err := r.c.Place("Câmara 2", "B", func() error {
		go func() { r.c.Follow("desconto"); close(followed) }()
		select {
		case <-followed:
			early = true
		case <-time.After(50 * time.Millisecond):
		}
		r.mu.Lock()
		r.set.Scene, r.set.Source = "Câmara 2", "B"
		r.mu.Unlock()
		return nil
	})
	if err != nil || early {
		t.Fatalf("err %v, squeezed during the switch %v", err, early)
	}
	<-followed
	r.wait(t)
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	if slices.Contains(r.f.setIn, "Câmara 1") {
		t.Errorf("the old camera moved: %v", r.f.setIn)
	}
}

func TestMoving(t *testing.T) {
	r := newRig(t, 1280, 720)
	if err := r.c.Follow("desconto"); err != nil {
		t.Fatal(err)
	}
	if !r.c.Moving() {
		t.Error("not moving right after the squeeze started")
	}
	r.wait(t)
	if r.c.Moving() {
		t.Error("still moving after the tween")
	}
}
