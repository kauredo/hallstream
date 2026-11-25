package web

import (
	"context"
	"net/http"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/update"
)

// updateEvery is how often the update check asks GitHub, while it is on.
const updateEvery = 24 * time.Hour

// RunUpdates asks GitHub for a newer release once a day while
// settings.update_check is on, and at once when it is switched on. It never
// downloads: the panel only links to the release page.
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
