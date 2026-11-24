package provider

import (
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

type fake struct{}

func (fake) ID() string                              { return "fpb" }
func (fake) Fixtures(store.Group) ([]Fixture, error) { return nil, nil }

func TestForOffersTheFPBOnlyToPortugueseBasketball(t *testing.T) {
	for _, c := range []struct {
		club store.Club
		want int
	}{
		{store.Club{Sport: "basketball", Country: "PT"}, 2},
		{store.Club{}, 2}, // an old settings file: the defaults
		{store.Club{Sport: "futsal", Country: "PT"}, 1},
		{store.Club{Sport: "basketball", Country: "ES"}, 1},
	} {
		got := For(c.club, fake{})
		if len(got) != c.want || got[len(got)-1].ID() != "manual" {
			t.Errorf("%+v: %d providers", c.club, len(got))
		}
	}
	if got := For(store.Club{Sport: "basketball", Country: "PT"}, nil); len(got) != 1 {
		t.Errorf("no FPB client: %d providers", len(got))
	}
}

func TestManualFixtures(t *testing.T) {
	g := store.Group{Fixtures: []store.Fixture{
		{ID: "a", Start: "2026-12-20T18:30:00Z", Opponent: store.Opponent{Name: "Ribeira", Short: "RIB", Color: "#0B6E4F", CrestURL: "/media/opp-x.png"}, Home: true, Round: "7"},
		{ID: "b", Start: "no date"},
	}}
	got, err := Manual{}.Fixtures(g)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	f := got[0]
	if f.Ref != "manual/a" || !f.Timed || !f.AtHome || f.Opponent.FullName != "Ribeira" || f.Opponent.CrestURL != "/media/opp-x.png" || f.Round != "7" {
		t.Errorf("%+v", f)
	}
}
