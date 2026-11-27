package web

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/update"
)

// updateEvery is how often the update check asks GitHub, while it is on.
const updateEvery = 24 * time.Hour

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
// check is off.
func (s *Server) newerRelease() *update.Release {
	if !s.Store.Settings().UpdateCheck {
		return nil
	}
	return s.newer.Load()
}

// Updater finds, fetches, unpacks and swaps the program; the real one is
// internal/update, and tests put in one that leaves the test binary alone.
type Updater interface {
	Target() (string, error)
	Installable(target string) error
	StageDir(target string) (string, error)
	Download(ctx context.Context, c *http.Client, rel update.Release, name, dir string) (string, error)
	Prepare(archive, target string) (string, error)
	Swap(staged, target string) (undo func() error, err error)
}

type realUpdater struct{}

func (realUpdater) Target() (string, error)                { return update.Target() }
func (realUpdater) Installable(target string) error        { return update.Installable(target) }
func (realUpdater) StageDir(target string) (string, error) { return update.StageDir(target) }
func (realUpdater) Download(ctx context.Context, c *http.Client, rel update.Release, name, dir string) (string, error) {
	return update.Download(ctx, c, rel, name, dir)
}
func (realUpdater) Prepare(archive, target string) (string, error) {
	return update.Prepare(archive, target)
}
func (realUpdater) Swap(staged, target string) (func() error, error) {
	return update.Swap(staged, target)
}

func (s *Server) updater() Updater {
	if s.Updater == nil {
		return realUpdater{}
	}
	return s.Updater
}

// refusal is why this copy can't install an update itself, worked out once:
// the folder check and codesign are too slow for every status poll.
func (s *Server) refusal() error {
	s.updOnce.Do(func() {
		if s.Restart == nil || update.AssetName(runtime.GOOS, runtime.GOARCH, "x") == "" {
			s.updRefusal = i18n.Err("log.atualizacao.sem_programa")
			return
		}
		u := s.updater()
		t, err := u.Target()
		if err == nil {
			err = u.Installable(t)
		}
		s.updRefusal = err
	})
	return s.updRefusal
}

// updateStatus is "update" in GET /api/status: the newer release, whether
// this caller can install it from here, and how an install is going.
func (s *Server) updateStatus(here bool) map[string]any {
	rel := s.newerRelease()
	if rel == nil {
		return nil
	}
	s.updMu.Lock()
	state, msg := s.updState, s.updErr
	s.updMu.Unlock()
	return map[string]any{
		"version": rel.Version,
		"url":     rel.URL,
		"install": here && s.refusal() == nil,
		"state":   state,
		"error":   msg,
	}
}

// updateAsk says whether the panel on the PC should ask once whether to
// check for versions.
func (s *Server) updateAsk(here bool) bool {
	set := s.Store.Settings()
	return here && !set.UpdateAsked && !set.UpdateCheck
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
	rel := s.newerRelease()
	if rel == nil {
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.nenhuma"))
		return
	}
	if err := s.refusal(); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if s.Streaming() {
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.no_ar"))
		return
	}
	s.updMu.Lock()
	if s.updBusy {
		s.updMu.Unlock()
		fail(w, http.StatusConflict, i18n.T("api.atualizacao.a_decorrer"))
		return
	}
	s.updBusy, s.updState, s.updErr = true, "downloading", ""
	s.updMu.Unlock()
	s.Log(i18n.T("log.atualizacao.a_transferir", "versao", rel.Version))
	s.once(i18n.T("log.tarefa.atualizar"), func() { s.install(*rel) })
	reply(w, map[string]any{"ok": true, "version": rel.Version})
}

func (s *Server) setUpdate(state string) {
	s.updMu.Lock()
	s.updState = state
	s.updMu.Unlock()
}

// install downloads, checks, swaps and restarts. Until the restart has
// started, any failure, a panic included, puts the old program back.
func (s *Server) install(rel update.Release) {
	u := s.updater()
	var target, stage string
	var undo func() error
	started := false
	var failure error
	defer func() {
		if started {
			return
		}
		if undo != nil {
			if err := undo(); err != nil {
				s.Log(i18n.T("log.atualizacao.nao_repus", "err", err))
			}
		}
		if stage != "" {
			os.RemoveAll(stage)
		}
		if failure == nil {
			failure = i18n.Err("log.atualizacao.interrompida")
		}
		s.Log(i18n.T("log.atualizacao.falhou", "err", failure))
		s.updMu.Lock()
		s.updBusy, s.updState, s.updErr = false, "failed", failure.Error()
		s.updMu.Unlock()
	}()
	var err error
	if target, err = u.Target(); err != nil {
		failure = err
		return
	}
	if stage, err = u.StageDir(target); err != nil {
		failure = err
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	archive, err := u.Download(ctx, http.DefaultClient, rel, update.AssetName(runtime.GOOS, runtime.GOARCH, rel.Version), stage)
	if err != nil {
		failure = err
		return
	}
	s.setUpdate("installing")
	staged, err := u.Prepare(archive, target)
	if err != nil {
		failure = err
		return
	}
	if s.Streaming() {
		failure = i18n.Err("api.atualizacao.no_ar")
		return
	}
	if undo, err = u.Swap(staged, target); err != nil {
		failure = err
		return
	}
	if s.Streaming() {
		failure = i18n.Err("api.atualizacao.no_ar")
		return
	}
	s.setUpdate("restarting")
	if err := s.Restart(); err != nil {
		failure = err
		return
	}
	started = true
}
