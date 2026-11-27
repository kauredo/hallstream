package desktop

import (
	"net/url"
	"strings"
)

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

// outside says whether a link may go to the default browser: http or https
// with a host, or mailto. Anything else (file:, javascript:, a custom scheme)
// is dropped, so a page can't start programs through the window.
func outside(target string) bool {
	t, err := url.Parse(target)
	if err != nil {
		return false
	}
	switch strings.ToLower(t.Scheme) {
	case "http", "https":
		return t.Host != ""
	case "mailto":
		return true
	}
	return false
}

// stays says whether the window loads target in place: the panel's own pages,
// a blank page, data: (the browser lets a page reach one only as a download)
// and the panel's own blob: files, which are downloads too.
func stays(target, panel string) bool {
	if inWindow(target, panel) || target == "about:blank" {
		return true
	}
	scheme, rest, _ := strings.Cut(target, ":")
	switch strings.ToLower(scheme) {
	case "data":
		return true
	case "blob":
		return inWindow(rest, panel)
	}
	return false
}
