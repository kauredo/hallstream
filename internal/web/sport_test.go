package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
)

// setSport sets an escalão's sport straight in the store, whether or not the
// API accepts it.
func setSport(t *testing.T, s *Server, id, sport string) {
	t.Helper()
	if _, err := s.Store.UpdateGroup(id, func(g *store.Group) error { g.Sport = sport; return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestPutGroupSport(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	for _, sp := range []string{"curling", "handball"} {
		var e struct{ Error string }
		if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": sp}, &e); code != http.StatusBadRequest || e.Error != i18n.T("api.escalao.desporto") {
			t.Errorf("sport %q: %d %q", sp, code, e.Error)
		}
	}
	var got store.Group
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": "futsal"}, &got); code != http.StatusOK || got.Sport != "futsal" {
		t.Errorf("futsal: %d %+v", code, got)
	}
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": "basketball"}, &got); code != http.StatusOK || got.Sport != "basketball" {
		t.Errorf("basketball: %d %+v", code, got)
	}
	if kept, _ := s.Store.Group(g.ID); kept.Sport != "basketball" {
		t.Errorf("stored sport %q", kept.Sport)
	}
}

// The FPB routes of an escalão answer 404 unless that escalão is basketball.
func TestFPBGateSportPerEscalao(t *testing.T) {
	s := newServer(t)
	var bb, fut store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores"}, &bb)
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores futsal"}, &fut)
	setSport(t, s, fut.ID, "futsal")
	routes := []struct{ name, path string }{
		{"import", "/players/import"},
		{"from-fpb", "/players/from-fpb"},
		{"link", "/players/p1/link"},
	}
	for _, r := range routes {
		var e struct{ Error string }
		if code := call(t, s, "POST", "/api/groups/"+fut.ID+r.path, map[string]any{}, &e); code != http.StatusNotFound || e.Error != noFPB.Error() {
			t.Errorf("futsal %s: %d %q, want 404 %q", r.name, code, e.Error, noFPB.Error())
		}
		// Past the gate, the handlers refuse the empty request themselves.
		if code := call(t, s, "POST", "/api/groups/"+bb.ID+r.path, map[string]any{}, &e); code != http.StatusBadRequest || e.Error == noFPB.Error() {
			t.Errorf("basketball %s: %d %q, want the handler's 400", r.name, code, e.Error)
		}
	}
	var e struct{ Error string }
	if code := call(t, s, "POST", "/api/groups/nonexistent/players/import", map[string]any{}, &e); code != http.StatusNotFound || e.Error == noFPB.Error() {
		t.Errorf("unknown escalão: %d %q, want the handler's own 404", code, e.Error)
	}
	// The shared group follows the club, whatever sport it stored.
	setSport(t, s, store.Shared, "futsal")
	if code := call(t, s, "POST", "/api/groups/"+store.Shared+"/players/import", map[string]any{}, &e); e.Error == noFPB.Error() {
		t.Errorf("geral gated by its own sport: %d", code)
	}
}

// A club the FPB doesn't cover by its main sport still reaches it through a
// basketball escalão.
func TestFPBGateClubLevelSport(t *testing.T) {
	s := newServer(t)
	setClub(t, s, "futsal", "PT")
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores"}, &g)
	setSport(t, s, g.ID, "basketball")
	gated := func() bool {
		var e struct{ Error string }
		call(t, s, "GET", "/api/fpb/teams?name=Clube", nil, &e)
		return e.Error == noFPB.Error()
	}
	if gated() {
		t.Error("futsal club with a basketball escalão: /api/fpb/teams gated")
	}
	setSport(t, s, g.ID, "futsal")
	if !gated() {
		t.Error("futsal club with only futsal escalões: /api/fpb/teams not gated")
	}
}

func TestProvidersSportPerEscalao(t *testing.T) {
	ids := func(s *Server, g store.Group) string {
		var out []string
		for _, p := range s.providers(g) {
			out = append(out, p.ID())
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct {
		name           string
		sport, country string
		group          string
		noFPB          bool
		want           string
	}{
		{"PT basketball", "basketball", "PT", "basketball", false, "fpb,manual,ics"},
		{"old settings, no sport", "", "", "", false, "fpb,manual,ics"},
		{"futsal escalão", "basketball", "PT", "futsal", false, "manual,ics"},
		{"ES basketball", "basketball", "ES", "basketball", false, "manual,ics"},
		{"no FPB client", "basketball", "PT", "basketball", true, "manual,ics"},
	} {
		s := newServer(t)
		setClub(t, s, c.sport, c.country)
		if c.noFPB {
			s.FPB = nil
		}
		if got := ids(s, store.Group{Sport: c.group}); got != c.want {
			t.Errorf("%s: providers %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGameSportFollowsEscalao(t *testing.T) {
	s := newServer(t)
	if got := s.Store.GameSport(); got != "basketball" {
		t.Errorf("no escalão picked: %q, want the club's", got)
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID}, nil)
	setClub(t, s, "futsal", "PT")
	if got := s.Store.GameSport(); got != "basketball" {
		t.Errorf("GameSport %q, want the escalão's basketball", got)
	}
	s.SyncGame()
	if game := s.State.Snapshot().Game; game.Sport != "basketball" || game.Rules.ID != "basketball" {
		t.Errorf("game sport %q, rules %q", game.Sport, game.Rules.ID)
	}
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": ""}, nil)
	if got := s.Store.GameSport(); got != "futsal" {
		t.Errorf("no escalão again: %q, want the club's futsal", got)
	}
}

// An escalão that isn't basketball never reaches the FPB: no request from the
// next game, none from the daily update, and its LastSync stays empty.
func TestFPBNotAskedForOtherSports(t *testing.T) {
	s := newServer(t)
	f := clubFPB(t, s)
	s.Store.UpdateSettings(func(set *store.Settings) { set.ClubSearch = "" }) // only the escalão loop
	var fut, bb store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores futsal"}, &fut)
	setSport(t, s, fut.ID, "futsal")
	link := func(id string) {
		t.Helper()
		if _, err := s.Store.UpdateGroup(id, func(g *store.Group) error { g.FPBTeamID = 64791; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	link(fut.ID)
	hits := func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.hits }

	s.findNext(store.Game{GroupID: fut.ID}, time.Now())
	s.SyncAll(0)
	if n := hits(); n != 0 {
		t.Errorf("futsal escalão: %d FPB requests", n)
	}
	if g, _ := s.Store.Group(fut.ID); g.LastSync != "" {
		t.Errorf("futsal escalão synced: %q", g.LastSync)
	}

	// Control: the same link on a basketball escalão does reach the FPB.
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores"}, &bb)
	link(bb.ID)
	s.findNext(store.Game{GroupID: bb.ID}, time.Now())
	if hits() == 0 {
		t.Error("basketball escalão: findNext made no FPB request")
	}
	before := hits()
	s.SyncAll(0)
	if hits() == before {
		t.Error("basketball escalão: SyncAll made no FPB request")
	}
	if g, _ := s.Store.Group(bb.ID); g.LastSync == "" {
		t.Error("basketball escalão not synced")
	}
}

func TestPutGroupSportFPBLink(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	setSport(t, s, g.ID, "futsal")
	var e struct{ Error string }
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 5}, &e); code != http.StatusBadRequest || e.Error != i18n.T("api.escalao.desporto_fpb") {
		t.Errorf("link on futsal: %d %q", code, e.Error)
	}
	if kept, _ := s.Store.Group(g.ID); kept.FPBTeamID != 0 {
		t.Errorf("link stored despite the 400: %d", kept.FPBTeamID)
	}
	// Negative controls. Basketball sent again on a linked group is fine.
	link := func(sport string) {
		t.Helper()
		if _, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error { g.Sport, g.FPBTeamID = sport, 5; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	link("basketball")
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": "basketball"}, nil); code != http.StatusOK {
		t.Errorf("basketball on a linked group: %d", code)
	}
	// One PUT can unlink and change the sport.
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 0, "sport": "basketball"}, nil); code != http.StatusOK {
		t.Errorf("unlink and basketball: %d", code)
	}
	// Unlinking a futsal group leaves nothing to refuse.
	link("futsal")
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 0}, nil); code != http.StatusOK {
		t.Errorf("unlink a futsal group: %d", code)
	}
	if kept, _ := s.Store.Group(g.ID); kept.FPBTeamID != 0 {
		t.Errorf("still linked: %d", kept.FPBTeamID)
	}
}

// Club setup reuses an escalão by name only when it is basketball.
func TestSetupTeamSportMatch(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	var fut store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior Masculino"}, &fut)
	setSport(t, s, fut.ID, "futsal")
	var res setupResult
	if code := call(t, s, "POST", "/api/club/setup", nil, &res); code != http.StatusOK || len(res.Errors) != 0 {
		t.Fatalf("setup: %d %+v", code, res)
	}
	if kept, _ := s.Store.Group(fut.ID); kept.FPBTeamID != 0 || kept.Sport != "futsal" {
		t.Errorf("futsal escalão reused: %+v", kept)
	}
	n := 0
	for _, g := range clubGroups(s) {
		if g.Name == "Sénior Masculino" && g.ID != fut.ID {
			n++
			if g.FPBTeamID != 63135 || g.Sport != "basketball" {
				t.Errorf("created escalão %+v", g)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d basketball escalões created for the team, want 1", n)
	}
}

// A futsal club's new escalão made from an FPB team plays basketball.
func TestSetupTeamLinkMakesBasketball(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	setClub(t, s, "futsal", "PT")
	var bb store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Outro"}, &bb)
	setSport(t, s, bb.ID, "basketball") // so the FPB is offered
	var res setupResult
	if code := call(t, s, "POST", "/api/club/setup", nil, &res); code != http.StatusOK {
		t.Fatalf("setup: %d %+v", code, res)
	}
	if g := groupNamed(t, s, "Sénior Masculino"); g.Sport != "basketball" || g.FPBTeamID != 63135 {
		t.Errorf("escalão from an FPB team: %+v", g)
	}
}
