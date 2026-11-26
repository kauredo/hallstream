package obsws

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Rect is a box on the OBS canvas, in pixels.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Place is where the camera sits: the box its picture fills and the source
// pixels cut from each side.
type Place struct {
	Rect
	CropLeft   float64 `json:"crop_left"`
	CropRight  float64 `json:"crop_right"`
	CropTop    float64 `json:"crop_top"`
	CropBottom float64 `json:"crop_bottom"`
}

// Contain fits a w x h picture inside box, centred, without cutting it.
func Contain(w, h float64, box Rect) Place {
	s := math.Min(box.W/w, box.H/h)
	return Place{Rect: Rect{box.X + (box.W-w*s)/2, box.Y + (box.H-h*s)/2, w * s, h * s}}
}

// Fill covers box with a w x h picture, cutting what sticks out on two sides.
func Fill(w, h float64, box Rect) Place {
	s := math.Max(box.W/w, box.H/h)
	cx, cy := (w-box.W/s)/2, (h-box.H/s)/2
	return Place{Rect: box, CropLeft: cx, CropRight: cx, CropTop: cy, CropBottom: cy}
}

// Fit is Fill for "fill" and Contain for anything else.
func Fit(fit string, w, h float64, box Rect) Place {
	if fit == "fill" {
		return Fill(w, h, box)
	}
	return Contain(w, h, box)
}

func (p Place) lerp(q Place, k float64) Place {
	l := func(a, b float64) float64 { return a + (b-a)*k }
	return Place{Rect{l(p.X, q.X), l(p.Y, q.Y), l(p.W, q.W), l(p.H, q.H)},
		l(p.CropLeft, q.CropLeft), l(p.CropRight, q.CropRight), l(p.CropTop, q.CropTop), l(p.CropBottom, q.CropBottom)}
}

// transform is p as a sceneItemTransform for a w x h source. OBS crops in
// whole pixels and scales what is left, so the scale follows the rounded crop.
func (p Place) transform(w, h float64) map[string]any {
	l, r := math.Round(p.CropLeft), math.Round(p.CropRight)
	t, b := math.Round(p.CropTop), math.Round(p.CropBottom)
	return map[string]any{
		"positionX": p.X, "positionY": p.Y,
		"scaleX": p.W / (w - l - r), "scaleY": p.H / (h - t - b),
		"alignment":  5, // top left
		"boundsType": "OBS_BOUNDS_NONE",
		"cropLeft":   int(l), "cropRight": int(r), "cropTop": int(t), "cropBottom": int(b),
	}
}

// Bezier is the CSS timing function cubic-bezier(x1, y1, x2, y2).
func Bezier(x1, y1, x2, y2 float64) func(float64) float64 {
	b := func(p1, p2, t float64) float64 { u := 1 - t; return 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t }
	return func(x float64) float64 {
		if x <= 0 || x >= 1 {
			return math.Max(0, math.Min(1, x))
		}
		lo, hi := 0.0, 1.0
		for range 40 {
			if t := (lo + hi) / 2; b(x1, x2, t) < x {
				lo = t
			} else {
				hi = t
			}
		}
		return b(y1, y2, (lo+hi)/2)
	}
}

// CameraSettings is what the camera follows; Camera reads it on every move.
type CameraSettings struct {
	Config
	Enabled      bool
	Source       string // "" picks the camera by itself
	Fit          string // full screen: "contain" or "fill"
	TimeoutScene string // set and present in OBS: the old two-scene mode
	GameScene    string
	Full         *Full
	// Scene is the live camera's scene; "" is the program scene (the old
	// layout, or the live camera not known).
	Scene string
	// Fulls are the saved full-screen placements by source (camera_fulls).
	Fulls map[string]*Full
}

// Full is the camera's full-screen placement and what it was made for.
type Full struct {
	Place
	SourceWidth  float64 `json:"source_width"`
	SourceHeight float64 `json:"source_height"`
	CanvasWidth  float64 `json:"canvas_width"`
	CanvasHeight float64 `json:"canvas_height"`
	Fit          string  `json:"fit"`
}

// Camera moves the camera source between full screen and the time-out
// window in the program scene, one tween at a time.
type Camera struct {
	OBS      *Client
	Settings func() CameraSettings
	Save     func(source string, f Full) // keeps a source's new full-screen placement
	Show     func(full, window *Rect)    // where the picture is, in overlay px; nil when unknown
	Window   Rect                        // the time-out window, in 1920x1080 overlay px
	Dur      time.Duration
	Ease     func(float64) float64
	Step     time.Duration

	follow sync.Mutex // one Follow at a time
	in     bool       // the last screen followed was desconto
	ferr   error

	mu       sync.Mutex
	moving   bool
	it       item
	cur      Place
	from, to Place
	start    time.Time
	dur      time.Duration
	gen      int // counts moves, so the runner sees one that came in
}

type item struct {
	cfg        Config
	scene      string
	id         int
	name       string
	srcW, srcH float64
	canvas     Rect
	place      *Place // where OBS has it now; nil when it uses bounds
}

type sceneItem struct {
	ID        int    `json:"sceneItemId"`
	Index     int    `json:"sceneItemIndex"`
	Name      string `json:"sourceName"`
	Kind      string `json:"inputKind"`
	Enabled   bool   `json:"sceneItemEnabled"`
	Transform struct {
		PositionX, PositionY, ScaleX, ScaleY float64
		SourceWidth, SourceHeight            float64
		Alignment                            int
		BoundsType                           string
		CropLeft, CropRight                  float64
		CropTop, CropBottom                  float64
	} `json:"sceneItemTransform"`
}

// Source is an input in the program scene, for the panel's list.
type Source struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Follow moves the camera for a screen change: into the window on desconto,
// back to full screen after. Repeating the same side returns the last result.
func (c *Camera) Follow(screen string) error {
	c.follow.Lock()
	defer c.follow.Unlock()
	in := screen == "desconto"
	if in == c.in {
		return c.ferr
	}
	c.in = in
	c.ferr = c.squeeze(in)
	return c.ferr
}

func (c *Camera) squeeze(in bool) error {
	set := c.Settings()
	if !set.Enabled {
		return nil
	}
	legacy, err := c.legacy(set)
	if err != nil {
		return err
	}
	if legacy {
		scene := set.GameScene
		if in {
			scene = set.TimeoutScene
		}
		return c.OBS.SetScene(set.Config, scene)
	}
	it, err := c.resolveIn(set, set.Scene)
	if err != nil {
		return err
	}
	full, _ := c.full(set, it)
	win := c.window(it)
	c.show(it, full, win)
	if in {
		c.move(it, win, full, true)
	} else {
		c.move(it, full, win, true)
	}
	return nil
}

// Check refits the full-screen camera when the source or canvas size
// changed (the tablet turned, DroidCam changed resolution). The server calls
// it every few seconds while the screen is marcador or nada.
func (c *Camera) Check() error {
	set := c.Settings()
	legacy, err := c.legacy(set)
	if !set.Enabled || legacy {
		c.Show(nil, nil)
		return err
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	busy := c.moving
	c.mu.Unlock()
	c.follow.Lock()
	in := c.in
	c.follow.Unlock()
	if busy || in {
		return nil
	}
	it, err := c.resolveIn(set, set.Scene)
	if err != nil {
		return err
	}
	full, changed := c.full(set, it)
	c.show(it, full, c.window(it))
	if changed {
		c.move(it, full, full, false)
	}
	return nil
}

// FitReport says what "Ajustar câmara" fitted.
type FitReport struct {
	Source       string  `json:"source"`
	SourceWidth  float64 `json:"source_width"`
	SourceHeight float64 `json:"source_height"`
	CanvasWidth  float64 `json:"canvas_width"`
	CanvasHeight float64 `json:"canvas_height"`
}

// FitNow puts the camera at full screen with the chosen fit, at once, and
// saves a new placement.
func (c *Camera) FitNow() (FitReport, error) {
	set := c.Settings()
	set.Full = nil
	return c.fitIn(set, set.Scene)
}

// FitIn puts source in scene, on program or not, at full screen with its
// own saved placement, or a new one when that no longer fits.
func (c *Camera) FitIn(scene, source string) (FitReport, error) {
	set := c.Settings()
	if source != set.Source {
		set.Full = set.Fulls[source]
	}
	set.Source = source
	return c.fitIn(set, scene)
}

func (c *Camera) fitIn(set CameraSettings, scene string) (FitReport, error) {
	it, err := c.resolveIn(set, scene)
	if err != nil {
		return FitReport{}, err
	}
	full, _ := c.full(set, it)
	onAir := scene == ""
	if !onAir {
		program, err := c.program(set.Config)
		if err != nil {
			return FitReport{}, err
		}
		onAir = program == it.scene
	}
	// The overlay draws the program scene's camera only.
	if onAir {
		c.show(it, full, c.window(it))
	}
	// A squeeze still running keeps its camera: taking over would leave it half way.
	if c.settle(c.Dur + time.Second) {
		c.move(it, full, full, false)
	}
	return FitReport{it.name, it.srcW, it.srcH, it.canvas.W, it.canvas.H}, nil
}

// Place puts source in scene where the screen needs it, at once: in the
// time-out window when the last screen followed was desconto, else at its
// full-screen placement; and shows those boxes to the overlay. Then it runs
// then (when not nil) before any squeeze can start, so the server switches
// OBS to scene there and a time-out that comes in meanwhile squeezes the new
// camera. placed is why the camera could not be placed, which does not stop
// then; err is what then returned.
func (c *Camera) Place(scene, source string, then func() error) (placed, err error) {
	c.follow.Lock()
	defer c.follow.Unlock()
	placed = c.place(scene, source, c.in)
	if then != nil {
		err = then()
	}
	return placed, err
}

func (c *Camera) place(scene, source string, window bool) error {
	set := c.Settings()
	if legacy, err := c.legacy(set); !set.Enabled || legacy || err != nil {
		return err
	}
	set.Source, set.Full = source, nil // Full belongs to the live camera
	it, err := c.resolveIn(set, scene)
	if err != nil {
		return err
	}
	full, _ := c.full(set, it)
	win := c.window(it)
	c.show(it, full, win)
	target := full
	if window {
		target = win
	}
	c.move(it, target, target, false)
	c.Stop() // the switch must not go out before the camera is in place
	return nil
}

// Legacy reports whether the old two-scene mode applies.
func (c *Camera) Legacy() (bool, error) { return c.legacy(c.Settings()) }

// Sources lists the inputs of the program scene, top first, and the one
// that would be picked with no camera set.
func (c *Camera) Sources() (scene string, list []Source, auto string, err error) {
	set := c.Settings()
	scene, items, err := c.items(set.Config)
	if err != nil {
		return "", nil, "", err
	}
	list = []Source{}
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind != "" {
			list = append(list, Source{items[i].Name, items[i].Kind})
		}
	}
	if p := pick(items, ""); p != nil {
		auto = p.Name
	}
	return scene, list, auto, nil
}

// legacy reports whether the old two-scene mode applies.
func (c *Camera) legacy(set CameraSettings) (bool, error) {
	if !set.Enabled || set.TimeoutScene == "" {
		return false, nil
	}
	var r struct {
		Scenes []struct {
			Name string `json:"sceneName"`
		} `json:"scenes"`
	}
	if err := c.OBS.Request(set.Config, "GetSceneList", nil, &r); err != nil {
		return false, err
	}
	for _, s := range r.Scenes {
		if s.Name == set.TimeoutScene {
			return true, nil
		}
	}
	return false, nil
}

func (c *Camera) items(cfg Config) (string, []sceneItem, error) { return c.itemsIn(cfg, "") }

// itemsIn lists scene's items; "" is the program scene.
func (c *Camera) itemsIn(cfg Config, scene string) (string, []sceneItem, error) {
	if scene == "" {
		var err error
		if scene, err = c.program(cfg); err != nil {
			return "", nil, err
		}
	}
	var r struct {
		Items []sceneItem `json:"sceneItems"`
	}
	err := c.OBS.Request(cfg, "GetSceneItemList", map[string]string{"sceneName": scene}, &r)
	return scene, r.Items, err
}

// program is the program scene's name.
func (c *Camera) program(cfg Config) (string, error) {
	var sc struct {
		Name string `json:"sceneName"`
		Old  string `json:"currentProgramSceneName"`
	}
	if err := c.OBS.Request(cfg, "GetCurrentProgramScene", nil, &sc); err != nil {
		return "", err
	}
	if sc.Name == "" {
		return sc.Old, nil
	}
	return sc.Name, nil
}

// pick finds the camera: the named source, else the top shown DroidCam
// input below the browser source, else the top shown video capture there.
func pick(items []sceneItem, name string) *sceneItem {
	if name != "" {
		for i := range items {
			if items[i].Name == name {
				return &items[i]
			}
		}
		return nil
	}
	top := math.MaxInt
	for _, it := range items {
		if it.Kind == "browser_source" && (top == math.MaxInt || it.Index > top) {
			top = it.Index
		}
	}
	var best, droid *sceneItem
	for i, it := range items {
		dc := strings.Contains(strings.ToLower(it.Kind), "droidcam") || strings.Contains(strings.ToLower(it.Name), "droidcam")
		if !(isCapture(it.Kind) || dc) || it.Index >= top || !it.Enabled {
			continue
		}
		if best == nil || it.Index > best.Index {
			best = &items[i]
		}
		if dc && (droid == nil || it.Index > droid.Index) {
			droid = &items[i]
		}
	}
	if droid != nil {
		return droid
	}
	return best
}

// resolveIn finds the camera in scene; "" is the program scene.
func (c *Camera) resolveIn(set CameraSettings, scene string) (item, error) {
	var v struct {
		W float64 `json:"baseWidth"`
		H float64 `json:"baseHeight"`
	}
	if err := c.OBS.Request(set.Config, "GetVideoSettings", nil, &v); err != nil {
		return item{}, err
	}
	scene, items, err := c.itemsIn(set.Config, scene)
	if err != nil {
		return item{}, err
	}
	si := pick(items, set.Source)
	if si == nil {
		if set.Source != "" {
			return item{}, i18n.Err("obs.camera.sem_fonte", "cena", strconv.Quote(scene), "fonte", strconv.Quote(set.Source))
		}
		return item{}, i18n.Err("obs.camera.nao_encontrei", "cena", strconv.Quote(scene))
	}
	tr := si.Transform
	if tr.SourceWidth <= 0 || tr.SourceHeight <= 0 || v.W <= 0 || v.H <= 0 {
		return item{}, i18n.Err("obs.camera.sem_imagem", "camara", si.Name)
	}
	it := item{cfg: set.Config, scene: scene, id: si.ID, name: si.Name,
		srcW: tr.SourceWidth, srcH: tr.SourceHeight, canvas: Rect{0, 0, v.W, v.H}}
	if tr.BoundsType == "" || tr.BoundsType == "OBS_BOUNDS_NONE" {
		w := (tr.SourceWidth - tr.CropLeft - tr.CropRight) * tr.ScaleX
		h := (tr.SourceHeight - tr.CropTop - tr.CropBottom) * tr.ScaleY
		x, y := tr.PositionX, tr.PositionY
		switch {
		case tr.Alignment&2 != 0: // right
			x -= w
		case tr.Alignment&1 == 0: // centre
			x -= w / 2
		}
		switch {
		case tr.Alignment&8 != 0: // bottom
			y -= h
		case tr.Alignment&4 == 0:
			y -= h / 2
		}
		it.place = &Place{Rect{x, y, w, h}, tr.CropLeft, tr.CropRight, tr.CropTop, tr.CropBottom}
	}
	return it, nil
}

// full is the saved full-screen placement, or a new one (saved) when the
// source, canvas or fit changed since.
func (c *Camera) full(set CameraSettings, it item) (Place, bool) {
	fit := "contain"
	if set.Fit == "fill" {
		fit = "fill"
	}
	saved := set.Full
	if f := set.Fulls[it.name]; f != nil {
		saved = f
	}
	if f := saved; f != nil && f.SourceWidth == it.srcW && f.SourceHeight == it.srcH &&
		f.CanvasWidth == it.canvas.W && f.CanvasHeight == it.canvas.H && f.Fit == fit {
		return f.Place, false
	}
	f := Full{Fit(fit, it.srcW, it.srcH, it.canvas), it.srcW, it.srcH, it.canvas.W, it.canvas.H, fit}
	if c.Save != nil {
		c.Save(it.name, f)
	}
	return f.Place, true
}

// window is the camera contained in the time-out window, on the canvas.
func (c *Camera) window(it item) Place {
	sx, sy := it.canvas.W/1920, it.canvas.H/1080
	w := c.Window
	return Contain(it.srcW, it.srcH, Rect{w.X * sx, w.Y * sy, w.W * sx, w.H * sy})
}

func (c *Camera) show(it item, full, win Place) {
	sx, sy := 1920/it.canvas.W, 1080/it.canvas.H
	px := func(r Rect) *Rect { return &Rect{r.X * sx, r.Y * sy, r.W * sx, r.H * sy} }
	c.Show(px(full.Rect), px(win.Rect))
}

// move tweens the camera to target (other is the far end, which sets the
// pace), or puts it there at once. A move while one runs takes over from
// wherever the camera is.
func (c *Camera) move(it item, target, other Place, animate bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.moving {
		c.cur = other
		if it.place != nil {
			c.cur = *it.place
		}
	}
	c.it, c.from, c.to, c.start, c.dur = it, c.cur, target, time.Now(), 0
	c.gen++
	if animate {
		frac := 1.0
		if span := math.Abs(target.W - other.W); span > 0 {
			frac = math.Min(1, math.Abs(target.W-c.cur.W)/span)
		}
		c.dur = time.Duration(float64(c.Dur) * frac)
	}
	if !c.moving {
		c.moving = true
		go c.run()
	}
}

// Stop ends a running tween at its target and waits for it (2 s at most),
// so the program never exits with the camera half way.
func (c *Camera) Stop() {
	c.mu.Lock()
	c.dur = 0
	c.mu.Unlock()
	c.settle(2 * time.Second)
}

// settle waits up to max for a running move to end, and reports whether it did.
func (c *Camera) settle(max time.Duration) bool {
	for end := time.Now().Add(max); ; time.Sleep(c.Step) {
		c.mu.Lock()
		moving := c.moving
		c.mu.Unlock()
		if !moving {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

func (c *Camera) run() {
	tick := time.NewTicker(c.Step)
	defer tick.Stop()
	for {
		c.mu.Lock()
		k := 1.0
		if c.dur > 0 {
			k = math.Min(1, float64(time.Since(c.start))/float64(c.dur))
		}
		p := c.from.lerp(c.to, c.Ease(k))
		it, gen := c.it, c.gen
		c.cur = p
		done := k >= 1
		c.mu.Unlock()
		err := c.OBS.Request(it.cfg, "SetSceneItemTransform", map[string]any{
			"sceneName": it.scene, "sceneItemId": it.id, "sceneItemTransform": p.transform(it.srcW, it.srcH),
		}, nil)
		c.mu.Lock()
		// A move that came in meanwhile keeps the tween going.
		if err != nil || done && gen == c.gen {
			c.moving = false
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		<-tick.C
	}
}
