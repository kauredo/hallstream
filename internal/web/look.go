package web

import (
	"net/http"
	"slices"

	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

// overlayState is the club's graphics setup as the overlay reads it.
func overlayState(o store.Overlay) state.Overlay {
	o.Clean()
	return state.Overlay{
		Style: o.Style, BugPosition: o.BugPosition, LowerThird: o.LowerThird,
		Margin: o.Margin, Scale: o.Scale, Accent: o.Accent, AwayColor: o.AwayColor,
		Hide: slices.Clone(o.Hide), Photo: o.Photo,
		SponsorCorner: o.SponsorCorner, SponsorEvery: o.SponsorEvery, BackLine: o.BackLine,
	}
}

// overlayPhoto takes the hall photo for the full screens.
func (s *Server) overlayPhoto(w http.ResponseWriter, r *http.Request) {
	data, ext, okf := upload(w, r)
	if !okf {
		return
	}
	u, err := s.Store.SaveMedia("hall-"+store.NewID()+ext, data)
	if !s.save(w, err) {
		return
	}
	s.setPhoto(w, u)
}

// setPhoto makes u the hall photo ("" for the stand-in) and deletes the old upload.
func (s *Server) setPhoto(w http.ResponseWriter, u string) {
	var old string
	set, err := s.Store.UpdateSettings(func(v *store.Settings) { old, v.Overlay.Photo = v.Overlay.Photo, u })
	if !s.save(w, err) {
		s.Store.RemoveMedia(u)
		return
	}
	if old != u {
		s.Store.RemoveMedia(old)
	}
	reply(w, set.Overlay)
}
