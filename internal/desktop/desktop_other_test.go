//go:build !windows

package desktop

import "testing"

func TestOtherSystems(t *testing.T) {
	if Shortcut != nil || Autostart != nil {
		t.Error("only Windows makes shortcuts")
	}
	if !Ask("Sair?") {
		t.Error("with no one to ask, Ask says yes")
	}
}
