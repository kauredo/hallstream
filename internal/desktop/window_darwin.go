package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>

// window_darwin.m: the panel's NSWindow and WKWebView.
void showPanel(const char *url);
*/
import "C"

import (
	"sync"
	"unsafe"
)

// HasWindow says the panel opens in the app's own window, not a browser. Only
// inside Hallstream.app: outside it there is no Info.plist to allow loopback
// HTTP, so the web view would stay blank.
var HasWindow = !Console

var (
	panelMu  sync.Mutex
	panelURL string
)

// ShowPanel opens the panel at url in the app's window, or brings the window
// back, once AppKit runs.
func ShowPanel(url string) bool {
	if !HasWindow {
		return false
	}
	panelMu.Lock()
	panelURL = url
	panelMu.Unlock()
	c := C.CString(url)
	defer C.free(unsafe.Pointer(c))
	C.showPanel(c)
	return true
}

//export hallstreamInWindow
func hallstreamInWindow(url *C.char) C.int {
	panelMu.Lock()
	panel := panelURL
	panelMu.Unlock()
	if inWindow(C.GoString(url), panel) {
		return 1
	}
	return 0
}
