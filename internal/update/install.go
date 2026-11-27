package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"io"
	"net/http"
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

// The steps Install takes; vars so tests replace the ones that touch the
// running program or need another system.
var (
	findTarget = Target
	prepare    = Prepare
)

// Hooks are what Install asks of the program around it.
type Hooks struct {
	OffAir   func() bool               // false stops the install: the stream went live
	Status   func(state string)        // "installing", then "restarting"
	Progress func(percent int)         // the download's progress, when the size is known
	Restart  func(target string) error // starts the new program at target and closes this one
}

// Install downloads rel, checks it, puts it in place of the running program
// and restarts. Until Restart has started the new copy, any failure, a panic
// included, puts the old program back and removes what it downloaded.
func Install(ctx context.Context, c *http.Client, rel Release, h Hooks) (err error) {
	target, err := findTarget()
	if err != nil {
		return err
	}
	if err := Installable(target); err != nil {
		return err
	}
	name := AssetFor(rel.Version)
	if name == "" {
		return i18n.Err("log.atualizacao.sem_programa")
	}
	stage, err := stageFolder(target)
	if err != nil {
		return err
	}
	var undo func() error
	started := false
	defer func() {
		if started {
			return
		}
		if undo != nil {
			if uerr := undo(); uerr != nil {
				err = i18n.Err("log.atualizacao.nao_repus", "err", uerr)
			}
		}
		os.RemoveAll(stage)
	}()
	archive, err := Download(ctx, c, rel, name, stage, h.Progress)
	if err != nil {
		return err
	}
	h.Status("installing")
	staged, err := prepare(archive, target, rel.Version)
	if err != nil {
		return err
	}
	if !h.OffAir() {
		return i18n.Err("api.atualizacao.no_ar")
	}
	if undo, err = Swap(staged, target); err != nil {
		return err
	}
	if !h.OffAir() {
		return i18n.Err("api.atualizacao.no_ar")
	}
	h.Status("restarting")
	if err := h.Restart(target); err != nil {
		return err
	}
	started = true
	return nil
}

// AssetFor is AssetName for this system.
func AssetFor(version string) string { return AssetName(runtime.GOOS, runtime.GOARCH, version) }

// CanInstall says why this copy can't replace itself, or nil.
func CanInstall() error {
	if AssetFor("0.0.0") == "" {
		return i18n.Err("log.atualizacao.sem_programa")
	}
	t, err := findTarget()
	if err != nil {
		return err
	}
	return Installable(t)
}

// Target is what an update replaces: the running program, or on a Mac the
// Hallstream.app around it. Ask it before a swap: on Linux the running
// program's path follows the renamed file.
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
	// go run builds into a temporary go-build folder: nothing to replace.
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
	// On Linux a folder other users can write to would let them swap the
	// download between the check and the swap. (macOS's Applications is
	// writable by the admin group by design, so it isn't held to this.)
	if st, err := os.Stat(dir); runtime.GOOS == "linux" && err == nil && st.Mode().Perm()&0o022 != 0 {
		return i18n.Err("log.atualizacao.pasta_partilhada", "pasta", dir)
	}
	if runtime.GOOS == "darwin" && team(target) == "" {
		return i18n.Err("log.atualizacao.sem_assinatura")
	}
	return nil
}

// stageFolder empties and makes the folder beside target where an update is
// downloaded and unpacked; it sits on the same disk, so the swap is a rename.
// Mkdir (not MkdirAll) fails if someone made it again in between.
func stageFolder(target string) (string, error) {
	dir := stageDir(target)
	os.RemoveAll(dir)
	return dir, os.Mkdir(dir, 0o700)
}

func stageDir(target string) string { return filepath.Join(filepath.Dir(target), ".hallstream-update") }

// Prepare unpacks the program for version from the downloaded archive into
// the stage folder and returns it, ready for Swap.
func Prepare(archive, target, version string) (string, error) {
	staged := filepath.Join(stageDir(target), "new-"+filepath.Base(target))
	switch runtime.GOOS {
	case "windows":
		return staged, fromZip(archive, zipMember(runtime.GOARCH), staged)
	case "linux":
		return staged, fromTar(archive, "Hallstream/Hallstream", staged)
	case "darwin":
		return staged, fromDMG(archive, target, staged, version)
	}
	return "", i18n.Err("log.atualizacao.sem_programa")
}

// zipMember is the exe in the Windows zip that matches this build.
func zipMember(goarch string) string {
	if goarch == "386" {
		return "Hallstream (32-bit).exe"
	}
	return "Hallstream.exe"
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
	n, err := io.Copy(f, io.LimitReader(r, maxDownload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDownload {
		err = i18n.Err("log.atualizacao.grande_demais", "ficheiro", filepath.Base(dst))
	}
	return err
}

// fromDMG copies Hallstream.app out of the disk image and accepts it only
// when its signature holds, it names the same team as the running app, and
// it is the version the release says.
func fromDMG(archive, target, staged, version string) error {
	mount := filepath.Join(stageDir(target), "mnt")
	if err := os.Mkdir(mount, 0o700); err != nil {
		return err
	}
	if out, err := run("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, archive); err != nil {
		return toolErr("hdiutil", out, err)
	}
	defer run("/usr/bin/hdiutil", "detach", "-force", mount)
	if out, err := run("/usr/bin/ditto", filepath.Join(mount, "Hallstream.app"), staged); err != nil {
		return toolErr("ditto", out, err)
	}
	if st, err := os.Lstat(staged); err != nil || !st.IsDir() {
		return i18n.Err("log.atualizacao.sem_ficheiro", "ficheiro", "Hallstream.app")
	}
	if out, err := run("/usr/bin/codesign", "--verify", "--deep", "--strict", staged); err != nil {
		return toolErr("codesign", out, err)
	}
	if t := team(staged); t == "" || t != team(target) {
		return i18n.Err("log.atualizacao.outra_assinatura")
	}
	out, err := run("/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", filepath.Join(staged, "Contents", "Info.plist"))
	if got := strings.TrimSpace(string(out)); err != nil || got != version {
		return i18n.Err("log.atualizacao.outra_versao", "versao", got)
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
	if err := os.RemoveAll(old); err != nil {
		// On Windows a copy still running from it holds the file.
		return nil, i18n.Err("log.atualizacao.antiga_aberta", "ficheiro", old)
	}
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
