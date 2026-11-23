package web

import (
	"net/http"
	"strings"

	"github.com/kauredo/hallstream/internal/club"
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
		return userError("Escreva o nome do clube.")
	case n > clubNameMax:
		return userError("O nome do clube tem de caber em 12 letras.")
	}
	if n := len([]rune(c.Short)); n == 0 || n > clubShortMax {
		return userError("A sigla tem de ter entre 1 e 4 letras.")
	}
	var ok bool
	if c.Primary, ok = club.Hex(c.Primary); !ok {
		return userError("A cor principal tem de ser uma cor como #2F4B7C.")
	}
	if c.Secondary, ok = club.Hex(c.Secondary); !ok {
		return userError("A cor secundária tem de ser uma cor como #1C1A17.")
	}
	if sp, found := club.SportOf(c.Sport); !found || !sp.Active {
		return userError("Por agora só há basquetebol. As outras modalidades chegam em breve.")
	}
	if len(c.Country) != 2 || strings.Trim(c.Country, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return userError("Escolha o país do clube.")
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
