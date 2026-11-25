package source

import (
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
	Clock        int  `json:"clock"`   // tenths of a second left in the period
	Running      bool `json:"running"` // the clock is counting down
	Length       int  `json:"length"`  // tenths of a second in a period
	HomeFouls    int  `json:"home_fouls"`
	AwayFouls    int  `json:"away_fouls"`
	HomeTimeouts int  `json:"home_timeouts"`
	AwayTimeouts int  `json:"away_timeouts"`

	// Volleyball. Sets is an array, not a slice, so a Score stays comparable
	// with ==; a played set has a Winner, an empty slot has none.
	Sets  [maxSets]SetScore `json:"sets"`  // finished sets, in order
	Serve string            `json:"serve"` // "home", "away" or ""
}

// maxSets is the most sets a match can have (best of 5).
const maxSets = 5

// SetScore is one finished set. A set seeded from a console has no points
// (0-0) and only its Winner.
type SetScore struct {
	Home   int    `json:"home"`
	Away   int    `json:"away"`
	Winner string `json:"winner"`
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
	// MaxClock is the longest time the clock can show, 99:59.
	MaxClock = (99*60 + 59) * 10
	// Seen is how long the scorer's phone counts as connected after it last called in.
	Seen = 60 * time.Second
	// undoMax is how many actions undo can take back.
	undoMax = 50
)

func newScore(length int) Score { return Score{Period: 1, Clock: length, Length: length} }

// Action is one tap on the scorer's page. Do is one of:
//   - "point": N points (-3 to 3, not 0) for Team; never below 0
//   - "foul", "timeout": N (1 or -1) for Team; never below 0
//   - "undo": takes back the last action, up to 50, but never the clock's
//     start and stop, and only takes the clock back for an action that set it
//   - "serve": the serve to Team (volleyball)
//   - "set_next": ends the set (volleyball): refused until the points win it;
//     the score goes into the finished sets, the points and timeouts to 0 and
//     the serve to nobody; refused once a team has won the match
//   - "clock_start", "clock_stop"
//   - "clock_set": the clock to Tenths (0 to MaxClock); a running clock keeps running
//   - "clock_add": Tenths more (or less, below 0) on the clock, kept between 0 and MaxClock
//   - "period_next": the next period, with the clock stopped at the period
//     length and the team fouls at 0 when the sport resets them; the timeouts
//     go back to 0 when the sport's profile says so (basketball: the second
//     half and each overtime)
//   - "period_back": the period before, the clock stopped where it is
//   - "period_length": the period length to Tenths (above 0); a clock
//     stopped at the old length moves to the new one
//   - "new_game": 0-0 in period 1, the clock at the period length
//
// A sets sport (volleyball) takes only +1 or -1 point, which also gives the
// serve to the team that scored; it refuses fouls, the clock and the period
// actions.
type Action struct {
	Do     string `json:"do"`
	Team   string `json:"team"` // "home" or "away"
	N      int    `json:"n"`
	Tenths int    `json:"tenths"`
}

var (
	errTeam   error = i18n.Key("manual.equipa")
	errPoints error = i18n.Key("manual.pontos")
	errOne    error = i18n.Key("manual.um_de_cada_vez")
	errClock  error = i18n.Key("manual.tempo")
	errUndo   error = i18n.Key("manual.nada_para_desfazer")

	errNoFouls  error = i18n.Key("manual.sem_faltas")
	errSetOpen  error = i18n.Key("manual.set_por_acabar")
	errGameOver error = i18n.Key("manual.jogo_acabou")
	errNoClock  error = i18n.Key("manual.sem_relogio")
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
	m.s = newScore(DefaultLength)
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
	m.mu.Lock()
	m.s, m.undo = s, nil
	m.mu.Unlock()
}

// Seed starts from the score on air, when the scorer takes over from the
// console. A console that never sent a period leaves the scorer's own game.
func (m *Manual) Seed(l state.Live) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Rules().Sets() {
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
		s.Clock = s.Length
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
		s.Sets[i] = SetScore{Winner: "home"}
	}
	for j := 0; j < l.AwaySets && i < maxSets; j++ {
		s.Sets[i] = SetScore{Winner: "away"}
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
	return m.current()
}

// Heartbeat is the scorer's page calling in. It counts as connected for Seen.
func (m *Manual) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = m.Now()
	m.tellBox()
}

// Do applies one action from the scorer's page; it counts as a heartbeat too.
func (m *Manual) Do(a Action) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = m.Now()
	m.tellBox()
	m.s = m.current()
	prev, clock := m.s, false
	s := &m.s
	r := m.Rules()
	if r.Sets() {
		switch a.Do {
		case "foul":
			return errNoFouls
		case "clock_start", "clock_stop", "clock_set", "clock_add", "period_next", "period_back", "period_length":
			return errNoClock
		}
	}
	switch a.Do {
	case "point", "foul", "timeout":
		if r.Sets() && a.Do == "point" && a.N != 1 && a.N != -1 {
			return errPoints
		}
		p, err := m.count(a)
		if err != nil {
			return err
		}
		*p = max(*p+a.N, 0)
		if r.Sets() && a.Do == "point" && a.N == 1 {
			s.Serve = a.Team
		}
	case "serve":
		if !r.Sets() {
			return i18n.Err("manual.acao_desconhecida", "acao", a.Do)
		}
		if a.Team != "home" && a.Team != "away" {
			return errTeam
		}
		s.Serve = a.Team
	case "set_next":
		if !r.Sets() {
			return i18n.Err("manual.acao_desconhecida", "acao", a.Do)
		}
		n, hs, as := s.played()
		if hs >= r.SetsToWin || as >= r.SetsToWin || n >= maxSets {
			return errGameOver
		}
		w := r.SetWon(n+1, s.Home, s.Away)
		if w == "" {
			return errSetOpen
		}
		s.Sets[n] = SetScore{Home: s.Home, Away: s.Away, Winner: w}
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
		m.changed()
		return nil
	case "clock_start":
		if !s.Running && s.Clock > 0 {
			s.Running, m.from, m.fromClock = true, m.Now(), s.Clock
		}
	case "clock_stop":
		s.Running = false
	case "clock_set":
		if a.Tenths < 0 || a.Tenths > MaxClock {
			return errClock
		}
		s.Clock, m.from, m.fromClock, clock = a.Tenths, m.Now(), a.Tenths, true
		s.Running = s.Running && s.Clock > 0
	case "clock_add":
		if a.Tenths == 0 || a.Tenths < -MaxClock || a.Tenths > MaxClock {
			return errClock
		}
		s.Clock = min(max(s.Clock+a.Tenths, 0), MaxClock)
		m.from, m.fromClock, clock = m.Now(), s.Clock, true
		s.Running = s.Running && s.Clock > 0
	case "period_next":
		s.Period++
		s.Clock, s.Running, clock = s.Length, false, true
		if r.FoulsReset {
			s.HomeFouls, s.AwayFouls = 0, 0
		}
		if r.ResetsTimeouts(s.Period) {
			s.HomeTimeouts, s.AwayTimeouts = 0, 0
		}
	case "period_back":
		s.Period, s.Running, clock = max(s.Period-1, 1), false, true
	case "period_length":
		if a.Tenths <= 0 || a.Tenths > MaxClock {
			return errClock
		}
		if !s.Running && s.Clock == s.Length {
			s.Clock = a.Tenths
		}
		s.Length, clock = a.Tenths, true
	case "new_game":
		*s, clock = newScore(s.Length), true
	default:
		return i18n.Err("manual.acao_desconhecida", "acao", a.Do)
	}
	if *s == prev {
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
func (m *Manual) count(a Action) (*int, error) {
	if a.Team != "home" && a.Team != "away" {
		return nil, errTeam
	}
	home := a.Team == "home"
	switch a.Do {
	case "point":
		if a.N == 0 || a.N < -3 || a.N > 3 {
			return nil, errPoints
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
	m.s = m.current()
	was := m.sent
	if state.ClockText(m.s.Clock) == state.ClockText(was.Clock) && m.s.Running == was.Running {
		return
	}
	if m.s.Clock/10 != was.Clock/10 || !m.s.Running {
		m.save()
	}
	m.emit()
}

// current is the score with the running clock brought up to now; it stops at 0.
func (m *Manual) current() Score {
	s := m.s
	if s.Running {
		s.Clock = max(m.fromClock-int(m.Now().Sub(m.from)/(100*time.Millisecond)), 0)
		s.Running = s.Clock > 0
	}
	return s
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
	s := m.s
	m.sent = s
	n := strconv.Itoa
	if m.Rules().Sets() {
		played, hs, as := s.played()
		var done []string
		for _, x := range s.Sets[:played] {
			done = append(done, n(x.Home)+"-"+n(x.Away))
		}
		m.out(map[string]string{
			"home": n(s.Home), "away": n(s.Away),
			"clock": "", "period": n(s.Period),
			"home_timeouts": n(s.HomeTimeouts), "away_timeouts": n(s.AwayTimeouts),
			"home_sets": n(hs), "away_sets": n(as), "serve": s.Serve,
			"sets": strings.Join(done, ","), "shot_clock": "",
		})
		return
	}
	m.out(map[string]string{
		"home": n(s.Home), "away": n(s.Away),
		"clock": state.ClockText(s.Clock), "period": n(s.Period),
		"home_fouls": n(s.HomeFouls), "away_fouls": n(s.AwayFouls),
		"home_timeouts": n(s.HomeTimeouts), "away_timeouts": n(s.AwayTimeouts),
		"shot_clock": "",
	})
}

// tellBox reports whether the phone called in within Seen, while manual runs.
func (m *Manual) tellBox() {
	if m.connected != nil {
		m.connected(!m.seen.IsZero() && m.Now().Sub(m.seen) < Seen)
	}
}
