//go:build darwin || linux

package desktop

import (
	"os"
	"os/signal"
	"syscall"
)

// watchSignals runs stop on Ctrl+C in a terminal, or when the system asks
// the program to end.
func watchSignals(stop func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		stop()
	}()
}
