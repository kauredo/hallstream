package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
	"rsc.io/qr"
)

var (
	noManual error = i18n.Key("api.marcar.sem_marcacao")
	noTake   error = i18n.Key("api.marcar.consola_no_ar")
)

// marcarRoutes are the scorer's page (/marcar) and its API. The page is
// public like the panel; /api/marcar needs the PIN from another device.
func (s *Server) marcarRoutes(m *http.ServeMux, page func(string) http.HandlerFunc) {
	m.HandleFunc("GET /marcar", page("marcar/index.html"))
	m.HandleFunc("GET /marcar/qr.svg", s.marcarQR)
	m.HandleFunc("GET /api/marcar", s.marcarScore)
	m.HandleFunc("POST /api/marcar", s.marcarDo)
}

// marcarScore is the score kept by hand; an open scorer's page calls it
// every few seconds, which keeps the manual input "connected".
func (s *Server) marcarScore(w http.ResponseWriter, r *http.Request) {
	if s.Manual == nil {
		fail(w, http.StatusServiceUnavailable, noManual.Error())
		return
	}
	in := s.Store.Settings().Input
	if in == "manual" {
		s.Manual.Heartbeat()
	}
	s.marcarReply(w, in)
}

// marcarReply is the input, the manual score and the action undo would take back.
func (s *Server) marcarReply(w http.ResponseWriter, input string) {
	var undo *source.Action
	if a, ok := s.Manual.Undo(); ok {
		undo = &a
	}
	reply(w, map[string]any{"input": input, "score": s.Manual.Score(), "undo": undo})
}

// marcarDo applies one tap. "take" makes the manual input the one on air,
// starting from the score showing (and the clock running if the console's
// was); any other action needs the manual input on air already, so a stray
// tap never takes the scoring from a working console.
func (s *Server) marcarDo(w http.ResponseWriter, r *http.Request) {
	if s.Manual == nil {
		fail(w, http.StatusServiceUnavailable, noManual.Error())
		return
	}
	var a source.Action
	if !decode(w, r, &a) {
		return
	}
	manual := s.Store.Settings().Input == "manual"
	if a.Do == "take" {
		if !manual {
			running := s.State.ClockRanWithin(2 * time.Second)
			if _, err := s.Store.UpdateSettings(func(v *store.Settings) { v.Input = "manual" }); !s.save(w, err) {
				return
			}
			if s.UseInput != nil {
				s.UseInput("manual")
			}
			if running {
				s.Manual.Do(source.Action{Do: "clock_start"})
			}
			s.Log(i18n.T("log.marcar.manual"))
		}
		s.Manual.Heartbeat()
		s.marcarReply(w, "manual")
		return
	}
	if !manual {
		fail(w, http.StatusConflict, noTake.Error())
		return
	}
	s.Manual.Heartbeat()
	if err := s.Manual.Do(a); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.Do == "card" || a.Do == "sub" {
		s.showPlay(a.Do)
	}
	s.marcarReply(w, "manual")
}

// playSeconds is how long a card's or a substitution's lower third stays up.
const playSeconds = 8

// showPlay puts the card or substitution the scorer just entered on air as a
// lower third. A home player is named from the game's squad; the opponent's
// squad is not known, so an away player shows the shirt number only. Label is
// the team's name, which the overlay shows when there is no player's name.
func (s *Server) showPlay(do string) {
	score := s.Manual.Score()
	game := s.State.Snapshot().Game
	squad := game.Squad
	teamName := func(team string) string {
		if team == "home" {
			return game.Home.Name
		}
		return game.Away.Name
	}
	who := func(team, number string) (first, last, photo string) {
		if team != "home" {
			return
		}
		for _, p := range squad {
			if strings.TrimLeft(p.Number, "0") == strings.TrimLeft(number, "0") {
				first, last, _ = strings.Cut(strings.TrimSpace(p.Name), " ")
				return first, strings.TrimSpace(last), p.PhotoURL
			}
		}
		return
	}
	var lt state.LowerThird
	switch {
	case do == "card" && len(score.Cards) > 0:
		c := score.Cards[len(score.Cards)-1]
		first, last, photo := who(c.Team, c.Number)
		lt = state.LowerThird{Type: "cartao", Card: c.Kind, Number: c.Number, FirstName: first, LastName: last, PhotoURL: photo, Label: teamName(c.Team)}
	case do == "sub" && len(score.Subs) > 0:
		x := score.Subs[len(score.Subs)-1]
		first, last, photo := who(x.Team, x.In)
		outFirst, outLast, _ := who(x.Team, x.Out)
		lt = state.LowerThird{Type: "substituicao", Number: x.In, FirstName: first, LastName: last, PhotoURL: photo,
			OutNumber: x.Out, OutName: strings.TrimSpace(outFirst + " " + outLast), Label: teamName(x.Team)}
	default:
		return
	}
	s.State.ShowLowerThird(lt, playSeconds*time.Second)
}

// marcarQR draws the scorer page's address on the club's network, for a
// phone to scan off the panel; 404 when the PC has no network address.
func (s *Server) marcarQR(w http.ResponseWriter, r *http.Request) {
	ip := LanIP()
	if ip == "" {
		http.NotFound(w, r)
		return
	}
	code, err := qr.Encode(fmt.Sprintf("http://%s:%d/marcar", ip, s.Port), qr.M)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(qrSVG(code))
}
