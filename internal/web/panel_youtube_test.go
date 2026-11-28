package web

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

// With the built-in client, Definições > YouTube is one button; the club's own
// client fields open behind "Usar um cliente Google próprio" and close again.
// Without it, the fields show straight away, as before.
func TestPanelYouTubeBuiltInClient(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&setup=done&yt=builtin#definicoes")); err != nil {
		t.Fatal(err)
	}
	box := `document.querySelector("#definicoes #ytSetup")`
	if err := until(tab, `(b => !!b && !!b.querySelector('[data-act="yt-connect"]:not([disabled])') && !b.querySelector("#ytId") && !!b.querySelector('[data-act="yt-own"]'))(`+box+`)`); err != nil {
		t.Fatalf("one button with the built-in client: %v", err)
	}
	click(t, tab, `#definicoes #ytSetup [data-act="yt-own"]`)
	if err := until(tab, `(b => !!b.querySelector("#ytId") && !!b.querySelector('[data-act="yt-connect"][disabled]') && !!b.querySelector('[data-act="yt-builtin"]'))(`+box+`)`); err != nil {
		t.Fatalf("the own client's fields: %v", err)
	}
	click(t, tab, `#definicoes #ytSetup [data-act="yt-builtin"]`)
	if err := until(tab, `(b => !b.querySelector("#ytId") && !!b.querySelector('[data-act="yt-connect"]:not([disabled])'))(`+box+`)`); err != nil {
		t.Fatalf("back to one button: %v", err)
	}

	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&setup=done&yt=off#definicoes")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `(b => !!b && !!b.querySelector("#ytId") && !b.querySelector('[data-act="yt-own"]') && !b.querySelector('[data-act="yt-builtin"]'))(`+box+`)`); err != nil {
		t.Errorf("fields without the built-in client: %v", err)
	}
}
