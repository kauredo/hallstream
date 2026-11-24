package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
)

// What every club can do without the FPB: import a squad from a CSV, give a
// player a photo, and keep the escalão's fixtures by hand.
func (s *Server) manualRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/groups/{id}/players/csv", s.squadCSV)
	m.HandleFunc("POST /api/groups/{id}/players/{pid}/photo", s.playerPhoto)
	m.HandleFunc("DELETE /api/groups/{id}/players/{pid}/photo", s.removePlayerPhoto)
	m.HandleFunc("GET /api/groups/{id}/fixtures", s.listFixtures)
	m.HandleFunc("POST /api/groups/{id}/fixtures", s.addFixture)
	m.HandleFunc("POST /api/groups/{id}/fixtures/csv", s.fixturesCSV)
	m.HandleFunc("PUT /api/groups/{id}/fixtures/{fid}", s.putFixture)
	m.HandleFunc("DELETE /api/groups/{id}/fixtures/{fid}", s.deleteFixture)
	m.HandleFunc("POST /api/groups/{id}/fixtures/{fid}/crest", s.fixtureCrest)
}

// manualUpload is a POST path that takes multipart/form-data (sameSite).
func manualUpload(p string) bool {
	return strings.HasPrefix(p, "/api/groups/") &&
		(strings.HasSuffix(p, "/players/csv") || strings.HasSuffix(p, "/photo") ||
			strings.HasSuffix(p, "/fixtures/csv") || strings.HasSuffix(p, "/crest"))
}

const maxCSV = 1 << 20

// csvFile is the "file" part of a CSV upload.
func csvFile(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCSV+64<<10)
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, i18n.T("api.falta_ficheiro"))
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCSV+1))
	if err != nil || len(data) > maxCSV {
		fail(w, http.StatusBadRequest, i18n.T("csv.grande"))
		return nil, false
	}
	return data, true
}

// squadCSV adds a number,name,photo CSV to the escalão's players. Photo
// addresses are fetched in the background, as the FPB's are.
func (s *Server) squadCSV(w http.ResponseWriter, r *http.Request) {
	data, ok := csvFile(w, r)
	if !ok {
		return
	}
	rows, bad, err := store.ParseSquad(data)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var before, after []store.Player
	added, updated := 0, 0
	g, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		before = g.Players
		g.Players, added, updated = store.MergeSquad(g.Players, rows)
		after = g.Players
		return nil
	})
	if !s.save(w, err) {
		return
	}
	s.once(i18n.T("log.tarefa.fotos"), func() { s.urlPhotos(g.ID, before, after) })
	if bad == nil {
		bad = []store.LineError{}
	}
	reply(w, map[string]any{"added": added, "updated": updated, "skipped": bad, "players": g.Players})
}

// urlPhotos fetches the photo addresses a CSV gave (any host, unlike the
// FPB's photos) into dados/media.
func (s *Server) urlPhotos(gid string, before, after []store.Player) {
	old := map[string]string{}
	for _, p := range before {
		old[p.ID] = p.PhotoSource
	}
	got := map[string][2]string{}
	for _, p := range after {
		if p.PhotoSource == "" || p.FPBPlayerID != nil || p.PhotoSource == old[p.ID] && p.PhotoURL != "" {
			continue
		}
		data, ext, err := s.FPB.Download(p.PhotoSource)
		if err != nil {
			s.Log(i18n.T("log.foto.nao_descarreguei", "jogador", p.DisplayName, "err", err))
			continue
		}
		sum := sha256.Sum256(data)
		u, err := s.Store.SaveMedia("pl-"+p.ID+"-"+hex.EncodeToString(sum[:4])+ext, data)
		if err == nil {
			got[p.ID] = [2]string{p.PhotoSource, u}
		}
	}
	if len(got) == 0 {
		return
	}
	s.Store.UpdateGroup(gid, func(g *store.Group) error {
		for i := range g.Players {
			if ph, ok := got[g.Players[i].ID]; ok && g.Players[i].PhotoSource == ph[0] {
				g.Players[i].PhotoURL = ph[1]
			}
		}
		return nil
	})
	s.SyncGame()
}

// playerPhoto sets a player's photo from an upload. A photo given by hand
// stays until removed: the FPB sync and a CSV only fill an empty one.
func (s *Server) playerPhoto(w http.ResponseWriter, r *http.Request) {
	data, ext, ok := upload(w, r)
	if !ok {
		return
	}
	pid := r.PathValue("pid")
	sum := sha256.Sum256(data)
	u, err := s.Store.SaveMedia("pl-"+pid+"-"+hex.EncodeToString(sum[:4])+ext, data)
	if !s.save(w, err) {
		return
	}
	s.setPlayerPhoto(w, r.PathValue("id"), pid, u)
}

func (s *Server) removePlayerPhoto(w http.ResponseWriter, r *http.Request) {
	s.setPlayerPhoto(w, r.PathValue("id"), r.PathValue("pid"), "")
}

func (s *Server) setPlayerPhoto(w http.ResponseWriter, gid, pid, u string) {
	var old string
	var p store.Player
	_, err := s.Store.UpdateGroup(gid, func(g *store.Group) error {
		i := slices.IndexFunc(g.Players, func(p store.Player) bool { return p.ID == pid })
		if i < 0 {
			return store.ErrNotFound
		}
		old = g.Players[i].PhotoURL
		g.Players[i].PhotoURL, g.Players[i].PhotoSource, g.Players[i].PhotoByHand = u, "", u != ""
		p = g.Players[i]
		return nil
	})
	if !s.save(w, err) {
		s.Store.RemoveMedia(u)
		return
	}
	if old != u {
		s.Store.RemoveMedia(old)
	}
	reply(w, p)
}

// sortFixtures keeps an escalão's games in date order.
func sortFixtures(fs []store.Fixture) {
	slices.SortStableFunc(fs, func(a, b store.Fixture) int { return strings.Compare(start(a), start(b)) })
}

func start(f store.Fixture) string {
	t, err := time.Parse(time.RFC3339, f.Start)
	if err != nil {
		return f.Start
	}
	return t.UTC().Format(time.RFC3339)
}

func (s *Server) listFixtures(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if !s.save(w, err) {
		return
	}
	if g.Fixtures == nil {
		g.Fixtures = []store.Fixture{}
	}
	reply(w, g.Fixtures)
}

func (s *Server) addFixture(w http.ResponseWriter, r *http.Request) {
	var f store.Fixture
	if !decode(w, r, &f) {
		return
	}
	if err := store.ValidFixture(&f); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	f.ID, f.Opponent.CrestURL = store.NewID(), ""
	f.Opponent.Color = cleanColor(f.Opponent.Color)
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		g.Fixtures = append(g.Fixtures, f)
		sortFixtures(g.Fixtures)
		return nil
	})
	if s.save(w, err) {
		s.nextChanged()
		replyCode(w, http.StatusCreated, f)
	}
}

func (s *Server) putFixture(w http.ResponseWriter, r *http.Request) {
	var f store.Fixture
	if !decode(w, r, &f) {
		return
	}
	if err := store.ValidFixture(&f); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	f.ID = r.PathValue("fid")
	f.Opponent.Color = cleanColor(f.Opponent.Color)
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		i := slices.IndexFunc(g.Fixtures, func(x store.Fixture) bool { return x.ID == f.ID })
		if i < 0 {
			return store.ErrNotFound
		}
		f.Opponent.CrestURL = g.Fixtures[i].Opponent.CrestURL // /crest owns it
		g.Fixtures[i] = f
		sortFixtures(g.Fixtures)
		return nil
	})
	if s.save(w, err) {
		s.nextChanged()
		reply(w, f)
	}
}

func (s *Server) deleteFixture(w http.ResponseWriter, r *http.Request) {
	var crest string
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		i := slices.IndexFunc(g.Fixtures, func(x store.Fixture) bool { return x.ID == r.PathValue("fid") })
		if i < 0 {
			return store.ErrNotFound
		}
		crest = g.Fixtures[i].Opponent.CrestURL
		g.Fixtures = slices.Delete(g.Fixtures, i, i+1)
		return nil
	})
	if s.save(w, err) {
		s.removeUnusedCrest(crest)
		s.nextChanged()
		ok(w)
	}
}

// removeUnusedCrest deletes a fixture's crest upload once no fixture and
// not the game of the day use it.
func (s *Server) removeUnusedCrest(u string) {
	if u == "" || s.Store.Game().Away.CrestURL == u {
		return
	}
	for _, g := range s.Store.Groups() {
		for _, f := range g.Fixtures {
			if f.Opponent.CrestURL == u {
				return
			}
		}
	}
	s.Store.RemoveMedia(u)
}

// fixtureCrest is the opponent's crest for a typed fixture; its colour comes
// from the crest unless the club typed one.
func (s *Server) fixtureCrest(w http.ResponseWriter, r *http.Request) {
	data, ext, ok := upload(w, r)
	if !ok {
		return
	}
	u, err := s.Store.SaveMedia("opp-"+store.NewID()+ext, data)
	if !s.save(w, err) {
		return
	}
	var f store.Fixture
	var old string
	_, err = s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		i := slices.IndexFunc(g.Fixtures, func(x store.Fixture) bool { return x.ID == r.PathValue("fid") })
		if i < 0 {
			return store.ErrNotFound
		}
		old = g.Fixtures[i].Opponent.CrestURL
		g.Fixtures[i].Opponent.CrestURL = u
		if g.Fixtures[i].Opponent.Color == "" {
			g.Fixtures[i].Opponent.Color = fpb.PickColor(data)
		}
		f = g.Fixtures[i]
		return nil
	})
	if !s.save(w, err) {
		s.Store.RemoveMedia(u)
		return
	}
	s.removeUnusedCrest(old)
	s.nextChanged()
	reply(w, f)
}

// fixturesCSV adds data,hora,adversario,casa_fora,local,competicao,jornada lines.
func (s *Server) fixturesCSV(w http.ResponseWriter, r *http.Request) {
	data, ok := csvFile(w, r)
	if !ok {
		return
	}
	rows, bad, err := store.ParseFixtures(data, lisbon)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	g, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		for _, row := range rows {
			g.Fixtures = append(g.Fixtures, row.Fixture)
		}
		sortFixtures(g.Fixtures)
		return nil
	})
	if !s.save(w, err) {
		return
	}
	s.nextChanged()
	if bad == nil {
		bad = []store.LineError{}
	}
	reply(w, map[string]any{"added": len(rows), "skipped": bad, "fixtures": g.Fixtures})
}

// cleanColor keeps a typed "#RRGGBB" (or #rgb), else "" (the crest's colour).
func cleanColor(c string) string {
	if h, ok := club.Hex(c); ok {
		return h
	}
	return ""
}

// nextChanged reads the next game again after the club's fixtures changed.
func (s *Server) nextChanged() { s.refreshNextInBackground() }
