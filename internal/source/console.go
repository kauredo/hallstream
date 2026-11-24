package source

import (
	"sync"

	"github.com/kauredo/hallstream/internal/ble"
)

// Console is the Stramatel box: frames from ble.Reader, decoded. The reader
// keeps running, and the box connected, while another source is on, so the
// panel's Caixa tab still works and the box is ready when the console comes
// back; its frames are dropped meanwhile.
type Console struct {
	Raw func([]byte) // every frame used, for raw.log

	frames    chan []byte
	mu        sync.Mutex
	vals      map[string]string // the decoder's values, kept across frames and runs
	linked    bool              // the reader's last word on the link
	connected func(bool)        // the newest Run's; nil while none runs
	runs      int               // Run calls so far
}

// NewConsole fills in the defaults.
func NewConsole(c *Console) *Console {
	if c.Raw == nil {
		c.Raw = func([]byte) {}
	}
	c.frames = make(chan []byte, 64)
	c.vals = map[string]string{}
	return c
}

func (c *Console) Name() string { return "console" }

// Frame is ble.Reader.Frame. It never blocks the Bluetooth stack: with the
// console off, or too far behind, the frame is dropped.
func (c *Console) Frame(f []byte) {
	select {
	case c.frames <- f:
	default:
	}
}

// Connected is ble.Reader.Connected.
func (c *Console) Connected(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.linked = on
	if c.connected != nil {
		c.connected(on)
	}
}

func (c *Console) Run(stop <-chan struct{}, out func(map[string]string), connected func(bool)) {
	for old := true; old; { // drop the frames from before this run
		select {
		case <-c.frames:
		default:
			old = false
		}
	}
	run := c.attach(connected)
	defer c.detach(run)
	for {
		select {
		case <-stop:
			return
		case f := <-c.frames:
			c.frame(f, out)
		}
	}
}

func (c *Console) attach(connected func(bool)) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs++
	c.connected = connected
	connected(c.linked)
	return c.runs
}

func (c *Console) detach(run int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runs == run {
		c.connected = nil
	}
}

func (c *Console) frame(f []byte, out func(map[string]string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Raw(f)
	ble.Decode(f, c.vals)
	out(c.vals)
}
