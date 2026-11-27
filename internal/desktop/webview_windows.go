package desktop

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// The panel window runs in its own process (Hallstream.exe -window <url>):
// go-webview2 ends the process on any WebView2 error, which must not take the
// server and the overlay with it.

const panelClass = "HallstreamPanel"

var (
	registerClassEx  = user32.NewProc("RegisterClassExW")
	createWindowEx   = user32.NewProc("CreateWindowExW")
	defWindowProc    = user32.NewProc("DefWindowProcW")
	showWindow       = user32.NewProc("ShowWindow")
	setForeground    = user32.NewProc("SetForegroundWindow")
	getMessage       = user32.NewProc("GetMessageW")
	translateMessage = user32.NewProc("TranslateMessage")
	dispatchMessage  = user32.NewProc("DispatchMessageW")
	loadIcon         = user32.NewProc("LoadIconW")
	loadCursor       = user32.NewProc("LoadCursorW")
	getDpiForWindow  = user32.NewProc("GetDpiForWindow")
)

const (
	wmMove          = 0x0003
	wmSize          = 0x0005
	wmActivate      = 0x0006
	wmClose         = 0x0010
	wmGetMinMaxInfo = 0x0024
	swHide          = 0
	swShowMaximized = 3
	swShow          = 5
	swRestore       = 9
	cwUseDefault    = 0x80000000
	wsOverlapped    = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	idcArrow        = 32512
	colorWindow     = 5
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	x, y    int32
	private uint32
}

// The child's one window and browser; set before the message loop runs.
var (
	panelChromium *edge.Chromium
	panelReady    atomic.Bool
	panelURL      string
)

// RunWindow is the child process: the panel at url in a window of its own,
// until the main program ends it. It returns only when it can't start.
func RunWindow(url string) int {
	panelURL = url
	// WebView2 can also stall without an error; the main program then retries.
	time.AfterFunc(20*time.Second, func() {
		if !panelReady.Load() {
			os.Exit(4)
		}
	})
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := loadIcon.Call(inst, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("APP"))))
	cursor, _, _ := loadCursor.Call(0, idcArrow)
	class := windows.StringToUTF16Ptr(panelClass)
	wc := wndClassEx{
		wndProc:    windows.NewCallback(panelProc),
		instance:   inst,
		icon:       icon,
		cursor:     cursor,
		background: colorWindow + 1,
		className:  class,
		iconSm:     icon,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, _ := registerClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return 2
	}
	hwnd, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Hallstream"))),
		wsOverlapped, cwUseDefault, cwUseDefault, cwUseDefault, cwUseDefault, 0, 0, inst, 0)
	if hwnd == 0 {
		return 2
	}

	c := edge.NewChromium()
	c.DataPath = filepath.Join(appDataBase(), "webview2")
	c.SetErrorCallback(func(error) { os.Exit(3) })
	c.ProcessFailedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2ProcessFailedEventArgs) {
		c.Navigate(panelURL)
	}
	panelChromium = c
	if !c.Embed(hwnd) {
		return 3
	}
	if s, err := c.GetSettings(); err == nil {
		s.PutAreDevToolsEnabled(false)
		s.PutAreDefaultContextMenusEnabled(true)
		s.PutIsStatusBarEnabled(false)
	}
	wv, err := c.GetController().GetCoreWebView2()
	if err != nil {
		return 3
	}
	var token int64
	comCall(unsafe.Pointer(wv), 7, uintptr(unsafe.Pointer(&navStarting)), uintptr(unsafe.Pointer(&token)))
	comCall(unsafe.Pointer(wv), 44, uintptr(unsafe.Pointer(&newWindow)), uintptr(unsafe.Pointer(&token)))
	panelReady.Store(true)
	c.Navigate(url)
	showWindow.Call(hwnd, swShowMaximized)
	// Made while its window was hidden, the browser shows nothing until told.
	c.Show()
	c.Resize()
	setForeground.Call(hwnd)
	c.Focus()

	var m msg
	for {
		if r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return 0
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func panelProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmSize:
		if panelReady.Load() {
			panelChromium.Resize()
		}
	case wmMove:
		if panelReady.Load() {
			panelChromium.NotifyParentWindowPositionChanged()
		}
	case wmActivate:
		if panelReady.Load() && wParam&0xFFFF != 0 {
			panelChromium.Focus()
		}
	case wmClose:
		// Closing only hides: the main program brings the window back.
		showWindow.Call(hwnd, swHide)
		return 0
	case wmGetMinMaxInfo:
		dpi := uintptr(96)
		if getDpiForWindow.Find() == nil {
			if d, _, _ := getDpiForWindow.Call(hwnd); d != 0 {
				dpi = d
			}
		}
		// MINMAXINFO: ptReserved, ptMaxSize, ptMaxPosition, ptMinTrackSize, ...
		mmi := *(*unsafe.Pointer)(unsafe.Pointer(&lParam)) // here lParam is a MINMAXINFO*
		minTrack := (*[2]int32)(unsafe.Add(mmi, 24))
		minTrack[0], minTrack[1] = int32(420*dpi/96), int32(560*dpi/96)
		return 0
	}
	r, _, _ := defWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

// A COM event handler: a pointer to its vtable (IUnknown, then Invoke).
type comHandler struct{ vtbl *[4]uintptr }

var (
	handlerQI      = windows.NewCallback(func(this, _, out unsafe.Pointer) uintptr { *(*unsafe.Pointer)(out) = this; return 0 })
	handlerRef     = windows.NewCallback(func(unsafe.Pointer) uintptr { return 1 })
	navStartingFns = [4]uintptr{handlerQI, handlerRef, handlerRef, windows.NewCallback(onNavStarting)}
	newWindowFns   = [4]uintptr{handlerQI, handlerRef, handlerRef, windows.NewCallback(onNewWindow)}
	navStarting    = comHandler{&navStartingFns}
	newWindow      = comHandler{&newWindowFns}
)

// onNavStarting is ICoreWebView2NavigationStartingEventHandler.Invoke. A page
// that isn't the panel never loads in the window; a click on one opens it in
// the default browser.
func onNavStarting(_, _, args unsafe.Pointer) uintptr {
	uri := comString(args, 3) // get_Uri
	if stays(uri, panelURL) {
		return 0
	}
	var user int32
	comCall(args, 4, uintptr(unsafe.Pointer(&user))) // get_IsUserInitiated
	comCall(args, 8, 1)                              // put_Cancel(TRUE)
	if user != 0 && outside(uri) {
		openOutside(uri)
	}
	return 0
}

// onNewWindow is ICoreWebView2NewWindowRequestedEventHandler.Invoke: no
// popup ever opens, and the address goes to the default browser. A blank
// window.open gets null back, so the panel shows its link instead.
func onNewWindow(_, _, args unsafe.Pointer) uintptr {
	uri := comString(args, 3) // get_Uri
	comCall(args, 6, 1)       // put_Handled(TRUE)
	var user int32
	comCall(args, 8, uintptr(unsafe.Pointer(&user))) // get_IsUserInitiated
	if user != 0 && outside(uri) {
		openOutside(uri)
	}
	return 0
}

func openOutside(uri string) {
	windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(uri), nil, nil, windows.SW_SHOWNORMAL)
}

// comCall calls method idx of the COM object obj.
func comCall(obj unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

// comString calls a getter that hands back a string WebView2 allocated.
func comString(obj unsafe.Pointer, idx int) string {
	var p *uint16
	if comCall(obj, idx, uintptr(unsafe.Pointer(&p))) != 0 || p == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}
