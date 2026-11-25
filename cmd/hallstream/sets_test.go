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

func volleyballMachine() *state.Machine {
	m := state.New(time.Now)
	m.SetGame(state.Game{Sport: "volleyball"}, false)
	return m
}

// The sets a game recorded come back after a restart, and the restart does
// not tell the timeline about them again.
func TestSetsSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := volleyballMachine()
	restorePeriods(st, first)
	restoreSets(st, first)
	first.ApplyConsole(map[string]string{"home": "0", "away": "0", "home_sets": "1", "away_sets": "0", "sets": "25-20h", "period": "2"})
	first.ApplyConsole(map[string]string{"home": "0", "away": "0", "home_sets": "1", "away_sets": "1", "sets": "25-20h,18-25a", "period": "3"})
	want := []state.SetScore{{Set: 1, Home: 25, Away: 20, Winner: "home"}, {Set: 2, Home: 18, Away: 25, Winner: "away"}}
	if got := first.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Fatalf("before the restart: %v", got)
	}

	st2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	second := volleyballMachine()
	var events []string
	second.OnEvent = func(ev string, _ map[string]any) { events = append(events, ev) }
	restorePeriods(st2, second)
	restoreSets(st2, second)
	if got := second.Snapshot().Sets; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the restart: %v, want %v", got, want)
	}
	second.ApplyConsole(map[string]string{"home": "5", "away": "3", "home_sets": "1", "away_sets": "1", "sets": "25-20h,18-25a", "period": "3"})
	for _, e := range events {
		if e == "set" {
			t.Errorf("the restart told the timeline about a set again: %v", events)
		}
	}
}

// manual.json keeps the sets and the serve, and one saved before volleyball
// (no such keys) still loads.
func TestManualFileHoldsSetsAndServe(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	played := source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Volleyball }})
	for i := 0; i < 25; i++ {
		if err := played.Do(source.Action{Do: "point", Team: "away", N: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := played.Do(source.Action{Do: "set_next"}); err != nil {
		t.Fatal(err)
	}
	if err := played.Do(source.Action{Do: "serve", Team: "home"}); err != nil {
		t.Fatal(err)
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
	if _, ok := keys["sets"]; !ok {
		t.Error("manual.json has no sets key")
	}
	if string(keys["serve"]) != `"home"` {
		t.Errorf("serve key %s", keys["serve"])
	}
	back := source.NewManual(&source.Manual{Rules: func() sport.Profile { return sport.Volleyball }})
	restoreManual(st, back)
	if got, want := back.Score(), played.Score(); got != want {
		t.Errorf("restored %+v, want %+v", got, want)
	}

	old := `{"home":7,"away":3,"period":2,"clock":4000,"running":false,"length":6000,"home_fouls":1,"away_fouls":2,"home_timeouts":0,"away_timeouts":1}`
	if err := os.WriteFile(filepath.Join(dir, "manual.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	basket := source.NewManual(&source.Manual{})
	restoreManual(st, basket)
	if s := basket.Score(); s.Home != 7 || s.Period != 2 || s.Serve != "" || s.Sets != [5]state.SetScore{} {
		t.Errorf("an old manual.json loaded as %+v", s)
	}
}
