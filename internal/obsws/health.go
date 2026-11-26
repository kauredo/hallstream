package obsws

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
)

// Look is one camera as one health round saw it.
type Look struct {
	Found  bool    // the scene has an item with the source's name
	Width  float64 // the source's size; 0 when it sends nothing
	Height float64
	Shot   string // the screenshot's imageData; "" when none was taken
	Flat   bool   // the screenshot is one colour
}

// The health screenshot: small enough to be cheap, big enough that a live
// camera's sensor noise survives the scaling.
const shotW, shotH = 128, 72

// flatBy is how far a channel may stray from the first pixel's in a picture
// that still counts as one colour.
const flatBy = 6

// LookAt reads source's size in scene, and when it has one, a 128x72 PNG
// screenshot. An empty scene means the program scene.
func (c *Client) LookAt(cfg Config, scene, source string) (Look, error) {
	if scene == "" {
		var err error
		if scene, err = c.program(cfg); err != nil {
			return Look{}, err
		}
	}
	var r struct {
		Items []sceneItem `json:"sceneItems"`
	}
	if err := c.Request(cfg, "GetSceneItemList", map[string]string{"sceneName": scene}, &r); err != nil {
		return Look{}, err
	}
	var l Look
	for _, it := range r.Items {
		if it.Name == source {
			l = Look{Found: true, Width: it.Transform.SourceWidth, Height: it.Transform.SourceHeight}
			break
		}
	}
	if !l.Found || l.Width <= 0 || l.Height <= 0 {
		return l, nil
	}
	var shot struct {
		Data string `json:"imageData"`
	}
	err := c.Request(cfg, "GetSourceScreenshot", map[string]any{
		"sourceName": source, "imageFormat": "png", "imageWidth": shotW, "imageHeight": shotH,
	}, &shot)
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

// flatPNG reports whether a base64 PNG, with or without a data: prefix, is
// one colour.
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
	r0, g0, b0, a0 := img.At(box.Min.X, box.Min.Y).RGBA()
	near := func(v, w uint32) bool { d := int(v>>8) - int(w>>8); return d >= -flatBy && d <= flatBy }
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if !near(r, r0) || !near(g, g0) || !near(b, b0) || !near(a, a0) {
				return false, nil
			}
		}
	}
	return true, nil
}

// Judge is the camera's health from this look and the one 10 s before:
// "no_image", "frozen" or "ok".
func Judge(prev, cur Look) string {
	switch {
	case !cur.Found || cur.Width <= 0 || cur.Height <= 0 || cur.Flat:
		return "no_image"
	case prev.Shot != "" && prev.Shot == cur.Shot:
		return "frozen"
	}
	return "ok"
}

// Frames are OBS's skipped and total frames since it started, render and
// output threads together.
type Frames struct{ Skipped, Total float64 }

// Stats reads Frames from GetStats.
func (c *Client) Stats(cfg Config) (Frames, error) {
	var st struct {
		RenderSkipped float64 `json:"renderSkippedFrames"`
		RenderTotal   float64 `json:"renderTotalFrames"`
		OutputSkipped float64 `json:"outputSkippedFrames"`
		OutputTotal   float64 `json:"outputTotalFrames"`
	}
	if err := c.Request(cfg, "GetStats", nil, &st); err != nil {
		return Frames{}, err
	}
	return Frames{st.RenderSkipped + st.OutputSkipped, st.RenderTotal + st.OutputTotal}, nil
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
