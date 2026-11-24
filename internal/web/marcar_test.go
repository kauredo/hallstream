package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/source"
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
