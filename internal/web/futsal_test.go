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

// The scorer's page for futsal: halves, five foul marks, "Livre" at the sixth
// foul, and "Repor" gives extra time its own 5:00.
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
	if err := until(tab, `document.querySelectorAll('[data-marks="homeFouls"] i').length === 5`); err != nil {
		t.Fatalf("not five foul marks: %v", err)
	}
	// A goal is 1: the page shows only +1, and the server refuses a bigger one.
	if err := until(tab, `[...document.querySelectorAll('.casa .somar button')].filter((b) => getComputedStyle(b).display !== 'none').length === 1`); err != nil {
		t.Fatalf("futsal shows more than +1: %v", err)
	}
	fouls(t, s, "home", 5)
	if err := until(tab, `document.querySelector('[data-v="homeFouls"]').textContent === "5" && !document.querySelector('.casa .faltas').hasAttribute("data-bonus")`); err != nil {
		t.Fatalf("at five fouls: %v", err)
	}
	fouls(t, s, "home", 1)
	if err := until(tab, `document.querySelector('.casa .faltas').hasAttribute("data-bonus") && document.querySelector('.casa .aviso').textContent === "Livre"`); err != nil {
		t.Fatalf("at six fouls: %v", err)
	}
	// Through the page's own arrow and confirm dialog, so its score follows.
	for i, want := range []string{"2", "Prolongamento"} {
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('[data-ask="period_next"]').click(), true`)); err != nil {
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

// Basketball's page keeps its own word at the penalty.
func TestMarcarPageBasketballKeepsBonus(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "basketball")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/marcar")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `[...document.querySelectorAll('.casa .somar button')].filter((b) => getComputedStyle(b).display !== 'none').length === 3`); err != nil {
		t.Fatalf("basketball lost its +2 or +3: %v", err)
	}
	fouls(t, s, "home", 5)
	if err := until(tab, `document.querySelector('.casa .faltas').hasAttribute("data-bonus") && document.querySelector('.casa .aviso').textContent === "Bónus"`); err != nil {
		t.Fatalf("basketball at five fouls: %v", err)
	}
	if err := until(tab, `document.querySelector("#periodo").textContent.includes("período")`); err != nil {
		t.Fatalf("basketball period word: %v", err)
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
