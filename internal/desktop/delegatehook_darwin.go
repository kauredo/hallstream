package desktop

/*
#include <objc/runtime.h>

extern void hallstreamReopen(void);
extern void hallstreamQuit(void);

static BOOL shouldHandleReopen(id self, SEL cmd, id app, BOOL visible) {
	hallstreamReopen();
	return NO;
}

// shouldTerminate answers NSTerminateCancel (0): the program runs "Sair",
// which asks first when the stream is live, and ends the app itself.
static unsigned long shouldTerminate(id self, SEL cmd, id app) {
	hallstreamQuit();
	return 0;
}

// addToDelegate adds a handler to fyne.io/systray's app delegate class. It
// runs before systray sets the delegate, which is when AppKit checks which
// handlers the delegate has.
static int addToDelegate(const char *name, IMP imp) {
	Class c = objc_getClass("SystrayAppDelegate");
	Protocol *p = objc_getProtocol("NSApplicationDelegate");
	if (c == Nil || p == NULL) {
		return 0;
	}
	SEL sel = sel_registerName(name);
	struct objc_method_description d = protocol_getMethodDescription(p, sel, NO, YES);
	if (d.types == NULL) {
		return 0;
	}
	return class_addMethod(c, sel, imp, d.types);
}

static int hookReopen(void) {
	return addToDelegate("applicationShouldHandleReopen:hasVisibleWindows:", (IMP)shouldHandleReopen);
}

static int hookTerminate(void) {
	return addToDelegate("applicationShouldTerminate:", (IMP)shouldTerminate);
}
*/
import "C"

func hookReopen() bool    { return C.hookReopen() != 0 }
func hookTerminate() bool { return C.hookTerminate() != 0 }
