package obsws

import (
	"regexp"
	"testing"
)

func TestVDOLinks(t *testing.T) {
	if got := VDOPush("abc"); got != "https://vdo.ninja/?push=abc&webcam&autostart&facing=rear&quality=0&audiodevice=0" {
		t.Errorf("push %s", got)
	}
	if got := VDOView("abc"); got != "https://vdo.ninja/?view=abc&noaudio&cleanoutput&videobitrate=4000" {
		t.Errorf("view %s", got)
	}
	if id, ok := IsVDOView(VDOView("abc")); id != "abc" || !ok {
		t.Errorf("view link: %q %v", id, ok)
	}
	for _, u := range []string{"http://127.0.0.1:8765/overlay", "https://vdo.ninja/?view=abc", "https://vdo.ninja/?view=&noaudio&cleanoutput&videobitrate=4000",
		"https://vdo.ninja/?push=abc&webcam&autostart&facing=rear&quality=0&audiodevice=0", ""} {
		if id, ok := IsVDOView(u); ok {
			t.Errorf("%q is no view link, got %q", u, id)
		}
	}
}

func TestVDOID(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9]{20}$`)
	seen := map[string]bool{}
	for range 100 {
		id, err := NewVDOID()
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(id) || seen[id] {
			t.Errorf("id %q (seen %v)", id, seen[id])
		}
		seen[id] = true
	}
}
