package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

// The scorer's page for handball: a clock that counts up in MM:SS, "Período
// seguinte" only at the end of each half, and the suspension chips: +2' adds one
// with its time, tapping it opens the number field and "Acabou", and a team holds
// three.
func TestMarcarPageHandball(t *testing.T) {
	s := sportServer(t, newServerFiles(t, assets.FS), "handball")
	for _, a := range []source.Action{{Do: "period_length", Tenths: 18000}, {Do: "new_game"}} {
		if err := s.Manual.Do(a); err != nil {
			t.Fatal(err)
		}
	}
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
	if err := until(tab, `!document.querySelector("#mesa").hidden && document.querySelector("#periodo").textContent.includes("parte")`); err != nil {
		t.Fatalf("the period is not a half: %v", err)
	}
	if err := until(tab, `getComputedStyle(document.querySelector('.casa .somar button:nth-child(2)')).display === 'none'`); err != nil {
		t.Errorf("+2 is on a count-up sport's page: %v", err)
	}

	// The clock reads MM:SS, and the first half's end is 30:00.
	if err := s.Manual.Do(source.Action{Do: "clock_set", Tenths: 10200}); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#relogio').textContent === "17:00" && document.querySelector('#arrancar').textContent === "Iniciar"`); err != nil {
		t.Fatalf("mid-half: %v", err)
	}
	if err := s.Manual.Do(source.Action{Do: "clock_set", Tenths: 18000}); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#relogio').textContent === "30:00" && document.querySelector('#arrancar').textContent === "Período seguinte"`); err != nil {
		t.Fatalf("at the end of the first half: %v", err)
	}
	if err := until(tab, `document.querySelector('#arrancar').disabled === false`); err != nil {
		t.Fatalf("the button never woke after its wait: %v", err)
	}

	// Through the page's own arrow and confirm dialog: the second half starts at 30:00, which is not its end.
	if err := run(`document.querySelector('[data-ask="period_next"]').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#confirmar').open`); err != nil {
		t.Fatal(err)
	}
	if err := run(`document.querySelector('#confirmarSim').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector('#confirmar').open && document.querySelector("#periodo").textContent.includes("2")`); err != nil {
		t.Fatalf("the second half: %v", err)
	}
	if err := until(tab, `document.querySelector('#relogio').textContent === "30:00" && document.querySelector('#arrancar').textContent === "Iniciar"`); err != nil {
		t.Fatalf("the second half's first frame offers %q", textOf(t, tab, `#arrancar`))
	}
	if err := run(`document.querySelector('#relogio').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#acertarRepor').textContent.includes("30:00")`); err != nil {
		t.Fatalf("Repor in the second half: %v", err)
	}
	if err := run(`document.querySelector('#acertar').close(), true`); err != nil {
		t.Fatal(err)
	}
	if err := s.Manual.Do(source.Action{Do: "clock_set", Tenths: 36000}); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#relogio').textContent === "60:00" && document.querySelector('#arrancar').textContent === "Período seguinte"`); err != nil {
		t.Fatalf("at the end of the second half: %v", err)
	}

	// Extra time starts at the 60:00 played, and Repor says so.
	time.Sleep(400 * time.Millisecond) // the page ignores the same tap again for 350 ms after its reply
	if err := run(`document.querySelector('[data-ask="period_next"]').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#confirmar').open`); err != nil {
		t.Fatal(err)
	}
	if err := run(`document.querySelector('#confirmarSim').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector('#confirmar').open && document.querySelector("#periodo").textContent.includes("Prolongamento")`); err != nil {
		t.Fatalf("extra time: %v (page %q, scorer period %d, confirm %q, toast %q)", err, textOf(t, tab, "#periodo"), s.Manual.Score().Period, textOf(t, tab, "#confirmarTitulo"), textOf(t, tab, "#faixa"))
	}
	if err := run(`document.querySelector('#relogio').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector('#acertarRepor').textContent.includes("60:00")`); err != nil {
		t.Fatalf("Repor in extra time: %v", err)
	}
	if err := run(`document.querySelector('#acertar').close(), true`); err != nil {
		t.Fatal(err)
	}

	// Suspensions: +2' adds a chip with its time.
	tap(`.casa [data-do="suspension"]`)
	if err := until(tab, `document.querySelectorAll('.casa .exc').length === 1 && document.querySelector('.casa .exc span').textContent === "02:00"`); err != nil {
		t.Fatalf("no chip after +2': %v", err)
	}
	if err := until(tab, `document.querySelector('.casa [data-do="suspension"]').getAttribute('aria-label').startsWith("Exclusão de 2 minutos, ")`); err != nil {
		t.Errorf("the +2' button has no label: %v", err)
	}
	// The chip opens the number field; "Feito" shows the number on it.
	tap(`.casa .exc`)
	if err := until(tab, `document.querySelector('#exclusao').open`); err != nil {
		t.Fatalf("the chip opened no dialog: %v", err)
	}
	if err := run(`(document.querySelector('#exclusaoNumero').value = "7", document.querySelector('#exclusaoFeito').click(), true)`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector('#exclusao').open && document.querySelector('.casa .exc b').textContent === "7"`); err != nil {
		t.Fatalf("the number did not show: %v", err)
	}
	// "Acabou" removes it.
	tap(`.casa .exc`)
	if err := until(tab, `document.querySelector('#exclusao').open`); err != nil {
		t.Fatal(err)
	}
	if err := run(`document.querySelector('#exclusaoAcabou').click(), true`); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector('#exclusao').open && document.querySelectorAll('.casa .exc').length === 0`); err != nil {
		t.Fatalf("Acabou left the chip: %v", err)
	}
	// Four taps: three chips and the full-toast text.
	// Each chip is waited for before the next tap: the page's 350 ms guard against a
	// double tap starts at the server's reply, which a slow runner delays.
	for i := 1; i <= 3; i++ {
		tap(`.casa [data-do="suspension"]`)
		if err := until(tab, fmt.Sprintf(`document.querySelectorAll('.casa .exc').length === %d`, i)); err != nil {
			t.Fatalf("tap %d: %v", i, err)
		}
		time.Sleep(400 * time.Millisecond)
	}
	tap(`.casa [data-do="suspension"]`)
	if err := until(tab, `document.querySelectorAll('.casa .exc').length === 3 && !document.querySelector('#faixa').hidden && document.querySelector('#faixa').textContent.includes("3 exclusões")`); err != nil {
		t.Fatalf("four taps: %v (chips %d, toast %q)", err, len(s.Manual.Score().HomeSusp), textOf(t, tab, `#faixa`))
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool {
		// The fourth tap is refused with a 400, which the browser logs.
		return (m.Type == chromedp.ConsoleError && !strings.Contains(m.String(), "400")) || m.IsException()
	}); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

func textOf(t *testing.T, tab context.Context, sel string) string {
	t.Helper()
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](`(document.querySelector(`+"`"+sel+"`"+`) || {}).textContent || ""`))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// handballState is the handball fixture with live.suspensions replaced by list:
// a JSON text, or "" to leave the key out.
func handballState(t *testing.T, list string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../web/overlay/fixtures/handball/marcador-exclusoes.json")
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	live := st["live"].(map[string]any)
	delete(live, "suspensions")
	if list != "" {
		var v any
		if err := json.Unmarshal([]byte(list), &v); err != nil {
			t.Fatal(err)
		}
		live["suspensions"] = v
	}
	out, _ := json.Marshal(st)
	return out
}

// What the overlay's suspensions cell says for each team, as read off the page.
type cellText struct{ Count, Time, Number, Soon, Text string }

const readCells = `JSON.stringify(["home", "away"].map((t) => {
	const box = document.querySelector('[data-susp="' + t + '"]');
	const q = (s) => box.querySelector(s);
	return { Count: q('.out b') ? q('.out b').textContent : '', Time: q('time') ? q('time').textContent : '',
		Number: q('.no') ? q('.no').textContent : '', Soon: q('time.soon') ? 'soon' : '', Text: box.textContent };
}))`

// On handball the scoreboard's cell says how many are out, when the soonest
// returns and, once typed, that player's number; the last 10 seconds are
// tinted; no one out draws nothing; and other sports keep their fouls cells.
func TestOverlaySuspensions(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	var body []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/test-state.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.Handle("/", s)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	tab := chrome(t)
	cells := func(list string) []cellText {
		t.Helper()
		body = handballState(t, list)
		if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/test-state.json")); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `document.documentElement.dataset.penalties === "suspensions" && document.documentElement.dataset.screen !== undefined`); err != nil {
			t.Fatal(err)
		}
		raw, err := chromedp.Run(tab, chromedp.Evaluate[string](readCells))
		if err != nil {
			t.Fatal(err)
		}
		var out []cellText
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := cells(`[{"team":"home","id":1,"number":"7","left":"00:48"},{"team":"home","id":2,"number":"","left":"01:23"},
		{"team":"away","id":3,"number":"11","left":"01:59"},{"team":"away","id":4,"number":"","left":"01:02"},{"team":"away","id":5,"number":"5","left":"00:07"}]`)
	if want := (cellText{"\u22122", "00:48", "#7", "", "\u2212200:48#7"}); got[0] != want {
		t.Errorf("home %+v, want %+v", got[0], want)
	}
	if want := (cellText{"\u22123", "00:07", "#5", "soon", "\u2212300:07#5"}); got[1] != want {
		t.Errorf("away %+v, want %+v", got[1], want)
	}

	// The tint starts at 10 seconds, not 11; the soonest return wins, whatever the order.
	for left, soon := range map[string]string{"00:10": "soon", "00:11": "", "00:01": "soon", "01:00": ""} {
		got := cells(`[{"team":"home","id":1,"number":"","left":"02:00"},{"team":"home","id":2,"number":"","left":"` + left + `"}]`)
		if got[0].Soon != soon || got[0].Time != left || got[0].Count != "\u22122" {
			t.Errorf("left %s: %+v, want soon %q", left, got[0], soon)
		}
	}

	// No one out: nothing in the cell, whether the list is empty, null or missing.
	for name, list := range map[string]string{"empty": `[]`, "null": `null`, "missing": ""} {
		got := cells(list)
		if got[0].Text != "" || got[1].Text != "" {
			t.Errorf("%s list: cells %+v, want nothing", name, got)
		}
	}

	// Basketball keeps its fouls cell: the suspensions cell is hidden and the swap is off.
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&state=/overlay/fixtures/marcador.json")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.documentElement.dataset.screen !== undefined`); err != nil {
		t.Fatal(err)
	}
	ok, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.documentElement.dataset.penalties === undefined &&
		[...document.querySelectorAll('.only-susp')].length > 0 &&
		[...document.querySelectorAll('.only-susp')].every((e) => getComputedStyle(e).display === 'none') &&
		getComputedStyle(document.querySelector('.sb-f .pips')).display !== 'none'`))
	if err != nil || !ok {
		t.Errorf("basketball's fouls cell changed: %v %v", ok, err)
	}
}

// The handball bug, with its wider cell, stays inside the frame in every look and
// corner at the largest size and the smallest margin.
func TestOverlayHandballBugStaysInFrame(t *testing.T) {
	tab := chrome(t)
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	for _, style := range store.Styles {
		for _, bug := range store.BugPositions {
			t.Run(style+"/"+bug, func(t *testing.T) {
				u := srv.URL + "/overlay?anim=0&style=" + style + "&bug=" + bug + "&scale=125&margin=32&state=/overlay/fixtures/handball/marcador-exclusoes.json"
				if err := chromedp.Do(tab, chromedp.Navigate(u)); err != nil {
					t.Fatal(err)
				}
				if err := until(tab, `document.documentElement.dataset.bugX !== undefined && [...document.images].every((i) => i.complete)`); err != nil {
					t.Fatal(err)
				}
				in, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(() => { const b = document.querySelector('.sb').getBoundingClientRect(); return b.width > 0 && b.left >= 0 && b.top >= 0 && b.right <= 1920 && b.bottom <= 1080; })()`))
				if err != nil || !in {
					t.Errorf("the bug is outside the frame: %v %v", in, err)
				}
			})
		}
	}
}
