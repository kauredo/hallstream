package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	assets "github.com/kauredo/hallstream/web"
)

// volleyballServer is a server whose game is a volleyball escalão scored from
// the phone: the manual scorer follows the game's sport, as in main.go.
func volleyballServer(t *testing.T, files *Server) *Server {
	t.Helper()
	s := files
	s.Log = func(string) {}
	s.Manual = source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Get(s.Store.GameSport()) }})
	sw := &source.Switch{Sources: []source.Source{source.NewConsole(&source.Console{}), s.Manual}, State: s.State}
	s.UseInput = sw.Use
	if code := call(t, s, "POST", "/api/groups", map[string]string{"name": "Juvenis"}, nil); code != 201 {
		t.Fatalf("POST group: %d", code)
	}
	if code := call(t, s, "PUT", "/api/groups/juvenis", map[string]string{"sport": "volleyball"}, nil); code != 200 {
		t.Fatalf("PUT group: %d", code)
	}
	game := map[string]any{"group_id": "juvenis", "away": map[string]string{"name": "VISITANTE", "short": "VIS"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	onManual(t, s)
	sw.Use("manual")
	return s
}

func points(t *testing.T, s *Server, team string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := s.Manual.Do(source.Action{Do: "point", Team: team, N: 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A new game forgets the last match: the phone scorer holds its sets too, and
// would send them again for the machine to record and end the match with.
func TestNewGameForgetsAVolleyballMatch(t *testing.T) {
	s := volleyballServer(t, newServer(t))
	var finals int
	s.State.OnEvent = func(ev string, f map[string]any) {
		if ev == "screen" && f["name"] == "final" {
			finals++
		}
	}
	for i := 0; i < 3; i++ {
		points(t, s, "home", 25)
		if err := s.Manual.Do(source.Action{Do: "set_next"}); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the match to end", func() bool { st := s.State.Snapshot(); return len(st.Sets) == 3 && st.Screen.Current == "final" })
	if finals != 1 {
		t.Fatalf("final fired %d times before the reset", finals)
	}
	if code := call(t, s, "POST", "/api/game/reset", nil, nil); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if err := s.State.SetScreen("marcador", true); err != nil {
		t.Fatal(err)
	}
	// The scorer sends its score at least once a second; give it two.
	time.Sleep(2200 * time.Millisecond)
	st := s.State.Snapshot()
	if len(st.Sets) != 0 || st.Live.HomeSets != 0 || st.Screen.Current != "marcador" || finals != 1 {
		t.Errorf("after the reset: %d sets, live sets %d, screen %q, final fired %d times", len(st.Sets), st.Live.HomeSets, st.Screen.Current, finals)
	}
	if sc := s.Manual.Score(); sc.Home != 0 || sc.Away != 0 || sc.Sets[0].Winner != "" || sc.Period != 1 {
		t.Errorf("the scorer still holds %+v", sc)
	}
}

// Picking another game does the same.
func TestAnotherGameForgetsAVolleyballMatch(t *testing.T) {
	s := volleyballServer(t, newServer(t))
	points(t, s, "away", 25)
	if err := s.Manual.Do(source.Action{Do: "set_next"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the set", func() bool { return len(s.State.Snapshot().Sets) == 1 })
	game := map[string]any{"group_id": "juvenis", "away": map[string]string{"name": "OUTRO", "short": "OUT"}, "start": "2026-11-22T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	if sc := s.Manual.Score(); sc.Sets[0].Winner != "" || len(s.State.Snapshot().Sets) != 0 {
		t.Errorf("the next game kept the last one's sets: %+v", sc)
	}
}

// The scorer's page for volleyball: +1 to 25-23, "Set seguinte" lights, the
// confirm dialog ends the set, the serve toggles.
func TestMarcarPageVolleyball(t *testing.T) {
	s := volleyballServer(t, newServerFiles(t, assets.FS))
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t) // after the server, so the page's event stream ends before the server closes
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector("#mesa").hidden && document.body.classList.contains("sets")`); err != nil {
		t.Fatal(err)
	}
	run := func(js string) {
		t.Helper()
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js+`, true`)); err != nil {
			t.Fatal(err)
		}
	}
	if ok, _ := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#arrancar').disabled`)); !ok {
		t.Error("Set seguinte is enabled at 0-0")
	}
	points(t, s, "home", 24)
	points(t, s, "away", 23)
	if err := until(tab, `document.querySelector('[data-v="home"]').textContent === "24" && document.querySelector('#arrancar').disabled`); err != nil {
		t.Fatalf("at 24-23: %v", err)
	}
	run(`document.querySelector('.casa [data-n="1"]').click()`)
	if err := until(tab, `document.querySelector('[data-v="home"]').textContent === "25" && !document.querySelector('#arrancar').disabled`); err != nil {
		t.Fatalf("at 25-23: %v", err)
	}
	run(`document.querySelector('#arrancar').click()`)
	if err := until(tab, `document.querySelector('#confirmar').open && document.querySelector('#confirmarTitulo').textContent.includes("25-23")`); err != nil {
		t.Fatalf("confirm dialog: %v", err)
	}
	run(`document.querySelector('#confirmarSim').click()`)
	if err := until(tab, `document.querySelector('#periodo').textContent.includes("2") && document.querySelector('#feitos').textContent.includes("25–23")`); err != nil {
		t.Fatalf("after the set: %v", err)
	}
	if st := s.State.Snapshot(); len(st.Sets) != 1 || st.Sets[0].Home != 25 || st.Sets[0].Away != 23 || st.Live.HomeSets != 1 {
		t.Errorf("state: sets %v, live %+v", st.Sets, st.Live)
	}
	// A new set starts with nobody serving; a tap on the ring gives it to a side.
	run(`document.querySelector('.fora .servico').click()`)
	if err := until(tab, `document.querySelector('.fora .servico').classList.contains("on") && !document.querySelector('.casa .servico').classList.contains("on")`); err != nil {
		t.Fatalf("serve to the visitor: %v", err)
	}
	run(`document.querySelector('.casa .servico').click()`)
	if err := until(tab, `document.querySelector('.casa .servico').classList.contains("on") && !document.querySelector('.fora .servico').classList.contains("on")`); err != nil {
		t.Fatalf("serve back to the club: %v", err)
	}
	// With the console back on air the button is "Marcar daqui" again and must
	// not still open the set dialog.
	s.UseInput("console")
	if err := until(tab, `document.body.classList.contains("consola") && !("ask" in document.querySelector('#arrancar').dataset) && document.querySelector('#arrancar').dataset.do === "take"`); err != nil {
		t.Errorf("Marcar daqui kept the set dialog: %v", err)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}
