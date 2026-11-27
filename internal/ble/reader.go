package ble

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/state"
)

// Radio is the Bluetooth adapter, behind an interface so tests can fake it.
// Adapter() returns the real one.
type Radio interface {
	Enable() error
	// Scan calls found for every advertisement until StopScan.
	Scan(found func(Advert)) error
	StopScan() error
	// Connect connects to the device and finds the scoreboard channel. It
	// returns ErrNotBox when the device has no such channel.
	Connect(address string) (Link, error)
}

// Link is a connection to the box.
type Link interface {
	// Listen starts the frame notifications.
	Listen(frame func([]byte)) error
	Connected() bool
	Disconnect()
}

// Advert is one advertisement heard during a scan.
type Advert struct {
	Address  string
	Name     string
	RSSI     int
	Services []string // "a002" for 16-bit UUIDs, the full UUID otherwise
}

// Device is a device heard in the last DeviceTTL, for the panel.
type Device struct {
	Address    string    `json:"address"`
	Name       string    `json:"name"`
	NameQuoted string    `json:"name_quoted"` // shows leading and trailing spaces
	RSSI       int       `json:"rssi"`
	Services   []string  `json:"services"`
	Likely     bool      `json:"likely"`
	LastSeen   time.Time `json:"last_seen"`
}

var (
	ErrNotBox  error = i18n.Key("ble.nao_e_caixa")
	errTimeout error = i18n.Key("ble.sem_resposta")
	errNoBT    error = i18n.Key("ble.desligado")
)

const (
	DeviceTTL = 60 * time.Second
	logEvery  = 30 * time.Second
	btRetry   = 10 * time.Second
)

// Likely reports whether an advertisement looks like the Stream Box. The
// Android app looks for " Livestream Stramatel" exactly; Windows may trim or
// change it, so any of the words will do, or the scoreboard service.
func Likely(name string, services []string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, w := range []string{"stramatel", "livestream", "live stream"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	for _, s := range services {
		if strings.EqualFold(s, "a002") {
			return true
		}
	}
	return false
}

// Reader finds the box, keeps it connected and lists what it hears.
type Reader struct {
	Radio     Radio
	Frame     func([]byte)
	Connected func(bool)
	Log       func(string)     // console
	Status    func(state.Scan) // live state
	Saved     func() string    // the saved box address
	Save      func(string)
	DevLog    io.Writer // dados/bluetooth.log
	Now       func() time.Time
	// Capture records the raw frames when the panel's switch is on; nil
	// records nothing.
	Capture *Recorder

	// Round is one scan; Settle is how long to keep listening after the
	// first likely box, to pick the strongest; Timeout bounds a connection.
	Round, Settle, Timeout time.Duration

	reqs chan request

	mu       sync.Mutex
	devices  map[string]*Device
	logged   map[string]time.Time
	rejected map[string]bool // auto-connect skips these: not the box
	heard    int             // adverts in this round
	empty    int             // rounds in a row with nothing heard
	target   string          // saved address the scan waits for
	hit      chan struct{}
	scan     state.Scan
	link     Link // the box connected now
}

type request struct {
	addr string // "" forgets the saved box
	done chan error
}

// New fills in the defaults.
func New(r *Reader) *Reader {
	if r.Log == nil {
		r.Log = func(string) {}
	}
	if r.Status == nil {
		r.Status = func(state.Scan) {}
	}
	if r.Frame == nil {
		r.Frame = func([]byte) {}
	}
	if r.Connected == nil {
		r.Connected = func(bool) {}
	}
	if r.Saved == nil {
		r.Saved = func() string { return "" }
	}
	if r.Save == nil {
		r.Save = func(string) {}
	}
	if r.DevLog == nil {
		r.DevLog = io.Discard
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Round == 0 {
		r.Round = 15 * time.Second
	}
	if r.Settle == 0 {
		r.Settle = 2 * time.Second
	}
	if r.Timeout == 0 {
		r.Timeout = 15 * time.Second
	}
	r.reqs = make(chan request)
	r.devices = map[string]*Device{}
	r.logged = map[string]time.Time{}
	r.rejected = map[string]bool{}
	r.scan = state.Scan{Bluetooth: "ok"}
	return r
}

// Devices returns what was heard in the last minute: likely boxes first,
// then by signal.
func (r *Reader) Devices() []Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prune()
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		c := *d
		c.Services = append([]string{}, d.Services...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Likely != b.Likely {
			return a.Likely
		}
		if a.RSSI != b.RSSI {
			return a.RSSI > b.RSSI
		}
		return a.Address < b.Address
	})
	return out
}

// Try connects to one device by hand. If it is the box it stays connected
// and its address is saved.
func (r *Reader) Try(addr string) error {
	return r.ask(request{addr: addr, done: make(chan error, 1)})
}

// Forget clears the saved box and goes back to looking for one.
func (r *Reader) Forget() error {
	return r.ask(request{done: make(chan error, 1)})
}

func (r *Reader) ask(req request) error {
	timeout := time.After(r.Timeout + 2*time.Second)
	select {
	case r.reqs <- req:
	case <-timeout:
		return errTimeout
	}
	select {
	case err := <-req.done:
		return err
	case <-timeout:
		return errTimeout
	}
}

// Run keeps the box connected forever.
func (r *Reader) Run() {
	r.Log(i18n.T("ble.a_procura_box"))
	enabled := false
	var req *request
	for {
		if !enabled {
			if err := r.Radio.Enable(); err != nil {
				msg := i18n.T("ble.desligado")
				if runtime.GOOS == "darwin" {
					// macOS gives the same error for a radio that is off and a refused permission.
					msg = i18n.T("ble.sem_autorizacao_mac")
				}
				r.setScan(func(s *state.Scan) { *s = state.Scan{Bluetooth: "unavailable", Message: msg} })
				r.Log(i18n.T("ble.desligado_repete", "err", err))
				r.idle(btRetry)
				continue
			}
			enabled = true
		}
		if req == nil {
			req = r.auto()
		}
		if req != nil {
			req = r.handle(*req)
		}
	}
}

// idle waits, answering any request with the Bluetooth error.
func (r *Reader) idle(d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return
		case req := <-r.reqs:
			if req.addr == "" {
				r.forget()
				req.done <- nil
			} else {
				req.done <- errNoBT
			}
		}
	}
}

// auto connects to the saved box, or scans for a likely one.
func (r *Reader) auto() *request {
	if saved := r.Saved(); saved != "" {
		if link, err := r.connect(saved); err == nil {
			return r.follow(saved, link)
		}
	}
	addr, req, err := r.scanRound()
	if req != nil {
		return req
	}
	if err != nil {
		r.setScan(func(s *state.Scan) { s.Scanning, s.Bluetooth, s.Message = false, "off", i18n.T("ble.desligado") })
		r.Log(i18n.T("ble.desligado_erro", "err", err))
		r.idle(btRetry)
		return nil
	}
	if addr == "" {
		return nil
	}
	link, err := r.connect(addr)
	if err != nil {
		r.setScan(func(s *state.Scan) {
			s.Message = i18n.T("ble.nao_liguei_caixa")
		})
		r.Log(i18n.T("ble.nao_liguei_a", "endereco", addr, "err", err))
		return nil
	}
	return r.follow(addr, link)
}

// handle answers one request from the panel.
func (r *Reader) handle(req request) *request {
	if req.addr == "" {
		r.forget()
		req.done <- nil
		return nil
	}
	link, err := r.connect(req.addr)
	req.done <- err
	if err != nil {
		return nil
	}
	return r.follow(req.addr, link)
}

func (r *Reader) forget() {
	r.Save("")
	r.mu.Lock()
	r.rejected = map[string]bool{}
	r.mu.Unlock()
	r.setScan(func(s *state.Scan) { s.Saved = "" })
	r.Log(i18n.T("ble.esqueci"))
}

// scanRound listens for one round. It returns the address to connect to
// ("" when there is none), or a request that came in meanwhile.
func (r *Reader) scanRound() (string, *request, error) {
	saved := r.Saved()
	hit := make(chan struct{}, 1)
	r.mu.Lock()
	r.heard, r.target, r.hit = 0, saved, hit
	r.mu.Unlock()
	r.setScan(func(s *state.Scan) {
		s.Scanning, s.Bluetooth, s.Address, s.Name, s.Saved = true, "ok", "", "", saved
		if s.Message == "" || s.Message == i18n.T("ble.desligado") || s.Message == i18n.T("ble.sem_autorizacao_mac") {
			s.Message = i18n.T("ble.a_procura")
		}
	})

	done := make(chan error, 1)
	go func() { done <- r.Radio.Scan(r.seen) }()
	stop := func() {
		// StopScan fails if the scan has not started yet: keep asking.
		for {
			r.Radio.StopScan()
			select {
			case <-done:
				r.setScan(func(s *state.Scan) { s.Scanning = false })
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	round := time.NewTimer(r.Round)
	defer round.Stop()
	var settle <-chan time.Time
	for {
		select {
		case err := <-done:
			if err == nil {
				err = i18n.Err("ble.procura_parou")
			}
			return "", nil, err
		case req := <-r.reqs:
			stop()
			return "", &req, nil
		case <-hit:
			if settle == nil {
				settle = time.After(r.Settle)
			}
		case <-settle:
			stop()
			return r.best(saved), nil, nil
		case <-round.C:
			stop()
			r.endRound(saved)
			return "", nil, nil
		}
	}
}

// seen is the scan callback.
func (r *Reader) seen(a Advert) {
	now := r.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heard++
	d := r.devices[a.Address]
	fresh := d == nil || now.Sub(d.LastSeen) > DeviceTTL
	if d == nil {
		d = &Device{Address: a.Address}
		r.devices[a.Address] = d
	}
	// Names and services often come in separate packets: keep what we had.
	if a.Name != "" {
		d.Name = a.Name
	}
	if len(a.Services) > 0 {
		d.Services = append([]string{}, a.Services...)
	}
	d.NameQuoted = strconv.Quote(d.Name)
	d.RSSI, d.LastSeen = a.RSSI, now
	d.Likely = Likely(d.Name, d.Services)

	if now.Sub(r.logged[a.Address]) >= logEvery {
		r.logged[a.Address] = now
		fmt.Fprintf(r.DevLog, "%s %s %s rssi=%d services=%s likely=%t\n",
			now.Format("2006-01-02T15:04:05"), d.Address, d.NameQuoted, d.RSSI, strings.Join(d.Services, ","), d.Likely)
	}
	if fresh && d.Likely {
		r.Log(i18n.T("ble.vi_provavel", "nome", d.NameQuoted, "endereco", d.Address, "rssi", d.RSSI))
	}
	target := strings.EqualFold(a.Address, r.target) || (r.target == "" && d.Likely && !r.rejected[a.Address])
	if target && r.hit != nil {
		select {
		case r.hit <- struct{}{}:
		default:
		}
	}
	if n := len(r.devices); n != r.scan.DevicesSeen {
		r.scan.DevicesSeen = n
		r.Status(r.scan)
	}
}

// best picks the saved address, else the strongest likely box.
func (r *Reader) best(saved string) string {
	if saved != "" {
		return saved
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var pick *Device
	for _, d := range r.devices {
		if d.Likely && !r.rejected[d.Address] && (pick == nil || d.RSSI > pick.RSSI) {
			pick = d
		}
	}
	if pick == nil {
		return ""
	}
	return pick.Address
}

// endRound updates the message after a round that found nothing to connect to.
func (r *Reader) endRound(saved string) {
	r.mu.Lock()
	if r.heard == 0 {
		r.empty++
	} else {
		r.empty = 0
	}
	empty := r.empty
	r.prune()
	n := len(r.devices)
	r.mu.Unlock()

	msg := i18n.T("ble.a_procura")
	switch {
	case empty >= 2:
		msg = i18n.T("ble.nenhum_dispositivo")
	case saved != "":
		msg = i18n.T("ble.nao_encontro_guardada", "endereco", saved)
	case n > 0:
		msg = i18n.T("ble.vejo_dispositivos", "n", n)
	}
	r.setScan(func(s *state.Scan) { s.Message, s.DevicesSeen = msg, n })
	if empty >= 2 {
		r.Log(i18n.T("ble.nenhum_dispositivo"))
	} else {
		r.Log(i18n.T("ble.nao_encontrei"))
	}
}

// prune drops devices not heard for DeviceTTL. Call with mu held.
func (r *Reader) prune() {
	now := r.Now()
	for k, d := range r.devices {
		if now.Sub(d.LastSeen) > DeviceTTL {
			delete(r.devices, k)
		}
	}
}

// connect connects to addr and starts the frames, within Timeout.
func (r *Reader) connect(addr string) (Link, error) {
	r.setScan(func(s *state.Scan) { s.Scanning, s.Message = false, i18n.T("ble.a_ligar_a", "endereco", addr) })
	type result struct {
		link Link
		err  error
	}
	res := make(chan result, 1)
	go func() {
		link, err := r.Radio.Connect(addr)
		if err == nil {
			if err = link.Listen(r.onFrame); err != nil {
				link.Disconnect()
			}
		}
		res <- result{link, err}
	}()
	var out result
	select {
	case out = <-res:
	case <-time.After(r.Timeout):
		go func() {
			// Too late: drop the link if it ever comes up.
			if late := <-res; late.err == nil {
				late.link.Disconnect()
			}
		}()
		out.err = errTimeout
	}
	if out.err != nil {
		if errors.Is(out.err, ErrNotBox) {
			r.mu.Lock()
			r.rejected[addr] = true
			r.mu.Unlock()
		}
		r.setScan(func(s *state.Scan) { s.Message = "" })
		return nil, out.err
	}
	r.Save(addr)
	r.mu.Lock()
	delete(r.rejected, addr)
	r.mu.Unlock()
	return out.link, nil
}

// onFrame is what the link calls for every notification: the recorder gets
// the raw bytes first, and never blocks.
func (r *Reader) onFrame(f []byte) {
	r.Capture.Offer(f)
	r.Frame(f)
}

// Linked reports whether the box is connected now, and its name.
func (r *Reader) Linked() (bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.link != nil, r.scan.Name
}

// follow keeps the box connected until the link drops or the panel asks
// for something else.
func (r *Reader) follow(addr string, link Link) *request {
	name := ""
	r.mu.Lock()
	if d := r.devices[addr]; d != nil {
		name = strings.TrimSpace(d.Name)
	}
	r.mu.Unlock()
	r.setScan(func(s *state.Scan) {
		s.Scanning, s.Bluetooth, s.Address, s.Name, s.Saved = false, "ok", addr, name, addr
		s.Message = i18n.T("ble.ligado")
	})
	r.mu.Lock()
	r.link = link
	r.mu.Unlock()
	r.Connected(true)
	r.Log(i18n.T("ble.ligado_a", "endereco", addr))
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	drop := func() {
		r.mu.Lock()
		r.link = nil
		r.mu.Unlock()
		link.Disconnect()
		r.Connected(false)
		r.setScan(func(s *state.Scan) { s.Address, s.Name, s.Message = "", "", "" })
		r.Capture.StopNoWait("box") // after the link is told, and never waiting on a disk
	}
	for {
		select {
		case req := <-r.reqs:
			drop()
			return &req
		case <-tick.C:
			if !link.Connected() {
				drop()
				r.Log(i18n.T("ble.ligacao_caiu"))
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
}

// Close lets go of the box and stops scanning, before the program exits.
func (r *Reader) Close() {
	r.mu.Lock()
	link := r.link
	r.link = nil
	r.mu.Unlock()
	if link != nil {
		link.Disconnect()
	}
	r.Radio.StopScan()
}

func (r *Reader) setScan(fn func(*state.Scan)) {
	r.mu.Lock()
	fn(&r.scan)
	s := r.scan
	r.Status(s)
	r.mu.Unlock()
}
