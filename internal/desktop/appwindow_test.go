package desktop

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestAppBrowsers(t *testing.T) {
	// A 32-bit exe on 64-bit Windows: ProgramFiles is the x86 folder, ProgramW6432 the real one.
	env := map[string]string{
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"ProgramFiles":      `C:\Program Files (x86)`,
		"ProgramW6432":      `C:\Program Files`,
		"LOCALAPPDATA":      `C:\Users\Clube\AppData\Local`,
	}
	got := appBrowsers(func(k string) string { return env[k] }, "base")
	var exes []string
	for _, b := range got {
		exes = append(exes, b.exe)
	}
	want := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Users\Clube\AppData\Local\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Users\Clube\AppData\Local\Google\Chrome\Application\chrome.exe`,
	}
	if !slices.Equal(exes, want) {
		t.Errorf("tried\n%q\nwant\n%q", exes, want)
	}
	if got[0].profile != filepath.Join("base", "edge") || got[3].profile != filepath.Join("base", "chrome") {
		t.Errorf("profiles %q, %q", got[0].profile, got[3].profile)
	}
	if len(appBrowsers(func(string) string { return "" }, "base")) != 0 {
		t.Error("no environment, still a browser")
	}
}

func TestAppArgs(t *testing.T) {
	got := appArgs("http://127.0.0.1:8765/painel", `C:\p`)
	want := []string{"--app=http://127.0.0.1:8765/painel", `--user-data-dir=C:\p`, "--no-first-run", "--no-default-browser-check", "--start-maximized"}
	if !slices.Equal(got, want) {
		t.Errorf("%q", got)
	}
}

func TestRetryWindow(t *testing.T) {
	for _, c := range []struct {
		code          int
		lived         time.Duration
		tries         int
		again, giveUp bool
	}{
		{0, time.Hour, 0, false, false},  // ended on its own
		{3, time.Second, 0, true, false}, // failed at the start: once more
		{3, time.Second, 1, false, true}, // and again: Edge
		{3, time.Hour, 1, true, false},
		{3, time.Hour, 5, true, false},        // a good run wipes earlier tries
		{0, time.Second, 1, false, false},     // crashed after a good run
		{1, 29 * time.Second, 1, false, true}, // never had a good run
		{1, 30 * time.Second, 1, true, false}, // half a minute counts as one
	} {
		again, giveUp, next := retryWindow(c.code, c.lived, c.tries)
		if again != c.again || giveUp != c.giveUp || (again && next != 1) {
			t.Errorf("retryWindow(%d, %v, %d) = %v, %v", c.code, c.lived, c.tries, again, giveUp)
		}
	}
}
