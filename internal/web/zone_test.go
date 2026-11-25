package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

func TestClubZone(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	if got := s.zone().String(); got != "Europe/Lisbon" {
		t.Errorf("default zone %s", got)
	}

	set := s.Store.Settings()
	set.Club.Country = "ES"
	if code := call(t, s, "PUT", "/api/settings", set, nil); code != 200 {
		t.Fatalf("PUT country: %d", code)
	}
	if got := s.zone().String(); got != "Europe/Madrid" {
		t.Errorf("Spanish club's zone %s", got)
	}

	set = s.Store.Settings()
	set.Club.TimeZone = "Atlantic/Canary"
	call(t, s, "PUT", "/api/settings", set, nil)
	req := httptest.NewRequest("GET", "/i18n.js", nil)
	req.Host, req.RemoteAddr = "127.0.0.1:8765", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `window.HS_TZ = "Atlantic/Canary";`) {
		t.Errorf("/i18n.js lacks the zone: %.200s", w.Body.String())
	}

	set.Club.TimeZone = "Mars/Olympus"
	if code := call(t, s, "PUT", "/api/settings", set, nil); code != 400 {
		t.Errorf("an unknown zone saved: %d", code)
	}
}

// A CSV's times are the club's wall clock: 18:30 in Madrid is 17:30 UTC in December.
func TestFixturesCSVInClubZone(t *testing.T) {
	s := newServer(t)
	set := s.Store.Settings()
	set.Club.Country = "ES"
	call(t, s, "PUT", "/api/settings", set, nil)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Senior"}, &g)
	var got struct{ Fixtures []store.Fixture }
	csv := "data,hora,adversario,casa_fora\n20/12/2030,18:30,Ribeira Basket,fora\n"
	if code := postFile(t, s, "/api/groups/"+g.ID+"/fixtures/csv", "games.csv", []byte(csv), &got); code != 200 || len(got.Fixtures) != 1 {
		t.Fatalf("csv: %d %+v", code, got)
	}
	start, err := time.Parse(time.RFC3339, got.Fixtures[0].Start)
	if err != nil || !start.Equal(time.Date(2030, 12, 20, 17, 30, 0, 0, time.UTC)) {
		t.Errorf("start %q, want 18:30 Madrid time", got.Fixtures[0].Start)
	}
}
