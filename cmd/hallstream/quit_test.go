package main

import (
	"testing"

	"github.com/kauredo/hallstream/internal/i18n"
)

func TestConfirmQuitAsksOnlyWhenLive(t *testing.T) {
	asked := ""
	ask := func(answer bool) func(string) bool {
		return func(msg string) bool { asked = msg; return answer }
	}
	no := func() bool { return false }
	yes := func() bool { return true }

	if !confirmQuit(no, yes, ask(false)) || asked != "" {
		t.Errorf("off air: asked %q", asked)
	}
	if confirmQuit(yes, no, ask(false)) || asked != i18n.T("app.sair.no_ar") {
		t.Errorf("live, said no: asked %q", asked)
	}
	if !confirmQuit(yes, yes, ask(true)) || asked != i18n.T("app.sair.no_ar_fecha_obs") {
		t.Errorf("live, closing OBS, said yes: asked %q", asked)
	}
}
