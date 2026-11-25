package web

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/youtube"
)

// setupSteps are the ids /api/setup/steps/{id} takes: the steps and the
// ticks inside them that the program can't check by itself.
var setupSteps = []string{"idioma", "clube", "entrada", "youtube", "canal-diretos", "canal-verificado",
	"ensaio-demo", "ensaio-previa", "ensaio-desconto", "ensaio-promo", "ensaio-teste"}

func (s *Server) setupRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/setup", func(w http.ResponseWriter, r *http.Request) { s.setupReply(w, r, s.Store.Settings()) })
	m.HandleFunc("PUT /api/setup/steps/{id}", s.setupStep)
	m.HandleFunc("POST /api/setup/done", func(w http.ResponseWriter, r *http.Request) {
		s.setupChange(w, r, func(f *store.FirstSetup) { f.Done, f.Later = time.Now().Format(time.RFC3339), false })
	})
	m.HandleFunc("POST /api/setup/later", func(w http.ResponseWriter, r *http.Request) {
		s.setupChange(w, r, func(f *store.FirstSetup) { f.Later = true })
	})
	m.HandleFunc("POST /api/setup/channel", s.setupChannel)
}

func (s *Server) setupReply(w http.ResponseWriter, r *http.Request, set store.Settings) {
	f := set.FirstSetup
	if f.Steps == nil {
		f.Steps = map[string]string{}
	}
	reply(w, map[string]any{"pc": isLoopback(r.RemoteAddr), "done": f.Done, "later": f.Later, "steps": f.Steps})
}

// onPC refuses a request from another device: the setup is done on the PC.
func onPC(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.setup.so_no_pc"))
		return false
	}
	return true
}

func (s *Server) setupChange(w http.ResponseWriter, r *http.Request, fn func(*store.FirstSetup)) {
	if !decode(w, r, &struct{}{}) || !onPC(w, r) {
		return
	}
	set, err := s.Store.UpdateSettings(func(cur *store.Settings) { fn(&cur.FirstSetup) })
	if s.save(w, err) {
		s.setupReply(w, r, set)
	}
}

func (s *Server) setupStep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State string `json:"state"`
	}
	id := r.PathValue("id")
	if !decode(w, r, &req) || !onPC(w, r) {
		return
	}
	if !slices.Contains(setupSteps, id) {
		fail(w, http.StatusNotFound, i18n.T("api.setup.passo_desconhecido", "passo", id))
		return
	}
	if req.State != "" && req.State != "done" && req.State != "skipped" {
		fail(w, http.StatusBadRequest, i18n.T("api.setup.estado_invalido", "estado", req.State))
		return
	}
	set, err := s.Store.UpdateSettings(func(cur *store.Settings) {
		steps := maps.Clone(cur.FirstSetup.Steps)
		if steps == nil {
			steps = map[string]string{}
		}
		if req.State == "" {
			delete(steps, id)
		} else {
			steps[id] = req.State
		}
		cur.FirstSetup.Steps = steps
	})
	if s.save(w, err) {
		s.setupReply(w, r, set)
	}
}

// setupChannel asks YouTube whether the channel can go live and is verified.
func (s *Server) setupChannel(w http.ResponseWriter, r *http.Request) {
	if !decode(w, r, &struct{}{}) || !onPC(w, r) {
		return
	}
	if !s.ytConnected() {
		fail(w, http.StatusConflict, i18n.T("api.setup.yt_primeiro"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.ytClient().CheckChannel(ctx)
	if err != nil {
		if errors.Is(err, youtube.ErrReconnect) {
			s.ytFailed(err)
		}
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	reply(w, res)
}
