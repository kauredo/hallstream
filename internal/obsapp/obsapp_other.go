//go:build !windows && !darwin && !linux

package obsapp

// System knows only where the config file is: starting and closing OBS is
// for Windows, macOS and Linux.
func System() App { return App{ConfigPath: configPath()} }
