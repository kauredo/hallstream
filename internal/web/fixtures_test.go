package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/kauredo/hallstream/internal/store"
	assets "github.com/kauredo/hallstream/web"
)

// postFile sends name/data as the "file" part of a multipart POST from the PC.
func postFile(t *testing.T, s *Server, path, name string, data []byte, out any) int {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	s.bg.Wait()
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("POST %s: %v in %q", path, err, w.Body.String())
		}
	}
	return w.Code
}

func setClub(t *testing.T, s *Server, sport, country string) {
	t.Helper()
	if _, err := s.Store.UpdateSettings(func(v *store.Settings) { v.Club.Sport, v.Club.Country = sport, country }); err != nil {
		t.Fatal(err)
	}
}

// A club the FPB does not cover never reaches it: the FPB routes answer 404
// and the jobs that read the FPB do nothing.
func TestFPBOnlyForPortugueseBasketball(t *testing.T) {
	for _, c := range []struct {
		sport, country string
		offered        bool
	}{{"basketball", "PT", true}, {"futsal", "PT", false}, {"basketball", "ES", false}} {
		s := newServer(t)
		setClub(t, s, c.sport, c.country)
		var g store.Group
		call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior"}, &g)
		for _, p := range []string{"/api/fpb/teams?name=Clube", "/api/fpb/teams/64791/roster"} {
			var e struct{ Error string }
			call(t, s, "GET", p, nil, &e)
			if (e.Error == noFPB.Error()) == c.offered {
				t.Errorf("%s %s: GET %s answered %q", c.sport, c.country, p, e.Error)
			}
		}
		if code := call(t, s, "POST", "/api/groups/"+g.ID+"/players/import", map[string]any{}, nil); !c.offered && code != http.StatusNotFound {
			t.Errorf("%s %s: import = %d", c.sport, c.country, code)
		}
		if s.fpbOffered() != c.offered {
			t.Errorf("%s %s: fpbOffered %v", c.sport, c.country, s.fpbOffered())
		}
	}
}

func TestSquadCSV(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Hélio Vaz", "number": "4"}, nil)
	var got struct {
		Added, Updated int
		Skipped        []store.LineError
		Players        []store.Player
	}
	csv := "Número;Nome;Foto\n7;Hélio Vaz;\n9;Ana Lima;\n;;\nxx;Rui;\n"
	if code := postFile(t, s, "/api/groups/"+g.ID+"/players/csv", "plantel.csv", []byte(csv), &got); code != 200 {
		t.Fatalf("csv: %d", code)
	}
	if got.Added != 1 || got.Updated != 1 || len(got.Skipped) != 1 || got.Skipped[0].Line != 5 || len(got.Players) != 2 {
		t.Errorf("%+v", got)
	}
	if p := got.Players[0]; p.Number != "7" {
		t.Errorf("number not updated: %+v", p)
	}
}

// A photo the club uploads stays through an FPB import of the same player.
func TestPlayerPhotoByHand(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sub 16"}, &g)
	var p store.Player
	call(t, s, "POST", "/api/groups/"+g.ID+"/players", map[string]string{"display_name": "Ana Lima", "number": "9"}, &p)
	var got store.Player
	if code := postFile(t, s, "/api/groups/"+g.ID+"/players/"+p.ID+"/photo", "ana.png", redPNG(), &got); code != 200 || !got.PhotoByHand || !strings.HasPrefix(got.PhotoURL, "/media/pl-"+p.ID) {
		t.Fatalf("upload: %d %+v", code, got)
	}
	linked, fpbID := got, int64(5)
	linked.FPBPlayerID = &fpbID
	players, _ := store.Import([]store.Player{linked}, []store.RosterPlayer{{ID: 5, Name: "ANA LIMA", PhotoURL: "https://fpb.example/ana.jpg"}})
	for _, x := range players {
		if x.ID == p.ID && (x.PhotoSource != "" || x.PhotoURL != got.PhotoURL) {
			t.Errorf("an FPB import replaced the photo: %+v", x)
		}
	}
	if code := call(t, s, "DELETE", "/api/groups/"+g.ID+"/players/"+p.ID+"/photo", nil, &got); code != 200 || got.PhotoURL != "" || got.PhotoByHand {
		t.Errorf("remove: %d %+v", code, got)
	}
	if code := postFile(t, s, "/api/groups/"+g.ID+"/players/nobody/photo", "x.png", redPNG(), nil); code != 404 {
		t.Errorf("unknown player: %d", code)
	}
}

// Fixtures typed by hand feed the next game for a club with no FPB, and its
// at_home for the game of the day.
func TestTypedFixturesMakeTheNextGame(t *testing.T) {
	s := newServer(t)
	setClub(t, s, "futsal", "ES")
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior"}, &g)
	day := func(d int, h int) string {
		return time.Now().In(lisbon).AddDate(0, 0, d).Truncate(time.Hour).Add(time.Duration(h) * time.Hour).Format(time.RFC3339)
	}
	today := time.Date(time.Now().In(lisbon).Year(), time.Now().In(lisbon).Month(), time.Now().In(lisbon).Day(), 21, 0, 0, 0, lisbon).Format(time.RFC3339)
	var f store.Fixture
	if code := call(t, s, "POST", "/api/groups/"+g.ID+"/fixtures", store.Fixture{Start: day(9, 0), Opponent: store.Opponent{Name: "GD Monte", Short: "gdm", Color: "#0b6e4f"}, Home: false, Round: "8"}, &f); code != 201 || f.ID == "" || f.Opponent.Short != "GDM" || f.Opponent.Color != "#0B6E4F" {
		t.Fatalf("add: %d %+v", code, f)
	}
	call(t, s, "POST", "/api/groups/"+g.ID+"/fixtures", store.Fixture{Start: today, Opponent: store.Opponent{Name: "Ribeira"}, Home: true}, nil)
	call(t, s, "POST", "/api/groups/"+g.ID+"/fixtures", store.Fixture{Start: day(4, 0), Opponent: store.Opponent{Name: "Vale"}, Home: true}, nil)
	if code := call(t, s, "POST", "/api/groups/"+g.ID+"/fixtures", store.Fixture{Start: "amanhã", Opponent: store.Opponent{Name: "X"}}, nil); code != 400 {
		t.Errorf("bad date: %d", code)
	}
	var list []store.Fixture
	call(t, s, "GET", "/api/groups/"+g.ID+"/fixtures", nil, &list)
	if len(list) != 3 || list[0].Opponent.Name != "Ribeira" || list[2].Opponent.Name != "GD Monte" {
		t.Fatalf("not in date order: %+v", list)
	}
	call(t, s, "PUT", "/api/game", map[string]any{"group_id": g.ID, "start": today, "away": map[string]string{"name": "Ribeira"}}, nil)
	s.RefreshNext()
	gm := s.State.Snapshot().Game
	if gm.Next == nil || gm.Next.Opponent.Name != "Vale" || !gm.Next.AtHome || gm.AtHome == nil || !*gm.AtHome {
		t.Fatalf("next %+v, at_home %v, note %q", gm.Next, gm.AtHome, gm.NextNote)
	}
	// Moving Vale after GD Monte makes GD Monte the next game, away, with its round.
	list[1].Start = day(12, 0)
	call(t, s, "PUT", "/api/groups/"+g.ID+"/fixtures/"+list[1].ID, list[1], nil)
	s.RefreshNext()
	if n := s.State.Snapshot().Game.Next; n == nil || n.Opponent.Name != "GD Monte" || n.AtHome || n.Round != "Jornada 8" || n.Opponent.Color == "" {
		t.Errorf("after the move: %+v", n)
	}
	call(t, s, "DELETE", "/api/groups/"+g.ID+"/fixtures/"+f.ID, nil, nil)
	call(t, s, "GET", "/api/groups/"+g.ID+"/fixtures", nil, &list)
	if len(list) != 2 {
		t.Errorf("delete: %+v", list)
	}
}

// The panel, for a club the FPB does not cover: a squad CSV dropped on the
// Plantel tab adds the players, and a typed fixture picked in Jogo becomes
// the game of the day.
func TestPanelSquadCSVAndFixtures(t *testing.T) {
	s := newServerFiles(t, assets.FS)
	setClub(t, s, "basketball", "ES")
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior"}, &g)
	start := time.Now().In(lisbon).AddDate(0, 0, 3).Truncate(time.Hour).Format(time.RFC3339)
	call(t, s, "POST", "/api/groups/"+g.ID+"/fixtures", store.Fixture{Start: start, Opponent: store.Opponent{Name: "Ribeira FC"}, Venue: "Pavilhão da Ribeira"}, nil)
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	tab := chrome(t) // after the server, so the panel's event stream ends before the server closes
	messages := chromedp.Console(tab)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/painel#plantel")); err != nil {
		t.Fatal(err)
	}
	if err := until(tab, `document.querySelector("#pCsv") !== null`); err != nil {
		t.Fatalf("no CSV import on Plantel: %v", err)
	}
	run := func(js string) {
		t.Helper()
		if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](js+`, true`)); err != nil {
			t.Fatal(err)
		}
	}
	run(`(() => { const dt = new DataTransfer(); dt.items.add(new File(["numero;nome\n4;Ana Exemplo\n7;Rui Exemplo\n"], "p.csv", { type: "text/csv" }));
		const i = document.querySelector("#pCsv"); i.files = dt.files; i.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	if err := until(tab, `document.querySelectorAll("table.players tbody tr").length === 2 && document.querySelector(".csv-result") !== null`); err != nil {
		t.Errorf("after the CSV: %v", err)
	}
	run(`document.querySelector('.nav [data-tab="jogo"]').click()`)
	run(`(() => { const sel = document.querySelector("#jGroup"); sel.value = "` + g.ID + `"; sel.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	if err := until(tab, `document.querySelector('#jFixtures [data-act="pick-fixture"]') !== null`); err != nil {
		t.Fatalf("no fixture to pick: %v", err)
	}
	run(`document.querySelector('#jFixtures [data-act="pick-fixture"]').click()`)
	deadline := time.Now().Add(10 * time.Second)
	for s.Store.Game().Away.Name != "Ribeira FC" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if gm := s.Store.Game(); gm.Away.Name != "Ribeira FC" || gm.Venue != "Pavilhão da Ribeira" || gm.GroupID != g.ID {
		t.Errorf("the game of the day %+v", gm)
	}
	if errs := problems(t, tab, messages, func(m chromedp.ConsoleMessage) bool { return m.Type == chromedp.ConsoleError || m.IsException() }); len(errs) > 0 {
		t.Errorf("console errors:\n%s", strings.Join(errs, "\n"))
	}
}

func TestFixturesCSVAndCrest(t *testing.T) {
	s := newServer(t)
	var g store.Group
	call(t, s, "POST", "/api/groups", map[string]string{"name": "Sénior"}, &g)
	var got struct {
		Added    int
		Skipped  []store.LineError
		Fixtures []store.Fixture
	}
	csv := "data,hora,adversario,casa_fora\n20/12/2030,18:30,Ribeira Basket,fora\n21/12/2030,talvez,Vale,casa\n"
	if code := postFile(t, s, "/api/groups/"+g.ID+"/fixtures/csv", "jogos.csv", []byte(csv), &got); code != 200 || got.Added != 1 || len(got.Skipped) != 1 {
		t.Fatalf("csv: %d %+v", code, got)
	}
	id := got.Fixtures[0].ID
	var f store.Fixture
	if code := postFile(t, s, "/api/groups/"+g.ID+"/fixtures/"+id+"/crest", "ribeira.png", redPNG(), &f); code != 200 || !strings.HasPrefix(f.Opponent.CrestURL, "/media/opp-") || f.Opponent.Color == "" {
		t.Fatalf("crest: %d %+v", code, f)
	}
	// Editing the fixture keeps the crest; deleting it removes the upload.
	f.Venue = "Pavilhão da Ribeira"
	call(t, s, "PUT", "/api/groups/"+g.ID+"/fixtures/"+id, f, &f)
	if f.Opponent.CrestURL == "" {
		t.Error("edit dropped the crest")
	}
	crest := f.Opponent.CrestURL
	call(t, s, "DELETE", "/api/groups/"+g.ID+"/fixtures/"+id, nil, nil)
	if _, err := s.Store.ReadMedia(crest); err == nil {
		t.Error("the crest stayed after the fixture went")
	}
}
