package web

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

const updBox = `document.querySelector("#updateAsk")`

func TestPanelUpdateOffer(t *testing.T) {
	tab := settingsPanel(t, "&update=1")
	if err := until(tab, `!`+updBox+`.hidden && document.querySelector("#updateTxt").textContent.includes("9.9.9") &&
		!document.querySelector('[data-act="up-now"]').hidden && document.querySelector("#updateLink").textContent === I18N["painel.atualizacao.notas"]`); err != nil {
		t.Fatalf("the offer: %v", err)
	}
	click(t, tab, `[data-act="up-now"]`)
	if err := until(tab, `document.querySelector("#updateHint").textContent === I18N["painel.atualizacao.a_transferir"] &&
		document.querySelector('[data-act="up-now"]').disabled && document.querySelector('[data-act="up-later"]').disabled`); err != nil {
		t.Errorf("after Atualizar agora: %v", err)
	}
}

func TestPanelUpdateLater(t *testing.T) {
	tab := settingsPanel(t, "&update=1")
	if err := until(tab, `!`+updBox+`.hidden`); err != nil {
		t.Fatal(err)
	}
	click(t, tab, `[data-act="up-later"]`)
	if err := until(tab, updBox+`.hidden && localStorage.getItem("hs-update-later") === "9.9.9"`); err != nil {
		t.Errorf("Mais tarde: %v", err)
	}
	evalJS(t, tab, `(localStorage.removeItem("hs-update-later"), true)`)
}

// Where this copy can't install itself, the box only links to the release.
func TestPanelUpdateLinkOnly(t *testing.T) {
	for _, q := range []string{"&update=link", "&update=1&pc=0"} {
		tab := settingsPanel(t, q)
		if err := until(tab, `!`+updBox+`.hidden && document.querySelector('[data-act="up-now"]').hidden &&
			document.querySelector("#updateLink").classList.contains("primary") && document.querySelector("#updateLink").textContent === I18N["painel.atualizacao.ver"]`); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
}

func TestPanelUpdateFailed(t *testing.T) {
	tab := settingsPanel(t, "&update=fail")
	if err := until(tab, `!`+updBox+`.hidden && document.querySelector("#updateAsk .msg").className === "msg err" &&
		document.querySelector("#updateAsk .msg").textContent.includes("Hallstream-9.9.9.zip") && !document.querySelector('[data-act="up-now"]').disabled`); err != nil {
		t.Errorf("a failed install: %v", err)
	}
}

// Live, the offer waits for the stream to end.
func TestPanelUpdateHiddenLive(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel?mock=1&obs=live&update=1#ecras")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `!document.querySelector("#liveAir").hidden && `+updBox+`.hidden`); err != nil {
		t.Errorf("shown while live: %v", err)
	}
}

// A new install is asked once, before the shortcut question.
func TestPanelUpdateAskOnce(t *testing.T) {
	tab := settingsPanel(t, "&update=ask")
	if err := until(tab, `!document.querySelector("#checkAsk").hidden && document.querySelector("#shortcutAsk").hidden`); err != nil {
		t.Fatalf("the question: %v", err)
	}
	click(t, tab, `[data-act="ck-yes"]`)
	if err := until(tab, `document.querySelector("#checkAsk").hidden && !document.querySelector("#shortcutAsk").hidden`); err != nil {
		t.Errorf("after Sim: %v", err)
	}
}
