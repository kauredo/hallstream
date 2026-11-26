package web

import (
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"regexp"
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
			return obsws.CameraSettings{
				Config:  obsws.Config{Host: o.Host, Port: o.Port, Password: o.Password},
				Enabled: o.Enabled, Source: o.CameraSource, Fit: o.CameraFit,
				TimeoutScene: o.TimeoutScene, GameScene: o.GameScene, Full: o.FullFor(o.CameraSource),
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
				// An automatic camera ("") is camera 1 too.
				if source == set.OBS.CameraSource || set.OBS.CameraSource == "" {
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
}
