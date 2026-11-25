package web

import (
	"fmt"
	"strings"

	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/provider"
	"github.com/kauredo/hallstream/internal/store"
)

// fpbGames is the FPB as a provider: the next games of the group's linked team.
type fpbGames struct{ c *fpb.Client }

func (fpbGames) ID() string { return "fpb" }

func (p fpbGames) Fixtures(g store.Group) ([]provider.Fixture, error) {
	if g.FPBTeamID == 0 || p.c == nil {
		return nil, nil
	}
	list, err := p.c.NextGames(g.FPBTeamID)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Fixture, 0, len(list.Games))
	for _, x := range list.Games {
		opp, home := x.Away, true
		if x.Away.ID == g.FPBTeamID {
			opp, home = x.Home, false
		}
		f := provider.Fixture{
			Ref: fmt.Sprintf("fpb/%d", x.ID), AtHome: home,
			Venue: tidyPlace(x.Location), Competition: x.CompetitionName, Round: x.PhaseName,
			Opponent: provider.Side{Name: opponentName(opp.Name, opp.ShortName), CrestKey: fmt.Sprintf("opp-%d", opp.ID)},
		}
		f.Start, f.Timed = fpbStart(x.Date, x.Time)
		if x.GameWeek != "" {
			f.Round = i18n.T("jogo.jornada", "n", string(x.GameWeek))
		}
		if r := []rune(strings.TrimSpace(opp.ShortName)); len(r) > 0 && len(r) <= 4 {
			f.Opponent.Short = strings.ToUpper(string(r))
		}
		f.Opponent.CrestURL = opp.LogoURL
		if f.Opponent.CrestURL == "" {
			f.Opponent.CrestURL = opp.ClubLogoURL
		}
		f.Opponent.FullName = strings.Join(strings.Fields(opp.Name), " ")
		out = append(out, f)
	}
	return out, nil
}

// providers are the sources of the club's games, FPB first where it covers the
// club, then the games typed or imported, then the escalão's calendar link.
func (s *Server) providers(g store.Group) []provider.Provider {
	var f provider.Provider
	if s.FPB != nil {
		f = fpbGames{s.FPB}
	}
	return append(provider.For(f, s.fpbFor(g)), s.calendar())
}

var noFPB error = i18n.Key("api.fpb.nao_disponivel")

// fpbOffered reports whether the FPB covers this club (Portuguese, with a
// basketball team); every FPB route and job checks it.
func (s *Server) fpbOffered() bool {
	return provider.FPBOffered(s.Store.Settings().Club, s.Store.Groups())
}

// fpbFor reports whether the FPB covers this escalão: a basketball one.
func (s *Server) fpbFor(g store.Group) bool {
	return provider.FPBOfferedFor(s.Store.Settings().Club, g)
}
