package web

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

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

// settingsPanel opens Definições in the panel's mock with query added.
func settingsPanel(t *testing.T, query string) context.Context {
	t.Helper()
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&obs=ready"+query+"#definicoes")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!!document.querySelector("#dObsFit")`); err != nil {
		t.Fatalf("Definições never drew: %v", err)
	}
	return tab
}

func TestPanelTimeoutCamera(t *testing.T) {
	tab := settingsPanel(t, "&cams=3&tcam=2")
	if err := until(tab, `document.querySelector("#dObsTcam").value === "2" && document.querySelectorAll("#dObsTcam option").length === 4`); err != nil {
		t.Fatalf("camera 2 not chosen: %v", err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.querySelector("#dObsTcam").value = "3", true`)); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-act="save-settings"]`)
	saved := 0.0
	for end := time.Now().Add(5 * time.Second); saved != 3 && time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		var err error
		saved, err = chromedp.Run(tab, chromedp.Evaluate[float64](`fetch("/api/settings").then(r => r.json()).then(s => s.obs.timeout_camera)`, chromedp.EvalAwaitPromise))
		if err != nil {
			t.Fatal(err)
		}
	}
	if saved != 3 {
		t.Errorf("saved timeout_camera %v", saved)
	}

	gone := settingsPanel(t, "&cams=2&tcam=3")
	if err := until(gone, `document.querySelector("#dObsTcam").value === "3" && document.querySelector("#dObsTcam").selectedOptions[0].textContent.includes("3")`); err != nil {
		t.Errorf("a camera that is gone must stay chosen: %v", err)
	}

	one := settingsPanel(t, "&cams=1")
	if shown, err := chromedp.Run(one, chromedp.Evaluate[bool](`!!document.querySelector("#dObsTcamField")`)); err != nil || shown {
		t.Errorf("one camera shows the time-out camera: %v %v", shown, err)
	}
}

func TestPanelCameraHealth(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=1&health=mixed")
	if err := until(tab, `[...document.querySelectorAll("#cams .cam-saude")].map(e => e.dataset.state).join() === "ok,no_image,frozen"`+
		` && document.querySelector('[data-cam="2"] .cam-saude').title !== "" && document.querySelector('[data-cam="1"] .cam-saude').title === ""`+
		` && document.querySelector("#camFrames").textContent === ""`); err != nil {
		t.Fatalf("health lines: %v", err)
	}
	tab = cameraPanel(t, "&cams=3&live=1&health=skipped")
	if err := until(tab, `/214/.test(document.querySelector("#camFrames").textContent)`); err != nil {
		t.Fatalf("no skipped frames line: %v", err)
	}
	// One camera: the card shows only for a problem, with the fix written out.
	tab = cameraPanel(t, "&cams=1&health=noimage")
	if err := until(tab, `!document.querySelector("#camCard").hidden && document.querySelectorAll("#cams .cam").length === 0`+
		` && !!document.querySelector("#camSaude .cam-dica")`); err != nil {
		t.Fatalf("one camera with no picture: %v", err)
	}
}
