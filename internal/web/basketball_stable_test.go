package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	assets "github.com/kauredo/hallstream/web"
)

// measure is what a basketball screen must draw the same every time it loads:
// the crest in the home tile, loaded, and the scores' font and width. It runs
// once the page has settled (web fonts loaded, every image done).
const measure = `(() => {
  const sel = '#s-' + document.documentElement.dataset.screen;
  const img = document.querySelector(sel + ' [data-tile="home"] img');
  return JSON.stringify({
    crest: !!img && img.complete && img.naturalWidth > 0 && img.closest('[data-tile]').classList.contains('has-img'),
    font: document.fonts.check('800 96px "Barlow Condensed"'),
    scores: [...document.querySelectorAll(sel + ' .blk-sc, ' + sel + ' .sb-row .sc > [data-b]')].map((el) => {
      const r = document.createRange();
      r.selectNodeContents(el);
      const cs = getComputedStyle(el);
      return [el.textContent.trim(), cs.fontFamily, cs.fontSize, cs.fontVariantNumeric, Math.round(r.getBoundingClientRect().width * 100) / 100, Math.round(el.getBoundingClientRect().width * 100) / 100];
    }),
  });
})()`

// The flat basketball fixtures draw the same on every load, with the crest in
// the home tile and the scores in the loaded web font. A screenshot taken
// before the fonts and the crest arrive shows a wider score and an empty tile,
// so the test waits for both and then compares two loads.
func TestBasketballScreensDrawTheSameEveryTime(t *testing.T) {
	srv := httptest.NewServer(newServerFiles(t, assets.FS))
	t.Cleanup(srv.Close)
	tab := chrome(t)
	for _, fx := range []string{"marcador", "final", "intervalo"} {
		t.Run(fx, func(t *testing.T) {
			var seen [2]string
			for i := range seen {
				if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/overlay?anim=0&preview=1&state=/overlay/fixtures/"+fx+".json")); err != nil {
					t.Fatal(err)
				}
				settled := `document.documentElement.dataset.screen === "` + fx + `" && document.fonts.status === "loaded" && [...document.images].every((i) => i.complete) && !!document.querySelector('#s-` + fx + ` [data-tile="home"]').classList.contains("has-img")`
				if err := until(tab, settled); err != nil {
					t.Fatalf("load %d: the screen never settled with the crest drawn: %v", i+1, err)
				}
				got, err := chromedp.Run(tab, chromedp.Evaluate[string](measure))
				if err != nil {
					t.Fatal(err)
				}
				seen[i] = got
			}
			if seen[0] != seen[1] {
				t.Errorf("two loads drew differently:\n%s\n%s", seen[0], seen[1])
			}
			if !strings.Contains(seen[0], `"crest":true`) || !strings.Contains(seen[0], `"font":true`) {
				t.Errorf("the crest or the web font is missing: %s", seen[0])
			}
			// The scores are tabular figures. A score bound under another name once lost
			// that and drew narrower (the selector on [data-b$="Score"] no longer matched).
			if got := strings.Count(seen[0], `"tabular-nums"`); got < 2 {
				t.Errorf("%d scores in tabular figures, want at least 2: %s", got, seen[0])
			}
		})
	}
}
