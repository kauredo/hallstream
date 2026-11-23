package web

import (
	"testing"

	"github.com/kauredo/hallstream/internal/ble"
)

// noBoxRadio hears nothing and nothing it connects to is the box.
type noBoxRadio struct{ stop chan struct{} }

func (f *noBoxRadio) Enable() error                    { return nil }
func (f *noBoxRadio) Scan(func(ble.Advert)) error      { <-f.stop; return nil }
func (f *noBoxRadio) StopScan() error                  { f.stop <- struct{}{}; return nil }
func (f *noBoxRadio) Connect(string) (ble.Link, error) { return nil, ble.ErrNotBox }

func TestBluetoothTry(t *testing.T) {
	s := newServer(t)

	var devs []ble.Device
	if code := call(t, s, "GET", "/api/ble/devices", nil, &devs); code != 200 || len(devs) != 0 {
		t.Fatalf("devices without a reader: %d %v", code, devs)
	}

	s.BLE = ble.New(&ble.Reader{Radio: &noBoxRadio{stop: make(chan struct{})}})
	go s.BLE.Run()

	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if code := call(t, s, "POST", "/api/ble/try", map[string]string{"address": "AA:BB:CC:DD:EE:FF"}, &res); code != 200 {
		t.Fatalf("try %d", code)
	}
	if res.OK || res.Error != "Este dispositivo não é a caixa (não tem o serviço do marcador)." {
		t.Fatalf("try = %+v", res)
	}
	if s.Store.Settings().BoxAddress != "" {
		t.Fatal("a device that is not the box was saved")
	}
	if code := call(t, s, "POST", "/api/ble/try", map[string]string{"address": " "}, nil); code != 400 {
		t.Fatalf("empty address %d", code)
	}
}

func TestSettingsKeepBoxAddress(t *testing.T) {
	s := newServer(t)
	s.Store.SetBoxAddress("AA:BB:CC:DD:EE:FF")
	set := s.Store.Settings()
	set.BoxAddress = ""
	call(t, s, "PUT", "/api/settings", set, nil)
	if got := s.Store.Settings().BoxAddress; got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("box_address = %q", got)
	}
}
