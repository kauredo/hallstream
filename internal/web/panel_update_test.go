package web

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

const offer = `document.querySelector("#updateOffer")`

// panelAt opens the mock panel with query, on a tab.
func panelAt(t *testing.T, query string) context.Context {
	t.Helper()
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1"+query)); err != nil {
		t.Fatal(err)
	}
	return tab
}

func TestPanelUpdateOffer(t *testing.T) {
	tab := panelAt(t, "&obs=ready&update=1#jogo")
	if err := until(tab, `!`+offer+`.hidden && document.querySelector("#updateTxt").textContent.includes("9.9.9") &&
		!document.querySelector('[data-act="up-now"]').hidden && document.querySelector("#updateLink").textContent === I18N["painel.atualizacao.notas"]`); err != nil {
		t.Fatalf("the offer: %v", err)
	}
	// The mock moves the install on with each poll: installing, restarting, two polls with no answer, a new version.
	evalJS(t, tab, `(window.beforeReload = true, true)`)
	click(t, tab, `[data-act="up-now"]`)
	if err := until(tab, `document.querySelector('[data-act="up-now"]').disabled && document.querySelector('[data-act="up-later"]').disabled`); err != nil {
		t.Errorf("after Atualizar agora: %v", err)
	}
	if err := until(tab, `document.querySelector("#updateHint").textContent === I18N["painel.atualizacao.a_reiniciar"]`); err != nil {
		t.Errorf("never said restarting: %v", err)
	}
	// The reload cuts an evaluation short; ask again on the new page.
	var err error
	for range 3 {
		if err = until(tab, `!window.beforeReload && !!document.querySelector("#updateOffer")`); err == nil {
			break
		}
	}
	if err != nil {
		t.Errorf("no reload on the new version: %v", err)
	}
}

func TestPanelUpdateLater(t *testing.T) {
	tab := panelAt(t, "&obs=ready&update=1#jogo")
	if err := until(tab, `!`+offer+`.hidden`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-act="up-later"]`)
	if err := until(tab, offer+`.hidden && localStorage.getItem("hs-update-later") === "9.9.9"`); err != nil {
		t.Errorf("Mais tarde: %v", err)
	}
	evalJS(t, tab, `(localStorage.removeItem("hs-update-later"), true)`)
}

// Where this copy can't install itself, the box only links to the release.
func TestPanelUpdateLinkOnly(t *testing.T) {
	for _, q := range []string{"&obs=ready&update=link#jogo", "&obs=ready&update=1&pc=0#jogo"} {
		tab := panelAt(t, q)
		if err := until(tab, `!`+offer+`.hidden && document.querySelector('[data-act="up-now"]').hidden &&
			document.querySelector("#updateLink").classList.contains("primary") && document.querySelector("#updateLink").textContent === I18N["painel.atualizacao.ver"]`); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
}

func TestPanelUpdateFailed(t *testing.T) {
	tab := panelAt(t, "&obs=ready&update=fail#jogo")
	if err := until(tab, `!`+offer+`.hidden && document.querySelector("#updateOffer .msg").className === "msg err" &&
		document.querySelector("#updateOffer .msg").textContent.includes("Hallstream-9.9.9.zip") && !document.querySelector('[data-act="up-now"]').disabled`); err != nil {
		t.Errorf("a failed install: %v", err)
	}
}

// A refused Atualizar agora says why and the button comes back.
func TestPanelUpdateRefused(t *testing.T) {
	tab := panelAt(t, "&obs=ready&update=busy#jogo")
	if err := until(tab, `!`+offer+`.hidden`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-act="up-now"]`)
	if err := until(tab, `document.querySelector("#updateOffer .msg").className === "msg err" && !document.querySelector('[data-act="up-now"]').disabled`); err != nil {
		t.Errorf("a refused install: %v", err)
	}
}

// Live, the offer waits for the stream to end.
func TestPanelUpdateHiddenLive(t *testing.T) {
	tab := panelAt(t, "&obs=live&update=1#ecras")
	// The text shows the status has drawn the box once, so hidden means live and not "not loaded yet".
	if err := until(tab, `!document.querySelector("#liveAir").hidden && document.querySelector("#updateTxt").textContent.includes("9.9.9") && `+offer+`.hidden`); err != nil {
		t.Errorf("shown while live: %v", err)
	}
}

// A new install is asked once, before the shortcut question.
func TestPanelUpdateAskOnce(t *testing.T) {
	for _, answer := range []string{"check-yes", "check-no"} {
		tab := panelAt(t, "&obs=ready&update=ask#jogo")
		if err := until(tab, `!document.querySelector("#checkAsk").hidden && document.querySelector("#shortcutAsk").hidden`); err != nil {
			t.Fatalf("the question: %v", err)
		}
		click(t, tab, `[data-act="`+answer+`"]`)
		if err := until(tab, `document.querySelector("#checkAsk").hidden && !document.querySelector("#shortcutAsk").hidden`); err != nil {
			t.Errorf("after %s: %v", answer, err)
		}
	}
}
