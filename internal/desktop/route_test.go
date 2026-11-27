package desktop

import "testing"

func TestInWindow(t *testing.T) {
	const panel = "http://127.0.0.1:8766/painel"
	for target, want := range map[string]bool{
		"http://127.0.0.1:8766/painel#jogo":             true,
		"http://127.0.0.1:8766/api/system/backup":       true,
		"about:blank":                                   true,
		"http://127.0.0.1:8765/painel":                  false, // another copy, or Marcador CNN
		"https://127.0.0.1:8766/painel":                 false,
		"http://localhost:8766/painel":                  false,
		"https://github.com/kauredo/hallstream/issues":  false,
		"https://accounts.google.com/o/oauth2/auth?x=1": false,
		"mailto:club@example.org":                       false,
		"data:text/csv;charset=utf-8,a":                 false,
		"%zz":                                           false,
	} {
		if got := inWindow(target, panel); got != want {
			t.Errorf("%s: %v", target, got)
		}
	}
}
