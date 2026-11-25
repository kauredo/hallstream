package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// manual.json keeps the suspensions and the last ID given out, and a file with
// out-of-range ends loads clamped. A restart that primes the state with the
// scorer's frame makes what it holds no news.
func TestManualFileHoldsSuspensions(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	hb := func() sport.Profile { return sport.Handball }
	played := source.NewManual(&source.Manual{Rules: hb})
	for _, a := range []source.Action{{Do: "new_game"}, {Do: "suspension", Team: "home"}, {Do: "suspension", Team: "away"},
		{Do: "suspension_number", ID: 2, Number: "11"}} {
		if err := played.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveManual(played.Score()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manual.json"))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"home_susp", "away_susp", "next_susp"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("manual.json has no %s key", k)
		}
	}
	if string(keys["next_susp"]) != "2" {
		t.Errorf("next_susp %s, want 2", keys["next_susp"])
	}

	back := source.NewManual(&source.Manual{Rules: hb})
	restoreManual(st, back)
	if got, want := back.Score(), played.Score(); !reflect.DeepEqual(got, want) {
		t.Errorf("restored %+v, want %+v", got, want)
	}

	// A damaged file: an end far beyond the clock, a negative ID, a bad number.
	bad := `{"period":1,"length":18000,"clock":100,"next_susp":0,` +
		`"home_susp":[{"id":4,"number":"x9","end":999999999},{"id":-3,"number":"1","end":50},{"id":0,"number":"","end":0}]}`
	if err := os.WriteFile(filepath.Join(dir, "manual.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	clamped := source.NewManual(&source.Manual{Rules: hb})
	restoreManual(st, clamped)
	s := clamped.Score()
	if s.HomeSusp[0].End > 2*source.MaxClock || s.HomeSusp[0].Number != "" || s.HomeSusp[1].ID != 0 || s.NextSusp != 4 {
		t.Errorf("a damaged file loaded as %+v", s)
	}
}

// The state is primed with the restored scorer's frame: its suspensions are
// on air at once and the timeline is told nothing.
func TestPrimeFromTheRestoredScorer(t *testing.T) {
	sc := source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Handball }})
	for _, a := range []source.Action{{Do: "new_game"}, {Do: "suspension", Team: "home"}, {Do: "suspension", Team: "home"}} {
		if err := sc.Do(a); err != nil {
			t.Fatal(err)
		}
	}
	m := state.New(func() time.Time { return time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC) })
	var told []string
	m.OnEvent = func(e string, _ map[string]any) { told = append(told, e) }
	m.Prime(state.LiveOf(sc.Frame()))
	m.SetGame(state.Game{Sport: "handball", Rules: sport.Handball}, true)
	if got := m.Snapshot().Live.Suspensions; len(got) != 2 {
		t.Errorf("live suspensions %+v", got)
	}
	if len(told) != 0 {
		t.Errorf("the timeline was told %v", told)
	}
}
