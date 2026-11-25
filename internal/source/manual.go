package source

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
)

// Score is the game the scorer keeps by hand, as saved in dados/manual.json.
type Score struct {
	Home         int  `json:"home"`
	Away         int  `json:"away"`
	Period       int  `json:"period"`  // 1 and up; 5 is the first overtime
	Clock        int  `json:"clock"`   // tenths on the clock: left in the period for a countdown, played in the game for a count-up
	Running      bool `json:"running"` // the clock is running
	Length       int  `json:"length"`  // tenths of a second in a period
	HomeFouls    int  `json:"home_fouls"`
	AwayFouls    int  `json:"away_fouls"`
	HomeTimeouts int  `json:"home_timeouts"`
	AwayTimeouts int  `json:"away_timeouts"`

	// Handball. Fixed arrays keep a Score comparable with ==; three
	// suspensions per team run at most. NextSusp is the last ID given out.
	HomeSusp [maxSusp]Suspension `json:"home_susp"`
	AwaySusp [maxSusp]Suspension `json:"away_susp"`
	NextSusp int                 `json:"next_susp"`

	// Soccer. The lists stay nil until the first card or substitution, never
	// an empty slice, so sameScore (reflect.DeepEqual) can't tell two empty
	// games apart. Anything that keeps a Score for later clones them (cloned).
	Cards    []Card `json:"cards"`
	Subs     []Sub  `json:"subs"`
	Stoppage int    `json:"stoppage"` // minutes of added time announced for this period
	Over     bool   `json:"over"`     // the scorer ended the game

	// Volleyball. Sets is an array, not a slice, so a Score stays comparable
	// with ==; a played set has a Winner, an empty slot has none.
	Sets  [maxSets]state.SetScore `json:"sets"`  // finished sets, in order
	Serve string                  `json:"serve"` // "home", "away" or ""
}

// Card is one card shown. ID is told apart from the others for the timeline
// and never reused in a run, not even after an undo (Manual.ids).
type Card struct {
	ID     int    `json:"id"`
	Team   string `json:"team"`   // "home" or "away"
	Number string `json:"number"` // shirt number
	Kind   string `json:"kind"`   // "yellow", "second_yellow" or "red"
	Clock  int    `json:"clock"`  // the game clock when it was entered, tenths
	Period int    `json:"period"` // the period it was entered in, for its minute
}

// Sub is one substitution.
type Sub struct {
	ID     int    `json:"id"`
	Team   string `json:"team"`
	Out    string `json:"out"` // shirt numbers
	In     string `json:"in"`
	Clock  int    `json:"clock"`
	Period int    `json:"period"`
}

// sameScore reports whether two scores are the same; == can't compare Score
// because of its lists.
func sameScore(a, b Score) bool { return reflect.DeepEqual(a, b) }

// cloned is s with its own copies of the lists, so an append to one score never
// writes into another's backing array. A nil list stays nil.
func (s Score) cloned() Score {
	s.Cards, s.Subs = slices.Clone(s.Cards), slices.Clone(s.Subs)
	return s
}

// maxSets is the most sets a match can have (best of 5).
const maxSets = 5

// maxSusp is the most suspensions a team can have running.
const maxSusp = 3

// maxSuspID bounds the IDs a saved score may hold.
const maxSuspID = 1_000_000

// Suspension is one running suspension; an empty slot has ID 0.
type Suspension struct {
	ID     int    `json:"id"`
	Number string `json:"number"` // shirt number, "" until the scorer adds it
	End    int    `json:"end"`    // the game clock (count-up, tenths) at which it ends
}

// sides are the two teams' suspension slots, home first.
func (s *Score) sides() [2]*[maxSusp]Suspension {
	return [2]*[maxSusp]Suspension{&s.HomeSusp, &s.AwaySusp}
}

// expire empties every suspension slot that has run out at the clock.
func (s *Score) expire() {
	for _, side := range s.sides() {
		for i, x := range side {
			if x.ID != 0 && x.End <= s.Clock {
				side[i] = Suspension{}
			}
		}
	}
}

// played is how many sets are finished, and how many each side won.
func (s Score) played() (n, home, away int) {
	for _, x := range s.Sets {
		switch x.Winner {
		case "home":
			home++
		case "away":
			away++
		default:
			continue
		}
		n++
	}
	return
}

const (
	// DefaultLength is a basketball period, 10:00.
	DefaultLength = 10 * 60 * 10
	// maxStoppage is the most added minutes a half can be given.
	maxStoppage = 15
	// MaxClock is the longest time the clock can show, 99:59.
	MaxClock = (99*60 + 59) * 10
	// Seen is how long the scorer's phone counts as connected after it last called in.
	Seen = 60 * time.Second
	// undoMax is how many actions undo can take back.
	undoMax = 50
)

// newScoreAt is a new game with the clock at start: the length for a countdown, 0 for a count-up.
func newScoreAt(length, start int) Score { return Score{Period: 1, Clock: start, Length: length} }

// Action is one tap on the scorer's page. Do is one of:
//   - "point": N points (-3 to 3, not 0) for Team; never below 0
//   - "foul", "timeout": N (1 or -1) for Team; never below 0
//   - "undo": takes back the last action, up to 50, but never the clock's
//     start and stop, and only takes the clock back for an action that set it
//   - "serve": the serve to Team (volleyball)
//   - "set_next": ends the set (volleyball): refused until the points win it;
//     the score goes into the finished sets, the points and timeouts to 0 and
//     the serve to nobody; refused once a team has won the match
//   - "clock_start", "clock_stop": a count-up clock that stops at the end of
//     its period (handball) does not start at the end
//   - "clock_set": the clock to Tenths (0 to MaxClock); a running clock keeps
//     running; a count-up clock is kept inside the current period
//   - "clock_add": Tenths more (or less, below 0) on the clock, kept between 0
//     and MaxClock (a count-up: inside the current period)
//   - "period_next": the next period, with the clock stopped at the period
//     length (an overtime runs the sport's OvertimeLength when it sets one; a
//     count-up clock starts at the time already played)
//     and the team fouls at 0 when the sport resets them for that period (futsal
//     carries them into extra time); the timeouts go back to 0 when the
//     sport's profile says so (basketball: the second half and each overtime)
//   - "period_back": the period before, the clock stopped where it is (a
//     count-up clock: at the end of the period before)
//   - "period_length": the period length to Tenths (above 0); a clock
//     stopped at the old length moves to the new one
//   - "new_game": 0-0 in period 1, the clock at the period length (a count-up: 0)
//   - "suspension": starts the sport's suspension for Team, in its first
//     free slot (three at most); it runs on the game clock, so it stops with it
//   - "suspension_number": the shirt Number (1 to 3 digits, or "" to clear) of suspension ID
//   - "suspension_end": ends suspension ID early
//   - "card": Kind "yellow" or "red" for the player with shirt Number (1 to 3
//     digits) of Team; a yellow to a player who has one is stored as
//     "second_yellow"; refused for a player already sent off
//   - "sub": Out and In (shirt numbers 1 to 99, different) for Team
//   - "stoppage": N (0 to 15) minutes of added time for this period
//   - "game_end": the game is over (last period only); the clock stops, and
//     starting it again or going back a period takes it back
//
// card, sub, stoppage and game_end are for a sport with cards (soccer), which
// also has no extra time, so period_next past its last period is refused.
//
// A sets sport (volleyball) takes only +1 or -1 point, which also gives the
// serve to the team that scored; it refuses fouls, the clock and the period
// actions.
type Action struct {
	Do     string `json:"do"`
	Team   string `json:"team"` // "home" or "away"
	N      int    `json:"n"`
	Tenths int    `json:"tenths"`
	ID     int    `json:"id"`     // a suspension's ID
	Number string `json:"number"` // a shirt number: a suspension's or a card's
	Kind   string `json:"kind"`   // a card's colour
	Out    string `json:"out"`    // a substitution's shirt numbers
	In     string `json:"in"`
}

var (
	errTeam   error = i18n.Key("manual.equipa")
	errPoints error = i18n.Key("manual.pontos")
	errOne    error = i18n.Key("manual.um_de_cada_vez")
	errClock  error = i18n.Key("manual.tempo")
	errUndo   error = i18n.Key("manual.nada_para_desfazer")

	errNoFouls  error = i18n.Key("manual.sem_faltas")
	errOnePoint error = i18n.Key("manual.um_ponto")
	errSetOpen  error = i18n.Key("manual.set_por_acabar")
	errGameOver error = i18n.Key("manual.jogo_acabou")
	errNoClock  error = i18n.Key("manual.sem_relogio")

	errExclusaoFull   error = i18n.Key("manual.exclusao_cheia")
	errExclusaoSport  error = i18n.Key("manual.exclusao_modalidade")
	errExclusaoGone   error = i18n.Key("manual.exclusao_desconhecida")
	errExclusaoNumber error = i18n.Key("manual.exclusao_numero")

	errNoCards  error = i18n.Key("manual.sem_cartoes")
	errCard     error = i18n.Key("manual.cartao")
	errSentOff  error = i18n.Key("manual.ja_expulso")
	errSub      error = i18n.Key("manual.substituicao")
	errStoppage error = i18n.Key("manual.descontos_tempo")
	errEndEarly error = i18n.Key("manual.fim_cedo")
	errNoExtra  error = i18n.Key("manual.sem_prolongamento")
)

// Manual is the score kept by hand on a phone (the /marcar page). The page
// calls Do for each tap and Heartbeat while it is open; the clock runs here,
// so the score stays right when the phone sleeps.
type Manual struct {
	Now  func() time.Time
	Save func(Score) error // dados/manual.json, after each change
	Log  func(string)
	// Rules is called under the scorer's lock, so it must not call back into Manual.
	Rules func() sport.Profile

	mu         sync.Mutex
	s          Score
	undo       []undone
	from       time.Time // when the running clock last started or was set
	fromClock  int       // the clock at from
	seen       time.Time // the phone's last call
	out        func(map[string]string)
	connected  func(bool)
	ids        int   // the last card or substitution ID given out; never goes back, not even on undo
	runs       int   // Run calls so far; the newest one owns out and connected
	sent       Score // the score out last got
	saveFailed bool
}

type undone struct {
	a     Action // what it takes back, for the scorer's "Anular …"
	s     Score
	clock bool // the action set the clock
}

// NewManual fills in the defaults and starts a new game.
func NewManual(m *Manual) *Manual {
	if m.Now == nil {
		m.Now = time.Now
	}
	if m.Save == nil {
		m.Save = func(Score) error { return nil }
	}
	if m.Log == nil {
		m.Log = func(string) {}
	}
	if m.Rules == nil {
		m.Rules = func() sport.Profile { return sport.Basketball }
	}
	m.s = newScoreAt(DefaultLength, DefaultLength)
	return m
}

func (m *Manual) Name() string { return "manual" }

// Restore puts back the score saved before a restart. The clock comes back
// stopped: the time the program was off does not count.
func (m *Manual) Restore(s Score) {
	if s.Length <= 0 || s.Length > MaxClock {
		s.Length = DefaultLength
	}
	s.Period = max(s.Period, 1)
	s.Clock = min(max(s.Clock, 0), MaxClock)
	s.Running = false
	// A saved file can be edited or damaged: keep only slots that make sense.
	highest := 0
	for _, side := range s.sides() {
		for i, x := range side {
			if x.ID <= 0 || x.ID > maxSuspID {
				side[i] = Suspension{}
				continue
			}
			if !validNumber(x.Number) {
				side[i].Number = ""
			}
			side[i].End = min(max(x.End, 0), 2*MaxClock)
			highest = max(highest, x.ID)
		}
	}
	s.NextSusp = min(max(s.NextSusp, highest), maxSuspID)
	s.Stoppage = min(max(s.Stoppage, 0), maxStoppage)
	ids := restoreLists(&s)
	m.mu.Lock()
	m.s, m.undo = s, nil
	m.ids = max(m.ids, ids)
	m.mu.Unlock()
}

// restoreLists drops the cards and substitutions of a saved score that make no
// sense (it can be edited or damaged), keeps an emptied list nil, and returns
// the highest ID left.
func restoreLists(s *Score) int {
	high := 0
	var cards []Card
	for _, c := range s.Cards {
		n, ok := shirt(c.Number, 999)
		if c.ID <= 0 || c.ID > maxSuspID || !validTeam(c.Team) || !ok ||
			(c.Kind != "yellow" && c.Kind != "second_yellow" && c.Kind != "red") {
			continue
		}
		c.Number, c.Clock, c.Period = n, min(max(c.Clock, 0), MaxClock), max(c.Period, 1)
		cards = append(cards, c)
		high = max(high, c.ID)
	}
	var subs []Sub
	for _, x := range s.Subs {
		out, okOut := shirt(x.Out, 99)
		in, okIn := shirt(x.In, 99)
		if x.ID <= 0 || x.ID > maxSuspID || !validTeam(x.Team) || !okOut || !okIn || out == in || out == "0" || in == "0" {
			continue
		}
		x.Out, x.In, x.Clock, x.Period = out, in, min(max(x.Clock, 0), MaxClock), max(x.Period, 1)
		subs = append(subs, x)
		high = max(high, x.ID)
	}
	s.Cards, s.Subs = cards, subs
	return high
}

func validTeam(t string) bool { return t == "home" || t == "away" }

// shirt is a shirt number written the one way: 1 to 3 digits, at most max, no
// leading zeros ("07" is "7"). The bool is false for anything else.
func shirt(n string, max int) (string, bool) {
	if n == "" || !validNumber(n) {
		return "", false
	}
	v, err := strconv.Atoi(n)
	if err != nil || v > max {
		return "", false
	}
	return strconv.Itoa(v), true
}

// Seed starts from the score on air, when the scorer takes over from the
// console. A console that never sent a period leaves the scorer's own game.
func (m *Manual) Seed(l state.Live) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.Rules()
	if r.Sets() {
		m.seedSets(l)
		return
	}
	p, err := strconv.Atoi(l.Period)
	if err != nil || p < 1 {
		return
	}
	s := Score{Home: max(l.Home, 0), Away: max(l.Away, 0), Period: p, Length: m.s.Length,
		HomeFouls: l.HomeFouls, AwayFouls: l.AwayFouls, HomeTimeouts: l.HomeTimeouts, AwayTimeouts: l.AwayTimeouts}
	s.Clock = min(state.Tenths(l.Clock), MaxClock)
	if s.Clock < 0 {
		s.Clock = r.Start(p, s.Length)
	}
	m.s, m.undo = s, nil
	m.changed()
}

// seedSets is Seed for a sets sport. The console sends the sets won, not
// their scores, so each seeded set has no points, home's wins first. A console
// that sent nothing leaves the scorer's own game.
func (m *Manual) seedSets(l state.Live) {
	if l.HomeSets+l.AwaySets == 0 && l.Home == 0 && l.Away == 0 && l.Serve == "" {
		return
	}
	s := Score{Home: max(l.Home, 0), Away: max(l.Away, 0), Length: m.s.Length, Serve: l.Serve}
	i := 0
	for ; i < min(l.HomeSets, maxSets); i++ {
		s.Sets[i] = state.SetScore{Set: i + 1, Winner: "home"}
	}
	for j := 0; j < l.AwaySets && i < maxSets; j++ {
		s.Sets[i] = state.SetScore{Set: i + 1, Winner: "away"}
		i++
	}
	s.Clock, s.Period = s.Length, i+1
	m.s, m.undo = s, nil
	m.changed()
}

// Score is the game as it stands, for the scorer's page.
func (m *Manual) Score() Score {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current(m.Rules()).cloned()
}

// Heartbeat is the scorer's page calling in. It counts as connected for Seen.
func (m *Manual) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = m.Now()
	m.tellBox()
}

// SetLength sets the period length to tenths, as "period_length" does: a
// clock sitting at the start of the first period moves to its new start. It is
// for the server, when the game's sport changes, so unlike Do it is not the
// phone calling in and it leaves the undo list alone. It only touches a fresh
// game (period 1, 0-0, no set played, the clock stopped) and changes nothing
// otherwise: a game under way keeps its length until the volunteer starts a new
// one. A length outside 1 to MaxClock is refused.
func (m *Manual) SetLength(tenths int) error {
	return m.SetLengthFrom(m.Rules(), tenths)
}

// SetLengthFrom is SetLength for a change of sport: was is the sport the game
// had, so a clock still at that sport's start (the old length on a countdown, 0
// on a count-up) goes to the new sport's start and the old game's suspensions
// go with it. A clock anywhere else, and the suspensions, stay.
func (m *Manual) SetLengthFrom(was sport.Profile, tenths int) error {
	if tenths <= 0 || tenths > MaxClock {
		return errClock
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.Rules()
	m.s = m.current(r)
	if n, _, _ := m.s.played(); m.s.Running || m.s.Period != 1 || m.s.Home != 0 || m.s.Away != 0 || n > 0 {
		return nil
	}
	prev := m.s
	if m.s.Clock == was.Start(1, m.s.Length) {
		m.s.Clock = r.Start(1, tenths)
		m.s.HomeSusp, m.s.AwaySusp = [maxSusp]Suspension{}, [maxSusp]Suspension{}
	}
	m.s.Length = tenths
	if !sameScore(m.s, prev) {
		m.changed()
	}
	return nil
}

// Do applies one action from the scorer's page; it counts as a heartbeat too.
func (m *Manual) Do(a Action) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = m.Now()
	m.tellBox()
	r := m.Rules()
	m.s = m.current(r)
	prev, clock := m.s.cloned(), false
	s := &m.s
	sets := r.Sets()
	if err := refused(a, sets); err != nil {
		return err
	}
	if a.Do == "foul" && r.FoulMarks == 0 {
		return errNoFouls
	}
	switch a.Do {
	case "card", "sub", "stoppage", "game_end":
		if !r.Cards {
			return errNoCards
		}
	}
	switch a.Do {
	case "point", "foul", "timeout":
		p, err := m.count(r, a)
		if err != nil {
			return err
		}
		*p = max(*p+a.N, 0)
		if sets && a.Do == "point" && a.N == 1 {
			s.Serve = a.Team
		}
	case "serve":
		if a.Team != "home" && a.Team != "away" {
			return errTeam
		}
		s.Serve = a.Team
	case "set_next":
		n, hs, as := s.played()
		if hs >= r.SetsToWin || as >= r.SetsToWin || n >= maxSets {
			return errGameOver
		}
		w := r.SetWon(n+1, s.Home, s.Away)
		if w == "" {
			return errSetOpen
		}
		s.Sets[n] = state.SetScore{Set: n + 1, Home: s.Home, Away: s.Away, Winner: w}
		s.Home, s.Away, s.HomeTimeouts, s.AwayTimeouts, s.Serve = 0, 0, 0, 0, ""
		s.Period = n + 2
	case "undo":
		if len(m.undo) == 0 {
			return errUndo
		}
		u := m.undo[len(m.undo)-1]
		m.undo = m.undo[:len(m.undo)-1]
		if u.clock {
			u.s.Running = false
		} else {
			u.s.Clock, u.s.Running = s.Clock, s.Running
		}
		*s = u.s
		s.expire()
		m.changed()
		return nil
	case "clock_start":
		if !s.Running && clockCanStart(r, s) {
			s.Running, m.from, m.fromClock = true, m.Now(), s.Clock
			s.Over = false
		}
	case "clock_stop":
		s.Running = false
	case "clock_set":
		if a.Tenths < 0 || a.Tenths > MaxClock {
			return errClock
		}
		s.Clock, clock = a.Tenths, true
		if r.CountUp() {
			s.Clock = clampUp(r, s, s.Clock)
		}
		m.from, m.fromClock = m.Now(), s.Clock
		s.Running = s.Running && clockCanStart(r, s)
	case "clock_add":
		if a.Tenths == 0 || a.Tenths < -MaxClock || a.Tenths > MaxClock {
			return errClock
		}
		s.Clock = min(max(s.Clock+a.Tenths, 0), MaxClock)
		if r.CountUp() {
			s.Clock = clampUp(r, s, s.Clock)
		}
		m.from, m.fromClock, clock = m.Now(), s.Clock, true
		s.Running = s.Running && clockCanStart(r, s)
	case "period_next":
		if r.Cards && s.Period >= r.Periods {
			return errNoExtra
		}
		s.Period++
		s.Stoppage = 0
		s.Clock, s.Running, clock = r.Start(s.Period, s.Length), false, true
		if r.ResetsFouls(s.Period) {
			s.HomeFouls, s.AwayFouls = 0, 0
		}
		if r.ResetsTimeouts(s.Period) {
			s.HomeTimeouts, s.AwayTimeouts = 0, 0
		}
	case "period_back":
		if r.CountUp() && s.Period > 1 {
			s.Clock = r.End(s.Period-1, s.Length)
		}
		s.Period, s.Running, s.Over, clock = max(s.Period-1, 1), false, false, true
	case "period_length":
		if a.Tenths <= 0 || a.Tenths > MaxClock {
			return errClock
		}
		if !s.Running && s.Clock == r.Start(s.Period, s.Length) {
			s.Clock = r.Start(s.Period, a.Tenths)
		}
		s.Length, clock = a.Tenths, true
	case "new_game":
		*s, clock = newScoreAt(s.Length, r.Start(1, s.Length)), true
	case "suspension":
		if r.Suspensions == 0 {
			return errExclusaoSport
		}
		side, err := suspSide(s, a.Team)
		if err != nil {
			return err
		}
		i := 0
		for i < maxSusp && side[i].ID != 0 {
			i++
		}
		if i == maxSusp {
			return errExclusaoFull
		}
		s.NextSusp++
		side[i] = Suspension{ID: s.NextSusp, End: s.Clock + r.Suspensions}
	case "suspension_number", "suspension_end":
		if r.Suspensions == 0 {
			return errExclusaoSport
		}
		x := s.suspension(a.ID)
		if x == nil {
			return errExclusaoGone
		}
		if a.Do == "suspension_end" {
			*x = Suspension{}
			break
		}
		if !validNumber(a.Number) {
			return errExclusaoNumber
		}
		x.Number = a.Number
	case "card":
		c, err := m.card(s, a)
		if err != nil {
			return err
		}
		s.Cards = append(s.Cards, c)
	case "sub":
		team, out, in := a.Team, a.Out, a.In
		o, okOut := shirt(out, 99)
		i, okIn := shirt(in, 99)
		if !validTeam(team) || !okOut || !okIn || o == i || o == "0" || i == "0" {
			return errSub
		}
		m.ids++
		s.Subs = append(s.Subs, Sub{ID: m.ids, Team: team, Out: o, In: i, Clock: s.Clock, Period: s.Period})
	case "stoppage":
		if a.N < 0 || a.N > maxStoppage {
			return errStoppage
		}
		s.Stoppage = a.N
	case "game_end":
		if s.Period < r.Periods {
			return errEndEarly
		}
		s.Over, s.Running = true, false
	default:
		return i18n.Err("manual.acao_desconhecida", "acao", a.Do)
	}
	if sameScore(*s, prev) {
		return nil
	}
	if a.Do != "clock_start" && a.Do != "clock_stop" {
		m.undo = append(m.undo, undone{a, prev, clock})
		if len(m.undo) > undoMax {
			m.undo = m.undo[len(m.undo)-undoMax:]
		}
	}
	m.changed()
	return nil
}

// card is the card a "card" action gives: a second yellow to a player who has
// one, nothing to a player already sent off. It takes the next ID only once it
// has said yes.
func (m *Manual) card(s *Score, a Action) (Card, error) {
	n, ok := shirt(a.Number, 999)
	if !validTeam(a.Team) || !ok || (a.Kind != "yellow" && a.Kind != "red") {
		return Card{}, errCard
	}
	kind := a.Kind
	for _, c := range s.Cards {
		if c.Team != a.Team || c.Number != n {
			continue
		}
		if c.Kind != "yellow" {
			return Card{}, errSentOff
		}
		if kind == "yellow" {
			kind = "second_yellow"
		}
	}
	m.ids++
	return Card{ID: m.ids, Team: a.Team, Number: n, Kind: kind, Clock: s.Clock, Period: s.Period}, nil
}

// clockCanStart reports whether a stopped clock may start: a countdown above
// 0, a count-up below the end of its period (below MaxClock when it runs on).
func clockCanStart(r sport.Profile, s *Score) bool {
	if !r.CountUp() {
		return s.Clock > 0
	}
	return s.Clock < clampUp(r, s, MaxClock)
}

// clampUp keeps t inside the current period of a count-up clock: from its
// start to its end, or to MaxClock when the sport's clock runs on.
func clampUp(r sport.Profile, s *Score, t int) int {
	hi := MaxClock
	if r.StopAtEnd {
		hi = min(r.End(s.Period, s.Length), MaxClock)
	}
	return min(max(t, r.Start(s.Period, s.Length)), hi)
}

// suspSide is the team's three suspension slots.
func suspSide(s *Score, team string) (*[maxSusp]Suspension, error) {
	switch team {
	case "home":
		return s.sides()[0], nil
	case "away":
		return s.sides()[1], nil
	}
	return nil, errTeam
}

// suspension is the running suspension with id, nil when there is none.
func (s *Score) suspension(id int) *Suspension {
	if id <= 0 {
		return nil
	}
	for _, side := range s.sides() {
		for i := range side {
			if side[i].ID == id {
				return &side[i]
			}
		}
	}
	return nil
}

// validNumber is a shirt number: 1 to 3 digits, or empty to clear it.
func validNumber(n string) bool {
	if len(n) > 3 {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// refused is the error for an action the sport's kind of scoring does not
// take, nil for one it does: a sets sport has no fouls, clock or periods and
// scores one point at a time, and only it has a serve and set ends.
func refused(a Action, sets bool) error {
	switch a.Do {
	case "serve", "set_next":
		if !sets {
			return i18n.Err("manual.acao_desconhecida", "acao", a.Do)
		}
	case "foul":
		if sets {
			return errNoFouls
		}
	case "clock_start", "clock_stop", "clock_set", "clock_add", "period_next", "period_back", "period_length":
		if sets {
			return errNoClock
		}
	case "point":
		if sets && a.N != 1 && a.N != -1 {
			return errPoints
		}
	}
	return nil
}

// Undo is the action undo would take back next; false when there is none.
func (m *Manual) Undo() (Action, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.undo) == 0 {
		return Action{}, false
	}
	return m.undo[len(m.undo)-1].a, true
}

// count is the number a point, foul or timeout action changes.
func (m *Manual) count(r sport.Profile, a Action) (*int, error) {
	if a.Team != "home" && a.Team != "away" {
		return nil, errTeam
	}
	home := a.Team == "home"
	switch a.Do {
	case "point":
		if a.N == 0 || a.N < -3 || a.N > 3 {
			return nil, errPoints
		}
		// A profile with no MaxPoint (a test's own) takes up to 3, as basketball does.
		if most := r.MaxPoint; most > 0 && (a.N < -most || a.N > most) {
			return nil, errOnePoint
		}
		if home {
			return &m.s.Home, nil
		}
		return &m.s.Away, nil
	case "foul":
		if a.N != 1 && a.N != -1 {
			return nil, errOne
		}
		if home {
			return &m.s.HomeFouls, nil
		}
		return &m.s.AwayFouls, nil
	}
	if a.N != 1 && a.N != -1 {
		return nil, errOne
	}
	if home {
		return &m.s.HomeTimeouts, nil
	}
	return &m.s.AwayTimeouts, nil
}

// Run sends the score while the clock runs and once a second besides, as
// the box does, so the state picks it up again after the demo.
func (m *Manual) Run(stop <-chan struct{}, out func(map[string]string), connected func(bool)) {
	run := m.attach(out, connected)
	defer m.detach(run)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for n := 1; ; n++ {
		select {
		case <-stop:
			return
		case <-tick.C:
			m.Tick()
			if n%10 == 0 {
				m.refresh()
			}
		}
	}
}

// attach makes out and connected the ones the score goes to and returns the run's number.
func (m *Manual) attach(out func(map[string]string), connected func(bool)) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs++
	m.out, m.connected = out, connected
	m.refreshLocked()
	return m.runs
}

// detach lets go of them, unless a newer run took over meanwhile.
func (m *Manual) detach(run int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs == run {
		m.out, m.connected = nil, nil
	}
}

func (m *Manual) refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked()
}

// Tick moves the running clock on. Run calls it every 100 ms.
func (m *Manual) Tick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.out == nil {
		return
	}
	r := m.Rules()
	m.s = m.current(r)
	was := m.sent
	if clockText(r, m.s.Clock) == clockText(r, was.Clock) && m.s.Running == was.Running &&
		suspensionsJSON(m.s) == suspensionsJSON(was) {
		return
	}
	if m.s.Clock/10 != was.Clock/10 || !m.s.Running {
		m.save()
	}
	m.emit()
}

// current is the score with the running clock brought up to now. A countdown
// stops at 0; a count-up stops at the end of its period when the sport says
// so, else at MaxClock. Suspensions that have run out are gone.
func (m *Manual) current(r sport.Profile) Score {
	s := m.s
	if s.Running {
		elapsed := int(m.Now().Sub(m.from) / (100 * time.Millisecond))
		if r.CountUp() {
			s.Clock = m.fromClock + elapsed
			if hi := clampUp(r, &s, MaxClock); s.Clock >= hi {
				s.Clock, s.Running = hi, false
			}
		} else {
			s.Clock = max(m.fromClock-elapsed, 0)
			s.Running = s.Clock > 0
		}
	}
	if r.Suspensions > 0 {
		s.expire()
	}
	return s
}

// clockText is the clock as the box shows it: MM:SS counting up, or the
// countdown's text.
func clockText(r sport.Profile, t int) string {
	if r.CountUp() {
		return state.ClockTextUp(t)
	}
	return state.ClockText(t)
}

// suspensionsJSON is the running suspensions for the state, home first:
// [{"team":"home","id":1,"number":"7","left":"01:23"}, ...]. The time left
// rounds up, so the last second reads 00:01. A slot that has run out is
// never in it.
func suspensionsJSON(s Score) string {
	type out struct {
		Team   string `json:"team"`
		ID     int    `json:"id"`
		Number string `json:"number"`
		Left   string `json:"left"`
	}
	list := []out{}
	for i, side := range s.sides() {
		team := [2]string{"home", "away"}[i]
		for _, x := range side {
			if x.ID != 0 && x.End > s.Clock {
				left := (x.End - s.Clock + 9) / 10 * 10
				list = append(list, out{team, x.ID, x.Number, state.ClockTextUp(left)})
			}
		}
	}
	b, _ := json.Marshal(list) // a list of strings and ints always marshals
	return string(b)
}

// The rest run with mu held.

func (m *Manual) refreshLocked() {
	m.tellBox()
	m.emit()
}

func (m *Manual) changed() {
	m.save()
	m.emit()
}

func (m *Manual) save() {
	err := m.Save(m.s)
	if err != nil && !m.saveFailed {
		m.Log(i18n.T("log.nao_guardei_marcacao", "err", err))
	}
	m.saveFailed = err != nil
}

func (m *Manual) emit() {
	if m.out == nil {
		return
	}
	m.sent = m.s.cloned()
	m.out(frame(m.Rules(), m.s))
}

// Frame is what the scorer sends the state for the game as it stands, for a
// caller that wants it before Run starts (the state is primed with it at start).
func (m *Manual) Frame() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.Rules()
	return frame(r, m.current(r))
}

// frame is the map of values for s under r's rules.
func frame(r sport.Profile, s Score) map[string]string {
	n := strconv.Itoa
	if r.Sets() {
		played, hs, as := s.played()
		var done []string
		for _, x := range s.Sets[:played] {
			side := "h"
			if x.Winner == "away" {
				side = "a"
			}
			done = append(done, n(x.Home)+"-"+n(x.Away)+side)
		}
		return map[string]string{
			"home": n(s.Home), "away": n(s.Away),
			"clock": "", "period": n(s.Period),
			"home_timeouts": n(s.HomeTimeouts), "away_timeouts": n(s.AwayTimeouts),
			"home_sets": n(hs), "away_sets": n(as), "serve": s.Serve,
			"sets": strings.Join(done, ","), "shot_clock": "",
			"period_end": "", "suspensions": "[]",
		}
	}
	end, susp := "", "[]"
	if r.CountUp() {
		end = n(r.End(s.Period, s.Length))
	}
	if r.Suspensions > 0 {
		susp = suspensionsJSON(s)
	}
	v := map[string]string{
		"home": n(s.Home), "away": n(s.Away),
		"clock": clockText(r, s.Clock), "period": n(s.Period), "period_end": end,
		"home_fouls": n(s.HomeFouls), "away_fouls": n(s.AwayFouls),
		"home_timeouts": n(s.HomeTimeouts), "away_timeouts": n(s.AwayTimeouts),
		"shot_clock": "", "suspensions": susp,
	}
	if r.Cards {
		v["over"] = ""
		if s.Over {
			v["over"] = "1"
		}
		v["stoppage"] = n(s.Stoppage)
		v["cards"], v["subs"] = cardsJSON(r, s), subsJSON(r, s)
	}
	return v
}

// minute is the clock of a card or substitution as the match report writes it,
// for an event at clock in period: up to the period's end the minute being
// played ("23'" at 22:10, "45'" at exactly 45:00.0), after it the added time
// ("45+1'" from 45:00.1, "45+3'" at 47:12).
func minute(r sport.Profile, length, period, clock int) string {
	end := r.End(period, length)
	if clock <= end {
		return strconv.Itoa(min(clock/600+1, end/600)) + "'"
	}
	return fmt.Sprintf("%d+%d'", end/600, (clock-end+599)/600)
}

// cardsJSON is the cards for the state, in the order they were shown:
// [{"id":1,"team":"home","number":"4","kind":"yellow","minute":"23'"}, ...]
// ("[]" when there are none).
func cardsJSON(r sport.Profile, s Score) string {
	type out struct {
		ID     int    `json:"id"`
		Team   string `json:"team"`
		Number string `json:"number"`
		Kind   string `json:"kind"`
		Minute string `json:"minute"`
	}
	list := []out{}
	for _, c := range s.Cards {
		list = append(list, out{c.ID, c.Team, c.Number, c.Kind, minute(r, s.Length, c.Period, c.Clock)})
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// subsJSON is cardsJSON for the substitutions:
// [{"id":2,"team":"home","out":"7","in":"14","minute":"60'"}, ...].
func subsJSON(r sport.Profile, s Score) string {
	type out struct {
		ID     int    `json:"id"`
		Team   string `json:"team"`
		Out    string `json:"out"`
		In     string `json:"in"`
		Minute string `json:"minute"`
	}
	list := []out{}
	for _, x := range s.Subs {
		list = append(list, out{x.ID, x.Team, x.Out, x.In, minute(r, s.Length, x.Period, x.Clock)})
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// tellBox reports whether the phone called in within Seen, while manual runs.
func (m *Manual) tellBox() {
	if m.connected != nil {
		m.connected(!m.seen.IsZero() && m.Now().Sub(m.seen) < Seen)
	}
}
