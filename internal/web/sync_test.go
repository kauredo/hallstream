package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/store"
)

func playerNamed(t *testing.T, g store.Group, name string) store.Player {
	t.Helper()
	for _, p := range g.Players {
		if p.DisplayName == name {
			return p
		}
	}
	t.Fatalf("%s has no player %q: %+v", g.Name, name, g.Players)
	return store.Player{}
}

func TestDailySync(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")
	rui := playerNamed(t, sen, "Rui Silva")
	if sen.LastSync == "" || rui.FPBAthleteID != 5001 {
		t.Fatalf("after setup: %+v", sen)
	}
	call(t, s, "PUT", "/api/groups/"+sen.ID+"/players/"+rui.ID, map[string]string{"display_name": "Rui S."}, nil)

	// FPB: Rui has a new number and photo, Pedro left, Ana is new.
	f.set(func(f *fakeClub) {
		f.rosters["63135"] = `[{"player_id":1,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"9","photo_url":"@/photo/1b.jpg"},
			{"player_id":4,"fpb_athlete_id":5004,"name":"Ana Reis","jersey_number":"5","photo_url":"@/photo/4.jpg"}]`
	})
	s.SyncAll(time.Hour) // just updated: nothing to do
	if g, _ := s.Store.Group(sen.ID); len(g.Players) != 2 {
		t.Fatalf("updated within the hour: %+v", g.Players)
	}
	s.SyncAll(0)
	sen, _ = s.Store.Group(sen.ID)
	want := store.SyncChanges{NewPlayers: 1, Numbers: 1, Photos: 2}
	if sen.LastChanges != want {
		t.Errorf("changes %+v, want %+v", sen.LastChanges, want)
	}
	rui = playerNamed(t, sen, "Rui S.")
	if rui.Number != "9" || rui.PhotoSource == "" || rui.PhotoURL == "" {
		t.Errorf("rui %+v", rui)
	}
	if p := playerNamed(t, sen, "Pedro Nunes"); p.Inactive {
		t.Errorf("pedro inactive without a new season")
	}
	if p := playerNamed(t, sen, "Ana Reis"); p.Number != "5" || p.PhotoURL == "" {
		t.Errorf("ana %+v", p)
	}

	// A number typed by hand stays.
	call(t, s, "PUT", "/api/groups/"+sen.ID+"/players/"+rui.ID, map[string]string{"number": "21"}, nil)
	f.set(func(f *fakeClub) {
		f.rosters["63135"] = `[{"player_id":1,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"10"}]`
	})
	s.SyncAll(0)
	sen, _ = s.Store.Group(sen.ID)
	if p := playerNamed(t, sen, "Rui S."); p.Number != "21" || len(sen.Players) != 3 {
		t.Errorf("hand-typed number lost: %+v", sen.Players)
	}

	// Picking a game for an escalão updates it.
	sub21 := groupNamed(t, s, "Sub 21 Masculino")
	f.set(func(f *fakeClub) {
		f.rosters["64791"] = `[{"player_id":3,"fpb_athlete_id":5003,"name":"João Costa","jersey_number":"10"},{"player_id":6,"name":"Luís Mota"}]`
	})
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": sub21.ID}, nil)
	if g, _ := s.Store.Group(sub21.ID); len(g.Players) != 2 || g.LastChanges.NewPlayers != 1 {
		t.Errorf("game pick: %+v", g)
	}
}

func TestSyncAllDoesNotCountTheSavedCopyAsAnUpdate(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")
	old := "2026-01-01T00:00:00Z"
	s.Store.UpdateGroup(sen.ID, func(g *store.Group) error { g.LastSync = old; return nil })

	// The FPB has a new player and answers 429: the saved roster is all there is.
	f.set(func(f *fakeClub) {
		f.busy = true
		f.rosters["63135"] = `[{"player_id":1,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"7"},
			{"player_id":4,"fpb_athlete_id":5004,"name":"Ana Reis","jersey_number":"5"}]`
	})
	s.SyncAll(0)
	if g, _ := s.Store.Group(sen.ID); g.LastSync != old || g.LastChanges != sen.LastChanges {
		t.Errorf("a 429 counted as an update: last sync %q, changes %+v", g.LastSync, g.LastChanges)
	}

	f.set(func(f *fakeClub) { f.busy = false })
	s.SyncAll(0)
	if g, _ := s.Store.Group(sen.ID); g.LastSync == old || g.LastChanges.NewPlayers != 1 {
		t.Errorf("the FPB answers again: last sync %q, changes %+v", g.LastSync, g.LastChanges)
	}
}

// nextSeason turns the fixture into next season's list: new ids, two
// "Sub 14 Feminino" teams, no Mini 8 Feminino, and a new Sub 19 team.
func nextSeason(t *testing.T, f *fakeClub) {
	var teams []map[string]any
	if err := json.Unmarshal(f.teams, &teams); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, tm := range teams {
		id := tm["id"].(float64)
		tm["id"] = id + 100000
		tm["slug"] = strings.Replace(tm["slug"].(string), "2026-2027", "2027-2028", 1)
		switch id {
		case 63841: // Mini 8 F
			continue
		case 63836: // Sub 14 F, twice
			twin := map[string]any{}
			for k, v := range tm {
				twin[k] = v
			}
			twin["id"] = float64(299999)
			out = append(out, twin)
		}
		out = append(out, tm)
	}
	out = append(out, map[string]any{"id": 300000, "name": "Clube Exemplo", "age": "Sub 19", "gender": "Masculino",
		"squad_label": "Sub 19 M", "slug": "clube-exemplo-sub-19-m-2027-2028", "has_fixtures": true})
	f.teams, _ = json.Marshal(out)
}

func TestNewSeason(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")
	rui, pedro := playerNamed(t, sen, "Rui Silva"), playerNamed(t, sen, "Pedro Nunes")

	// Rui stays with a new player id, Pedro is gone, Ana joins.
	f.set(func(f *fakeClub) {
		nextSeason(t, f)
		f.rosters["163135"] = `[{"player_id":101,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"7","photo_url":"@/photo/1.jpg"},
			{"player_id":104,"fpb_athlete_id":5004,"name":"Ana Reis","jersey_number":"5"}]`
	})
	s.SyncAll(time.Hour)

	sen, _ = s.Store.Group(sen.ID)
	if sen.FPBTeamID != 163135 || sen.NeedsTeam || sen.LastChanges.Left != 1 || sen.LastChanges.NewPlayers != 1 {
		t.Fatalf("sénior %+v", sen)
	}
	r := playerNamed(t, sen, "Rui Silva")
	if r.ID != rui.ID || *r.FPBPlayerID != 101 || r.PhotoURL != rui.PhotoURL || r.Inactive {
		t.Errorf("rui %+v (was %+v)", r, rui)
	}
	if p := playerNamed(t, sen, "Pedro Nunes"); !p.Inactive {
		t.Errorf("pedro should be inactive: %+v", p)
	}
	if g := groupNamed(t, s, "Sénior Masculino (2)"); g.FPBTeamID != 163831 {
		t.Errorf("sénior (2) %+v", g)
	}
	if g := groupNamed(t, s, "Sub 16 Masculino B"); g.FPBTeamID != 163834 {
		t.Errorf("sub 16 B %+v", g)
	}
	for _, name := range []string{"Mini 8 Feminino", "Sub 14 Feminino"} {
		if g := groupNamed(t, s, name); !g.NeedsTeam || g.FPBTeamID == 0 {
			t.Errorf("%s should wait for the volunteer: %+v", name, g)
		}
	}
	if g := groupNamed(t, s, "Sub 19 Masculino"); g.FPBTeamID != 300000 {
		t.Errorf("new team %+v", g)
	}
	if n := len(clubGroups(s)); n != 15 {
		t.Errorf("%d escalões, want 15", n)
	}

	// The squad overlay leaves Pedro out.
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": sen.ID, "squad": []map[string]string{{"player_id": rui.ID}, {"player_id": pedro.ID}}}, nil)
	if sq := s.State.Snapshot().Game.Squad; len(sq) != 1 || sq[0].Name != "Rui Silva" {
		t.Errorf("squad %+v", sq)
	}

	// The volunteer picks the Sub 14 Feminino team.
	sub14 := groupNamed(t, s, "Sub 14 Feminino")
	team := map[string]any{"id": 299999, "name": "Clube Exemplo", "age": "Sub 14", "gender": "Feminino", "squad_label": "Sub 14 F"}
	call(t, s, "PUT", "/api/groups/"+sub14.ID, map[string]any{"fpb_team_id": 299999, "fpb_team": team}, nil)
	if g, _ := s.Store.Group(sub14.ID); g.NeedsTeam || g.FPBTeamKey != "Sub 14 Feminino" {
		t.Errorf("picked %+v", g)
	}

	// Pedro is back in the squad later.
	f.set(func(f *fakeClub) {
		f.rosters["163135"] = `[{"player_id":101,"fpb_athlete_id":5001,"name":"Rui Silva","jersey_number":"7","photo_url":"@/photo/1.jpg"},
			{"player_id":102,"fpb_athlete_id":5002,"name":"Pedro Nunes"}]`
	})
	s.SyncAll(0)
	sen, _ = s.Store.Group(sen.ID)
	if p := playerNamed(t, sen, "Pedro Nunes"); p.Inactive || sen.LastChanges.Returned != 1 {
		t.Errorf("pedro back: %+v, %+v", p, sen.LastChanges)
	}

	// "Voltar a mostrar" by hand.
	call(t, s, "PUT", "/api/groups/"+sen.ID+"/players/"+playerNamed(t, sen, "Ana Reis").ID, map[string]any{"inactive": true}, nil)
	var p store.Player
	call(t, s, "PUT", "/api/groups/"+sen.ID+"/players/"+playerNamed(t, sen, "Ana Reis").ID, map[string]any{"inactive": false}, &p)
	if p.Inactive {
		t.Errorf("inactive not cleared: %+v", p)
	}
}

func TestSyncWithoutFPB(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.FirstRun(false)
	sen := groupNamed(t, s, "Sénior Masculino")
	f.set(func(f *fakeClub) { f.down = true })
	s.SyncAll(0)
	if g, _ := s.Store.Group(sen.ID); g.LastSync != sen.LastSync || g.FPBTeamID != 63135 || g.NeedsTeam {
		t.Errorf("changed without the FPB: %+v", g)
	}
}

func TestDailyRunLiveGame(t *testing.T) {
	setup := func(t *testing.T) *Server {
		s := newServer(t)
		clubFPB(t, s)
		s.Store.SetClubSetup("pending")
		return s
	}
	ran := func(s *Server) bool { return s.Store.Settings().ClubSetup != "pending" }

	t.Run("clock ran a minute ago", func(t *testing.T) {
		s := setup(t)
		s.State.ApplyConsole(map[string]string{"period": "1", "clock": "07:45"})
		s.State.ApplyConsole(map[string]string{"period": "1", "clock": "07:44"})
		if s.dailyRun() || ran(s) {
			t.Fatal("synced while the clock was running")
		}
	})
	t.Run("console on, clock still", func(t *testing.T) {
		s := setup(t)
		s.State.ApplyConsole(map[string]string{"period": "1", "clock": "10:00"})
		s.State.ApplyConsole(map[string]string{"period": "1", "clock": "10:00"})
		if !s.dailyRun() || !ran(s) {
			t.Fatal("a connected console with a stopped clock held the sync back")
		}
	})
	t.Run("OBS streaming", func(t *testing.T) {
		s := setup(t)
		host, port := fakeOBS(t, "", func(typ string) (bool, int, any) {
			return true, 100, map[string]any{"outputActive": true}
		})
		s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
		if s.dailyRun() || ran(s) {
			t.Fatal("synced while OBS was streaming")
		}
	})
}

func TestFetchPhotosStopsOnADeadHostAndAtTheDeadline(t *testing.T) {
	s := newServer(t)
	tried := 0
	s.Log = func(m string) {
		if strings.Contains(m, "a fotografia de") {
			tried++
		}
	}
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			io.WriteString(w, "photo")
		case "/away.jpg":
			http.Redirect(w, r, "http://elsewhere.invalid/1.jpg", http.StatusFound)
		case "/slow.jpg": // answers only once the program has given up
			select {
			case <-r.Context().Done():
			case <-time.After(20 * time.Second):
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(live.Close)
	gone := httptest.NewServer(nil)
	gone.Close() // nothing listens here any more
	s.FPB.AllowHTTP = true
	s.FPB.PhotoHosts = []string{strings.TrimPrefix(live.URL, "http://"), strings.TrimPrefix(gone.URL, "http://")}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 14"}, &g)
	dead, missing, ok := gone.URL+"/1.jpg", live.URL+"/missing.jpg", live.URL+"/ok.jpg"
	away, slow := live.URL+"/away.jpg", live.URL+"/slow.jpg"

	for _, c := range []struct {
		name         string
		photos       []string
		in           time.Duration // the deadline, from now
		tried, short int           // downloads that failed, and photos left without a copy
	}{
		{"a host that does not answer", []string{dead, dead, dead, dead, dead, dead, dead, dead}, time.Minute, maxPhotoFails, 8},
		{"one that works starts the count again", []string{dead, dead, ok, dead, dead, dead, dead}, time.Minute, 5, 6},
		{"a 404 is an answer", []string{missing, missing, missing, missing, missing}, time.Minute, 5, 5},
		{"a refused redirect is an answer", []string{away, away, away, away, away}, time.Minute, 5, 5},
		{"past the deadline", []string{ok, ok}, 0, 0, 2},
		{"the deadline cuts a download short", []string{slow, ok, ok}, 300 * time.Millisecond, 1, 3},
	} {
		var players []store.Player
		for i, u := range c.photos {
			players = append(players, store.Player{ID: "p" + strconv.Itoa(i), DisplayName: "Jogador", PhotoSource: u})
		}
		tried = 0
		start := time.Now()
		short := s.fetchPhotos(g.ID, nil, players, start.Add(c.in))
		if took := time.Since(start); short != c.short || tried != c.tried || took > 5*time.Second {
			t.Errorf("%s: %d failed downloads and %d photos short in %v, want %d and %d", c.name, tried, short, took.Round(time.Millisecond), c.tried, c.short)
		}
	}
}

func TestBackgroundPanicGoesToTheLog(t *testing.T) {
	s := newServer(t)
	var logged []string
	s.Log = func(m string) {
		logged = append(logged, m)
		if len(logged) == 1 {
			panic("boom") // inside syncInBackground, as it reports the unknown escalão
		}
	}
	s.syncInBackground("no such escalão")
	s.bg.Wait()
	if len(logged) != 2 || !strings.Contains(logged[1], "Atualização da FPB: erro inesperado (boom)") || !strings.Contains(logged[1], "sync.go") {
		t.Errorf("want the panic and its stack in the log, got %q", logged)
	}
}
