package web

import (
	"cmp"
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

// camLook is a camera's last look and the state judged from it.
type camLook struct {
	look  obsws.Look
	state string
}

// frameSamples is one minute of 10 s rounds, both ends included.
const frameSamples = 7

// RunCameraHealth looks at every camera every 10 s. It never returns.
func (s *Server) RunCameraHealth() {
	for range time.Tick(10 * time.Second) {
		s.cameraRound()
	}
}

// busy: a squeeze or a camera switch is sending to OBS. They share the one
// connection, and a screenshot between two squeeze steps would make it
// stutter on air.
func (s *Server) busy() bool {
	if s.Camera.Moving() {
		return true
	}
	if !s.switchMu.TryLock() {
		return true
	}
	s.switchMu.Unlock()
	return false
}

// cameraRound looks at each camera once and reads OBS's frame counts. It
// skips the round, keeping the last states and looks, when OBS is off or
// not connected, during a setup or health check, and while a squeeze or a
// switch runs.
func (s *Server) cameraRound() {
	o := s.Store.Settings().OBS
	if connected, _ := s.OBS.Status(); !o.Enabled || !connected || s.busy() {
		return
	}
	if !s.setupOBSMu.TryLock() {
		return
	}
	defer s.setupOBSMu.Unlock()
	cfg := obsConfig(o)
	s.obsMu.Lock()
	looks := s.camLooks
	s.obsMu.Unlock()
	next := map[string]camLook{}
	states := []camState{}
	for i, cam := range o.CameraList() {
		if cam.Source == "" {
			continue
		}
		key := cam.Scene + "\x00" + cam.Source
		prev := looks[key]
		if s.busy() {
			return
		}
		cur := prev
		if l, err := s.OBS.LookAt(cfg, cam.Scene, cam.Source); err == nil {
			cur = camLook{l, obsws.Judge(prev.look, l)}
			if prev.state != "" && cur.state != prev.state {
				s.logHealth(cmp.Or(cam.Label, cam.Source), cur.state)
			}
		}
		next[key] = cur
		states = append(states, camState{i + 1, cur.state})
	}
	s.obsMu.Lock()
	s.camLooks, s.camStates = next, states
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
	if n := len(s.frames); n > 0 && f.Total < s.frames[n-1].Total {
		s.frames = nil // OBS started again
	}
	s.frames = append(s.frames, f)
	if n := len(s.frames); n > frameSamples {
		s.frames = s.frames[n-frameSamples:]
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
// before a minute of samples or when none were skipped. The caller holds
// obsMu.
func (s *Server) skipped() map[string]float64 {
	if len(s.frames) < frameSamples {
		return nil
	}
	first, last := s.frames[0], s.frames[len(s.frames)-1]
	if last.Skipped-first.Skipped <= 0 {
		return nil
	}
	return map[string]float64{"frames": last.Skipped - first.Skipped, "total": last.Total - first.Total}
}
