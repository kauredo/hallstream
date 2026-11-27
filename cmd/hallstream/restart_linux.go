package main

import (
	"os/exec"
	"syscall"
)

// startCopy starts the updated program in a session of its own, so closing
// this one (or its terminal) doesn't close it.
func startCopy(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
