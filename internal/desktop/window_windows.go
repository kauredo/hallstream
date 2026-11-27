package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"

	"github.com/kauredo/hallstream/internal/i18n"
)

// HasWindow is true where the panel gets its WebView2 window, which closes
// with the program. HALLSTREAM_NO_WEBVIEW2=1 skips it, for the Edge window.
// The 32-bit exe skips it too: there WebView2 never finished starting.
var HasWindow = runtime.GOARCH != "386" && os.Getenv("HALLSTREAM_NO_WEBVIEW2") == "" && webview2Installed()

func webview2Installed() bool {
	v, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	return err == nil && v != ""
}

var (
	enumWindows      = user32.NewProc("EnumWindows")
	windowProcessID  = user32.NewProc("GetWindowThreadProcessId")
	getClassName     = user32.NewProc("GetClassNameW")
	isIconic         = user32.NewProc("IsIconic")
	allowForeground  = user32.NewProc("AllowSetForegroundWindow")
	findPanelWindows = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		windowProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != child.pid {
			return 1
		}
		var name [64]uint16
		getClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if windows.UTF16ToString(name[:]) != panelClass {
			return 1
		}
		child.hwnd = hwnd
		return 0
	})
)

// child is the panel window's process.
var child struct {
	sync.Mutex
	pid    uint32         // 0 when none runs
	hwnd   uintptr        // its window, found by raise
	failed bool           // WebView2 gave up: the Edge window from now on
	job    windows.Handle // ends the window when this program ends
}

// ShowPanel shows the panel's WebView2 window, starting it the first time,
// else opens an Edge (else Chrome) app window. False when none of these is
// there, and the caller opens the default browser.
func ShowPanel(url string) bool {
	child.Lock()
	defer child.Unlock()
	if HasWindow && !child.failed {
		if child.pid != 0 {
			raise()
			return true
		}
		if startWindow(url, 0) == nil {
			return true
		}
		child.failed = true
	}
	return showAppBrowser(url)
}

// AllowForeground lets the running copy bring its window to the front when
// this one, the copy the volunteer just started, asks it to.
func AllowForeground() {
	allowForeground.Call(uintptr(^uint32(0))) // ASFW_ANY
}

// raise brings the running window back and in front. Called with child held.
func raise() {
	child.hwnd = 0
	enumWindows.Call(findPanelWindows, 0)
	if child.hwnd == 0 {
		return // still starting; it shows itself when ready
	}
	if r, _, _ := isIconic.Call(child.hwnd); r != 0 {
		showWindow.Call(child.hwnd, swRestore)
	} else {
		showWindow.Call(child.hwnd, swShow)
	}
	setForeground.Call(child.hwnd)
}

// startWindow starts the window's process; tries counts the starts since its
// last good run. Called with child held.
func startWindow(url string, tries int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if child.job == 0 {
		if child.job, err = killOnClose(); err != nil {
			return err
		}
	}
	cmd := exec.Command(exe, "-window", url)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := uint32(cmd.Process.Pid)
	// A window outside the job would outlive the program, showing a dead panel.
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err == nil {
		err = windows.AssignProcessToJobObject(child.job, p)
		windows.CloseHandle(p)
	}
	if err != nil {
		cmd.Process.Kill()
		go cmd.Wait()
		return err
	}
	allowForeground.Call(uintptr(pid))
	child.pid = pid
	go watch(cmd, url, tries, time.Now())
	return nil
}

// watch waits for the window's process to end, then starts it again or falls
// back to the Edge window (retryWindow).
func watch(cmd *exec.Cmd, url string, tries int, started time.Time) {
	cmd.Wait()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	again, giveUp, next := retryWindow(code, time.Since(started), tries)
	child.Lock()
	child.pid = 0
	if again {
		logLine(i18n.T("app.janela.de_novo", "codigo", code))
		child.Unlock()
		time.Sleep(2 * time.Second)
		child.Lock()
		if child.pid == 0 && !child.failed && startWindow(url, next) != nil {
			giveUp = true
		}
	}
	if giveUp {
		child.failed = true
		logLine(i18n.T("app.janela.desisti", "codigo", code))
		if !showAppBrowser(url) {
			openOutside(url)
		}
	}
	child.Unlock()
}

// killOnClose makes a job object whose processes end when this program ends,
// however it ends.
func killOnClose() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// showAppBrowser opens the panel as an Edge (else Chrome) app window. False
// when neither is installed.
func showAppBrowser(url string) bool {
	for _, b := range appBrowsers(os.Getenv, appDataBase()) {
		if _, err := os.Stat(b.exe); err != nil {
			continue
		}
		cmd := exec.Command(b.exe, appArgs(url, b.profile)...)
		if cmd.Start() == nil {
			go cmd.Wait()
			return true
		}
	}
	return false
}

// appDataBase is %LOCALAPPDATA%\Hallstream, where the windows keep their
// browser data, out of dados so backups stay small.
func appDataBase() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "Hallstream")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "Hallstream")
	}
	return filepath.Join(os.TempDir(), "Hallstream")
}
