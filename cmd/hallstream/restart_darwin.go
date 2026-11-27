package main

import (
	"errors"
	"os/exec"
	"strings"
)

// startCopy opens a second instance of the updated app (-n: this one is
// still running) through LaunchServices, as a double-click would.
func startCopy(bundle string, args []string) error {
	out, err := exec.Command("/usr/bin/open", append([]string{"-n", "-a", bundle, "--args"}, args...)...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}
