package desktop

import "C"

var reopen, quitAsked func()

// onReopen runs f when macOS sends the running app a reopen event: a click
// on its Dock icon, or opening it again from Finder or Launchpad.
func onReopen(f func()) bool {
	reopen = f
	return hookReopen()
}

// onQuit runs f in place of macOS ending the app: Quit in the Dock menu,
// Cmd-Q, or signing out.
func onQuit(f func()) bool {
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
	if f := quitAsked; f != nil {
		go f()
	}
}
