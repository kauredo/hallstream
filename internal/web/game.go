package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// newGameState forgets the game that was played: the periods and sets the
// state machine recorded and, for a sets sport, the sets the phone scorer
// still holds (it would send them again and the machine would record them).
// The scorer's new_game cannot fail on a sets or soccer game, so its error is dropped.
func (s *Server) newGameState() {
	s.State.ResetPeriods()
	if s.Manual == nil {
		return
	}
	// A soccer scorer's cards, substitutions, added time and end of game belong to
	// the last game too, whatever sport the next one is.
	r, held := sport.Get(s.Store.GameSport()), s.Manual.Score()
	if r.Sets() || r.Cards || held.Over || held.Stoppage > 0 || held.Cards != nil || held.Subs != nil {
		_ = s.Manual.Do(source.Action{Do: "new_game"})
	}
}

func (s *Server) putGame(w http.ResponseWriter, r *http.Request) {
	var g store.Game
	if !decode(w, r, &g) {
		return
	}
	var grp store.Group
	if g.GroupID != "" {
		var err error
		if grp, err = s.Store.Group(g.GroupID); err != nil {
			fail(w, http.StatusBadRequest, i18n.T("api.escalao_desconhecido"))
			return
		}
	}
	if g.FPBGameID != nil && g.Away.CrestURL == "" && grp.FPBTeamID != 0 {
		s.fillOpponent(&g, grp.FPBTeamID)
	}
	if g.Away.Color == "" {
		g.Away.Color = fpb.Neutral
		if data, err := s.Store.ReadMedia(g.Away.CrestURL); err == nil {
			g.Away.Color = fpb.PickColor(data)
		}
	}
	g.Away.Color = fpb.EnsureContrast(g.Away.Color)
	old := s.Store.Game()
	if s.save(w, s.Store.SaveGame(g)) {
		reply(w, g)
		if old.GroupID != g.GroupID || !sameGame(old.FPBGameID, g.FPBGameID) || day(old.Start) != day(g.Start) {
			s.newGameState() // another game: last game's periods and sets go
		}
		s.refreshNextInBackground()
		// A game picked for a linked escalão: bring its squad up to date.
		if grp.FPBTeamID != 0 && (old.GroupID != g.GroupID || !sameGame(old.FPBGameID, g.FPBGameID)) {
			s.syncInBackground(grp.ID)
		}
	}
}

func sameGame(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// day is the date part of an ISO start, so fixing the tip-off time mid-game
// keeps the periods.
func day(start string) string { return start[:min(len(start), 10)] }

// fillOpponent finds the FPB game, downloads the opponent's logo and picks
// its colour. Without the FPB the game is saved as it came.
func (s *Server) fillOpponent(g *store.Game, teamID int64) {
	next, err := s.FPB.NextGames(teamID)
	if err != nil {
		s.Log(i18n.T("log.jogo.nao_li_fpb", "err", err))
		return
	}
	for _, x := range next.Games {
		if x.ID != *g.FPBGameID {
			continue
		}
		opp := x.Away
		if x.Away.ID == teamID {
			opp = x.Home
		}
		// The panel's FPB picker sends no name, so this is the one rule.
		if g.Away.Name == "" {
			g.Away.Name = opponentName(opp.Name, opp.ShortName)
		}
		if g.Away.Short == "" && len([]rune(opp.ShortName)) <= 4 {
			g.Away.Short = strings.ToUpper(opp.ShortName)
		}
		if g.Venue == "" {
			g.Venue = tidyPlace(x.Location)
		}
		if g.Competition == "" {
			g.Competition = x.CompetitionName
		}
		logo := opp.LogoURL
		if logo == "" {
			logo = opp.ClubLogoURL
		}
		if logo == "" {
			return
		}
		data, ext, err := s.FPB.Download(logo)
		if err != nil {
			s.Log(i18n.T("log.jogo.nao_descarreguei_logo", "err", err))
			return
		}
		u, err := s.Store.SaveMedia(fmt.Sprintf("opp-%d%s", opp.ID, ext), data)
		if err != nil {
			s.Log(i18n.T("log.jogo.nao_guardei_logo", "err", err))
			return
		}
		g.Away.CrestURL = u
		if g.Away.Color == "" {
			g.Away.Color = fpb.PickColor(data)
		}
		return
	}
}

// clubWords are dropped from an opponent's name that doesn't fit, compared
// without accents or case.
var clubWords = map[string]bool{
	"basket": true, "basquetebol": true, "basquete": true, "clube": true, "club": true, "grupo": true,
	"desportivo": true, "associacao": true, "academia": true,
	"de": true, "da": true, "do": true, "dos": true, "das": true, "e": true,
	"cb": true, "cd": true, "ad": true, "gd": true, "sc": true, "fc": true, "ac": true,
}

// nameMax is the longest opponent name. The overlay shrinks long names, and
// the corner scoreboard shows the sigla when the name gets too small there.
// Up to nameShort the full name is kept as it is.
const nameMax, nameShort = 20, 12

// opponentName is the name to show, in capitals: the full name when short,
// else the name without the club words, else the full name, else FPB's short
// name, else the name cut at a space; never over nameMax characters.
func opponentName(name, short string) string {
	up := strings.ToUpper(strings.Join(strings.Fields(name), " "))
	if len([]rune(up)) <= nameShort {
		return up
	}
	var kept []string
	for _, w := range strings.Fields(up) {
		if !clubWords[store.Plain(w)] {
			kept = append(kept, w)
		}
	}
	s := strings.Join(kept, " ")
	if s != "" && len([]rune(s)) <= nameMax {
		return s
	}
	if len([]rune(up)) <= nameMax {
		return up
	}
	if short = strings.ToUpper(strings.TrimSpace(short)); short != "" {
		return string([]rune(short)[:min(len([]rune(short)), nameMax)])
	}
	if s == "" {
		s = up
	}
	r := []rune(s)
	if i := strings.LastIndex(string(r[:nameMax+1]), " "); i > 0 {
		return string(r[:nameMax+1])[:i]
	}
	return string(r[:nameMax])
}

func (s *Server) opponentLogo(w http.ResponseWriter, r *http.Request) {
	data, ext, okf := upload(w, r)
	if !okf {
		return
	}
	// The panel sends crest_url (and color, if it wants it) on its next PUT /api/game.
	u, err := s.Store.SaveMedia("opp-"+store.NewID()+ext, data)
	if s.save(w, err) {
		reply(w, map[string]string{"crest_url": u, "color": fpb.PickColor(data)})
	}
}

// SyncGame rebuilds the game part of the live state from settings, groups
// and the current game.
func (s *Server) SyncGame() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	set := s.Store.Settings()
	g := s.Store.Game()
	cl := set.Club
	crest := cl.Crest
	if crest == "" {
		crest = store.DefaultCrest
	}
	sp, _ := club.SportOf(s.Store.SportOf(g.GroupID))
	out := state.Game{
		Home:        state.Team{Name: cl.Name, Short: cl.Short, CrestURL: &crest, Color: cl.Primary, Initials: cl.Short},
		Sport:       sp.ID,
		SportLabel:  sp.Short,
		Rules:       sport.Get(sp.ID),
		Theme:       club.Theme(cl.Primary, cl.Secondary),
		Away:        state.Team{Name: g.Away.Name, Short: g.Away.Short, Color: fpb.EnsureContrast(g.Away.Color)},
		Start:       g.Start,
		Venue:       g.Venue,
		Competition: g.Competition,
		Round:       g.Round,
		Squad:       []state.SquadPlayer{},
		Instagram:   set.Instagram,
		Photos:      state.Photos{Squad: set.Photos.Squad, LowerThird: set.Photos.LowerThird},
		Overlay:     overlayState(set.Overlay),
		Credit:      set.Credit,
	}
	s.fixMu.Lock()
	if s.fix.key == fixturesKey(g) {
		out.AtHome, out.Next, out.NextNote = s.fix.atHome, s.fix.next, s.fix.note
	} else {
		out.NextNote = i18n.T("proximo.a_ler")
	}
	s.fixMu.Unlock()
	if out.Away.Name == "" {
		out.Away.Name = i18n.T("jogo.visitante")
	}
	if g.Away.CrestURL != "" {
		c := g.Away.CrestURL
		out.Away.CrestURL = &c
	}
	grp, err := s.Store.Group(g.GroupID)
	if err == nil && g.GroupID != "" {
		out.Group = &state.Group{ID: grp.ID, Name: grp.Name, Coach: grp.Coach}
		for _, e := range g.Squad {
			if p := findPlayer(grp.Players, e.PlayerID); p != nil && !p.Inactive {
				n := e.Number
				if n == "" {
					n = p.Number
				}
				out.Squad = append(out.Squad, state.SquadPlayer{Number: n, Name: p.DisplayName, PhotoURL: p.PhotoURL})
			}
		}
	}
	shared, _ := s.Store.Group(store.Shared)
	out.Sponsors = sponsors(grp.Sponsors, shared.Sponsors)
	s.State.SetGame(out, g.Picked())
	s.switchSport(sp.ID)
	s.timelineGame(g, out)
}

// switchSport gives a fresh scorer game the new sport's period length when
// the game's sport changes. lastSport starts empty, so the first sync after a
// start only records the sport, unless no manual.json exists yet: a fresh
// install starts from basketball's length, so it moves to the sport's. A saved
// scorer keeps its length across a restart. An empty id is basketball, as
// sport.Get has it. Called with syncMu held.
func (s *Server) switchSport(id string) {
	id = sport.Get(id).ID
	prev := s.lastSport
	s.lastSport = id
	if s.Manual == nil {
		return
	}
	if prev == "" && !s.Store.HasManual() {
		prev = sport.Get("").ID
	}
	if prev == "" || prev == id {
		return
	}
	if l := sport.Get(id).Length; l > 0 {
		if err := s.Manual.SetLengthFrom(sport.Get(prev), l); err != nil {
			s.Log(i18n.T("log.nao_mudei_duracao", "err", err))
		}
	}
}

// sponsors puts the group's principal first (else the shared one), then the
// rest: the group's own, then the shared ones.
func sponsors(own, shared []store.Sponsor) state.Sponsors {
	out := state.Sponsors{Others: []state.Sponsor{}}
	all := append(append([]store.Sponsor{}, own...), shared...)
	chosen := -1
	for i, sp := range all {
		if sp.Principal {
			chosen = i
			break
		}
	}
	for i, sp := range all {
		v := state.Sponsor{URL: sp.URL, Name: sp.Name}
		if i == chosen {
			out.Principal = &v
		} else {
			out.Others = append(out.Others, v)
		}
	}
	return out
}

// tidyPlace fixes FPB venue text like "Pavilhão X ,Lisboa" to "Pavilhão X, Lisboa".
func tidyPlace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, " ,", ","), ",", ", ")), " ")
}
