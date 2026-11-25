package web

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Windows has no zone database

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/provider"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// fixtures is what the FPB says about the game of the day and the one after
// it, worked out for the game in key.
type fixtures struct {
	key    string
	atHome *bool
	next   *state.NextGame
	note   string
}

// onAirWindow is how long a game can still be on air after midnight.
const onAirWindow = 3 * time.Hour

// lisbon reads the FPB's times, which are Portugal's. Everything the club
// types or imports is read in its own zone (Server.zone).
var lisbon = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Lisbon"); err == nil {
		return l
	}
	return time.Local
}()

func fixturesKey(g store.Game) string {
	id := ""
	if g.FPBGameID != nil {
		id = strconv.FormatInt(*g.FPBGameID, 10)
	}
	return g.GroupID + "|" + id + "|" + g.Start
}

// WatchNext works out the next game now and then every hour, so the rule
// follows the clock and the FPB.
func (s *Server) WatchNext() {
	for {
		s.RefreshNext()
		time.Sleep(time.Hour)
	}
}

// refreshNextInBackground runs RefreshNext without making the caller wait.
// At most one waits for the running one; it reads the game once it starts.
func (s *Server) refreshNextInBackground() {
	if !s.nextQueued.CompareAndSwap(false, true) {
		return
	}
	s.once(i18n.T("log.tarefa.proximo"), func() {
		s.nextMu.Lock()
		defer s.nextMu.Unlock()
		s.nextQueued.Store(false)
		s.refreshNext()
	})
}

// RefreshNext reads the FPB games of the current game's escalão and keeps the
// next one (see "Próximo jogo" in docs/CONTRACT.md), with its crest saved in
// dados/media.
func (s *Server) RefreshNext() {
	s.nextMu.Lock()
	defer s.nextMu.Unlock()
	s.refreshNext()
}

// refreshNext must be called with nextMu held.
func (s *Server) refreshNext() {
	g := s.Store.Game()
	f := s.findNext(g, time.Now())
	f.key = fixturesKey(g)
	if f.key != fixturesKey(s.Store.Game()) {
		return // the game changed meanwhile; the refresh that change started wins
	}
	s.fixMu.Lock()
	s.fix = f
	s.fixMu.Unlock()
	s.SyncGame()
}

func (s *Server) findNext(g store.Game, now time.Time) fixtures {
	if g.GroupID == "" {
		return fixtures{note: i18n.T("proximo.sem_escalao")}
	}
	grp, err := s.Store.Group(g.GroupID)
	if err != nil {
		return fixtures{note: i18n.T("proximo.sem_escalao")}
	}
	// Every provider's games together; a provider that fails only matters
	// when the others have nothing either.
	var games []provider.Fixture
	note := ""
	for _, p := range s.providers() {
		list, err := p.Fixtures(grp)
		switch {
		case errors.Is(err, fpb.ErrNotFound):
			note = i18n.T("proximo.equipa_desconhecida")
		case errors.Is(err, fpb.ErrBusy):
			note = i18n.T("proximo.fpb_ocupada")
		case err != nil:
			note = i18n.T("proximo.sem_ligacao")
		}
		games = append(games, list...)
	}
	if len(games) == 0 {
		if note == "" {
			note = i18n.T("proximo.sem_jogos")
		}
		return fixtures{note: note}
	}
	start, _ := time.Parse(time.RFC3339, g.Start)
	var ref string
	if g.FPBGameID != nil {
		ref = fmt.Sprintf("fpb/%d", *g.FPBGameID)
	}
	cur, next := pickNext(games, ref, start, now.In(s.zone()))
	var f fixtures
	if cur >= 0 {
		home := games[cur].AtHome
		f.atHome = &home
	}
	if next < 0 {
		f.note = i18n.T("proximo.sem_jogo")
		return f
	}
	f.next = s.nextGame(games[next])
	return f
}

// pickNext finds the game of the day in games (by its ref) and the one after
// it. Without the ref, or when no provider lists it any more, the game on air
// is taken to be on ref's day, ref being the game's start or, when that is
// earlier, now less onAirWindow (a game that ran past midnight); the game of
// the day is then the one that day, and the next game the first on a later
// day. Indexes are -1 when there is none; games without a time are skipped.
func pickNext(games []provider.Fixture, ref string, start, now time.Time) (cur, next int) {
	cur, next = -1, -1
	if ref != "" {
		for i, x := range games {
			if x.Ref == ref {
				cur = i
				break
			}
		}
	}
	if cur >= 0 {
		// The earliest game after it; no provider's order is relied on.
		from, ok := games[cur].Start, games[cur].Timed
		var best time.Time
		for i, x := range games {
			if ok && x.Timed && i != cur && x.Start.After(from) && (next < 0 || x.Start.Before(best)) {
				next, best = i, x.Start
			}
		}
		return cur, next
	}
	at := now.Add(-onAirWindow)
	if start.After(at) {
		at = start
	}
	z := now.Location() // the club's zone: the caller passes now in it
	y, m, d := at.In(z).Date()
	day, dayAfter := time.Date(y, m, d, 0, 0, 0, 0, z), time.Date(y, m, d+1, 0, 0, 0, 0, z)
	var best time.Time
	for i, x := range games {
		switch {
		case !x.Timed:
		case !x.Start.Before(day) && x.Start.Before(dayAfter):
			cur = i
		case !x.Start.Before(dayAfter) && (next < 0 || x.Start.Before(best)):
			next, best = i, x.Start
		}
	}
	return cur, next
}

// fpbStart reads FPB's "2026-11-20" and "21H45" in Lisbon time.
func fpbStart(date, clock string) (time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), lisbon)
	if err != nil {
		return time.Time{}, false
	}
	h, m, found := strings.Cut(strings.ToUpper(strings.TrimSpace(clock)), "H")
	if !found {
		h, m, found = strings.Cut(clock, ":")
	}
	hh, err1 := strconv.Atoi(strings.TrimSpace(h))
	mm, err2 := strconv.Atoi(strings.TrimSpace(m))
	if !found || err1 != nil || err2 != nil || hh > 23 || mm > 59 {
		return time.Time{}, false // no time yet: the game can't be placed
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, lisbon), true
}

var digits = regexp.MustCompile(`^\d{1,3}$`)

func (s *Server) nextGame(x provider.Fixture) *state.NextGame {
	opp := x.Opponent
	n := &state.NextGame{
		AtHome:      x.AtHome,
		FullName:    opp.FullName,
		Venue:       x.Venue,
		Competition: x.Competition,
		Round:       x.Round,
	}
	if x.Timed {
		n.Start = x.Start.Format(time.RFC3339)
	}
	if digits.MatchString(n.Round) {
		n.Round = i18n.T("jogo.jornada", "n", n.Round)
	}
	t := state.Team{Name: opp.Name, Color: fpb.Neutral}
	if r := []rune(opp.Short); len(r) > 0 && len(r) <= 4 {
		t.Short = string(r)
	} else {
		t.Short = cutRunes(strings.ReplaceAll(t.Name, " ", ""), 3)
	}
	t.Initials = t.Short
	if u, data := s.opponentCrest(opp); u != "" {
		t.CrestURL = &u
		t.Color = fpb.PickColor(data)
	}
	if opp.Color != "" {
		t.Color = opp.Color
	}
	t.Color = fpb.EnsureContrast(t.Color)
	n.Opponent = t
	return n
}

// opponentCrest is the opponent's crest in dados/media: a typed fixture's own
// upload, else the copy kept as CrestKey (the name the game of the day uses
// too), else a download of the provider's crest into that name.
func (s *Server) opponentCrest(opp provider.Side) (string, []byte) {
	if strings.HasPrefix(opp.CrestURL, "/media/") {
		if data, err := s.Store.ReadMedia(opp.CrestURL); err == nil {
			return opp.CrestURL, data
		}
		return "", nil
	}
	if opp.CrestKey == "" {
		return "", nil
	}
	for _, ext := range []string{".png", ".jpg", ".svg", ".webp", ".gif"} {
		if data, err := s.Store.ReadMedia("/media/" + opp.CrestKey + ext); err == nil {
			return "/media/" + opp.CrestKey + ext, data
		}
	}
	if opp.CrestURL == "" {
		return "", nil
	}
	data, ext, err := s.FPB.Download(opp.CrestURL)
	if err != nil {
		s.Log(i18n.T("log.proximo.nao_descarreguei_logo", "err", err))
		return "", nil
	}
	u, err := s.Store.SaveMedia(opp.CrestKey+ext, data)
	if err != nil {
		return "", nil
	}
	return u, data
}

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
