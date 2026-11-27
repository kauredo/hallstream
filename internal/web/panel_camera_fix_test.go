package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

func evalJS(t *testing.T, tab context.Context, js string) {
	t.Helper()
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js)); err != nil {
		t.Fatal(err)
	}
}

// A camera's name with markup in it stays text everywhere the panel shows it.
func TestPanelCameraLabelEscaped(t *testing.T) {
	tab := settingsPanel(t, "&cams=vdo")
	if err := until(tab, `document.querySelectorAll("#dCams .cam-row").length === 3`); err != nil {
		t.Fatal(err)
	}
	evalJS(t, tab, `(i => (i.value = "<x-y>\"'&", true))(document.querySelector("#dCamNome1"))`)
	click(t, tab, `[data-act="cam-save"]`)
	const label = `"<x-y>\"'&"`
	if err := until(tab, `document.querySelector("#dCamNome1") && document.querySelector("#dCamNome1").value === `+label+` &&
		!document.querySelector("x-y") && [...document.querySelectorAll("#dObsTcam option")].some(o => o.textContent === `+label+`) &&
		document.querySelector('#dCams .cam-row:first-child [data-act="cam-remove"]').getAttribute("aria-label").includes(`+label+`)`); err != nil {
		t.Errorf("the name in Câmaras: %v", err)
	}
	click(t, tab, `[data-tab="ecras"]`)
	if err := until(tab, `[...document.querySelectorAll("#cams .cam-nome")].some(e => e.textContent === `+label+`) && !document.querySelector("x-y")`); err != nil {
		t.Errorf("the name on Ecrãs: %v", err)
	}
}

// A saved phone shows its push link, fetched behind the PIN, with Copiar.
func TestPanelPhoneLink(t *testing.T) {
	tab := settingsPanel(t, "&cams=vdo")
	if err := until(tab, `(document.querySelector("#dCamLink2") || {}).value === "https://vdo.ninja/?push=mockmockmockmockmock&webcam&autostart&facing=rear&quality=0&audiodevice=0" &&
		!!document.querySelector('#dCams .cam-row:nth-child(2) [data-act="cam-copy"]')`); err != nil {
		t.Errorf("no link under the code: %v", err)
	}
}

// With OBS not answering, the server keeps no list, and the rows stay as typed.
func TestPanelUnsavedListStays(t *testing.T) {
	tab := settingsPanel(t, "&cams=1&setupfail=noobs")
	click(t, tab, `[data-act="cam-add"]`)
	if err := until(tab, `!!document.querySelector("#dCam2") && !!document.querySelector("#dCamNome2")`); err != nil {
		t.Fatal(err)
	}
	evalJS(t, tab, `(s => (s.value = "vdo", s.dispatchEvent(new Event("change", { bubbles: true })), true))(document.querySelector("#dCam2"))`)
	evalJS(t, tab, `(i => (i.value = "Nova", true))(document.querySelector("#dCamNome2"))`)
	click(t, tab, `[data-act="cam-save"]`)
	if err := until(tab, `document.querySelector("#camSetupMsg").className === "msg err" && document.querySelectorAll("#dCams .cam-row").length === 2 &&
		document.querySelector("#dCam2").value === "vdo" && document.querySelector("#dCamNome2").value === "Nova"`); err != nil {
		t.Errorf("the rows after a list OBS never got: %v", err)
	}
}

// Seguinte with a new camera row not saved says so and stays on the step.
func TestPanelWizardUnsavedCamera(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&setup=part&obs=ready&act=wz&n=3&run=1#inicio")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!!document.querySelector('[data-wz-act="cam-add"]')`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-wz-act="cam-add"]`)
	click(t, tab, `#wzNav [data-wz="4"]`)
	if err := until(tab, `!!document.querySelector("#wzBody .cam-row") && document.querySelector('[data-wz-act="cam-save"]').parentElement.querySelector(".msg").className === "msg err"`); err != nil {
		t.Errorf("Seguinte left the unsaved camera: %v", err)
	}
}

// "Mostrar marcador" is on for a new camera, saves with the list and moves
// with its camera when another is taken out.
func TestPanelCameraShowSwitch(t *testing.T) {
	tab := settingsPanel(t, "&cams=3")
	if err := until(tab, `document.querySelectorAll("#dCams [data-cam-show]").length === 3 &&
		[...document.querySelectorAll("#dCams [data-cam-show]")].map(i => i.checked).join() === "true,true,false"`); err != nil {
		t.Fatalf("the saved switches: %v", err)
	}
	click(t, tab, `#dCamShow1`)
	click(t, tab, `#dCams .cam-row:nth-child(2) [data-act="cam-remove"]`)
	click(t, tab, `[data-act="cam-add"]`)
	if err := until(tab, `!!document.querySelector("#dCamShow3") && document.querySelector("#dCamShow3").checked`); err != nil {
		t.Errorf("a new camera's switch is off: %v", err)
	}
	evalJS(t, tab, `(s => (s.value = s.options[1].value, s.dispatchEvent(new Event("change", { bubbles: true })), true))(document.querySelector("#dCam3"))`)
	click(t, tab, `[data-act="cam-save"]`)
	if err := until(tab, `document.querySelector("#camSetupMsg").className.includes("ok") && document.querySelectorAll("#dCams [data-cam-show]").length === 3 &&
		[...document.querySelectorAll("#dCams [data-cam-show]")].map(i => i.checked).join() === "false,false,true"`); err != nil {
		got, _ := chromedp.Run(tab, chromedp.Evaluate[string](`[...document.querySelectorAll("#dCams [data-cam-show]")].map(i => i.checked).join() + " " + document.querySelector("#camSetupMsg").className`))
		t.Errorf("after the save: %s (%v)", got, err)
	}
}
