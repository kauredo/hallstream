package web

import (
	"time"

	"github.com/kauredo/hallstream/internal/club"
)

// zone is the club's time zone (settings.club.timezone, else its country's).
func (s *Server) zone() *time.Location {
	c := s.Store.Settings().Club
	return club.Zone(c.TimeZone, c.Country)
}
