package ble

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLikely(t *testing.T) {
	cases := []struct {
		name     string
		services []string
		want     bool
	}{
		{" Livestream Stramatel", nil, true},
		{"Livestream Stramatel", nil, true},
		{"  livestream stramatel  ", nil, true},
		{"LIVESTREAM", nil, true},
		{"SL Live Stream", nil, true},
		{"Stramatel", nil, true},
		{"", []string{"a002"}, true},
		{"", []string{"180f", "A002"}, true},
		{"", nil, false},
		{"JBL Flip 5", []string{"fe8f"}, false},
		{"Galaxy S21", nil, false},
		{"Live", nil, false},
	}
	for _, c := range cases {
		if got := Likely(c.name, c.services); got != c.want {
			t.Errorf("Likely(%q, %v) = %v, want %v", c.name, c.services, got, c.want)
		}
	}
}

func TestDevicesOrderAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC)
	r := New(&Reader{Now: func() time.Time { return now }})
	r.seen(Advert{Address: "AA", Name: "Phone", RSSI: -40})
	r.seen(Advert{Address: "BB", Name: "Livestream Stramatel", RSSI: -80})
	r.seen(Advert{Address: "CC", RSSI: -50, Services: []string{"a002"}})
	r.seen(Advert{Address: "DD", RSSI: -90})
	// A later packet without the name keeps the name.
	r.seen(Advert{Address: "AA", RSSI: -45})

	got := r.Devices()
	order := ""
	for _, d := range got {
		order += d.Address + " "
	}
	if order != "CC BB AA DD " {
		t.Fatalf("order = %q", order)
	}
	if got[2].Name != "Phone" || got[2].RSSI != -45 || got[2].NameQuoted != `"Phone"` {
		t.Errorf("AA = %+v", got[2])
	}

	now = now.Add(45 * time.Second)
	r.seen(Advert{Address: "DD", RSSI: -70})
	now = now.Add(20 * time.Second)
	got = r.Devices()
	if len(got) != 1 || got[0].Address != "DD" {
		t.Fatalf("after 65 s: %+v", got)
	}
}

// fakeRadio hears the adverts in ads on every scan and connects per boxes.
type fakeRadio struct {
	ads   []Advert
	boxes map[string]error // address → Connect error (nil = it is the box)

	mu       sync.Mutex
	stop     chan struct{}
	tried    []string
	links    []*fakeLink
	scanning bool
}

func (f *fakeRadio) Enable() error { return nil }

func (f *fakeRadio) Scan(found func(Advert)) error {
	f.mu.Lock()
	f.stop = make(chan struct{})
	stop := f.stop
	f.scanning = true
	f.mu.Unlock()
	for _, a := range f.ads {
		found(a)
	}
	<-stop
	return nil
}

func (f *fakeRadio) StopScan() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.scanning {
		return errors.New("not scanning")
	}
	f.scanning = false
	close(f.stop)
	return nil
}

func (f *fakeRadio) Connect(addr string) (Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tried = append(f.tried, addr)
	err, ok := f.boxes[addr]
	if !ok {
		err = ErrNotBox
	}
	if err != nil {
		return nil, err
	}
	l := &fakeLink{up: true}
	f.links = append(f.links, l)
	return l, nil
}

type fakeLink struct {
	mu sync.Mutex
	up bool
}

func (l *fakeLink) Listen(frame func([]byte)) error { frame([]byte{1}); return nil }
func (l *fakeLink) Connected() bool                 { l.mu.Lock(); defer l.mu.Unlock(); return l.up }
func (l *fakeLink) Disconnect()                     { l.mu.Lock(); l.up = false; l.mu.Unlock() }

func TestTryNotTheBoxThenTheBox(t *testing.T) {
	radio := &fakeRadio{
		ads:   []Advert{{Address: "11:11", Name: "JBL Flip 5", RSSI: -40}, {Address: "22:22", RSSI: -70}},
		boxes: map[string]error{"22:22": nil},
	}
	var mu sync.Mutex
	saved := ""
	r := New(&Reader{
		Radio: radio,
		Saved: func() string { mu.Lock(); defer mu.Unlock(); return saved },
		Save:  func(a string) { mu.Lock(); saved = a; mu.Unlock() },
		Round: time.Hour,
	})
	go r.Run()

	if err := r.Try("11:11"); !errors.Is(err, ErrNotBox) {
		t.Fatalf("Try(not the box) = %v", err)
	}
	mu.Lock()
	got := saved
	mu.Unlock()
	if got != "" {
		t.Fatalf("saved %q after a failed try", got)
	}
	if err := r.Try("22:22"); err != nil {
		t.Fatalf("Try(box) = %v", err)
	}
	mu.Lock()
	got = saved
	mu.Unlock()
	if got != "22:22" {
		t.Fatalf("saved = %q", got)
	}
	if err := r.Forget(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got = saved
	mu.Unlock()
	if got != "" {
		t.Fatalf("after Forget saved = %q", got)
	}
	radio.mu.Lock()
	defer radio.mu.Unlock()
	if len(radio.links) != 1 || radio.links[0].Connected() {
		t.Fatalf("the box link should be dropped on Forget: %+v", radio.links)
	}
}

func TestAutoConnectsStrongestLikely(t *testing.T) {
	radio := &fakeRadio{
		ads: []Advert{
			{Address: "far", Name: "Livestream Stramatel", RSSI: -90},
			{Address: "near", Name: "Livestream Stramatel", RSSI: -50},
			{Address: "phone", Name: "Galaxy", RSSI: -30},
		},
		boxes: map[string]error{"near": nil, "far": nil},
	}
	connected := make(chan bool, 4)
	r := New(&Reader{Radio: radio, Connected: func(b bool) { connected <- b }, Settle: 10 * time.Millisecond})
	go r.Run()
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("never connected")
	}
	radio.mu.Lock()
	defer radio.mu.Unlock()
	if len(radio.tried) != 1 || radio.tried[0] != "near" {
		t.Fatalf("tried %v", radio.tried)
	}
}

func TestCloseDropsTheBox(t *testing.T) {
	radio := &fakeRadio{ads: []Advert{{Address: "box", Name: "Livestream Stramatel", RSSI: -50}}, boxes: map[string]error{"box": nil}}
	connected := make(chan bool, 4)
	r := New(&Reader{Radio: radio, Connected: func(b bool) { connected <- b }, Settle: 10 * time.Millisecond})
	go r.Run()
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("never connected")
	}
	r.Close()
	radio.mu.Lock()
	defer radio.mu.Unlock()
	if len(radio.links) != 1 || radio.links[0].Connected() {
		t.Fatalf("Close left the box connected: %+v", radio.links)
	}
}
