package obsws

import (
	"slices"
	"testing"
)

// insetFake is fake with SetSceneItemEnabled on byScene.
type insetFake struct{ *fake }

func (f insetFake) answer(typ string, d map[string]any) (bool, int, any) {
	if typ != "SetSceneItemEnabled" {
		return f.fake.answer(typ, d)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, typ)
	for _, it := range f.byScene[d["sceneName"].(string)] {
		if it["sceneItemId"] == int(d["sceneItemId"].(float64)) {
			it["sceneItemEnabled"] = d["sceneItemEnabled"]
		}
	}
	return true, 100, nil
}

func (f insetFake) enabled(scene, source string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, it := range f.byScene[scene] {
		if it["sourceName"] == source {
			return it["sceneItemEnabled"] == true
		}
	}
	return false
}

func insetScenes(srcW, srcH float64) map[string][]map[string]any {
	it := func(id int, name string, on bool, w, h float64) map[string]any {
		return map[string]any{"sceneItemId": id, "sceneItemIndex": id - 1, "sourceName": name, "inputKind": "dshow_input", "sceneItemEnabled": on,
			"sceneItemTransform": map[string]any{"sourceWidth": w, "sourceHeight": h, "scaleX": 1.0, "scaleY": 1.0, "alignment": 5.0}}
	}
	return map[string][]map[string]any{
		"Câmara 1": {it(1, "A", true, 1920, 1080), it(2, "B", false, srcW, srcH), it(3, "Grafismo", true, 1920, 1080)},
		"Câmara 2": {it(1, "B", true, srcW, srcH), it(2, "A", true, 1920, 1080), it(3, "Grafismo", true, 1920, 1080)},
	}
}

func TestInsetShowsAndHides(t *testing.T) {
	f := insetFake{&fake{byScene: insetScenes(1280, 720), canvasW: 1280, canvasH: 720}}
	cfg := fakeOBS(t, "", f)
	c := &Client{}
	hide := []ItemRef{{"Câmara 1", "B"}, {"Câmara 2", "A"}}
	box := Rect{1376, 746, 480, 270}
	r, err := c.Inset(cfg, hide, &ItemRef{"Câmara 1", "B"}, box)
	if err != nil {
		t.Fatal(err)
	}
	if !samePlace(Place{Rect: *r}, Place{Rect: box}) {
		t.Errorf("rect %+v", *r)
	}
	tr := last(t, f.fake)
	// The canvas is 1280x720: the box is two thirds of the overlay's.
	if !near(tr["positionX"].(float64), 1376*2.0/3) || !near(tr["scaleX"].(float64), 320.0/1280) {
		t.Errorf("transform %v", tr)
	}
	if !f.enabled("Câmara 1", "B") || f.enabled("Câmara 2", "A") {
		t.Errorf("shown in camera 1 %v, in camera 2 %v", f.enabled("Câmara 1", "B"), f.enabled("Câmara 2", "A"))
	}
	if r, err := c.Inset(cfg, hide, nil, box); err != nil || r != nil || f.enabled("Câmara 1", "B") {
		t.Errorf("off: %v %v shown %v", r, err, f.enabled("Câmara 1", "B"))
	}
	if !f.enabled("Câmara 1", "A") || !f.enabled("Câmara 2", "B") {
		t.Error("hid a camera in its own scene")
	}
}

// A portrait picture sits in the middle of the box, whole.
func TestInsetPortrait(t *testing.T) {
	f := insetFake{&fake{byScene: insetScenes(720, 1280), canvasW: 1920, canvasH: 1080}}
	r, err := (&Client{}).Inset(fakeOBS(t, "", f), nil, &ItemRef{"Câmara 1", "B"}, Rect{64, 746, 480, 270})
	if err != nil {
		t.Fatal(err)
	}
	if w := 270 * 720.0 / 1280; !near(r.W, w) || !near(r.H, 270) || !near(r.X, 64+(480-w)/2) || !near(r.Y, 746) {
		t.Errorf("rect %+v", *r)
	}
}

func TestInsetMissingItem(t *testing.T) {
	f := insetFake{&fake{byScene: insetScenes(1280, 720), canvasW: 1920, canvasH: 1080}}
	_, err := (&Client{}).Inset(fakeOBS(t, "", f), nil, &ItemRef{"Câmara 1", "C"}, Rect{64, 746, 480, 270})
	if err == nil {
		t.Fatal("no error for a camera missing from the scene")
	}
	if slices.Contains(f.asked, "SetSceneItemTransform") {
		t.Error("moved something")
	}
}
