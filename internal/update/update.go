// Package update asks GitHub whether a newer Hallstream release exists. It
// only reads the release's version and page; it never downloads anything.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// URL is GitHub's newest published release of the app (drafts and
// pre-releases are left out by GitHub).
const URL = "https://api.github.com/repos/kauredo/hallstream/releases/latest"

// Release is a published version and the page that describes it.
type Release struct {
	Version string `json:"version"` // "1.2.0", without the tag's "v"
	URL     string `json:"url"`     // the release page on GitHub
}

// Latest asks url for the newest release. agent names the program to GitHub.
func Latest(ctx context.Context, c *http.Client, url, agent string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", agent)
	res, err := c.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub: %s", res.Status)
	}
	var body struct {
		Tag  string `json:"tag_name"`
		Page string `json:"html_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Release{}, err
	}
	v := strings.TrimPrefix(body.Tag, "v")
	if _, ok := parse(v); !ok || !strings.HasPrefix(body.Page, "https://github.com/") {
		return Release{}, fmt.Errorf("GitHub: unexpected release %q", body.Tag)
	}
	return Release{Version: v, URL: body.Page}, nil
}

// Newer reports whether latest is a later version than current. A build
// that is not a plain X.Y.Z (dev, 0.0.0-dev.abc1234) never asks to update.
func Newer(current, latest string) bool {
	a, okA := parse(current)
	b, okB := parse(latest)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
