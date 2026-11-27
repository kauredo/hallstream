//go:build !windows && !darwin && !linux

package desktop

import "testing"

func TestOtherSystems(t *testing.T) {
	if Shortcut != nil || Autostart != nil {
		t.Error("only Windows, macOS and Linux open at sign-in")
	}
	if !Ask("Sair?") {
		t.Error("with no one to ask, Ask says yes")
	}
}
