package obsws

import (
	"strconv"

	"github.com/kauredo/hallstream/internal/i18n"
)

// The inset's box on the 1920x1080 overlay.
const InsetW, InsetH = 480, 270

// ItemRef is a source's item in a scene.
type ItemRef struct {
	Scene  string
	Source string
}

// Inset hides the shown items of hide, all but show, then puts show's item
// contained in box (1920x1080 overlay px, scaled to the canvas) and shows
// it. It returns where the picture is in overlay px, nil with show nil. A
// scene OBS refuses to list is skipped while hiding.
func (c *Client) Inset(cfg Config, hide []ItemRef, show *ItemRef, box Rect) (*Rect, error) {
	lists := map[string][]sceneItem{}
	list := func(scene string) ([]sceneItem, error) {
		if l, ok := lists[scene]; ok {
			return l, nil
		}
		var r struct {
			Items []sceneItem `json:"sceneItems"`
		}
		err := c.Request(cfg, "GetSceneItemList", map[string]string{"sceneName": scene}, &r)
		lists[scene] = r.Items
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
	tr := it.Transform
	if tr.SourceWidth <= 0 || tr.SourceHeight <= 0 || v.W <= 0 || v.H <= 0 {
		return nil, i18n.Err("obs.camera.sem_imagem", "camara", show.Source)
	}
	sx, sy := v.W/1920, v.H/1080
	p := Contain(tr.SourceWidth, tr.SourceHeight, Rect{box.X * sx, box.Y * sy, box.W * sx, box.H * sy})
	if err := c.Request(cfg, "SetSceneItemTransform", map[string]any{"sceneName": show.Scene, "sceneItemId": it.ID,
		"sceneItemTransform": p.transform(tr.SourceWidth, tr.SourceHeight)}, nil); err != nil {
		return nil, err
	}
	if !it.Enabled {
		if err := enable(show.Scene, it.ID, true); err != nil {
			return nil, err
		}
	}
	return &Rect{p.X / sx, p.Y / sy, p.W / sx, p.H / sy}, nil
}
