package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/provider"
	"github.com/kauredo/hallstream/internal/store"
)

// calendarFresh is how long a fetched calendar is used before it is asked again.
const calendarFresh = 24 * time.Hour

// calendarMax bounds a calendar feed; a team's season is a few KB.
const calendarMax = 4 << 20

var calendarMu sync.Mutex // one fetch at a time; the next caller reads the cache

// calendar is the escalões' calendar links as a provider.
func (s *Server) calendar() provider.Provider {
	return provider.Calendar{
		Fetch: s.calendarFeed,
		Club:  func() store.Club { return s.Store.Settings().Club },
		Zone:  s.zone,
	}
}

// calendarURL checks a calendar link as the panel sends it: http, https or
// webcal, with a host. "" clears it.
func calendarURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "webcal") {
		return "", userError(i18n.T("api.calendario.link"))
	}
	return u.String(), nil
}

// calendarFeed is the feed at raw, from dados/cache/ics when fetched in the last
// day, else from the network; with the network down it falls back to the copy.
func (s *Server) calendarFeed(raw string) ([]byte, error) {
	calendarMu.Lock()
	defer calendarMu.Unlock()
	sum := sha256.Sum256([]byte(raw))
	path := filepath.Join(s.Store.Dir, "cache", "ics", hex.EncodeToString(sum[:8])+".ics")
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < calendarFresh {
		return os.ReadFile(path)
	}
	data, err := fetchCalendar(raw)
	if err != nil {
		if old, rerr := os.ReadFile(path); rerr == nil {
			return old, nil
		}
		s.Log(i18n.T("log.calendario.erro", "err", err))
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		os.WriteFile(path, data, 0o644)
	}
	return data, nil
}

func fetchCalendar(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "webcal://") {
		raw = "https://" + strings.TrimPrefix(raw, "webcal://")
	}
	c := &http.Client{Timeout: 20 * time.Second}
	res, err := c.Get(raw)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, calendarMax+1))
	if err != nil {
		return nil, err
	}
	if len(data) > calendarMax || !strings.Contains(string(data[:min(len(data), 2048)]), "BEGIN:VCALENDAR") {
		return nil, fmt.Errorf("not an iCalendar feed")
	}
	return data, nil
}

// calendarFixtures is GET /api/groups/{id}/calendar: the timed games of the
// escalão's calendar link, shaped like its typed fixtures (id "ics/<uid>"),
// for the panel to list and pick. They can't be edited or deleted here.
func (s *Server) calendarFixtures(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := s.calendar().Fixtures(g)
	if err != nil {
		fail(w, http.StatusBadGateway, i18n.T("api.calendario.sem_ligacao"))
		return
	}
	out := []store.Fixture{}
	for _, f := range list {
		if !f.Timed {
			continue
		}
		out = append(out, store.Fixture{
			ID: f.Ref, Start: f.Start.In(s.zone()).Format(time.RFC3339), Home: f.AtHome,
			Opponent: store.Opponent{Name: f.Opponent.Name}, Venue: f.Venue,
		})
	}
	reply(w, out)
}
