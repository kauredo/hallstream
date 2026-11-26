package web

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/source"
	assets "github.com/kauredo/hallstream/web"
)

// fitOverflow lists what does not fit the viewport: a page taller than it, a
// button in the team columns or the strip not wholly inside it, suspension
// chips that leave their row or wrap onto a second one, and a pencil less than
// 8 px from the clock's digits.
const fitOverflow = `(() => {
	const H = innerHeight, W = innerWidth, bad = [];
	const h = document.scrollingElement.scrollHeight;
	if (h > H) bad.push("page " + h + " > " + H);
	const name = (b) => (b.id ? "#" + b.id : b.className || b.dataset.do || "button") + " " + JSON.stringify(b.textContent.trim().slice(0, 30));
	for (const b of document.querySelectorAll('.equipa button, .mesa-centro button')) {
		const r = b.getBoundingClientRect();
		if (!r.width || !r.height) continue;
		if (r.top < -0.5 || r.left < -0.5 || r.bottom > H + 0.5 || r.right > W + 0.5) bad.push(name(b) + " at " + [r.left, r.top, r.right, r.bottom].map(Math.round));
	}
	for (const box of document.querySelectorAll('.chips')) {
		const o = box.getBoundingClientRect(), tops = new Set();
		for (const c of box.children) {
			const r = c.getBoundingClientRect();
			tops.add(Math.round(r.top));
			if (r.left < o.left - 0.5 || r.right > o.right + 0.5) bad.push("chip " + name(c) + " outside its row");
		}
		if (tops.size > 1) bad.push(box.dataset.susp + " chips on " + tops.size + " rows");
	}
	const pen = document.querySelector('.lapis').getBoundingClientRect(), range = document.createRange();
	range.selectNodeContents(document.querySelector('#tempo'));
	const time = range.getBoundingClientRect();
	if (pen.width && time.width && pen.left - 8 < time.right && pen.right + 8 > time.left && pen.top - 8 < time.bottom && pen.bottom + 8 > time.top) bad.push("pencil " + [pen.left, pen.top, pen.right, pen.bottom].map(Math.round) + " within 8 px of the time " + [time.left, time.top, time.right, time.bottom].map(Math.round));
	return bad.join("; ");
})()`

// Every sport's page fits a phone on its side (844 x 390) mid-game: nothing
// scrolls and no control is cut, with three suspensions a side and a
// two-line undo row included.
func TestMarcarPageFits(t *testing.T) {
	basket := sportServer(t, newServerFiles(t, assets.FS), "basketball")
	futsal := sportServer(t, newServerFiles(t, assets.FS), "futsal")
	handball := sportServer(t, newServerFiles(t, assets.FS), "handball")
	volley := sportServer(t, newServerFiles(t, assets.FS), "volleyball")
	soccer := soccerScorer(t)
	onManual(t, soccer)
	game := map[string]any{"group_id": "seniores", "away": map[string]string{"name": "ACADÉMICA SUL", "short": "ACA"}, "start": "2026-11-15T18:00:00+00:00"}
	if code := call(t, handball, "PUT", "/api/game", game, nil); code != 200 {
		t.Fatalf("PUT game: %d", code)
	}
	do := func(s *Server, actions ...source.Action) {
		t.Helper()
		for _, a := range actions {
			if err := s.Manual.Do(a); err != nil {
				t.Fatalf("%+v: %v", a, err)
			}
		}
	}
	p := func(team string, n int) source.Action { return source.Action{Do: "point", Team: team, N: n} }

	do(basket, source.Action{Do: "period_next"}, p("home", 3), p("home", 2), p("away", 3), source.Action{Do: "timeout", Team: "away", N: 1})
	fouls(t, basket, "home", 3)
	fouls(t, basket, "away", 5)
	do(futsal, p("home", 1), source.Action{Do: "period_next"}, p("away", 1))
	fouls(t, futsal, "home", 6)
	fouls(t, futsal, "away", 5)
	do(handball, source.Action{Do: "period_next"}, p("home", 1), p("away", 1))
	for _, team := range []string{"home", "away"} {
		for i := 0; i < 3; i++ {
			do(handball, source.Action{Do: "clock_add", Tenths: 50}, source.Action{Do: "suspension", Team: team})
		}
	}
	do(handball, source.Action{Do: "suspension_number", ID: handball.Manual.Score().HomeSusp[1].ID, Number: "77"})
	do(handball, source.Action{Do: "suspension_end", ID: handball.Manual.Score().AwaySusp[2].ID}, source.Action{Do: "suspension", Team: "away"})
	do(volley, p("home", 1), p("away", 1), source.Action{Do: "serve", Team: "home"})
	do(soccer, p("home", 1), source.Action{Do: "period_next"}, source.Action{Do: "clock_set", Tenths: 55500}, source.Action{Do: "stoppage", N: 3},
		source.Action{Do: "card", Team: "away", Number: "6", Kind: "yellow"},
		source.Action{Do: "card", Team: "away", Number: "11", Kind: "yellow"},
		source.Action{Do: "card", Team: "away", Number: "4", Kind: "red"},
		source.Action{Do: "sub", Team: "away", Out: "9", In: "14"})

	urls := map[*Server]string{}
	for _, s := range []*Server{basket, futsal, handball, volley, soccer} {
		srv := httptest.NewServer(s)
		t.Cleanup(srv.Close)
		urls[s] = srv.URL
	}
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.EmulateViewport(844, 390, chromedp.EmulateScale(2), chromedp.EmulateLandscape, chromedp.EmulateMobile, chromedp.EmulateTouch)); err != nil {
		t.Fatal(err)
	}
	const manual = `!document.body.classList.contains("consola") && !document.querySelector('#anular').disabled`
	for _, c := range []struct {
		name  string
		s     *Server
		ready string
	}{
		{"basketball", basket, manual},
		{"futsal", futsal, manual + ` && document.querySelector('.casa .faltas').hasAttribute('data-bonus') && document.querySelector('.fora .faltas').hasAttribute('data-warn')`},
		{"handball", handball, manual + ` && document.querySelectorAll('.exc').length === 6 && document.querySelector('#anular').classList.contains('duas')`},
		{"volleyball", volley, manual},
		{"soccer", soccer, manual + ` && document.querySelectorAll('.fora .reg').length === 3 && document.querySelector('#anular').classList.contains('duas')`},
	} {
		if err := chromedp.Do(tab, chromedp.Navigate(urls[c.s]+"/marcar")); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `!document.querySelector("#mesa").hidden && `+c.ready); err != nil {
			t.Errorf("%s: the page never drew its state: %v", c.name, err)
			continue
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.fonts.ready.then(() => true)`, chromedp.EvalAwaitPromise)); err != nil {
			t.Fatal(err)
		}
		bad, err := chromedp.Run(tab, chromedp.Evaluate[string](fitOverflow))
		if err != nil {
			t.Fatal(err)
		}
		if bad != "" {
			t.Errorf("%s at 844 x 390: %s", c.name, bad)
		}
	}
	if err := chromedp.Do(tab, chromedp.Navigate("about:blank")); err != nil {
		t.Fatal(err)
	}
}
