package desktop

import "C"

var reopen func()

// onReopen runs f when macOS sends the running app a reopen event: a click
// on its Dock icon, or opening it again from Finder or Launchpad.
func onReopen(f func()) bool {
	reopen = f
	return hookReopen()
}

//export hallstreamReopen
func hallstreamReopen() {
	if f := reopen; f != nil {
		go f()
	}
}
