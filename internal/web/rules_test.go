package web

import (
	"testing"

	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
)

func TestStateCarriesTheSportRules(t *testing.T) {
	for _, id := range []string{"basketball", ""} {
		t.Run("club sport "+id, func(t *testing.T) {
			s := newServer(t)
			set := s.Store.Settings()
			set.Club.Sport = id
			if err := s.Store.SaveSettings(set); err != nil {
				t.Fatal(err)
			}
			s.SyncGame()
			var st state.State
			call(t, s, "GET", "/api/state", nil, &st)
			if st.Game.Rules != sport.Basketball {
				t.Errorf("rules %+v, want basketball", st.Game.Rules)
			}
			if r := st.Game.Rules; r.Periods != 4 || r.Half != 2 || r.TimeoutsBy != [3]int{2, 3, 1} {
				t.Errorf("rules %+v", r)
			}
		})
	}
}
