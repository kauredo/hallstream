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
		got := For(fake{}, FPBOfferedFor(c.club, store.Group{Sport: c.club.Sport}))
		if len(got) != c.want || got[len(got)-1].ID() != "manual" {
			t.Errorf("%+v: %d providers", c.club, len(got))
		}
	}
	if got := For(nil, true); len(got) != 1 {
		t.Errorf("no FPB client: %d providers", len(got))
	}
}

func TestFPBOfferedSportPerEscalao(t *testing.T) {
	bb, fut := store.Group{ID: "a", Sport: "basketball"}, store.Group{ID: "b", Sport: "futsal"}
	geral := store.Group{ID: store.Shared, Sport: "basketball"}
	for _, c := range []struct {
		name   string
		club   store.Club
		groups []store.Group
		want   bool
	}{
		{"PT basketball, no escalões", store.Club{Sport: "basketball", Country: "PT"}, nil, true},
		{"PT futsal, only geral", store.Club{Sport: "futsal", Country: "PT"}, []store.Group{geral}, false},
		{"PT futsal, futsal escalão", store.Club{Sport: "futsal", Country: "PT"}, []store.Group{fut}, false},
		{"PT futsal, a basketball escalão", store.Club{Sport: "futsal", Country: "PT"}, []store.Group{fut, bb}, true},
		{"ES basketball", store.Club{Sport: "basketball", Country: "ES"}, []store.Group{bb}, false},
	} {
		if got := FPBOffered(c.club, c.groups); got != c.want {
			t.Errorf("%s: FPBOffered = %v", c.name, got)
		}
	}
	pt, es := store.Club{Country: "PT"}, store.Club{Country: "ES"}
	for _, c := range []struct {
		name string
		club store.Club
		g    store.Group
		want bool
	}{
		{"PT basketball", pt, bb, true},
		{"PT no sport", pt, store.Group{}, true},
		{"PT futsal", pt, fut, false},
		{"ES basketball", es, bb, false},
	} {
		if got := FPBOfferedFor(c.club, c.g); got != c.want {
			t.Errorf("%s: FPBOfferedFor = %v", c.name, got)
		}
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
