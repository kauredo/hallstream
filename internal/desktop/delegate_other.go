//go:build windows || linux

package desktop

// onReopen does nothing: only macOS sends a running app a reopen event.
func onReopen(f func()) bool { return true }

// onQuit does nothing: only macOS has a Dock menu with its own Quit.
func onQuit(f func()) bool { return true }
