package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kauredo/hallstream/internal/club"
)

func ptr[T any](v T) *T { return &v }

// A direct link keeps its roster entry even when another player, earlier in
// the list, could take it by athlete id.
func TestImportAthleteMatchOrder(t *testing.T) {
	players := []Player{
		{ID: "old", DisplayName: "Rui", FPBPlayerID: ptr(int64(1)), FPBAthleteID: 5001},
		{ID: "new", DisplayName: "Rui Silva", FPBPlayerID: ptr(int64(101))},
	}
	Import(players, []RosterPlayer{{ID: 101, AthleteID: 5001, Name: "Rui Silva"}})
	if *players[0].FPBPlayerID == 101 || *players[1].FPBPlayerID != 101 {
		t.Errorf("ids %d and %d", *players[0].FPBPlayerID, *players[1].FPBPlayerID)
	}

	// Without a direct link, the athlete id moves the player to the new id.
	players = []Player{{ID: "a", DisplayName: "Rui", FPBPlayerID: ptr(int64(1)), FPBAthleteID: 5001, Inactive: true}}
	Import(players, []RosterPlayer{{ID: 101, AthleteID: 5001, Name: "Rui Silva"}})
	if *players[0].FPBPlayerID != 101 || players[0].Inactive {
		t.Errorf("athlete match %+v", players[0])
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	set := s.Settings()
	set.Instagram = "@outro"
	if err := s.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	g, err := s.AddGroup("Sénior Masculino")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "senior-masculino" {
		t.Errorf("id %q", g.ID)
	}
	if g2, _ := s.AddGroup("Sénior Masculino"); g2.ID != "senior-masculino-2" {
		t.Errorf("second id %q", g2.ID)
	}
	_, err = s.UpdateGroup(g.ID, func(g *Group) error {
		g.Players = append(g.Players, Player{ID: "p1", DisplayName: "João", Number: "4", FPBPlayerID: ptr(int64(9))})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	game := Game{GroupID: g.ID, Away: Opponent{Name: "FARPAS"}, Squad: []SquadEntry{{"p1", "5"}}}
	if err := s.SaveGame(game); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s2.Settings(), set) {
		t.Errorf("settings %+v", s2.Settings())
	}
	if !reflect.DeepEqual(s2.Game(), game) {
		t.Errorf("game %+v", s2.Game())
	}
	got, err := s2.Group(g.ID)
	if err != nil || len(got.Players) != 1 || *got.Players[0].FPBPlayerID != 9 {
		t.Errorf("group %+v, %v", got, err)
	}
	if _, err := s2.Group(Shared); err != nil {
		t.Error("shared group missing")
	}
	if err := s2.DeleteGroup(Shared); err == nil {
		t.Error("shared group deleted")
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestUpdateGroupFailureKeepsData(t *testing.T) {
	s, _ := Open(t.TempDir())
	g, _ := s.AddGroup("Sub 14")
	s.UpdateGroup(g.ID, func(g *Group) error {
		g.Name = "mudado"
		return os.ErrInvalid
	})
	if got, _ := s.Group(g.ID); got.Name != "Sub 14" {
		t.Errorf("name %q", got.Name)
	}
}

func TestImport(t *testing.T) {
	players := []Player{
		{ID: "a", DisplayName: "Zé", Number: "4", FPBPlayerID: ptr(int64(1))},                      // hand-set number
		{ID: "b", DisplayName: "Rui", Number: "", FPBPlayerID: ptr(int64(2)), FPBName: ptr("RUI")}, // no number yet
		{ID: "c", DisplayName: "Tó", Number: "8", FPBPlayerID: ptr(int64(3)), NumberFromFPB: true}, // number from FPB
		{ID: "d", DisplayName: "João Gonçalves", Number: "10"},                                     // unlinked
		{ID: "e", DisplayName: "Miguel", Number: ""},                                               // unlinked
		{ID: "f", DisplayName: "Ana", FPBPlayerID: ptr(int64(99)), FPBName: ptr("Ana Sousa")},      // not in roster
	}
	roster := []RosterPlayer{
		{ID: 1, Name: "José Manuel Silva", Number: "7", Games: 3},
		{ID: 2, Name: "Rui Costa", Number: "11", Games: 2},
		{ID: 3, Name: "António Lopes", Number: "9", Games: 5},
		{ID: 4, Name: "GONCALVES, Joao Pedro", Number: "10", Games: 4},
		{ID: 5, Name: "Pedro Nunes", Number: "12", Games: 1},
	}
	updated, sugg := Import(players, roster)

	if p := players[0]; p.Number != "4" || p.DisplayName != "Zé" || *p.FPBName != "José Manuel Silva" {
		t.Errorf("hand-set number changed: %+v", p)
	}
	if p := players[1]; p.Number != "11" || !p.NumberFromFPB || *p.FPBName != "Rui Costa" {
		t.Errorf("empty number not filled: %+v", p)
	}
	if p := players[2]; p.Number != "9" {
		t.Errorf("FPB number not updated: %+v", p)
	}
	if p := players[5]; *p.FPBName != "Ana Sousa" {
		t.Errorf("player missing from roster changed: %+v", p)
	}
	if len(updated) != 3 {
		t.Errorf("updated %d, want 3", len(updated))
	}
	if len(sugg) != 2 {
		t.Fatalf("suggestions %+v", sugg)
	}
	if sugg[0].FPBPlayerID != 4 || sugg[0].BestMatch == nil || *sugg[0].BestMatch != "d" {
		t.Errorf("surname-first match: %+v", sugg[0])
	}
	if sugg[1].BestMatch != nil {
		t.Errorf("Pedro Nunes matched %q", *sugg[1].BestMatch)
	}

	// A second import with nothing new changes nothing.
	if updated, _ := Import(players, roster); len(updated) != 0 {
		t.Errorf("second import updated %d", len(updated))
	}
}

func TestImportPhotoSource(t *testing.T) {
	players := []Player{
		{ID: "a", DisplayName: "Zé", FPBPlayerID: ptr(int64(1)), FPBName: ptr("Zé"), PhotoURL: "/media/pl-a.png", PhotoSource: "https://sav2.fpb.pt/old.png"},
		{ID: "b", DisplayName: "Rui", FPBPlayerID: ptr(int64(2)), FPBName: ptr("Rui")},
	}
	roster := []RosterPlayer{
		{ID: 1, Name: "Zé", PhotoURL: "https://sav2.fpb.pt/new.png"},
		{ID: 2, Name: "Rui"},
		{ID: 3, Name: "Tó", PhotoURL: "https://sav2.fpb.pt/to.png"},
	}
	updated, sugg := Import(players, roster)
	if p := players[0]; p.PhotoSource != "https://sav2.fpb.pt/new.png" || p.PhotoURL != "/media/pl-a.png" {
		t.Errorf("photo source not updated, or local photo touched: %+v", p)
	}
	if len(updated) != 0 {
		t.Errorf("a new photo alone counted as an update: %+v", updated)
	}
	if len(sugg) != 1 || sugg[0].PhotoURL != "https://sav2.fpb.pt/to.png" {
		t.Errorf("suggestions %+v", sugg)
	}
}

func TestBestMatchNeedsMoreThanOneCommonWord(t *testing.T) {
	players := []Player{{ID: "x", DisplayName: "João Silva Santos"}}
	if m := BestMatch("João Ferreira Costa", players); m != nil {
		t.Errorf("matched on first name only")
	}
	if m := BestMatch("Santos, João", players); m == nil {
		t.Errorf("no match for Santos, João")
	}
}

func TestPeriodsFile(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	type period struct{ Period, Home int }
	var got []period
	if err := s.LoadPeriods(&got); err != nil || got != nil {
		t.Fatalf("no file: %v %v", got, err)
	}
	want := []period{{1, 20}, {2, 41}}
	if err := s.SavePeriods(want); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadPeriods(&got); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %v %v", got, err)
	}
}

func TestOldSettingsGetTheSponsorCorner(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"overlay":{"bug_position":"bottom-left"}}`), 0o644)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if o := s.Settings().Overlay; o != (Overlay{BugPosition: "bottom-left", SponsorCorner: true, SponsorEvery: 4}) {
		t.Errorf("overlay %+v", o)
	}
}

func TestOldSettingsMoveHomeNameToClub(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"home_name":"VALE","home_short":"VAL","instagram":"@vale"}`), 0o644)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultSettings().Club
	want.Name, want.Short = "VALE", "VAL"
	if c := s.Settings().Club; c != want {
		t.Errorf("club %+v, want %+v", c, want)
	}
	if err := s.SaveSettings(s.Settings()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var saved map[string]any
	json.Unmarshal(b, &saved)
	if _, old := saved["home_name"]; old || saved["club"] == nil {
		t.Errorf("saved settings still hold home_name or lack club: %s", b)
	}

	// A file that already has the club keeps it, whatever home_name says.
	os.WriteFile(path, []byte(`{"home_name":"VELHO","club":{"name":"NOVO","short":"NOV"}}`), 0o644)
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c := s.Settings().Club; c.Name != "NOVO" || c.Short != "NOV" || c.Primary != want.Primary || c.Sport != "basketball" {
		t.Errorf("club %+v", c)
	}
}

func TestDefaultClub(t *testing.T) {
	c := DefaultSettings().Club
	if c != (Club{Name: "CASA", Short: "CAS", Primary: club.DefaultPrimary, Secondary: club.DefaultSecondary,
		Crest: DefaultCrest, Sport: "basketball", Country: "PT"}) {
		t.Errorf("default club %+v", c)
	}
}
