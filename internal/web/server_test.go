package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kauredo/tribuna/internal/fpb"
	"github.com/kauredo/tribuna/internal/obsws"
	"github.com/kauredo/tribuna/internal/state"
	"github.com/kauredo/tribuna/internal/store"
)

func redPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for i := range 100 {
		img.Set(i%10, i/10, color.NRGBA{0x2E, 0x6F, 0xD8, 255})
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// fakeFPB serves team 64791 (roster, next games, a logo) and 404s the rest.
func fakeFPB(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/teams/64791/roster":
			io.WriteString(w, `{"team":{"id":64791,"name":"CASA"},"season":"2026","players":[
				{"player_id":11,"name":"SILVA, Rui","jersey_number":"7","games":3},
				{"player_id":12,"name":"Pedro Nunes","jersey_number":8,"games":1}]}`)
		case "/api/teams/64791/next-games":
			io.WriteString(w, `{"team":{"id":64791},"games":[{"id":555,"date":"2026-11-15","time":"18H00",
				"home":{"id":64791,"name":"CASA","short_name":"CAS"},
				"away":{"id":900,"name":"FARPAS CLUBE","short_name":"FARPAS","logo_url":"`+srv.URL+`/logo.png"}},
				{"id":556,"date":"2026-11-22","time":"20H30","location":"Pavilhão X ,Vale Fundo","competition_name":"2ª Divisão",
				"home":{"id":901,"name":"Atlético do Vale","short_name":"ATV","logo_url":"`+srv.URL+`/logo.png"},
				"away":{"id":64791,"name":"CASA","short_name":"CAS","logo_url":"`+srv.URL+`/ours.png"}}]}`)
		case "/api/teams/64792/roster":
			io.WriteString(w, `{"team":{"id":64792,"name":"CASA"},"season":"2026","players":[
				{"player_id":11,"fpb_athlete_id":5011,"name":"SILVA, Rui","jersey_number":null,"photo_url":"`+srv.URL+`/photo/11.jpg","games":3},
				{"player_id":12,"fpb_athlete_id":5012,"name":"Pedro Nunes","jersey_number":null,"photo_url":"`+srv.URL+`/photo/missing.jpg","games":1}]}`)
		case "/photo/11.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("jpeg"))
		case "/logo.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(redPNG())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newServer(t *testing.T) *Server {
	t.Helper()
	return newServerFiles(t, fstest.MapFS{
		"overlay/index.html":  {Data: []byte("overlay")},
		"painel/index.html":   {Data: []byte("painel")},
		"fonts/fonts.css":     {Data: []byte("css")},
		"overlay/overlay.css": {Data: overlayCSS(t)},
	})
}

func newServerFiles(t *testing.T, files fs.FS) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fakeFPB(t)
	s := New(&Server{
		Store:   st,
		State:   state.New(time.Now),
		FPB:     fpb.New(func() string { return f.URL }, st.CacheDir(), "test"),
		OBS:     &obsws.Client{},
		Files:   files,
		Version: "test",
		Port:    8765,
	})
	s.FPB.Every = 0
	s.SyncGame()
	t.Cleanup(s.bg.Wait)
	return s
}

// call sends a JSON request and decodes the JSON answer into out.
func call(t *testing.T, s *Server, method, path string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	s.bg.Wait()
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v in %q", method, path, err, w.Body.String())
		}
	}
	return w.Code
}

// localGet is a GET from the PC's own browser or OBS.
func localGet(path string) *http.Request {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Host = "127.0.0.1:8765"
	return req
}

// lanIP makes 192.168.1.40 an address of this PC for the rest of the test.
func lanIP(t *testing.T) {
	t.Helper()
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.IPv4(192, 168, 1, 40), Mask: net.CIDRMask(24, 32)}}, nil
	}
	t.Cleanup(func() { interfaceAddrs = net.InterfaceAddrs })
}

func TestPagesAndState(t *testing.T) {
	s := newServer(t)
	for path, want := range map[string]string{"/overlay": "overlay", "/painel": "painel", "/fonts/fonts.css": "css"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, localGet(path))
		if w.Code != 200 || w.Body.String() != want {
			t.Errorf("%s: %d %q", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, localGet("/"))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/painel" {
		t.Errorf("/ -> %d %s", w.Code, w.Header().Get("Location"))
	}
	var st state.State
	call(t, s, "GET", "/api/state", nil, &st)
	if st.Screen.Current != "marcador" || st.Game.Home.Name != "CASA" || st.Game.Away.CrestURL != nil {
		t.Errorf("state %+v", st)
	}
}

func TestPlayersImportAndLink(t *testing.T) {
	s := newServer(t)
	var g store.Group
	if code := call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior M"}, &g); code != 201 {
		t.Fatalf("create group %d", code)
	}
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 64791, "coach": "Treinador"}, nil)

	var rui store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Rui Silva", "number": ""}, &rui)

	var res struct {
		Updated     []store.Player     `json:"updated"`
		Suggestions []store.Suggestion `json:"suggestions"`
	}
	if code := call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", nil, &res); code != 200 {
		t.Fatalf("import %d", code)
	}
	if len(res.Suggestions) != 2 || res.Suggestions[0].BestMatch == nil || *res.Suggestions[0].BestMatch != rui.ID {
		t.Fatalf("suggestions %+v", res.Suggestions)
	}

	var linked store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/"+rui.ID+"/link", map[string]any{"fpb_player_id": 11, "fpb_name": "SILVA, Rui"}, &linked)
	if linked.FPBPlayerID == nil || *linked.FPBPlayerID != 11 {
		t.Fatalf("link %+v", linked)
	}
	var e map[string]string
	if code := call(t, s, "POST", "/api/groups/"+g.ID+"/players/from-fpb", map[string]any{"fpb_player_id": 11, "name": "x"}, &e); code != 400 || e["error"] == "" {
		t.Errorf("double link: %d %v", code, e)
	}
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/from-fpb", map[string]any{"fpb_player_id": 12, "name": "Pedro Nunes", "number": "8"}, nil)

	call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", nil, &res)
	if len(res.Suggestions) != 0 || len(res.Updated) != 1 || res.Updated[0].Number != "7" || res.Updated[0].DisplayName != "Rui Silva" {
		t.Errorf("second import %+v", res)
	}

	var players []store.Player
	call(t, s, "GET", "/api/groups/"+g.ID+"/players", nil, &players)
	if len(players) != 2 {
		t.Errorf("players %+v", players)
	}
}

func TestPlayersImportPhotos(t *testing.T) {
	s := newServer(t)
	s.FPB.AllowHTTP = true
	s.FPB.PhotoHosts = []string{strings.TrimPrefix(s.FPB.BaseURL(), "http://")}
	media := func(u string) int {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, localGet(u))
		return w.Code
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior M"}, &g)
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 64792}, nil)
	var rui store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Rui Silva"}, &rui)

	var res struct {
		Updated     []store.Player     `json:"updated"`
		Suggestions []store.Suggestion `json:"suggestions"`
		PhotoErrors int                `json:"photo_errors"`
	}
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", nil, &res)
	if len(res.Suggestions) != 2 || !strings.HasSuffix(res.Suggestions[0].PhotoURL, "/photo/11.jpg") {
		t.Fatalf("suggestions %+v", res.Suggestions)
	}
	pedroPhoto := res.Suggestions[1].PhotoURL

	// Linked without a photo (an older panel); the next import fetches it.
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/"+rui.ID+"/link", map[string]any{"fpb_player_id": 11, "fpb_name": "SILVA, Rui"}, &rui)
	if rui.PhotoURL != "" {
		t.Fatalf("photo without a source: %+v", rui)
	}
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", nil, &res)
	g, _ = s.Store.Group(g.ID)
	rui = g.Players[0]
	if !strings.HasPrefix(rui.PhotoURL, "/media/pl-"+rui.ID+"-") || !strings.HasSuffix(rui.PhotoURL, ".jpg") || len(res.Updated) != 0 || res.PhotoErrors != 0 {
		t.Fatalf("import photo: %+v, %+v", rui, res)
	}
	if code := media(rui.PhotoURL); code != 200 {
		t.Errorf("photo not served: %d", code)
	}

	// A photo that fails is retried on the next import.
	var pedro store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/from-fpb", map[string]any{"fpb_player_id": 12, "name": "Pedro Nunes",
		"photo_url": pedroPhoto}, &pedro)
	if pedro.PhotoURL != "" || pedro.PhotoSource == "" {
		t.Errorf("failed photo: %+v", pedro)
	}
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", nil, &res)
	if res.PhotoErrors != 1 {
		t.Errorf("photo_errors %d, want 1", res.PhotoErrors)
	}

	set := s.Store.Settings()
	set.Photos.Squad = true
	call(t, s, "PUT", "/api/settings", set, nil)
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "squad": []map[string]string{{"player_id": rui.ID, "number": "7"}, {"player_id": pedro.ID}}}, nil)
	st := s.State.Snapshot()
	if !st.Game.Photos.Squad || st.Game.Photos.LowerThird || len(st.Game.Squad) != 2 || st.Game.Squad[0].PhotoURL != rui.PhotoURL || st.Game.Squad[1].PhotoURL != "" {
		t.Errorf("state squad %+v photos %+v", st.Game.Squad, st.Game.Photos)
	}
	call(t, s, "POST", "/api/lower-third", map[string]string{"player_id": rui.ID}, nil)
	if lt := s.State.Snapshot().LowerThird; lt.PhotoURL != rui.PhotoURL {
		t.Errorf("lower third %+v", lt)
	}

	call(t, s, "DELETE", "/api/groups/"+g.ID+"/players/"+rui.ID, nil, nil)
	if code := media(rui.PhotoURL); code != 404 {
		t.Errorf("photo left after delete: %d", code)
	}
}

// photoFPB serves /a.jpg and /b.jpg (different bytes), /big.jpg (over 5 MB)
// and /hook.jpg (runs hook first), and lets s download photos from it.
func photoFPB(t *testing.T, s *Server, hook func()) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		switch r.URL.Path {
		case "/a.jpg":
			io.WriteString(w, "photo a")
		case "/b.jpg":
			io.WriteString(w, "photo b")
		case "/big.jpg":
			w.Write(make([]byte, 5<<20+1))
		case "/hook.jpg":
			hook()
			io.WriteString(w, "photo hook")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	s.FPB.AllowHTTP = true
	s.FPB.PhotoHosts = []string{strings.TrimPrefix(srv.URL, "http://")}
	return srv.URL
}

// photoFiles lists the player photos in dados/media.
func photoFiles(t *testing.T, s *Server) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(s.Store.MediaDir(), "pl-*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPlayerPhotoLinkRelinkUnlink(t *testing.T) {
	s := newServer(t)
	u := photoFPB(t, s, nil)
	media := func(path string) int {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, localGet(path))
		if w.Code == 200 && w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s served without nosniff", path)
		}
		return w.Code
	}
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior M"}, &g)
	var rui, pedro store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Rui Silva"}, &rui)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Pedro Nunes"}, &pedro)
	link := func(p *store.Player, id int, photo string) {
		t.Helper()
		if code := call(t, s, "POST", "/api/groups/"+g.ID+"/players/"+p.ID+"/link",
			map[string]any{"fpb_player_id": id, "fpb_name": p.DisplayName, "photo_url": photo}, p); code != 200 {
			t.Fatalf("link %d", code)
		}
	}

	link(&rui, 11, u+"/a.jpg")
	first := rui.PhotoURL
	if !strings.HasPrefix(first, "/media/pl-"+rui.ID+"-") || media(first) != 200 {
		t.Fatalf("link with a photo: %+v", rui)
	}
	link(&pedro, 12, "https://www.fpb.pt/wp-content/themes/fpbasquetebol/assets/images/noplayer.png")
	if pedro.PhotoURL != "" || pedro.PhotoSource != "" {
		t.Errorf("placeholder stored: %+v", pedro)
	}

	// Another athlete with another photo replaces the file under a new name.
	link(&rui, 13, u+"/b.jpg")
	if rui.PhotoURL == "" || rui.PhotoURL == first || media(rui.PhotoURL) != 200 || media(first) != 404 {
		t.Errorf("replaced photo: %+v, old %s", rui, first)
	}
	// Another athlete without a photo drops it.
	link(&rui, 14, "")
	if rui.PhotoURL != "" || len(photoFiles(t, s)) != 0 {
		t.Errorf("relink kept the photo: %+v %v", rui, photoFiles(t, s))
	}

	link(&rui, 15, u+"/a.jpg")
	call(t, s, "DELETE", "/api/groups/"+g.ID+"/players/"+rui.ID+"/link", nil, &rui)
	if rui.PhotoURL != "" || rui.PhotoSource != "" || len(photoFiles(t, s)) != 0 {
		t.Errorf("unlink kept the photo: %+v %v", rui, photoFiles(t, s))
	}

	var ana store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/from-fpb", map[string]any{"fpb_player_id": 16, "name": "Ana Sousa", "photo_url": u + "/a.jpg"}, &ana)
	if !strings.HasPrefix(ana.PhotoURL, "/media/pl-"+ana.ID+"-") || media(ana.PhotoURL) != 200 {
		t.Errorf("from-fpb photo: %+v", ana)
	}
	call(t, s, "DELETE", "/api/groups/"+g.ID, nil, nil)
	if files := photoFiles(t, s); len(files) != 0 {
		t.Errorf("photos left after deleting the group: %v", files)
	}
}

func TestPlayerPhotoRelinkedWhileDownloading(t *testing.T) {
	s := newServer(t)
	var g store.Group
	var rui store.Player
	u := photoFPB(t, s, func() {
		s.Store.UpdateGroup(g.ID, func(g *store.Group) error {
			g.Players[0].PhotoSource = "https://sav2.fpb.pt/other.png"
			return nil
		})
	})
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior M"}, &g)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Rui Silva"}, &rui)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/"+rui.ID+"/link", map[string]any{"fpb_player_id": 11, "fpb_name": "Rui", "photo_url": u + "/hook.jpg"}, &rui)
	if rui.PhotoURL != "" || len(photoFiles(t, s)) != 0 {
		t.Errorf("stale photo applied: %+v %v", rui, photoFiles(t, s))
	}
}

func TestPlayerPhotoTooBig(t *testing.T) {
	s := newServer(t)
	u := photoFPB(t, s, nil)
	var g store.Group
	var rui store.Player
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior M"}, &g)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Rui Silva"}, &rui)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players/"+rui.ID+"/link", map[string]any{"fpb_player_id": 11, "fpb_name": "Rui", "photo_url": u + "/big.jpg"}, &rui)
	if rui.PhotoURL != "" || len(photoFiles(t, s)) != 0 {
		t.Errorf("oversized photo kept: %+v %v", rui, photoFiles(t, s))
	}
}

func TestPhotoHTTPOnlyWhenAllowed(t *testing.T) {
	a, b := newServer(t), newServer(t)
	u := photoFPB(t, a, nil)
	if _, _, err := a.FPB.DownloadPhoto(context.Background(), u+"/a.jpg"); err != nil {
		t.Fatalf("allowed client: %v", err)
	}
	b.FPB.PhotoHosts = a.FPB.PhotoHosts
	if _, _, err := b.FPB.DownloadPhoto(context.Background(), u+"/a.jpg"); err == nil || b.FPB.AllowHTTP {
		t.Error("http accepted by a client that did not allow it")
	}
}

func TestPhotoSettings(t *testing.T) {
	s := newServer(t)
	set := s.Store.Settings()
	set.Photos.LowerThird = true
	call(t, s, "PUT", "/api/settings", set, nil)
	var got store.Settings
	call(t, s, "GET", "/api/settings", nil, &got)
	if got.Photos != (store.Photos{LowerThird: true}) {
		t.Errorf("settings photos %+v", got.Photos)
	}
	if p := s.State.Snapshot().Game.Photos; p.Squad || !p.LowerThird {
		t.Errorf("state photos %+v", p)
	}
}

func TestBugPosition(t *testing.T) {
	s := newServer(t)
	s.SyncGame()
	if p := s.State.Snapshot().Game.Overlay.BugPosition; p != "top-left" {
		t.Errorf("default %q", p)
	}
	for _, c := range []struct{ send, want string }{{"bottom-left", "bottom-left"}, {"middle", "top-left"}, {"", "top-left"}} {
		set := s.Store.Settings()
		set.Overlay.BugPosition = c.send
		var got store.Settings
		call(t, s, "PUT", "/api/settings", set, &got)
		if got.Overlay.BugPosition != c.want || s.State.Snapshot().Game.Overlay.BugPosition != c.want {
			t.Errorf("sent %q: settings %q, state %q", c.send, got.Overlay.BugPosition, s.State.Snapshot().Game.Overlay.BugPosition)
		}
	}
}

func TestSponsorCornerSettings(t *testing.T) {
	s := newServer(t)
	s.SyncGame()
	if o := s.State.Snapshot().Game.Overlay; !o.SponsorCorner || o.SponsorEvery != 4 {
		t.Errorf("default %+v", o)
	}
	for _, c := range []struct {
		on         bool
		send, want int
	}{{false, 6, 6}, {true, 0, 4}, {true, 31, 4}, {true, 30, 30}, {true, 1, 1}} {
		set := s.Store.Settings()
		set.Overlay.SponsorCorner, set.Overlay.SponsorEvery = c.on, c.send
		var got store.Settings
		call(t, s, "PUT", "/api/settings", set, &got)
		o := s.State.Snapshot().Game.Overlay
		if got.Overlay.SponsorCorner != c.on || got.Overlay.SponsorEvery != c.want || o.SponsorCorner != c.on || o.SponsorEvery != c.want {
			t.Errorf("sent %v %d: settings %+v, state %+v", c.on, c.send, got.Overlay, o)
		}
	}
}

func TestAnotherGameClearsPeriods(t *testing.T) {
	s := newServer(t)
	saved := -1
	s.State.SavePeriods = func(ps []state.Period) { saved = len(ps) }
	game := map[string]any{"away": map[string]string{"name": "ATLÉTICO VALE"}, "start": "2026-11-15T18:00:00+00:00"}
	call(t, s, "PUT", "/api/game", game, nil)
	saved = -1
	s.State.RestorePeriods([]state.Period{{Period: 1, Home: 20, Away: 18, HomePoints: 20, AwayPoints: 18}})
	game["away"] = map[string]string{"name": "ATLÉTICO VALE", "short": "LOU"} // same game, edited
	call(t, s, "PUT", "/api/game", game, nil)
	if n := len(s.State.Snapshot().Periods); n != 1 || saved != -1 {
		t.Fatalf("same game: %d periods, saved %d", n, saved)
	}
	game["start"] = "2026-11-15T18:30:00+00:00" // tip-off moved, same game
	call(t, s, "PUT", "/api/game", game, nil)
	if n := len(s.State.Snapshot().Periods); n != 1 || saved != -1 {
		t.Fatalf("new time: %d periods, saved %d", n, saved)
	}
	game["start"] = "2026-11-22T18:00:00+00:00"
	call(t, s, "PUT", "/api/game", game, nil)
	if n := len(s.State.Snapshot().Periods); n != 0 || saved != 0 {
		t.Errorf("next game: %d periods, saved %d", n, saved)
	}
}

func TestGameWithFPBOpponent(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	call(t, s, "PUT", "/api/groups/"+g.ID, map[string]any{"fpb_team_id": 64791}, nil)
	var p store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Nome Apelido Longo", "number": "4"}, &p)

	var game store.Game
	code := call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "fpb_game_id": 555,
		"squad": []map[string]string{{"player_id": p.ID, "number": "9"}}}, &game)
	if code != 200 || game.Away.CrestURL != "/media/opp-900.png" || game.Away.Name != "FARPAS CLUBE" {
		t.Fatalf("game %d %+v", code, game)
	}
	if fpb.Contrast(game.Away.Color) < 4.5 {
		t.Errorf("colour %s", game.Away.Color)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, localGet(game.Away.CrestURL))
	if w.Code != 200 {
		t.Errorf("logo not served: %d", w.Code)
	}

	st := s.State.Snapshot()
	if st.Game.Group == nil || st.Game.Away.Name != "FARPAS CLUBE" || len(st.Game.Squad) != 1 || st.Game.Squad[0].Number != "9" {
		t.Errorf("state game %+v", st.Game)
	}

	call(t, s, "POST", "/api/lower-third", map[string]string{"player_id": p.ID}, nil)
	if lt := s.State.Snapshot().LowerThird; !lt.Visible || lt.Number != "9" || lt.FirstName != "Nome" || lt.LastName != "Apelido Longo" {
		t.Errorf("lower third %+v", lt)
	}

	// Playing away: the opponent is the home side.
	code = call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "fpb_game_id": 556}, &game)
	if code != 200 || game.Away.CrestURL != "/media/opp-901.png" || game.Away.Name != "ATLÉTICO VALE" || game.Away.Short != "ATV" ||
		game.Venue != "Pavilhão X, Vale Fundo" || game.Away.Color == fpb.Neutral {
		t.Fatalf("away game %d %+v", code, game)
	}
	if a := s.State.Snapshot().Game.Away; a.CrestURL == nil || *a.CrestURL != "/media/opp-901.png" || a.Name != "ATLÉTICO VALE" {
		t.Errorf("state away %+v", a)
	}
}

func TestOpponentName(t *testing.T) {
	for _, tt := range []struct{ name, short, want string }{
		{"Atlético do Vale", "ATV", "ATLÉTICO VALE"},
		{"FARPAS CLUBE", "FARPAS", "FARPAS CLUBE"}, // fits: kept whole
		{"Grupo Desportivo da Ponte Velha", "GDP", "PONTE VELHA"},
		{"Associação Académica de Vila Rica", "AAC", "ACADÉMICA VILA RICA"},
		{"Clube Exemplo", "CEX", "EXEMPLO"},
		{"Grupo Desportivo de Basquetebol", "GDB", "GDB"},    // nothing left
		{"Clube de Basquetebol", "", "CLUBE DE BASQUETEBOL"}, // nothing left, but it fits whole
		{"Basquetebol Clube da Bairrada", "", "BAIRRADA"},
		{"Escola de Basquetebol de Vila Nova", "", "ESCOLA VILA NOVA"},
		{"Clube Desportivo Universitário de Vila Rica Norte", "", "UNIVERSITÁRIO VILA"}, // no short name: cut at a space
		{"Clube Desportivo Universitário de Vila Rica Norte", "CDUL", "CDUL"},
	} {
		if got := opponentName(tt.name, tt.short); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestManualOpponentLogo(t *testing.T) {
	s := newServer(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "logo.png")
	fw.Write(redPNG())
	mw.Close()
	req := httptest.NewRequest("POST", "/api/game/opponent-logo", &body)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var up map[string]string
	json.Unmarshal(w.Body.Bytes(), &up)
	if w.Code != 200 || !strings.HasPrefix(up["crest_url"], "/media/opp-") {
		t.Fatalf("upload %d %v", w.Code, up)
	}
	var game store.Game
	call(t, s, "PUT", "/api/game", map[string]any{"fpb_game_id": nil,
		"away": map[string]string{"name": "Adversário", "crest_url": up["crest_url"]}}, &game)
	if game.Away.Color == fpb.Neutral || fpb.Contrast(game.Away.Color) < 4.5 {
		t.Errorf("colour from uploaded logo: %s", game.Away.Color)
	}
	if st := s.State.Snapshot().Game.Away; st.CrestURL == nil || st.Name != "Adversário" {
		t.Errorf("state away %+v", st)
	}
}

func TestFPB404(t *testing.T) {
	s := newServer(t)
	var e map[string]string
	if code := call(t, s, "GET", "/api/fpb/teams/1/next-games", nil, &e); code != 404 || e["error"] == "" {
		t.Errorf("got %d %v", code, e)
	}
}

func TestSponsorUploadAndOrder(t *testing.T) {
	s := newServer(t)
	upload := func(name string) store.Sponsor {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		mw.WriteField("name", name)
		fw, _ := mw.CreateFormFile("file", name+".png")
		fw.Write(redPNG())
		mw.Close()
		req := httptest.NewRequest("POST", "/api/groups/geral/sponsors", &body)
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Host = "127.0.0.1:8765"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		var sp store.Sponsor
		json.Unmarshal(w.Body.Bytes(), &sp)
		if w.Code != 201 {
			t.Fatalf("upload %d %s", w.Code, w.Body.String())
		}
		return sp
	}
	a, b := upload("A"), upload("B")
	var out []store.Sponsor
	call(t, s, "PUT", "/api/groups/geral/sponsors", []map[string]any{{"id": b.ID, "principal": true}, {"id": a.ID}}, &out)
	if len(out) != 2 || out[0].ID != b.ID || !out[0].Principal {
		t.Fatalf("order %+v", out)
	}
	sp := s.State.Snapshot().Game.Sponsors
	if sp.Principal == nil || sp.Principal.Name != "B" || len(sp.Others) != 1 {
		t.Errorf("state sponsors %+v", sp)
	}
}

func TestScreenAndSettings(t *testing.T) {
	s := newServer(t)
	var e map[string]string
	if code := call(t, s, "POST", "/api/screen", map[string]any{"screen": "emblema"}, &e); code != 400 {
		t.Errorf("bad screen %d", code)
	}
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "intervalo", "auto": false}, nil)
	if sc := s.State.Snapshot().Screen; sc.Current != "intervalo" || sc.Auto {
		t.Errorf("screen %+v", sc)
	}
	if s.Store.Settings().AutoScreens {
		t.Error("auto_screens not saved")
	}
	call(t, s, "POST", "/api/screen", map[string]any{"screen": "apoiantes", "auto": false}, nil)
	if sc := s.State.Snapshot().Screen; sc.Current != "apoiantes" {
		t.Errorf("screen %+v", sc)
	}
	if code := call(t, s, "PUT", "/api/settings", map[string]any{"phone_access": true, "pin": "12"}, &e); code != 400 {
		t.Errorf("short pin accepted: %d", code)
	}
}

func TestPIN(t *testing.T) {
	s := newServer(t)
	set := s.Store.Settings()
	set.PhoneAccess, set.PIN = true, "1234"
	s.Store.SaveSettings(set)
	lanIP(t)

	get := func(path string, mod func(*http.Request)) int {
		req := httptest.NewRequest("GET", path, nil)
		req.RemoteAddr = "192.168.1.20:5000"
		req.Host = "192.168.1.40:8765"
		if mod != nil {
			mod(req)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w.Code
	}
	if code := get("/api/groups", nil); code != 401 {
		t.Errorf("no pin: %d", code)
	}
	if code := get("/api/groups", func(r *http.Request) { r.Header.Set("X-PIN", "1234") }); code != 200 {
		t.Errorf("header pin: %d", code)
	}
	if code := get("/api/groups", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "pin", Value: "1234"}) }); code != 200 {
		t.Errorf("cookie pin: %d", code)
	}
	if code := get("/api/events", nil); code != 401 {
		t.Errorf("events without pin: %d", code)
	}
	for _, page := range []string{"/painel", "/fonts/fonts.css"} {
		if code := get(page, nil); code != 200 {
			t.Errorf("%s: %d", page, code)
		}
	}
	if code := call(t, s, "GET", "/api/groups", nil, nil); code != 200 {
		t.Errorf("localhost: %d", code)
	}
}

func TestEvents(t *testing.T) {
	s := newServer(t)
	srv := httptest.NewServer(s)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rd := bufio.NewReader(resp.Body)
	next := func() string {
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "data: ") {
				return line
			}
		}
	}
	if !strings.Contains(next(), `"screen"`) {
		t.Error("first event has no state")
	}
	s.State.SetScreen("final", true)
	if !strings.Contains(next(), `"current":"final"`) {
		t.Error("change not sent")
	}
}
