package obsws

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strings"
)

// Look is one camera as one health round saw it.
type Look struct {
	Found   bool    // the scene has an item with the source's name
	Width   float64 // the source's size; 0 when it sends nothing
	Height  float64
	Shot    string // the screenshot's imageData; "" when none was taken
	Flat    bool   // the screenshot is one colour
	Refused bool   // OBS could not take the screenshot (600 or 702)
}

// The health screenshot: small enough to be cheap, big enough that a live
// camera's sensor noise survives the scaling.
const shotW, shotH = 128, 72

// flatRange is the widest spread, lowest to highest, a channel may have
// across a picture that still counts as one colour.
const flatRange = 12

// failedRequest is obs-websocket's RequestProcessingFailed, what
// GetSourceScreenshot answers when the source renders nothing.
const failedRequest = 702

// ErrBusy: LookAt stopped because busy said so between two requests.
var ErrBusy = errors.New("obsws: busy")

// ErrNotShown: LookAt was asked about the program scene and the camera is
// not in it, so there is nothing to judge.
var ErrNotShown = errors.New("obsws: camera not in the program scene")

// LookAt reads source's size in scene, and when it has one, a 128x72 PNG
// screenshot. An empty scene means the program scene, and a camera missing
// from it is ErrNotShown. busy is asked before each request; when it says
// yes LookAt returns ErrBusy. A missing scene, or a screenshot OBS can't
// take, is a Look, not an error.
func (c *Client) LookAt(cfg Config, scene, source string, busy func() bool) (Look, error) {
	program := scene == ""
	if program {
		if busy() {
			return Look{}, ErrBusy
		}
		var err error
		if scene, err = c.program(cfg); err != nil {
			return Look{}, err
		}
	}
	if busy() {
		return Look{}, ErrBusy
	}
	var r struct {
		Items []sceneItem `json:"sceneItems"`
	}
	err := c.Request(cfg, "GetSceneItemList", map[string]string{"sceneName": scene}, &r)
	if refused(err) {
		err = nil // the scene is gone: no item, so no picture
	}
	if err != nil {
		return Look{}, err
	}
	var l Look
	for _, it := range r.Items {
		if it.Name == source {
			l = Look{Found: true, Width: it.Transform.SourceWidth, Height: it.Transform.SourceHeight}
			break
		}
	}
	if !l.Found && program {
		return Look{}, ErrNotShown
	}
	if !l.Found || l.Width <= 0 || l.Height <= 0 {
		return l, nil
	}
	if busy() {
		return Look{}, ErrBusy
	}
	var shot struct {
		Data string `json:"imageData"`
	}
	err = c.Request(cfg, "GetSourceScreenshot", map[string]any{
		"sourceName": source, "imageFormat": "png", "imageWidth": shotW, "imageHeight": shotH,
	}, &shot)
	if refused(err) {
		l.Refused = true
		return l, nil
	}
	if err != nil {
		return Look{}, err
	}
	flat, err := flatPNG(shot.Data)
	if err != nil {
		return Look{}, err
	}
	l.Shot, l.Flat = shot.Data, flat
	return l, nil
}

// refused reports whether OBS answered that the thing asked for is missing
// or gave nothing.
func refused(err error) bool {
	var re *RequestError
	return errors.As(err, &re) && (re.Code == NotFound || re.Code == failedRequest)
}

// flatPNG reports whether a base64 PNG, with or without a data: prefix, is
// one colour: each of R, G, B and A spans at most flatRange across it.
func flatPNG(data string) (bool, error) {
	if _, after, ok := strings.Cut(data, ","); ok {
		data = after
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return false, err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return false, err
	}
	box := img.Bounds()
	lo := [4]int{255, 255, 255, 255}
	var hi [4]int
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			for i, v := range [4]uint32{r, g, b, a} {
				v8 := int(v >> 8)
				lo[i], hi[i] = min(lo[i], v8), max(hi[i], v8)
			}
		}
	}
	for i := range lo {
		if hi[i]-lo[i] > flatRange {
			return false, nil
		}
	}
	return true, nil
}

// Rounds before a state changes: a camera is frozen after this many
// identical screenshot pairs in a row, and has no image after this many
// one-colour screenshots in a row. One good look brings it back to "ok".
const (
	frozenPairs = 2
	flatLooks   = 2
	// failedLooks looks that fail in a row forget the camera's state.
	failedLooks = 3
)

// Vitals is one camera's health across rounds.
type Vitals struct {
	State string // "no_image", "frozen", "ok", or "" when not known
	last  Look
	same  int // identical screenshot pairs in a row
	flat  int // one-colour screenshots in a row
	fails int // failed looks in a row
}

// See is the camera's health after a new look. A missing item, a source
// with no size or a refused screenshot is "no_image" at once; a one-colour
// screenshot only on the second in a row, and a frozen picture only on the
// second identical pair (three identical screenshots). Before that the
// state stays as it was.
func (w Vitals) See(cur Look) Vitals {
	next := Vitals{State: w.State, last: cur}
	switch {
	case !cur.Found || cur.Width <= 0 || cur.Height <= 0 || cur.Refused:
		next.State = "no_image"
	case cur.Flat:
		if next.flat = w.flat + 1; next.flat >= flatLooks {
			next.State = "no_image"
		}
	default:
		if w.last.Shot != "" && w.last.Shot == cur.Shot {
			next.same = w.same + 1
		}
		next.State = "ok"
		if next.same >= frozenPairs {
			next.State = "frozen"
		}
	}
	return next
}

// Failed is the camera's health after a look that failed: unchanged, until
// failedLooks in a row forget it.
func (w Vitals) Failed() Vitals {
	if w.fails++; w.fails >= failedLooks {
		return Vitals{}
	}
	return w
}

// Frames are OBS's skipped and total frames since it started, from the
// render thread. Its output counts only move while streaming or recording,
// and adding them would count each frame twice.
type Frames struct{ Skipped, Total float64 }

// Stats reads Frames from GetStats.
func (c *Client) Stats(cfg Config) (Frames, error) {
	var st struct {
		Skipped float64 `json:"renderSkippedFrames"`
		Total   float64 `json:"renderTotalFrames"`
	}
	if err := c.Request(cfg, "GetStats", nil, &st); err != nil {
		return Frames{}, err
	}
	return Frames{st.Skipped, st.Total}, nil
}

// program is the program scene's name.
func (c *Client) program(cfg Config) (string, error) {
	var sc struct {
		Name string `json:"sceneName"`
		Old  string `json:"currentProgramSceneName"`
	}
	if err := c.Request(cfg, "GetCurrentProgramScene", nil, &sc); err != nil {
		return "", err
	}
	if sc.Name == "" {
		return sc.Old, nil
	}
	return sc.Name, nil
}
