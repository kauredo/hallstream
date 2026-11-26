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

// sportServer is a server whose game is an escalão of the given sport, scored
// from the phone: the manual scorer follows the game's sport, as in main.go.
func sportServer(t *testing.T, s *Server, id string) *Server {
	t.Helper()
	s.Log = func(string) {}
	s.Manual = source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Get(s.Store.GameSport()) }})
	sw := &source.Switch{Sources: []source.Source{source.NewConsole(&source.Console{}), s.Manual}, State: s.State}
	s.UseInput = sw.Use
	if code := call(t, s, "POST", "/api/groups", map[string]string{"name": "Seniores"}, nil); code != 201 {
		t.Fatalf("POST group: %d", code)
	}
	if code := call(t, s, "PUT", "/api/groups/seniores", map[string]string{"sport": id}, nil); code != 200 {
		t.Fatalf("PUT group: %d", code)
	}
	game := map[string]any{"group_id": "seniores", "away": map[string]string{"name": "VISITANTE", "short": "VIS"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	onManual(t, s)
	sw.Use("manual")
	return s
}

func fouls(t *testing.T, s *Server, team string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := s.Manual.Do(source.Action{Do: "foul", Team: team, N: 1}); err != nil {
			t.Fatal(err)
		}
	}
}

// The scorer's page for futsal: halves, fouls and "Livre", the length steps, and extra time's own 5:00.
func TestMarcarPageFutsal(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "futsal")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t)
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector("#mesa").hidden && document.querySelector("#periodo").textContent.includes("parte")`); err != nil {
		t.Fatalf("the period is not a half: %v", err)
	}
	if err := until(tab, `document.querySelector('[data-v="homeFouls"]').textContent === "0" && getComputedStyle(document.querySelector('.casa .faltas')).display !== 'none'`); err != nil {
		t.Fatalf("no fouls record: %v", err)
	}
	// The period button opens its sheet; a length step applies at once.
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#periodoBotao').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#periodos').open && document.querySelector('#duracao').textContent === "20:00" && document.querySelector('#anterior').disabled`); err != nil {
		t.Fatalf("the period sheet: %v", err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('[data-length="600"]').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#duracao').textContent === "21:00" && document.querySelector('#tempo').textContent === "21:00"`); err != nil {
		t.Fatalf("+1 min on the length: %v (scorer length %d)", err, s.Manual.Score().Length)
	}
	time.Sleep(400 * time.Millisecond) // the page ignores the same tap again for 350 ms after its reply
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('[data-length="-600"]').click(), document.querySelector('#periodosFechar').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector('#periodos').open && document.querySelector('#duracao').textContent === "20:00"`); err != nil {
		t.Fatalf("−1 min on the length: %v", err)
	}
	// A goal is 1: the page shows only +1, and the server refuses a bigger one.
	if err := until(tab, `[...document.querySelectorAll('.casa .somar button')].filter((b) => getComputedStyle(b).display !== 'none').length === 1`); err != nil {
		t.Fatalf("futsal shows more than +1: %v", err)
	}
	fouls(t, s, "home", 5)
	if err := until(tab, `document.querySelector('[data-v="homeFouls"]').textContent === "5" && document.querySelector('.casa .faltas').hasAttribute("data-warn") && !document.querySelector('.casa .faltas').hasAttribute("data-bonus") && getComputedStyle(document.querySelector('.casa .faltas .conta')).backgroundColor === getComputedStyle(document.querySelector('.casa .aviso')).backgroundColor`); err != nil {
		t.Fatalf("at five fouls: %v", err)
	}
	fouls(t, s, "home", 1)
	if err := until(tab, `document.querySelector('.casa .faltas').hasAttribute("data-bonus") && document.querySelector('.casa .aviso').textContent === "Livre"`); err != nil {
		t.Fatalf("at six fouls: %v", err)
	}
	// Through the period sheet's button and confirm dialog, so its score follows.
	for i, want := range []string{"2", "Prolongamento"} {
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#seguinte').click(), true`)); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `document.querySelector('#confirmar').open`); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#confirmarSim').click(), true`)); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `!document.querySelector('#confirmar').open && document.querySelector("#periodo").textContent.includes("`+want+`")`); err != nil {
			got, _ := chromedp.Run(tab, chromedp.Evaluate[string](`document.querySelector("#periodo").textContent + "|" + document.querySelector("#estado").textContent`))
			t.Fatalf("period %d: %v (page says %q, scorer at period %d)", i+2, err, got, s.Manual.Score().Period)
		}
		time.Sleep(400 * time.Millisecond) // the page ignores the same tap again for 350 ms after its reply
	}
	// Extra time has no timeouts, and its length steps are off: they set the regulation length.
	if err := until(tab, `getComputedStyle(document.querySelector('.casa .rec.descontos')).display === 'none' && getComputedStyle(document.querySelector('.casa .acao.descontos')).display === 'none'`); err != nil {
		t.Fatalf("timeouts in extra time: %v", err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#periodoBotao').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#periodos').open && document.querySelector('#duracao').textContent === "05:00" && [...document.querySelectorAll('[data-length]')].every((b) => b.disabled)`); err != nil {
		got, _ := chromedp.Run(tab, chromedp.Evaluate[string](`document.querySelector('#duracao').textContent`))
		t.Fatalf("the period sheet in extra time: %v (length %q)", err, got)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#periodosFechar').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('#relogio').click(), true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#acertarRepor').textContent.includes("05:00")`); err != nil {
		t.Fatalf("Repor in extra time: %v", err)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

// Basketball's page: the penalty word, timeouts, the clock sheet, the undo row and "Período anterior".
func TestMarcarPageBasketballKeepsBonus(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "basketball")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t)
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.EmulateViewport(844, 390), chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	tap := func(sel string) {
		t.Helper()
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool]("document.querySelector(`"+sel+"`).click(), true")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(400 * time.Millisecond) // the page ignores the same tap again for 350 ms after its reply
	}
	if err := until(tab, `[...document.querySelectorAll('.casa .somar button')].filter((b) => getComputedStyle(b).display !== 'none').length === 3`); err != nil {
		t.Fatalf("basketball lost its +2 or +3: %v", err)
	}
	if err := until(tab, `!document.body.classList.contains('consola') && document.querySelector('#anular').disabled && document.querySelector('#anularTexto').textContent === "Anular"`); err != nil {
		t.Fatalf("the undo row with nothing to undo: %v", err)
	}

	// With the clock stopped a step moves it at once, and Repor puts back the period's 10:00.
	tap(`#relogio`)
	// Each column of steps is one unit, minus above plus.
	if err := until(tab, `[...document.querySelectorAll('#acertar .passos [data-tenths]')].map((b) => b.dataset.tenths).join() === "-600,-100,-10,600,100,10"`); err != nil {
		t.Errorf("the clock steps are out of column order: %v", err)
	}
	tap(`#acertar [data-tenths="-100"]`)
	if err := until(tab, `document.querySelector('#tempo').textContent === "09:50"`); err != nil {
		t.Fatalf("−10 s: %v", err)
	}
	if c := s.Manual.Score().Clock; c != 5900 {
		t.Errorf("after −10 s the scorer's clock is %d", c)
	}
	tap(`#acertarRepor`)
	if err := until(tab, `document.querySelector('#tempo').textContent === "10:00"`); err != nil {
		t.Fatalf("Repor: %v", err)
	}
	if c := s.Manual.Score().Clock; c != 6000 {
		t.Errorf("after Repor the scorer's clock is %d", c)
	}
	tap(`#acertarFechar`)

	// The undo row names the tap and the visitor in full at 16 px; a long name goes to a second line at 844 px.
	fits := `(() => { const u = document.querySelector('#anular'), e = document.querySelector('#anularTexto'), q = e.querySelector('.eq');
		const two = u.classList.contains('duas');
		return getComputedStyle(e).fontSize === "16px" && e.scrollWidth <= e.clientWidth && u.offsetHeight >= 44 &&
			(!two || (getComputedStyle(q).display === 'block' && q.scrollWidth <= q.clientWidth && getComputedStyle(e.querySelector('.sep')).display === 'none')); })()`
	tap(`.fora .acao[data-do="foul"]`)
	if err := until(tab, `!document.querySelector('#anular').disabled && document.querySelector('#anularTexto').textContent === "Anular falta · VISITANTE" && `+fits); err != nil {
		t.Fatalf("the undo row after a visitor foul: %v", err)
	}
	game := map[string]any{"group_id": "seniores", "away": map[string]string{"name": "ACADÉMICA SUL", "short": "ACA"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, s, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	tap(`.fora .acao[data-do="timeout"]`)
	if err := until(tab, `document.querySelector('#anularTexto').textContent === "Anular desconto · ACADÉMICA SUL" && document.querySelector('#anular').classList.contains('duas') && `+fits); err != nil {
		size, _ := chromedp.Run(tab, chromedp.Evaluate[string](`(() => { const e = document.querySelector('#anularTexto'); return e.scrollWidth + "/" + e.clientWidth + " " + getComputedStyle(e).fontSize + " " + document.querySelector('#anular').className; })()`))
		t.Fatalf("a twelve-letter name in the undo row: %v (%s)", err, size)
	}

	// Timeouts show what is left of the half's allowance.
	if err := until(tab, `document.querySelector('[data-v="homeTimeoutsLeft"]').textContent === "2" && document.querySelector('[data-v="homeTimeoutsWord"]').textContent === "restam"`); err != nil {
		t.Fatalf("basketball timeouts at the start: %v", err)
	}
	if err := s.Manual.Do(source.Action{Do: "timeout", Team: "home", N: 1}); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('[data-v="homeTimeoutsLeft"]').textContent === "1" && document.querySelector('[data-v="homeTimeoutsWord"]').textContent === "resta"`); err != nil {
		t.Fatalf("basketball timeouts after one: %v", err)
	}
	fouls(t, s, "home", 4)
	if err := until(tab, `document.querySelector('.casa .faltas').hasAttribute("data-warn") && !document.querySelector('.casa .faltas').hasAttribute("data-bonus")`); err != nil {
		t.Fatalf("basketball at four fouls: %v", err)
	}
	fouls(t, s, "home", 1)
	if err := until(tab, `document.querySelector('.casa .faltas').hasAttribute("data-bonus") && !document.querySelector('.casa .faltas').hasAttribute("data-warn") && document.querySelector('.casa .aviso').textContent === "Bónus"`); err != nil {
		t.Fatalf("basketball at five fouls: %v", err)
	}
	if err := s.Manual.Do(source.Action{Do: "timeout", Team: "home", N: 1}); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('[data-v="homeTimeoutsLeft"]').textContent === "0" && document.querySelector('[data-v="homeTimeoutsWord"]').textContent === "restam"`); err != nil {
		t.Fatalf("basketball timeouts all spent: %v", err)
	}
	if err := until(tab, `document.querySelector("#periodo").textContent.includes("período")`); err != nil {
		t.Fatalf("basketball period word: %v", err)
	}

	// After the next period, "Período anterior" goes back to the first behind a confirmation.
	if err := s.Manual.Do(source.Action{Do: "period_next"}); err != nil {
		t.Fatal(err)
	}
	tap(`#periodoBotao`)
	if err := until(tab, `document.querySelector('#periodos').open && document.querySelector('#periodo').textContent.includes("2") && !document.querySelector('#anterior').disabled`); err != nil {
		t.Fatalf("Período anterior in period 2: %v", err)
	}
	tap(`#anterior`)
	if err := until(tab, `!document.querySelector('#periodos').open && document.querySelector('#confirmar').open`); err != nil {
		t.Fatalf("no confirmation for Período anterior: %v", err)
	}
	if p := s.Manual.Score().Period; p != 2 {
		t.Fatalf("the period changed before the confirmation: %d", p)
	}
	tap(`#confirmarSim`)
	eventually(t, func() bool { return s.Manual.Score().Period == 1 }, "period 1 again")
	if err := until(tab, `document.querySelector('#periodo').textContent.includes("1")`); err != nil {
		t.Fatalf("the page after period_back: %v", err)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

// The scoreboard's foul pips and penalty word per sport, read from the page.
func TestOverlayFoulPips(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	// Per team: pips drawn, whether they are hot, whether the cell is in penalty, and the penalty word.
	probe := `JSON.stringify(["home", "away"].map((tx) => {
		const p = document.querySelector('.sb-fouls [data-pips="' + tx + '"]'), c = document.querySelector('.sb-fouls [data-bonus="' + tx + '"]');
		return [p.children.length, p.classList.contains("hot"), c.classList.contains("bonus"), c.querySelector(".bonus").textContent];
	}))`
	for _, c := range []struct{ fixture, want string }{
		{"futsal/marcador-faltas", `[[5,true,true,"LIVRE"],[5,true,false,"LIVRE"]]`},
		{"futsal/prolongamento-faltas", `[[5,true,true,"LIVRE"],[5,false,false,"LIVRE"]]`},
		{"marcador", `[[4,false,false,"BÓNUS"],[4,true,false,"BÓNUS"]]`},
		{"marcador-bonus", `[[4,false,false,"BÓNUS"],[4,true,true,"BÓNUS"]]`},
	} {
		t.Run(c.fixture, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/overlay/fixtures/"+c.fixture+".json")); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, `document.documentElement.dataset.screen === "marcador" && document.querySelector('.sb-fouls .bonus').textContent !== ""`); err != nil {
				t.Fatal(err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](probe))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("pips, hot, bonus, word:\n got  %s\n want %s", got, c.want)
			}
		})
	}
}
