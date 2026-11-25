package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

func putClub(t *testing.T, s *Server, c map[string]any) (int, store.Settings) {
	t.Helper()
	set := s.Store.Settings()
	b, _ := json.Marshal(set)
	var body map[string]any
	json.Unmarshal(b, &body)
	club := body["club"].(map[string]any)
	for k, v := range c {
		club[k] = v
	}
	code := call(t, s, "PUT", "/api/settings", body, nil)
	return code, s.Store.Settings()
}

func TestClubProfileFeedsTheHomeSide(t *testing.T) {
	s := newServer(t)
	code, set := putClub(t, s, map[string]any{"name": "VALE", "short": "val", "primary": "#f4c430", "secondary": "#0b6e4f"})
	if code != 200 {
		t.Fatalf("PUT %d", code)
	}
	if c := set.Club; c.Short != "VAL" || c.Primary != "#F4C430" || c.Secondary != "#0B6E4F" {
		t.Errorf("saved club %+v", c)
	}
	g := s.State.Snapshot().Game
	if g.Home.Name != "VALE" || g.Home.Short != "VAL" || g.Home.Initials != "VAL" || g.Home.Color != "#F4C430" ||
		g.Home.CrestURL == nil || *g.Home.CrestURL != store.DefaultCrest {
		t.Errorf("home %+v", g.Home)
	}
	if g.Sport != "basketball" || g.SportLabel != "BASKET" {
		t.Errorf("sport %q %q", g.Sport, g.SportLabel)
	}
	if g.Theme["red"] != "#F4C430" || g.Theme["on-red"] == "#FFFFFF" {
		t.Errorf("a gold club gets dark text on its colour: %v", g.Theme)
	}
}

func TestClubProfileRefusesBadValues(t *testing.T) {
	s := newServer(t)
	before := s.Store.Settings().Club
	for _, c := range []map[string]any{
		{"primary": "vermelho"},
		{"secondary": "#12345"},
		{"short": "ABCDE"},
		{"short": " "},
		{"name": ""},
		{"name": "CLUBE DO VALE VERDE"},
		{"sport": "soccer"},
		{"sport": "curling"},
		{"country": "Portugal"},
	} {
		if code, _ := putClub(t, s, c); code != 400 {
			t.Errorf("%v: got %d, want 400", c, code)
		}
	}
	if after := s.Store.Settings().Club; after != before {
		t.Errorf("a refused PUT changed the club: %+v", after)
	}
}

func TestSettingsKeepClubCrest(t *testing.T) {
	s := newServer(t)
	if code, set := putClub(t, s, map[string]any{"crest": "/media/outro.png"}); code != 200 || set.Club.Crest != store.DefaultCrest {
		t.Errorf("PUT /api/settings changed the crest: %d %q", code, set.Club.Crest)
	}
}

func postCrest(t *testing.T, s *Server, ct string) (int, store.Club) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "emblema.png")
	fw.Write(redPNG())
	mw.Close()
	if ct == "" {
		ct = mw.FormDataContentType()
	}
	req := httptest.NewRequest("POST", "/api/club/crest", &body)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var c store.Club
	json.Unmarshal(w.Body.Bytes(), &c)
	return w.Code, c
}

func TestClubCrest(t *testing.T) {
	s := newServer(t)
	media := func(u string) string { return filepath.Join(s.Store.MediaDir(), strings.TrimPrefix(u, "/media/")) }

	code, c := postCrest(t, s, "")
	if code != 200 || !strings.HasPrefix(c.Crest, "/media/club-") || s.Store.Settings().Club.Crest != c.Crest {
		t.Fatalf("upload %d %+v", code, c)
	}
	if _, err := os.Stat(media(c.Crest)); err != nil {
		t.Errorf("crest file: %v", err)
	}
	if h := s.State.Snapshot().Game.Home; h.CrestURL == nil || *h.CrestURL != c.Crest {
		t.Errorf("home crest in the state %v", h.CrestURL)
	}

	first := c.Crest
	if code, c = postCrest(t, s, ""); code != 200 || c.Crest == first {
		t.Fatalf("second upload %d %+v", code, c)
	}
	if _, err := os.Stat(media(first)); !os.IsNotExist(err) {
		t.Errorf("the old crest stayed in dados/media: %v", err)
	}

	var out store.Club
	if code := call(t, s, "DELETE", "/api/club/crest", nil, &out); code != 200 || out.Crest != store.DefaultCrest {
		t.Errorf("DELETE %d %+v", code, out)
	}
	if _, err := os.Stat(media(c.Crest)); !os.IsNotExist(err) {
		t.Errorf("the removed crest stayed in dados/media: %v", err)
	}
	if h := s.State.Snapshot().Game.Home; h.CrestURL == nil || *h.CrestURL != store.DefaultCrest {
		t.Errorf("home crest after DELETE %v", h.CrestURL)
	}

	// Like every upload, a form another page could post is refused.
	if code, _ := postCrest(t, s, "text/plain"); code != 415 {
		t.Errorf("text/plain upload: %d, want 415", code)
	}
}

// overlay.css draws a state without game.theme (an old fixture, the first
// paint) in the default club's colours, so its :root must match them.
func TestOverlayCSSDefaultsAreTheDefaultClub(t *testing.T) {
	css, err := fs.ReadFile(assets.FS, "overlay/overlay.css")
	if err != nil {
		t.Fatal(err)
	}
	root, _, _ := strings.Cut(string(css), "}")
	for k, v := range club.Theme(club.DefaultPrimary, club.DefaultSecondary) {
		if k == "tone" {
			continue // not a colour: overlay.js puts it on <html data-club-tone>, mid by default
		}
		if !strings.Contains(root, "--"+k+": "+v+";") {
			t.Errorf("overlay.css :root lacks --%s: %s;", k, v)
		}
	}
}
