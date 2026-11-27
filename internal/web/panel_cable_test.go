package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

func cablePanel(t *testing.T, query string) context.Context {
	t.Helper()
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&setup=done"+query+"#caixa")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!!document.querySelector('#cxInput input[name="cxIn"][value="serial"]')`); err != nil {
		t.Fatalf("the input card never drew: %v", err)
	}
	return tab
}

// Picking the cable opens its console and port under it, and choosing a port saves input "serial".
func TestPanelCablePicked(t *testing.T) {
	tab := cablePanel(t, "")
	if err := until(tab, `!document.querySelector("#cxPort")`); err != nil {
		t.Fatalf("the cable fields show for the box: %v", err)
	}
	click(t, tab, `#cxInput input[value="serial"]`)
	if err := until(tab, `!!document.querySelector('#cxInput .cx-in > label.open + .cx-serial #cxPort') &&
		[...document.querySelectorAll("#cxPort option")].map(o => o.textContent).join() === "Nenhuma porta,COM1,COM3,COM4"`); err != nil {
		t.Fatalf("the cable fields: %v", err)
	}
	evalJS(t, tab, `(s => (s.value = "COM4", s.dispatchEvent(new Event("change", { bubbles: true })), true))(document.querySelector("#cxPort"))`)
	if err := until(tab, `fetch("/api/settings").then(r => r.json()).then(s => window.__s = s), window.__s && window.__s.input === "serial" && window.__s.serial.port === "COM4"`); err != nil {
		t.Errorf("not saved: %v", err)
	}
}

// Each state of the cable draws its own line: what is read, the next step, or why.
func TestPanelCableStates(t *testing.T) {
	for state, want := range map[string]string{
		"lendo":    `p.textContent.startsWith("A ler a consola · ") && !!row.querySelector(".dot.on")`,
		"aberta":   `p.textContent === "A porta está aberta, mas a consola não envia nada. Carregue uma vez em STOP na consola." && !!row.querySelector(".dot.wait") && p.classList.contains("wait")`,
		"erro":     `p.classList.contains("err") && p.querySelector("b").textContent === "A porta não abre." && !!row.querySelector(".dot.off")`,
		"semporta": `p.querySelector("b").textContent === "Escolha a porta." && !!row.querySelector(".dot.gutter") && !!document.querySelector(".cx-port.empty")`,
	} {
		t.Run(state, func(t *testing.T) {
			tab := cablePanel(t, "&cabo="+state)
			if err := until(tab, `(row => { const p = row && row.querySelector("p"); return !!p && (`+want+`); })(document.querySelector("#cxSerialState"))`); err != nil {
				t.Error(err)
			}
		})
	}
}
