package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/youtube"
)

// fakeYT is Google: the token endpoint and the YouTube Data API.
type fakeYT struct {
	mu          sync.Mutex
	calls       map[string]int
	refreshFail bool
	verified    bool
	status      map[string]string // broadcast id → lifeCycleStatus
	privacy     map[string]string
	finishIn    int // lookups of a live broadcast before it turns complete
	updateFail  int // videos.update calls that fail
	streams     []map[string]any
	completed   []map[string]any
	inserted    map[string]any // the last liveBroadcasts.insert body
	updated     map[string]any // the last videos.update body
	bound       map[string]string
	next        int
	longUploads string // channels.list status.longUploadsStatus
	liveOff     bool   // liveBroadcasts.list answers liveStreamingNotEnabled
}

func (f *fakeYT) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

func (f *fakeYT) set(fn func(*fakeYT)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeYT) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/youtube/v3"), "/upload/youtube/v3")
	f.calls[r.Method+" "+path]++
	w.Header().Set("Content-Type", "application/json")
	send := func(v any) { json.NewEncoder(w).Encode(v) }
	apiErr := func(code int, reason string) {
		w.WriteHeader(code)
		send(map[string]any{"error": map[string]any{"code": code, "message": reason, "errors": []any{map[string]any{"reason": reason}}}})
	}
	if path == "/token" {
		if f.refreshFail {
			w.WriteHeader(400)
			send(map[string]string{"error": "invalid_grant"})
			return
		}
		send(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600})
		return
	}
	if path == "/revoke" {
		return
	}
	if r.Header.Get("Authorization") != "Bearer at" {
		apiErr(401, "authError")
		return
	}
	q := r.URL.Query()
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	item := func(id string) map[string]any {
		return map[string]any{"id": id, "snippet": map[string]any{"title": "Jogo " + id, "actualStartTime": "2026-11-15T18:01:00Z"},
			"status":         map[string]any{"lifeCycleStatus": f.status[id], "privacyStatus": f.privacy[id]},
			"contentDetails": map[string]any{"boundStreamId": f.bound[id]}}
	}
	switch r.Method + " " + path {
	case "GET /channels":
		send(map[string]any{"items": []any{map[string]any{"id": "UC1", "snippet": map[string]any{"title": "Clube Exemplo Basket",
			"thumbnails": map[string]any{"default": map[string]any{"url": "https://yt3.example/a.jpg"}}},
			"status": map[string]any{"longUploadsStatus": f.longUploads}}}})
	case "GET /liveStreams":
		send(map[string]any{"items": f.streams})
	case "POST /liveStreams":
		s := map[string]any{"id": "s1", "snippet": body["snippet"], "contentDetails": map[string]any{"isReusable": true},
			"cdn": map[string]any{"ingestionType": "rtmp", "ingestionInfo": map[string]any{"streamName": "key-1", "ingestionAddress": "rtmp://a.rtmp.youtube.com/live2"}}}
		f.streams = append(f.streams, s)
		send(s)
	case "POST /liveBroadcasts":
		f.next++
		id := fmt.Sprintf("vid%d", f.next)
		f.inserted, f.status[id], f.privacy[id] = body, "ready", body["status"].(map[string]any)["privacyStatus"].(string)
		send(item(id))
	case "POST /liveBroadcasts/bind":
		f.bound[q.Get("id")] = q.Get("streamId")
		send(item(q.Get("id")))
	case "GET /liveBroadcasts":
		if f.liveOff {
			apiErr(403, "liveStreamingNotEnabled")
			return
		}
		if q.Get("broadcastStatus") == "completed" {
			send(map[string]any{"items": f.completed})
			return
		}
		var items []any
		for _, id := range strings.Split(q.Get("id"), ",") {
			if f.status[id] == "live" && f.finishIn > 0 {
				if f.finishIn--; f.finishIn == 0 {
					f.status[id] = "complete"
				}
			}
			if _, ok := f.status[id]; ok {
				items = append(items, item(id))
			}
		}
		send(map[string]any{"items": items})
	case "POST /liveBroadcasts/transition":
		f.status[q.Get("id")] = q.Get("broadcastStatus")
		send(item(q.Get("id")))
	case "GET /videos":
		id := q.Get("id")
		send(map[string]any{"items": []any{map[string]any{"id": id, "status": map[string]any{"privacyStatus": f.privacy[id],
			"embeddable": false, "license": "creativeCommon", "publicStatsViewable": false, "madeForKids": false, "uploadStatus": "processed"}}}})
	case "PUT /videos":
		if f.updateFail > 0 {
			f.updateFail--
			apiErr(500, "backendError")
			return
		}
		f.updated = body
		st := body["status"].(map[string]any)
		f.privacy[body["id"].(string)] = st["privacyStatus"].(string)
		send(body)
	case "POST /thumbnails/set":
		if !f.verified {
			apiErr(403, "forbidden")
			return
		}
		send(map[string]any{"items": []any{}})
	default:
		apiErr(404, "notFound")
	}
}

// obsFake is OBS as the stream calls see it.
type obsFake struct {
	mu      sync.Mutex
	live    bool
	typ     string
	server  string
	key     string
	starts  int
	setSvc  int
	stopped int
}

func (o *obsFake) answer(typ string, data json.RawMessage) (bool, int, any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	switch typ {
	case "GetStreamServiceSettings":
		return true, 100, map[string]any{"streamServiceType": o.typ, "streamServiceSettings": map[string]any{"server": o.server, "key": o.key}}
	case "SetStreamServiceSettings":
		var d struct {
			Type     string                       `json:"streamServiceType"`
			Settings struct{ Server, Key string } `json:"streamServiceSettings"`
		}
		json.Unmarshal(data, &d)
		o.typ, o.server, o.key = d.Type, d.Settings.Server, d.Settings.Key
		o.setSvc++
	case "StartStream":
		o.live = true
		o.starts++
	case "StopStream":
		o.live = false
		o.stopped++
	case "GetStreamStatus":
		return true, 100, map[string]any{"outputActive": o.live, "outputDuration": 1000}
	}
	return true, 100, nil
}

func newFakeYT(t *testing.T) *fakeYT {
	f := &fakeYT{calls: map[string]int{}, status: map[string]string{}, privacy: map[string]string{}, bound: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api, up, google, revoke := youtube.APIBase, youtube.UploadBase, youtube.Google, youtube.RevokeURL
	youtube.APIBase, youtube.UploadBase = srv.URL+"/youtube/v3", srv.URL+"/upload/youtube/v3"
	youtube.Google.TokenURL, youtube.RevokeURL = srv.URL+"/token", srv.URL+"/revoke"
	t.Cleanup(func() {
		youtube.APIBase, youtube.UploadBase, youtube.Google, youtube.RevokeURL = api, up, google, revoke
	})
	return f
}

// connect gives s a saved login whose access token has run out.
func connect(s *Server) {
	s.ytMu.Lock()
	s.yt = ytSaved{ClientID: "id.apps.googleusercontent.com", ClientSecret: "segredo",
		Token: &oauth2.Token{AccessToken: "old", RefreshToken: "r1", TokenType: "Bearer", Expiry: time.Now().Add(-time.Minute)}}
	s.ytWrite()
	s.ytMu.Unlock()
}

func withOBS(t *testing.T, s *Server, o *obsFake) {
	host, port := fakeOBSData(t, "", o.answer)
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port, set.OBS.Enabled = host, port, true })
}

func ytGame(t *testing.T, s *Server) {
	t.Helper()
	gid := ""
	for _, g := range s.Store.Groups() {
		if g.Name == "Sénior Masculino" {
			gid = g.ID
		}
	}
	if gid == "" {
		g, err := s.Store.AddGroup("Sénior Masculino")
		if err != nil {
			t.Fatal(err)
		}
		gid = g.ID
	}
	id := int64(55102)
	if err := s.Store.SaveGame(store.Game{GroupID: gid, FPBGameID: &id, Start: "2026-11-15T18:00:00Z", Away: store.Opponent{Name: "ATLÉTICO VALE", Short: "LOU"},
		Competition: "2ª Divisão Masculina", Round: "Jornada 6", Venue: "Pavilhão Municipal do Vale", Squad: []store.SquadEntry{}}); err != nil {
		t.Fatal(err)
	}
	s.SyncGame()
}

type streamReply struct {
	Stream  *obsws.Stream `json:"stream"`
	YouTube *struct {
		Live    *ytRecord `json:"live"`
		Ended   *ytRecord `json:"ended"`
		Expired bool      `json:"expired"`
	} `json:"youtube"`
	Error string `json:"error"`
}

func ytSetup(t *testing.T) (*Server, *fakeYT, *obsFake) {
	t.Helper()
	f := newFakeYT(t)
	s := newServer(t)
	s.ytSleep = func(time.Duration) {}
	o := &obsFake{typ: "rtmp_common"}
	withOBS(t, s, o)
	connect(s)
	ytGame(t, s)
	return s, f, o
}

func TestYouTubeStartBindsAndStarts(t *testing.T) {
	s, f, o := ytSetup(t)
	var r streamReply
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &r); code != 200 || r.YouTube == nil || r.YouTube.Live == nil {
		t.Fatalf("%d %+v", code, r)
	}
	live := r.YouTube.Live
	if live.Link != "https://youtu.be/vid1" || live.Escalao != "Sénior Masculino" || live.Opponent != "Atlético Vale" || live.GameKey != "fpb:55102" {
		t.Errorf("record %+v", live)
	}
	if o.typ != "rtmp_custom" || o.server != "rtmp://a.rtmp.youtube.com/live2" || o.key != "key-1" || o.starts != 1 {
		t.Errorf("OBS %+v", o)
	}
	if f.bound["vid1"] != "s1" || f.count("POST /liveStreams") != 1 || f.count("POST /liveBroadcasts") != 1 {
		t.Errorf("calls %v bound %v", f.calls, f.bound)
	}
	snip, st, cd := f.inserted["snippet"].(map[string]any), f.inserted["status"].(map[string]any), f.inserted["contentDetails"].(map[string]any)
	if snip["title"] != "Casa vs Atlético Vale · Sénior Masculino · 2ª Divisão Masculina J6" || snip["scheduledStartTime"] == nil ||
		st["privacyStatus"] != "public" || st["selfDeclaredMadeForKids"] != false || cd["enableAutoStart"] != true || cd["enableAutoStop"] != true {
		t.Errorf("insert %v", f.inserted)
	}
	if d := snip["description"].(string); !strings.Contains(d, "Pavilhão Municipal do Vale") || !strings.Contains(d, "Domingo, 15 de novembro de 2026, 18:00") {
		t.Errorf("description %q", d)
	}
	// The access token was refreshed and saved, 0600 where the system has it.
	if y := s.ytGet(); y.Token.AccessToken != "at" || y.Token.RefreshToken != "r1" {
		t.Errorf("token %+v", y.Token)
	}
	if fi, err := os.Stat(filepath.Join(s.Store.Dir, ytFile)); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("youtube.json %v %v", fi.Mode(), err)
	}
}

func TestYouTubeTestStreamStaysPrivate(t *testing.T) {
	s, f, o := ytSetup(t)
	var r streamReply
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{"test": true}, &r); code != 200 || r.YouTube.Live == nil {
		t.Fatalf("%d %+v", code, r)
	}
	live := r.YouTube.Live
	snip, st := f.inserted["snippet"].(map[string]any), f.inserted["status"].(map[string]any)
	if st["privacyStatus"] != "private" || !strings.HasPrefix(snip["title"].(string), "Teste · Casa vs Atlético Vale") {
		t.Errorf("insert %v", f.inserted)
	}
	if !live.Test || live.Privacy != "private" || live.GameKey != "teste:fpb:55102" || o.starts != 1 {
		t.Errorf("record %+v, OBS starts %d", live, o.starts)
	}
	f.set(func(f *fakeYT) { f.status["vid1"] = "complete" })
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, &r)
	s.bg.Wait()
	if f.count("PUT /videos") != 0 || r.YouTube.Ended == nil || r.YouTube.Ended.Privacy != "private" {
		t.Errorf("after stop: calls %v ended %+v", f.calls, r.YouTube.Ended)
	}
	if code := call(t, s, "POST", "/api/youtube/unlist", map[string]string{"broadcast_id": "vid1"}, nil); code != 409 || f.count("PUT /videos") != 0 {
		t.Errorf("unlist a test: %d", code)
	}
	// The real start afterwards makes the public broadcast, not the test's.
	f.set(func(f *fakeYT) { f.status["vid1"] = "ready" })
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &r)
	if r.YouTube.Live.BroadcastID != "vid2" || r.YouTube.Live.Privacy != "public" || r.YouTube.Live.Test {
		t.Errorf("real start %+v", r.YouTube.Live)
	}
	// No test while the game is on air: the game's record would lose OnAir.
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{"test": true}, nil); code != 409 || !s.ytRecord("vid2").OnAir {
		t.Errorf("test over the game: %d %+v", code, s.ytRecord("vid2"))
	}
}

func TestYouTubeTestStreamNeedsYouTube(t *testing.T) {
	s, _, o := ytSetup(t)
	s.ytMu.Lock()
	s.yt.Token = nil // not connected: OBS would send to the public stream key
	s.ytMu.Unlock()
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{"test": true}, nil); code != 409 || o.starts != 0 {
		t.Errorf("%d, OBS starts %d", code, o.starts)
	}
}

func TestYouTubeReuseOnRestart(t *testing.T) {
	s, f, o := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	f.set(func(f *fakeYT) { f.status["vid1"] = "live" })
	// The program closes mid-game and opens again on the same dados.
	st, _ := store.Open(s.Store.Dir)
	s2 := New(&Server{Store: st, State: state.New(time.Now), FPB: fpb.New(func() string { return "" }, st.CacheDir(), "test"),
		OBS: &obsws.Client{}, Files: s.Files, Version: "test", Port: 8765})
	s2.FPB.Every = 0
	s2.ytSleep = func(time.Duration) {}
	ytGame(t, s2)
	var r streamReply
	if code := call(t, s2, "POST", "/api/obs/stream/start", map[string]any{}, &r); code != 200 || r.YouTube.Live == nil || r.YouTube.Live.BroadcastID != "vid1" {
		t.Fatalf("%d %+v", code, r)
	}
	if n := f.count("POST /liveBroadcasts"); n != 1 {
		t.Errorf("made %d broadcasts", n)
	}
	if o.starts != 2 {
		t.Errorf("OBS starts %d", o.starts)
	}
	// A broadcast that ended is not reused.
	f.set(func(f *fakeYT) { f.status["vid1"] = "complete" })
	call(t, s2, "POST", "/api/obs/stream/start", map[string]any{}, &r)
	if f.count("POST /liveBroadcasts") != 2 || r.YouTube.Live.BroadcastID != "vid2" {
		t.Errorf("after complete: %d %+v", f.count("POST /liveBroadcasts"), r.YouTube.Live)
	}
}

func thumbnail(t *testing.T, s *Server, id string) (int, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("broadcast_id", id)
	fw, _ := mw.CreateFormFile("file", "miniatura.png")
	fw.Write(redPNG())
	mw.Close()
	req := httptest.NewRequest("POST", "/api/youtube/thumbnail", &b)
	req.RemoteAddr, req.Host = "127.0.0.1:5000", "127.0.0.1:8765"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestYouTubeThumbnail(t *testing.T) {
	s, f, o := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	code, body := thumbnail(t, s, "vid1")
	if code != 502 || !strings.Contains(body, "verificado") {
		t.Errorf("unverified: %d %s", code, body)
	}
	if r := s.ytRecord("vid1"); r.ThumbnailSet || r.ThumbnailError == "" || !o.live {
		t.Errorf("record %+v, live %v", r, o.live)
	}
	f.set(func(f *fakeYT) { f.verified = true })
	if code, body := thumbnail(t, s, "vid1"); code != 200 || !s.ytRecord("vid1").ThumbnailSet {
		t.Errorf("verified: %d %s", code, body)
	}
	if code, _ := thumbnail(t, s, "not-ours"); code != 404 {
		t.Errorf("other video: %d", code)
	}
}

func TestYouTubeEndUnlisted(t *testing.T) {
	s, f, _ := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	f.set(func(f *fakeYT) { f.status["vid1"], f.finishIn = "live", 2 })
	s.State.ApplyConsole(map[string]string{"home": "82", "away": "76"})
	var r streamReply
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, &r)
	call(t, s, "GET", "/api/obs/stream", nil, &r)
	e := r.YouTube.Ended
	if e == nil || e.Privacy != "unlisted" || e.Result != "Casa 82-76 Atlético Vale" || e.PrivacyError != "" || r.YouTube.Live != nil {
		t.Fatalf("%+v", e)
	}
	st := f.updated["status"].(map[string]any)
	if st["privacyStatus"] != "unlisted" || st["license"] != "creativeCommon" || st["embeddable"] != false || st["publicStatsViewable"] != false {
		t.Errorf("videos.update %v", f.updated)
	}
	if n := f.count("GET /liveBroadcasts"); n > ytMaxPolls {
		t.Errorf("%d lookups", n)
	}
}

func TestYouTubeStaysLiveIsTransitioned(t *testing.T) {
	s, f, _ := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	f.set(func(f *fakeYT) { f.status["vid1"] = "live" })
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	if f.count("POST /liveBroadcasts/transition") != 1 || s.ytRecord("vid1").Privacy != "unlisted" {
		t.Errorf("calls %v record %+v", f.calls, s.ytRecord("vid1"))
	}
	if n := f.count("GET /liveBroadcasts"); n > ytMaxPolls {
		t.Errorf("%d lookups", n)
	}
}

func TestYouTubeUnlistedFailsThenRetry(t *testing.T) {
	s, f, _ := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	f.set(func(f *fakeYT) { f.status["vid1"], f.finishIn, f.updateFail = "live", 1, 1 })
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	if r := s.ytRecord("vid1"); r.Privacy != "public" || !strings.Contains(r.PrivacyError, "não listado") {
		t.Fatalf("%+v", r)
	}
	var resp map[string]any
	if code := call(t, s, "POST", "/api/youtube/unlist", map[string]string{"broadcast_id": "vid1"}, &resp); code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	if r := s.ytRecord("vid1"); r.Privacy != "unlisted" || r.PrivacyError != "" {
		t.Errorf("%+v", r)
	}
}

func TestYouTubeHistory(t *testing.T) {
	s, f, _ := ytSetup(t)
	s.ytMu.Lock()
	s.ytRecs = []ytRecord{
		{BroadcastID: "old", Title: "Casa vs Lagoa Seca", Escalao: "Sub 16 Masculino", Date: "2026-03-01T17:00:00Z", Privacy: "unlisted"},
		{BroadcastID: "new", Title: "Casa vs Atlético Vale", Escalao: "Sénior Masculino", Date: "2026-11-15T18:00:00Z", Privacy: "public"},
	}
	s.ytMu.Unlock()
	f.set(func(f *fakeYT) {
		f.status["new"], f.privacy["new"] = "complete", "unlisted"
		f.status["ext"], f.privacy["ext"] = "complete", "public"
		item := func(id, start string) map[string]any {
			return map[string]any{"id": id, "snippet": map[string]any{"title": "Treino " + id, "actualStartTime": start},
				"status": map[string]any{"lifeCycleStatus": "complete", "privacyStatus": f.privacy[id]}}
		}
		f.completed = []map[string]any{item("new", "2026-11-15T18:01:00Z"), item("ext", "2026-10-01T10:00:00Z")}
	})
	var h struct {
		Items    []ytRecord `json:"items"`
		Escaloes []string   `json:"escaloes"`
		Seasons  []string   `json:"seasons"`
	}
	call(t, s, "GET", "/api/youtube/history", nil, &h)
	ids := []string{}
	for _, it := range h.Items {
		ids = append(ids, it.BroadcastID)
	}
	if strings.Join(ids, ",") != "new,ext,old" || h.Items[0].Privacy != "unlisted" || !h.Items[1].External || h.Items[0].External {
		t.Fatalf("%v %+v", ids, h.Items)
	}
	if strings.Join(h.Seasons, ",") != "2026/27,2025/26" || strings.Join(h.Escaloes, ",") != "Fora do Hallstream,Sub 16 Masculino,Sénior Masculino" {
		t.Errorf("filters %v %v", h.Seasons, h.Escaloes)
	}
	call(t, s, "GET", "/api/youtube/history?escalao=Fora+do+Hallstream", nil, &h)
	if len(h.Items) != 1 || h.Items[0].BroadcastID != "ext" {
		t.Errorf("escalão %+v", h.Items)
	}
	call(t, s, "GET", "/api/youtube/history?season=2025%2F26", nil, &h)
	if len(h.Items) != 1 || h.Items[0].BroadcastID != "old" {
		t.Errorf("season %+v", h.Items)
	}
	if n := f.count("GET /liveBroadcasts"); n != 1 {
		t.Errorf("read the channel %d times, cached for 10 min", n)
	}
}

func TestYouTubeRefreshFails(t *testing.T) {
	s, f, o := ytSetup(t)
	f.set(func(f *fakeYT) { f.refreshFail = true })
	var r streamReply
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &r); code != 409 || r.Error != youtube.ErrReconnect.Error() || o.starts != 0 {
		t.Fatalf("%d %+v starts %d", code, r, o.starts)
	}
	var st map[string]any
	call(t, s, "GET", "/api/youtube", nil, &st)
	if st["expired"] != true || st["connected"] != true {
		t.Errorf("%v", st)
	}
}

func TestNoYouTubeStreamAsBefore(t *testing.T) {
	f := newFakeYT(t)
	s := newServer(t)
	o := &obsFake{typ: "rtmp_common"}
	withOBS(t, s, o)
	var r streamReply
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, &r); code != 200 || r.YouTube != nil || o.starts != 1 || o.setSvc != 0 {
		t.Fatalf("%d %+v %+v", code, r, o)
	}
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, &r)
	if len(f.calls) != 0 || o.typ != "rtmp_common" {
		t.Errorf("calls %v, OBS %+v", f.calls, o)
	}
}

func TestYouTubeSecretsStayOnPC(t *testing.T) {
	s, _, _ := ytSetup(t)
	lanIP(t)
	s.Store.UpdateSettings(func(set *store.Settings) { set.PhoneAccess, set.PIN = true, "1234" })
	phone := func(method, path string, body any) (int, string) {
		var b bytes.Buffer
		json.NewEncoder(&b).Encode(body)
		req := httptest.NewRequest(method, path, &b)
		req.RemoteAddr, req.Host = "192.168.1.20:5000", "192.168.1.40:8765"
		req.Header.Set("X-PIN", "1234")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	for _, path := range []string{"/api/youtube", "/api/status", "/api/obs/stream", "/api/youtube/history", "/api/settings"} {
		code, body := phone("GET", path, nil)
		if code != 200 || strings.Contains(body, "segredo") || strings.Contains(body, "r1") || strings.Contains(body, `"at"`) ||
			strings.Contains(body, "apps.googleusercontent") || strings.Contains(body, "key-1") {
			t.Errorf("%s from a phone: %d %s", path, code, body)
		}
	}
	for _, path := range []string{"/api/youtube/connect", "/api/youtube/disconnect"} {
		if code, _ := phone("POST", path, map[string]any{}); code != 403 {
			t.Errorf("%s from a phone: %d", path, code)
		}
	}
	if code, _ := phone("PUT", "/api/youtube/client", map[string]string{"client_id": "x.apps.googleusercontent.com", "client_secret": "y"}); code != 403 {
		t.Errorf("client from a phone: %d", code)
	}
	// On the PC: the id, never the secret.
	var st map[string]any
	call(t, s, "GET", "/api/youtube", nil, &st)
	if st["client_id"] != "id.apps.googleusercontent.com" || st["has_secret"] != true || st["client_secret"] != nil {
		t.Errorf("%v", st)
	}
}

func TestYouTubeClientAndDisconnect(t *testing.T) {
	f := newFakeYT(t)
	s := newServer(t)
	var st map[string]any
	if code := call(t, s, "PUT", "/api/youtube/client", map[string]string{"client_id": "nope", "client_secret": "x"}, &st); code != 400 {
		t.Errorf("bad id: %d", code)
	}
	call(t, s, "PUT", "/api/youtube/client", map[string]string{"client_id": " id.apps.googleusercontent.com ", "client_secret": "segredo"}, &st)
	if st["configured"] != true {
		t.Fatalf("%v", st)
	}
	// Blank secret keeps the saved one.
	call(t, s, "PUT", "/api/youtube/client", map[string]string{"client_id": "id.apps.googleusercontent.com"}, &st)
	if s.ytGet().ClientSecret != "segredo" {
		t.Error("lost the secret")
	}
	var c map[string]string
	if code := call(t, s, "POST", "/api/youtube/connect", map[string]any{}, &c); code != 200 || !strings.Contains(c["auth_url"], "code_challenge=") {
		t.Fatalf("%d %v", code, c)
	}
	connect(s)
	call(t, s, "POST", "/api/youtube/disconnect", map[string]any{}, &st)
	if st["connected"] != false || f.count("POST /revoke") != 1 || s.ytGet().Token != nil || s.ytGet().ClientID == "" {
		t.Errorf("%v revoke %d", st, f.count("POST /revoke"))
	}
	b, _ := os.ReadFile(filepath.Join(s.Store.Dir, ytFile))
	if strings.Contains(string(b), "r1") {
		t.Error("token left in youtube.json")
	}
}

func TestBroadcastTitle(t *testing.T) {
	away := false
	g := state.Game{Group: &state.Group{Name: "Sub 16 Masculino"}, Home: state.Team{Name: "CASA"}, Away: state.Team{Name: "GDP"},
		AtHome: &away, Competition: "Campeonato Distrital <Lisboa> " + strings.Repeat("muito longo ", 10), Round: "Jornada 3"}
	title, _ := broadcastText(g, time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC))
	if title != "GDP vs Casa · Sub 16 Masculino" {
		t.Errorf("%q", title)
	}
	g.Competition = "Taça <A>"
	if title, _ = broadcastText(g, time.Now()); strings.ContainsAny(title, "<>") || len([]rune(title)) > 100 {
		t.Errorf("%q", title)
	}
}

// Connect and Desligar while logins time out must never lock up (a Cancel
// under ytMu used to wait for a login finishing into ytLoggedIn).
func TestYouTubeConnectRaceNoDeadlock(t *testing.T) {
	newFakeYT(t)
	s := newServer(t)
	call(t, s, "PUT", "/api/youtube/client", map[string]string{"client_id": "id.apps.googleusercontent.com", "client_secret": "segredo"}, nil)
	old := ytLoginWait
	ytLoginWait = 300 * time.Microsecond
	t.Cleanup(func() { ytLoginWait = old })
	done := make(chan struct{})
	go func() {
		// Each login times out about when the next connect cancels it.
		for i := 0; i < 1500; i++ {
			call(t, s, "POST", "/api/youtube/connect", map[string]any{}, nil)
			time.Sleep(time.Duration(i%7) * 50 * time.Microsecond)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock")
	}
}

func TestYouTubeDisconnectDuringPoll(t *testing.T) {
	s, f, _ := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	f.set(func(f *fakeYT) { f.status["vid1"] = "live" })
	polls := 0
	s.ytSleep = func(time.Duration) {
		if polls++; polls == 2 {
			s.ytMu.Lock()
			s.yt.Token = nil
			s.ytMu.Unlock()
		}
	}
	call(t, s, "POST", "/api/obs/stream/stop", map[string]any{}, nil)
	if r := s.ytRecord("vid1"); r.Privacy == "unlisted" || !strings.Contains(r.PrivacyError, "desligado") {
		t.Errorf("%+v", r)
	}
	// A client with no login fails its calls instead of panicking.
	if _, err := s.ytClient().Broadcasts(context.Background(), "vid1"); err == nil {
		t.Error("no error without a login")
	}
}

func TestYouTubeStartFailsNotOnAir(t *testing.T) {
	s, _, o := ytSetup(t)
	o.mu.Lock()
	o.typ, o.server, o.key = "rtmp_custom", "rtmp://a.rtmp.youtube.com/live2", "key-1"
	o.mu.Unlock()
	// OBS refuses to start: the broadcast is ready but not on air.
	host, port := fakeOBSData(t, "", func(typ string, d json.RawMessage) (bool, int, any) {
		if typ == "StartStream" {
			return false, 600, nil
		}
		return o.answer(typ, d)
	})
	s.Store.UpdateSettings(func(set *store.Settings) { set.OBS.Host, set.OBS.Port = host, port })
	if code := call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil); code != 502 {
		t.Fatalf("%d", code)
	}
	if r := s.ytRecord("vid1"); r == nil || r.OnAir {
		t.Errorf("%+v", r)
	}
	s.ytEnd()
	if r := s.ytRecord("vid1"); r.Ended {
		t.Error("an unrelated stop ended it")
	}
}

func TestYouTubeUnlistUnknown(t *testing.T) {
	s, f, _ := ytSetup(t)
	if code := call(t, s, "POST", "/api/youtube/unlist", map[string]string{"broadcast_id": "someone-elses"}, nil); code != 404 || f.count("PUT /videos") != 0 {
		t.Errorf("%d %v", code, f.calls)
	}
}

func TestYouTubeThumbnailTooBig(t *testing.T) {
	s, f, _ := ytSetup(t)
	call(t, s, "POST", "/api/obs/stream/start", map[string]any{}, nil)
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("broadcast_id", "vid1")
	fw, _ := mw.CreateFormFile("file", "m.png")
	fw.Write(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, ytThumbMax)...))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/youtube/thumbnail", &b)
	req.RemoteAddr, req.Host = "127.0.0.1:5000", "127.0.0.1:8765"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code == 200 || f.count("POST /thumbnails/set") != 0 {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
