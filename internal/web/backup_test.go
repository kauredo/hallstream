package web

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

func TestBackup(t *testing.T) {
	s := newServer(t)
	s.Store.UpdateSettings(func(set *store.Settings) { set.Instagram = "@clube" })
	dir := s.Store.Dir
	for name, body := range map[string]string{
		"raw.log":         "frames",
		".tmp-123":        "half written",
		"media/logo.png":  "png",
		"promo/ab12.mp4":  "mp4",
		"jogos/jogo.json": "{}",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest("GET", "/api/system/backup", nil)
	req.RemoteAddr, req.Host = "127.0.0.1:5000", "127.0.0.1:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(w.Header().Get("Content-Disposition"), `filename="Hallstream-copia-`) {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	got := strings.Join(names, " ")
	for _, want := range []string{"dados/settings.json", "dados/media/logo.png", "dados/promo/ab12.mp4", "dados/jogos/jogo.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "raw.log") || strings.Contains(got, ".tmp-") {
		t.Errorf("raw.log or a temp file in %s", got)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("another site got %d", w.Code)
	}
}

// The copy holds the OBS password and the YouTube login, so only the PC
// itself can download it.
func TestBackupFromPhone(t *testing.T) {
	s := newServer(t)
	lanIP(t)
	s.Store.UpdateSettings(func(set *store.Settings) { set.PhoneAccess, set.PIN = true, "1234" })
	req := httptest.NewRequest("GET", "/api/system/backup", nil)
	req.RemoteAddr, req.Host = "192.168.1.20:5000", "192.168.1.40:8765"
	req.Header.Set("X-PIN", "1234")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("phone got %d", w.Code)
	}
}
