package desktop

import "C"

var (
	reopen    func()
	quitAsked func(reply func(yes bool))
)

// onReopen runs f when macOS sends the running app a reopen event: a click
// on its Dock icon, or opening it again from Finder or Launchpad.
func onReopen(f func()) bool {
	reopen = f
	return hookReopen()
}

// onQuit runs f when macOS asks to end the app: Quit in the Dock menu, or
// signing out. macOS waits until f calls reply, once: true ends the app,
// false keeps it running.
func onQuit(f func(reply func(yes bool))) bool {
	quitAsked = f
	return hookTerminate()
}

//export hallstreamReopen
func hallstreamReopen() {
	if f := reopen; f != nil {
		go f()
	}
}

//export hallstreamQuit
func hallstreamQuit() {
	f := quitAsked
	if f == nil {
		replyTerminate(true)
		return
	}
	go f(replyTerminate)
}
