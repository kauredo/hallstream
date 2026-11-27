package desktop

import (
	"path/filepath"
	"slices"
	"testing"
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
	if got[0] != "--app=http://127.0.0.1:8765/painel" || !slices.Contains(got, `--user-data-dir=C:\p`) || !slices.Contains(got, "--no-first-run") {
		t.Errorf("%q", got)
	}
}
