package web

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/update"
)

// updateEvery is how often the update check asks GitHub, while it is on.
const updateEvery = 24 * time.Hour

// keepOldCopy is how long a copy started by an update leaves the program it
// replaced beside it, so a new version that dies at once can be put back.
const keepOldCopy = 10 * time.Minute

// RunUpdates asks GitHub for a newer release once a day while
// settings.update_check is on, and at once when it is switched on. It
// downloads nothing: POST /api/update does, when the operator asks.
func (s *Server) RunUpdates() {
	tick := time.NewTicker(updateEvery)
	defer tick.Stop()
	for {
		if s.Store.Settings().UpdateCheck {
			s.checkUpdate()
		}
		select {
		case <-tick.C:
		case <-s.updateNow:
		}
	}
}

func (s *Server) checkUpdate() {
	url := s.UpdateURL
	if url == "" {
		url = update.URL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rel, err := update.Latest(ctx, http.DefaultClient, url, "Hallstream/"+s.Version)
	if err != nil {
		s.Log(i18n.T("log.atualizacao.erro", "err", err))
		return
	}
	if !update.Newer(s.Version, rel.Version) {
		s.newer.Store(nil)
		return
	}
	if old := s.newer.Swap(&rel); old == nil || old.Version != rel.Version {
		s.Log(i18n.T("log.atualizacao.nova", "versao", rel.Version))
	}
}

// newerRelease is the release the panel offers, or nil: none found, or the
// check is off. While an install runs it is the release being installed.
func (s *Server) newerRelease() *update.Release {
	s.update.mu.Lock()
	rel := s.update.rel
	s.update.mu.Unlock()
	if rel != nil {
		return rel
	}
	if !s.Store.Settings().UpdateCheck {
		return nil
	}
	return s.newer.Load()
}

// updateRun is the install the operator asked for.
type updateRun struct {
	mu       sync.Mutex
	rel      *update.Release // being installed; nil when none runs
	state    string          // "", downloading, installing, restarting, failed
	progress int             // the download's percent; -1 while unknown
	err      string
	once     sync.Once
	refusal  error // why this copy can't install itself, worked out once
}

func (s *Server) install() func(context.Context, *http.Client, update.Release, update.Hooks) error {
	if s.Install != nil {
		return s.Install
	}
	return update.Install
}

func (s *Server) canInstall() error {
	if s.Restart == nil {
		return i18n.Err("log.atualizacao.sem_programa")
	}
	if s.CanInstall != nil {
		return s.CanInstall()
	}
	return update.CanInstall()
}

// cachedRefusal is canInstall worked out once, for the status polls: the
// folder check and codesign are too slow to run every 10 s.
func (s *Server) cachedRefusal() error {
	s.update.once.Do(func() { s.update.refusal = s.canInstall() })
	return s.update.refusal
}

// updateStatus is "update" in GET /api/status: the newer release and, on
// the PC, whether it can be installed from here and how an install goes.
func (s *Server) updateStatus(onPC bool) map[string]any {
	rel := s.newerRelease()
	if rel == nil {
		return nil
	}
	out := map[string]any{"version": rel.Version, "url": rel.URL, "on_pc": onPC, "install": false, "state": "", "progress": -1, "error": ""}
	if !onPC {
		return out
	}
	s.update.mu.Lock()
	out["state"], out["progress"], out["error"] = s.update.state, s.update.progress, s.update.err
	s.update.mu.Unlock()
	out["install"] = s.Restart != nil && s.cachedRefusal() == nil
	return out
}

// updateCheckAsk says whether the panel on the PC should ask once whether
// to check for versions.
func (s *Server) updateCheckAsk(onPC bool) bool {
	set := s.Store.Settings()
	return onPC && !set.UpdateAsked && !set.UpdateCheck
}

// installing says whether an install runs; the stream can't start then.
func (s *Server) installing() bool {
	s.update.mu.Lock()
	defer s.update.mu.Unlock()
	return s.update.rel != nil
}

// answerUpdateAsk is POST /api/update/ask { "check" }: the PC's answer.
func (s *Server) answerUpdateAsk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Check bool `json:"check"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.so_no_pc"))
		return
	}
	set, err := s.Store.UpdateSettings(func(set *store.Settings) {
		set.UpdateAsked = true
		set.UpdateCheck = set.UpdateCheck || req.Check
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Check {
		select {
		case s.updateNow <- struct{}{}:
		default:
		}
	}
	reply(w, forCaller(set, r))
}

// startUpdate is POST /api/update: download the newer release and install
// it, in the background. The panel follows "update" in GET /api/status.
func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.so_no_pc"))
		return
	}
	s.update.mu.Lock()
	busy := s.update.rel != nil
	s.update.mu.Unlock()
	if busy {
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.a_decorrer"))
		return
	}
	rel := s.newerRelease()
	if rel == nil {
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.nenhuma"))
		return
	}
	if err := s.canInstall(); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if s.Streaming() {
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.no_ar"))
		return
	}
	s.update.mu.Lock()
	if s.update.rel != nil {
		s.update.mu.Unlock()
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.a_decorrer"))
		return
	}
	s.update.rel, s.update.state, s.update.progress, s.update.err = rel, "downloading", -1, ""
	s.update.mu.Unlock()
	s.Log(i18n.T("log.atualizacao.a_transferir", "versao", rel.Version))
	s.once(i18n.T("log.tarefa.atualizar"), func() { s.runInstall(*rel) })
	reply(w, map[string]any{"ok": true, "version": rel.Version})
}

func (s *Server) setUpdateState(state string) {
	s.update.mu.Lock()
	s.update.state = state
	s.update.mu.Unlock()
}

// runInstall runs the install and keeps its outcome for the panel. After a
// started restart the run stays marked, so nothing starts another.
func (s *Server) runInstall(rel update.Release) {
	var err error
	finished := false
	defer func() {
		if finished && err == nil {
			return
		}
		if !finished {
			err = i18n.Err("log.atualizacao.interrompida")
		}
		s.Log(i18n.T("log.atualizacao.falhou", "err", err))
		s.update.mu.Lock()
		s.update.rel, s.update.state, s.update.err = nil, "failed", err.Error()
		s.update.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	err = s.install()(ctx, http.DefaultClient, rel, update.Hooks{
		OffAir: func() bool { return !s.Streaming() },
		Status: s.setUpdateState,
		Progress: func(p int) {
			s.update.mu.Lock()
			s.update.progress = p
			s.update.mu.Unlock()
		},
		Restart: s.Restart,
	})
	finished = true
}

// CleanUpAfterUpdate removes, keepOldCopy after a restart by an update, the
// program the update replaced; not while another install runs.
func (s *Server) CleanUpAfterUpdate() {
	s.once(i18n.T("log.tarefa.atualizar"), func() {
		time.Sleep(keepOldCopy)
		if s.installing() {
			return
		}
		if t, err := update.Target(); err == nil {
			update.Cleanup(t)
		}
	})
}
