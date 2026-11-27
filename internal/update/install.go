package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

// run runs a macOS tool and returns what it printed; a var so tests fake it.
var run = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Target is what an update replaces: the running program, or on a Mac the
// Hallstream.app around it.
func Target() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return target(runtime.GOOS, exe)
}

func target(goos, exe string) (string, error) {
	if strings.Contains(exe, "go-build") {
		return "", i18n.Err("log.atualizacao.sem_programa")
	}
	if goos != "darwin" {
		return exe, nil
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if filepath.Base(filepath.Dir(exe)) != "MacOS" || !strings.HasSuffix(bundle, ".app") {
		return "", i18n.Err("log.atualizacao.sem_programa")
	}
	return bundle, nil
}

// Installable says why target can't be replaced here, or nil.
func Installable(target string) error {
	if strings.Contains(target, "/AppTranslocation/") || strings.HasPrefix(target, "/Volumes/") {
		return i18n.Err("log.atualizacao.mover_app")
	}
	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, ".hallstream-*")
	if err != nil {
		return i18n.Err("log.atualizacao.pasta_protegida", "pasta", dir)
	}
	f.Close()
	os.Remove(f.Name())
	if runtime.GOOS == "darwin" && team(target) == "" {
		return i18n.Err("log.atualizacao.sem_assinatura")
	}
	return nil
}

// StageDir empties and returns the folder beside target where an update is
// downloaded and unpacked; it sits on the same disk, so the swap is a rename.
func StageDir(target string) (string, error) {
	dir := stageDir(target)
	os.RemoveAll(dir)
	return dir, os.MkdirAll(dir, 0o755)
}

func stageDir(target string) string { return filepath.Join(filepath.Dir(target), ".hallstream-update") }

// Prepare unpacks the program from the downloaded archive into the stage
// folder and returns it, ready for Swap.
func Prepare(archive, target string) (string, error) {
	staged := filepath.Join(stageDir(target), "new-"+filepath.Base(target))
	switch runtime.GOOS {
	case "windows":
		member := "Hallstream.exe"
		if runtime.GOARCH == "386" {
			member = "Hallstream (32-bit).exe"
		}
		return staged, fromZip(archive, member, staged)
	case "linux":
		return staged, fromTar(archive, "Hallstream/Hallstream", staged)
	case "darwin":
		return staged, fromDMG(archive, target, staged)
	}
	return "", i18n.Err("log.atualizacao.sem_programa")
}

func fromZip(archive, member, dst string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name != member {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		return write(dst, r, 0o755)
	}
	return i18n.Err("log.atualizacao.sem_ficheiro", "ficheiro", member)
}

func fromTar(archive, member, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return i18n.Err("log.atualizacao.sem_ficheiro", "ficheiro", member)
		}
		if err != nil {
			return err
		}
		if h.Name == member && h.Typeflag == tar.TypeReg {
			return write(dst, tr, 0o755)
		}
	}
}

func write(dst string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(r, maxDownload)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fromDMG copies Hallstream.app out of the disk image and accepts it only
// when its signature holds and names the same team as the running app.
func fromDMG(archive, target, staged string) error {
	mount := filepath.Join(stageDir(target), "mnt")
	if err := os.MkdirAll(mount, 0o755); err != nil {
		return err
	}
	if out, err := run("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, archive); err != nil {
		return toolErr("hdiutil", out, err)
	}
	defer run("/usr/bin/hdiutil", "detach", "-force", mount)
	if out, err := run("/usr/bin/ditto", filepath.Join(mount, "Hallstream.app"), staged); err != nil {
		return toolErr("ditto", out, err)
	}
	if out, err := run("/usr/bin/codesign", "--verify", "--deep", "--strict", staged); err != nil {
		return toolErr("codesign", out, err)
	}
	if t := team(staged); t == "" || t != team(target) {
		return i18n.Err("log.atualizacao.outra_assinatura")
	}
	return nil
}

// team is the TeamIdentifier an app is signed with, or "" for none.
func team(app string) string {
	out, _ := run("/usr/bin/codesign", "-dv", "--verbose=2", app)
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "TeamIdentifier="); ok && v != "not set" {
			return v
		}
	}
	return ""
}

func toolErr(tool string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return i18n.Err("log.atualizacao.ferramenta", "ferramenta", tool, "err", msg)
}

// Swap puts staged where target is and keeps the old one as target.old. undo
// puts the old one back.
func Swap(staged, target string) (undo func() error, err error) {
	old := target + ".old"
	os.RemoveAll(old)
	if err := os.Rename(target, old); err != nil {
		return nil, err
	}
	if err := os.Rename(staged, target); err != nil {
		os.Rename(old, target)
		return nil, err
	}
	return func() error {
		os.RemoveAll(target)
		return os.Rename(old, target)
	}, nil
}

// Cleanup removes what an earlier update left beside target.
func Cleanup(target string) {
	os.RemoveAll(target + ".old")
	os.RemoveAll(stageDir(target))
}
