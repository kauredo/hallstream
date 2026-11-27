package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestAssets(t *testing.T) {
	body := `{"tag_name":"v1.4.0","html_url":"https://github.com/kauredo/hallstream/releases/tag/v1.4.0","assets":[
		{"name":"Hallstream-1.4.0.zip","browser_download_url":"https://github.com/kauredo/hallstream/releases/download/v1.4.0/Hallstream-1.4.0.zip"},
		{"name":"SHA256SUMS","browser_download_url":"https://github.com/kauredo/hallstream/releases/download/v1.4.0/SHA256SUMS"},
		{"name":"evil.zip","browser_download_url":"https://evil.example/evil.zip"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer srv.Close()
	got, err := Latest(context.Background(), srv.Client(), srv.URL, "Hallstream/1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 2 || got.Assets["SHA256SUMS"] == "" || got.Assets["evil.zip"] != "" {
		t.Errorf("assets %v", got.Assets)
	}
}

func TestAssetName(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "Hallstream-1.2.3.zip"},
		{"windows", "386", "Hallstream-1.2.3.zip"},
		{"windows", "arm64", ""},
		{"darwin", "arm64", "Hallstream-1.2.3-macos.dmg"},
		{"darwin", "amd64", "Hallstream-1.2.3-macos.dmg"},
		{"linux", "amd64", "Hallstream-1.2.3-linux-x64.tar.gz"},
		{"linux", "arm64", ""},
		{"freebsd", "amd64", ""},
	} {
		if got := AssetName(c.goos, c.goarch, "1.2.3"); got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

// release serves files and a SHA256SUMS listing sums (the real hash of each
// file unless sums overrides it).
func release(t *testing.T, files map[string]string, sums map[string]string) (*httptest.Server, Release) {
	t.Helper()
	var list strings.Builder
	for name, data := range files {
		h := sha256.Sum256([]byte(data))
		sum := hex.EncodeToString(h[:])
		if s, ok := sums[name]; ok {
			sum = s
		}
		if sum != "" {
			fmt.Fprintf(&list, "%s  %s\n", sum, name)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(list.String())) })
	for name, data := range files {
		mux.HandleFunc("/files/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(data)) })
		mux.HandleFunc("/moved/"+name, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/files/"+name, http.StatusFound)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	rel := Release{Version: "1.4.0", Assets: map[string]string{"SHA256SUMS": srv.URL + "/SHA256SUMS"}}
	for name := range files {
		rel.Assets[name] = srv.URL + "/files/" + name
	}
	return srv, rel
}

func TestDownload(t *testing.T) {
	_, rel := release(t, map[string]string{"Hallstream-1.4.0.zip": "the program"}, nil)
	dir := t.TempDir()
	var seen []int
	path, err := Download(context.Background(), http.DefaultClient, rel, "Hallstream-1.4.0.zip", dir, func(p int) { seen = append(seen, p) })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "the program" || path != filepath.Join(dir, "Hallstream-1.4.0.zip") {
		t.Errorf("%s: %q", path, b)
	}
	if len(seen) == 0 || seen[len(seen)-1] != 100 {
		t.Errorf("progress %v", seen)
	}
}

func TestDownloadRefuses(t *testing.T) {
	wrong := strings.Repeat("0", 64)
	for _, c := range []struct {
		name  string
		files map[string]string
		sums  map[string]string
		ask   string
	}{
		{"wrong hash", map[string]string{"a.zip": "x"}, map[string]string{"a.zip": wrong}, "a.zip"},
		{"not in sums", map[string]string{"a.zip": "x"}, map[string]string{"a.zip": ""}, "a.zip"},
		{"not in release", map[string]string{"a.zip": "x"}, nil, "b.zip"},
	} {
		_, rel := release(t, c.files, c.sums)
		dir := t.TempDir()
		if _, err := Download(context.Background(), http.DefaultClient, rel, c.ask, dir, nil); err == nil {
			t.Errorf("%s: no error", c.name)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("%s: left %v", c.name, left)
		}
	}
}

func TestDownloadTooBig(t *testing.T) {
	old := maxDownload
	maxDownload = 4
	t.Cleanup(func() { maxDownload = old })
	_, rel := release(t, map[string]string{"a.zip": "too long"}, nil)
	dir := t.TempDir()
	if _, err := Download(context.Background(), http.DefaultClient, rel, "a.zip", dir, nil); err == nil {
		t.Error("no error")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left %v", left)
	}
}

func TestDownloadRedirects(t *testing.T) {
	srv, rel := release(t, map[string]string{"a.zip": "x"}, nil)
	rel.Assets["a.zip"] = srv.URL + "/moved/a.zip"
	if _, err := Download(context.Background(), http.DefaultClient, rel, "a.zip", t.TempDir(), nil); err == nil {
		t.Error("a redirect to a host that isn't GitHub's was followed")
	}
	old := redirectHost
	redirectHost = func(string) bool { return true }
	t.Cleanup(func() { redirectHost = old })
	if _, err := Download(context.Background(), http.DefaultClient, rel, "a.zip", t.TempDir(), nil); err != nil {
		t.Errorf("redirect: %v", err)
	}
}

func TestRedirectHost(t *testing.T) {
	for host, want := range map[string]bool{
		"github.com": true, "release-assets.githubusercontent.com": true, "objects.githubusercontent.com": true,
		"raw.githubusercontent.com": false, "gist.githubusercontent.com": false, "evilgithub.com": false,
		"githubusercontent.com.evil.example": false,
	} {
		if redirectHost(host) != want {
			t.Errorf("%s: %v", host, !want)
		}
	}
}

func TestReleaseFile(t *testing.T) {
	for u, want := range map[string]bool{
		"https://github.com/kauredo/hallstream/releases/download/v1.0.0/Hallstream-1.0.0.zip":        true,
		"https://github.com/kauredo/hallstream/releases/download/../../../other/repo/releases/x.zip": false,
		"https://github.com/kauredo/hallstream/releases/download/v1.0.0/x.zip?redirect=evil.example": false,
		"https://evil.example/kauredo/hallstream/releases/download/v1.0.0/x.zip":                     false,
	} {
		if releaseFile(u) != want {
			t.Errorf("%s: %v", u, !want)
		}
	}
}

func TestSumFor(t *testing.T) {
	h := strings.Repeat("a", 64)
	sums := h + "  Hallstream-1.0.0.zip\r\n" + strings.Repeat("b", 64) + " *x-Hallstream-1.0.0.zip\n" + "abc  short.zip\n"
	if sumFor(sums, "Hallstream-1.0.0.zip") != h || sumFor(sums, "x-Hallstream-1.0.0.zip") != strings.Repeat("b", 64) || sumFor(sums, "short.zip") != "" {
		t.Error("sumFor")
	}
}

func TestDownloadTooManyRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("0", 64) + "  a.zip\n")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := redirectHost
	redirectHost = func(string) bool { return true }
	t.Cleanup(func() { redirectHost = old })
	rel := Release{Assets: map[string]string{"SHA256SUMS": srv.URL + "/SHA256SUMS", "a.zip": srv.URL + "/loop"}}
	if _, err := Download(context.Background(), http.DefaultClient, rel, "a.zip", t.TempDir(), nil); err == nil {
		t.Error("an endless redirect was followed")
	}
}
