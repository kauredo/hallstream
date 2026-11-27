//go:build !darwin

package desktop

// installMenus does nothing: only macOS has an app menu bar.
func installMenus() {}
