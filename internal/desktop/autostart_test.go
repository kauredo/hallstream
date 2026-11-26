package desktop

import (
	"strings"
	"testing"
)

func TestLaunchAgent(t *testing.T) {
	got := string(launchAgent(bundleID, "/Users/a/A&B/Hallstream.app"))
	for _, want := range []string{
		"<string>com.hallstream.app</string>",
		"<string>/usr/bin/open</string>",
		"<string>/Users/a/A&amp;B/Hallstream.app</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist lacks %q:\n%s", want, got)
		}
	}
}

func TestAutostartEntry(t *testing.T) {
	got := string(autostartEntry(`/home/a/My "club"/Hallstream/Hallstream`))
	for _, want := range []string{
		"[Desktop Entry]\n",
		`Exec="/home/a/My \"club\"/Hallstream/Hallstream"` + "\n",
		`Path=/home/a/My "club"/Hallstream` + "\n",
		"Terminal=false\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("entry lacks %q:\n%s", want, got)
		}
	}
	if got := string(autostartEntry("/x/$a`b\\c/h")); !strings.Contains(got, "Exec=\"/x/\\$a\\`b\\\\c/h\"\n") {
		t.Errorf("not escaped:\n%s", got)
	}
}

func TestBundlePath(t *testing.T) {
	const moved = "/private/var/folders/x/T/AppTranslocation/ABC/d/Hallstream.app/Contents/MacOS/Hallstream"
	if !translocated(moved) {
		t.Error("translocated copy not seen")
	}
	if translocated("/Applications/Hallstream.app/Contents/MacOS/Hallstream") {
		t.Error("an app in Applications is not translocated")
	}
	if got := bundlePath(moved); got != "/private/var/folders/x/T/AppTranslocation/ABC/d/Hallstream.app" {
		t.Errorf("bundlePath = %q", got)
	}
	if got := bundlePath("/usr/local/bin/hallstream"); got != "" {
		t.Errorf("bundlePath outside a bundle = %q", got)
	}
}
