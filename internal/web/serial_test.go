package web

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/store"
)

func TestCleanSerial(t *testing.T) {
	cur := store.Serial{Port: "COM3", Protocol: "bodet"}
	for _, c := range []struct {
		name string
		req  store.Serial
		pc   bool
		want store.Serial
	}{
		{"pc", store.Serial{Port: "COM4", Protocol: "daktronics"}, true, store.Serial{Port: "COM4", Protocol: "daktronics"}},
		{"unknown protocol", store.Serial{Port: "COM4", Protocol: "fairplay"}, true, store.Serial{Port: "COM4", Protocol: "daktronics"}},
		{"long port", store.Serial{Port: strings.Repeat("x", maxPort+1), Protocol: "bodet"}, true, cur},
		{"control characters", store.Serial{Port: "COM4\nlog line", Protocol: "bodet"}, true, cur},
		{"phone", store.Serial{Port: "COM9", Protocol: "daktronics"}, false, cur},
	} {
		got := c.req
		cleanSerial(&got, cur, c.pc)
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestSerialSettingsRestartTheCable(t *testing.T) {
	s := newServer(t)
	restarts := 0
	s.RestartInput = func() { restarts++ }
	s.UseInput = func(string) {}
	set := s.Store.Settings()
	if set.Serial.Protocol != "daktronics" {
		t.Fatalf("default protocol %q", set.Serial.Protocol)
	}
	set.Input = "serial"
	call(t, s, "PUT", "/api/settings", set, nil) // a new input uses UseInput, not a restart
	set.Serial.Port = "COM5"
	call(t, s, "PUT", "/api/settings", set, nil)
	call(t, s, "PUT", "/api/settings", set, nil) // nothing changed
	if restarts != 1 {
		t.Errorf("%d restarts, want 1", restarts)
	}
	if got := s.Store.Settings().Serial; got.Port != "COM5" {
		t.Errorf("saved %+v", got)
	}
}

func TestSerialEndpoint(t *testing.T) {
	s := newServer(t)
	var got struct {
		source.SerialStatus
		Ports []string `json:"ports"`
	}
	if code := call(t, s, "GET", "/api/serial", nil, &got); code != 200 || got.Ports == nil || got.Protocol != "daktronics" {
		t.Fatalf("no cable: %d %+v", code, got)
	}
	s.SerialPorts = func() ([]string, error) { return []string{"COM3", "COM4"}, nil }
	s.Serial = source.NewSerial(&source.Serial{
		Open: func(string, int) (io.ReadCloser, error) {
			return nil, &source.OpenError{Key: i18n.Key("api.serial.nao_existe"), Err: errors.New("no such port")}
		},
		Settings: func() (string, string) { return "COM7", "bodet" },
		Retry:    time.Hour,
	})
	stop := make(chan struct{})
	defer close(stop)
	go s.Serial.Run(stop, func(map[string]string) {}, func(bool) {})
	waitUntil(t, "the failed open", func() bool { return s.Serial.Status().Error != "" })
	call(t, s, "GET", "/api/serial", nil, &got)
	if len(got.Ports) != 2 || got.Port != "COM7" || got.Protocol != "bodet" || got.Open || got.Error != i18n.T("api.serial.nao_existe") {
		t.Errorf("got %+v", got)
	}
}

func TestCaptureFollowsTheCable(t *testing.T) {
	s := newServer(t)
	pr, pw := io.Pipe()
	s.Serial = source.NewSerial(&source.Serial{
		Open:     func(string, int) (io.ReadCloser, error) { return pr, nil },
		Settings: func() (string, string) { return "COM3", "daktronics" },
		Sport:    func() string { return "basketball" },
		Capture:  &ble.Recorder{Dir: s.captureDir(), App: "test", Link: "serial", Protocol: func() string { return "daktronics" }},
	})
	s.State.SetInput("serial")
	var errMsg struct{ Error string }
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, &errMsg); code != 409 || errMsg.Error != i18n.T("api.captura.sem_cabo") {
		t.Fatalf("no cable: %d %q", code, errMsg.Error)
	}
	stop := make(chan struct{})
	defer close(stop)
	go s.Serial.Run(stop, func(map[string]string) {}, func(bool) {})
	body := "00000000\x010042100107\x02  52\x04"
	var sum byte
	for i := 0; i < len(body); i++ {
		sum += body[i]
	}
	frame := []byte(fmt.Sprintf("\x16%s%02X\x17", body, sum))
	go pw.Write(frame)
	waitUntil(t, "the cable link", func() bool { ok, _ := s.Serial.Linked(); return ok })
	t.Cleanup(func() { s.Serial.Capture.Stop("off") })
	var st ble.CaptureStatus
	if code := post(s, "POST", "/api/ble/capture", map[string]bool{"on": true}, &st); code != 200 || !st.On {
		t.Fatalf("switch on: %d %+v", code, st)
	}
	go pw.Write(frame)
	waitUntil(t, "a frame written", func() bool { return s.Serial.Capture.Status().Frames == 1 })
	post(s, "POST", "/api/ble/capture", map[string]bool{"on": false}, &st)
	b, _ := os.ReadFile(filepath.Join(s.captureDir(), st.File))
	if got := string(b); !strings.Contains(got, `link="serial" protocol="daktronics"`) || !strings.Contains(got, `box="COM3"`) {
		t.Errorf("header: %q", strings.SplitN(got, "\n", 2)[0])
	}
}
