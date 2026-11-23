package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	assets "github.com/kauredo/hallstream/web"
)

func TestWordsScript(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	req := httptest.NewRequest("GET", "/i18n.js", nil)
	req.Host, req.RemoteAddr = "127.0.0.1:8765", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	body := w.Body.String()
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("/i18n.js: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"))
	}
	js, ok := strings.CutPrefix(strings.TrimSpace(body), "window.I18N = ")
	if !ok || !strings.HasSuffix(js, ";") {
		t.Fatalf("/i18n.js does not set window.I18N: %.80q", body)
	}
	var words map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSuffix(js, ";")), &words); err != nil {
		t.Fatal(err)
	}
	if words["overlay.final.titulo"] != "RESULTADO FINAL" {
		t.Errorf("overlay.final.titulo = %q", words["overlay.final.titulo"])
	}

	req = httptest.NewRequest("GET", "/i18n.js", nil) // Host example.com: another site's name
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("/i18n.js with a foreign Host: %d, want 403", w.Code)
	}
}

func TestSettingsLang(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	if got := s.Store.Settings().Lang; got != "pt" {
		t.Errorf("default lang = %q, want pt", got)
	}
	for in, want := range map[string]string{"": "pt", "EN": "en", "../pt": "pt"} {
		set := s.Store.Settings()
		set.Lang = in
		b, _ := json.Marshal(set)
		req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(string(b)))
		req.Host, req.RemoteAddr = "127.0.0.1:8765", "127.0.0.1:5000"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("PUT lang %q: %d %s", in, w.Code, w.Body)
		}
		if got := s.Store.Settings().Lang; got != want {
			t.Errorf("PUT lang %q saved %q, want %q", in, got, want)
		}
	}
	// No en.json ships: the words stay Portuguese.
	req := httptest.NewRequest("POST", "/api/screen", strings.NewReader(`{"screen":"xyz"}`))
	req.Host, req.RemoteAddr = "127.0.0.1:8765", "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "Ecrã desconhecido") {
		t.Errorf("error with lang en and no en.json: %s", w.Body)
	}
}
