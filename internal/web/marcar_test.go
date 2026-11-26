package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

type marcarReply struct {
	Input string         `json:"input"`
	Score source.Score   `json:"score"`
	Undo  *source.Action `json:"undo"`
}

func newMarcar(t *testing.T) (*Server, *[]string) {
	t.Helper()
	s := newServer(t)
	s.Manual = source.NewManual(&source.Manual{})
	used := &[]string{}
	s.UseInput = func(name string) { *used = append(*used, name) }
	s.Log = func(string) {}
	return s, used
}

// onManual puts the manual input on air, as a "take" from the page does.
func onManual(t *testing.T, s *Server) {
	t.Helper()
	if _, err := s.Store.UpdateSettings(func(v *store.Settings) { v.Input = "manual" }); err != nil {
		t.Fatal(err)
	}
}

// A tap never takes the scoring from the console by itself: "take" does,
// once; the taps after it score, and each reply names what undo takes back.
func TestMarcarTakeOver(t *testing.T) {
	s, used := newMarcar(t)
	var got marcarReply
	if code := call(t, s, "GET", "/api/marcar", nil, &got); code != 200 || got.Input != "console" {
		t.Fatalf("GET: %d %+v", code, got)
	}
	if code := call(t, s, "POST", "/api/marcar", source.Action{Do: "point", Team: "home", N: 2}, nil); code != 409 {
		t.Errorf("a point with the console on air: %d, want 409", code)
	}
	if code := call(t, s, "POST", "/api/marcar", source.Action{Do: "take"}, &got); code != 200 || got.Input != "manual" || s.Store.Settings().Input != "manual" {
		t.Fatalf("take: %d %+v, settings.input %q", code, got, s.Store.Settings().Input)
	}
	if got.Undo != nil {
		t.Errorf("undo before any tap: %+v", got.Undo)
	}
	call(t, s, "POST", "/api/marcar", source.Action{Do: "point", Team: "home", N: 2}, &got)
	call(t, s, "POST", "/api/marcar", source.Action{Do: "point", Team: "away", N: 3}, &got)
	if got.Score.Home != 2 || got.Score.Away != 3 || len(*used) != 1 || (*used)[0] != "manual" {
		t.Errorf("taps: %+v, UseInput calls %v", got.Score, *used)
	}
	if got.Undo == nil || *got.Undo != (source.Action{Do: "point", Team: "away", N: 3}) {
		t.Errorf("undo would take back %+v", got.Undo)
	}
	call(t, s, "POST", "/api/marcar", source.Action{Do: "take"}, &got)
	if len(*used) != 1 {
		t.Errorf("a second take switched again: %v", *used)
	}
	call(t, s, "POST", "/api/marcar", source.Action{Do: "undo"}, &got)
	if got.Score.Away != 0 || got.Score.Home != 2 || got.Undo == nil || got.Undo.Team != "home" {
		t.Errorf("undo: %+v, next undo %+v", got.Score, got.Undo)
	}
}

func TestMarcarRefusesABadTap(t *testing.T) {
	s, _ := newMarcar(t)
	onManual(t, s)
	var e struct{ Error string }
	if code := call(t, s, "POST", "/api/marcar", source.Action{Do: "point", Team: "home", N: 7}, &e); code != 400 || e.Error == "" {
		t.Errorf("7 points: %d %q", code, e.Error)
	}
	if code := call(t, s, "POST", "/api/marcar", source.Action{Do: "undo"}, &e); code != 400 {
		t.Errorf("undo with nothing done: %d", code)
	}
}

func TestMarcarWithoutManual(t *testing.T) {
	s := newServer(t)
	if code := call(t, s, "GET", "/api/marcar", nil, nil); code != 503 {
		t.Errorf("GET: %d", code)
	}
	if code := call(t, s, "POST", "/api/marcar", source.Action{Do: "clock_start"}, nil); code != 503 {
		t.Errorf("POST: %d", code)
	}
}

// A phone on the club's network needs the PIN to score, like the panel.
func TestMarcarNeedsThePINFromThePhone(t *testing.T) {
	s, _ := newMarcar(t)
	onManual(t, s)
	set := s.Store.Settings()
	set.PhoneAccess, set.PIN = true, "1234"
	if err := s.Store.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pin  string
		want int
	}{{"", 401}, {"9999", 401}, {"1234", 200}} {
		req := httptest.NewRequest("POST", "/api/marcar", strings.NewReader(`{"do":"point","team":"home","n":1}`))
		req.RemoteAddr = "192.168.1.20:5000"
		req.Host = "127.0.0.1:8765"
		req.Header.Set("Content-Type", "application/json")
		if c.pin != "" {
			req.Header.Set("X-PIN", c.pin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("PIN %q: %d, want %d", c.pin, w.Code, c.want)
		}
	}
	if s.Manual.Score().Home != 1 {
		t.Errorf("score %+v", s.Manual.Score())
	}
}

// GET answers the input on air with the score kept by hand.
func TestMarcarScore(t *testing.T) {
	s, _ := newMarcar(t)
	if _, err := s.Store.UpdateSettings(func(v *store.Settings) { v.Input = "manual" }); err != nil {
		t.Fatal(err)
	}
	if err := s.Manual.Do(source.Action{Do: "foul", Team: "away", N: 1}); err != nil {
		t.Fatal(err)
	}
	var got marcarReply
	call(t, s, "GET", "/api/marcar", nil, &got)
	if got.Input != "manual" || got.Score.AwayFouls != 1 || got.Score.Period != 1 {
		t.Errorf("got %+v", got)
	}
}

// The page is public like the panel, and the QR code holds its address on the network.
func TestMarcarPageAndQR(t *testing.T) {
	s := newServerFiles(t, fstest.MapFS{
		"marcar/index.html":   {Data: []byte("marcar")},
		"overlay/overlay.css": {Data: overlayCSS(t)},
	})
	req := httptest.NewRequest("GET", "/marcar", nil)
	req.RemoteAddr = "192.168.1.20:5000"
	req.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "marcar" {
		t.Errorf("/marcar: %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765/marcar/qr.svg", nil))
	switch {
	case LanIP() == "" && w.Code != 404:
		t.Errorf("no network: %d", w.Code)
	case LanIP() != "" && (w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<svg")):
		t.Errorf("qr: %d %.40q", w.Code, w.Body.String())
	}
}

// The real page, from the embedded files: a tap on +2 takes the score over
// from the console and the page shows it, and Iniciar starts the clock.
func TestMarcarPageScores(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	s.Log = func(string) {}
	s.Manual = source.NewManual(&source.Manual{})
	sw := &source.Switch{Sources: []source.Source{source.NewConsole(&source.Console{}), s.Manual}, State: s.State}
	sw.Use("console")
	s.UseInput = sw.Use
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t) // after the server, so the page's event stream ends before the server closes
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector("#mesa").hidden && document.body.classList.contains("consola")`); err != nil {
		t.Fatal(err)
	}
	run := func(js string) {
		t.Helper()
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js+`, true`)); err != nil {
			t.Fatal(err)
		}
	}
	// With the console on air the points wait for "Marcar daqui".
	if ok, _ := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('.casa [data-n="2"]').disabled`)); !ok {
		t.Error("+2 works before Marcar daqui")
	}
	run(`document.querySelector('#arrancar').click()`)
	if err := until(tab, `!document.body.classList.contains("consola") && !document.querySelector('.casa [data-n="2"]').disabled`); err != nil {
		t.Fatalf("Marcar daqui: %v", err)
	}
	run(`document.querySelector('.casa [data-n="2"]').click()`)
	if err := until(tab, `document.querySelector('[data-v="home"]').textContent === "2" && document.querySelector('#anular').textContent.includes("+2")`); err != nil {
		t.Errorf("after +2: %v", err)
	}
	// A long press on +2 takes two away.
	run(`(() => { const b = document.querySelector('.casa [data-n="2"]'); b.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true })); setTimeout(() => { b.dispatchEvent(new PointerEvent('pointerup', { bubbles: true })); b.click(); }, 700); })()`)
	if err := until(tab, `document.querySelector('[data-v="home"]').textContent === "0"`); err != nil {
		t.Errorf("after the long press: %v", err)
	}
	run(`document.querySelector('#arrancar').click()`)
	if err := until(tab, `document.body.classList.contains("corre")`); err != nil {
		t.Errorf("clock not running: %v", err)
	}
	if st := s.State.Snapshot(); st.Input != "manual" || st.Live.Home != 0 {
		t.Errorf("state: input %q, home %d", st.Input, st.Live.Home)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

// A card or a substitution from the scorer goes on air as a lower third: a home
// player by name from the game's squad, an away player by number only.
func TestMarcarCardAndSubLowerThirds(t *testing.T) {
	s := newServer(t)
	s.Log = func(string) {}
	s.Manual = source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Get(s.Store.GameSport()) }})
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores"}, nil)
	if _, err := s.Store.UpdateGroup("seniores", func(g *store.Group) error { g.Sport = "soccer"; return nil }); err != nil {
		t.Fatal(err)
	}
	var squad []map[string]string
	for _, p := range [][2]string{{"Rui Costa Lima", "9"}, {"Tomé", "14"}, {"Diogo Alves", "7"}, {"Sem Número", ""}} {
		var pl store.Player
		call(t, s, "POST", "/api/groups/seniores/players", map[string]string{"display_name": p[0], "number": p[1]}, &pl)
		squad = append(squad, map[string]string{"player_id": pl.ID})
	}
	game := map[string]any{"group_id": "seniores", "squad": squad, "away": map[string]string{"name": "VISITANTE", "short": "VIS"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	onManual(t, s)
	if got := s.State.Snapshot().Game.Squad; len(got) != 4 || s.State.Snapshot().Game.Rules.ID != "soccer" {
		t.Fatalf("game %+v", s.State.Snapshot().Game)
	}
	post := func(a source.Action) int { return call(t, s, "POST", "/api/marcar", a, nil) }

	if code := post(source.Action{Do: "card", Team: "home", Number: "9", Kind: "yellow"}); code != 200 {
		t.Fatalf("card: %d", code)
	}
	lt := s.State.Snapshot().LowerThird
	if !lt.Visible || lt.Type != "cartao" || lt.Card != "yellow" || lt.Number != "9" || lt.FirstName != "Rui" || lt.LastName != "Costa Lima" || lt.Label == "" || lt.Team != "" {
		t.Errorf("home card: %+v", lt)
	}
	post(source.Action{Do: "card", Team: "home", Number: "9", Kind: "yellow"})
	if lt := s.State.Snapshot().LowerThird; lt.Card != "second_yellow" {
		t.Errorf("second yellow: %+v", lt)
	}
	post(source.Action{Do: "card", Team: "away", Number: "9", Kind: "red"})
	if lt := s.State.Snapshot().LowerThird; lt.Card != "red" || lt.Number != "9" || lt.FirstName != "" || lt.LastName != "" || lt.PhotoURL != "" || lt.Label != "VISITANTE" || lt.Team != "away" {
		t.Errorf("away card must show the number only: %+v", lt)
	}
	post(source.Action{Do: "sub", Team: "home", Out: "7", In: "14"})
	lt = s.State.Snapshot().LowerThird
	if lt.Type != "substituicao" || lt.Number != "14" || lt.FirstName != "Tomé" || lt.OutNumber != "7" || lt.OutName != "Diogo Alves" {
		t.Errorf("home sub: %+v", lt)
	}
	post(source.Action{Do: "sub", Team: "away", Out: "3", In: "4"})
	if lt := s.State.Snapshot().LowerThird; lt.Number != "4" || lt.FirstName != "" || lt.OutNumber != "3" || lt.OutName != "" {
		t.Errorf("away sub: %+v", lt)
	}
	// A card for #0 never matches a squad player that has no number.
	post(source.Action{Do: "card", Team: "home", Number: "0", Kind: "yellow"})
	if lt := s.State.Snapshot().LowerThird; lt.Number != "0" || lt.FirstName != "" || lt.LastName != "" {
		t.Errorf("a card for #0 named a numberless player: %+v", lt)
	}
	before := s.State.Snapshot().LowerThird
	if code := post(source.Action{Do: "card", Team: "home", Number: "9", Kind: "red"}); code != 400 {
		t.Errorf("a card for a sent-off player: %d", code)
	}
	if s.State.Snapshot().LowerThird != before {
		t.Error("a refused card changed the lower third")
	}
}

// Picking another game after a soccer game was ended: no final screen, no cards,
// no substitutions and no recorded periods carry over.
func TestNextGameAfterASoccerGameStartsClean(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "soccer")
	eventually(t, func() bool { return s.State.Snapshot().Live.Period != "" }, "the scorer's first frame")
	for _, a := range []source.Action{
		{Do: "period_length", Tenths: 27000}, {Do: "new_game"},
		{Do: "point", Team: "home", N: 1},
		{Do: "card", Team: "home", Number: "4", Kind: "yellow"},
		{Do: "sub", Team: "away", Out: "7", In: "14"},
		{Do: "stoppage", N: 2},
		{Do: "period_next"}, {Do: "game_end"},
	} {
		if err := s.Manual.Do(a); err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	eventually(t, func() bool {
		st := s.State.Snapshot()
		return st.Screen.Current == "final" && len(st.Periods) == 2 && len(st.Live.Cards) == 1 && st.Live.Over
	}, "the game is over on air")

	game := map[string]any{"group_id": "seniores", "away": map[string]string{"name": "OUTRO", "short": "OUT"}, "start": "2026-11-22T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	eventually(t, func() bool { return !s.State.Snapshot().Live.Over }, "the next frame")
	st := s.State.Snapshot()
	if st.Screen.Current == "final" || len(st.Periods) != 0 || len(st.Live.Cards) != 0 || len(st.Live.Subs) != 0 || st.Live.Over || st.Live.Stoppage != 0 {
		t.Errorf("after: screen %q, periods %v, live %+v", st.Screen.Current, st.Periods, st.Live)
	}
	if sc := s.Manual.Score(); sc.Over || sc.Cards != nil || sc.Subs != nil || sc.Stoppage != 0 || sc.Period != 1 || sc.Home != 0 {
		t.Errorf("the scorer still holds %+v", sc)
	}
}

// A soccer scorer with only goals on it starts the next soccer game at 0-0 too.
func TestNextSoccerGameStartsAtNilNil(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "soccer")
	if err := s.Manual.Do(source.Action{Do: "point", Team: "home", N: 1}); err != nil {
		t.Fatal(err)
	}
	game := map[string]any{"group_id": "seniores", "away": map[string]string{"name": "OUTRO", "short": "OUT"}, "start": "2026-11-22T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	if sc := s.Manual.Score(); sc.Home != 0 {
		t.Errorf("the next game starts %d-%d", sc.Home, sc.Away)
	}
}

// A soccer scorer's leftovers go with the game even when the next game is another sport.
func TestNextGameInAnotherSportDropsSoccerLeftovers(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "soccer")
	for _, a := range []source.Action{{Do: "new_game"}, {Do: "card", Team: "home", Number: "4", Kind: "red"}} {
		if err := s.Manual.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Basquete"}, &g)
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]string{"sport": "basketball"}, nil)
	game := map[string]any{"group_id": g.ID, "away": map[string]string{"name": "OUTRO", "short": "OUT"}, "start": "2026-11-22T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	if sc := s.Manual.Score(); sc.Cards != nil || sc.Over {
		t.Errorf("a basketball game starts with soccer's cards: %+v", sc)
	}
}

// The guard that keeps a sport nobody can score yet out of the club and escalão
// settings stays, though every listed sport is playable now.
func TestAnInactiveSportIsRefused(t *testing.T) {
	saved := club.Sports
	club.Sports = append(slices.Clone(saved), club.Sport{ID: "curling", Name: "CURLING", Short: "CURLING", Active: false})
	t.Cleanup(func() { club.Sports = saved })
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	var e struct{ Error string }
	if code := call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"sport": "curling"}, &e); code != http.StatusBadRequest || e.Error != i18n.T("api.escalao.desporto") {
		t.Errorf("escalão: %d %q", code, e.Error)
	}
	if code, _ := putClub(t, s, map[string]any{"sport": "curling"}); code != http.StatusBadRequest {
		t.Errorf("club: %d", code)
	}
	if code, _ := putClub(t, s, map[string]any{"sport": "soccer"}); code != http.StatusOK {
		t.Errorf("an active sport: %d", code)
	}
}

// eventually waits for the scorer's frames to reach the state, which they do on
// their own goroutine.
func eventually(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("never reached: %s", what)
}

// soccerScorer is a server whose game is a soccer escalão with a three-player squad, scored from the phone.
func soccerScorer(t *testing.T) *Server {
	t.Helper()
	s := sportServer(t, newServerFiles(t, assets.FS), "soccer")
	var squad []map[string]string
	for _, p := range [][2]string{{"Rui Costa Lima", "9"}, {"Tomé", "14"}, {"Diogo Alves", "7"}} {
		var pl store.Player
		call(t, s, "POST", "/api/groups/seniores/players", map[string]string{"display_name": p[0], "number": p[1]}, &pl)
		squad = append(squad, map[string]string{"player_id": pl.ID})
	}
	game := map[string]any{"group_id": "seniores", "squad": squad, "away": map[string]string{"name": "VISITANTE", "short": "VIS"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	for _, a := range []source.Action{{Do: "period_length", Tenths: 27000}, {Do: "new_game"}} {
		if err := s.Manual.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// The scorer's page for soccer: the added-time stepper behind the clock, the
// card choice and shirt-number picker with a chip under the right team, the
// substitution recorded as a count and the last pair, and the period sheet's
// next half, then "Fim do jogo" behind a confirmation.
func TestMarcarSoccerPage(t *testing.T) {
	s := soccerScorer(t)
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t)
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	run := func(js string) error { _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js)); return err }
	tap := func(sel string) {
		t.Helper()
		if err := run("document.querySelector(`" + sel + "`).click(), true"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(400 * time.Millisecond) // the page ignores the same tap again for 350 ms after its reply
	}
	must := func(expr, what string) {
		t.Helper()
		if err := until(tab, expr); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	must(`!document.querySelector("#mesa").hidden && document.body.classList.contains("cartoes")`, "the page has no cards row")
	// First half: the period sheet offers the next half. Added time starts at +0', with nothing under the clock.
	tap(`#periodoBotao`)
	must(`document.querySelector('#periodos').open && document.querySelector('#seguinte').textContent === "Parte seguinte" && document.querySelector('#seguinte').dataset.ask === "period_next"`, "the sheet has no next half")
	tap(`#periodosFechar`)
	tap(`#relogio`)
	must(`document.querySelector('#acertar').open && document.querySelector('#compensacao').textContent === "+0'" && document.querySelector('#compensa').textContent === ""`, "added time at the start")
	tap(`[data-stoppage="1"]`)
	must(`document.querySelector('#compensacao').textContent === "+1'" && document.querySelector('#compensa').textContent === "+1'"`, "the stepper did not move")
	tap(`#acertarFechar`)

	// A home yellow from the squad, an away red by typing the number.
	tap(`.casa [data-pick="card"]`)
	must(`document.querySelector('#cartao').open`, "no choice of card")
	tap(`#cartao [data-kind="yellow"]`)
	must(`document.querySelector('#numero').open && document.querySelectorAll('#camisolas button').length === 3`, "the picker has no squad")
	if err := run(`[...document.querySelectorAll('#camisolas button')].find((b) => b.textContent === "9").click(), true`); err != nil {
		t.Fatal(err)
	}
	must(`!document.querySelector('#numero').open && document.querySelectorAll('.casa [data-registo] .reg').length === 1 && document.querySelectorAll('.fora [data-registo] .reg').length === 0`, "the yellow is not a chip under the home team")
	tap(`.fora [data-pick="card"]`)
	tap(`#cartao [data-kind="red"]`)
	must(`document.querySelector('#numero').open && !document.querySelector('#numeroCampo').hidden && document.querySelector('#camisolas').hidden`, "the away picker is not a typed number")
	if err := run(`(document.querySelector('#numeroCampo').value = "6", document.querySelector('#numeroFeito').click(), true)`); err != nil {
		t.Fatal(err)
	}
	must(`document.querySelectorAll('.fora [data-registo] .reg').length === 1 && document.querySelector('.fora [data-registo] .reg').textContent === "6" && document.querySelectorAll('.casa [data-registo] .reg').length === 1`, "the red is not a chip under the away team")
	// A substitution: off 7, on 14.
	tap(`.casa [data-pick="sub"]`)
	must(`document.querySelector('#numero').open && document.querySelector('#numeroTitulo').textContent.includes("Sai")`, "the substitution does not ask who goes off")
	if err := run(`[...document.querySelectorAll('#camisolas button')].find((b) => b.textContent === "7").click(), true`); err != nil {
		t.Fatal(err)
	}
	must(`document.querySelector('#numero').open && document.querySelector('#numeroTitulo').textContent.includes("Entra")`, "the substitution does not ask who comes on")
	if err := run(`[...document.querySelectorAll('#camisolas button')].find((b) => b.textContent === "14").click(), true`); err != nil {
		t.Fatal(err)
	}
	must(`!document.querySelector('#numero').open && document.querySelector('.casa [data-trocas] .conta').textContent === "1" && document.querySelector('.casa [data-trocas] .sai').textContent === "7" && document.querySelector('.casa [data-trocas] .entra').textContent === "14"`, "the substitution is not recorded under the home team")
	if sc := s.Manual.Score(); len(sc.Cards) != 2 || len(sc.Subs) != 1 || sc.Stoppage != 1 {
		t.Errorf("the scorer holds %d cards, %d subs, stoppage %d", len(sc.Cards), len(sc.Subs), sc.Stoppage)
	}

	// At 45:00 the clock button stays Iniciar: the clock runs on into added time.
	if err := s.Manual.Do(source.Action{Do: "clock_set", Tenths: 27000}); err != nil {
		t.Fatal(err)
	}
	must(`document.querySelector('#tempo').textContent === "45:00" && document.querySelector('#arrancar').textContent === "Iniciar"`, "the clock button changed at 45:00")

	// Second half: no next period in the sheet; "Fim do jogo" asks first, then ends the game.
	if err := s.Manual.Do(source.Action{Do: "period_next"}); err != nil {
		t.Fatal(err)
	}
	tap(`#periodoBotao`)
	must(`document.querySelector('#periodos').open && document.querySelector('#seguinte').textContent === "Fim do jogo" && document.querySelector('#seguinte').dataset.ask === "game_end" && !document.querySelector('[data-ask="period_next"]')`, "no Fim do jogo in the second half")
	tap(`#seguinte`)
	must(`!document.querySelector('#periodos').open && document.querySelector('#confirmar').open && document.querySelector('#confirmarTitulo').textContent === "Acabar o jogo?"`, "no confirmation")
	if s.Manual.Score().Over {
		t.Fatal("the game ended before the confirmation")
	}
	if err := run(`document.querySelector('#confirmarSim').click(), true`); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return s.State.Snapshot().Live.Over }, "live.over")
	must(`document.querySelector('#seguinte').disabled === true`, "the end button stays live after the game ended")

	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool {
		return m.Type == chromedp.ConsoleError || m.IsException()
	}); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}
