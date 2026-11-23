package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/store"
)

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	out := []store.Group{}
	for _, g := range s.Store.Groups() {
		if g.ID != store.Shared {
			out = append(out, g)
		}
	}
	reply(w, out)
}

func (s *Server) addGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		fail(w, http.StatusBadRequest, "Falta o nome do escalão.")
		return
	}
	g, err := s.Store.AddGroup(strings.TrimSpace(req.Name))
	if s.save(w, err) {
		replyCode(w, http.StatusCreated, g)
	}
}

func (s *Server) putGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Coach       *string `json:"coach"`
		FPBTeamID   *int64  `json:"fpb_team_id"`
		FPBTeamName *string `json:"fpb_team_name"`
		// FPBTeam is the team picked, so the escalão can follow it into next season.
		FPBTeam *fpb.TeamInfo `json:"fpb_team"`
	}
	if !decode(w, r, &req) {
		return
	}
	search := s.Store.Settings().ClubSearch // not inside UpdateGroup: same lock
	g, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		if req.FPBTeamID != nil && *req.FPBTeamID != g.FPBTeamID {
			g.FPBTeamKey, g.NeedsTeam = "", false
		}
		// Only a club team can follow the escalão into next season.
		if req.FPBTeam != nil && req.FPBTeamID != nil && req.FPBTeam.ID == *req.FPBTeamID && clubTeam(search, req.FPBTeam.Name) {
			g.FPBTeamKey = escalaoName(*req.FPBTeam)
		}
		if req.Name != nil {
			g.Name = strings.TrimSpace(*req.Name)
		}
		if req.Coach != nil {
			g.Coach = strings.TrimSpace(*req.Coach)
		}
		if req.FPBTeamID != nil {
			g.FPBTeamID = *req.FPBTeamID
		}
		if req.FPBTeamName != nil {
			g.FPBTeamName = strings.TrimSpace(*req.FPBTeamName)
		}
		return nil
	})
	if s.save(w, err) {
		reply(w, g)
		if req.FPBTeamID != nil && g.ID == s.Store.Game().GroupID {
			s.refreshNextInBackground()
		}
	}
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if s.save(w, s.Store.DeleteGroup(r.PathValue("id"))) {
		ok(w)
	}
}

// Players.

func findPlayer(players []store.Player, id string) *store.Player {
	for i := range players {
		if players[i].ID == id {
			return &players[i]
		}
	}
	return nil
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func (s *Server) listPlayers(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if s.save(w, err) {
		reply(w, nonNil(g.Players))
	}
}

// editPlayer runs fn on one player of the group in the URL and returns the
// player before and after. On failure it has already replied.
func (s *Server) editPlayer(w http.ResponseWriter, r *http.Request, fn func(g *store.Group, p *store.Player) error) (before, after store.Player, ok bool) {
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		p := findPlayer(g.Players, r.PathValue("pid"))
		if p == nil {
			return store.ErrNotFound
		}
		before = *p
		if err := fn(g, p); err != nil {
			return err
		}
		after = *p
		return nil
	})
	return before, after, s.save(w, err)
}

// fetchPhotos downloads the FPB photo of every player whose photo address
// changed, or who still has no local copy (a failed download retries), and
// returns how many are still missing. It stops at the deadline, a download
// under way included, and once maxPhotoFails downloads in a row got no
// answer: each of those can wait the whole timeout. It runs outside the store
// lock.
func (s *Server) fetchPhotos(gid string, before, after []store.Player, deadline time.Time) int {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	old := map[string]string{}
	for _, p := range before {
		old[p.ID] = p.PhotoSource
	}
	type photo struct{ src, url string }
	got := map[string]photo{}
	failed, skipped, silent := 0, 0, 0
	for _, p := range after {
		if p.PhotoSource == "" || p.PhotoSource == old[p.ID] && p.PhotoURL != "" {
			continue
		}
		if silent >= maxPhotoFails || ctx.Err() != nil {
			skipped++
			continue
		}
		data, ext, err := s.FPB.DownloadPhoto(ctx, p.PhotoSource)
		// Only a host that could not be reached or took too long counts. A
		// missing photo or a refused redirect comes back at once.
		var slow net.Error
		var dial *net.OpError
		if errors.As(err, &slow) && slow.Timeout() || errors.As(err, &dial) && dial.Op == "dial" {
			silent++
		} else {
			silent = 0
		}
		u := ""
		if err == nil {
			// The content hash in the name makes OBS load a replaced photo.
			sum := sha256.Sum256(data)
			u, err = s.Store.SaveMedia("pl-"+p.ID+"-"+hex.EncodeToString(sum[:4])+ext, data)
		}
		if err != nil {
			s.Log("Não consegui descarregar a fotografia de " + p.DisplayName + ": " + err.Error())
			failed++
			continue
		}
		got[p.ID] = photo{p.PhotoSource, u}
	}
	if skipped > 0 {
		s.Log("As fotografias da FPB estão a demorar demasiado: " + strconv.Itoa(skipped) + " ficam para a próxima atualização.")
		failed += skipped
	}
	if len(got) == 0 {
		return failed
	}
	var stale []string
	_, err := s.Store.UpdateGroup(gid, func(g *store.Group) error {
		stale = nil
		for i := range g.Players {
			p := &g.Players[i]
			// A player relinked while downloading keeps waiting for the new photo.
			if ph, ok := got[p.ID]; ok && p.PhotoSource == ph.src && p.PhotoURL != ph.url {
				if p.PhotoURL != "" {
					stale = append(stale, p.PhotoURL)
				}
				p.PhotoURL = ph.url
			}
		}
		return nil
	})
	if err != nil {
		s.Log("Não consegui guardar as fotografias: " + err.Error())
		failed += len(got)
	}
	// Remove every file no player points at: replaced copies, and downloads
	// not applied (player deleted or relinked meanwhile, or the save failed).
	inUse := map[string]bool{}
	if g, err := s.Store.Group(gid); err == nil {
		for _, p := range g.Players {
			inUse[p.PhotoURL] = true
		}
	}
	for _, ph := range got {
		stale = append(stale, ph.url)
	}
	for _, u := range stale {
		if !inUse[u] {
			s.Store.RemoveMedia(u)
		}
	}
	return failed
}

// playerNow re-reads a player after fetchPhotos, falling back to p.
func (s *Server) playerNow(gid string, p store.Player) store.Player {
	if g, err := s.Store.Group(gid); err == nil {
		if q := findPlayer(g.Players, p.ID); q != nil {
			return *q
		}
	}
	return p
}

type playerReq struct {
	DisplayName *string `json:"display_name"`
	Number      *string `json:"number"`
	Inactive    *bool   `json:"inactive"`
}

func (s *Server) addPlayer(w http.ResponseWriter, r *http.Request) {
	var req playerReq
	if !decode(w, r, &req) {
		return
	}
	p := store.Player{ID: store.NewID()}
	if req.DisplayName != nil {
		p.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Number != nil {
		p.Number = strings.TrimSpace(*req.Number)
	}
	if p.DisplayName == "" {
		fail(w, http.StatusBadRequest, "Falta o nome do jogador.")
		return
	}
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		g.Players = append(g.Players, p)
		return nil
	})
	if s.save(w, err) {
		replyCode(w, http.StatusCreated, p)
	}
}

func (s *Server) putPlayer(w http.ResponseWriter, r *http.Request) {
	var req playerReq
	if !decode(w, r, &req) {
		return
	}
	_, p, okp := s.editPlayer(w, r, func(_ *store.Group, p *store.Player) error {
		if req.DisplayName != nil {
			if strings.TrimSpace(*req.DisplayName) == "" {
				return userError("O nome não pode ficar vazio.")
			}
			p.DisplayName = strings.TrimSpace(*req.DisplayName)
		}
		if req.Number != nil && strings.TrimSpace(*req.Number) != p.Number {
			p.Number = strings.TrimSpace(*req.Number)
			p.NumberFromFPB = false // set by hand now
		}
		if req.Inactive != nil {
			p.Inactive = *req.Inactive
		}
		return nil
	})
	if okp {
		reply(w, p)
	}
}

func (s *Server) deletePlayer(w http.ResponseWriter, r *http.Request) {
	var photo string
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		for i := range g.Players {
			if g.Players[i].ID == r.PathValue("pid") {
				photo = g.Players[i].PhotoURL
				g.Players = append(g.Players[:i], g.Players[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	})
	if s.save(w, err) {
		s.Store.RemoveMedia(photo)
		ok(w)
	}
}

func (s *Server) importPlayers(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if !s.save(w, err) {
		return
	}
	if g.FPBTeamID == 0 {
		fail(w, http.StatusBadRequest, "Este escalão ainda não está ligado a uma equipa da FPB.")
		return
	}
	roster, err := s.FPB.Roster(g.FPBTeamID)
	if err != nil {
		fpbFail(w, err)
		return
	}
	entries := rosterEntries(roster)
	var before []store.Player
	var updated []store.Player
	var sugg []store.Suggestion
	after, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error {
		before = append([]store.Player{}, g.Players...)
		updated, sugg = store.Import(g.Players, entries)
		finishSync(g, entries, before)
		return nil
	})
	if !s.save(w, err) {
		return
	}
	failed := s.fetchPhotos(g.ID, before, after.Players, time.Now().Add(maxRun))
	s.countPhotos(g.ID, before)
	for i := range updated {
		updated[i] = s.playerNow(g.ID, updated[i])
	}
	reply(w, map[string]any{"updated": nonNil(updated), "suggestions": nonNil(sugg), "photo_errors": failed})
}

func rosterEntries(roster fpb.Roster) []store.RosterPlayer {
	var entries []store.RosterPlayer
	for _, p := range roster.Players[:min(len(roster.Players), maxPlayers)] {
		entries = append(entries, store.RosterPlayer{ID: p.PlayerID, AthleteID: p.FPBAthleteID, Name: p.Name, Number: string(p.JerseyNumber), Games: p.Games, PhotoURL: fpb.RealPhoto(p.PhotoURL)})
	}
	return entries
}

type linkReq struct {
	FPBPlayerID  int64  `json:"fpb_player_id"`
	FPBAthleteID int64  `json:"fpb_athlete_id"`
	FPBName      string `json:"fpb_name"`
	Name         string `json:"name"`
	Number       string `json:"number"`
	PhotoURL     string `json:"photo_url"`
}

// linkedElsewhere fails when another local player already has this FPB id.
func linkedElsewhere(g *store.Group, fpbID int64, except string) error {
	for _, p := range g.Players {
		if p.ID != except && p.FPBPlayerID != nil && *p.FPBPlayerID == fpbID {
			return userError("Esse jogador da FPB já está ligado a " + p.DisplayName + ".")
		}
	}
	return nil
}

func (s *Server) linkPlayer(w http.ResponseWriter, r *http.Request) {
	var req linkReq
	if !decode(w, r, &req) {
		return
	}
	if req.FPBPlayerID == 0 {
		fail(w, http.StatusBadRequest, "Falta o jogador da FPB.")
		return
	}
	before, p, okp := s.editPlayer(w, r, func(g *store.Group, p *store.Player) error {
		if err := linkedElsewhere(g, req.FPBPlayerID, p.ID); err != nil {
			return err
		}
		id, name, src := req.FPBPlayerID, req.FPBName, fpb.RealPhoto(req.PhotoURL)
		if p.FPBPlayerID == nil || *p.FPBPlayerID != id || p.PhotoSource != src {
			p.PhotoURL = "" // another athlete or photo: drop the old copy
		}
		p.FPBPlayerID, p.FPBName, p.PhotoSource = &id, &name, src
		if req.FPBAthleteID != 0 {
			p.FPBAthleteID = req.FPBAthleteID
		}
		return nil
	})
	if okp {
		if p.PhotoURL == "" {
			s.Store.RemoveMedia(before.PhotoURL)
		}
		s.fetchPhotos(r.PathValue("id"), []store.Player{before}, []store.Player{p}, time.Now().Add(maxRun))
		reply(w, s.playerNow(r.PathValue("id"), p))
	}
}

func (s *Server) unlinkPlayer(w http.ResponseWriter, r *http.Request) {
	before, p, okp := s.editPlayer(w, r, func(_ *store.Group, p *store.Player) error {
		p.FPBPlayerID, p.FPBName, p.NumberFromFPB = nil, nil, false
		p.PhotoURL, p.PhotoSource = "", ""
		return nil
	})
	if okp {
		s.Store.RemoveMedia(before.PhotoURL)
		reply(w, p)
	}
}

func (s *Server) playerFromFPB(w http.ResponseWriter, r *http.Request) {
	var req linkReq
	if !decode(w, r, &req) {
		return
	}
	if req.FPBPlayerID == 0 || strings.TrimSpace(req.Name) == "" {
		fail(w, http.StatusBadRequest, "Falta o jogador da FPB.")
		return
	}
	id, name := req.FPBPlayerID, strings.TrimSpace(req.Name)
	p := store.Player{ID: store.NewID(), DisplayName: name, Number: strings.TrimSpace(req.Number),
		FPBPlayerID: &id, FPBName: &name, PhotoSource: fpb.RealPhoto(req.PhotoURL), FPBAthleteID: req.FPBAthleteID}
	p.NumberFromFPB = p.Number != ""
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		if err := linkedElsewhere(g, id, ""); err != nil {
			return err
		}
		g.Players = append(g.Players, p)
		return nil
	})
	if s.save(w, err) {
		s.fetchPhotos(r.PathValue("id"), nil, []store.Player{p}, time.Now().Add(maxRun))
		replyCode(w, http.StatusCreated, s.playerNow(r.PathValue("id"), p))
	}
}

// Sponsors.

func (s *Server) listSponsors(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Group(r.PathValue("id"))
	if s.save(w, err) {
		reply(w, nonNil(g.Sponsors))
	}
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".svg": true, ".webp": true, ".gif": true}

// upload reads the "file" field of a multipart form (max 10 MB).
func upload(w http.ResponseWriter, r *http.Request) (data []byte, ext string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	f, h, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "Falta o ficheiro (até 10 MB).")
		return nil, "", false
	}
	defer f.Close()
	data, err = io.ReadAll(f)
	if err != nil {
		fail(w, http.StatusBadRequest, "Não consegui ler o ficheiro.")
		return nil, "", false
	}
	ext = strings.ToLower(filepath.Ext(h.Filename))
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	if !imageExts[ext] {
		ct := h.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "image/") {
			fail(w, http.StatusBadRequest, "O ficheiro tem de ser uma imagem (PNG, JPG ou SVG).")
			return nil, "", false
		}
		ext = fpb.ImageExt(ct, "")
	}
	return data, ext, true
}

func (s *Server) addSponsor(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Store.Group(r.PathValue("id")); !s.save(w, err) {
		return
	}
	data, ext, okf := upload(w, r)
	if !okf {
		return
	}
	sp := store.Sponsor{ID: store.NewID(), Name: strings.TrimSpace(r.FormValue("name"))}
	url, err := s.Store.SaveMedia("sp-"+sp.ID+ext, data)
	if !s.save(w, err) {
		return
	}
	sp.URL = url
	_, err = s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		g.Sponsors = append(g.Sponsors, sp)
		return nil
	})
	if s.save(w, err) {
		replyCode(w, http.StatusCreated, sp)
	}
}

func (s *Server) orderSponsors(w http.ResponseWriter, r *http.Request) {
	var req []struct {
		ID        string `json:"id"`
		Principal bool   `json:"principal"`
	}
	if !decode(w, r, &req) {
		return
	}
	g, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		if len(req) != len(g.Sponsors) {
			return userError("A lista tem de ter todos os patrocinadores.")
		}
		byID := map[string]store.Sponsor{}
		for _, sp := range g.Sponsors {
			byID[sp.ID] = sp
		}
		out := make([]store.Sponsor, 0, len(req))
		principal := false
		for _, e := range req {
			sp, found := byID[e.ID]
			if !found {
				return userError("Patrocinador desconhecido: " + e.ID)
			}
			delete(byID, e.ID)
			sp.Principal = e.Principal && !principal // only one principal
			principal = principal || sp.Principal
			out = append(out, sp)
		}
		g.Sponsors = out
		return nil
	})
	if s.save(w, err) {
		reply(w, nonNil(g.Sponsors))
	}
}

func (s *Server) deleteSponsor(w http.ResponseWriter, r *http.Request) {
	var url string
	_, err := s.Store.UpdateGroup(r.PathValue("id"), func(g *store.Group) error {
		for i, sp := range g.Sponsors {
			if sp.ID == r.PathValue("sid") {
				url = sp.URL
				g.Sponsors = append(g.Sponsors[:i], g.Sponsors[i+1:]...)
				return nil
			}
		}
		return store.ErrNotFound
	})
	if s.save(w, err) {
		s.Store.RemoveMedia(url)
		ok(w)
	}
}

// FPB proxy.

func teamID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "Equipa inválida.")
		return 0, false
	}
	return id, true
}

func fpbFail(w http.ResponseWriter, err error) {
	if errors.Is(err, fpb.ErrNotFound) {
		fail(w, http.StatusNotFound, "A FPB não tem esta equipa, ou o calendário ainda não tem esta função.")
		return
	}
	if errors.Is(err, fpb.ErrBusy) {
		fail(w, http.StatusTooManyRequests, "A FPB está a receber demasiados pedidos. Tente outra vez daqui a um minuto.")
		return
	}
	fail(w, http.StatusBadGateway, "Não consegui falar com a FPB: "+err.Error())
}

func (s *Server) proxy(w http.ResponseWriter, path string) {
	b, err := s.FPB.Get(path)
	if err != nil {
		fpbFail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
