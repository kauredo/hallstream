package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/store"
)

// An escalão's calendar link feeds the next game, fetched once and then read
// from the cache, which also keeps it going when the link stops answering.
func TestCalendarLinkMakesTheNextGame(t *testing.T) {
	s := newServer(t)
	setClub(t, s, "basketball", "ES")
	set := s.Store.Settings()
	set.Club.Name, set.Club.Short = "CB Vale", "CBV"
	call(t, s, "PUT", "/api/settings", set, nil)

	soon := time.Now().UTC().AddDate(0, 0, 3).Format("20060102T150405Z")
	var hits, down atomic.Int32
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if down.Load() == 1 {
			http.Error(w, "gone", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:g1\r\nDTSTART:%s\r\nSUMMARY:Norte vs CB Vale\r\nLOCATION:Pavilhão Norte\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", soon)
	}))
	defer feed.Close()

	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Senior"}, &g)
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]string{"calendar_url": "ftp://example.com/x.ics"}, nil); code != 400 {
		t.Errorf("an ftp link saved: %d", code)
	}
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]string{"calendar_url": feed.URL + "/team.ics"}, &g); code != 200 || g.CalendarURL == "" {
		t.Fatalf("save the link: %d %+v", code, g)
	}
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID}, nil)
	s.RefreshNext()
	n := s.State.Snapshot().Game.Next
	if n == nil || n.Opponent.Name != "Norte" || n.AtHome {
		t.Fatalf("next %+v, note %q", n, s.State.Snapshot().Game.NextNote)
	}
	var listed []store.Fixture
	if code := call(t, s, "GET", "/api/groups/"+g.ID+"/calendar", nil, &listed); code != 200 || len(listed) != 1 || listed[0].ID != "ics/g1" || listed[0].Home || listed[0].Venue != "Pavilhão Norte" {
		t.Errorf("calendar list: %d %+v", code, listed)
	}
	s.RefreshNext()
	if got := hits.Load(); got != 1 {
		t.Errorf("%d fetches, want 1 (the others read the cache)", got)
	}

	// The cache is a day old and the link fails: the old copy still answers.
	down.Store(1)
	matches, _ := filepath.Glob(s.Store.Dir + "/cache/ics/*.ics")
	for _, m := range matches {
		old := time.Now().Add(-2 * calendarFresh)
		os.Chtimes(m, old, old)
	}
	s.RefreshNext()
	if n := s.State.Snapshot().Game.Next; n == nil || n.Opponent.Name != "Norte" {
		t.Errorf("offline, next %+v", n)
	}
}
