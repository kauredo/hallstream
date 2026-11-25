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
