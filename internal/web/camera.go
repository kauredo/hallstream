package web

import (
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// The time-out window, its timing and its curve are defined once, in
// overlay.css; the camera reads the same values.
var (
	cssPx     = regexp.MustCompile(`--(win-[xywh]):\s*([\d.]+)px`)
	cssMs     = regexp.MustCompile(`--t-squeeze:\s*([\d.]+)ms`)
	cssBezier = regexp.MustCompile(`--ease-in-out:\s*cubic-bezier\(([^)]*)\)`)
)

type squeezeTokens struct {
	win  obsws.Rect
	dur  time.Duration
	ease func(float64) float64
}

func readSqueeze(css string) (squeezeTokens, error) {
	var t squeezeTokens
	px := map[string]float64{}
	for _, m := range cssPx.FindAllStringSubmatch(css, -1) {
		px[m[1]], _ = strconv.ParseFloat(m[2], 64)
	}
	ms := cssMs.FindStringSubmatch(css)
	bz := cssBezier.FindStringSubmatch(css)
	if len(px) != 4 || ms == nil || bz == nil {
		return t, i18n.Err("log.camara.css_sem_janela")
	}
	var b [4]float64
	if n, _ := fmt.Sscanf(bz[1], "%g,%g,%g,%g", &b[0], &b[1], &b[2], &b[3]); n != 4 {
		return t, i18n.Err("log.camara.css_sem_ease")
	}
	d, _ := strconv.ParseFloat(ms[1], 64)
	t.win = obsws.Rect{X: px["win-x"], Y: px["win-y"], W: px["win-w"], H: px["win-h"]}
	t.dur = time.Duration(d * float64(time.Millisecond))
	t.ease = obsws.Bezier(b[0], b[1], b[2], b[3])
	return t, nil
}

func (s *Server) newCamera() {
	css, _ := fs.ReadFile(s.Files, "overlay/overlay.css")
	tok, err := readSqueeze(string(css))
	if err != nil {
		s.camErr = err
	}
	s.Camera = &obsws.Camera{
		OBS: s.OBS,
		Settings: func() obsws.CameraSettings {
			o := s.Store.Settings().OBS
			scene, source := s.liveScene(o)
			fulls := o.CameraFulls
			if f := o.FullFor(o.CameraSource); f != nil && fulls[o.CameraSource] == nil {
				// Camera 1 saved before camera_fulls: its placement is camera_full.
				fulls = maps.Clone(fulls)
				if fulls == nil {
					fulls = map[string]*obsws.Full{}
				}
				fulls[o.CameraSource] = f
			}
			return obsws.CameraSettings{
				Config:  obsws.Config{Host: o.Host, Port: o.Port, Password: o.Password},
				Enabled: o.Enabled, Source: source, Fit: o.CameraFit,
				TimeoutScene: o.TimeoutScene, GameScene: o.GameScene, Full: o.FullFor(source),
				Scene: scene, Fulls: fulls,
			}
		},
		Save: func(source string, f obsws.Full) {
			_, err := s.Store.UpdateSettings(func(set *store.Settings) {
				fulls := maps.Clone(set.OBS.CameraFulls)
				if fulls == nil {
					fulls = map[string]*obsws.Full{}
				}
				fulls[source] = &f
				set.OBS.CameraFulls = fulls
				if cameraOne(set.OBS, source) {
					set.OBS.CameraFull = &f
				}
			})
			if err != nil {
				s.Log(i18n.T("log.camara.nao_guardei_posicao", "err", err))
			}
		},
		Show: func(full, win *obsws.Rect) {
			if full == nil || win == nil {
				s.State.SetCamera(nil)
				return
			}
			s.State.SetCamera(&state.Camera{Full: state.Rect(*full), Window: state.Rect(*win)})
		},
		Window: tok.win,
		Dur:    tok.dur,
		Ease:   tok.ease,
		Step:   time.Second / 50,
	}
}

// cameraOne reports whether source is camera 1. With camera_source "" the
// automatic camera is, unless source is another camera's.
func cameraOne(o store.OBS, source string) bool {
	if o.CameraSource != "" {
		return source == o.CameraSource
	}
	return !slices.ContainsFunc(o.CameraList()[1:], func(c store.Camera) bool { return c.Source == source })
}

// followCamera moves the camera with the screen.
func (s *Server) followCamera() error {
	if s.camErr != nil {
		return s.camErr
	}
	return s.Camera.Follow(s.State.Snapshot().Screen.Current)
}

// RunCamera keeps the OBS camera with the screen (automatic changes too) and
// refits it when the tablet's picture changes size. It never returns.
func (s *Server) RunCamera() {
	if s.camErr != nil {
		s.Log(i18n.T("log.camara.erro", "err", s.camErr))
		return
	}
	ch, _ := s.State.Subscribe()
	check := time.NewTicker(4 * time.Second)
	refit := func() {
		if cur := s.State.Snapshot().Screen.Current; cur == "marcador" || cur == "nada" {
			s.Camera.Check()
		}
	}
	refit()
	for {
		select {
		case <-ch:
			s.followCamera()
		case <-check.C:
			refit()
		}
	}
}

// liveScene is the program scene and its camera's source when the program
// is a camera's scene, else "" and camera_source (the camera in whatever
// scene is on program).
func (s *Server) liveScene(o store.OBS) (scene, source string) {
	s.liveMu.Lock()
	program := s.program
	s.liveMu.Unlock()
	for _, c := range o.CameraList() {
		if c.Scene != "" && c.Scene == program {
			return c.Scene, c.Source
		}
	}
	return "", o.CameraSource
}

// liveIndex is the camera whose scene is scene, 1 to 3, or 0. With one
// camera the old Hallstream scene counts as camera 1.
func liveIndex(o store.OBS, scene string) int {
	cams := o.CameraList()
	for i, c := range cams {
		if c.Scene != "" && c.Scene == scene {
			return i + 1
		}
	}
	if len(cams) == 1 && scene == obsws.SceneName {
		return 1
	}
	return 0
}

// programScene takes a program scene change from the watch.
func (s *Server) programScene(name string) { s.sceneSeen(name, true) }

// sceneSeen takes the program scene from the watch: a change (event), or
// the answer to its question. A change the panel asked for is crossed off
// expect. A camera scene the program did not switch to itself (a click in
// OBS, a scene renamed back) gets its camera placed for the screen, so one
// left in the window does not stay small. OBS sends each change about
// 300 ms after the switch, so while a later switch from the panel is still
// on its way an earlier scene does not become the program again.
func (s *Server) sceneSeen(name string, event bool) {
	o := s.Store.Settings().OBS
	n := liveIndex(o, name)
	s.liveMu.Lock()
	s.dropExpired()
	expected := false
	if event {
		i := slices.IndexFunc(s.expect, func(e pending) bool { return e.scene == name })
		if expected = i >= 0; expected {
			s.expect = s.expect[i+1:]
		} else {
			s.expect = nil // an OBS click; a switch whose event never came is over
		}
	}
	if len(s.expect) > 0 {
		s.liveMu.Unlock()
		return
	}
	changed := s.program != name
	s.program = name
	s.State.SetLiveCamera(n)
	s.liveMu.Unlock()
	if !changed || expected || n == 0 || !o.Enabled || s.camErr != nil {
		return
	}
	cam := o.CameraList()[n-1]
	if cam.Scene != name {
		return // the old Hallstream scene: the camera follows the program scene
	}
	s.once(i18n.T("log.tarefa.cenas"), func() {
		if placed, _ := s.Camera.Place(cam.Scene, cam.Source, nil); placed != nil {
			s.Log(i18n.T("log.camara.nao_pus", "camara", n, "err", placed))
		}
	})
}

// pending is a scene the program switched to, waiting for its event until
// until.
type pending struct {
	scene string
	until time.Time
}

// expectFor is how long a switch waits for its event. OBS sends it about
// 315 ms after a switch with the default 300 ms fade; 3 s also covers a
// slow fade or a stinger. An event later than that finds the scene already
// the program, so it places nothing.
var expectFor = 3 * time.Second

// dropExpired forgets the switches whose events never came (OBS skipped
// it, or a transition far longer than expectFor), so the watch's answers
// and liveAgain count again. The caller holds liveMu.
func (s *Server) dropExpired() {
	now := time.Now()
	s.expect = slices.DeleteFunc(s.expect, func(e pending) bool { return now.After(e.until) })
}

// liveAgain works obs.live_camera out again from the program scene, for
// settings that changed with no scene event (a camera removed, reordered
// or given another scene).
func (s *Server) liveAgain() {
	o := s.Store.Settings().OBS
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.dropExpired()
	if s.program != "" && len(s.expect) == 0 {
		s.State.SetLiveCamera(liveIndex(o, s.program))
	}
}

// switchTo puts camera cam, number n, where the screen needs it (in the
// window or at full screen, at once) and switches OBS to its scene, with no
// squeeze in between. A failure to place comes back as warning and does not
// stop the switch. The caller holds switchMu.
func (s *Server) switchTo(n int, cam store.Camera) (warning string, err error) {
	cfg := obsConfig(s.Store.Settings().OBS)
	s.liveMu.Lock()
	onAir := s.program == cam.Scene
	s.liveMu.Unlock()
	do := func() error {
		s.liveMu.Lock()
		s.expect = append(s.expect, pending{cam.Scene, time.Now().Add(expectFor)})
		s.liveMu.Unlock()
		err := s.OBS.SetScene(cfg, cam.Scene)
		s.liveMu.Lock()
		defer s.liveMu.Unlock()
		k := len(s.expect) - 1
		if k < 0 || s.expect[k].scene != cam.Scene {
			k = -1 // the event came already, or the entry expired
		}
		if err != nil {
			if k >= 0 {
				s.expect = s.expect[:k] // no event comes for a refused switch
			}
			return err
		}
		if k >= 0 {
			s.expect[k].until = time.Now().Add(expectFor) // the transition starts now
		}
		s.program = cam.Scene
		s.State.SetLiveCamera(n)
		return nil
	}
	// The camera on air is already in place, maybe half way through a squeeze.
	if s.camErr != nil || onAir {
		return "", do()
	}
	placed, err := s.Camera.Place(cam.Scene, cam.Source, do)
	if placed != nil {
		warning = placed.Error()
	}
	return warning, err
}

type watchFunc func(cfg obsws.Config, stop <-chan struct{}, scene func(name string, event bool)) error

// The scene watch's timing; tests shorten them.
var (
	scenesPoll   = 5 * time.Second  // how often the settings are read again
	scenesWait   = time.Second      // the first wait after a lost watch
	scenesMax    = 10 * time.Second // the longest wait
	scenesSteady = time.Minute      // a watch that lasted this long starts the waits again
)

// RunScenes keeps a watch on OBS's program scene while OBS control is on,
// so obs.live_camera follows a switch made in OBS too. It never returns.
func (s *Server) RunScenes() {
	wait := scenesWait
	for {
		var pause time.Duration
		pause, wait = s.scenesOnce(wait)
		time.Sleep(pause)
	}
}

// scenesOnce runs one watch, or none while OBS control is off, and returns
// the pause before the next and the wait after the next failure.
func (s *Server) scenesOnce(wait time.Duration) (pause, next time.Duration) {
	o := s.Store.Settings().OBS
	if !o.Enabled {
		s.State.SetLiveCamera(0)
		return scenesPoll, wait
	}
	cfg := obsConfig(o)
	stop, done := make(chan struct{}), make(chan struct{})
	tick := time.NewTicker(scenesPoll)
	go func() {
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if o := s.Store.Settings().OBS; !o.Enabled || obsConfig(o) != cfg {
					close(stop)
					return
				}
				s.liveAgain()
			}
		}
	}()
	start, seen := time.Now(), false
	err := s.watch(cfg, stop, func(name string, event bool) { seen = true; s.sceneSeen(name, event) })
	close(done)
	s.liveMu.Lock()
	s.program, s.expect = "", nil
	s.State.SetLiveCamera(0)
	s.liveMu.Unlock()
	if err == nil {
		return 0, wait // the settings changed
	}
	if seen {
		s.Log(i18n.T("log.obs.cenas_perdi", "err", err))
	}
	if time.Since(start) > scenesSteady {
		wait = scenesWait
	}
	return wait, min(2*wait, scenesMax)
}

func (s *Server) cameraRoutes() {
	s.mux.HandleFunc("GET /api/obs/sources", func(w http.ResponseWriter, r *http.Request) {
		scene, list, auto, err := s.Camera.Sources()
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, map[string]any{"scene": scene, "sources": list, "auto": auto})
	})
	s.mux.HandleFunc("POST /api/obs/camera/fit", func(w http.ResponseWriter, r *http.Request) {
		if s.camErr != nil {
			fail(w, http.StatusInternalServerError, s.camErr.Error())
			return
		}
		rep, err := s.Camera.FitNow()
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		res := map[string]any{"ok": true, "fit": rep, "warning": ""}
		if rep.CanvasWidth != 1920 || rep.CanvasHeight != 1080 {
			res["warning"] = i18n.T("api.camara.resolucao_base", "resolucao", fmt.Sprintf("%.0fx%.0f", rep.CanvasWidth, rep.CanvasHeight))
		}
		reply(w, res)
	})
	s.mux.HandleFunc("POST /api/obs/camera/live", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Index int `json:"index"`
		}
		if !decode(w, r, &req) {
			return
		}
		o := s.Store.Settings().OBS
		if !o.Enabled {
			fail(w, http.StatusConflict, i18n.T("api.camara.obs_desligado"))
			return
		}
		cams := o.CameraList()
		if req.Index < 1 || req.Index > len(cams) {
			fail(w, http.StatusBadRequest, i18n.T("api.camara.nao_existe", "n", req.Index))
			return
		}
		if cams[req.Index-1].Scene == "" {
			fail(w, http.StatusConflict, i18n.T("api.camara.sem_cena", "n", req.Index))
			return
		}
		if s.State.Snapshot().Screen.Current == "desconto" {
			// The old mode's time-out scene is on program; a switch would leave it.
			legacy, err := s.Camera.Legacy()
			if err != nil {
				fail(w, http.StatusBadGateway, err.Error())
				return
			}
			if legacy {
				fail(w, http.StatusConflict, i18n.T("api.camara.modo_antigo"))
				return
			}
		}
		s.switchMu.Lock()
		defer s.switchMu.Unlock()
		warning, err := s.switchTo(req.Index, cams[req.Index-1])
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, map[string]any{"ok": true, "live_camera": req.Index, "warning": warning})
	})
}
