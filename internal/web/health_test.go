package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// healthOBS is camOBS that also answers screenshots by source and GetStats.
type healthOBS struct {
	*camOBS
	hmu     sync.Mutex
	shots   map[string]string
	skipped float64 // renderSkippedFrames
	total   float64 // renderTotalFrames
	asked   []string
}

func (h *healthOBS) answer(typ string, raw json.RawMessage) (bool, int, any) {
	h.hmu.Lock()
	h.asked = append(h.asked, typ)
	switch typ {
	case "GetSourceScreenshot":
		defer h.hmu.Unlock()
		var d struct {
			Source string `json:"sourceName"`
		}
		json.Unmarshal(raw, &d)
		return true, 100, map[string]any{"imageData": h.shots[d.Source]}
	case "GetStats":
		defer h.hmu.Unlock()
		return true, 100, map[string]any{"renderSkippedFrames": h.skipped, "renderTotalFrames": h.total,
			"outputSkippedFrames": 0, "outputTotalFrames": 0}
	}
	h.hmu.Unlock()
	return h.camOBS.answer(typ, raw)
}

func (h *healthOBS) shoot(source string, seed int) {
	h.hmu.Lock()
	h.shots[source] = shotPNG(seed)
	h.hmu.Unlock()
}

func (h *healthOBS) sent() []string {
	h.hmu.Lock()
	defer h.hmu.Unlock()
	return slices.Clone(h.asked)
}

func (h *healthOBS) forget() {
	h.hmu.Lock()
	h.asked = nil
	h.hmu.Unlock()
}

// size sets the camera's source size in scene.
func (h *healthOBS) size(scene string, w, hgt float64) {
	h.camOBS.mu.Lock()
	defer h.camOBS.mu.Unlock()
	tr := h.scenes[scene][0]["sceneItemTransform"].(map[string]any)
	tr["sourceWidth"], tr["sourceHeight"] = w, hgt
}

// shotPNG is a noisy 128x72 PNG as obs-websocket sends it; each seed
// gives other pixels.
func shotPNG(seed int) string {
	img := image.NewRGBA(image.Rect(0, 0, 128, 72))
	for y := range 72 {
		for x := range 128 {
			img.Set(x, y, color.RGBA{uint8(x*7 + seed), uint8(y*13 + seed*3), uint8(x + y), 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

var healthCams = []store.Camera{{Source: "A", Scene: "Câmara 1", Label: "Bancada"}, {Source: "B", Scene: "Câmara 2"}, {Source: "C", Scene: "Câmara 3"}}

// healthServer is a server on healthOBS with three cameras, connected.
func healthServer(t *testing.T) (*Server, *healthOBS) {
	t.Helper()
	h := &healthOBS{camOBS: newCamOBS("A", "B", "C"), shots: map[string]string{}, total: 1000}
	s := newServer(t)
	host, port := fakeOBSData(t, "", h.answer)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true
		set.OBS.CameraSource, set.OBS.Cameras, set.OBS.GraphicsScene = "A", healthCams, "Grafismo"
	})
	s.OBS.OnConnect = nil // the health check it starts would hold setupOBSMu
	if err := s.OBS.Request(obsConfig(s.Store.Settings().OBS), "GetVersion", nil, nil); err != nil {
		t.Fatal(err)
	}
	for i, src := range []string{"A", "B", "C"} {
		h.shoot(src, i*50)
	}
	return s, h
}

func statusCameras(t *testing.T, s *Server) ([]camState, any) {
	t.Helper()
	var st struct {
		OBS struct {
			Cameras []camState `json:"cameras"`
			Skipped any        `json:"skipped"`
		} `json:"obs"`
	}
	call(t, s, "GET", "/api/status", nil, &st)
	return st.OBS.Cameras, st.OBS.Skipped
}

func TestCameraHealthStates(t *testing.T) {
	s, h := healthServer(t)
	h.size("Câmara 2", 0, 0)
	s.cameraRound()
	if got, _ := statusCameras(t, s); !slices.Equal(got, []camState{{1, "ok"}, {2, "no_image"}, {3, "ok"}}) {
		t.Errorf("first round: %v", got)
	}
	h.shoot("A", 7) // camera 1 moves, camera 3 sends the same picture
	s.cameraRound()
	want := []camState{{1, "ok"}, {2, "no_image"}, {3, "frozen"}}
	if got, _ := statusCameras(t, s); !slices.Equal(got, want) {
		t.Errorf("second round: %v, want %v", got, want)
	}
	if slices.Index(h.sent(), "GetSourceScreenshot") < 0 {
		t.Fatal("no screenshot taken")
	}
}

func TestCameraHealthOnePictureIsNoImage(t *testing.T) {
	s, h := healthServer(t)
	img := image.NewRGBA(image.Rect(0, 0, 128, 72))
	var b bytes.Buffer
	png.Encode(&b, img)
	h.hmu.Lock()
	h.shots["B"] = base64.StdEncoding.EncodeToString(b.Bytes())
	h.hmu.Unlock()
	s.cameraRound()
	if got, _ := statusCameras(t, s); len(got) != 3 || got[1].State != "no_image" {
		t.Errorf("a blank phone page: %v", got)
	}
}

func TestCameraHealthSkips(t *testing.T) {
	for _, c := range []struct {
		name string
		hold func(s *Server) func()
	}{
		{"setup", func(s *Server) func() { s.setupOBSMu.Lock(); return s.setupOBSMu.Unlock }},
		{"switch", func(s *Server) func() { s.switchMu.Lock(); return s.switchMu.Unlock }},
		{"squeeze", func(s *Server) func() {
			if err := s.Camera.Follow("desconto"); err != nil {
				t.Fatal(err)
			}
			if !s.Camera.Moving() {
				t.Fatal("no squeeze running")
			}
			return s.Camera.Stop
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, h := healthServer(t)
			done := c.hold(s)
			h.forget()
			s.cameraRound()
			for _, typ := range h.sent() {
				if typ != "SetSceneItemTransform" {
					t.Errorf("sent %s during a %s", typ, c.name)
				}
			}
			done()
			if got, _ := statusCameras(t, s); len(got) != 0 {
				t.Errorf("states from a skipped round: %v", got)
			}
		})
	}
}

func TestCameraHealthOBSOff(t *testing.T) {
	s, _ := healthServer(t)
	s.cameraRound()
	if got, _ := statusCameras(t, s); len(got) != 3 {
		t.Fatalf("connected: %v", got)
	}
	s.OBS = &obsws.Client{} // a connection never made
	if got, _ := statusCameras(t, s); got == nil || len(got) != 0 {
		t.Errorf("disconnected: %v, want []", got)
	}
	s.cameraRound() // must not dial
	if ok, _ := s.OBS.Status(); ok {
		t.Error("a round connected to OBS")
	}
}

func TestCameraHealthSkippedFrames(t *testing.T) {
	s, h := healthServer(t)
	for i := range 7 {
		h.hmu.Lock()
		h.skipped, h.total = float64(i*5), float64(1000+i*300)
		h.hmu.Unlock()
		s.cameraRound()
		if _, sk := statusCameras(t, s); i < 6 && sk != nil {
			t.Fatalf("round %d: %v before a minute of samples", i+1, sk)
		}
	}
	_, sk := statusCameras(t, s)
	if m, _ := sk.(map[string]any); m["frames"] != 30.0 || m["total"] != 1800.0 {
		t.Errorf("skipped %v, want 30 of 1800", sk)
	}
	for range 7 {
		s.cameraRound() // no more skipped frames
	}
	if _, sk := statusCameras(t, s); sk != nil {
		t.Errorf("no rise: %v", sk)
	}
	h.hmu.Lock()
	h.total = 10 // OBS started again
	h.hmu.Unlock()
	s.cameraRound()
	s.obsMu.Lock()
	n := len(s.frames)
	s.obsMu.Unlock()
	if n != 1 {
		t.Errorf("%d samples after OBS restarted", n)
	}
}

func TestCameraHealthLogsChanges(t *testing.T) {
	s, h := healthServer(t)
	var mu sync.Mutex
	var logs []string
	s.Log = func(m string) { mu.Lock(); logs = append(logs, m); mu.Unlock() }
	count := func(want string) int {
		mu.Lock()
		defer mu.Unlock()
		return len(slices.DeleteFunc(slices.Clone(logs), func(l string) bool { return l != want }))
	}
	moving := func(seed int) {
		for i, src := range []string{"A", "B", "C"} {
			h.shoot(src, seed+i*50)
		}
	}
	s.cameraRound()
	moving(1)
	s.cameraRound()
	mu.Lock()
	if len(logs) != 0 {
		t.Errorf("logged the first states: %v", logs)
	}
	mu.Unlock()
	h.size("Câmara 1", 0, 0)
	moving(2)
	s.cameraRound()
	moving(3)
	s.cameraRound()
	if n := count(i18n.T("log.camara.sem_imagem", "camara", "Bancada")); n != 1 {
		t.Errorf("%d lines for camera 1 losing its picture: %v", n, logs)
	}
	h.size("Câmara 1", 1280, 720)
	s.cameraRound()
	if n := count(i18n.T("log.camara.voltou", "camara", "Bancada")); n != 1 {
		t.Errorf("%d lines for camera 1 coming back: %v", n, logs)
	}
	s.cameraRound() // B keeps the same picture: named by its source
	if n := count(i18n.T("log.camara.parada", "camara", "B")); n != 1 {
		t.Errorf("%d lines for camera 2 freezing: %v", n, logs)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, l := range logs {
		if strings.Contains(l, "{") {
			t.Errorf("unfilled log line %q", l)
		}
	}
}
