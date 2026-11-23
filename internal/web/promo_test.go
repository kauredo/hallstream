package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

// fakeMP4 starts like a real MP4 (an ftyp box) and is n bytes long.
func fakeMP4(n int) []byte {
	b := append([]byte{0, 0, 0, 0x18}, "ftypmp42\x00\x00\x00\x00mp42isom"...)
	return append(b, bytes.Repeat([]byte{7}, max(n-len(b), 0))...)
}

func uploadPromo(t *testing.T, s *Server, name string, data []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest("POST", "/api/promo", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Host = "127.0.0.1:8765"
	req.RemoteAddr = "127.0.0.1:5000"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// get sends a GET from the PC itself.
func get(s *Server, path string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1:8765"
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func promoFiles(t *testing.T, s *Server) []string {
	t.Helper()
	es, _ := os.ReadDir(s.Store.PromoDir())
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestPromoUpload(t *testing.T) {
	s := newServer(t)
	code, body := uploadPromo(t, s, `C:\Vídeos\Loja Halloween.MP4`, fakeMP4(5000))
	if code != 201 {
		t.Fatalf("upload %d %s", code, body)
	}
	var c store.Clip
	json.Unmarshal([]byte(body), &c)
	if c.Name != "Loja Halloween" || c.File != c.ID+".mp4" || c.Size != 5000 || c.Line != "Disponível na receção do clube" ||
		c.Link != "" || c.ShowQR || c.End != "back" {
		t.Fatalf("clip %+v", c)
	}
	if got := promoFiles(t, s); len(got) != 1 || got[0] != c.File {
		t.Fatalf("files %v", got)
	}

	for _, bad := range []struct {
		name string
		data []byte
		code int
	}{
		{"clip.mov", fakeMP4(100), 400},
		{"clip.mp4", []byte("<html>not a video</html>"), 400},
		{"clip.mp4", append([]byte{0, 0, 0, 0x14}, "ftypqt  \x00\x00\x00\x00qt  "...), 400},
		{"clip.mp4", nil, 400},
	} {
		if code, body := uploadPromo(t, s, bad.name, bad.data); code != bad.code {
			t.Errorf("%s %q: %d %s", bad.name, bad.data, code, body)
		}
	}

	old := promoMax
	promoMax = 4000
	defer func() { promoMax = old }()
	if code, body := uploadPromo(t, s, "big.mp4", fakeMP4(4001)); code != 413 {
		t.Errorf("too big: %d %s", code, body)
	}
	if code, _ := uploadPromo(t, s, "exact.mp4", fakeMP4(4000)); code != 201 {
		t.Errorf("exactly the limit: %d", code)
	}
	if got := promoFiles(t, s); len(got) != 2 {
		t.Errorf("refused uploads left files: %v", got)
	}

	oldTotal := promoTotal
	promoTotal = 12000 // 5000 + 4000 stored
	defer func() { promoTotal = oldTotal }()
	if code, body := uploadPromo(t, s, "full.mp4", fakeMP4(3001)); code != 400 || len(promoFiles(t, s)) != 2 {
		t.Errorf("over the library cap: %d %s %v", code, body, promoFiles(t, s))
	}
}

func TestPromoTempSweptAtStart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "promo"), 0o755)
	os.WriteFile(filepath.Join(dir, "promo", ".up-123"), []byte("half"), 0o644)
	os.WriteFile(filepath.Join(dir, "promo", "deadbeef.mp4"), []byte("kept"), 0o644)
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	es, _ := os.ReadDir(st.PromoDir())
	if len(es) != 1 || es[0].Name() != "deadbeef.mp4" {
		t.Errorf("promo dir after start: %v", es)
	}
}

func TestPromoUploadGuard(t *testing.T) {
	s := newServer(t)
	req := httptest.NewRequest("POST", "/api/promo", strings.NewReader("x"))
	req.Header.Set("Content-Type", "text/plain")
	req.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 415 {
		t.Errorf("text/plain: %d", w.Code)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "a.mp4")
	fw.Write(fakeMP4(100))
	mw.Close()
	req = httptest.NewRequest("POST", "/api/promo", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "127.0.0.1:8765"
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 403 || len(promoFiles(t, s)) != 0 {
		t.Errorf("other origin: %d %v", w.Code, promoFiles(t, s))
	}
}

func TestPromoServeAndQR(t *testing.T) {
	s := newServer(t)
	_, body := uploadPromo(t, s, "a.mp4", fakeMP4(1000))
	var c store.Clip
	json.Unmarshal([]byte(body), &c)

	w := get(s, "/promo/"+c.File, "Range", "bytes=100-199")
	if w.Code != 206 || w.Body.Len() != 100 || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %d %d %q", w.Code, w.Body.Len(), w.Header().Get("Content-Type"))
	}
	os.WriteFile(filepath.Join(s.Store.PromoDir(), "deadbeef.mp4"), []byte("x"), 0o644)
	req := httptest.NewRequest("GET", "/promo/"+c.File, nil)
	req.Host = "attacker.example"
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("other Host: %d", w.Code)
	}
	for _, p := range []string{"/promo/deadbeef.mp4", "/promo/..%2fsettings.json", "/promo/.up-1"} {
		if w := get(s, p); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}

	var out store.Clip
	if code := call(t, s, "PUT", "/api/promo/"+c.ID, map[string]any{"link": "javascript:alert(1)", "show_qr": true}, nil); code != 400 {
		t.Errorf("javascript link: %d", code)
	}
	call(t, s, "PUT", "/api/promo/"+c.ID, map[string]any{"name": "Loja", "link": "exemplo.pt/loja", "show_qr": true, "volume": 300,
		"orientation": "vertical", "width": 480, "height": 848, "duration": 15.3, "end": "loop", "size": 1, "file": "x"}, &out)
	if out.Link != "https://exemplo.pt/loja" || !out.ShowQR || out.Volume != 100 || out.End != "loop" ||
		out.Orientation != "vertical" || out.File != c.File || out.Size != 1000 {
		t.Fatalf("put %+v", out)
	}
	w = get(s, "/promo/"+c.ID+"/qr.svg")
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<svg") || !strings.Contains(w.Body.String(), "h1v1h-1z") {
		t.Fatalf("qr: %d %.80s", w.Code, w.Body.String())
	}
	call(t, s, "PUT", "/api/promo/"+c.ID, map[string]any{"link": "", "show_qr": true}, &out)
	if out.ShowQR {
		t.Error("QR on without a link")
	}
}

func TestPromoShowEndDelete(t *testing.T) {
	s := newServer(t)
	_, body := uploadPromo(t, s, "a.mp4", fakeMP4(1000))
	var c store.Clip
	json.Unmarshal([]byte(body), &c)
	call(t, s, "PUT", "/api/promo/"+c.ID, map[string]any{"orientation": "vertical", "link": "https://exemplo.pt", "show_qr": true}, nil)

	call(t, s, "POST", "/api/screen", map[string]any{"screen": "desconto"}, nil)
	if code := call(t, s, "PUT", "/api/promo", map[string]any{"timeout_clip": c.ID}, nil); code != 200 {
		t.Fatalf("timeout clip: %d", code)
	}
	if code := call(t, s, "POST", "/api/promo/"+c.ID+"/show", nil, nil); code != 200 {
		t.Fatalf("show: %d", code)
	}
	st := s.State.Snapshot()
	if st.Screen.Current != "promo" || st.Promo.Clip == nil || st.Promo.Clip.URL != "/promo/"+c.File ||
		!strings.HasPrefix(st.Promo.Clip.QRURL, "/promo/"+c.ID+"/qr.svg?v=") || st.Promo.Mini == nil {
		t.Fatalf("state %+v %+v", st.Screen, st.Promo)
	}
	call(t, s, "POST", "/api/promo/ended", map[string]any{"seq": st.Promo.Seq + 1}, nil)
	if s.State.Snapshot().Screen.Current != "promo" {
		t.Fatal("a stale end stopped the clip")
	}
	call(t, s, "POST", "/api/promo/ended", map[string]any{"seq": st.Promo.Seq}, nil)
	if cur := s.State.Snapshot().Screen.Current; cur != "desconto" {
		t.Fatalf("after end: %q", cur)
	}

	call(t, s, "POST", "/api/promo/"+c.ID+"/show", nil, nil)
	call(t, s, "DELETE", "/api/promo/"+c.ID, nil, nil)
	st = s.State.Snapshot()
	if st.Screen.Current != "desconto" || st.Promo.Mini != nil || len(promoFiles(t, s)) != 0 || len(s.Store.Promo().Clips) != 0 {
		t.Fatalf("after delete: %+v %+v %v", st.Screen, st.Promo, promoFiles(t, s))
	}
	if code := call(t, s, "POST", "/api/promo/"+c.ID+"/show", nil, nil); code != 404 {
		t.Errorf("show deleted: %d", code)
	}
}

func TestPromoTimeoutNeedsVertical(t *testing.T) {
	s := newServer(t)
	_, body := uploadPromo(t, s, "a.mp4", fakeMP4(1000))
	var c store.Clip
	json.Unmarshal([]byte(body), &c)
	if code := call(t, s, "PUT", "/api/promo/"+c.ID, map[string]any{"orientation": "horizontal"}, nil); code != 200 {
		t.Fatalf("put: %d", code)
	}
	if code := call(t, s, "PUT", "/api/promo", map[string]any{"timeout_clip": c.ID}, nil); code != 400 {
		t.Errorf("horizontal on desconto: %d", code)
	}
}

func TestIsMP4(t *testing.T) {
	for _, tc := range []struct {
		b    []byte
		want bool
	}{
		{fakeMP4(64), true},
		{append([]byte{0, 0, 0, 0x14}, "ftypXXXX\x00\x00\x00\x00isom"...), true},
		{append([]byte{0, 0, 0, 0x14}, "ftypqt  \x00\x00\x00\x00qt  "...), false},
		{append([]byte{0, 0, 0, 0x08}, "ftypmp42"...), false},
		{[]byte("RIFF....AVI LIST"), false},
	} {
		if got := isMP4(tc.b); got != tc.want {
			t.Errorf("isMP4(%q) = %v", tc.b, got)
		}
	}
}
