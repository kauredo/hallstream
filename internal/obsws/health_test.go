package obsws

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

// shotOBS is a fake that also answers GetSourceScreenshot and GetStats.
type shotOBS struct {
	*fake
	shot  string
	stats map[string]any
}

func (s *shotOBS) answer(typ string, d map[string]any) (bool, int, any) {
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

// pngOf is a 128x72 PNG in base64, each pixel from at.
func pngOf(t *testing.T, at func(x, y int) color.Color) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 128, 72))
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

func TestJudge(t *testing.T) {
	pic := Look{Found: true, Width: 1280, Height: 720, Shot: "a"}
	other := pic
	other.Shot = "b"
	flat := pic
	flat.Flat = true
	for _, c := range []struct {
		name      string
		prev, cur Look
		want      string
	}{
		{"missing", pic, Look{}, "no_image"},
		{"0x0", pic, Look{Found: true}, "no_image"},
		{"flat", Look{}, flat, "no_image"},
		{"same shot", pic, pic, "frozen"},
		{"new shot", pic, other, "ok"},
		{"first look", Look{}, pic, "ok"},
		{"flat and same", flat, flat, "no_image"},
	} {
		if got := Judge(c.prev, c.cur); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLookAt(t *testing.T) {
	f := &shotOBS{fake: &fake{program: "Jogo", byScene: map[string][]map[string]any{
		"Câmara 1": {camItem("Grafismo", 1920, 1080), camItem("Cam", 1280, 720)},
		"Câmara 2": {camItem("Telemóvel", 0, 0)},
		"Jogo":     {camItem("Cam", 1280, 720)},
	}}}
	cfg := fakeOBS(t, "", f)
	var c Client

	f.shot = pngOf(t, noisy)
	l, err := c.LookAt(cfg, "Câmara 1", "Cam")
	if err != nil {
		t.Fatal(err)
	}
	if !l.Found || l.Width != 1280 || l.Height != 720 || l.Shot != f.shot || l.Flat {
		t.Errorf("noisy: %+v", l)
	}

	f.shot = pngOf(t, black)
	if l, err = c.LookAt(cfg, "Câmara 1", "Cam"); err != nil || !l.Flat {
		t.Errorf("black: %+v %v", l, err)
	}
	// Within 6 of the first pixel is still one colour; 7 is not.
	f.shot = pngOf(t, func(x, _ int) color.Color { return color.RGBA{uint8(x % 7), 0, 0, 255} })
	if l, err = c.LookAt(cfg, "Câmara 1", "Cam"); err != nil || !l.Flat {
		t.Errorf("near black: %+v %v", l, err)
	}
	f.shot = pngOf(t, func(x, _ int) color.Color { return color.RGBA{uint8(x % 8), 0, 0, 255} })
	if l, err = c.LookAt(cfg, "Câmara 1", "Cam"); err != nil || l.Flat {
		t.Errorf("dark noise: %+v %v", l, err)
	}

	f.shot = "data:image/png;base64," + pngOf(t, noisy)
	if l, err = c.LookAt(cfg, "Câmara 1", "Cam"); err != nil || l.Flat || l.Shot != f.shot {
		t.Errorf("data URI: %+v %v", l, err)
	}

	f.fake.asked = nil
	if l, err = c.LookAt(cfg, "Câmara 2", "Telemóvel"); err != nil || !l.Found || l.Width != 0 || l.Shot != "" {
		t.Errorf("0x0: %+v %v", l, err)
	}
	if l, err = c.LookAt(cfg, "Câmara 2", "Outra"); err != nil || l.Found {
		t.Errorf("missing: %+v %v", l, err)
	}
	if slices.Contains(f.fake.asked, "GetSourceScreenshot") {
		t.Errorf("a camera with no picture got a screenshot: %v", f.fake.asked)
	}

	f.shot = pngOf(t, noisy)
	if l, err = c.LookAt(cfg, "", "Cam"); err != nil || !l.Found {
		t.Errorf("program scene: %+v %v", l, err)
	}

	f.shot = "not a png"
	if _, err = c.LookAt(cfg, "Câmara 1", "Cam"); err == nil {
		t.Error("a screenshot that does not decode must be an error")
	}
}

func TestStats(t *testing.T) {
	f := &shotOBS{fake: &fake{}, stats: map[string]any{"renderSkippedFrames": 3, "renderTotalFrames": 1000,
		"outputSkippedFrames": 4, "outputTotalFrames": 900, "activeFps": 30}}
	var c Client
	fr, err := c.Stats(fakeOBS(t, "", f))
	if err != nil || fr != (Frames{7, 1900}) {
		t.Errorf("%+v %v", fr, err)
	}
}
