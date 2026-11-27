package obsws

import (
	"strconv"

	"github.com/kauredo/hallstream/internal/i18n"
)

// ItemRef is a source's item in a scene.
type ItemRef struct {
	Scene  string
	Source string
}

// Inset hides the shown items of hide, all but show, then puts show's item
// contained in box (1920x1080 overlay px, scaled to the canvas) and shows
// it. It returns where the picture is in overlay px, nil with show nil. A
// scene OBS refuses to list is skipped while hiding. home is the scene
// where show's source is the camera, asked for the picture's size when
// show's item has none.
func (c *Client) Inset(cfg Config, hide []ItemRef, show *ItemRef, home string, box Rect) (*Rect, error) {
	lists := map[string][]sceneItem{}
	list := func(scene string) ([]sceneItem, error) {
		if l, ok := lists[scene]; ok {
			return l, nil
		}
		var r struct {
			Items []sceneItem `json:"sceneItems"`
		}
		err := c.Request(cfg, "GetSceneItemList", map[string]string{"sceneName": scene}, &r)
		if err == nil {
			lists[scene] = r.Items
		}
		return r.Items, err
	}
	enable := func(scene string, id int, on bool) error {
		return c.Request(cfg, "SetSceneItemEnabled", map[string]any{"sceneName": scene, "sceneItemId": id, "sceneItemEnabled": on}, nil)
	}
	for _, h := range hide {
		if show != nil && h == *show {
			continue
		}
		items, err := list(h.Scene)
		if _, refused := err.(*RequestError); refused {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Name == h.Source && it.Enabled {
				if err := enable(h.Scene, it.ID, false); err != nil {
					return nil, err
				}
			}
		}
	}
	if show == nil {
		return nil, nil
	}
	var v struct {
		W float64 `json:"baseWidth"`
		H float64 `json:"baseHeight"`
	}
	if err := c.Request(cfg, "GetVideoSettings", nil, &v); err != nil {
		return nil, err
	}
	items, err := list(show.Scene)
	if err != nil {
		return nil, err
	}
	it := find(items, show.Source)
	if it == nil {
		return nil, i18n.Err("obs.inset.sem_item", "camara", strconv.Quote(show.Source), "cena", strconv.Quote(show.Scene))
	}
	w, h := it.Transform.SourceWidth, it.Transform.SourceHeight
	if (w <= 0 || h <= 0) && home != "" && home != show.Scene {
		// An item that has never shown may have no size yet.
		if l, err := list(home); err == nil {
			if o := find(l, show.Source); o != nil {
				w, h = o.Transform.SourceWidth, o.Transform.SourceHeight
			}
		}
	}
	if w <= 0 || h <= 0 || v.W <= 0 || v.H <= 0 {
		return nil, i18n.Err("obs.camera.sem_imagem", "camara", show.Source)
	}
	sx, sy := v.W/1920, v.H/1080
	p := Contain(w, h, Rect{box.X * sx, box.Y * sy, box.W * sx, box.H * sy})
	if err := c.Request(cfg, "SetSceneItemTransform", map[string]any{"sceneName": show.Scene, "sceneItemId": it.ID,
		"sceneItemTransform": p.transform(w, h)}, nil); err != nil {
		return nil, err
	}
	if !it.Enabled {
		if err := enable(show.Scene, it.ID, true); err != nil {
			return nil, err
		}
	}
	return &Rect{p.X / sx, p.Y / sy, p.W / sx, p.H / sy}, nil
}
