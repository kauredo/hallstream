//go:build !windows

package desktop

// RunWindow is Windows' panel window process; elsewhere it can't start.
func RunWindow(string) int { return 2 }

// AllowForeground is for Windows, which keeps a program in the background
// from bringing its window to the front.
func AllowForeground() {}
