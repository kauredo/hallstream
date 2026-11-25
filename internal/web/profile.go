package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/store"
)

// The longest club name and sigla the overlay has room for.
const clubNameMax, clubShortMax = 12, 4

// checkClub tidies the club sent to PUT /api/settings, or says what is wrong
// with it in words for the volunteer.
func checkClub(c *store.Club) error {
	c.Name = strings.Join(strings.Fields(c.Name), " ")
	c.Short = strings.ToUpper(strings.TrimSpace(c.Short))
	c.Country = strings.ToUpper(strings.TrimSpace(c.Country))
	switch n := len([]rune(c.Name)); {
	case n == 0:
		return userError(i18n.T("api.clube.escreva_nome"))
	case n > clubNameMax:
		return userError(i18n.T("api.clube.nome_12"))
	}
	if n := len([]rune(c.Short)); n == 0 || n > clubShortMax {
		return userError(i18n.T("api.clube.sigla"))
	}
	var ok bool
	if c.Primary, ok = club.Hex(c.Primary); !ok {
		return userError(i18n.T("api.clube.cor_principal"))
	}
	if c.Secondary, ok = club.Hex(c.Secondary); !ok {
		return userError(i18n.T("api.clube.cor_secundaria"))
	}
	if sp, found := club.SportOf(c.Sport); !found || !sp.Active {
		return userError(i18n.T("api.clube.so_basquetebol"))
	}
	if len(c.Country) != 2 || strings.Trim(c.Country, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return userError(i18n.T("api.clube.pais"))
	}
	c.TimeZone = strings.TrimSpace(c.TimeZone)
	if c.TimeZone != "" {
		if _, err := time.LoadLocation(c.TimeZone); err != nil || c.TimeZone == "Local" {
			return userError(i18n.T("api.clube.fuso", "fuso", c.TimeZone))
		}
	}
	return nil
}

// clubCrest saves an uploaded crest as the club's.
func (s *Server) clubCrest(w http.ResponseWriter, r *http.Request) {
	data, ext, okf := upload(w, r)
	if !okf {
		return
	}
	u, err := s.Store.SaveMedia("club-"+store.NewID()+ext, data)
	if !s.save(w, err) {
		return
	}
	s.setCrest(w, u)
}

// setCrest makes u the club's crest and deletes the old upload.
func (s *Server) setCrest(w http.ResponseWriter, u string) {
	var old string
	set, err := s.Store.UpdateSettings(func(v *store.Settings) { old, v.Club.Crest = v.Club.Crest, u })
	if !s.save(w, err) {
		s.Store.RemoveMedia(u)
		return
	}
	if old != u {
		s.Store.RemoveMedia(old) // leaves /assets alone
	}
	reply(w, set.Club)
}
