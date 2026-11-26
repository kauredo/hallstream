package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

// These tests load the overlay, which runs live in OBS, and the panel in
// headless Chrome through the real server. A machine with no Chrome skips them.

const (
	screenIs = `document.documentElement.dataset.screen === %q`
	dotOn    = `document.documentElement.classList.contains("off")`
	done     = "overlay test done"
)

// chrome starts a headless Chrome and returns a tab in it.
func chrome(t *testing.T) context.Context {
	t.Helper()
	// No sandbox: CI runners and containers often cannot give Chrome one, and
	// the only pages opened here are this program's own.
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.NoSandbox, chromedp.WindowSize(1920, 1080))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancel)
	tab, cancel := chromedp.NewContext(alloc)
	t.Cleanup(cancel)
	if err := chromedp.Do(tab); errors.Is(err, exec.ErrNotFound) {
		t.Skip("no Chrome on this machine")
	} else if err != nil {
		t.Fatal(err)
	}
	// The browser lives as long as tab, so the time limit goes on a child.
	limited, cancel := context.WithTimeout(tab, 2*time.Minute)
	t.Cleanup(cancel)
	return limited
}

// until waits for a JavaScript expression to turn true in the tab.
func until(tab context.Context, expr string) error {
	return chromedp.Do(tab, chromedp.Poll[chromedp.Void](expr,
		chromedp.WithPollingInterval(50*time.Millisecond), chromedp.WithPollingTimeout(20*time.Second)))
}

// problems reads what the page wrote to the console since messages was
// opened and returns the lines for which bad says so.
func problems(t *testing.T, tab context.Context, messages iter.Seq2[chromedp.ConsoleMessage, error], bad func(chromedp.ConsoleMessage) bool) []string {
	t.Helper()
	if err := chromedp.Do(tab, chromedp.Evaluate[chromedp.Void](fmt.Sprintf("console.debug(%q)", done))); err != nil {
		t.Fatal(err)
	}
	var out []string
	for m, err := range messages {
		if err != nil {
			t.Fatal(err)
		}
		if m.Text == done {
			break
		}
		if bad(m) {
			out = append(out, m.String())
		}
	}
	return out
}

func TestOverlayRendersEveryFixture(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	names, err := fs.Glob(assets.FS, "overlay/fixtures/*.json")
	if err != nil || len(names) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	// Sports with their own screens keep their fixtures in a folder.
	more, err := fs.Glob(assets.FS, "overlay/fixtures/*/*.json")
	if err != nil || len(more) == 0 {
		t.Fatalf("no fixtures in the sport folders: %v", err)
	}
	names = append(names, more...)
	for _, name := range names {
		t.Run(strings.TrimSuffix(strings.TrimPrefix(name, "overlay/fixtures/"), ".json"), func(t *testing.T) {
			data, _ := fs.ReadFile(assets.FS, name)
			var st struct{ Screen struct{ Current string } }
			if err := json.Unmarshal(data, &st); err != nil || st.Screen.Current == "" {
				t.Fatalf("fixture has no screen.current: %v", err)
			}
			messages := chromedp.Console(tab)
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/"+name)); err != nil {
				t.Fatal(err)
			}
			// render sets data-screen last, so a state it chokes on never gets there.
			if err := until(tab, fmt.Sprintf(screenIs, st.Screen.Current)+` && [...document.images].every((i) => i.complete)`); err != nil {
				got, _ := chromedp.Run(tab, chromedp.Evaluate[string](`document.documentElement.dataset.screen || ""`))
				t.Errorf("data-screen is %q, want %q (%v)", got, st.Screen.Current, err)
			}
			errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool {
				return m.Type == chromedp.ConsoleError || m.IsException()
			})
			if len(errs) > 0 {
				t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
			}
		})
	}
}

// Every style draws every screen kind without an error, and puts its name on <html>.
func TestOverlayStyles(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	for _, style := range store.Styles {
		for _, fx := range []string{"marcador", "marcador-12", "rodape-fotos", "rodape-comentador", "abertura", "plantel", "desconto", "intervalo", "final", "apoiantes", "proximo-casa", "voltamos", "volleyball/marcador", "volleyball/final", "futsal/marcador", "futsal/marcador-faltas", "futsal/intervalo", "futsal/final-prolongamento", "handball/marcador-exclusoes", "handball/final", "soccer/marcador-cartoes", "soccer/rodape-substituicao"} {
			t.Run(style+"/"+fx, func(t *testing.T) {
				messages := chromedp.Console(tab)
				if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style="+style+"&state=/overlay/fixtures/"+fx+".json")); err != nil {
					t.Fatal(err)
				}
				if err := until(tab, `document.documentElement.dataset.style === "`+style+`" && document.documentElement.dataset.screen !== undefined`); err != nil {
					t.Fatal(err)
				}
				if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
					t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
				}
			})
		}
	}
}

// SYSTEM-R8 took the strips from under the emblem tiles; the scoreboard rows
// left a grey stub under the visitor's in every look.
func TestOverlayScoreboardTilesHaveNoStrip(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	for _, style := range store.Styles {
		t.Run(style, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style="+style+"&state=/overlay/fixtures/marcador.json")); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, `document.documentElement.dataset.style === "`+style+`" && document.querySelector('.sb-row .tile[data-tile="away"]') !== null`); err != nil {
				t.Fatal(err)
			}
			for _, side := range []string{"home", "away"} {
				marked, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('.sb-row .tile[data-tile="`+side+`"]').hasAttribute('data-strip')`))
				if err != nil {
					t.Fatal(err)
				}
				if !marked {
					t.Fatalf("the %s tile has no data-strip, so the check below would pass for nothing", side)
				}
				display, err := chromedp.Run(tab, chromedp.Evaluate[string](`getComputedStyle(document.querySelector('.sb-row .tile[data-tile="`+side+`"]'), '::after').display`))
				if err != nil {
					t.Fatal(err)
				}
				if display != "none" {
					t.Errorf("the %s tile has a strip: display %s", side, display)
				}
			}
		})
	}
}

// The overlay names the game's sport on <html>, and only basketball gets the court-lined stand-in hall.
func TestOverlaySportArt(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	for fx, want := range map[string][2]string{
		"marcador":            {"basketball", "pavilhao-demo.jpg"},
		"volleyball/marcador": {"volleyball", "pavilhao-neutro.svg"},
	} {
		t.Run(fx, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style=pavilhao&state=/overlay/fixtures/"+fx+".json")); err != nil {
				t.Fatal(err)
			}
			cond := `document.documentElement.dataset.sport === "` + want[0] + `" && getComputedStyle(document.documentElement).getPropertyValue("--hall").includes("` + want[1] + `")`
			if err := until(tab, cond); err != nil {
				t.Fatalf("want sport %s with hall %s: %v", want[0], want[1], err)
			}
		})
	}
}

// Wherever the club puts the bug and the lower third, at the largest size and the smallest
// margin, in every style: both stay inside the frame and never cover each other, and the
// credit and the sponsor corner only show where they cover neither.
func TestOverlayPlaces(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	const check = `(() => {
		const r = (s) => { const e = document.querySelector(s); if (!e || getComputedStyle(e).visibility === 'hidden') return null; const b = e.getBoundingClientRect(); return b.width ? b : null; };
		const over = (a, b) => a && b && a.left < b.right - 1 && b.left < a.right - 1 && a.top < b.bottom - 1 && b.top < a.bottom - 1;
		const inside = (a) => !a || (a.left >= 0 && a.top >= 0 && a.right <= 1920 && a.bottom <= 1080);
		const bug = r('.sb'), lt = r('.lt-body'), spc = r('#spc'), cr = document.querySelector('#credit.on') && r('#credit');
		const bad = [];
		if (!bug) bad.push('no bug');
		if (!inside(bug)) bad.push('bug out of frame');
		if (!inside(lt)) bad.push('lower third out of frame');
		if (over(bug, lt)) bad.push('bug covers lower third');
		if (over(cr, bug) || over(cr, lt)) bad.push('credit covers something');
		if (over(spc, bug)) bad.push('sponsor corner covers the bug');
		return bad.join(', ');
	})()`
	for _, style := range store.Styles {
		for _, bug := range store.BugPositions {
			for _, side := range store.LowerThirds {
				t.Run(style+"/"+bug+"/"+side, func(t *testing.T) {
					u := srv.URL + "/overlay?anim=0&style=" + style + "&bug=" + bug + "&lt=" + side + "&scale=125&margin=32&state=/overlay/fixtures/rodape-fotos.json"
					if err := chromedp.Do(tab, chromedp.Navigate(u)); err != nil {
						t.Fatal(err)
					}
					if err := until(tab, `document.documentElement.dataset.bugX !== undefined && [...document.images].every((i) => i.complete)`); err != nil {
						t.Fatal(err)
					}
					bad, err := chromedp.Run(tab, chromedp.Evaluate[string](check))
					if err != nil {
						t.Fatal(err)
					}
					if bad != "" {
						t.Error(bad)
					}
				})
			}
		}
	}
}

// The credit shows where the bottom-right corner is free, and only with
// game.credit on (or missing, from an older program).
func TestOverlayCredit(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	for name, want := range map[string]bool{
		"credito-ligado":    true,
		"credito-desligado": false,
		"marcador":          true,  // no credit key
		"rodape-baixo":      false, // the lower third has the corner
		"rodape-jogador":    true,  // the lower third is bottom-left
		"nada":              false,
		"intervalo":         false, // the sponsor strip fills the bottom
		"desconto":          false,
		"apoiantes":         true,
	} {
		t.Run(name, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/overlay/fixtures/"+name+".json")); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, `document.documentElement.dataset.screen !== undefined`); err != nil {
				t.Fatal(err)
			}
			on, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector("#credit").classList.contains("on")`))
			if err != nil {
				t.Fatal(err)
			}
			if on != want {
				t.Errorf("credit shown %v, want %v", on, want)
			}
		})
	}
}

func TestOverlayDotWhenTheProgramStops(t *testing.T) {
	tab := chrome(t)
	s := newServerFiles(t, assets.FS)
	s.State.SetBox(true) // a lost box turns the same dot on
	serve := func(addr string) (*http.Server, string) {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		program := &http.Server{Handler: s}
		go program.Serve(l)
		t.Cleanup(func() { program.Close() })
		return program, l.Addr().String()
	}
	program, addr := serve("127.0.0.1:0")
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate("http://"+addr+"/overlay")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, fmt.Sprintf(screenIs, "marcador")); err != nil {
		t.Fatalf("no state from the event stream: %v", err)
	}
	isOn := func() bool {
		on, err := chromedp.Run(tab, chromedp.Evaluate[bool](dotOn))
		if err != nil {
			t.Fatal(err)
		}
		return on
	}
	if isOn() {
		t.Fatal("the dot is on while the program runs")
	}

	program.Close()
	time.Sleep(3 * time.Second)
	if isOn() {
		t.Error("the dot came on 3 s after the program stopped: a quick restart would flash it")
	}
	if err := until(tab, dotOn); err != nil {
		t.Fatalf("the program stopped and the dot never came on: %v", err)
	}

	serve(addr) // the program is back on the same port
	if err := until(tab, "!"+dotOn); err != nil {
		t.Errorf("the program is back and the dot stayed on: %v", err)
	}
	if thrown := problems(t, tab, messages, chromedp.ConsoleMessage.IsException); len(thrown) > 0 {
		t.Errorf("the overlay threw:\n%s", strings.Join(thrown, "\n"))
	}
}

func TestPanelOpensOnAFreshFolder(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t) // after the server, so the panel's event stream ends before the server closes
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel")); err != nil {
		t.Fatal(err)
	}
	// With no escalão the players load at once, so an error at start is
	// already on screen when the Jogo tab is.
	if err := until(tab, `document.querySelector("#jogo .card") !== null`); err != nil {
		t.Fatalf("the Jogo tab never drew: %v", err)
	}
	dialog, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`document.querySelector("#modal").hidden ? "" : document.querySelector("#modal").innerText`))
	if err != nil {
		t.Fatal(err)
	}
	if dialog != "" {
		t.Errorf("the panel opened with a dialog: %q", dialog)
	}
	errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool {
		return m.Type == chromedp.ConsoleError || m.IsException()
	})
	if len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

// Soccer on the scoreboard and in the lower thirds: each team's yellow and red
// counts (a second yellow is both), the added time under the clock, and the
// lower third under the tab of the team the card is for.
func TestOverlaySoccerCards(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	messages := chromedp.Console(tab)
	load := func(fx string) {
		t.Helper()
		if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/overlay/fixtures/soccer/"+fx+".json")); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `document.documentElement.dataset.screen !== undefined`); err != nil {
			t.Fatal(err)
		}
	}
	read := func(js string) string {
		t.Helper()
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](js))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	counts := func(team string) string {
		return read(`[...document.querySelectorAll('[data-cds="` + team + `"] .cd')].map((e) => e.className.replace('cd ', '') + e.textContent).join(' ')`)
	}

	load("marcador-cartoes")
	if got := read(`document.documentElement.dataset.penalties`); got != "cards" {
		t.Errorf("data-penalties %q, want cards", got)
	}
	// Home: yellow 4, yellow 9, second yellow 9, red 2 = 3 yellows and 2 reds; away: one yellow.
	if got := counts("home"); got != "y3 r2" {
		t.Errorf("home counts %q, want \"y3 r2\"", got)
	}
	if got := counts("away"); got != "y1" {
		t.Errorf("away counts %q, want \"y1\" (a red at 0 is not drawn)", got)
	}
	if got := read(`(document.querySelector('[data-add]').classList.contains('blank') ? '' : document.querySelector('[data-add] b').textContent)`); got != "90+4'" {
		t.Errorf("added time %q, want 90+4'", got)
	}
	load("marcador")
	if got := counts("home") + counts("away"); got != "" {
		t.Errorf("counts with no cards: %q", got)
	}
	if got := read(`document.querySelector('[data-add]').classList.contains('blank') ? 'blank' : 'shown'`); got != "blank" {
		t.Errorf("added time without stoppage is %s", got)
	}

	// A home card draws the home tab, an away card the visitor's.
	load("rodape-cartao")
	if got := read(`document.querySelector('.lt-team').textContent + '|' + document.querySelector('.lt-tc').dataset.tile + '|' + document.querySelector('#lt').className`); !strings.HasPrefix(got, "CASA|home|") || !strings.Contains(got, "segundo") {
		t.Errorf("home card tab %q", got)
	}
	load("rodape-cartao-fora")
	if err := until(tab, `document.querySelector('.lt-team').textContent === "VISITANTE"`); err != nil {
		t.Fatalf("the away card's tab names %q: %v", read(`document.querySelector('.lt-team').textContent`), err)
	}
	got := read(`document.querySelector('.lt-tc').dataset.tile + '|' + document.querySelector('.lt-tc .ini').textContent + '|' + getComputedStyle(document.querySelector('.lt-tc')).backgroundColor + '|' + document.querySelector('.lt-label').textContent + '|' + document.querySelector('.lt-big').textContent`)
	away := strings.Split(got, "|")
	home := read(`getComputedStyle(document.querySelector('.tile[data-tile="home"]')).backgroundColor`)
	if len(away) != 5 || away[0] != "away" || away[1] != "VIS" || away[2] == home || away[3] != "CARTÃO VERMELHO" || away[4] != "VISITANTE" {
		t.Errorf("away card: tile %q (home tile %q)", got, home)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool {
		return m.Type == chromedp.ConsoleError || m.IsException()
	}); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

// A web font loads when text first uses it, so a state that arrives after
// document.fonts.ready resolved asks for fonts that land later. Names fitted with the fallback font must be
// fitted again, or two renders of the same state differ by the load order.
func TestOverlayRefitsNamesWhenAFontLandsLate(t *testing.T) {
	tab := chrome(t)
	for _, style := range []string{"classico", "cor", "pavilhao"} {
		t.Run(style, func(t *testing.T) {
			// A new server per look, so the fonts are fetched again. The state
			// is held back until the first font has been served, then a beat
			// longer, so it always lands after fonts.ready.
			s := newServerFiles(t, assets.FS)
			var once sync.Once
			fontServed := make(chan struct{})
			var stateServed atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, ".woff2"):
					time.Sleep(300 * time.Millisecond)
					s.ServeHTTP(w, r)
					once.Do(func() { close(fontServed) })
					return
				case strings.Contains(r.URL.Path, "fixtures"):
					select {
					case <-fontServed:
					case <-time.After(10 * time.Second):
					}
					time.Sleep(300 * time.Millisecond)
					defer stateServed.Store(true)
				}
				s.ServeHTTP(w, r)
			}))
			t.Cleanup(srv.Close)
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style="+style+"&state=/overlay/fixtures/marcador-edge.json")); err != nil {
				t.Fatal(err)
			}
			const fitted = `document.fonts.status === "loaded" && document.documentElement.dataset.screen === "marcador" && [...document.querySelectorAll("[data-fit]")].filter(e => e.textContent).every(e => {
				const size = parseFloat(e.style.fontSize), min = Math.round(e.dataset.fit * 0.55);
				const fits = e.scrollWidth <= e.clientWidth + 0.5;
				e.style.fontSize = (size + 1) + "px";
				const roomLeft = e.scrollWidth <= e.clientWidth + 0.5;
				e.style.fontSize = size + "px";
				return (fits || size <= min) && (!roomLeft || size >= e.dataset.fit);
			})`
			for deadline := time.Now().Add(15 * time.Second); !stateServed.Load(); time.Sleep(20 * time.Millisecond) {
				if time.Now().After(deadline) {
					t.Fatal("the page never asked for the state")
				}
			}
			if err := until(tab, fitted); err != nil {
				t.Errorf("a name is not fitted to the fonts that landed after the state: %v", err)
			}
		})
	}
}

// Placa draws each team's timeouts under its name and Pavilhão under its crest, filled for each one
// left and outlined for each taken, only from the phone scorer: a console always sends 0 taken.
// Placa's away marks turn ink when the away colour is too light for the white plate, and the other
// looks never show them.
func TestOverlayTimeoutMarks(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	const marks = `JSON.stringify(['home', 'away'].map((tm) => {
		const box = document.querySelector('[data-tos="' + tm + '"]');
		return [getComputedStyle(box).display, box.children.length, box.querySelectorAll('i.on').length, getComputedStyle(box).color];
	}))`
	for _, c := range []struct{ style, fixture, want string }{
		{"placa", "marcador-descontos", `[["flex",2,2,"rgb(47, 75, 124)"],["flex",2,1,"rgb(26, 31, 35)"]]`},
		{"placa", "marcador-descontos-claro", `[["flex",2,1,"rgb(47, 75, 124)"],["flex",2,0,"rgb(26, 31, 35)"]]`},
		{"placa", "marcador", `[["none",0,0,"rgb(255, 255, 255)"],["none",0,0,"rgb(255, 255, 255)"]]`},
		{"pavilhao", "marcador-descontos", `[["flex",2,2,"rgb(255, 255, 255)"],["flex",2,1,"rgb(255, 255, 255)"]]`},
		{"pavilhao", "marcador", `[["none",0,0,"rgb(255, 255, 255)"],["none",0,0,"rgb(255, 255, 255)"]]`},
		{"classico", "marcador-descontos", `[["none",2,2,"rgb(255, 255, 255)"],["none",2,1,"rgb(255, 255, 255)"]]`},
	} {
		t.Run(c.style+"/"+c.fixture, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style="+c.style+"&state=/overlay/fixtures/"+c.fixture+".json")); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, fmt.Sprintf(screenIs, "marcador")); err != nil {
				t.Fatal(err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](marks))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("timeout marks %s, want %s", got, c.want)
			}
		})
	}
}

// With the box gone the grey dot shows on the scoreboard, on top of every cell.
func TestOverlayDeadLinkDotShows(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	// The overlay takes no pointer events and the hidden screens lie over it, transparent; the probe
	// turns pointer events on and looks for the topmost element of the scoreboard screen.
	const onTop = `(() => {
		if (!document.getElementById('probe')) document.head.insertAdjacentHTML('beforeend', '<style id="probe">* { pointer-events: auto !important; }</style>');
		const scr = document.getElementById('s-marcador'), dot = scr.querySelector('.sb-dot'), r = dot.getBoundingClientRect();
		const top = document.elementsFromPoint(r.left + r.width / 2, r.top + r.height / 2).find((e) => scr.contains(e) && e !== scr);
		return r.width > 0 && getComputedStyle(dot).opacity === '1' && top === dot;
	})()`
	for _, style := range []string{"classico", "lombada", "cor", "placa", "pavilhao"} {
		t.Run(style, func(t *testing.T) {
			if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&style="+style+"&state=/overlay/fixtures/marcador-edge.json")); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, fmt.Sprintf(screenIs, "marcador")+" && "+dotOn); err != nil {
				t.Fatal(err)
			}
			if err := until(tab, onTop); err != nil {
				t.Errorf("the dead-link dot is hidden: %v", err)
			}
		})
	}
}
