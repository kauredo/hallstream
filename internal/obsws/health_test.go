package obsws

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// shotOBS is a fake that also answers GetSourceScreenshot and GetStats.
// codes makes a request type answer with that status (or hangUp, silent).
type shotOBS struct {
	*fake
	shot  string
	stats map[string]any
	codes map[string]int
}

func (s *shotOBS) answer(typ string, d map[string]any) (bool, int, any) {
	s.fake.mu.Lock()
	code := s.codes[typ]
	s.fake.mu.Unlock()
	if code != 0 {
		s.fake.mu.Lock()
		s.fake.asked = append(s.fake.asked, typ)
		s.fake.mu.Unlock()
		return false, code, nil
	}
	switch typ {
	case "GetSourceScreenshot":
		s.fake.mu.Lock()
		s.fake.asked = append(s.fake.asked, typ)
		s.fake.mu.Unlock()
		if d["imageFormat"] != "png" || d["imageWidth"] != 128.0 || d["imageHeight"] != 72.0 {
			return false, 400, nil
		}
		return true, 100, map[string]any{"imageData": s.shot}
	case "GetStats":
		return true, 100, s.stats
	}
	return s.fake.answer(typ, d)
}

func (s *shotOBS) answerWith(codes map[string]int) {
	s.fake.mu.Lock()
	s.codes = codes
	s.fake.mu.Unlock()
}

// pngOf is a 128x72 PNG in base64, each pixel from at.
func pngOf(t *testing.T, at func(x, y int) color.Color) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 128, 72))
	for y := range 72 {
		for x := range 128 {
			img.Set(x, y, at(x, y))
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func noisy(x, y int) color.Color {
	return color.RGBA{uint8(x * 7 % 256), uint8(y * 13 % 256), uint8((x + y) % 256), 255}
}

func black(int, int) color.Color { return color.RGBA{0, 0, 0, 255} }

func camItem(name string, w, h float64) map[string]any {
	return map[string]any{"sceneItemId": 1, "sourceName": name, "inputKind": "dshow_input", "sceneItemEnabled": true,
		"sceneItemTransform": map[string]any{"sourceWidth": w, "sourceHeight": h}}
}

func never() bool { return false }

// see runs looks through a fresh Vitals and returns each state.
func see(looks ...Look) []string {
	var v Vitals
	var got []string
	for _, l := range looks {
		v = v.See(l)
		got = append(got, v.State)
	}
	return got
}

func TestVitals(t *testing.T) {
	pic := func(shot string) Look { return Look{Found: true, Width: 1280, Height: 720, Shot: shot} }
	flat := pic("f")
	flat.Flat = true
	refused := Look{Found: true, Width: 1280, Height: 720, Refused: true}
	for _, c := range []struct {
		name  string
		looks []Look
		want  []string
	}{
		{"live camera", []Look{pic("a"), pic("b"), pic("c"), pic("d")}, []string{"ok", "ok", "ok", "ok"}},
		{"three identical shots", []Look{pic("a"), pic("a"), pic("a"), pic("a")}, []string{"ok", "ok", "frozen", "frozen"}},
		{"one identical pair", []Look{pic("a"), pic("a"), pic("b"), pic("b"), pic("c")}, []string{"ok", "ok", "ok", "ok", "ok"}},
		{"back after one good round", []Look{pic("a"), pic("a"), pic("a"), pic("b")}, []string{"ok", "ok", "frozen", "ok"}},
		{"missing", []Look{pic("a"), {}}, []string{"ok", "no_image"}},
		{"0x0", []Look{pic("a"), {Found: true}}, []string{"ok", "no_image"}},
		{"refused", []Look{pic("a"), refused}, []string{"ok", "no_image"}},
		{"flat twice", []Look{pic("a"), flat, flat, pic("b")}, []string{"ok", "ok", "no_image", "ok"}},
		{"flat once", []Look{pic("a"), flat, pic("b"), flat, pic("c")}, []string{"ok", "ok", "ok", "ok", "ok"}},
		{"flat first", []Look{flat, flat}, []string{"", "no_image"}},
		{"flat breaks a freeze count", []Look{pic("a"), pic("a"), flat, pic("a"), pic("a")}, []string{"ok", "ok", "ok", "ok", "ok"}},
	} {
		if got := see(c.looks...); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVitalsFailed(t *testing.T) {
	v := Vitals{}.See(Look{})
	for i := range failedLooks - 1 {
		if v = v.Failed(); v.State != "no_image" {
			t.Fatalf("failure %d forgot the state: %q", i+1, v.State)
		}
	}
	if v = v.Failed(); v.State != "" {
		t.Errorf("%d failures in a row kept %q", failedLooks, v.State)
	}
	// A look in between starts the count again.
	v = Vitals{}.See(Look{}).Failed().Failed().See(Look{}).Failed().Failed()
	if v.State != "no_image" {
		t.Errorf("failures not in a row forgot the state")
	}
}

func lookRig(t *testing.T) (*shotOBS, Config) {
	f := &shotOBS{fake: &fake{program: "Jogo", byScene: map[string][]map[string]any{
		"Câmara 1":  {camItem("Grafismo", 1920, 1080), camItem("Cam", 1280, 720)},
		"Câmara 2":  {camItem("Telemóvel", 0, 0)},
		"Jogo":      {camItem("Cam", 1280, 720)},
		"Intervalo": {camItem("Grafismo", 1920, 1080)},
	}}, codes: map[string]int{}}
	return f, fakeOBS(t, "", f)
}

func TestLookAt(t *testing.T) {
	f, cfg := lookRig(t)
	var c Client

	f.shot = pngOf(t, noisy)
	l, err := c.LookAt(cfg, "Câmara 1", "Cam", never)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Found || l.Width != 1280 || l.Height != 720 || l.Shot != f.shot || l.Flat {
		t.Errorf("noisy: %+v", l)
	}

	f.shot = pngOf(t, black)
	if l, err = c.LookAt(cfg, "Câmara 1", "Cam", never); err != nil || !l.Flat {
		t.Errorf("black: %+v %v", l, err)
	}

	f.fake.asked = nil
	if l, err = c.LookAt(cfg, "Câmara 2", "Telemóvel", never); err != nil || !l.Found || l.Width != 0 || l.Shot != "" {
		t.Errorf("0x0: %+v %v", l, err)
	}
	if l, err = c.LookAt(cfg, "Câmara 2", "Outra", never); err != nil || l.Found {
		t.Errorf("missing: %+v %v", l, err)
	}
	if slices.Contains(f.fake.asked, "GetSourceScreenshot") {
		t.Errorf("a camera with no picture got a screenshot: %v", f.fake.asked)
	}

	f.shot = pngOf(t, noisy)
	if l, err = c.LookAt(cfg, "", "Cam", never); err != nil || !l.Found {
		t.Errorf("program scene: %+v %v", l, err)
	}

	f.shot = "not a png"
	if _, err = c.LookAt(cfg, "Câmara 1", "Cam", never); err == nil {
		t.Error("a screenshot that does not decode must be an error")
	}
}

// With no scene named, a program scene without the camera (the scoreboard,
// the interval) is nothing to judge.
func TestLookAtProgramWithoutTheCamera(t *testing.T) {
	f, cfg := lookRig(t)
	var c Client
	f.fake.mu.Lock()
	f.fake.program = "Intervalo"
	f.fake.mu.Unlock()
	if _, err := c.LookAt(cfg, "", "Cam", never); !errors.Is(err, ErrNotShown) {
		t.Errorf("err %v, want ErrNotShown", err)
	}
	// A named scene without the camera is a camera with no picture.
	if l, err := c.LookAt(cfg, "Intervalo", "Cam", never); err != nil || l.Found {
		t.Errorf("named scene: %+v %v", l, err)
	}
}

func TestLookAtErrors(t *testing.T) {
	f, cfg := lookRig(t)
	f.shot = pngOf(t, noisy)
	var c Client
	for _, code := range []int{NotFound, failedRequest} {
		f.answerWith(map[string]int{"GetSourceScreenshot": code})
		l, err := c.LookAt(cfg, "Câmara 1", "Cam", never)
		if err != nil || !l.Refused {
			t.Errorf("screenshot answered %d: %+v %v", code, l, err)
		}
		if v := (Vitals{}).See(l); v.State != "no_image" {
			t.Errorf("screenshot answered %d: state %q", code, v.State)
		}
	}
	f.answerWith(map[string]int{"GetSceneItemList": NotFound})
	if l, err := c.LookAt(cfg, "Câmara 9", "Cam", never); err != nil || l.Found {
		t.Errorf("scene gone: %+v %v", l, err)
	}
	f.answerWith(map[string]int{"GetSourceScreenshot": 500})
	if _, err := c.LookAt(cfg, "Câmara 1", "Cam", never); err == nil {
		t.Error("another refusal must be an error")
	}
	f.answerWith(map[string]int{"GetSourceScreenshot": hangUp})
	if _, err := c.LookAt(cfg, "Câmara 1", "Cam", never); err == nil {
		t.Error("a lost connection must be an error")
	}
}

// OBS that stops answering: the request times out (5 s, then once more).
func TestLookAtTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits 10 s")
	}
	t.Parallel()
	f, cfg := lookRig(t)
	f.shot = pngOf(t, noisy)
	f.answerWith(map[string]int{"GetSourceScreenshot": silent})
	var c Client
	_, err := c.LookAt(cfg, "Câmara 1", "Cam", never)
	if err == nil || errors.Is(err, ErrBusy) || errors.Is(err, ErrNotShown) {
		t.Errorf("err %v, want a timeout", err)
	}
	var re *RequestError
	if errors.As(err, &re) {
		t.Errorf("a timeout is not a refusal: %v", err)
	}
}

// A squeeze or switch that starts during a look stops it before the next
// request.
func TestLookAtBusy(t *testing.T) {
	f, cfg := lookRig(t)
	f.shot = pngOf(t, noisy)
	var c Client
	for _, c2 := range []struct {
		scene string
		after int // busy says yes from this ask on
		sent  []string
	}{
		{"Câmara 1", 0, nil},
		{"Câmara 1", 1, []string{"GetSceneItemList"}},
		{"", 1, []string{"GetCurrentProgramScene"}},
		{"", 2, []string{"GetCurrentProgramScene", "GetSceneItemList"}},
	} {
		f.fake.mu.Lock()
		f.fake.asked = nil
		f.fake.mu.Unlock()
		n := 0
		busy := func() bool { n++; return n > c2.after }
		if _, err := c.LookAt(cfg, c2.scene, "Cam", busy); !errors.Is(err, ErrBusy) {
			t.Errorf("scene %q, busy after %d: err %v", c2.scene, c2.after, err)
		}
		f.fake.mu.Lock()
		got := slices.Clone(f.fake.asked)
		f.fake.mu.Unlock()
		if !slices.Equal(got, c2.sent) {
			t.Errorf("scene %q, busy after %d: sent %v, want %v", c2.scene, c2.after, got, c2.sent)
		}
	}
}

// encode is img as obs-websocket sends it, base64 PNG.
func encode(t *testing.T, img image.Image) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestFlatPNG(t *testing.T) {
	spread := func(n int) func(x, y int) color.Color {
		return func(x, _ int) color.Color { return color.RGBA{uint8(40 + x%(n+1)), 40, 40, 255} }
	}
	for _, c := range []struct {
		name string
		data string
		want bool
	}{
		{"black", pngOf(t, black), true},
		{"range 12", pngOf(t, spread(12)), true},
		{"range 13", pngOf(t, spread(13)), false},
		// The first pixel at one end of a 12 range: still one colour.
		{"first pixel at the edge", pngOf(t, func(x, y int) color.Color {
			if x == 0 && y == 0 {
				return color.RGBA{28, 40, 40, 255}
			}
			return color.RGBA{uint8(34 + x%7), 40, 40, 255}
		}), true},
		{"one bright pixel", pngOf(t, func(x, y int) color.Color {
			if x == 64 && y == 36 {
				return color.RGBA{255, 255, 255, 255}
			}
			return color.RGBA{0, 0, 0, 255}
		}), false},
		{"green spread", pngOf(t, func(x, _ int) color.Color { return color.RGBA{40, uint8(40 + x%14), 40, 255} }), false},
		{"clear", pngOf(t, func(int, int) color.Color { return color.NRGBA{200, 10, 10, 0} }), true},
		{"alpha spread", pngOf(t, func(x, _ int) color.Color { return color.NRGBA{0, 0, 0, uint8(200 + x%14)} }), false},
		{"noisy", pngOf(t, noisy), false},
		{"data: prefix", "data:image/png;base64," + pngOf(t, black), true},
		{"gray image", encode(t, image.NewGray(image.Rect(0, 0, 128, 72))), true},
	} {
		got, err := flatPNG(c.data)
		if err != nil || got != c.want {
			t.Errorf("%s: %v %v, want %v", c.name, got, err, c.want)
		}
	}
	for _, bad := range []string{"%%%", "data:image/png;base64,!!", base64.StdEncoding.EncodeToString([]byte("GIF89a not a png"))} {
		if _, err := flatPNG(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// Render counts only: output counts would count each frame twice.
func TestStats(t *testing.T) {
	f := &shotOBS{fake: &fake{}, stats: map[string]any{"renderSkippedFrames": 3, "renderTotalFrames": 1000,
		"outputSkippedFrames": 4, "outputTotalFrames": 900, "activeFps": 30}}
	var c Client
	fr, err := c.Stats(fakeOBS(t, "", f))
	if err != nil || fr != (Frames{3, 1000}) {
		t.Errorf("%+v %v", fr, err)
	}
}
