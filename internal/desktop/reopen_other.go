//go:build windows || linux

package desktop

// onReopen does nothing: only macOS sends a running app a reopen event.
func onReopen(f func()) bool { return true }
