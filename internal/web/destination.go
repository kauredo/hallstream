package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// rtmp reports whether OBS sends to a custom RTMP server instead of the
// YouTube stream the program manages.
func (s *Server) rtmp() bool { return s.Store.Settings().Stream.Service == store.ServiceRTMP }

// putStream is PUT /api/stream: where OBS sends the picture. It saves the
// choice and, for a custom server, points OBS at it at once.
func (s *Server) putStream(w http.ResponseWriter, r *http.Request) {
	var req store.Stream
	if !decode(w, r, &req) {
		return
	}
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.destino.so_no_pc"))
		return
	}
	req.Server, req.Key = strings.TrimSpace(req.Server), strings.TrimSpace(req.Key)
	switch req.Service {
	case store.ServiceYouTube:
		req.Server, req.Key = "", ""
	case store.ServiceRTMP:
		low := strings.ToLower(req.Server)
		if !strings.HasPrefix(low, "rtmp://") && !strings.HasPrefix(low, "rtmps://") {
			fail(w, http.StatusBadRequest, i18n.T("api.destino.servidor"))
			return
		}
		if req.Key == "" {
			fail(w, http.StatusBadRequest, i18n.T("api.destino.chave"))
			return
		}
	default:
		fail(w, http.StatusBadRequest, i18n.T("api.destino.servico", "servico", req.Service))
		return
	}
	set, err := s.Store.UpdateSettings(func(cur *store.Settings) { cur.Stream = req })
	if !s.save(w, err) {
		return
	}
	out := map[string]any{"stream": set.Stream}
	if req.Service == store.ServiceRTMP {
		out["obs"] = s.rtmpSetupStep()
	} else if s.ytConnected() {
		s.once("YouTube", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, _, err := s.ytPrepare(ctx); err != nil {
				s.ytFailed(err)
			}
		})
	}
	reply(w, out)
}

// rtmpSetupStep points OBS at the custom server: the destination line of
// Configurar OBS, and what PUT /api/stream does.
func (s *Server) rtmpSetupStep() obsws.Step {
	set := s.Store.Settings()
	d := set.Stream
	step := obsws.Step{ID: "rtmp", Label: i18n.T("obs.setup.rtmp")}
	if !set.OBS.Enabled {
		step.Result, step.Detail = "later", i18n.T("obs.setup.rtmp_sem_obs")
		return step
	}
	changed, err := s.OBS.SetCustomService(obsConfig(set.OBS), d.Server, d.Key)
	switch {
	case errors.Is(err, obsws.ErrStreaming):
		step.Result, step.Detail = "later", i18n.T("obs.setup.youtube_depois")
	case err != nil:
		step.Result, step.Detail = "failed", i18n.T("obs.setup.rtmp_falhou", "err", err)
	case changed:
		s.Log(i18n.T("log.rtmp.obs_envia", "servidor", d.Server))
		step.Result, step.Detail = "fixed", i18n.T("obs.setup.rtmp_pus", "servidor", d.Server)
	default:
		step.Result, step.Detail = "ok", i18n.T("obs.setup.rtmp_ok", "servidor", d.Server)
	}
	return step
}
