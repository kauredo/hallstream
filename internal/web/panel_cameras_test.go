package web

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/i18n"
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

// Four buttons sit in one row on a wide screen and two by two on a phone,
// never cut and never three plus one.
func TestPanelFourCameras(t *testing.T) {
	for _, c := range []struct {
		width int
		rows  string
	}{{1280, "4"}, {375, "2,2"}} {
		srv := httptest.NewServer(newServerFiles(t, assets.FS))
		t.Cleanup(srv.Close)
		tab := chrome(t)
		if err := chromedp.Do(tab, chromedp.EmulateViewport(int64(c.width), 800), chromedp.Navigate(srv.URL+"/painel?mock=1&obs=ready&cams=4&live=4#ecras")); err != nil {
			t.Fatal(err)
		}
		if err := until(tab, `document.querySelectorAll("#cams .cam").length === 4 && document.querySelector('[data-cam="4"]').getAttribute("aria-pressed") === "true"`); err != nil {
			t.Fatalf("%d px: four buttons with camera 4 on air: %v", c.width, err)
		}
		rows, err := chromedp.Run(tab, chromedp.Evaluate[string](`(() => {
			const tops = [...document.querySelectorAll("#cams .cam")].map(b => Math.round(b.getBoundingClientRect().top));
			const counts = [];
			tops.forEach((y, i) => { if (i && y === tops[i - 1]) counts[counts.length - 1]++; else counts.push(1); });
			return counts.join();
		})()`))
		if err != nil {
			t.Fatal(err)
		}
		if rows != c.rows {
			t.Errorf("%d px: buttons per row %s, want %s", c.width, rows, c.rows)
		}
		if fits, err := chromedp.Run(tab, chromedp.Evaluate[bool](`document.documentElement.scrollWidth <= innerWidth &&
			[...document.querySelectorAll("#cams .cam")].every(b => b.scrollWidth <= b.clientWidth && b.getBoundingClientRect().right <= document.querySelector("#cams").getBoundingClientRect().right + 0.5)`)); err != nil || !fits {
			t.Errorf("%d px: a camera button is cut or the page scrolls sideways: %v %v", c.width, fits, err)
		}
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

// A server with no camera health in its status draws no health lines.
func TestPanelCameraHealthAbsent(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=1&health=none")
	if err := until(tab, `document.querySelectorAll("#cams .cam").length === 3 && [...document.querySelectorAll("#cams .cam-saude")].every(e => e.textContent === "" && e.dataset.state === "")`+
		` && document.querySelector("#camFrames").textContent === ""`); err != nil {
		t.Fatalf("health lines with no health: %v", err)
	}
	tab = cameraPanel(t, "&cams=1&health=none")
	if err := until(tab, `document.querySelector("#camCard").dataset.cams === "1" && document.querySelector("#camCard").hidden`); err != nil {
		t.Errorf("one camera with no health shows the card: %v", err)
	}
}

// Three cameras, then one, then three again: the buttons and their health
// lines follow the saved list.
func TestPanelCameraHealthListChanges(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=1&health=mixed")
	lines := `[...document.querySelectorAll("#cams .cam-saude")].map(e => e.dataset.state).join() === "ok,no_image,frozen"`
	if err := until(tab, lines); err != nil {
		t.Fatalf("three health lines: %v", err)
	}
	save := func() {
		t.Helper()
		click(t, tab, `[data-act="cam-save"]`)
		if err := until(tab, `/\bmsg (ok|err)\b/.test(document.querySelector("#camSetupMsg").className)`); err != nil {
			t.Fatalf("the camera list was not saved: %v", err)
		}
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`location.hash = "#ecras", true`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`location.hash = "#definicoes", true`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelectorAll("#dCams .cam-row").length === 3`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `#dCams .cam-row:nth-child(3) [data-act="cam-remove"]`)
	click(t, tab, `#dCams .cam-row:nth-child(2) [data-act="cam-remove"]`)
	save()
	if err := until(tab, `document.querySelector("#camCard").dataset.cams === "1" && document.querySelector("#camCard").hidden`); err != nil {
		t.Fatalf("one camera, picture fine: %v", err)
	}
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`location.hash = "#definicoes", true`)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dCam2", "dCam3"} {
		if err := until(tab, `!!document.querySelector('[data-act="cam-add"]')`); err != nil {
			t.Fatal(err)
		}
		click(t, tab, `[data-act="cam-add"]`)
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(s => (s.value = "vdo", s.dispatchEvent(new Event("change", { bubbles: true })), true))(document.querySelector("#`+id+`"))`)); err != nil {
			t.Fatal(err)
		}
	}
	save()
	if err := until(tab, `!document.querySelector("#camCard").hidden && document.querySelectorAll("#cams .cam").length === 3 && `+lines); err != nil {
		t.Errorf("three cameras again: %v", err)
	}
}

// A status read while a switch runs updates the health lines and leaves
// "A mudar…" and the disabled buttons alone.
func TestPanelCameraHealthDuringSwitch(t *testing.T) {
	tab := cameraPanel(t, "&cams=3&live=1&health=ok")
	if err := until(tab, `document.querySelectorAll("#cams .cam-saude[data-state=ok]").length === 3`); err != nil {
		t.Fatal(err)
	}
	// The switch answers after 15 s; the next status read (every 10 s) has camera 2 frozen.
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(f => (window.fetch = (u, o) => {
		if (String(u).includes("/api/obs/camera/live")) return new Promise(r => setTimeout(() => r(f(u, o)), 15000));
		if (String(u).endsWith("/api/status")) return f(u, o).then(r => r.json()).then(j => {
			j.obs.cameras[1].state = "frozen";
			return new Response(JSON.stringify(j), { status: 200, headers: { "Content-Type": "application/json" } });
		});
		return f(u, o);
	}, true))(window.fetch)`)); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-cam="2"]`)
	if err := until(tab, `document.querySelector('[data-cam="2"] .cam-saude').dataset.state === "frozen"`+
		` && document.querySelector('[data-cam="2"] .cam-estado').textContent === `+strconv.Quote(i18n.T("painel.camaras.a_mudar"))+
		` && [...document.querySelectorAll("#cams .cam")].every(b => b.disabled)`); err != nil {
		t.Errorf("health during a switch: %v", err)
	}
}
