package web

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
)

// Limits for one setup or daily update, so a strange FPB answer can't keep
// the program busy.
const (
	maxTeams   = 50
	maxPlayers = 100
	maxRun     = 10 * time.Minute
	// maxPhotoFails is how many photo downloads in a row may get no answer
	// before the rest wait for the next update.
	maxPhotoFails = 3
)

// liveRetry is how long the daily update waits when a game is live.
var liveRetry = 10 * time.Minute

// Daily keeps every linked escalão up to date with the FPB: now, then every
// 24 hours. A run that falls in a live game waits for it to end. Not in demo
// mode.
func (s *Server) Daily(demo bool) {
	if demo {
		return
	}
	for {
		for !s.dailyRun() {
			time.Sleep(liveRetry)
		}
		time.Sleep(24 * time.Hour)
	}
}

// liveWindow is how long after the clock last moved a game still counts as
// live, so timeouts and half-time do not open a gap.
const liveWindow = 30 * time.Minute

// dailyRun updates from the FPB unless a game is live, and says whether it ran.
// The same PC runs OBS and the stream, so a live game is OBS streaming or a
// game clock that ran in the last liveWindow. The console's connection alone
// does not count: it stays up between games.
func (s *Server) dailyRun() bool {
	if s.State.ClockRanWithin(liveWindow) || s.Streaming() {
		return false
	}
	s.SyncAll(time.Hour)
	return true
}

// SyncAll finishes a club setup left pending, looks for a new season in the
// club's team list, then updates every linked escalão not updated within
// maxAge. It does nothing without the FPB, or while the FPB answers 429.
func (s *Server) SyncAll(maxAge time.Duration) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	// Without club_search there is no team list, only the linked escalões.
	if search := s.Store.Settings().ClubSearch; search != "" {
		teams, err := s.FPB.Teams(search)
		if reachable, _ := s.FPB.Status(); err != nil || !reachable || s.FPB.Busy() {
			return
		}
		if s.Store.Settings().ClubSetup == "pending" {
			res, err := s.setupClub()
			if err != nil {
				return
			}
			s.Log(i18n.T("log.clube.feito", "criados", res.Created, "jogadores", res.ImportedPlayers))
		}
		s.newSeason(clubPlan(search, teams))
	}
	deadline := time.Now().Add(maxRun)
	for _, g := range s.Store.Groups() {
		if g.FPBTeamID == 0 || g.NeedsTeam {
			continue
		}
		if t, err := time.Parse(time.RFC3339, g.LastSync); err == nil && time.Since(t) < maxAge {
			continue
		}
		if time.Now().After(deadline) {
			s.Log(i18n.T("log.fpb.demorou"))
			break
		}
		_, _, err := s.syncGroup(g.ID, deadline)
		if errors.Is(err, fpb.ErrBusy) {
			s.Log(i18n.T("log.fpb.ocupada"))
			break
		}
		if err != nil {
			s.Log(i18n.T("log.fpb.nao_atualizei", "escalao", g.Name, "err", err))
		}
	}
	s.SyncGame()
}

// syncInBackground updates one escalão without making the caller wait.
func (s *Server) syncInBackground(gid string) {
	s.once(i18n.T("log.tarefa.fpb"), func() {
		if _, _, err := s.syncGroup(gid, time.Now().Add(maxRun)); err != nil {
			s.Log(i18n.T("log.fpb.nao_atualizei_escalao", "err", err))
		}
		s.SyncGame()
	})
}

var (
	copyNumber = regexp.MustCompile(` \(\d+\)$`)
	slugSeason = regexp.MustCompile(`(\d{4}-\d{4})$`)
)

// season is the newest season in the plan's slugs, "" when none has one.
func season(plan []clubTeamPlan) string {
	out := ""
	for _, t := range plan {
		if m := slugSeason.FindString(t.Slug); m > out {
			out = m
		}
	}
	return out
}

// recordClubTeams saves the plan's teams that ended up linked to an escalão.
func (s *Server) recordClubTeams(plan []clubTeamPlan) {
	linked := map[int64]bool{}
	for _, g := range s.Store.Groups() {
		linked[g.FPBTeamID] = true
	}
	var ids []int64
	for _, t := range plan {
		if linked[t.ID] {
			ids = append(ids, t.ID)
		}
	}
	if err := s.Store.SetClubTeams(ids); err != nil {
		s.Log(i18n.T("log.nao_guardei_definicoes", "err", err))
	}
}

// newSeason runs when the slugs show a season newer than the last one seen,
// so an empty or partial answer for the current season changes nothing. It
// moves each club escalão whose team left the list to the one new team with
// the same escalão name rule, or flags it for the volunteer to pick, and gives
// the new club teams no escalão has one, as on the first run.
func (s *Server) newSeason(plan []clubTeamPlan) {
	if len(plan) == 0 {
		return
	}
	set := s.Store.Settings()
	now := season(plan)
	if now != "" && set.ClubSeason != "" && now > set.ClubSeason {
		s.moveSeason(plan, set.ClubTeams)
	}
	s.recordClubTeams(plan)
	if now > set.ClubSeason {
		if err := s.Store.SetClubSeason(now); err != nil {
			s.Log(i18n.T("log.nao_guardei_definicoes", "err", err))
		}
	}
}

func (s *Server) moveSeason(plan []clubTeamPlan, clubTeams []int64) {
	current := map[int64]bool{}
	for _, t := range plan {
		current[t.ID] = true
	}
	groups := s.Store.Groups()
	linked := map[int64]bool{}
	for _, g := range groups {
		linked[g.FPBTeamID] = true
	}
	offered := map[int64]bool{} // candidates of an escalão left to the volunteer
	for _, g := range groups {
		if g.FPBTeamID == 0 || current[g.FPBTeamID] {
			continue
		}
		// Only escalões linked to a club team; one linked by hand to any
		// other team is left alone.
		key := g.FPBTeamKey
		if key == "" && slices.Contains(clubTeams, g.FPBTeamID) {
			key = copyNumber.ReplaceAllString(g.Name, "")
		}
		if key == "" {
			continue
		}
		var cand []clubTeamPlan
		for _, t := range plan {
			if !linked[t.ID] && store.Plain(escalaoName(t.TeamInfo)) == store.Plain(key) {
				cand = append(cand, t)
			}
		}
		if len(cand) > 1 {
			for _, t := range cand {
				offered[t.ID] = true
			}
			// Two teams for one escalão (Sénior): keep the one with the same club name.
			club, _, _ := strings.Cut(g.FPBTeamName, " · ")
			cand = slices.DeleteFunc(cand, func(t clubTeamPlan) bool { return store.Plain(t.Name) != store.Plain(club) })
		}
		_, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error {
			if len(cand) == 1 {
				linkTeam(g, cand[0].TeamInfo)
				g.LastSync = "" // update now
			} else {
				g.NeedsTeam = true
			}
			return nil
		})
		switch {
		case err != nil:
			s.Log(i18n.T("log.nao_guardei_escalao", "escalao", g.Name, "err", err))
		case len(cand) == 1:
			linked[cand[0].ID] = true
			s.Log(i18n.T("log.fpb.epoca_nova", "escalao", g.Name, "equipa", teamLabel(cand[0].TeamInfo)))
		case !g.NeedsTeam:
			s.Log(i18n.T("log.fpb.epoca_nova_escolha", "escalao", g.Name))
		}
	}
	res := setupResult{}
	deadline := time.Now().Add(maxRun)
	for _, t := range plan {
		if !linked[t.ID] && !offered[t.ID] && time.Now().Before(deadline) {
			s.setupTeam(t, &res, deadline)
		}
	}
	for _, e := range res.Errors {
		s.Log(e)
	}
}

func teamLabel(t fpb.TeamInfo) string {
	return t.Name + " · " + strings.TrimSpace(t.Age+" "+genderLabel(t.Gender))
}

func linkTeam(g *store.Group, t fpb.TeamInfo) {
	g.FPBTeamID, g.FPBTeamName, g.FPBTeamKey, g.NeedsTeam = t.ID, teamLabel(t), escalaoName(t), false
}

var errTeamChanged error = i18n.Key("log.fpb.mudou_equipa")

// syncGroup is "Importar da FPB" with every suggestion accepted, for one
// linked escalão. Returns what changed and how many photos failed.
func (s *Server) syncGroup(gid string, deadline time.Time) (store.SyncChanges, int, error) {
	g, err := s.Store.Group(gid)
	if err != nil || g.FPBTeamID == 0 {
		return store.SyncChanges{}, 0, err
	}
	team := g.FPBTeamID
	roster, err := s.FPB.Roster(team)
	if err == nil && s.FPB.Busy() {
		err = fpb.ErrBusy // the saved copy is no update
	}
	if err != nil {
		return store.SyncChanges{}, 0, err
	}
	entries := rosterEntries(roster)
	var before []store.Player
	var ch store.SyncChanges
	after, err := s.Store.UpdateGroup(gid, func(g *store.Group) error {
		if g.FPBTeamID != team {
			return errTeamChanged // relinked meanwhile: this roster is someone else's
		}
		before = append([]store.Player{}, g.Players...)
		added := acceptRoster(g, entries)
		ch = finishSync(g, entries, before)
		ch.NewPlayers = added
		g.LastChanges = ch
		return nil
	})
	if errors.Is(err, errTeamChanged) {
		return store.SyncChanges{}, 0, nil
	}
	if err != nil {
		return store.SyncChanges{}, 0, err
	}
	failed := s.fetchPhotos(gid, before, after.Players, deadline)
	ch.Photos = s.countPhotos(gid, before)
	return ch, failed, nil
}

// finishSync runs after a roster was merged into g: when the escalão moved to
// another team, linked players missing from the new squad become inactive.
// An empty new squad marks nobody and waits for the next update. It counts
// the changes against before and stamps the sync.
func finishSync(g *store.Group, entries []store.RosterPlayer, before []store.Player) store.SyncChanges {
	moved := g.SyncedTeam != 0 && g.SyncedTeam != g.FPBTeamID
	wait := moved && len(entries) == 0
	in := map[int64]bool{}
	for _, e := range entries {
		in[e.ID] = true
	}
	var ch store.SyncChanges
	for i := range g.Players {
		p := &g.Players[i]
		old := findPlayer(before, p.ID)
		if old == nil {
			continue
		}
		if moved && !wait && p.FPBPlayerID != nil && !in[*p.FPBPlayerID] {
			p.Inactive = true
		}
		switch {
		case old.Inactive && !p.Inactive:
			ch.Returned++
		case !old.Inactive && p.Inactive:
			ch.Left++
		}
		if old.Number != p.Number {
			ch.Numbers++
		}
	}
	if !wait {
		g.SyncedTeam = g.FPBTeamID
	}
	g.LastSync = time.Now().Format(time.RFC3339)
	g.LastChanges = ch
	return ch
}

// countPhotos adds the photos fetchPhotos just saved to the group's last changes.
func (s *Server) countPhotos(gid string, before []store.Player) int {
	n := 0
	g, err := s.Store.UpdateGroup(gid, func(g *store.Group) error {
		n = 0
		for _, p := range g.Players {
			if old := findPlayer(before, p.ID); p.PhotoURL != "" && (old == nil || old.PhotoURL != p.PhotoURL) {
				n++
			}
		}
		g.LastChanges.Photos = n
		return nil
	})
	if err != nil {
		return 0
	}
	return g.LastChanges.Photos
}
