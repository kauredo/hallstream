package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/store"
)

func TestPickNext(t *testing.T) {
	games := []fpb.NextGame{
		{ID: 329595, Date: "2026-11-14", Time: "20H30"},
		{ID: 329591, Date: "2026-11-20", Time: "21H45"},
		{ID: 329600, Date: "2026-11-28", Time: "18H00"},
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, lisbon)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	id := func(n int64) *int64 { return &n }
	for _, c := range []struct {
		name      string
		id        *int64
		start     time.Time
		now       string
		cur, next int
	}{
		{"by id", id(329595), time.Time{}, "2026-11-14 21:30", 0, 1},
		{"by id, last game", id(329600), time.Time{}, "2026-11-28 19:00", 2, -1},
		{"id gone from the list", id(1), time.Time{}, "2026-11-14 21:30", -1, 1},
		{"no id, during the game", nil, time.Time{}, "2026-11-14 21:30", -1, 1},
		{"no id, morning of the game", nil, time.Time{}, "2026-11-14 09:00", -1, 1},
		{"no id, after midnight", nil, time.Time{}, "2026-11-15 00:40", -1, 1},
		{"no id, a quiet weekday", nil, time.Time{}, "2026-11-11 15:00", -1, 0},
		{"manual start days ahead", nil, at("2026-11-14 20:30"), "2026-11-11 15:00", -1, 1},
		{"stale start", nil, at("2026-11-01 18:00"), "2026-11-11 15:00", -1, 0},
		{"nothing after", nil, time.Time{}, "2026-11-28 10:00", -1, -1},
	} {
		cur, next := pickNext(games, c.id, c.start, at(c.now))
		if cur != c.cur || next != c.next {
			t.Errorf("%s: got %d, %d; want %d, %d", c.name, cur, next, c.cur, c.next)
		}
	}
}

func TestPickNextOrderAndTimes(t *testing.T) {
	id := int64(1)
	now := time.Date(2026, 11, 14, 21, 0, 0, 0, lisbon)
	// Out of order: the game after 1 is 3 (20 Nov), not 2 (28 Nov) or 4 (earlier).
	games := []fpb.NextGame{
		{ID: 2, Date: "2026-11-28", Time: "18H00"},
		{ID: 1, Date: "2026-11-14", Time: "20H30"},
		{ID: 4, Date: "2026-11-10", Time: "20H30"},
		{ID: 3, Date: "2026-11-20", Time: "21:45"},
	}
	if cur, next := pickNext(games, &id, time.Time{}, now); cur != 1 || next != 3 {
		t.Errorf("unsorted: %d, %d", cur, next)
	}
	// A game with no time yet is never picked.
	games = []fpb.NextGame{{ID: 1, Date: "2026-11-14", Time: "20H30"}, {ID: 5, Date: "2026-11-18", Time: ""}, {ID: 6, Date: "2026-11-21", Time: "TBD"}}
	if _, next := pickNext(games, &id, time.Time{}, now); next != -1 {
		t.Errorf("game without a time picked: %d", next)
	}
	if _, next := pickNext(games, nil, time.Time{}, now); next != -1 {
		t.Errorf("no id, game without a time picked: %d", next)
	}
}

func TestNextGameInState(t *testing.T) {
	s := newServer(t)
	s.RefreshNext()
	if g := s.State.Snapshot().Game; g.Next != nil || !strings.Contains(g.NextNote, "escalão") {
		t.Errorf("no game: %+v %q", g.Next, g.NextNote)
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior Masculino"}, &g)
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "away": map[string]string{"name": "ATLÉTICO VALE"}}, nil)
	if note := s.State.Snapshot().Game.NextNote; !strings.Contains(note, "não está ligado") {
		t.Errorf("unlinked note %q", note)
	}

	// Linking the escalão of the game reads its games.
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 64791}, nil)
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "fpb_game_id": 555}, nil)
	st := s.State.Snapshot().Game
	if st.AtHome == nil || !*st.AtHome {
		t.Errorf("game 555 is at home: %v", st.AtHome)
	}
	n := st.Next
	if n == nil {
		t.Fatalf("no next game: %q", st.NextNote)
	}
	if n.AtHome || n.Opponent.Name != "ATLÉTICO VALE" || n.FullName != "Atlético do Vale" || n.Opponent.Short != "ATV" ||
		n.Venue != "Pavilhão X, Vale Fundo" || n.Competition != "2ª Divisão" || n.Start != "2026-11-22T20:30:00Z" {
		t.Errorf("next %+v", n)
	}
	if n.Opponent.CrestURL == nil || *n.Opponent.CrestURL != "/media/opp-901.png" || fpb.Contrast(n.Opponent.Color) < 4.5 {
		t.Errorf("next crest %v %s", n.Opponent.CrestURL, n.Opponent.Color)
	}
	if code := call(t, s, "POST", "/api/screen", map[string]string{"screen": "proximo"}, nil); code != 200 {
		t.Errorf("proximo with a next game: %d", code)
	}

	// The last game in the list has no next one: the screen is refused.
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "fpb_game_id": 556}, nil)
	st = s.State.Snapshot().Game
	if st.Next != nil || st.NextNote == "" || st.AtHome == nil || *st.AtHome {
		t.Errorf("after 556: next %+v note %q home %v", st.Next, st.NextNote, st.AtHome)
	}
	var e map[string]string
	if code := call(t, s, "POST", "/api/screen", map[string]string{"screen": "proximo"}, &e); code != 400 || !strings.Contains(e["error"], "14 dias") {
		t.Errorf("proximo without a next game: %d %v", code, e)
	}
	if code := call(t, s, "POST", "/api/screen", map[string]string{"screen": "voltamos"}, nil); code != 200 {
		t.Errorf("voltamos: %d", code)
	}
}

func TestBusyFPBDoesNotReadAsNoConnection(t *testing.T) {
	s := newServer(t)
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(busy.Close)
	s.FPB.BaseURL = func() string { return busy.URL }

	var e map[string]string
	code := call(t, s, "GET", "/api/fpb/teams?name=casa", nil, &e)
	if code != http.StatusTooManyRequests || !strings.Contains(e["error"], "demasiados pedidos") {
		t.Errorf("team search on a 429: %d %q", code, e["error"])
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior Masculino"}, &g)
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 64791}, nil)
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "away": map[string]string{"name": "ATLÉTICO VALE"}}, nil)
	if note := s.State.Snapshot().Game.NextNote; !strings.Contains(note, "demasiados pedidos") {
		t.Errorf("next game note on a 429: %q", note)
	}
}

func TestBackLine(t *testing.T) {
	s := newServer(t)
	for _, c := range []struct{ send, want string }{
		{"  A imagem   regressa já ", "A imagem regressa já"},
		{strings.Repeat("é", 70), strings.Repeat("é", maxBackLine)},
		{"", ""},
	} {
		set := s.Store.Settings()
		set.Overlay.BackLine = c.send
		var got store.Settings
		call(t, s, "PUT", "/api/settings", set, &got)
		if got.Overlay.BackLine != c.want || s.State.Snapshot().Game.Overlay.BackLine != c.want {
			t.Errorf("sent %q: settings %q, state %q", c.send, got.Overlay.BackLine, s.State.Snapshot().Game.Overlay.BackLine)
		}
	}
}
