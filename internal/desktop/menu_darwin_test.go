package desktop

import (
	"strings"
	"testing"
)

// buildMenus reads a fixed number of titles; a mismatch would panic at launch.
func TestMenuTitles(t *testing.T) {
	titles := menuTitles()
	if len(titles) != menuCount() {
		t.Fatalf("%d titles, buildMenus reads %d", len(titles), menuCount())
	}
	for i, s := range titles {
		if s == "" || strings.HasPrefix(s, "app.menu.") {
			t.Errorf("title %d is %q", i, s)
		}
	}
}
