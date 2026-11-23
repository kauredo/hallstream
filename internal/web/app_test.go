package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	assets "github.com/kauredo/hallstream/web"
)

// lanGet asks for path the way a phone on the club Wi-Fi does, without a PIN.
func lanGet(s *Server, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "192.168.1.20:5000"
	req.Host = "192.168.1.40:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestManifestAndIconsOverLAN(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	set := s.Store.Settings()
	set.PhoneAccess, set.PIN = true, "1234"
	s.Store.SaveSettings(set)
	lanIP(t)

	w := lanGet(s, "/painel/manifest.webmanifest")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	var man struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		StartURL  string `json:"start_url"`
		Display   string `json:"display"`
		Icons     []struct{ Src, Sizes, Type, Purpose string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	if man.Name != "Hallstream" || man.ShortName != "Hallstream" || man.StartURL != "/painel" || man.Display != "standalone" {
		t.Errorf("manifest = %+v", man)
	}
	maskable := map[string]bool{}
	for _, ic := range man.Icons {
		if !strings.HasPrefix(ic.Src, "/") {
			t.Errorf("icon %s is relative: it must resolve the same from /painel on any host", ic.Src)
		}
		w := lanGet(s, ic.Src)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: %d %q", ic.Src, w.Code, w.Header().Get("Content-Type"))
			continue
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
		if err != nil || ic.Sizes != fmt.Sprintf("%dx%d", cfg.Width, cfg.Height) {
			t.Errorf("%s is %dx%d, manifest says %s (%v)", ic.Src, cfg.Width, cfg.Height, ic.Sizes, err)
		}
		if ic.Purpose == "maskable" {
			maskable[ic.Sizes] = true
		}
	}
	if !maskable["192x192"] || !maskable["512x512"] {
		t.Errorf("maskable icons: %v", maskable)
	}

	for path, ctype := range map[string]string{
		"/favicon.ico":                      "image/x-icon",
		"/assets/icon/favicon.svg":          "image/svg+xml",
		"/assets/icon/favicon-32.png":       "image/png",
		"/assets/icon/apple-touch-icon.png": "image/png",
	} {
		w := lanGet(s, path)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), ctype) {
			t.Errorf("%s: %d %q", path, w.Code, w.Header().Get("Content-Type"))
		}
	}
	page := lanGet(s, "/painel").Body.String()
	for _, link := range []string{`rel="manifest" href="/painel/manifest.webmanifest"`, `href="/assets/icon/favicon.svg"`, `rel="apple-touch-icon"`} {
		if !strings.Contains(page, link) {
			t.Errorf("/painel lacks %s", link)
		}
	}
}

// Every tab button, "?" button and Ajuda link in the panel points at something
// that is there.
func TestPainelLinksResolve(t *testing.T) {
	page, err := assets.FS.ReadFile("painel/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, ref := range []struct{ attr, prefix string }{{"data-tab", "tab-"}, {"data-goto", "tab-"}, {"data-help", ""}} {
		rest, n := html, 0
		for {
			i := strings.Index(rest, ref.attr+`="`)
			if i < 0 {
				break
			}
			rest = rest[i+len(ref.attr)+2:]
			name := rest[:strings.IndexByte(rest, '"')]
			n++
			if !strings.Contains(html, `id="`+ref.prefix+name+`"`) {
				t.Errorf(`%s="%s" has no id="%s%s"`, ref.attr, name, ref.prefix, name)
			}
		}
		if n == 0 {
			t.Errorf("no %s in the panel", ref.attr)
		}
	}
	if !strings.Contains(html, `data-tab="ajuda"`) {
		t.Error("no Ajuda tab")
	}
}

// After an exe update the browser and OBS must not keep an old script.
func TestEmbeddedFilesNoCache(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	for _, path := range []string{"/painel", "/overlay", "/painel/painel.js", "/overlay/overlay.js", "/overlay/overlay.css", "/fonts/fonts.css", "/assets/icon/favicon.svg", "/favicon.ico", "/painel/manifest.webmanifest"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, localGet(path))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d, Cache-Control %q", path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}

func TestDesktopShortcut(t *testing.T) {
	s := newServer(t)
	calls := 0
	s.Shortcut = func() (string, error) { calls++; return `C:\Users\Ana\Desktop\Hallstream.lnk`, nil }

	var res struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if code := call(t, s, "POST", "/api/system/desktop-shortcut", map[string]bool{"create": true}, &res); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if !res.OK || res.Path != `C:\Users\Ana\Desktop\Hallstream.lnk` || calls != 1 {
		t.Errorf("create = %+v, %d calls", res, calls)
	}
	if got := s.Store.Settings().DesktopShortcut; got != "created" {
		t.Errorf("desktop_shortcut = %q", got)
	}

	// The panel's settings form can't undo the answer.
	set := s.Store.Settings()
	set.DesktopShortcut = ""
	call(t, s, "PUT", "/api/settings", set, nil)
	if got := s.Store.Settings().DesktopShortcut; got != "created" {
		t.Errorf("PUT /api/settings changed desktop_shortcut to %q", got)
	}

	// A phone on the LAN (with the PIN) can't put a shortcut on the PC.
	set.PhoneAccess, set.PIN = true, "1234"
	s.Store.SaveSettings(set)
	req := httptest.NewRequest("POST", "/api/system/desktop-shortcut", strings.NewReader(`{"create":true}`))
	req.RemoteAddr = "192.168.1.20:5000"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PIN", "1234")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 403 || calls != 1 {
		t.Errorf("from the LAN: %d, %d calls", w.Code, calls)
	}

	if code := call(t, s, "POST", "/api/system/desktop-shortcut", map[string]bool{"create": false}, nil); code != 200 {
		t.Fatalf("decline: %d", code)
	}
	if got := s.Store.Settings().DesktopShortcut; got != "declined" || calls != 1 {
		t.Errorf("decline: desktop_shortcut = %q, %d calls", got, calls)
	}
}

func TestDesktopShortcutFails(t *testing.T) {
	s := newServer(t)
	s.Shortcut = func() (string, error) { return "", errors.New("acesso negado") }
	var e struct{ Error string }
	if code := call(t, s, "POST", "/api/system/desktop-shortcut", map[string]bool{"create": true}, &e); code != 500 || !strings.Contains(e.Error, "acesso negado") {
		t.Errorf("failure: %d %q", code, e.Error)
	}
	if got := s.Store.Settings().DesktopShortcut; got != "" {
		t.Errorf("a failed shortcut was remembered as %q", got)
	}
}

func TestDesktopShortcutNotWindows(t *testing.T) {
	s := newServer(t) // Shortcut is nil, as on macOS
	var e struct{ Error string }
	if code := call(t, s, "POST", "/api/system/desktop-shortcut", map[string]bool{"create": true}, &e); code != 501 || !strings.Contains(e.Error, "Windows") {
		t.Errorf("not windows: %d %q", code, e.Error)
	}
	if got := s.Store.Settings().DesktopShortcut; got != "" {
		t.Errorf("desktop_shortcut = %q", got)
	}
	var st struct{ Shortcut bool }
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Shortcut {
		t.Error("status offers a shortcut the system can't make")
	}
}

func TestDesktopShortcutWriteGuard(t *testing.T) {
	s := newServer(t)
	calls := 0
	s.Shortcut = func() (string, error) { calls++; return "x.lnk", nil }
	send := func(mod func(*http.Request)) int {
		req := httptest.NewRequest("POST", "/api/system/desktop-shortcut", strings.NewReader(`{"create":true}`))
		req.RemoteAddr = "127.0.0.1:5000"
		req.Host = "127.0.0.1:8765"
		req.Header.Set("Content-Type", "application/json")
		mod(req)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w.Code
	}
	cases := map[string]struct {
		mod  func(*http.Request)
		want int
	}{
		"other site":     {func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, 403},
		"rebinding host": {func(r *http.Request) { r.Host = "evil.example:8765" }, 403},
		"plain form":     {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
	}
	for name, c := range cases {
		if code := send(c.mod); code != c.want {
			t.Errorf("%s: %d, want %d", name, code, c.want)
		}
	}
	if calls != 0 || s.Store.Settings().DesktopShortcut != "" {
		t.Errorf("a refused request made the shortcut (%d calls)", calls)
	}
}

func TestStatusOffersShortcutOnThePCOnly(t *testing.T) {
	s := newServer(t)
	s.Shortcut = func() (string, error) { return "", nil }
	set := s.Store.Settings()
	set.PhoneAccess, set.PIN = true, "1234"
	s.Store.SaveSettings(set)

	var st struct {
		Shortcut bool   `json:"shortcut"`
		DataDir  string `json:"data_dir"`
	}
	call(t, s, "GET", "/api/status", nil, &st)
	if !st.Shortcut || st.DataDir != s.Store.Dir {
		t.Errorf("on the PC: %+v", st)
	}
	lanIP(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "192.168.1.20:5000"
	req.Host = "192.168.1.40:8765"
	req.Header.Set("X-PIN", "1234")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	st.Shortcut = true
	json.Unmarshal(w.Body.Bytes(), &st)
	if st.Shortcut {
		t.Error("the phone is offered a shortcut on the PC's desktop")
	}
}

func TestOurs(t *testing.T) {
	ours := httptest.NewServer(newServer(t))
	defer ours.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"app":"something-else"}`))
	}))
	defer other.Close()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	free := l.Addr().String()
	l.Close()

	for addr, want := range map[string]bool{ours.Listener.Addr().String(): true, other.Listener.Addr().String(): false, free: false} {
		if got := Ours(addr); got != want {
			t.Errorf("Ours(%s) = %v, want %v", addr, got, want)
		}
	}
}
