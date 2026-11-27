// Package source is where the live score comes from: the Stramatel box over
// Bluetooth (Console) or a phone keeping it by hand (Manual). One source
// runs at a time, and Switch swaps them while the program runs.
//
// The demo is not a source: it lives in internal/state, which mutes the
// timeline, period saves and history while it plays and lets the panel turn
// it on and off over whichever source is running.
package source

import (
	"sync"

	"github.com/kauredo/hallstream/internal/state"
)

// Source sends the score as console values: the keys of
// state.Machine.ApplyConsole (home, away, clock, period, home_fouls, …).
type Source interface {
	// Name is the source's settings.input value.
	Name() string
	// Run sends values with out, and whether the source is connected (the
	// box linked, the phone calling in) with connected, until stop closes. A
	// source may also call both from other goroutines, one call at a time.
	Run(stop <-chan struct{}, out func(map[string]string), connected func(bool))
}

// seeded is a source that keeps its own score and can start from the one on air.
type seeded interface {
	Seed(state.Live)
}

// Switch runs one of Sources at a time.
type Switch struct {
	Sources []Source
	State   *state.Machine
	// Values gets each set of values before the state does (score/*.txt).
	Values func(map[string]string)
	// Go starts a background loop; main passes safely, so a panic in a
	// source goes to the log and the source starts again.
	Go func(name string, fn func())

	use  sync.Mutex // one Use at a time
	mu   sync.Mutex // cur and stop
	cur  Source
	stop chan struct{}
}

// Use runs the source called name and stops the one before. An unknown name
// runs the first source. A source with its own score (manual) that takes over
// from another starts from the score on air, unless the demo is playing.
func (w *Switch) Use(name string) {
	src := w.Sources[0]
	for _, s := range w.Sources {
		if s.Name() == name {
			src = s
		}
	}
	w.use.Lock()
	defer w.use.Unlock()
	w.mu.Lock()
	prev := w.cur
	w.mu.Unlock()
	if src == prev {
		return
	}
	if s, ok := src.(seeded); ok && prev != nil {
		if snap := w.State.Snapshot(); !snap.Demo {
			s.Seed(snap.Live)
		}
	}
	w.start(src)
}

// Restart stops the running source and starts it again, so it reads its
// settings afresh (the cable's port and protocol).
func (w *Switch) Restart() {
	w.use.Lock()
	defer w.use.Unlock()
	w.mu.Lock()
	cur := w.cur
	w.mu.Unlock()
	if cur != nil {
		w.start(cur)
	}
}

// start runs src in place of the current source; w.use held.
func (w *Switch) start(src Source) {
	w.State.SetInput(src.Name())

	stop := make(chan struct{})
	w.mu.Lock()
	if w.stop != nil {
		close(w.stop)
	}
	w.cur, w.stop = src, stop
	w.mu.Unlock()
	start := w.Go
	if start == nil {
		start = func(_ string, fn func()) { go fn() }
	}
	start(src.Name(), func() {
		select {
		case <-stop:
			return // stopped before it started again after a panic
		default:
		}
		src.Run(stop, func(v map[string]string) { w.out(stop, v) }, func(on bool) { w.connected(stop, on) })
	})
}

// out passes on the values of the run that stop belongs to, while it is the
// current one. A source swapped out may still send for a moment.
func (w *Switch) out(stop chan struct{}, v map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop != stop {
		return
	}
	if w.Values != nil {
		w.Values(v)
	}
	w.State.ApplyConsole(v)
}

func (w *Switch) connected(stop chan struct{}, on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop == stop {
		w.State.SetBox(on)
	}
}
