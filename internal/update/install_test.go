package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
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
	calls := fakeTools(t, map[string]string{target: "TEAM1", staged: "TEAM1"}, "")
	if err := fromDMG("x.dmg", target, staged); err != nil {
		t.Fatal(err)
	}
	if want := []string{"hdiutil attach", "ditto " + filepath.Join(stageDir(target), "mnt", "Hallstream.app"), "codesign --verify", "codesign -dv", "codesign -dv", "hdiutil detach"}; !slices.Equal(*calls, want) {
		t.Errorf("calls %q", *calls)
	}
	fakeTools(t, map[string]string{target: "TEAM1", staged: "OTHER"}, "")
	if fromDMG("x.dmg", target, staged) == nil {
		t.Error("another team's app passed")
	}
	fakeTools(t, map[string]string{target: "TEAM1", staged: "TEAM1"}, "codesign --verify")
	if err := fromDMG("x.dmg", target, staged); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("a broken signature: %v", err)
	}
	calls = fakeTools(t, nil, "ditto "+filepath.Join(stageDir(target), "mnt", "Hallstream.app"))
	fromDMG("x.dmg", target, staged)
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
