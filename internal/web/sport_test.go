package web

import (
	"net/http"
	"testing"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
)

func TestPutGroupSport(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	for _, sp := range []string{"curling", "futsal"} {
		var e struct{ Error string }
		if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": sp}, &e); code != http.StatusBadRequest || e.Error != i18n.T("api.escalao.desporto") {
			t.Errorf("sport %q: %d %q", sp, code, e.Error)
		}
	}
	var got store.Group
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
	if _, err := s.Store.UpdateGroup(fut.ID, func(g *store.Group) error { g.Sport = "futsal"; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   string
		want int
	}{{fut.ID, http.StatusNotFound}, {bb.ID, http.StatusOK}, {"nonexistent", http.StatusNotFound}} {
		var e struct{ Error string }
		code := call(t, s, "POST", "/api/groups/"+c.id+"/players/import", map[string]any{}, &e)
		gated := e.Error == noFPB.Error()
		if c.id == fut.ID && !gated || c.id != fut.ID && gated {
			t.Errorf("%s: gate answered %d %q", c.id, code, e.Error)
		}
		if c.id == "nonexistent" && code != http.StatusNotFound {
			t.Errorf("nonexistent: %d", code)
		}
	}
}

// Club setup reuses an escalão by name only when it is basketball.
func TestSetupTeamSportMatch(t *testing.T) {
	s := newServer(t)
	clubFPB(t, s)
	var fut store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior Masculino"}, &fut)
	if _, err := s.Store.UpdateGroup(fut.ID, func(g *store.Group) error { g.Sport = "futsal"; return nil }); err != nil {
		t.Fatal(err)
	}
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

func TestPutGroupSportFPBLink(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	// Futsal can't be picked yet, so set it straight in the store.
	if _, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error { g.Sport = "futsal"; return nil }); err != nil {
		t.Fatal(err)
	}
	var e struct{ Error string }
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 5}, &e); code != http.StatusBadRequest || e.Error != i18n.T("api.escalao.desporto_fpb") {
		t.Errorf("link on futsal: %d %q", code, e.Error)
	}
	if kept, _ := s.Store.Group(g.ID); kept.FPBTeamID != 0 {
		t.Errorf("link stored despite the 400: %d", kept.FPBTeamID)
	}
	// One PUT that unlinks and sets basketball succeeds.
	if _, err := s.Store.UpdateGroup(g.ID, func(g *store.Group) error { g.Sport, g.FPBTeamID = "basketball", 5; return nil }); err != nil {
		t.Fatal(err)
	}
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 0, "sport": "basketball"}, nil); code != http.StatusOK {
		t.Errorf("unlink and basketball: %d", code)
	}
}
