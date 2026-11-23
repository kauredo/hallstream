package ble

import (
	"errors"
	"fmt"
	"strings"

	"tinygo.org/x/bluetooth"
)

var (
	serviceUUID = bluetooth.New16BitUUID(0xa002)
	notifyUUID  = bluetooth.New16BitUUID(0xc305)
)

// Adapter returns the computer's Bluetooth adapter.
func Adapter() Radio { return adapter{bluetooth.DefaultAdapter} }

type adapter struct{ a *bluetooth.Adapter }

func (t adapter) Enable() error   { return t.a.Enable() }
func (t adapter) StopScan() error { return t.a.StopScan() }

func (t adapter) Scan(found func(Advert)) error {
	return t.a.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		var services []string
		for _, u := range r.ServiceUUIDs() {
			services = append(services, uuidHex(u))
		}
		found(Advert{Address: r.Address.String(), Name: r.LocalName(), RSSI: int(r.RSSI), Services: services})
	})
}

func uuidHex(u bluetooth.UUID) string {
	if u.Is16Bit() {
		return fmt.Sprintf("%04x", u.Get16Bit())
	}
	return u.String()
}

// Connect takes the address as Address.String() gave it: a MAC on Windows,
// a UUID on macOS.
func (t adapter) Connect(address string) (Link, error) {
	var addr bluetooth.Address
	addr.Set(address)
	if !strings.EqualFold(addr.String(), address) {
		return nil, errors.New("endereço inválido: " + address)
	}
	device, err := t.a.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, err
	}
	char, err := channel(device)
	if err != nil {
		device.Disconnect()
		return nil, err
	}
	return &link{device, char}, nil
}

// channel finds characteristic 0xc305 in service 0xa002. It lists everything,
// because asking for a missing service is an error on Windows and we need
// to tell "not the box" from "could not talk to it".
func channel(device bluetooth.Device) (bluetooth.DeviceCharacteristic, error) {
	services, err := device.DiscoverServices(nil)
	if err != nil {
		return bluetooth.DeviceCharacteristic{}, err
	}
	for _, s := range services {
		if s.UUID().String() != serviceUUID.String() {
			continue
		}
		chars, err := s.DiscoverCharacteristics(nil)
		if err != nil {
			return bluetooth.DeviceCharacteristic{}, err
		}
		for _, c := range chars {
			if c.UUID().String() == notifyUUID.String() {
				return c, nil
			}
		}
	}
	return bluetooth.DeviceCharacteristic{}, ErrNotBox
}

type link struct {
	device bluetooth.Device
	char   bluetooth.DeviceCharacteristic
}

func (l *link) Listen(frame func([]byte)) error {
	err := l.char.EnableNotifications(func(buf []byte) {
		frame(append([]byte(nil), buf...)) // the library reuses buf
	})
	if err != nil {
		return err
	}
	// The app also reads once after subscribing.
	buf := make([]byte, 20)
	if n, err := l.char.Read(buf); err == nil {
		frame(buf[:n])
	}
	return nil
}

func (l *link) Connected() bool {
	ok, err := l.device.Connected()
	return err == nil && ok
}

func (l *link) Disconnect() { l.device.Disconnect() }
