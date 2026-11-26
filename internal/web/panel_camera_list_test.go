package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

func TestPanelCameraList(t *testing.T) {
	tab := settingsPanel(t, "&cams=vdo")
	if err := until(tab, `document.querySelectorAll("#dCams .cam-row").length === 3 && document.querySelectorAll("#dCams .cx-qr img").length === 1 &&
		!!document.querySelector('[data-act="cam-add"]') && document.querySelector("#dCams .cam-row:nth-child(2) .cam-net") !== null`); err != nil {
		t.Fatalf("three rows, camera 2 a phone with its QR code: %v", err)
	}
	click(t, tab, `[data-act="cam-add"]`)
	if err := until(tab, `document.querySelectorAll("#dCams .cam-row").length === 4 && document.activeElement.id === "dCam4" && !document.querySelector('[data-act="cam-add"]')`); err != nil {
		t.Fatalf("a fourth row and no more: %v", err)
	}
	click(t, tab, `#dCams .cam-row:nth-child(4) [data-act="cam-remove"]`)
	click(t, tab, `#dCams .cam-row:nth-child(2) [data-act="cam-remove"]`)
	if err := until(tab, `document.querySelectorAll("#dCams .cam-row").length === 2 && document.activeElement.id === "dCam2" &&
		document.querySelector("#dCam2").value === "source:HD Webcam" && !!document.querySelector('[data-act="cam-add"]') && document.querySelector("#dObsTcam").disabled`); err != nil {
		t.Fatalf("Tirar: %v", err)
	}
	click(t, tab, `[data-act="cam-add"]`)
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(s => (s.value = "vdo", s.dispatchEvent(new Event("change", { bubbles: true })), true))(document.querySelector("#dCam3"))`)); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!!document.querySelector("#dCams .cam-row:nth-child(3) .cam-net") && !document.querySelector("#dCams .cam-row:nth-child(3) img")`); err != nil {
		t.Errorf("a new phone shows the note, not a code: %v", err)
	}

	one := settingsPanel(t, "&cams=1")
	if err := until(one, `document.querySelectorAll("#dCams .cam-row").length === 1 && !document.querySelector('[data-act="cam-remove"]')`); err != nil {
		t.Errorf("one camera can't be removed: %v", err)
	}
}

// The wizard's OBS step offers another camera once Configurar OBS worked, on the PC only.
func TestPanelWizardAnotherCamera(t *testing.T) {
	open := func(query string) context.Context {
		srv := httptest.NewServer(newServerFiles(t, assets.FS))
		t.Cleanup(srv.Close)
		tab := chrome(t)
		if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&setup=part&obs=ready&act=wz&n=3&run=1"+query+"#inicio")); err != nil {
			t.Fatal(err)
		}
		return tab
	}
	tab := open("")
	if err := until(tab, `!!document.querySelector('[data-wz-act="cam-add"]')`); err != nil {
		t.Fatalf("no Adicionar outra câmara after the setup: %v", err)
	}
	click(t, tab, `[data-wz-act="cam-add"]`)
	if err := until(tab, `!!document.querySelector("#wzBody .cam-row #wCam2") && !document.querySelector('[data-wz-act="cam-add"]')`); err != nil {
		t.Fatalf("no camera 2 row: %v", err)
	}
	three := open("&cams=3")
	if err := until(three, `!!document.querySelector('[data-wz-act="cam-add"]')`); err != nil {
		t.Fatalf("three cameras saved: no Adicionar outra câmara: %v", err)
	}
	click(t, three, `[data-wz-act="cam-add"]`)
	if err := until(three, `!!document.querySelector("#wzBody .cam-row #wCam4")`); err != nil {
		t.Fatalf("no camera 4 row: %v", err)
	}
	four := open("&cams=4")
	if err := until(four, `!!document.querySelector("#wzObsResult") && document.querySelector("#wzObsResult").textContent !== ""`); err != nil {
		t.Fatal(err)
	}
	if shown, err := chromedp.Run(four, chromedp.Evaluate[bool](`!!document.querySelector('[data-wz-act="cam-add"]')`)); err != nil || shown {
		t.Errorf("four cameras saved still offer another: %v %v", shown, err)
	}
	phone := open("&pc=0")
	if err := until(phone, `!!document.querySelector("#wzBody") && document.querySelector("#wzBody").textContent !== ""`); err != nil {
		t.Fatal(err)
	}
	if shown, err := chromedp.Run(phone, chromedp.Evaluate[bool](`!!document.querySelector('[data-wz-act="cam-add"]')`)); err != nil || shown {
		t.Errorf("a phone offers another camera: %v %v", shown, err)
	}
}
