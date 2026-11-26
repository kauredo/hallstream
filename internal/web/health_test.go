package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// healthOBS is camOBS that also answers screenshots by source and GetStats.
type healthOBS struct {
	*camOBS
	hmu     sync.Mutex
	shots   map[string]string
	refuse  map[string]int // a screenshot of that source answers with this status
	skipped float64        // renderSkippedFrames
	total   float64        // renderTotalFrames
	asked   []string
	hook    func(typ, scene string) // runs before each answer, outside the locks
}

func (h *healthOBS) answer(typ string, raw json.RawMessage) (bool, int, any) {
	var d struct {
		Source string `json:"sourceName"`
		Scene  string `json:"sceneName"`
	}
	json.Unmarshal(raw, &d)
	h.hmu.Lock()
	hook := h.hook
	h.hmu.Unlock()
	if hook != nil {
		hook(typ, d.Scene)
	}
	h.hmu.Lock()
	h.asked = append(h.asked, typ)
	switch typ {
	case "GetSourceScreenshot":
		defer h.hmu.Unlock()
		if code := h.refuse[d.Source]; code != 0 {
			return false, code, nil
		}
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

// moving gives every camera a new picture.
func (h *healthOBS) moving(seed int) {
	for i, src := range []string{"A", "B", "C"} {
		h.shoot(src, seed+i*50)
	}
}

func (h *healthOBS) set(f func()) {
	h.hmu.Lock()
	f()
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
	return encodePNG(img)
}

func encodePNG(img image.Image) string {
	var b bytes.Buffer
	png.Encode(&b, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

var healthCams = []store.Camera{{Source: "A", Scene: "Câmara 1", Label: "Bancada"}, {Source: "B", Scene: "Câmara 2"}, {Source: "C", Scene: "Câmara 3"}}

// healthServer is a server on healthOBS with three cameras, connected.
func healthServer(t *testing.T) (*Server, *healthOBS) {
	t.Helper()
	h := &healthOBS{camOBS: newCamOBS("A", "B", "C"), shots: map[string]string{}, refuse: map[string]int{}, total: 1000}
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
	h.moving(0)
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

func wantStates(t *testing.T, s *Server, when string, want ...string) {
	t.Helper()
	got, _ := statusCameras(t, s)
	var states []string
	for _, c := range got {
		states = append(states, c.State)
	}
	if !slices.Equal(states, want) {
		t.Errorf("%s: %v, want %v", when, got, want)
	}
}

func TestCameraHealthStates(t *testing.T) {
	s, h := healthServer(t)
	h.size("Câmara 2", 0, 0)
	s.cameraRound()
	wantStates(t, s, "first round", "ok", "no_image", "ok")
	// Camera 1 moves, camera 3 sends the same picture: one identical pair
	// is not yet frozen, the second is.
	h.shoot("A", 7)
	s.cameraRound()
	wantStates(t, s, "second round", "ok", "no_image", "ok")
	h.shoot("A", 8)
	s.cameraRound()
	wantStates(t, s, "third round", "ok", "no_image", "frozen")
	h.shoot("C", 9)
	s.cameraRound()
	wantStates(t, s, "camera 3 moves again", "ok", "no_image", "ok")
	if slices.Index(h.sent(), "GetSourceScreenshot") < 0 {
		t.Fatal("no screenshot taken")
	}
}

// A live camera's noise changes every screenshot: never frozen.
func TestCameraHealthLiveNeverFrozen(t *testing.T) {
	s, h := healthServer(t)
	for i := range 10 {
		h.moving(i + 1)
		s.cameraRound()
		wantStates(t, s, "a live round", "ok", "ok", "ok")
	}
}

func TestCameraHealthOnePictureIsNoImage(t *testing.T) {
	s, h := healthServer(t)
	blank := encodePNG(image.NewRGBA(image.Rect(0, 0, 128, 72)))
	h.set(func() { h.shots["B"] = blank })
	s.cameraRound()
	wantStates(t, s, "one blank screenshot", "ok", "", "ok")
	h.moving(1)
	h.set(func() { h.shots["B"] = blank })
	s.cameraRound()
	wantStates(t, s, "two blank screenshots", "ok", "no_image", "ok")
	h.moving(2)
	s.cameraRound()
	wantStates(t, s, "a picture again", "ok", "ok", "ok")
}

// A dark hall is not one colour: a few levels of noise around black.
func TestCameraHealthDarkHall(t *testing.T) {
	s, h := healthServer(t)
	for i := range 4 {
		img := image.NewRGBA(image.Rect(0, 0, 128, 72))
		for y := range 72 {
			for x := range 128 {
				v := uint8((x*7 + y*3 + i) % 20)
				img.Set(x, y, color.RGBA{v, v, v, 255})
			}
		}
		h.moving(i + 1)
		h.set(func() { h.shots["A"] = encodePNG(img) })
		s.cameraRound()
	}
	wantStates(t, s, "a dark hall", "ok", "ok", "ok")
}

func TestCameraHealthLookErrors(t *testing.T) {
	s, h := healthServer(t)
	s.cameraRound()
	wantStates(t, s, "first round", "ok", "ok", "ok")
	// OBS can't take B's screenshot (702), and C's screenshot fails.
	h.set(func() { h.refuse["B"], h.refuse["C"] = 702, 500 })
	for i := range 2 {
		h.moving(i + 1)
		s.cameraRound()
		wantStates(t, s, "a failing look keeps the state", "ok", "no_image", "ok")
	}
	h.moving(3)
	s.cameraRound()
	wantStates(t, s, "three failed looks forget it", "ok", "no_image", "")
	h.set(func() { h.refuse = map[string]int{"B": obsws.NotFound} })
	h.moving(4)
	s.cameraRound()
	wantStates(t, s, "C back, B not found", "ok", "no_image", "ok")
}

// With no cameras saved, camera 1 is camera_source in the program scene.
// On a scene without it (the interval, the scoreboard), nothing changes.
func TestCameraHealthProgramWithoutTheCamera(t *testing.T) {
	s, h := healthServer(t)
	var logs []string
	var mu sync.Mutex
	s.Log = func(m string) { mu.Lock(); logs = append(logs, m); mu.Unlock() }
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Cameras = nil })
	s.cameraRound()
	wantStates(t, s, "camera on program", "ok")
	for i := range 4 {
		h.camOBS.mu.Lock()
		h.camOBS.program = "Câmara 2" // shows B, not A
		h.camOBS.mu.Unlock()
		h.moving(i + 1)
		s.cameraRound()
		wantStates(t, s, "another scene on program", "ok")
	}
	h.camOBS.mu.Lock()
	h.camOBS.program = "Câmara 1"
	h.camOBS.mu.Unlock()
	h.moving(9)
	s.cameraRound()
	wantStates(t, s, "camera back on program", "ok")
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != 0 {
		t.Errorf("scene changes logged: %v", logs)
	}
}

func TestCameraHealthSkips(t *testing.T) {
	for _, c := range []struct {
		name string
		hold func(s *Server) func()
	}{
		{"setup", func(s *Server) func() { s.lockSetup(); return s.unlockSetup }},
		{"round", func(s *Server) func() { s.roundMu.Lock(); return s.roundMu.Unlock }},
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

// A switch that starts while a look is under way stops the round before
// the screenshot.
func TestCameraHealthBusyMidLook(t *testing.T) {
	s, h := healthServer(t)
	var once sync.Once
	h.set(func() {
		h.hook = func(typ, scene string) {
			if typ == "GetSceneItemList" && scene == "Câmara 1" {
				once.Do(s.switchMu.Lock)
			}
		}
	})
	s.cameraRound()
	s.switchMu.Unlock()
	if sent := h.sent(); slices.Contains(sent, "GetSourceScreenshot") || slices.Contains(sent, "GetStats") {
		t.Errorf("the round went on during a switch: %v", sent)
	}
	if got, _ := statusCameras(t, s); len(got) != 0 {
		t.Errorf("states from a stopped round: %v", got)
	}
}

// Configurar OBS during a round neither waits for it to end nor fails: the
// round stops at its next request.
func TestCameraHealthSetupDuringRound(t *testing.T) {
	s, h := healthServer(t)
	shot, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h.set(func() {
		h.hook = func(typ, _ string) {
			if typ == "GetSourceScreenshot" {
				once.Do(func() { close(shot); <-release })
			}
		}
	})
	round := make(chan struct{})
	go func() { s.cameraRound(); close(round) }()
	<-shot
	code := make(chan int, 1)
	go func() { code <- call(t, s, "POST", "/api/obs/setup", map[string]any{}, nil) }()
	for !s.setupOn.Load() {
		select {
		case c := <-code:
			close(release)
			t.Fatalf("Configurar OBS ended at once during a round: %d", c)
		case <-time.After(time.Millisecond):
		}
	}
	close(release)
	<-round
	if c := <-code; c == http.StatusConflict {
		t.Errorf("Configurar OBS during a round: %d", c)
	}
	sent := h.sent()
	if n := len(slices.DeleteFunc(slices.Clone(sent), func(typ string) bool { return typ != "GetSourceScreenshot" })); n != 1 {
		t.Errorf("%d screenshots, want the one before the setup: %v", n, sent)
	}
}

func TestCameraHealthOBSOff(t *testing.T) {
	s, _ := healthServer(t)
	s.cameraRound()
	if got, _ := statusCameras(t, s); len(got) != 3 {
		t.Fatalf("connected: %v", got)
	}
	conn := s.OBS
	s.OBS = &obsws.Client{} // a connection never made
	if got, _ := statusCameras(t, s); got == nil || len(got) != 0 {
		t.Errorf("disconnected: %v, want []", got)
	}
	s.cameraRound() // must not dial
	if ok, _ := s.OBS.Status(); ok {
		t.Error("a round connected to OBS")
	}
	s.OBS = conn // connected again: nothing from before the drop
	if got, _ := statusCameras(t, s); len(got) != 0 {
		t.Errorf("after a reconnect: %v", got)
	}
	s.obsMu.Lock()
	n, v := len(s.frames), len(s.camVitals)
	s.obsMu.Unlock()
	if n != 0 || v != 0 {
		t.Errorf("%d frame samples and %d cameras kept across a drop", n, v)
	}

	s.cameraRound()
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = false })
	s.cameraRound()
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled = true })
	if got, _ := statusCameras(t, s); len(got) != 0 {
		t.Errorf("after OBS control off and on: %v", got)
	}
}

// A camera taken off the list, or given another source, starts over.
func TestCameraHealthDropsOldCameras(t *testing.T) {
	s, h := healthServer(t)
	s.cameraRound()
	s.cameraRound() // A, B and C each one identical pair
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Cameras = []store.Camera{{Source: "A", Scene: "Câmara 1"}, {Source: "B", Scene: "Câmara 3"}}
	})
	h.camOBS.mu.Lock()
	h.scenes["Câmara 3"][0]["sourceName"] = "B"
	h.camOBS.mu.Unlock()
	s.cameraRound()
	wantStates(t, s, "camera 2 gone, camera 3 renamed", "frozen", "ok")
	s.obsMu.Lock()
	defer s.obsMu.Unlock()
	if _, ok := s.camVitals["Câmara 2\x00B"]; ok || len(s.camVitals) != 2 {
		t.Errorf("kept the old cameras: %v", s.camVitals)
	}
}

// ageFrames moves every frame sample d into the past.
func ageFrames(s *Server, d time.Duration) {
	s.obsMu.Lock()
	for i := range s.frames {
		s.frames[i].at = s.frames[i].at.Add(-d)
	}
	s.obsMu.Unlock()
}

func TestCameraHealthSkippedFrames(t *testing.T) {
	s, h := healthServer(t)
	counts := func(skipped, total float64) { h.set(func() { h.skipped, h.total = skipped, total }) }
	minute := func(skipped, total func(i int) float64) {
		for i := range 7 {
			counts(skipped(i), total(i))
			s.cameraRound()
			if _, sk := statusCameras(t, s); i < 5 && sk != nil {
				t.Fatalf("round %d: %v before a minute of samples", i+1, sk)
			}
			ageFrames(s, 10*time.Second)
		}
	}
	minute(func(i int) float64 { return float64(i * 5) }, func(i int) float64 { return float64(1000 + i*300) })
	_, sk := statusCameras(t, s)
	if m, _ := sk.(map[string]any); m["frames"] != 30.0 || m["total"] != 1800.0 {
		t.Errorf("skipped %v, want 30 of 1800 (render frames only)", sk)
	}
	for range 7 {
		s.cameraRound() // no more skipped frames
		ageFrames(s, 10*time.Second)
	}
	if _, sk := statusCameras(t, s); sk != nil {
		t.Errorf("no rise: %v", sk)
	}

	// 1% of the minute's frames, the rounds' own cost, is no warning; more is.
	s.obsMu.Lock()
	s.frames = nil
	s.obsMu.Unlock()
	minute(func(i int) float64 { return float64(i * 3) }, func(i int) float64 { return float64(2000 + i*300) })
	if _, sk := statusCameras(t, s); sk != nil {
		t.Errorf("18 of 1800: %v", sk)
	}
	counts(3+19, 2300+1800) // the oldest sample left is now 3 of 2300
	s.cameraRound()
	if _, sk := statusCameras(t, s); sk == nil {
		t.Error("19 of 1800: no warning")
	}

	// A long gap with no samples starts over.
	ageFrames(s, 2*time.Minute)
	counts(40, 9000)
	s.cameraRound()
	if _, sk := statusCameras(t, s); sk != nil {
		t.Errorf("after a gap: %v", sk)
	}

	for _, c := range []struct {
		name           string
		skipped, total float64
	}{{"total falls", 50, 10}, {"skipped falls", 0, 20000}} {
		counts(c.skipped, c.total)
		s.cameraRound()
		s.obsMu.Lock()
		n := len(s.frames)
		s.obsMu.Unlock()
		if n != 1 {
			t.Errorf("%s (OBS started again): %d samples", c.name, n)
		}
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
	s.cameraRound()
	h.moving(1)
	s.cameraRound()
	mu.Lock()
	if len(logs) != 0 {
		t.Errorf("logged the first states: %v", logs)
	}
	mu.Unlock()
	h.size("Câmara 1", 0, 0)
	h.moving(2)
	s.cameraRound()
	h.moving(3)
	s.cameraRound()
	if n := count(i18n.T("log.camara.sem_imagem", "camara", "Bancada")); n != 1 {
		t.Errorf("%d lines for camera 1 losing its picture: %v", n, logs)
	}
	h.size("Câmara 1", 1280, 720)
	h.moving(4)
	s.cameraRound()
	if n := count(i18n.T("log.camara.voltou", "camara", "Bancada")); n != 1 {
		t.Errorf("%d lines for camera 1 coming back: %v", n, logs)
	}
	h.shoot("A", 5)
	s.cameraRound() // B's same picture, first pair
	h.shoot("A", 6)
	s.cameraRound() // second pair: frozen, named by its source
	if n := count(i18n.T("log.camara.parada", "camara", "B")); n != 1 {
		t.Errorf("%d lines for camera 2 freezing: %v", n, logs)
	}
	// Camera 3 blank every other round: never long enough to log.
	h.moving(9)
	s.cameraRound()
	mu.Lock()
	before := len(logs)
	mu.Unlock()
	blank := encodePNG(image.NewRGBA(image.Rect(0, 0, 128, 72)))
	for i := range 6 {
		h.moving(10 + i)
		if i%2 == 0 {
			h.set(func() { h.shots["C"] = blank })
		}
		s.cameraRound()
	}
	mu.Lock()
	defer mu.Unlock()
	for _, l := range logs {
		if strings.Contains(l, "{") {
			t.Errorf("unfilled log line %q", l)
		}
	}
	if len(logs) != before {
		t.Errorf("a flickering camera logged %v", logs[before:])
	}
}
