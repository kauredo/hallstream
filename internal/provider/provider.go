// Package provider is where a club's games come from: the Portuguese
// basketball federation (FPB) for the clubs it covers, and the games the club
// typed or imported itself, for every club. More sources can join here (see
// docs/research/team-data-sources.md); each one turns its own data into the
// neutral Fixture the next-game code reads.
package provider

import (
	"time"

	"github.com/kauredo/hallstream/internal/store"
)

// Fixture is one game of an escalão, from any provider.
type Fixture struct {
	// Ref is unique across providers: "fpb/123", "manual/<id>".
	Ref string
	// Start is the tip-off; Timed is false when the source has no time yet,
	// and then the game can't be placed in time.
	Start       time.Time
	Timed       bool
	AtHome      bool // the club plays at home
	Opponent    Side
	Venue       string
	Competition string
	Round       string // the round as shown ("Jornada 7"), "" for none
}

// Side is the other team of a fixture.
type Side struct {
	Name, Short string
	FullName    string // the name as the source writes it, for the next-game card
	Color       string // "#RRGGBB", or "" to pick it from the crest
	// CrestURL is a /media copy (typed fixtures) or a remote address the
	// server downloads once and keeps as CrestKey in dados/media.
	CrestURL string
	CrestKey string
}

// Provider lists an escalão's games. A provider that has nothing for the
// group (an FPB provider and a group with no FPB team) returns nil, nil.
type Provider interface {
	ID() string
	Fixtures(g store.Group) ([]Fixture, error)
}

// FPBOffered reports whether the FPB covers the club: Portuguese basketball.
func FPBOffered(c store.Club) bool {
	return (c.Country == "" || c.Country == "PT") && (c.Sport == "" || c.Sport == "basketball")
}

// For lists the providers for the club, the club's own games last.
func For(c store.Club, fpb Provider) []Provider {
	if fpb != nil && FPBOffered(c) {
		return []Provider{fpb, Manual{}}
	}
	return []Provider{Manual{}}
}

// Manual is the games the club entered itself (store.Group.Fixtures).
type Manual struct{}

func (Manual) ID() string { return "manual" }

func (Manual) Fixtures(g store.Group) ([]Fixture, error) {
	var out []Fixture
	for _, f := range g.Fixtures {
		t, err := time.Parse(time.RFC3339, f.Start)
		if err != nil {
			continue
		}
		out = append(out, Fixture{
			Ref: "manual/" + f.ID, Start: t, Timed: true, AtHome: f.Home,
			Opponent: Side{Name: f.Opponent.Name, FullName: f.Opponent.Name, Short: f.Opponent.Short, Color: f.Opponent.Color, CrestURL: f.Opponent.CrestURL},
			Venue:    f.Venue, Competition: f.Competition, Round: f.Round,
		})
	}
	return out, nil
}
