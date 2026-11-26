package web

import (
	"cmp"
	"errors"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
)

// camState is one camera's health for the panel: "no_image", "frozen",
// "ok", or "" before the first look.
type camState struct {
	Index int    `json:"index"`
	State string `json:"state"`
}

// frameSample is OBS's frame counts as one round read them.
type frameSample struct {
	obsws.Frames
	at time.Time
}

// The skipped frames cover the samples of the last frameWindow; they show
// once the samples span at least frameSpan, and only when more than
// 1/skippedShare of the frames were skipped, so a stray frame, or the
// rounds' own screenshots, never warn.
const (
	frameWindow  = 65 * time.Second
	frameSpan    = 50 * time.Second
	skippedShare = 100
)

// RunCameraHealth looks at every camera every 10 s. It never returns.
func (s *Server) RunCameraHealth() {
	for range time.Tick(10 * time.Second) {
		s.cameraRound()
	}
}

// busy: a squeeze, a camera switch, or a setup or health check is sending
// to OBS. They share the one connection, and a screenshot between two
// squeeze steps would make it stutter on air.
func (s *Server) busy() bool {
	if s.Camera.Moving() || s.setupOn.Load() {
		return true
	}
	if !s.switchMu.TryLock() {
		return true
	}
	s.switchMu.Unlock()
	return false
}

// cameraRound looks at each camera once and reads OBS's frame counts. With
// OBS off or not connected it forgets every state and sample. It skips the
// round, keeping the last states, while another round, a setup or health
// check, a squeeze or a switch runs, and stops when one starts during it.
func (s *Server) cameraRound() {
	o := s.Store.Settings().OBS
	if connected, _ := s.OBS.Status(); !o.Enabled || !connected {
		s.obsMu.Lock()
		s.camVitals, s.camStates, s.frames = nil, nil, nil
		s.obsMu.Unlock()
		return
	}
	if !s.roundMu.TryLock() {
		return
	}
	defer s.roundMu.Unlock()
	if s.busy() {
		return
	}
	cfg := obsConfig(o)
	s.obsMu.Lock()
	vitals := s.camVitals
	s.obsMu.Unlock()
	next := map[string]obsws.Vitals{}
	states := []camState{}
	for i, cam := range o.CameraList() {
		if cam.Source == "" {
			continue
		}
		key := cam.Scene + "\x00" + cam.Source
		prev := vitals[key]
		cur := prev
		l, err := s.OBS.LookAt(cfg, cam.Scene, cam.Source, s.busy)
		switch {
		case errors.Is(err, obsws.ErrBusy):
			return
		case errors.Is(err, obsws.ErrNotShown):
			// Another scene is on program: nothing to judge.
		case err != nil:
			cur = prev.Failed()
		default:
			cur = prev.See(l)
		}
		if prev.State != "" && cur.State != "" && cur.State != prev.State {
			s.logHealth(cmp.Or(cam.Label, cam.Source), cur.State)
		}
		next[key] = cur
		states = append(states, camState{i + 1, cur.State})
	}
	s.obsMu.Lock()
	s.camVitals, s.camStates = next, states
	s.obsMu.Unlock()
	if s.busy() {
		return
	}
	f, err := s.OBS.Stats(cfg)
	if err != nil {
		return
	}
	s.obsMu.Lock()
	defer s.obsMu.Unlock()
	s.addFrames(frameSample{f, time.Now()})
}

// addFrames keeps sample and the ones from the frameWindow before it. A
// count lower than the last one (OBS started again) starts the samples
// again. The caller holds obsMu.
func (s *Server) addFrames(f frameSample) {
	if n := len(s.frames); n > 0 && (f.Total < s.frames[n-1].Total || f.Skipped < s.frames[n-1].Skipped) {
		s.frames = nil
	}
	s.frames = append(s.frames, f)
	for len(s.frames) > 1 && f.at.Sub(s.frames[0].at) > frameWindow {
		s.frames = s.frames[1:]
	}
}

func (s *Server) logHealth(name, state string) {
	switch state {
	case "no_image":
		s.Log(i18n.T("log.camara.sem_imagem", "camara", name))
	case "frozen":
		s.Log(i18n.T("log.camara.parada", "camara", name))
	case "ok":
		s.Log(i18n.T("log.camara.voltou", "camara", name))
	}
}

// skipped is OBS's skipped and total frames over the last minute, or nil
// before the samples span frameSpan or when at most 1/skippedShare of the
// frames were skipped. The caller holds obsMu.
func (s *Server) skipped() map[string]float64 {
	if len(s.frames) < 2 {
		return nil
	}
	first, last := s.frames[0], s.frames[len(s.frames)-1]
	if last.at.Sub(first.at) < frameSpan {
		return nil
	}
	n, total := last.Skipped-first.Skipped, last.Total-first.Total
	if n*skippedShare <= total || n <= 0 {
		return nil
	}
	return map[string]float64{"frames": n, "total": total}
}
