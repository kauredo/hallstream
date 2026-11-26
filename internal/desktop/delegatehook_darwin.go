package desktop

/*
#include <dispatch/dispatch.h>
#include <objc/message.h>
#include <objc/runtime.h>

extern void hallstreamReopen(void);
extern void hallstreamQuit(void);

static BOOL shouldHandleReopen(id self, SEL cmd, id app, BOOL visible) {
	hallstreamReopen();
	return NO;
}

// shouldTerminate answers NSTerminateLater (2): the program runs "Sair",
// which asks first while the stream is live, and answers through
// replyTerminate.
static unsigned long shouldTerminate(id self, SEL cmd, id app) {
	hallstreamQuit();
	return 2;
}

static void reply(void *yes) {
	id app = ((id (*)(Class, SEL))objc_msgSend)(objc_getClass("NSApplication"), sel_registerName("sharedApplication"));
	((void (*)(id, SEL, BOOL))objc_msgSend)(app, sel_registerName("replyToApplicationShouldTerminate:"), yes != NULL);
}

// replyTerminate gives AppKit the answer on the main thread. AppKit waits
// for it with the run loop in NSModalPanelRunLoopMode, one of the common
// modes, and the main queue runs in all of them.
static void replyTerminate(int yes) {
	dispatch_async_f(dispatch_get_main_queue(), yes ? (void *)1 : NULL, reply);
}

// addToDelegate adds a handler to fyne.io/systray's app delegate class. It
// runs before systray sets the delegate, which is when AppKit checks which
// handlers the delegate has. Adding the same handler again succeeds; 0 means
// systray changed (the class is gone, or has a handler of its own).
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
	if (class_addMethod(c, sel, imp, d.types)) {
		return 1;
	}
	return method_getImplementation(class_getInstanceMethod(c, sel)) == imp;
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

// replyTerminate answers macOS's applicationShouldTerminate: true ends the
// app, false keeps it running.
func replyTerminate(yes bool) {
	v := C.int(0)
	if yes {
		v = 1
	}
	C.replyTerminate(v)
}
