package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/store"
)

var clubNames = []string{
	"Sénior Masculino", "Sub 21 Masculino", "Sub 18 Masculino", "Sub 16 Masculino", "Sub 16 Masculino B",
	"Sub 14 Masculino", "Sub 14 Feminino", "Mini 12 Masculino", "Mini 12 Feminino", "Mini 10 Masculino",
	"Mini 10 Feminino", "Mini 8 Masculino", "Mini 8 Feminino", "Sénior Masculino (2)",
}

// testClub is the settings.club_search the club tests use.
const testClub = "Clube Exemplo"

// fakeClub is an FPB serving a team search for testClub (testdata, with one
// team of another club whose name only contains it), small rosters ("@" in a roster is the server address), empty
// rosters for the other teams and photos under /photo/. While down it
// answers 503 to everything, and while busy 429.
type fakeClub struct {
	mu      sync.Mutex
	down    bool
	busy    bool
	teams   []byte
	rosters map[string]string
	hits    int // requests received, whatever the answer
	// onRoster runs before a roster is served.
	onRoster func(id string)
}

func (f *fakeClub) set(fn func(f *fakeClub)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func clubFPB(t *testing.T, s *Server) *fakeClub {
	teams, err := os.ReadFile("testdata/fpb-teams-clube.json")
	if err != nil {
		t.Fatal(err)
	}
	s.Store.UpdateSettings(func(set *store.Settings) { set.ClubSearch = testClub })
	f := &fakeClub{teams: teams, rosters: map[string]string{
		"63135": `[{"player_id":1,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"7","photo_url":"@/photo/1.jpg","games":3},
			{"player_id":2,"fpb_athlete_id":5002,"name":"Pedro Nunes","jersey_number":8,"photo_url":null,"games":1}]`,
		"64791": `[{"player_id":3,"fpb_athlete_id":5003,"name":"João Costa","jersey_number":"10","games":2}]`,
	}}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		if f.down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if f.busy {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		p := r.URL.Path
		id, isRoster := strings.CutSuffix(strings.TrimPrefix(p, "/api/teams/"), "/roster")
		switch {
		case p == "/api/teams" && r.URL.Query().Get("name") == testClub && r.UserAgent() == "hallstream/test":
			w.Write(f.teams)
		case isRoster:
			if f.onRoster != nil {
				f.onRoster(id)
			}
			players := f.rosters[id]
			if players == "" {
				players = "[]"
			}
			io.WriteString(w, `{"team":{"id":`+id+`},"players":`+strings.ReplaceAll(players, "@", srv.URL)+`}`)
		case strings.HasPrefix(p, "/photo/"):
			w.Header().Set("Content-Type", "image/jpeg")
			io.WriteString(w, "photo "+p)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	s.FPB = fpb.New(func() string { return srv.URL }, s.Store.CacheDir(), "test")
	s.FPB.Every = 0
	s.FPB.AllowHTTP = true
	s.FPB.PhotoHosts = []string{strings.TrimPrefix(srv.URL, "http://")}
	return f
}

func clubGroups(s *Server) (out []store.Group) {
	for _, g := range s.Store.Groups() {
		if g.ID != store.Shared {
			out = append(out, g)
		}
	}
	return out
}

func groupNamed(t *testing.T, s *Server, name string) store.Group {
	t.Helper()
	for _, g := range clubGroups(s) {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no escalão %q", name)
	return store.Group{}
}

func TestFirstRunSetsUpClub(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)

	s.FirstRun(false)
	groups := clubGroups(s)
	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	if strings.Join(names, "|") != strings.Join(clubNames, "|") {
		t.Fatalf("names\n got %q\nwant %q", names, clubNames)
	}
	for _, g := range groups {
		if g.FPBTeamID == 0 || g.FPBTeamName == "" {
			t.Errorf("%s not linked: %+v", g.Name, g)
		}
	}
	sen := groupNamed(t, s, "Sénior Masculino")
	if sen.FPBTeamID != 63135 || sen.Coach != "" || len(sen.Players) != 2 {
		t.Fatalf("sénior %+v", sen)
	}
	rui := sen.Players[0]
	if rui.DisplayName != "Rui Silva" || rui.Number != "7" || !rui.NumberFromFPB || rui.FPBPlayerID == nil || *rui.FPBPlayerID != 1 ||
		!strings.HasPrefix(rui.PhotoURL, "/media/pl-"+rui.ID+"-") || sen.Players[1].PhotoURL != "" {
		t.Errorf("players %+v", sen.Players)
	}
	if g := groupNamed(t, s, "Sub 21 Masculino"); g.FPBTeamID != 64791 || len(g.Players) != 1 || g.Coach != "" {
		t.Errorf("sub 21 %+v", g)
	}
	if g := groupNamed(t, s, "Sénior Masculino (2)"); g.FPBTeamID != 63831 {
		t.Errorf("sénior (2) %+v", g)
	}
	if st := s.Store.Settings().ClubSetup; st == "" || st == "pending" {
		t.Errorf("club_setup %q", st)
	}

	// The user renames one, types a coach on it and deletes another.
	call(t, s, "PUT", "/api/groups/"+sen.ID, map[string]any{"name": "Seniores", "coach": "Outro Treinador"}, nil)
	call(t, s, "DELETE", "/api/groups/"+groupNamed(t, s, "Sub 18 Masculino").ID, nil, nil)
	var res setupResult
	if code := call(t, s, "POST", "/api/club/setup", nil, &res); code != 200 || res.Created != 1 || res.ImportedPlayers != 0 || len(res.Errors) != 0 {
		t.Fatalf("second run %d %+v", code, res)
	}
	if n := len(clubGroups(s)); n != 14 {
		t.Errorf("%d escalões after second run", n)
	}
	sen, _ = s.Store.Group(sen.ID)
	if sen.Name != "Seniores" || sen.Coach != "Outro Treinador" || len(sen.Players) != 2 {
		t.Errorf("user edits lost: %+v", sen)
	}
	groupNamed(t, s, "Sub 18 Masculino")

	// Done: the next start does nothing, even with no escalões left.
	for _, g := range clubGroups(s) {
		s.Store.DeleteGroup(g.ID)
	}
	s.FirstRun(false)
	if n := len(clubGroups(s)); n != 0 {
		t.Errorf("first run ran again: %d escalões", n)
	}
}

func TestFirstRunFPBDown(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	f.set(func(f *fakeClub) { f.down = true })

	s.FirstRun(false)
	if st := s.Store.Settings().ClubSetup; st != "pending" {
		t.Fatalf("club_setup %q", st)
	}
	if n := len(clubGroups(s)); n != 0 {
		t.Fatalf("%d escalões with the FPB down", n)
	}
	var e map[string]string
	if code := call(t, s, "POST", "/api/club/setup", nil, &e); code != 502 || e["error"] == "" {
		t.Errorf("button while down: %d %v", code, e)
	}

	// Next start, still down: tries again, adds nothing twice.
	s.FirstRun(false)
	if n := len(clubGroups(s)); n != 0 || s.Store.Settings().ClubSetup != "pending" {
		t.Fatalf("retry: %d escalões, %q", n, s.Store.Settings().ClubSetup)
	}

	// The FPB is back and the button runs the setup.
	f.set(func(f *fakeClub) { f.down = false })
	var res setupResult
	if code := call(t, s, "POST", "/api/club/setup", nil, &res); code != 200 || res.Created != 14 || res.ImportedPlayers != 3 {
		t.Fatalf("button %d %+v", code, res)
	}
	sen := groupNamed(t, s, "Sénior Masculino")
	if sen.FPBTeamID != 63135 || len(sen.Players) != 2 || len(clubGroups(s)) != 14 {
		t.Errorf("sénior %+v", sen)
	}
	if st := s.Store.Settings().ClubSetup; st == "" || st == "pending" {
		t.Errorf("club_setup %q", st)
	}
}

func TestClubTeam(t *testing.T) {
	for _, c := range []struct {
		search, name string
		want         bool
	}{
		{"Clube Exemplo", "Clube Exemplo", true},
		{"Clube Exemplo", "CLUBE EXEMPLO-B", true},
		{"clube exemplo", "Clube Exemplo Seguros Tejo", true},
		{"Clube Exemplo", "Outro Clube Exemplo", false},
		{"", "Clube Exemplo", false},
	} {
		if got := clubTeam(c.search, c.name); got != c.want {
			t.Errorf("clubTeam(%q, %q) = %v", c.search, c.name, got)
		}
	}
}

func TestFirstRunSkipped(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	s.FirstRun(true) // -demo
	if len(clubGroups(s)) != 0 || s.Store.Settings().ClubSetup != "" {
		t.Errorf("demo ran the setup")
	}

	// An older dados folder with escalões is left alone.
	s.Store.AddGroup("Sub 14 Feminino")
	s.FirstRun(false)
	if len(clubGroups(s)) != 1 || s.Store.Settings().ClubSetup != "" {
		t.Errorf("setup ran on a folder with escalões")
	}
}

func TestSettingsKeepClubSetup(t *testing.T) {
	s := newServer(t)
	s.Store.SetClubSetup("pending")
	set := s.Store.Settings()
	set.ClubSetup = ""
	call(t, s, "PUT", "/api/settings", set, nil)
	if got := s.Store.Settings().ClubSetup; got != "pending" {
		t.Errorf("club_setup %q after PUT /api/settings", got)
	}
}

func TestFirstRunNeedsClubSearch(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	s.Store.UpdateSettings(func(set *store.Settings) { set.ClubSearch = "" })
	s.FirstRun(false)
	if len(clubGroups(s)) != 0 || s.Store.Settings().ClubSetup != "" {
		t.Errorf("setup ran with no club_search")
	}
	var e map[string]string
	if code := call(t, s, "POST", "/api/club/setup", nil, &e); code != 400 || e["error"] == "" {
		t.Errorf("button with no club_search: %d %v", code, e)
	}
}
