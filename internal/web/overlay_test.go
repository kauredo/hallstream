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
	"path"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

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
	for _, name := range names {
		t.Run(strings.TrimSuffix(path.Base(name), ".json"), func(t *testing.T) {
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
