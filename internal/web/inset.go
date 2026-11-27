package web

import (
	"net/http"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// The inset's box on the 1920x1080 overlay.
const insetW, insetH = 480, 270

// insetBox is the inset's box on the overlay, at the overlay's margin: the
// bottom corner the lower third does not take, or with the bug at the
// bottom (the lower third steps around it there), the top corner away from
// the bug.
func insetBox(o store.Overlay) obsws.Rect {
	o.Clean()
	m, w, h := float64(o.Margin), float64(insetW), float64(insetH)
	left, right := m, 1920-m-w
	if bug, ok := strings.CutPrefix(o.BugPosition, "bottom-"); ok {
		x := left
		if bug == "left" {
			x = right
		}
		return obsws.Rect{X: x, Y: m, W: w, H: h}
	}
	x := right
	if o.LowerThird == "right" {
		x = left
	}
	return obsws.Rect{X: x, Y: 1080 - m - h, W: w, H: h}
}

// insetKey is what the inset was last applied for.
type insetKey struct {
	set        bool
	cfg        obsws.Config
	on         bool
	live, pick int
	at         obsws.ItemRef
	box        obsws.Rect
	cams       string // each camera's scene and source
}

// applyInset shows the picked camera small in the scene of camera live, on
// marcador and nada only and never when it is live itself, and hides the
// item it showed before, then puts where it shows in the live state. The
// first apply, and the first after a failure, hides every inset item. With
// none picked and none shown it asks OBS nothing. The same screen, cameras
// and place return the last result unless force. The caller holds switchMu.
func (s *Server) applyInset(live int, force bool) error {
	if s.inset == 0 && s.insetOn == nil {
		s.insetErr = nil
		s.State.SetInset(0, nil)
		return nil
	}
	set := s.Store.Settings()
	o := set.OBS
	cfg := obsConfig(o)
	cams := o.CameraList()
	n := s.inset
	screen := s.State.Snapshot().Screen.Current
	var show *obsws.ItemRef
	if o.Enabled && n >= 1 && n <= len(cams) && live >= 1 && live <= len(cams) && n != live &&
		(screen == "marcador" || screen == "nada") && cams[live-1].Scene != "" && cams[n-1].Source != "" {
		show = &obsws.ItemRef{Scene: cams[live-1].Scene, Source: cams[n-1].Source}
	}
	key := insetKey{set: true, cfg: cfg, on: o.Enabled, live: live, pick: n, box: insetBox(set.Overlay)}
	if show != nil {
		key.at = *show
	}
	var b strings.Builder
	for _, c := range cams {
		b.WriteString(c.Scene + "\x00" + c.Source + "\x00")
	}
	key.cams = b.String()
	if !force && key == s.insetKey {
		return s.insetErr
	}
	s.insetKey = key
	if !o.Enabled {
		s.insetOn, s.insetErr = nil, nil
		s.State.SetInset(n, nil)
		return nil
	}
	var hide []obsws.ItemRef
	if !s.insetSwept {
		for _, c := range cams {
			for _, x := range cams {
				if c.Scene != "" && x.Source != "" && x.Source != c.Source {
					hide = append(hide, obsws.ItemRef{Scene: c.Scene, Source: x.Source})
				}
			}
		}
	}
	if s.insetOn != nil {
		hide = append(hide, *s.insetOn)
	}
	home := ""
	if show != nil {
		home = cams[n-1].Scene
	}
	r, err := s.OBS.Inset(cfg, hide, show, home, key.box)
	if err != nil {
		if s.insetErr == nil || s.insetErr.Error() != err.Error() {
			s.Log(i18n.T("log.camara.inset_falhou", "err", err))
		}
		s.insetErr, s.insetSwept = err, false
		if show != nil {
			s.insetOn = show
		}
		s.State.SetInset(n, nil)
		return err
	}
	s.insetErr, s.insetSwept, s.insetOn = nil, true, show
	var in *state.Inset
	if r != nil {
		label := cams[n-1].Label
		if label == "" {
			label = i18n.T("obs.cena_camara", "n", n)
		}
		in = &state.Inset{Label: label, Rect: state.Rect(*r)}
	}
	s.State.SetInset(n, in)
	return nil
}

// insetRetry applies the inset again when the last apply failed.
func (s *Server) insetRetry() {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()
	if s.insetErr != nil {
		s.applyInset(s.State.Snapshot().OBS.LiveCamera, true)
	}
}

// insetRoute is POST /api/obs/inset: { "camera": n }, 0 for none.
func (s *Server) insetRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Camera int `json:"camera"`
	}
	if !decode(w, r, &req) {
		return
	}
	o := s.Store.Settings().OBS
	if !o.Enabled {
		fail(w, http.StatusConflict, i18n.T("api.camara.obs_desligado"))
		return
	}
	if req.Camera < 0 || req.Camera > len(o.CameraList()) {
		fail(w, http.StatusBadRequest, i18n.T("api.camara.nao_existe", "n", req.Camera))
		return
	}
	s.switchMu.Lock()
	defer s.switchMu.Unlock()
	was := s.inset
	s.inset = req.Camera
	if err := s.applyInset(s.State.Snapshot().OBS.LiveCamera, true); err != nil {
		s.inset, s.insetKey = was, insetKey{}
		s.State.SetInset(was, nil)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	reply(w, map[string]any{"ok": true, "camera": req.Camera})
}

// insetMoved moves the inset's pick with its camera in a new camera list; a
// removed camera turns it off. The caller holds switchMu.
func (s *Server) insetMoved(list []setupCamera) {
	n := 0
	for i, c := range list {
		if c.Was > 0 && c.Was == s.inset {
			n = i + 1
		}
	}
	s.inset = n
	in := s.State.Snapshot().OBS.Inset
	if n == 0 {
		in = nil
	}
	s.State.SetInset(n, in)
}

// insetAfterSetup applies the inset again once a setup has run.
func (s *Server) insetAfterSetup() {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()
	if s.inset > len(s.Store.Settings().OBS.CameraList()) {
		s.inset = 0
	}
	s.applyInset(s.State.Snapshot().OBS.LiveCamera, true)
}

// insetShown is the item the inset shows now, for the setup to leave shown.
func (s *Server) insetShown() obsws.ItemRef {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()
	if s.insetOn == nil {
		return obsws.ItemRef{}
	}
	return *s.insetOn
}
