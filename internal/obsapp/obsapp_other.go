//go:build !windows

package obsapp

// System knows only where the config file is: starting and closing OBS is
// for Windows.
func System() App { return App{ConfigPath: configPath()} }
