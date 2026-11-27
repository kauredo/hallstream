package desktop

import "net/url"

// inWindow says whether the panel's window should follow a link to target,
// where panel is the panel's own address. Anything on another scheme, host or
// port leaves for the default browser.
func inWindow(target, panel string) bool {
	t, err := url.Parse(target)
	if err != nil {
		return false
	}
	p, err := url.Parse(panel)
	if err != nil {
		return false
	}
	return t.Host != "" && t.Scheme == p.Scheme && t.Host == p.Host
}
