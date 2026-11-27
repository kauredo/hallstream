//go:build !darwin

package desktop

// HasWindow is false: the panel opens in the browser here.
const HasWindow = false

// ShowPanel does nothing and says so; the caller opens the browser.
func ShowPanel(string) bool { return false }
