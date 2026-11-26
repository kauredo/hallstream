package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

// cameraPanel opens Ecrãs in the panel's mock with query added.
func cameraPanel(t *testing.T, query string) context.Context {
	t.Helper()
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&obs=ready"+query+"#ecras")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!!document.querySelector("#camCard") && !!document.querySelector("#screens .scr")`); err != nil {
		t.Fatalf("Ecrãs never drew: %v", err)
	}
	return tab
}

func click(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector('`+selector+`').click(), true`)); err != nil {
		t.Fatal(err)
	}
}

func TestPanelCameras(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=2")
	if err := until(tab, `document.querySelectorAll("#cams .cam").length === 3 && document.querySelector('[data-cam="2"]').getAttribute("aria-pressed") === "true"`); err != nil {
		t.Fatalf("three buttons with camera 2 on air: %v", err)
	}
	click(t, tab, `[data-cam="3"]`)
	if err := until(tab, `document.querySelector('[data-cam="3"]').getAttribute("aria-pressed") === "true" && document.querySelector('[data-cam="2"]').getAttribute("aria-pressed") === "false"`); err != nil {
		t.Fatalf("camera 3 never went on air: %v", err)
	}
}

func TestPanelOneCameraNoCard(t *testing.T) {
	tab := cameraPanel(t, "&cams=1")
	// The card stays hidden once drawn with the status in.
	if err := until(tab, `document.querySelector("#camCard").dataset.cams === "1" && document.querySelector("#camCard").hidden`); err != nil {
		t.Fatalf("one camera shows the camera card: %v", err)
	}
	tab = cameraPanel(t, "&cams=2")
	if err := until(tab, `document.querySelector("#camCard").dataset.cams === "2" && !document.querySelector("#camCard").hidden`); err != nil {
		t.Fatalf("two cameras show no camera card: %v", err)
	}
}

func TestPanelCameraRefused(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=1&camerr=1")
	if err := until(tab, `document.querySelectorAll("#cams .cam").length === 3`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-cam="2"]`)
	if err := until(tab, `document.querySelector("#camMsg").classList.contains("err") && !document.querySelector('[data-cam="1"]').disabled`); err != nil {
		t.Fatalf("no error shown: %v", err)
	}
	if err := until(tab, `document.querySelector('[data-cam="1"]').getAttribute("aria-pressed") === "true" && document.querySelector('[data-cam="2"]').getAttribute("aria-pressed") === "false"`); err != nil {
		t.Errorf("camera 1 must stay on air: %v", err)
	}
}
