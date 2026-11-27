package main

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"time"

	"go.bug.st/serial"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
)

// openSerial opens a console's port at baud, 8N1, and names what went wrong
// in words the panel can show.
func openSerial(port string, baud int) (io.ReadCloser, error) {
	p, err := serial.Open(port, &serial.Mode{BaudRate: baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err == nil {
		return p, nil
	}
	key := i18n.Key("api.serial.nao_abri")
	var pe *serial.PortError
	if errors.As(err, &pe) {
		switch pe.Code() {
		case serial.PortBusy:
			key = i18n.Key("api.serial.ocupada")
		case serial.PortNotFound:
			key = i18n.Key("api.serial.nao_existe")
		case serial.PermissionDenied:
			key = i18n.Key("api.serial.sem_permissao")
		}
	}
	return nil, &source.OpenError{Key: key, Err: err}
}

// serialPorts lists the ports. On a Mac each port is there twice, and only
// the cu.* one opens without waiting for a modem line.
func serialPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if runtime.GOOS != "darwin" {
		return ports, err
	}
	out := []string{}
	for _, p := range ports {
		if strings.HasPrefix(p, "/dev/cu.") {
			out = append(out, p)
		}
	}
	return out, err
}

// replayCable makes the cable read a recording instead of a port, with the
// recording's protocol and sport: the first open gets a pipe that the
// recording is played into at its pace. The pipe stays open after the last
// frame, so the score stays on air and the cable never goes quiet.
func replayCable(c *source.Serial, head map[string]string, frames []ble.Frame) {
	proto, sport, gameSport := head["protocol"], head["sport"], c.Sport
	c.Settings = func() (string, string) { return "replay", proto }
	if sport != "" {
		c.Sport = func() string { return sport }
	} else {
		c.Sport = gameSport
	}
	c.Quiet = 100 * 365 * 24 * time.Hour
	opened := false
	c.Open = func(string, int) (io.ReadCloser, error) {
		if opened {
			return nil, i18n.Err("log.replay.fim")
		}
		opened = true
		r, w := io.Pipe()
		safely(i18n.T("log.tarefa.replay"), func() {
			ble.Play(frames, func(b []byte) { w.Write(b) }, func(d time.Duration) bool { time.Sleep(d); return true })
			say(i18n.T("log.replay.fim"))
		})
		return r, nil
	}
}
