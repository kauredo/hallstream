// Package update asks GitHub whether a newer Hallstream release exists, and,
// when the operator asks for it, downloads and installs it.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

// URL is GitHub's newest published release of the app (drafts and
// pre-releases are left out by GitHub).
const URL = "https://api.github.com/repos/kauredo/hallstream/releases/latest"

// assetPrefix is where every file of a release is downloaded from.
const assetPrefix = "https://github.com/kauredo/hallstream/releases/download/"

// Release is a published version and the page that describes it.
type Release struct {
	Version string `json:"version"` // "1.2.0", without the tag's "v"
	URL     string `json:"url"`     // the release page on GitHub
	// Assets maps each file's name to its download address.
	Assets map[string]string `json:"-"`
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
		Tag    string `json:"tag_name"`
		Page   string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Release{}, err
	}
	v := strings.TrimPrefix(body.Tag, "v")
	if _, ok := parse(v); !ok || !strings.HasPrefix(body.Page, "https://github.com/") {
		return Release{}, fmt.Errorf("GitHub: unexpected release %q", body.Tag)
	}
	rel := Release{Version: v, URL: body.Page, Assets: map[string]string{}}
	for _, a := range body.Assets {
		if releaseFile(a.URL) {
			rel.Assets[a.Name] = a.URL
		}
	}
	return rel, nil
}

// releaseFile reports whether u is a file of this project's releases, with
// no ".." or other trick in its path.
func releaseFile(u string) bool {
	p, err := url.Parse(u)
	return err == nil && strings.HasPrefix(u, assetPrefix) && p.Scheme == "https" && p.Host == "github.com" &&
		p.RawQuery == "" && path.Clean(p.Path) == p.Path && strings.HasPrefix(p.Path, "/kauredo/hallstream/releases/download/")
}

// AssetName is the release file that holds the program for this system, or
// "" where no release is built.
func AssetName(goos, goarch, version string) string {
	switch {
	case goos == "windows" && (goarch == "amd64" || goarch == "386"):
		return "Hallstream-" + version + ".zip"
	case goos == "darwin":
		return "Hallstream-" + version + "-macos.dmg"
	case goos == "linux" && goarch == "amd64":
		return "Hallstream-" + version + "-linux-x64.tar.gz"
	}
	return ""
}

// maxDownload caps a release file; a var so tests can lower it.
var maxDownload int64 = 300 << 20

// sumsName is the release file listing every other file's SHA-256.
const sumsName = "SHA256SUMS"

// maxSums caps SHA256SUMS, a few lines of text.
const maxSums = 1 << 20

// redirectHost reports whether a download may be sent on to host; GitHub
// answers with a redirect to its release file host (the newer name first).
// A var so tests can allow theirs.
var redirectHost = func(host string) bool {
	return host == "github.com" || host == "release-assets.githubusercontent.com" || host == "objects.githubusercontent.com"
}

// Download fetches the release file name into dir and checks it against the
// release's SHA256SUMS. It returns the file's path, and leaves nothing
// behind when it fails. progress, when not nil, gets the share done in
// percent as the file arrives, when the server says how big it is.
func Download(ctx context.Context, c *http.Client, rel Release, name, dir string, progress func(percent int)) (string, error) {
	sumsURL, fileURL := rel.Assets[sumsName], rel.Assets[name]
	if sumsURL == "" || fileURL == "" {
		return "", i18n.Err("log.atualizacao.sem_ficheiro", "ficheiro", name)
	}
	client := *c
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != via[0].URL.Scheme || !redirectHost(req.URL.Hostname()) {
			return fmt.Errorf("GitHub: redirect to %s refused", req.URL.Host)
		}
		return nil
	}
	sums, err := fetch(ctx, &client, sumsURL, maxSums)
	if err != nil {
		return "", err
	}
	want := sumFor(string(sums), name)
	if want == "" {
		return "", i18n.Err("log.atualizacao.sem_soma", "ficheiro", name)
	}
	path := filepath.Join(dir, name)
	if err := save(ctx, &client, fileURL, path, want, progress); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// sumFor finds name's hash in sha256sum output ("<hex>  <name>", or
// "<hex> *<name>" for binary mode).
func sumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		hash, file, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && strings.TrimLeft(file, " *") == name && len(hash) == 64 {
			return strings.ToLower(hash)
		}
	}
	return ""
}

func get(ctx context.Context, c *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("GitHub: %s", res.Status)
	}
	return res, nil
}

func fetch(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	res, err := get(ctx, c, url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(io.LimitReader(res.Body, limit))
}

func save(ctx context.Context, c *http.Client, url, path, want string, progress func(int)) error {
	res, err := get(ctx, c, url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	h := sha256.New()
	var w io.Writer = io.MultiWriter(f, h)
	if progress != nil && res.ContentLength > 0 {
		w = &counter{w: w, total: res.ContentLength, report: progress}
	}
	n, err := io.Copy(w, io.LimitReader(res.Body, maxDownload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		return err
	case n > maxDownload:
		return i18n.Err("log.atualizacao.grande_demais", "ficheiro", filepath.Base(path))
	case hex.EncodeToString(h.Sum(nil)) != want:
		return i18n.Err("log.atualizacao.soma_errada")
	}
	return nil
}

// counter reports each new whole percent of total written through it.
type counter struct {
	w           io.Writer
	done, total int64
	last        int
	report      func(int)
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.done += int64(n)
	if pct := int(min(c.done*100/c.total, 100)); pct != c.last {
		c.last = pct
		c.report(pct)
	}
	return n, err
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
