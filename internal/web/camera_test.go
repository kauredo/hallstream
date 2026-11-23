package web

import (
	"os"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
)

// The camera and the overlay read the time-out window from the same file.
func TestSqueezeTokens(t *testing.T) {
	tok, err := readSqueeze(string(overlayCSS(t)))
	if err != nil {
		t.Fatal(err)
	}
	if tok.win != (obsws.Rect{X: 416, Y: 64, W: 1440, H: 810}) || tok.dur != 900*time.Millisecond {
		t.Errorf("window %v, %v", tok.win, tok.dur)
	}
	if e := tok.ease(0.5); e < 0.49 || e > 0.51 || tok.ease(0.2) > 0.1 {
		t.Errorf("ease %v %v", tok.ease(0.5), tok.ease(0.2))
	}
	if _, err := readSqueeze("--win-x: 1px;"); err == nil {
		t.Error("want an error for a CSS without the tokens")
	}
}

func TestSettingsKeepCamera(t *testing.T) {
	s := newServer(t)
	full := &obsws.Full{SourceWidth: 1280, SourceHeight: 720, Fit: "contain"}
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.CameraFull = full })

	set := s.Store.Settings()
	set.OBS.CameraFull = nil // a panel page from before the fit
	set.OBS.CameraFit = ""
	var got store.Settings
	if code := call(t, s, "PUT", "/api/settings", set, &got); code != 200 {
		t.Fatal(code)
	}
	if got.OBS.CameraFull == nil || got.OBS.CameraFit != "contain" {
		t.Errorf("camera %+v fit %q", got.OBS.CameraFull, got.OBS.CameraFit)
	}
	set.OBS.CameraSource = "DroidCam"
	call(t, s, "PUT", "/api/settings", set, &got)
	if got.OBS.CameraFull != nil {
		t.Error("another camera must be fitted again")
	}
}

func TestScreenWithoutOBS(t *testing.T) {
	s := newServer(t)
	var resp map[string]any
	if code := call(t, s, "POST", "/api/screen", map[string]any{"screen": "plantel"}, &resp); code != 200 || resp["obs_error"] != nil {
		t.Fatalf("%d %v", code, resp)
	}
	// OBS on but not running: the overlay changes screen and the panel gets the error.
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Enabled, set.OBS.Port = true, 1 })
	resp = nil
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, &resp)
	if resp["obs_error"] == nil || s.camErr != nil || s.State.Snapshot().Screen.Current != "desconto" {
		t.Errorf("%v %q", resp, s.State.Snapshot().Screen.Current)
	}
}

func overlayCSS(t *testing.T) []byte {
	css, err := os.ReadFile("../../web/overlay/overlay.css")
	if err != nil {
		t.Fatal(err)
	}
	return css
}
