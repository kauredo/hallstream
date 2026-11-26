package desktop

/*
#include <objc/runtime.h>

extern void hallstreamReopen(void);

static BOOL shouldHandleReopen(id self, SEL cmd, id app, BOOL visible) {
	hallstreamReopen();
	return NO;
}

// hookReopen adds the reopen handler to fyne.io/systray's app delegate
// class. It runs before systray sets the delegate, which is when AppKit
// checks which handlers the delegate has.
static int hookReopen(void) {
	Class c = objc_getClass("SystrayAppDelegate");
	Protocol *p = objc_getProtocol("NSApplicationDelegate");
	if (c == Nil || p == NULL) {
		return 0;
	}
	SEL sel = sel_registerName("applicationShouldHandleReopen:hasVisibleWindows:");
	struct objc_method_description d = protocol_getMethodDescription(p, sel, NO, YES);
	if (d.types == NULL) {
		return 0;
	}
	return class_addMethod(c, sel, (IMP)shouldHandleReopen, d.types);
}
*/
import "C"

func hookReopen() bool { return C.hookReopen() != 0 }
