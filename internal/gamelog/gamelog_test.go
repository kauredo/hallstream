package gamelog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTimelineLine(t *testing.T) {
	var b bytes.Buffer
	tl := NewTimeline(&b)
	tl.Now = func() time.Time { return time.Date(2026, 11, 15, 18, 0, 1, 250e6, time.FixedZone("", 3600)) }
	tl.Add("screen", map[string]any{"name": "intervalo", "auto": true})
	tl.Add("stream_stop", nil)
	var nilTL *Timeline
	nilTL.Add("box", nil)
	want := `{"ts":"2026-11-15T18:00:01.250+01:00","event":"screen","auto":true,"name":"intervalo"}` + "\n" +
		`{"ts":"2026-11-15T18:00:01.250+01:00","event":"stream_stop"}` + "\n"
	if b.String() != want {
		t.Errorf("got\n%s", b.String())
	}
}

func TestCutRawLog(t *testing.T) {
	raw := strings.Join([]string{
		"2026-11-15T17:29:59.999 aa01", // before the window
		"2026-11-15T17:30:00.000 aa02",
		"garbage line",
		"2026-11-15T18:4x:00.000 aa03", // malformed stamp
		"2026-11-15T19:00:00.000 aa04",
		"2026-11-15T19:00:00.001 aa05", // after
		"",
	}, "\n")
	from := time.Date(2026, 11, 15, 17, 30, 0, 0, time.Local)
	to := time.Date(2026, 11, 15, 19, 0, 0, 0, time.Local)
	var out bytes.Buffer
	if err := Cut(&out, strings.NewReader(raw), from, to, RawStamp); err != nil {
		t.Fatal(err)
	}
	want := "2026-11-15T17:30:00.000 aa02\n2026-11-15T19:00:00.000 aa04\n"
	if out.String() != want {
		t.Errorf("got\n%s", out.String())
	}

	tl := `{"ts":"2026-11-15T17:00:00.000Z","event":"box"}` + "\n" +
		`{"ts":"2026-11-15T17:45:00.000Z","event":"screen"}` + "\n" + "{oops\n"
	out.Reset()
	Cut(&out, strings.NewReader(tl), time.Date(2026, 11, 15, 17, 30, 0, 0, time.UTC), time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC), TimelineStamp)
	if out.String() != `{"ts":"2026-11-15T17:45:00.000Z","event":"screen"}`+"\n" {
		t.Errorf("timeline got\n%s", out.String())
	}
}

// received is one upload as the fake server saw it.
type received struct {
	auth, ua, batch, meta string
	files                 map[string]string // name -> gunzipped content
}

type fakeServer struct {
	mu    sync.Mutex
	code  int
	codes map[string]int // per batch, over code
	got   []received
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := received{auth: r.Header.Get("Authorization"), ua: r.Header.Get("User-Agent"), files: map[string]string{}}
	_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		switch p.FormName() {
		case "batch":
			rec.batch = string(b)
		case "meta":
			rec.meta = string(b)
		case "files[]":
			zr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				rec.files[p.FileName()] = "NOT GZIP"
				continue
			}
			plain, _ := io.ReadAll(zr)
			rec.files[p.FileName()] = string(plain)
		}
	}
	f.mu.Lock()
	f.got = append(f.got, rec)
	code := f.code
	if c, ok := f.codes[rec.batch]; ok {
		code = c
	}
	f.mu.Unlock()
	w.WriteHeader(code)
	if code == 200 {
		w.Write([]byte(`{"ok":true}`))
	} else {
		w.Write([]byte(`{"error":"nope"}`))
	}
}

func newUploader(t *testing.T, token string) (*Uploader, *fakeServer, *[]string) {
	t.Helper()
	dir := t.TempDir()
	f := &fakeServer{code: 200}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	logs := &[]string{}
	u := &Uploader{Dir: dir, Token: token, Log: func(s string) { *logs = append(*logs, s) },
		Post: func(token, ct string, body []byte) (int, []byte, error) {
			req, _ := http.NewRequest("POST", srv.URL+Path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", ct)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return 0, nil, err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, b, nil
		}}
	write := func(name, s string) { os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644) }
	write("raw.log", "2026-11-15T17:00:00.000 01\n2026-11-15T18:00:00.000 02\n")
	write("timeline.jsonl", `{"ts":"2026-11-15T18:00:00.000Z","event":"box","connected":true}`+"\n")
	write("marcador.log", "2026-11-15 18:00:00 Dados em: /home/clube/dados\n")
	write("bluetooth.log", "2026-11-15T17:59:00 AA:BB \"iPhone de Maria\" rssi=-60 services= likely=false\n"+
		"2026-11-15T17:59:01 C4:4F \"Livestream Stramatel\" rssi=-50 services=a002 likely=true\n")
	return u, f, logs
}

var (
	start = time.Date(2026, 11, 15, 18, 0, 0, 0, time.Local)
	end   = start.Add(2 * time.Hour)
)

func pending(t *testing.T, u *Uploader) []string {
	t.Helper()
	return u.pending()
}

func TestUploadSentAndRemoved(t *testing.T) {
	u, f, _ := newUploader(t, "tok-123")
	scrub := func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("/home/clube"), []byte("~")) }
	if err := u.Write("vid1", Meta{Version: "1.2.3", BroadcastID: "vid1", Score: Score{82, 76}}, start, end, scrub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(u.Dir, "envios", "vid1", "raw.log.gz")); err != nil {
		t.Fatal("batch not on disk:", err)
	}
	u.Send()
	if len(f.got) != 1 {
		t.Fatalf("%d uploads", len(f.got))
	}
	g := f.got[0]
	if g.auth != "Bearer tok-123" || g.batch != "vid1" {
		t.Errorf("auth %q batch %q", g.auth, g.batch)
	}
	var m Meta
	if err := json.Unmarshal([]byte(g.meta), &m); err != nil || m.Version != "1.2.3" || m.Score != (Score{82, 76}) || !strings.Contains(g.meta, `"score": {`) {
		t.Errorf("meta %q %v", g.meta, err)
	}
	// raw.log only from 30 minutes before the start: the 17:00 line stays home.
	if g.files["raw.log.gz"] != "2026-11-15T18:00:00.000 02\n" {
		t.Errorf("raw %q", g.files["raw.log.gz"])
	}
	if !strings.Contains(g.files["timeline.jsonl.gz"], `"event":"box"`) {
		t.Errorf("timeline %q", g.files["timeline.jsonl.gz"])
	}
	if g.files["marcador.log.gz"] != "2026-11-15 18:00:00 Dados em: ~/dados\n" {
		t.Errorf("marcador.log %q", g.files["marcador.log.gz"])
	}
	if b := g.files["bluetooth.log.gz"]; strings.Contains(b, "Maria") || !strings.Contains(b, "Stramatel") {
		t.Errorf("bluetooth.log %q", b)
	}
	if _, ok := g.files["marcador.log.1.gz"]; ok {
		t.Error("sent a marcador.log.1 that doesn't exist")
	}
	if p := pending(t, u); len(p) != 0 {
		t.Errorf("still pending %v", p)
	}
	if st := u.Status(); st["pending"] != 0 || st["last_ok"] == "" || st["last_error"] != "" {
		t.Errorf("status %v", st)
	}
}

func TestUploadFailsThenRetried(t *testing.T) {
	u, f, _ := newUploader(t, "tok")
	f.code = 500
	u.Write("local-20261115-180000", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Write("vid2", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Send()
	// A 5xx doesn't stop the round, and both stay.
	if len(f.got) != 2 || len(pending(t, u)) != 2 {
		t.Fatalf("%d uploads, pending %v", len(f.got), pending(t, u))
	}
	if st := u.Status(); st["pending"] != 2 || st["last_error"] == "" {
		t.Errorf("status %v", st)
	}
	// The next start sends both, oldest first.
	f.code = 200
	u.Send()
	if len(f.got) != 4 || f.got[2].batch != f.got[0].batch || f.got[3].batch != "vid2" || len(pending(t, u)) != 0 {
		t.Errorf("uploads %d, pending %v", len(f.got), pending(t, u))
	}
}

func TestRetryPolicy(t *testing.T) {
	for _, code := range []int{401, 404, 429, 400, 413} {
		u, f, _ := newUploader(t, "tok")
		f.code = code
		u.Write("vid", Meta{}, start, end, func(b []byte) []byte { return b })
		u.Send()
		_, err := os.Stat(filepath.Join(u.Dir, "envios", "falhados", "vid"))
		stays := code == 401 || code == 404 || code == 429
		if stays && (len(pending(t, u)) != 1 || err == nil) || !stays && (len(pending(t, u)) != 0 || err != nil) {
			t.Errorf("%d: pending %v, falhados %v", code, pending(t, u), err)
		}
	}
}

func TestFailingBatchDoesNotBlockNewer(t *testing.T) {
	u, f, _ := newUploader(t, "tok")
	f.codes = map[string]int{"old": 401}
	u.Write("old", Meta{}, start, end, func(b []byte) []byte { return b })
	past := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(u.Dir, "envios", "old", "meta.json"), past, past)
	u.Write("new", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Send()
	if len(f.got) != 2 || f.got[0].batch != "old" || f.got[1].batch != "new" {
		t.Fatalf("uploads %+v", f.got)
	}
	if p := pending(t, u); len(p) != 1 || p[0] != "old" {
		t.Errorf("pending %v", p)
	}
}

func TestNetworkErrorEndsRound(t *testing.T) {
	u, _, logs := newUploader(t, "tok")
	calls := 0
	u.Post = func(string, string, []byte) (int, []byte, error) { calls++; return 0, nil, io.ErrUnexpectedEOF }
	u.Write("a1", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Write("b1", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Send()
	u.Send()
	if calls != 2 || len(pending(t, u)) != 2 || len(*logs) != 1 {
		t.Errorf("calls %d, pending %v, logs %q", calls, pending(t, u), *logs)
	}
}

func TestBadBatchNames(t *testing.T) {
	u, _, _ := newUploader(t, "tok")
	for _, name := range []string{"", ".", "..", ".tmp-x", "falhados", "_x", "-x", "a/b", "../x", `a\b`} {
		if err := u.Write(name, Meta{}, start, end, func(b []byte) []byte { return b }); err == nil {
			t.Errorf("%q accepted", name)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(u.Dir, "envios")); len(entries) != 0 {
		t.Errorf("written: %v", entries)
	}
	if n := BatchName("-yt", start); n != "local-20261115-180000" {
		t.Error(n)
	}
}

func TestPrune(t *testing.T) {
	u, _, logs := newUploader(t, "tok")
	failed := filepath.Join(u.Dir, "envios", "falhados")
	for i := range 25 {
		d := filepath.Join(failed, "f"+strconv.Itoa(i))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "meta.json"), []byte("{}"), 0o644)
		at := time.Now().Add(-time.Duration(100-i) * time.Hour)
		os.Chtimes(filepath.Join(d, "meta.json"), at, at)
	}
	u.Write("stale", Meta{}, start, end, func(b []byte) []byte { return b })
	old := time.Now().Add(-61 * 24 * time.Hour)
	os.Chtimes(filepath.Join(u.Dir, "envios", "stale", "meta.json"), old, old)
	u.Post = func(string, string, []byte) (int, []byte, error) {
		t.Error("a stale batch was sent")
		return 200, nil, nil
	}
	u.Send()
	left := batches(failed)
	if len(left) != KeepFailed || left[len(left)-1] != "stale" || left[0] != "f6" {
		t.Errorf("falhados %v", left)
	}
	if len(pending(t, u)) != 0 || len(*logs) != 1 || !strings.Contains((*logs)[0], "60 dias") {
		t.Errorf("pending %v, logs %q", pending(t, u), *logs)
	}
}

func TestScrub(t *testing.T) {
	scrub := NewScrubber(`C:\Users\Joao`, []string{"Ana", "João Pereira", "  ", "Al"})
	in := `Analisar ANA, ana. Joao Pereira e JOÃO PEREIRA; Anaís c:/users/joao/x C:\USERS\JOAO\y joao D:\Users\Outro\z Al`
	want := `Analisar (jogador), (jogador). (jogador) e (jogador); Anaís ~/x ~\y (utilizador) ~\z Al`
	if got := string(scrub([]byte(in))); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := string(NewScrubber("/Users/ana", nil)([]byte("dados: /users/ANA/dados"))); got != "dados: ~/dados" {
		t.Errorf("unix home %q", got)
	}
}

func TestWindowCutLogs(t *testing.T) {
	u, f, _ := newUploader(t, "tok")
	long := strings.Repeat("ab", 50<<10) // a 100 KB line
	write := func(name, s string) { os.WriteFile(filepath.Join(u.Dir, name), []byte(s), 0o644) }
	write("raw.log", "2026-11-15T18:00:00.000 "+long+"\n")
	write("marcador.log", "2026-11-15 17:00:00 antes\n"+
		"2026-11-15 18:00:00 durante\n  continua\n"+
		"2026-11-15 19:00:00 "+long+"\n"+
		"2026-11-15 21:00:00 depois\n  continua depois\n")
	write("marcador.log.1", "2026-11-15 17:29:59 antes\n2026-11-15 17:30:00 inicio\n")
	write("bluetooth.log", "2026-11-15T17:00:00 C4:4F \"Livestream Stramatel\" rssi=-50 services=a002 likely=true\n"+
		"2026-11-15T18:30:00 C4:4F \"Livestream "+long+"\" rssi=-50 services=a002 likely=true\n"+
		"2026-11-15T18:31:00 AA:BB \"iPhone de Maria\" rssi=-60 services= likely=false\n")
	if err := u.Write("vid", Meta{}, start, end, func(b []byte) []byte { return b }); err != nil {
		t.Fatal(err)
	}
	u.Send()
	g := f.got[0]
	if want := "2026-11-15 18:00:00 durante\n  continua\n2026-11-15 19:00:00 " + long + "\n"; g.files["marcador.log.gz"] != want {
		t.Errorf("marcador.log %.200q", g.files["marcador.log.gz"])
	}
	if g.files["marcador.log.1.gz"] != "2026-11-15 17:30:00 inicio\n" {
		t.Errorf("marcador.log.1 %q", g.files["marcador.log.1.gz"])
	}
	if b := g.files["bluetooth.log.gz"]; b != "2026-11-15T18:30:00 C4:4F \"Livestream "+long+"\" rssi=-50 services=a002 likely=true\n" {
		t.Errorf("bluetooth.log %.200q", b)
	}
	if g.files["raw.log.gz"] != "2026-11-15T18:00:00.000 "+long+"\n" {
		t.Errorf("raw.log %.200q", g.files["raw.log.gz"])
	}
}

func TestRawLogTruncatedToFit(t *testing.T) {
	u, f, _ := newUploader(t, "tok")
	defer func(a, b int64) { maxFile, maxBody = a, b }(maxFile, maxBody)
	maxFile = 8 << 10
	var raw strings.Builder
	rnd := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 { // random hex hardly compresses
		fmt.Fprintf(&raw, "%s %016x%016x\n", start.Add(time.Duration(i)*time.Second).Format("2006-01-02T15:04:05.000"), rnd.Uint64(), rnd.Uint64())
	}
	os.WriteFile(filepath.Join(u.Dir, "raw.log"), []byte(raw.String()), 0o644)
	if err := u.Write("vid", Meta{}, start, end, func(b []byte) []byte { return b }); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(u.Dir, "envios", "vid", "raw.log.gz"))
	if fi.Size() > maxFile {
		t.Errorf("raw.log.gz %d bytes", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(u.Dir, "envios", "vid", ".raw.log")); err == nil {
		t.Error("the plain copy stayed in the batch")
	}
	u.Send()
	g := f.got[0]
	lines := strings.Split(strings.TrimSuffix(g.files["raw.log.gz"], "\n"), "\n")
	all := strings.Split(strings.TrimSuffix(raw.String(), "\n"), "\n")
	// The newest lines, whole, up to the last.
	if !strings.Contains(g.meta, `"truncated": true`) || len(lines) < 100 || len(lines) >= len(all) ||
		lines[len(lines)-1] != all[len(all)-1] || lines[0] != all[len(all)-len(lines)] {
		t.Errorf("meta %s, %d of %d lines, first %q", g.meta, len(lines), len(all), lines[0])
	}
	if _, ok := g.files[".raw.log"]; ok {
		t.Error("sent the plain copy")
	}
}

func TestUploadRefusedMovesToFailed(t *testing.T) {
	u, f, logs := newUploader(t, "tok")
	f.code = 400
	u.Write("vid3", Meta{}, start, end, func(b []byte) []byte { return b })
	u.Send()
	if len(pending(t, u)) != 0 {
		t.Error("a refused batch is still pending")
	}
	if _, err := os.Stat(filepath.Join(u.Dir, "envios", "falhados", "vid3", "meta.json")); err != nil {
		t.Error("not in falhados:", err)
	}
	if len(*logs) != 1 || !strings.Contains((*logs)[0], "400: nope") {
		t.Errorf("logs %q", *logs)
	}
	u.Send()
	if len(f.got) != 1 {
		t.Error("a refused batch was sent again")
	}
}

func TestNoTokenNoUpload(t *testing.T) {
	u, f, logs := newUploader(t, "")
	os.MkdirAll(filepath.Join(u.Dir, "envios", "old"), 0o755)
	os.WriteFile(filepath.Join(u.Dir, "envios", "old", "meta.json"), []byte("{}"), 0o644)
	u.Send()
	u.Send()
	if len(f.got) != 0 || len(*logs) != 1 {
		t.Errorf("uploads %d, logs %q", len(f.got), *logs)
	}
}

func TestBatchName(t *testing.T) {
	if n := BatchName("abc_DEF-1", start); n != "abc_DEF-1-20261115-180000" {
		t.Error(n)
	}
	if n := BatchName("", start); n != "local-20261115-180000" {
		t.Error(n)
	}
	if n := BatchName("../x", start); n != "local-20261115-180000" {
		t.Error(n)
	}
}
