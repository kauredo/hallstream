package state

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/sport"
)

// The demo of every sport but basketball is one scripted game, played
// through the same keys a source sends (the manual scorer's frame), so the
// state machine and the overlay take it the way they take a real game.

// demoFinalHold is how many ticks a finished game stays up, long enough for
// the final screen (a soccer or volleyball game has no hold of its own).
const demoFinalHold = 700

// playSport runs demo games of p until hold says stop; hold(n) waits n ticks.
func playSport(p sport.Profile, show func(Live), hold func(int) bool, newGame func()) {
	for {
		newGame()
		var ok bool
		if p.Sets() {
			ok = playSets(p, show, hold)
		} else {
			ok = playTimed(p, show, hold)
		}
		if !ok {
			return
		}
	}
}

// demoEv is one thing that happens a number of seconds into a half. kind is
// "foul", "timeout", "susp" (a is the shirt number), "card" (a number, b kind),
// "sub" (a out, b in) or "stoppage" (a minutes of added time).
type demoEv struct {
	at         int
	kind, team string
	a, b       string
}

// demoScript is a timed game: the goals of each half ("H" the club, "A" the
// visitors, spread evenly over the half), the other events, how many game
// seconds go by per tick and the added seconds the halves run on.
type demoScript struct {
	goals  [2]string
	events [2][]demoEv
	step   int
	extra  int
}

var demoScripts = map[string]demoScript{
	sport.Futsal.ID: {
		goals: [2]string{"HAH", "HAHHH"}, step: 2,
		events: [2][]demoEv{
			{{100, "foul", "away", "", ""}, {260, "foul", "away", "", ""}, {330, "foul", "home", "", ""},
				{400, "foul", "away", "", ""}, {520, "foul", "away", "", ""}, {600, "timeout", "home", "", ""},
				{640, "foul", "away", "", ""}, {700, "foul", "home", "", ""}, {780, "foul", "away", "", ""},
				{900, "foul", "away", "", ""}},
			{{180, "foul", "away", "", ""}, {260, "foul", "home", "", ""}, {300, "foul", "away", "", ""},
				{420, "foul", "home", "", ""}, {500, "timeout", "away", "", ""}, {560, "foul", "home", "", ""},
				{700, "foul", "home", "", ""}, {840, "foul", "home", "", ""}, {990, "foul", "home", "", ""}},
		},
	},
	sport.Handball.ID: {
		goals: [2]string{"HAHHAHAHHAHA", "AHHAHAHHAHHA"}, step: 3,
		events: [2][]demoEv{
			{{400, "susp", "home", "7", ""}, {900, "susp", "away", "11", ""}, {1300, "timeout", "away", "", ""}},
			{{300, "susp", "away", "4", ""}, {340, "susp", "home", "9", ""}, {1000, "timeout", "home", "", ""}},
		},
	},
	sport.Soccer.ID: {
		goals: [2]string{"HA", "HH"}, step: 5, extra: 180,
		events: [2][]demoEv{
			{{1330, "card", "home", "5", "yellow"}, {2100, "card", "away", "8", "yellow"}, {2580, "stoppage", "", "3", ""}},
			{{600, "sub", "home", "10", "14"}, {900, "sub", "away", "7", "17"}, {1320, "card", "away", "9", "red"},
				{2580, "stoppage", "", "3", ""}},
		},
	},
}

// demoMinute is a card's or substitution's minute as the scorer writes it:
// the minute being played up to the period's end, "45+2'" after it.
func demoMinute(r sport.Profile, period, clock int) string {
	end := r.End(period, r.Length)
	if clock <= end {
		return strconv.Itoa(min(clock/600+1, end/600)) + "'"
	}
	return fmt.Sprintf("%d+%d'", end/600, (clock-end+599)/600)
}

type demoSusp struct {
	team, number string
	id, end      int
}

// demoBoard is what the scorer would hold at one moment of the game.
type demoBoard struct {
	home, away, homeFouls, awayFouls, homeTO, awayTO int
	stoppage                                         int
	over                                             bool
	susp                                             []demoSusp
	cards                                            []Card
	subs                                             []Sub
	nextSusp, nextCard, nextSub                      int
}

func (b *demoBoard) apply(p sport.Profile, e demoEv, period, clock int) {
	home := e.team == "home"
	switch e.kind {
	case "foul":
		if home {
			b.homeFouls++
		} else {
			b.awayFouls++
		}
	case "timeout":
		if home {
			b.homeTO++
		} else {
			b.awayTO++
		}
	case "susp":
		b.nextSusp++
		b.susp = append(b.susp, demoSusp{e.team, e.a, b.nextSusp, clock + p.Suspensions})
	case "card":
		b.nextCard++
		b.cards = append(b.cards, Card{ID: b.nextCard, Team: e.team, Number: e.a, Kind: e.b, Minute: demoMinute(p, period, clock)})
	case "sub":
		b.nextSub++
		b.subs = append(b.subs, Sub{ID: b.nextSub, Team: e.team, Out: e.a, In: e.b, Minute: demoMinute(p, period, clock)})
	case "stoppage":
		b.stoppage, _ = strconv.Atoi(e.a)
	}
}

// frame is the board as the scorer's frame carries it, read back the way the
// state reads a source's.
func (b *demoBoard) frame(p sport.Profile, period, clock int) Live {
	n := strconv.Itoa
	text := ClockText(clock)
	end := ""
	if p.CountUp() {
		text, end = ClockTextUp(clock), n(p.End(period, p.Length))
	}
	susp := []Suspension{}
	for _, s := range b.susp {
		if s.end > clock {
			susp = append(susp, Suspension{Team: s.team, ID: s.id, Number: s.number, Left: ClockTextUp((s.end - clock + 9) / 10 * 10)})
		}
	}
	list := func(v any) string { j, _ := json.Marshal(v); return string(j) }
	v := map[string]string{
		"home_name": i18n.T("jogo.casa"), "away_name": i18n.T("jogo.visitante"), "sport": p.ID,
		"home": n(b.home), "away": n(b.away), "clock": text, "period": n(period), "period_end": end,
		"home_fouls": n(b.homeFouls), "away_fouls": n(b.awayFouls),
		"home_timeouts": n(b.homeTO), "away_timeouts": n(b.awayTO),
		"shot_clock": "", "suspensions": list(susp),
	}
	if p.Cards {
		v["over"] = ""
		if b.over {
			v["over"] = "1"
		}
		v["stoppage"] = n(b.stoppage)
		v["cards"], v["subs"] = list(append([]Card{}, b.cards...)), list(append([]Sub{}, b.subs...))
	}
	return LiveOf(v)
}

// playTimed plays one game of a clock sport (futsal, handball, soccer) and
// reports false if hold stopped it.
func playTimed(p sport.Profile, show func(Live), hold func(int) bool) bool {
	sc := demoScripts[p.ID]
	var b demoBoard
	for half := 1; half <= p.Periods; half++ {
		if p.FoulsReset {
			b.homeFouls, b.awayFouls = 0, 0
		}
		if p.ResetsTimeouts(half) {
			b.homeTO, b.awayTO = 0, 0
		}
		b.stoppage = 0
		regular := p.Length / 10
		dur := regular + sc.extra
		// Seconds into the half to tenths on the clock.
		clock := func(s int) int {
			if p.CountUp() {
				return p.Start(half, p.Length) + s*10
			}
			return p.Length - s*10
		}
		var evs []demoEv
		for i, c := range sc.goals[half-1] {
			team := "away"
			if c == 'H' {
				team = "home"
			}
			evs = append(evs, demoEv{at: regular * (i + 1) / (len(sc.goals[half-1]) + 1), kind: "goal", team: team})
		}
		evs = append(evs, sc.events[half-1]...)
		show(b.frame(p, half, clock(0)))
		if !hold(30) {
			return false
		}
		for prev, s := 0, 0; s < dur; prev = s {
			s = min(s+sc.step, dur)
			for _, e := range evs {
				if e.at <= prev || e.at > s {
					continue
				}
				switch {
				case e.kind == "goal" && e.team == "home":
					b.home++
				case e.kind == "goal":
					b.away++
				default:
					b.apply(p, e, half, clock(e.at))
				}
			}
			show(b.frame(p, half, clock(s)))
			if !hold(1) {
				return false
			}
		}
		if half == p.Periods {
			break
		}
		if !hold(200) {
			return false
		}
	}
	if p.Cards {
		b.over = true
		show(b.frame(p, p.Periods, p.End(p.Periods, p.Length)+sc.extra*10))
	}
	// Long enough for the final screen, then a new game.
	return hold(demoFinalHold)
}

// The volleyball match: the points of each set, the club winning 3-1.
var demoSets = [][2]int{{25, 21}, {19, 25}, {25, 22}, {25, 23}}

// rallies is who scores each point of a set that ends h-a, the loser's points
// spread evenly and the winner's last.
func rallies(h, a int) []string {
	win, lose, wn, ln := "home", "away", h, a
	if a > h {
		win, lose, wn, ln = "away", "home", a, h
	}
	var out []string
	got := 0
	for n := 1; n < wn+ln; n++ {
		if got < (n*ln+(wn+ln-2))/(wn+ln-1) && got < ln {
			out, got = append(out, lose), got+1
		} else {
			out = append(out, win)
		}
	}
	return append(out, win)
}

// playSets plays one volleyball match, 3-1, and reports false if hold stopped it.
func playSets(p sport.Profile, show func(Live), hold func(int) bool) bool {
	n := strconv.Itoa
	var done []string
	hs, as := 0, 0
	frame := func(set, home, away int, serve string, ht, at int) Live {
		return LiveOf(map[string]string{
			"home_name": i18n.T("jogo.casa"), "away_name": i18n.T("jogo.visitante"), "sport": p.ID,
			"home": n(home), "away": n(away), "clock": "", "period": n(set),
			"home_timeouts": n(ht), "away_timeouts": n(at),
			"home_sets": n(hs), "away_sets": n(as), "serve": serve,
			"sets": strings.Join(done, ","), "shot_clock": "", "period_end": "", "suspensions": "[]",
		})
	}
	serve := "home"
	for i, set := range demoSets {
		home, away, ht, at := 0, 0, 0, 0
		show(frame(i+1, 0, 0, serve, 0, 0))
		if !hold(30) {
			return false
		}
		for k, side := range rallies(set[0], set[1]) {
			if side == "home" {
				home++
			} else {
				away++
			}
			serve = side
			if k == 14 {
				if i%2 == 0 {
					ht++
				} else {
					at++
				}
			}
			show(frame(i+1, home, away, serve, ht, at))
			if !hold(10) {
				return false
			}
		}
		letter := "h"
		if away > home {
			letter = "a"
			as++
		} else {
			hs++
		}
		done = append(done, fmt.Sprintf("%d-%d%s", home, away, letter))
		show(frame(i+1, home, away, serve, ht, at))
		if i == len(demoSets)-1 {
			break
		}
		if !hold(50) {
			return false
		}
		serve = "away"
		if i%2 == 1 {
			serve = "home"
		}
	}
	return hold(demoFinalHold)
}
