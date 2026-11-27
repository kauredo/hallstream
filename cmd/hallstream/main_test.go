package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/state"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestSafelyRestartsAfterPanic(t *testing.T) {
	var log lockedBuffer
	oldOut, oldMin := logOut, restartMin
	logOut, restartMin = &log, 5*time.Millisecond
	defer func() { logOut, restartMin = oldOut, oldMin }()

	calls := make(chan time.Time, 4)
	n := 0
	safely("Câmara", func() {
		n++
		calls <- time.Now()
		if n < 3 {
			panic("boom")
		}
	})
	var at [3]time.Time
	for i := range at {
		select {
		case at[i] = <-calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("call %d never came: the loop did not start again after the panic", i+1)
		}
	}
	if first, second := at[1].Sub(at[0]), at[2].Sub(at[1]); first < restartMin || second < 2*restartMin {
		t.Errorf("pauses of %v then %v, want at least %v then twice that", first, second, restartMin)
	}
	select {
	case <-calls:
		t.Fatal("started again after a run that ended without a panic")
	case <-time.After(50 * time.Millisecond):
	}
	out := log.String()
	if strings.Count(out, "Câmara: erro inesperado (boom)") != 2 {
		t.Errorf("want both panics in the log with the loop's name, got:\n%s", out)
	}
	if !strings.Contains(out, "main_test.go") {
		t.Errorf("want the stack in the log, got:\n%s", out)
	}
}

func TestTrayStatus(t *testing.T) {
	var s state.State
	cases := []struct {
		set  func(*state.State)
		want string
	}{
		{func(s *state.State) {}, "Caixa: à procura"},
		{func(s *state.State) { s.Box.Scan.Bluetooth = "off" }, "Caixa: Bluetooth desligado"},
		{func(s *state.State) { s.Box.Scan.Bluetooth = "unavailable" }, "Caixa: Bluetooth desligado"},
		{func(s *state.State) { s.Box.Connected = true }, "Caixa: ligada"},
		{func(s *state.State) { s.Demo = true }, "Modo demonstração"},
		{func(s *state.State) { s.Input = "manual" }, "Marcação manual: à espera do telemóvel"},
		{func(s *state.State) { s.Input, s.Box.Scan.Bluetooth = "manual", "off" }, "Marcação manual: à espera do telemóvel"},
		{func(s *state.State) { s.Input, s.Box.Connected = "manual", true }, "Marcação manual: ligada"},
		{func(s *state.State) { s.Input, s.Demo = "manual", true }, "Modo demonstração"},
		{func(s *state.State) { s.Input = "serial" }, "Consola por cabo: à espera de dados"},
		{func(s *state.State) { s.Input, s.Box.Connected = "serial", true }, "Consola por cabo: a ler"},
		{func(s *state.State) { s.Input, s.Demo = "serial", true }, "Modo demonstração"},
	}
	for _, c := range cases {
		c.set(&s)
		if got := trayStatus(s); got != c.want {
			t.Errorf("trayStatus(%+v) = %q, want %q", s.Box, got, c.want)
		}
		s = state.State{}
	}
}
