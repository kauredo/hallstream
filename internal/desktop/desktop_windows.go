package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Console is false: the exe is built without a console window.
const Console = false

const title = "Tribuna"

// Fatal shows why the program can't start, in a message box.
func Fatal(msg string) { box(msg, windows.MB_ICONERROR) }

// Warn shows a problem the program can carry on with. It blocks until the
// volunteer clicks OK.
func Warn(msg string) { box(msg, windows.MB_ICONWARNING) }

func box(msg string, icon uint32) {
	text, _ := windows.UTF16PtrFromString(msg)
	caption, _ := windows.UTF16PtrFromString(title)
	windows.MessageBox(0, text, caption, windows.MB_OK|icon|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
}

// Ask shows a yes/no question; true is yes. No is the default button.
func Ask(msg string) bool {
	text, _ := windows.UTF16PtrFromString(msg)
	caption, _ := windows.UTF16PtrFromString(title)
	r, _ := windows.MessageBox(0, text, caption, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2|windows.MB_SETFOREGROUND|windows.MB_TOPMOST)
	return r == 6 // IDYES
}

// Shortcut puts "Tribuna.lnk" on the user's desktop (the real one, also
// when OneDrive moved it), pointing at this exe, started in its folder.
var Shortcut = func() (string, error) { return link(windows.FOLDERID_Desktop, "o ambiente de trabalho") }

// Autostart puts the same shortcut in the user's Startup folder, so
// Windows opens the program at sign-in, or takes it out.
var Autostart = func(on bool) error {
	if on {
		_, err := link(windows.FOLDERID_Startup, "a pasta Arranque")
		return err
	}
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Startup, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return errors.New("não encontrei a pasta Arranque: " + err.Error())
	}
	if err := os.Remove(filepath.Join(dir, title+".lnk")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// link makes "Tribuna.lnk" in the known folder, replacing one there.
func link(folder *windows.KNOWNFOLDERID, name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir, err := windows.KnownFolderPath(folder, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", errors.New("não encontrei " + name + ": " + err.Error())
	}
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	lnk := filepath.Join(dir, title+".lnk")
	// The paths go in environment variables, never in the script, so a
	// quote or an apostrophe in a folder name is only ever data.
	const script = `$ErrorActionPreference = 'Stop'; ` +
		`$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:TRIBUNA_LNK); ` +
		`$s.TargetPath = $env:TRIBUNA_EXE; $s.WorkingDirectory = $env:TRIBUNA_DIR; ` +
		`$s.IconLocation = $env:TRIBUNA_EXE + ',0'; $s.Description = 'Tribuna'; $s.Save()`
	cmd := exec.Command(filepath.Join(sys, `WindowsPowerShell\v1.0\powershell.exe`),
		"-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "TRIBUNA_LNK="+lnk, "TRIBUNA_EXE="+exe, "TRIBUNA_DIR="+filepath.Dir(exe))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return lnk, nil
}

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
)

// CopyText puts s on the clipboard.
func CopyText(s string) error {
	const cfUnicodeText, gmemMoveable = 13, 0x0002
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, err := openClipboard.Call(0); r == 0 {
		return err
	}
	defer closeClipboard.Call()
	emptyClipboard.Call()
	h, _, err := globalAlloc.Call(gmemMoveable, uintptr(len(u)*2))
	if h == 0 {
		return err
	}
	p, _, err := globalLock.Call(h)
	if p == 0 {
		globalFree.Call(h)
		return err
	}
	copy(unsafe.Slice((*uint16)(*(*unsafe.Pointer)(unsafe.Pointer(&p))), len(u)), u)
	globalUnlock.Call(h)
	// The clipboard owns the memory once this works.
	if r, _, err := setClipboardData.Call(cfUnicodeText, h); r == 0 {
		globalFree.Call(h)
		return err
	}
	return nil
}
