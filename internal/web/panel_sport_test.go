package web

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

// In the panel's mock, two escalões are both called "Seniores": they show
// their sport, "Sub 16" stays bare, a futsal escalão is offered no FPB, and
// "Guardar" sends the sport.
func TestPanelSportPerEscalao(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t) // after the server, so the panel's event stream ends before the server closes
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelectorAll("#jGroup option").length >= 4`); err != nil {
		t.Fatalf("the Jogo tab never listed the escalões: %v", err)
	}
	run := func(js string) {
		t.Helper()
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js+`, true`)); err != nil {
			t.Fatal(err)
		}
	}
	labels := func(selector string) []string {
		t.Helper()
		var out []string
		raw, err := chromedp.Run(tab, chromedp.Evaluate[string](`JSON.stringify([...document.querySelectorAll(`+"`"+selector+"`"+`)].map(o => o.textContent))`))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	has := func(js string) bool {
		t.Helper()
		v, err := chromedp.Run(tab, chromedp.Evaluate[bool](js))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	pick := func(selectID, value string) {
		run(`(() => { const sel = document.querySelector("#` + selectID + `"); sel.value = "` + value + `"; sel.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	}
	want := []string{"Sénior Masculino", "Sub 16", "Seniores · Basquetebol", "Seniores · Futsal"}
	if got := labels("#jGroup option"); !slices.Equal(got, want) {
		t.Errorf("Jogo escalões %q, want %q", got, want)
	}

	// Jogo: the FPB button follows the escalão picked.
	const fpbButton = `document.querySelector('#jogo [data-act="fpb-games"]') !== null`
	pick("jGroup", "seniores-futsal")
	if err := until(tab, `document.querySelector("#jGroup").value === "seniores-futsal"`); err != nil {
		t.Fatal(err)
	}
	if has(fpbButton) {
		t.Error("a futsal escalão offers 'Escolher jogo da FPB'")
	}
	pick("jGroup", "seniores-basquetebol")
	if err := until(tab, `document.querySelector("#jGroup").value === "seniores-basquetebol" && `+fpbButton); err != nil {
		t.Errorf("a basketball escalão offers no 'Escolher jogo da FPB': %v", err)
	}

	// Plantel: the same labels, the FPB card only on a basketball escalão.
	run(`document.querySelector('.nav [data-tab="plantel"]').click()`)
	if err := until(tab, `document.querySelector("#pGroup") !== null && document.querySelector("#pGSport") !== null`); err != nil {
		t.Fatalf("Plantel never drew: %v", err)
	}
	if got := labels("#pGroup option"); !slices.Equal(got, want) {
		t.Errorf("Plantel escalões %q, want %q", got, want)
	}
	const fpbCard = `[...document.querySelectorAll("#plantel h2")].some(h => h.textContent === "FPB")`
	pick("pGroup", "seniores-futsal")
	if err := until(tab, `document.querySelector("#pGroup").value === "seniores-futsal" && document.querySelector("#pGSport").value === "futsal"`); err != nil {
		t.Fatalf("the futsal escalão never showed its sport: %v", err)
	}
	if has(fpbCard) {
		t.Error("a futsal escalão shows the FPB card")
	}
	if has(`document.querySelector("#plantel .photo-sw + .hint").textContent.includes("FPB")`) {
		t.Error("a futsal escalão tells the photos come from the FPB")
	}
	if has(`document.querySelector('#pGSport option[value="futsal"]').disabled`) {
		t.Error("futsal is playable now, but the option is disabled")
	}
	if has(`document.querySelector('#pGSport option[value="handball"]').disabled`) {
		t.Error("handball is playable now, but the option is disabled")
	}
	if has(`document.querySelector('#pGSport option[value="soccer"]').disabled`) {
		t.Error("soccer is playable now, but the option is disabled")
	}
	pick("pGroup", "seniores-basquetebol")
	if err := until(tab, `document.querySelector("#pGroup").value === "seniores-basquetebol" && `+fpbCard); err != nil {
		t.Errorf("a basketball escalão shows no FPB card: %v", err)
	}
	if !has(`document.querySelector("#plantel .photo-sw + .hint").textContent.includes("FPB")`) {
		t.Error("a basketball escalão does not tell the photos come from the FPB")
	}

	// Guardar sends the sport with the name and the coach.
	run(`window.__puts = []; const f = window.fetch; window.fetch = (u, o) => { if (o && o.method === "PUT") window.__puts.push(JSON.parse(o.body)); return f(u, o); }`)
	run(`document.querySelector('[data-act="save-group"]').click()`)
	if err := until(tab, `window.__puts.length > 0`); err != nil {
		t.Fatalf("Guardar sent nothing: %v", err)
	}
	raw, err := chromedp.Run(tab, chromedp.Evaluate[string](`JSON.stringify(window.__puts[0])`))
	if err != nil {
		t.Fatal(err)
	}
	var put struct{ Name, Coach, Sport string }
	if err := json.Unmarshal([]byte(raw), &put); err != nil || put.Sport != "basketball" || put.Name != "Seniores" {
		t.Errorf("Guardar sent %s (%v)", raw, err)
	}
}
