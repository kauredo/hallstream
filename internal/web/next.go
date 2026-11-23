package web

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Windows has no zone database

	"github.com/kauredo/tribuna/internal/fpb"
	"github.com/kauredo/tribuna/internal/state"
	"github.com/kauredo/tribuna/internal/store"
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
	s.once("Próximo jogo", func() {
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
		return fixtures{note: "Escolha primeiro o escalão do jogo."}
	}
	grp, err := s.Store.Group(g.GroupID)
	if err != nil || grp.FPBTeamID == 0 {
		return fixtures{note: "O escalão do jogo não está ligado à FPB."}
	}
	list, err := s.FPB.NextGames(grp.FPBTeamID)
	if errors.Is(err, fpb.ErrNotFound) {
		return fixtures{note: "A FPB não conhece a equipa deste escalão."}
	}
	if errors.Is(err, fpb.ErrBusy) {
		return fixtures{note: "A FPB está a receber demasiados pedidos."}
	}
	if err != nil {
		return fixtures{note: "Sem ligação à FPB."}
	}
	start, _ := time.Parse(time.RFC3339, g.Start)
	cur, next := pickNext(list.Games, g.FPBGameID, start, now)
	var f fixtures
	if cur >= 0 {
		home := list.Games[cur].Home.ID == grp.FPBTeamID
		f.atHome = &home
	}
	if next < 0 {
		f.note = "A FPB não tem outro jogo deste escalão nos próximos 14 dias."
		return f
	}
	f.next = s.nextGame(list.Games[next], grp.FPBTeamID)
	return f
}

// pickNext finds the game of the day in games (by FPB id) and the one after
// it. Without the id, or when the FPB no longer lists it, the game on air is
// taken to be on ref's day, ref being the game's start or, when that is
// earlier, now less onAirWindow (a game that ran past midnight); the next game
// is then the first on a later day. Indexes are -1 when there is none.
func pickNext(games []fpb.NextGame, id *int64, start, now time.Time) (cur, next int) {
	cur, next = -1, -1
	if id != nil {
		for i, x := range games {
			if x.ID == *id {
				cur = i
				break
			}
		}
	}
	if cur >= 0 {
		// The earliest game after it; the FPB's order is not relied on.
		from, ok := fpbStart(games[cur].Date, games[cur].Time)
		var best time.Time
		for i, x := range games {
			if t, tok := fpbStart(x.Date, x.Time); ok && tok && i != cur && t.After(from) && (next < 0 || t.Before(best)) {
				next, best = i, t
			}
		}
		return cur, next
	}
	ref := now.Add(-onAirWindow)
	if start.After(ref) {
		ref = start
	}
	y, m, d := ref.In(lisbon).Date()
	dayAfter := time.Date(y, m, d+1, 0, 0, 0, 0, lisbon)
	for i, x := range games {
		if t, ok := fpbStart(x.Date, x.Time); ok && !t.Before(dayAfter) {
			return cur, i
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

func (s *Server) nextGame(x fpb.NextGame, teamID int64) *state.NextGame {
	opp, home := x.Away, true
	if x.Away.ID == teamID {
		opp, home = x.Home, false
	}
	n := &state.NextGame{
		AtHome:      home,
		FullName:    strings.Join(strings.Fields(opp.Name), " "),
		Venue:       tidyPlace(x.Location),
		Competition: x.CompetitionName,
		Round:       x.PhaseName,
	}
	if t, ok := fpbStart(x.Date, x.Time); ok {
		n.Start = t.Format(time.RFC3339)
	}
	if x.GameWeek != "" {
		n.Round = "Jornada " + string(x.GameWeek)
	}
	t := state.Team{Name: opponentName(opp.Name, opp.ShortName), Color: fpb.Neutral}
	if r := []rune(strings.TrimSpace(opp.ShortName)); len(r) > 0 && len(r) <= 4 {
		t.Short = strings.ToUpper(string(r))
	} else {
		t.Short = cutRunes(strings.ReplaceAll(t.Name, " ", ""), 3)
	}
	t.Initials = t.Short
	if u, data := s.opponentCrest(opp); u != "" {
		t.CrestURL = &u
		t.Color = fpb.PickColor(data)
	}
	t.Color = fpb.EnsureContrast(t.Color)
	n.Opponent = t
	return n
}

// opponentCrest is the opponent's logo in dados/media (opp-<id>, the name the
// game of the day uses too): the copy already there, else a download.
func (s *Server) opponentCrest(opp fpb.Team) (string, []byte) {
	base := fmt.Sprintf("opp-%d", opp.ID)
	for _, ext := range []string{".png", ".jpg", ".svg", ".webp", ".gif"} {
		if data, err := s.Store.ReadMedia("/media/" + base + ext); err == nil {
			return "/media/" + base + ext, data
		}
	}
	logo := opp.LogoURL
	if logo == "" {
		logo = opp.ClubLogoURL
	}
	if logo == "" {
		return "", nil
	}
	data, ext, err := s.FPB.Download(logo)
	if err != nil {
		s.Log("Não consegui descarregar o logótipo do próximo adversário: " + err.Error())
		return "", nil
	}
	u, err := s.Store.SaveMedia(base+ext, data)
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
