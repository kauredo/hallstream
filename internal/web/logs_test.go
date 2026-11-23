package web

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/gamelog"
	"github.com/kauredo/hallstream/internal/store"
)

// logServer is the club's server: it keeps each upload's fields and files.
type logServer struct {
	mu    sync.Mutex
	posts []map[string]string // field or file name -> content (files gunzipped)
}

func (l *logServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != gamelog.Path || r.Header.Get("Authorization") != "Bearer up-token" || r.Header.Get("User-Agent") != "hallstream/test" {
		http.Error(w, `{"error":"no"}`, 401)
		return
	}
	got := map[string]string{}
	_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		if p.FileName() != "" {
			zr, err := gzip.NewReader(bytes.NewReader(b))
			if err == nil {
				b, _ = io.ReadAll(zr)
			}
			got[p.FileName()] = string(b)
			continue
		}
		got[p.FormName()] = string(b)
	}
	l.mu.Lock()
	l.posts = append(l.posts, got)
	l.mu.Unlock()
	w.Write([]byte(`{"ok":true}`))
}

// withUploads points s at a fake club server with a token and a timeline.
func withUploads(t *testing.T, s *Server) *logServer {
	t.Helper()
	l := &logServer{}
	srv := httptest.NewTLSServer(l)
	t.Cleanup(srv.Close)
	s.FPB = fpb.New(func() string { return srv.URL }, s.Store.CacheDir(), "test")
	s.FPB.Every = 0
	s.FPB.HTTP.Transport = srv.Client().Transport
	s.FPB.UploadHost = "127.0.0.1"
	s.UploadToken = "up-token"
	s.newUploads()
	tl, err := gamelog.OpenTimeline(filepath.Join(s.Store.Dir, "timeline.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s.Timeline = tl
	s.State.OnEvent = tl.Add
	s.lastGame = "" // so the next SyncGame puts the game in this timeline
	return l
}

func TestStreamLogsUploaded(t *testing.T) {
	s, _, o := ytSetup(t)
	l := withUploads(t, s)
	// The secrets that must stay on the PC.
	host, port := fakeOBSData(t, "obs-pass-xyz", o.answer)
	s.Store.UpdateSettings(func(set *store.Settings) {
		set.OBS.Host, set.OBS.Port, set.OBS.Password = host, port, "obs-pass-xyz"
		set.PIN, set.BoxAddress = "4821", "C4:4F:33:12:A0:9B"
	})
	g, _ := s.Store.AddGroup("Sub 14 Masculino")
	s.Store.UpdateGroup(g.ID, func(g *store.Group) error {
		g.Players = append(g.Players, store.Player{ID: "p1", DisplayName: "Joana Pereira", Number: "7"})
		return nil
	})
	home, _ := os.UserHomeDir()
	// One frame from long before the stream, one during it.
	os.WriteFile(filepath.Join(s.Store.Dir, "raw.log"), []byte(time.Now().Add(-2*time.Hour).Format("2006-01-02T15:04:05.000")+" 0a0b\n"+
		time.Now().Format("2006-01-02T15:04:05.000")+" 0c0d\n"), 0o644)
	stamp := time.Now().Format("2006-01-02 15:04:05 ")
	os.WriteFile(filepath.Join(s.Store.Dir, "marcador.log"),
		[]byte(stamp+"Dados em: "+filepath.Join(home, "Marcador", "dados")+"\n"+stamp+"Não consegui descarregar a fotografia de JOANA PEREIRA: 404\n"), 0o644)
	s.SyncGame()

	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	s.State.ApplyConsole(map[string]string{"home": "82", "away": "76", "period": "4", "clock": "0.0"})
	// A line stamped by the clock after the stop would fall outside the cut.
	s.Timeline.Now = func() time.Time { return time.Now().Add(time.Hour) }
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)

	if len(l.posts) != 1 {
		t.Fatalf("%d uploads", len(l.posts))
	}
	p := l.posts[0]
	var meta gamelog.Meta
	if err := json.Unmarshal([]byte(p["meta"]), &meta); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p["batch"], "vid1-") || meta.BroadcastID != "vid1" || meta.Escalao != "Sénior Masculino" || meta.Opponent != "Atlético Vale" ||
		meta.Score != (gamelog.Score{Home: 82, Away: 76}) || meta.Title == "" || meta.BoxAddress != "C4:4F:33:12:A0:9B" || meta.Version != "test" || meta.Test || meta.StreamStart == "" {
		t.Errorf("meta %+v", meta)
	}
	if !strings.HasSuffix(p["raw.log.gz"], " 0c0d\n") || strings.Contains(p["raw.log.gz"], "0a0b") {
		t.Errorf("raw.log %q", p["raw.log.gz"])
	}
	for _, ev := range []string{`"event":"stream_start","broadcast_id":"vid1"`, `"event":"stream_stop","broadcast_id":"vid1"`, `"event":"game"`, `"key":"fpb:55102"`} {
		if !strings.Contains(p["timeline.jsonl.gz"], ev) {
			t.Errorf("timeline lacks %s:\n%s", ev, p["timeline.jsonl.gz"])
		}
	}
	// Right after stream_start, the state on air.
	tl := strings.Split(p["timeline.jsonl.gz"], "\n")
	at := -1
	for i, l := range tl {
		if strings.Contains(l, `"event":"stream_start"`) {
			at = i
		}
	}
	if at < 0 || len(tl) < at+5 {
		t.Fatalf("timeline:\n%s", p["timeline.jsonl.gz"])
	}
	for i, ev := range []string{`"event":"game"`, `"event":"screen"`, `"event":"lower_third"`, `"event":"box"`} {
		if l := tl[at+1+i]; !strings.Contains(l, ev) || !strings.Contains(l, `"initial":true`) {
			t.Errorf("line %d after stream_start: %s", i+1, l)
		}
	}
	if !strings.Contains(tl[at+1], `"key":"fpb:55102"`) || !strings.Contains(tl[at+2], `"name":"marcador"`) {
		t.Errorf("initial game and screen:\n%s\n%s", tl[at+1], tl[at+2])
	}
	all := strings.Join([]string{p["meta"], p["batch"], p["raw.log.gz"], p["timeline.jsonl.gz"], p["marcador.log.gz"], p["bluetooth.log.gz"]}, "\n")
	for _, secret := range []string{"4821", "obs-pass-xyz", "segredo", "r1", `"at"`, "key-1", "up-token", "Joana", home} {
		if strings.Contains(all, secret) {
			t.Errorf("%q went to the server:\n%s", secret, all)
		}
	}
	if !strings.Contains(p["marcador.log.gz"], "fotografia de (jogador)") {
		t.Errorf("marcador.log %q", p["marcador.log.gz"])
	}
	if entries, _ := os.ReadDir(filepath.Join(s.Store.Dir, "envios")); len(entries) != 0 {
		t.Errorf("left on disk: %v", entries)
	}
	var st struct {
		Uploads struct {
			Pending int    `json:"pending"`
			LastOK  string `json:"last_ok"`
		} `json:"uploads"`
	}
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Uploads.Pending != 0 || st.Uploads.LastOK == "" {
		t.Errorf("status %+v", st)
	}
}

func TestStreamWithoutYouTubeLocalBatch(t *testing.T) {
	s := newServer(t)
	l := withUploads(t, s)
	withOBS(t, s, &obsFake{typ: "rtmp_common"})
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	if len(l.posts) != 1 || !strings.HasPrefix(l.posts[0]["batch"], "local-") {
		t.Fatalf("%v", l.posts)
	}
}

func TestUploadElsewhereStaysPending(t *testing.T) {
	s := newServer(t)
	l := withUploads(t, s)
	var mu sync.Mutex
	var logs []string
	s.Log = func(m string) { mu.Lock(); logs = append(logs, m); mu.Unlock() }
	s.FPB.UploadHost = "calendariofpb.pt" // the fpb_base_url set points elsewhere
	s.newUploads()
	withOBS(t, s, &obsFake{typ: "rtmp_common"})
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	s.bg.Wait()
	s.uploads.Send()
	n := 0
	for _, m := range logs {
		if strings.Contains(m, "não é o calendariofpb.pt") {
			n++
		}
	}
	if len(l.posts) != 0 || n != 1 || s.uploads.Status()["pending"] != 1 {
		t.Errorf("uploads %d, log lines %d %q, status %v", len(l.posts), n, logs, s.uploads.Status())
	}
}

func TestWindowStart(t *testing.T) {
	obs := time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)
	end := obs.Add(2 * time.Hour)
	at := func(d time.Duration) string { return obs.Add(d).Format(time.RFC3339) }
	for _, c := range []struct {
		startedAt string
		end       time.Time
		want      time.Time
	}{
		{"", end, obs},
		{at(-time.Hour), end, obs.Add(-time.Hour)}, // a reused broadcast
		{at(-maxWindow), obs.Add(time.Minute), obs.Add(-maxWindow + time.Minute)},
		{at(-maxWindow - time.Minute), end, obs},                 // too old to be this game
		{at(time.Minute), end, obs},                              // later than OBS's start
		{"", obs.Add(maxWindow + time.Hour), obs.Add(time.Hour)}, // never more than 6 h
	} {
		if got := windowStart(obs, c.startedAt, c.end); !got.Equal(c.want) {
			t.Errorf("%q to %v: %v, want %v", c.startedAt, c.end, got, c.want)
		}
	}
}

func TestNoTokenNoBatch(t *testing.T) {
	s := newServer(t)
	l := withUploads(t, s)
	var logs []string
	s.Log = func(m string) { logs = append(logs, m) }
	s.UploadToken = ""
	s.newUploads()
	withOBS(t, s, &obsFake{typ: "rtmp_common"})
	for range 2 {
		call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
		call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	}
	n := 0
	for _, m := range logs {
		if strings.Contains(m, "chave de envio") {
			n++
		}
	}
	if len(l.posts) != 0 || n != 1 {
		t.Errorf("uploads %d, log lines %d", len(l.posts), n)
	}
	if _, err := os.Stat(filepath.Join(s.Store.Dir, "envios")); err == nil {
		t.Error("a batch was written")
	}
}
