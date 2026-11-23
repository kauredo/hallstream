package obsapp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/kauredo/hallstream/internal/i18n"
)

const exeName = "obs64.exe"

// System is OBS on this Windows PC.
func System() App {
	return App{ConfigPath: configPath(), Running: running, Start: start, Close: closeOBS}
}

func running() (bool, error) {
	pid, err := find()
	return pid != 0, err
}

// find is the pid of the first obs64.exe, or 0.
func find() (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) {
			return e.ProcessID, nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return 0, nil
	}
	return 0, err
}

// Path is obs64.exe from the installer's registry key, else the default
// folder. The installer writes HKLM\Software\OBS Studio in both views.
func Path() (string, error) {
	var tried []string
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\OBS Studio`, registry.QUERY_VALUE|view)
		if err != nil {
			continue
		}
		dir, _, err := k.GetStringValue("")
		k.Close()
		if err == nil && dir != "" {
			tried = append(tried, filepath.Join(dir, "bin", "64bit", exeName))
		}
	}
	for _, env := range []string{"ProgramW6432", "ProgramFiles"} {
		if dir := os.Getenv(env); dir != "" {
			tried = append(tried, filepath.Join(dir, "obs-studio", "bin", "64bit", exeName))
		}
	}
	for _, p := range tried {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", i18n.Err("obs.nao_instalado")
}

// start runs OBS in the tray. OBS finds its own files from the folder it
// starts in, so that must be its bin\64bit. --disable-shutdown-check skips
// the "OBS did not close properly" question on OBS 30 and 31; later
// versions no longer have it and ignore it.
func start() error {
	exe, err := Path()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--minimize-to-tray", "--disable-shutdown-check")
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return i18n.Err("obs.nao_abri", "err", err)
	}
	go cmd.Wait()
	return nil
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	postMessage     = user32.NewProc("PostMessageW")
	closePID        atomic.Uint32 // the OBS whose windows enumWindowsProc closes
	enumWindowsProc = syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		windows.GetWindowThreadProcessId(hwnd, &owner)
		if owner == closePID.Load() {
			const wmClose = 0x0010
			postMessage.Call(uintptr(hwnd), wmClose, 0, 0)
		}
		return 1
	})
)

// closeOBS sends WM_CLOSE, what the window's X sends, to every top-level
// window of OBS (also the hidden one when it sits in the tray), so OBS
// saves its settings and exits. If it is still running after 15 s and force
// is true, it is ended.
func closeOBS(force bool) error {
	pid, err := find()
	if err != nil || pid == 0 {
		return err
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	closePID.Store(pid)
	windows.EnumWindows(enumWindowsProc, nil)
	if ev, _ := windows.WaitForSingleObject(h, 15000); ev == windows.WAIT_OBJECT_0 {
		return nil
	}
	if !force {
		return i18n.Err("obs.nao_fechou")
	}
	return windows.TerminateProcess(h, 1)
}
