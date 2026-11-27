package main

import (
	"os/exec"
	"syscall"
)

// startCopy starts the updated exe in its own process group, so it outlives
// this one.
func startCopy(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
