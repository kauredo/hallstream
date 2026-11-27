package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/web"
)

// waitForOldCopy waits until no Hallstream answers on its ports, so a copy
// started by an update doesn't find the old one still closing and hand over
// to it. After max it carries on, and the usual "already open" check decides.
func waitForOldCopy(ours func(addr string) bool, max time.Duration) {
	for end := time.Now().Add(max); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		busy := false
		for p := web.FirstPort; p <= web.LastPort && !busy; p++ {
			busy = ours(fmt.Sprintf("127.0.0.1:%d", p))
		}
		if !busy {
			return
		}
	}
}

// restartArgs are the new copy's arguments: this copy's, without the
// browser (the panel is already open) and marked as a restart.
func restartArgs(args []string) []string {
	var out []string
	for _, a := range args {
		switch strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-") {
		case "restarted", "nobrowser":
			continue
		}
		out = append(out, a)
	}
	return append(out, "-nobrowser", "-restarted")
}
