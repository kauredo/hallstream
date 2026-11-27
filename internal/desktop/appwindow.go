package desktop

import (
	"path/filepath"
	"strings"
)

// appBrowser is a browser that can show a page as an app window: its own
// window and taskbar entry, with no address bar or tabs.
type appBrowser struct {
	exe     string // the browser's program file
	profile string // the profile folder it gets, kept apart from the volunteer's own browsing
}

// appBrowsers lists where Edge, then Chrome, install on Windows, in the order
// to try them. env reads the environment; base is the folder the profiles go
// in. A 32-bit program on 64-bit Windows sees ProgramFiles as the x86 folder,
// so ProgramW6432 is there for the 64-bit installs.
func appBrowsers(env func(string) string, base string) []appBrowser {
	var out []appBrowser
	seen := map[string]bool{}
	add := func(exe, profile string) {
		if key := strings.ToLower(exe); !seen[key] {
			seen[key] = true
			out = append(out, appBrowser{exe: exe, profile: filepath.Join(base, profile)})
		}
	}
	roots := []string{env("ProgramFiles(x86)"), env("ProgramW6432"), env("ProgramFiles"), env("LOCALAPPDATA")}
	for _, r := range roots {
		if r != "" {
			add(r+`\Microsoft\Edge\Application\msedge.exe`, "edge")
		}
	}
	for _, r := range roots {
		if r != "" {
			add(r+`\Google\Chrome\Application\chrome.exe`, "chrome")
		}
	}
	return out
}

// appArgs open url as an app window, maximised, in its own profile, with
// none of the browser's first-run screens.
func appArgs(url, profile string) []string {
	return []string{"--app=" + url, "--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check", "--start-maximized"}
}
