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
			return obsws.CameraSettings{
				Config:  obsws.Config{Host: o.Host, Port: o.Port, Password: o.Password},
				Enabled: o.Enabled, Source: source, Fit: o.CameraFit,
				TimeoutScene: o.TimeoutScene, GameScene: o.GameScene, Full: o.FullFor(source),
				Scene: scene, Fulls: o.CameraFulls,
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

// programScene takes the program scene from the watch. A camera scene the
// program did not switch to itself (a click in OBS) gets its camera placed
// for the screen, so one left in the window does not stay small.
func (s *Server) programScene(name string) {
	o := s.Store.Settings().OBS
	n := liveIndex(o, name)
	s.liveMu.Lock()
	s.program = name
	expected := s.expect == name
	s.expect = ""
	s.liveMu.Unlock()
	s.State.SetLiveCamera(n)
	if n == 0 || expected || !o.Enabled || s.camErr != nil {
		return
	}
	cam := o.CameraList()[n-1]
	if cam.Scene != name {
		return // the old Hallstream scene: the camera follows the program scene
	}
	s.once(i18n.T("log.tarefa.cenas"), func() {
		if err := s.Camera.Place(cam.Scene, cam.Source, s.Camera.Squeezed()); err != nil {
			s.Log(i18n.T("log.camara.nao_pus", "camara", n, "err", err))
		}
	})
}

// switchTo puts camera n in place (in the window or at full screen, at
// once) and switches OBS to its scene. A failure to place comes back as
// warning and does not stop the switch. The caller holds switchMu.
func (s *Server) switchTo(n int, window bool) (warning string, err error) {
	o := s.Store.Settings().OBS
	cam := o.CameraList()[n-1]
	s.liveMu.Lock()
	onAir := s.program == cam.Scene
	s.liveMu.Unlock()
	// The camera on air is already in place, maybe half way through a squeeze.
	if s.camErr == nil && !onAir {
		if err := s.Camera.Place(cam.Scene, cam.Source, window); err != nil {
			warning = err.Error()
		}
	}
	s.liveMu.Lock()
	s.expect = cam.Scene
	s.liveMu.Unlock()
	if err := s.OBS.SetScene(obsConfig(o), cam.Scene); err != nil {
		s.liveMu.Lock()
		s.expect = ""
		s.liveMu.Unlock()
		return warning, err
	}
	s.liveMu.Lock()
	s.program = cam.Scene
	s.liveMu.Unlock()
	s.State.SetLiveCamera(n)
	return warning, nil
}

// RunScenes keeps a watch on OBS's program scene while OBS control is on,
// so obs.live_camera follows a switch made in OBS too. It never returns.
func (s *Server) RunScenes() {
	wait := time.Second
	for {
		o := s.Store.Settings().OBS
		if !o.Enabled {
			s.State.SetLiveCamera(0)
			time.Sleep(5 * time.Second)
			continue
		}
		cfg := obsConfig(o)
		stop, done := make(chan struct{}), make(chan struct{})
		go func() {
			tick := time.NewTicker(5 * time.Second)
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
				}
			}
		}()
		start, seen := time.Now(), false
		err := obsws.Watch(cfg, stop, func(name string) { seen = true; s.programScene(name) })
		close(done)
		s.liveMu.Lock()
		s.program, s.expect = "", ""
		s.liveMu.Unlock()
		s.State.SetLiveCamera(0)
		if err == nil {
			continue // the settings changed
		}
		if seen {
			s.Log(i18n.T("log.obs.cenas_perdi", "err", err))
		}
		if time.Since(start) > time.Minute {
			wait = time.Second
		}
		time.Sleep(wait)
		wait = min(2*wait, 10*time.Second)
	}
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
			if legacy, _ := s.Camera.Legacy(); legacy {
				fail(w, http.StatusConflict, i18n.T("api.camara.modo_antigo"))
				return
			}
		}
		s.switchMu.Lock()
		defer s.switchMu.Unlock()
		warning, err := s.switchTo(req.Index, s.Camera.Squeezed())
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		reply(w, map[string]any{"ok": true, "live_camera": req.Index, "warning": warning})
	})
}
