package desktop

import (
	"strings"
	"testing"
)

func TestLaunchAgent(t *testing.T) {
	got := string(launchAgent(bundleID, `/Users/a/A&B <"x">/Hallstream.app`))
	for _, want := range []string{
		"<string>com.hallstream.app</string>",
		"<string>/usr/bin/open</string>",
		"<string>/Users/a/A&amp;B &lt;&#34;x&#34;&gt;/Hallstream.app</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist lacks %q:\n%s", want, got)
		}
	}
}

func TestAutostartEntry(t *testing.T) {
	cases := []struct{ exe, exec, path string }{
		{`/home/a/My "club"/Hallstream/Hallstream`, `"/home/a/My \\"club\\"/Hallstream/Hallstream"`, `/home/a/My "club"/Hallstream`},
		{"/x/$a`b/h", "\"/x/\\\\$a\\\\`b/h\"", "/x/$a`b"},
		{`/x/a\b/h`, `"/x/a\\\\b/h"`, `/x/a\\b`},
		{"/x/100%/h", `"/x/100%%/h"`, "/x/100%"},
		{"/x/a b/h (deleted)", `"/x/a b/h"`, "/x/a b"},
	}
	for _, c := range cases {
		b, err := autostartEntry(c.exe)
		if err != nil {
			t.Errorf("%s: %v", c.exe, err)
			continue
		}
		got := string(b)
		for _, want := range []string{"[Desktop Entry]\n", "Exec=" + c.exec + "\n", "Path=" + c.path + "\n", "Terminal=false\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: entry lacks %q:\n%s", c.exe, want, got)
			}
		}
	}
	for _, exe := range []string{"/x/a\nb/h", "/x/a\tb/h", "/x/a\x7fb/h"} {
		if _, err := autostartEntry(exe); err == nil {
			t.Errorf("%q: a control character went into the entry", exe)
		}
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
