package desktop

import "testing"

func TestInWindow(t *testing.T) {
	const panel = "http://127.0.0.1:8766/painel"
	for target, want := range map[string]bool{
		"http://127.0.0.1:8766/painel#jogo":       true,
		"http://127.0.0.1:8766/api/system/backup": true,
		"about:blank":                                   false, // the window allows it on its own
		"http://x@127.0.0.1:8766/painel":                true,  // same host and port; userinfo changes nothing
		"http://127.0.0.1.:8766/painel":                 false,
		"HTTP://127.0.0.1:8766/painel":                  true,
		"http://[::1]:8766/painel":                      false,
		"file:///etc/passwd":                            false,
		"http://127.0.0.1:8766@evil.example/":           false,
		"http://127.0.0.1:8766.evil.example/":           false,
		"blob:http://127.0.0.1:8766/1234":               false,
		"javascript:alert(1)":                           false,
		"":                                              false,
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
	if inWindow("", "") || inWindow(panel, "%zz") {
		t.Error("an empty or broken panel address matched")
	}
}

func TestOutside(t *testing.T) {
	for target, want := range map[string]bool{
		"https://accounts.google.com/o/oauth2/auth?x=1": true,
		"HTTPS://github.com/kauredo/hallstream":         true,
		"http://example.org":                            true,
		"mailto:club@example.org":                       true,
		"https:///nohost":                               false,
		"http://":                                       false,
		"http:foo":                                      false,
		"HTTP://":                                       false,
		"//example.org":                                 false,
		"ftp://example.org":                             false,
		"tel:1":                                         false,
		"https://u@example.org/":                        true,
		"MAILTO:a@b":                                    true,
		"https://a\nb":                                  false,
		"file:///C:/Windows/System32/calc.exe":          false,
		"javascript:alert(1)":                           false,
		"ms-settings:":                                  false,
		"about:blank":                                   false,
		"":                                              false,
		"%zz":                                           false,
	} {
		if got := outside(target); got != want {
			t.Errorf("outside(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestStays(t *testing.T) {
	const panel = "http://127.0.0.1:8766/painel"
	for target, want := range map[string]bool{
		"http://127.0.0.1:8766/painel#jogo":       true,
		"http://127.0.0.1:8766/api/system/backup": true,
		"about:blank":                         true,
		"data:text/csv;charset=utf-8,a":       true,
		"blob:http://127.0.0.1:8766/1234":     true,
		"blob:https://evil.example/1":         false,
		"BLOB:http://127.0.0.1:8766/1234":     true,
		"DATA:text/csv,a":                     true,
		"ABOUT:BLANK":                         false,
		"about:blank#x":                       false,
		"":                                    false,
		"%zz":                                 false,
		"javascript:1":                        false,
		"http://127.0.0.1:8766@evil.example/": false,
		"https://accounts.google.com/":        false,
		"http://127.0.0.1:8765/painel":        false,
		"about:srcdoc":                        false,
		"file:///etc/passwd":                  false,
	} {
		if got := stays(target, panel); got != want {
			t.Errorf("stays(%q) = %v, want %v", target, got, want)
		}
	}
}
