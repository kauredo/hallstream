package web

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kauredo/tribuna/internal/fpb"
	"github.com/kauredo/tribuna/internal/store"
)

// clubTeam reports whether an FPB team name is one of the club's: its plain
// form starts with the plain form of settings.club_search. An empty search
// matches nothing.
func clubTeam(search, name string) bool {
	q := store.Plain(strings.TrimSpace(search))
	return q != "" && strings.HasPrefix(store.Plain(strings.TrimSpace(name)), q)
}

type setupProgress struct {
	Running bool `json:"running"`
	Done    int  `json:"done"`
	Total   int  `json:"total"`
}

func (s *Server) setProgress(p setupProgress) {
	s.progMu.Lock()
	s.prog = p
	s.progMu.Unlock()
}

func (s *Server) progress() setupProgress {
	s.progMu.Lock()
	defer s.progMu.Unlock()
	return s.prog
}

type setupResult struct {
	Created         int      `json:"created"`
	ImportedPlayers int      `json:"imported_players"`
	Errors          []string `json:"errors"`
}

var (
	errSetupRunning = errors.New("as equipas do clube já estão a ser importadas")
	errNoClubSearch = errors.New("falta o nome do clube na FPB")
)

type clubTeamPlan struct {
	fpb.TeamInfo
	name string // the escalão name
}

func genderLabel(g string) string {
	switch strings.ToLower(g + " ")[0] {
	case 'm':
		return "Masculino"
	case 'f':
		return "Feminino"
	}
	return g
}

var subAge = regexp.MustCompile(`(?i)\bsub[- ]?(\d+)`)

// escalaoName is the team's age and gender, with the Sub-N age of its squad
// label when that differs ("Sub-21 M" on a Sénior team) and " B" for a B team.
func escalaoName(t fpb.TeamInfo) string {
	age := strings.TrimSpace(t.Age)
	if m := subAge.FindStringSubmatch(t.SquadLabel); m != nil && store.Plain(age) != "sub "+m[1] {
		age = "Sub " + m[1]
	}
	name := strings.TrimSpace(age + " " + genderLabel(strings.TrimSpace(t.Gender)))
	if n := strings.TrimSpace(t.Name); strings.HasSuffix(n, "-B") || strings.HasSuffix(n, " B") {
		name += " B"
	}
	return name
}

// ageRank orders Sénior, Sub 21 … Sub 14, Mini 12 … Mini 8, then the rest.
func ageRank(name string) int {
	f := strings.Fields(store.Plain(name))
	n := 0
	if len(f) > 1 {
		n, _ = strconv.Atoi(f[1])
	}
	switch {
	case len(f) > 0 && f[0] == "senior":
		return 0
	case len(f) > 1 && f[0] == "sub":
		return 100 - n
	case len(f) > 1 && f[0] == "mini":
		return 200 - n
	}
	return 300
}

func genderRank(g string) int {
	switch genderLabel(g) {
	case "Masculino":
		return 0
	case "Feminino":
		return 1
	}
	return 2
}

// clubPlan picks the club's teams, names them and puts them in escalão order:
// teams with fixtures first, then by age and gender. Of two teams with the
// same name the first keeps it and the other gets " (2)" and goes last.
func clubPlan(search string, teams []fpb.TeamInfo) []clubTeamPlan {
	var all []clubTeamPlan
	for _, t := range teams {
		if clubTeam(search, t.Name) {
			all = append(all, clubTeamPlan{t, escalaoName(t)})
		}
	}
	slices.SortStableFunc(all, func(a, b clubTeamPlan) int {
		fa, fb := 1, 1
		if a.HasFixtures {
			fa = 0
		}
		if b.HasFixtures {
			fb = 0
		}
		return cmp.Or(cmp.Compare(fa, fb), cmp.Compare(ageRank(a.name), ageRank(b.name)),
			cmp.Compare(genderRank(a.Gender), genderRank(b.Gender)), strings.Compare(a.name, b.name), cmp.Compare(a.ID, b.ID))
	})
	if len(all) > maxTeams {
		all = all[:maxTeams]
	}
	var out, dup []clubTeamPlan
	seen := map[string]int{}
	for _, t := range all {
		k := store.Plain(t.name)
		if seen[k]++; seen[k] > 1 {
			t.name += fmt.Sprintf(" (%d)", seen[k])
			dup = append(dup, t)
		} else {
			out = append(out, t)
		}
	}
	return append(out, dup...)
}

// FirstRun sets the club up in a fresh dados folder (no escalões yet) and
// retries on every start until the FPB has answered once. Not in demo mode,
// and not before settings.club_search is filled in.
func (s *Server) FirstRun(demo bool) {
	if demo || s.Store.Settings().ClubSearch == "" {
		return
	}
	switch s.Store.Settings().ClubSetup {
	case "":
		for _, g := range s.Store.Groups() {
			if g.ID != store.Shared {
				return
			}
		}
		if err := s.Store.SetClubSetup("pending"); err != nil {
			s.Log("Não consegui guardar as definições: " + err.Error())
			return
		}
	case "pending":
	default:
		return
	}
	s.Log("A criar os escalões do clube com as equipas da FPB…")
	res, err := s.SetupClub()
	s.SyncGame()
	if err != nil {
		s.Log("Não consegui falar com a FPB (" + err.Error() + "). Tento outra vez quando voltar a abrir o programa, ou carregue em \"Importar equipas do clube\" no Plantel.")
		return
	}
	s.Log(fmt.Sprintf("Escalões do clube: %d criados, %d jogadores importados.", res.Created, res.ImportedPlayers))
	for _, e := range res.Errors {
		s.Log(e)
	}
}

// SetupClub creates an escalão for every club team on the FPB, linked, with
// all its players and photos. Teams already linked are skipped and names
// already there never change. club_setup is marked done once the FPB has
// answered.
func (s *Server) SetupClub() (setupResult, error) {
	if !s.setupMu.TryLock() {
		return setupResult{}, errSetupRunning
	}
	defer s.setupMu.Unlock()
	return s.setupClub()
}

// setupClub is SetupClub for a caller holding setupMu.
func (s *Server) setupClub() (setupResult, error) {
	s.setProgress(setupProgress{Running: true})
	defer s.setProgress(setupProgress{})

	res := setupResult{Errors: []string{}}
	search := s.Store.Settings().ClubSearch
	if search == "" {
		return res, errNoClubSearch
	}
	teams, err := s.FPB.Teams(search)
	if err != nil {
		return res, err
	}
	plan := clubPlan(search, teams)
	deadline := time.Now().Add(maxRun)
	finished := true
	for i, t := range plan {
		if time.Now().After(deadline) {
			res.Errors = append(res.Errors, "Parei a meio porque a FPB estava a demorar demasiado. Carregue outra vez para continuar.")
			finished = false
			break
		}
		s.setProgress(setupProgress{Running: true, Done: i, Total: len(plan)})
		s.setupTeam(t, &res, deadline)
	}
	s.recordClubTeams(plan)
	if now := season(plan); now > s.Store.Settings().ClubSeason {
		if err := s.Store.SetClubSeason(now); err != nil {
			res.Errors = append(res.Errors, "Não consegui guardar as definições: "+err.Error())
		}
	}
	if finished {
		if err := s.Store.SetClubSetup(time.Now().Format(time.RFC3339)); err != nil {
			res.Errors = append(res.Errors, "Não consegui guardar as definições: "+err.Error())
		}
	}
	return res, nil
}

func (s *Server) setupTeam(t clubTeamPlan, res *setupResult, deadline time.Time) {
	groups := s.Store.Groups()
	for _, g := range groups {
		if g.FPBTeamID == t.ID {
			return
		}
	}
	var g store.Group
	for _, x := range groups {
		if x.ID != store.Shared && x.FPBTeamID == 0 && store.Plain(x.Name) == store.Plain(t.name) {
			g = x
			break
		}
	}
	if g.ID == "" {
		var err error
		if g, err = s.Store.AddGroup(t.name); err != nil {
			res.Errors = append(res.Errors, t.name+": "+err.Error())
			return
		}
		res.Created++
	}
	_, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error {
		linkTeam(g, t.TeamInfo)
		return nil
	})
	if err != nil {
		res.Errors = append(res.Errors, t.name+": "+err.Error())
		return
	}
	ch, failed, err := s.syncGroup(g.ID, deadline)
	if err != nil {
		res.Errors = append(res.Errors, t.name+": não consegui trazer os jogadores ("+err.Error()+").")
		return
	}
	res.ImportedPlayers += ch.NewPlayers
	if failed > 0 {
		res.Errors = append(res.Errors, fmt.Sprintf("%s: %d fotografias por descarregar.", t.name, failed))
	}
}

// acceptRoster is "Importar da FPB" with every suggestion taken: a player
// that looks like a local one is linked to it, the rest are added. Returns
// how many were linked or added.
func acceptRoster(g *store.Group, entries []store.RosterPlayer) int {
	_, sugg := store.Import(g.Players, entries)
	for _, sg := range sugg {
		id, name := sg.FPBPlayerID, sg.Name
		if sg.BestMatch != nil {
			if p := findPlayer(g.Players, *sg.BestMatch); p != nil && p.FPBPlayerID == nil {
				p.FPBPlayerID, p.FPBName, p.PhotoSource, p.FPBAthleteID = &id, &name, sg.PhotoURL, sg.FPBAthleteID
				if p.Number == "" && sg.Number != "" {
					p.Number, p.NumberFromFPB = sg.Number, true
				}
				continue
			}
		}
		g.Players = append(g.Players, store.Player{ID: store.NewID(), DisplayName: name, Number: sg.Number,
			FPBPlayerID: &id, FPBName: &name, NumberFromFPB: sg.Number != "", PhotoSource: sg.PhotoURL, FPBAthleteID: sg.FPBAthleteID})
	}
	return len(sugg)
}

func (s *Server) clubSetup(w http.ResponseWriter, r *http.Request) {
	res, err := s.SetupClub()
	switch {
	case errors.Is(err, errSetupRunning):
		fail(w, http.StatusConflict, "As equipas do clube já estão a ser importadas. Aguarde um pouco.")
	case errors.Is(err, errNoClubSearch):
		fail(w, http.StatusBadRequest, "Escreva primeiro o nome do clube na FPB, em Definições > Clube.")
	case err != nil:
		fpbFail(w, err)
	default:
		reply(w, res)
	}
}
