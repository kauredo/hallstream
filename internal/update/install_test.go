package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestTarget(t *testing.T) {
	for _, c := range []struct{ goos, exe, want string }{
		{"windows", `C:\Hallstream\Hallstream.exe`, `C:\Hallstream\Hallstream.exe`},
		{"linux", "/opt/Hallstream/Hallstream", "/opt/Hallstream/Hallstream"},
		{"darwin", "/Applications/Hallstream.app/Contents/MacOS/hallstream", "/Applications/Hallstream.app"},
		{"darwin", "/Users/x/hallstream", ""},
		{"linux", "/tmp/go-build123/b001/exe/hallstream", ""},
	} {
		got, err := target(c.goos, c.exe)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("%s %s: %q %v", c.goos, c.exe, got, err)
		}
	}
}

func TestInstallableRefuses(t *testing.T) {
	for _, p := range []string{"/private/var/folders/x/AppTranslocation/y/d/Hallstream.app", "/Volumes/Hallstream/Hallstream.app"} {
		if Installable(p) == nil {
			t.Errorf("%s passed", p)
		}
	}
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("folder permissions don't stop this user")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if Installable(filepath.Join(dir, "Hallstream")) == nil {
		t.Error("a read-only folder passed")
	}
}

func TestFromZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "Hallstream-1.0.0.zip")
	f, _ := os.Create(archive)
	z := zip.NewWriter(f)
	for name, data := range map[string]string{"Hallstream.exe": "64", "Hallstream (32-bit).exe": "32", "LEIA-ME.txt": "olá"} {
		w, _ := z.Create(name)
		w.Write([]byte(data))
	}
	z.Close()
	f.Close()
	for member, want := range map[string]string{"Hallstream.exe": "64", "Hallstream (32-bit).exe": "32"} {
		dst := filepath.Join(dir, "out")
		if err := fromZip(archive, member, dst); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(dst); string(b) != want {
			t.Errorf("%s: %q", member, b)
		}
	}
	if fromZip(archive, "missing.exe", filepath.Join(dir, "out")) == nil {
		t.Error("a missing member passed")
	}
}

func TestFromTar(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "Hallstream/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, data := range map[string]string{"Hallstream/Hallstream": "program", "Hallstream/LEIA-ME.txt": "olá"} {
		tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))})
		tw.Write([]byte(data))
	}
	tw.Close()
	gz.Close()
	f.Close()
	dst := filepath.Join(dir, "out")
	if err := fromTar(archive, "Hallstream/Hallstream", dst); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	st, _ := os.Stat(dst)
	if string(b) != "program" || runtime.GOOS != "windows" && st.Mode().Perm()&0o100 == 0 {
		t.Errorf("%q %v", b, st.Mode())
	}
}

func TestSwap(t *testing.T) {
	dir := t.TempDir()
	target, staged := filepath.Join(dir, "Hallstream.exe"), filepath.Join(dir, ".hallstream-update", "new")
	os.WriteFile(target, []byte("old"), 0o755)
	os.MkdirAll(filepath.Dir(staged), 0o755)
	os.WriteFile(staged, []byte("new"), 0o755)
	os.WriteFile(target+".old", []byte("stale"), 0o755)
	undo, err := Swap(staged, target)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Errorf("target %q", b)
	}
	if b, _ := os.ReadFile(target + ".old"); string(b) != "old" {
		t.Errorf("old %q", b)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Errorf("after undo %q", b)
	}
	if _, err := Swap(filepath.Join(dir, "missing"), target); err == nil {
		t.Error("a missing staged file passed")
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Errorf("a failed swap left %q", b)
	}
	os.WriteFile(target+".old", nil, 0o644)
	os.MkdirAll(stageDir(target), 0o755)
	Cleanup(target)
	for _, p := range []string{target + ".old", stageDir(target)} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s left", p)
		}
	}
}

// fakeTools answers the macOS tools as a signed DMG would; team is the
// TeamIdentifier for each app path.
func fakeTools(t *testing.T, team map[string]string, fail string) *[]string {
	return fakeToolsVersion(t, team, fail, "1.2.3")
}

func fakeToolsVersion(t *testing.T, team map[string]string, fail, version string) *[]string {
	t.Helper()
	var calls []string
	old := run
	run = func(name string, args ...string) ([]byte, error) {
		tool := filepath.Base(name)
		calls = append(calls, tool+" "+args[0])
		if tool+" "+args[0] == fail {
			return []byte("refused"), errors.New("exit 1")
		}
		if tool == "ditto" {
			os.MkdirAll(args[1], 0o755)
		}
		if tool == "plutil" {
			return []byte(version + "\n"), nil
		}
		if tool == "codesign" && args[0] == "-dv" {
			id := team[args[len(args)-1]]
			if id == "" {
				id = "not set"
			}
			return []byte("Executable=x\nTeamIdentifier=" + id + "\n"), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { run = old })
	return &calls
}

func TestFromDMG(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Hallstream.app")
	staged := filepath.Join(stageDir(target), "new-Hallstream.app")
	dmg := func() error {
		os.RemoveAll(stageDir(target))
		os.MkdirAll(stageDir(target), 0o700)
		return fromDMG("x.dmg", target, staged, "1.2.3")
	}
	calls := fakeTools(t, map[string]string{target: "TEAM1", staged: "TEAM1"}, "")
	if err := dmg(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"hdiutil attach", "ditto " + filepath.Join(stageDir(target), "mnt", "Hallstream.app"), "codesign --verify", "codesign -dv", "codesign -dv", "plutil -extract", "hdiutil detach"}; !slices.Equal(*calls, want) {
		t.Errorf("calls %q", *calls)
	}
	for name, team := range map[string]map[string]string{
		"another team":         {target: "TEAM1", staged: "OTHER"},
		"both unsigned":        {},
		"unsigned new version": {target: "TEAM1"},
	} {
		fakeTools(t, team, "")
		if dmg() == nil {
			t.Errorf("%s passed", name)
		}
	}
	fakeToolsVersion(t, map[string]string{target: "TEAM1", staged: "TEAM1"}, "", "1.2.2")
	if dmg() == nil {
		t.Error("an older version inside the image passed")
	}
	fakeTools(t, map[string]string{target: "TEAM1", staged: "TEAM1"}, "codesign --verify")
	if err := dmg(); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("a broken signature: %v", err)
	}
	calls = fakeTools(t, nil, "ditto "+filepath.Join(stageDir(target), "mnt", "Hallstream.app"))
	dmg()
	if (*calls)[len(*calls)-1] != "hdiutil detach" {
		t.Errorf("the image stayed mounted: %q", *calls)
	}
}

func TestTeam(t *testing.T) {
	fakeTools(t, map[string]string{"/A.app": "H5587PYVHV"}, "")
	if team("/A.app") != "H5587PYVHV" || team("/B.app") != "" {
		t.Error(team("/A.app"), team("/B.app"))
	}
}

func TestInstallableAllows(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Hallstream.app")
	fakeTools(t, map[string]string{target: "TEAM1"}, "")
	if err := Installable(target); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left %v", left)
	}
	switch runtime.GOOS {
	case "darwin":
		fakeTools(t, nil, "")
		if Installable(target) == nil {
			t.Error("an unsigned app passed")
		}
	case "linux":
		os.Chmod(dir, 0o777)
		if Installable(target) == nil {
			t.Error("a folder anyone can write to passed")
		}
	}
}

func TestZipMember(t *testing.T) {
	if zipMember("386") != "Hallstream (32-bit).exe" || zipMember("amd64") != "Hallstream.exe" {
		t.Error(zipMember("386"), zipMember("amd64"))
	}
}

func TestWriteTooBig(t *testing.T) {
	old := maxDownload
	maxDownload = 4
	t.Cleanup(func() { maxDownload = old })
	if write(filepath.Join(t.TempDir(), "x"), strings.NewReader("too long"), 0o755) == nil {
		t.Error("a member over the cap was cut and kept")
	}
}

// installHere points Install at a plain file in a temp folder, with an
// unpack step that just writes "new".
func installHere(t *testing.T) (target string, rel Release) {
	t.Helper()
	name := AssetFor("1.4.0")
	if name == "" {
		t.Skip("no release file for this system")
	}
	target = filepath.Join(t.TempDir(), "Hallstream")
	os.WriteFile(target, []byte("old"), 0o755)
	fakeTools(t, map[string]string{target: "TEAM1"}, "")
	oldTarget, oldPrepare := findTarget, prepare
	findTarget = func() (string, error) { return target, nil }
	prepare = func(archive, target, version string) (string, error) {
		staged := filepath.Join(stageDir(target), "new")
		return staged, os.WriteFile(staged, []byte("new"), 0o755)
	}
	t.Cleanup(func() { findTarget, prepare = oldTarget, oldPrepare })
	_, rel = release(t, map[string]string{name: "archive"}, nil)
	return target, rel
}

func TestInstall(t *testing.T) {
	target, rel := installHere(t)
	var states []string
	restarted := ""
	err := Install(context.Background(), http.DefaultClient, rel, Hooks{
		OffAir:  func() bool { return true },
		Status:  func(s string) { states = append(states, s) },
		Restart: func(t string) error { restarted = t; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" || restarted != target || !slices.Equal(states, []string{"installing", "restarting"}) {
		t.Errorf("%q, restarted %q, states %v", b, restarted, states)
	}
	if b, _ := os.ReadFile(target + ".old"); string(b) != "old" {
		t.Errorf("old copy %q", b)
	}
}

// Until the restart has started, a failure puts the old program back and
// removes the download.
func TestInstallPutsTheOldBack(t *testing.T) {
	for name, h := range map[string]Hooks{
		"restart fails":   {OffAir: func() bool { return true }, Restart: func(string) error { return errors.New("no") }},
		"live after swap": {OffAir: offAirFor(1), Restart: func(string) error { t.Error("restarted"); return nil }},
		"live before":     {OffAir: offAirFor(0), Restart: func(string) error { t.Error("restarted"); return nil }},
	} {
		target, rel := installHere(t)
		h.Status = func(string) {}
		if Install(context.Background(), http.DefaultClient, rel, h) == nil {
			t.Errorf("%s: no error", name)
		}
		if b, _ := os.ReadFile(target); string(b) != "old" {
			t.Errorf("%s: left %q", name, b)
		}
		if _, err := os.Stat(stageDir(target)); err == nil {
			t.Errorf("%s: download left", name)
		}
	}
}

// offAirFor says off air n times, then live.
func offAirFor(n int) func() bool {
	return func() bool { n--; return n >= 0 }
}
