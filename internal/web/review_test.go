package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/tribuna/internal/fpb"
	"github.com/kauredo/tribuna/internal/store"
)

func needsTeam(s *Server) []string {
	var out []string
	for _, g := range clubGroups(s) {
		if g.NeedsTeam {
			out = append(out, g.Name)
		}
	}
	return out
}

// An empty or partial team list for the current season moves and flags nothing.
func TestSyncEmptyOrPartialTeamList(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	teams := s.Store.Settings().ClubTeams
	if len(teams) != 14 || s.Store.Settings().ClubSeason != "2026-2027" {
		t.Fatalf("after setup: %v %q", teams, s.Store.Settings().ClubSeason)
	}
	full := f.teams

	f.set(func(f *fakeClub) { f.teams = []byte("[]") })
	s.SyncAll(0)
	if n := needsTeam(s); len(n) != 0 || len(s.Store.Settings().ClubTeams) != 14 {
		t.Errorf("empty list: flagged %v, club_teams %v", n, s.Store.Settings().ClubTeams)
	}

	var list []map[string]any
	json.Unmarshal(full, &list)
	part, _ := json.Marshal(list[:2])
	f.set(func(f *fakeClub) { f.teams = part })
	s.SyncAll(0)
	if n := needsTeam(s); len(n) != 0 {
		t.Errorf("partial list flagged %v", n)
	}
	if g := groupNamed(t, s, "Mini 8 Masculino"); g.FPBTeamID != 59377 {
		t.Errorf("partial list relinked %+v", g)
	}
}

// An escalão linked by hand to a team outside the club stays as it is.
func TestNewSeasonLeavesHandLinkedTeams(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 14 Feminino Amigos"}, &g)
	other := map[string]any{"id": 777, "name": "Outro Clube", "age": "Sub 14", "gender": "Feminino", "squad_label": "Sub 14 F"}
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 777, "fpb_team": other}, nil)
	if g, _ = s.Store.Group(g.ID); g.FPBTeamKey != "" {
		t.Fatalf("non-club team got a key: %+v", g)
	}

	f.set(func(f *fakeClub) { nextSeason(t, f) })
	s.SyncAll(time.Hour)
	if g, _ = s.Store.Group(g.ID); g.FPBTeamID != 777 || g.NeedsTeam {
		t.Errorf("hand-linked escalão touched: %+v", g)
	}
}

// A new team with an empty roster marks nobody; the next update with players does.
func TestNewSeasonEmptyRosterWaits(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")

	f.set(func(f *fakeClub) { nextSeason(t, f) })
	s.SyncAll(time.Hour)
	sen, _ = s.Store.Group(sen.ID)
	if sen.FPBTeamID != 163135 || sen.SyncedTeam != 63135 || sen.LastChanges.Left != 0 {
		t.Fatalf("empty roster: %+v", sen)
	}
	for _, p := range sen.Players {
		if p.Inactive {
			t.Errorf("%s inactive on an empty roster", p.DisplayName)
		}
	}

	f.set(func(f *fakeClub) {
		f.rosters["163135"] = `[{"player_id":101,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"7","photo_url":"@/photo/1.jpg"}]`
	})
	s.SyncAll(0)
	sen, _ = s.Store.Group(sen.ID)
	if p := playerNamed(t, sen, "Pedro Nunes"); !p.Inactive || sen.SyncedTeam != 163135 {
		t.Errorf("roster filled: pedro %+v, synced %d", p, sen.SyncedTeam)
	}
}

// The daily update finishes a setup left pending; club_teams keeps only linked teams.
func TestSyncFinishesPendingSetup(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	f.set(func(f *fakeClub) { f.down = true })
	s.FirstRun(false)
	f.set(func(f *fakeClub) { f.down = false })

	s.SyncAll(time.Hour)
	if n := len(clubGroups(s)); n != 14 || s.Store.Settings().ClubSetup == "pending" {
		t.Fatalf("pending setup not finished: %d escalões, %q", n, s.Store.Settings().ClubSetup)
	}

	mini := groupNamed(t, s, "Mini 8 Feminino")
	call(t, s, "DELETE", "/api/groups/"+mini.ID, nil, nil)
	s.SyncAll(0)
	if slices.Contains(s.Store.Settings().ClubTeams, mini.FPBTeamID) || len(clubGroups(s)) != 13 {
		t.Errorf("deleted escalão: club_teams %v, %d escalões", s.Store.Settings().ClubTeams, len(clubGroups(s)))
	}
}

// PUT /api/settings with an old copy keeps what the program changed meanwhile.
func TestSettingsKeepProgramFields(t *testing.T) {
	s := newServer(t)
	stale := s.Store.Settings()
	s.Store.SetBoxAddress("AA:BB")
	s.Store.SetClubSetup("2026-11-08T10:00:00Z")
	s.Store.SetClubTeams([]int64{1, 2})
	s.Store.SetClubSeason("2026-2027")
	stale.Instagram = "@outro"
	call(t, s, "PUT", "/api/settings", stale, nil)
	got := s.Store.Settings()
	if got.Instagram != "@outro" || got.BoxAddress != "AA:BB" || got.ClubSetup == "" || len(got.ClubTeams) != 2 || got.ClubSeason != "2026-2027" {
		t.Errorf("settings %+v", got)
	}
}

// Saving name and coach from an old page doesn't undo a relink.
func TestSaveGroupKeepsLink(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")
	s.Store.UpdateGroup(sen.ID, func(g *store.Group) error { g.FPBTeamID = 163135; return nil })
	call(t, s, "PUT", "/api/groups/"+sen.ID, map[string]string{"name": "Seniores", "coach": "Outro"}, nil)
	if g, _ := s.Store.Group(sen.ID); g.FPBTeamID != 163135 || g.Name != "Seniores" || g.FPBTeamKey != "Sénior Masculino" {
		t.Errorf("group %+v", g)
	}
}

// A roster fetched for a team the escalão left meanwhile is thrown away.
func TestSyncDropsRosterOfOldTeam(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sub21 := groupNamed(t, s, "Sub 21 Masculino")
	f.set(func(f *fakeClub) {
		f.rosters["64791"] = `[{"player_id":3,"name":"João Costa"},{"player_id":9,"name":"Novo Jogador"}]`
		f.onRoster = func(id string) {
			if id == "64791" {
				s.Store.UpdateGroup(sub21.ID, func(g *store.Group) error { g.FPBTeamID = 999; return nil })
			}
		}
	})
	if _, _, err := s.syncGroup(sub21.ID, time.Now().Add(maxRun)); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Store.Group(sub21.ID); len(g.Players) != 1 {
		t.Errorf("old team's roster applied: %+v", g.Players)
	}
}

func TestSameSiteWrites(t *testing.T) {
	s := newServer(t)
	send := func(mod func(r *http.Request)) int {
		r := httptest.NewRequest("POST", "/api/groups", strings.NewReader(`{"name":"X"}`))
		r.RemoteAddr = "127.0.0.1:5000"
		r.Host = "127.0.0.1:8765"
		r.Header.Set("Content-Type", "application/json")
		mod(r)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Code
	}
	for name, c := range map[string]struct {
		mod  func(r *http.Request)
		want int
	}{
		"panel":         {func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:8765") }, 201},
		"localhost":     {func(r *http.Request) { r.Host = "localhost:8765" }, 201},
		"other origin":  {func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, 403},
		"rebound host":  {func(r *http.Request) { r.Host = "evil.example:8765" }, 403},
		"lan, no phone": {func(r *http.Request) { r.Host = "192.168.1.40:8765" }, 403},
		"form post":     {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		"no type":       {func(r *http.Request) { r.Header.Del("Content-Type") }, 415},
		"multipart":     {func(r *http.Request) { r.Header.Set("Content-Type", "multipart/form-data; boundary=x") }, 415},
	} {
		if got := send(c.mod); got != c.want {
			t.Errorf("%s: %d, want %d", name, got, c.want)
		}
	}
	// Uploads take multipart.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "a.png")
	fw.Write(redPNG())
	mw.Close()
	r := httptest.NewRequest("POST", "/api/groups/geral/sponsors", &body)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Host = "127.0.0.1:8765"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Errorf("upload %d %s", w.Code, w.Body)
	}
}

// Every request, reads included, needs a Host of this PC, so a page that
// rebinds its own name to 127.0.0.1 can't read the PIN or the state.
func TestHostOnEveryRequest(t *testing.T) {
	s := newServer(t)
	lanIP(t)
	done, cancel := context.WithCancel(context.Background())
	cancel() // the SSE stream sends one event and returns
	get := func(path, from, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil).WithContext(done)
		r.RemoteAddr = from
		r.Host = host
		r.Header.Set("X-PIN", "1234")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, phone := range []bool{false, true} {
		set := s.Store.Settings()
		set.PhoneAccess, set.PIN = phone, "1234"
		s.Store.SaveSettings(set)
		for host, want := range map[string]int{
			"127.0.0.1:8765":    200,
			"127.0.0.1:8766":    200,
			"localhost:8765":    200,
			"localhost":         200,
			"[::1]:8765":        200,
			"evil.example:8765": 403,
			"evil.example":      403,
			"127.0.0.1.nip.io":  403,
			"":                  403,
			"192.168.1.40:8765": map[bool]int{false: 403, true: 200}[phone],
			"192.168.1.99:8765": 403,
		} {
			for _, from := range []string{"127.0.0.1:5000", "192.168.1.20:5000"} {
				for _, path := range []string{"/api/settings", "/api/status", "/api/events", "/overlay", "/painel"} {
					w := get(path, from, host)
					if w.Code != want {
						t.Errorf("phone %v, from %s, Host %q, %s: %d, want %d", phone, from, host, path, w.Code, want)
					}
					if want == 403 && w.Body.Len() != 0 {
						t.Errorf("Host %q, %s: 403 with body %q", host, path, w.Body)
					}
				}
			}
		}
	}

	// Phone access turned off keeps listening on the LAN until a restart:
	// a phone that names the PC as 127.0.0.1 still needs the PIN.
	set := s.Store.Settings()
	set.PhoneAccess = false
	s.Store.SaveSettings(set)
	r := httptest.NewRequest("GET", "/api/settings", nil)
	r.RemoteAddr = "192.168.1.20:5000"
	r.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), "1234") {
		t.Errorf("LAN with a forged Host after phone access off: %d %s", w.Code, w.Body)
	}
}

func TestLimits(t *testing.T) {
	var teams []fpb.TeamInfo
	for i := range 60 {
		teams = append(teams, fpb.TeamInfo{ID: int64(i + 1), Name: "Clube Exemplo", Age: fmt.Sprintf("Sub %d", i), Gender: "Masculino"})
	}
	if n := len(clubPlan(testClub, teams)); n != maxTeams {
		t.Errorf("plan of %d teams", n)
	}
	var roster fpb.Roster
	for i := range 150 {
		roster.Players = append(roster.Players, fpb.RosterPlayer{PlayerID: int64(i + 1), Name: fmt.Sprint("P", i)})
	}
	if n := len(rosterEntries(roster)); n != maxPlayers {
		t.Errorf("roster of %d players", n)
	}
}
