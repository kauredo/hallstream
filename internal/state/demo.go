package state

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/sport"
)

// SetDemo turns the fake game on or off. While it runs, box frames are ignored.
func (m *Machine) SetDemo(on bool) {
	m.setDemo(on, func(stop <-chan struct{}) { go m.demo(stop) })
}

// setDemo is SetDemo with the start of the loop passed in, for tests that
// drive it themselves.
func (m *Machine) setDemo(on bool, start func(stop <-chan struct{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if on == m.s.Demo {
		return
	}
	m.s.Demo = on
	if on {
		m.realPeriods, m.realSets = m.s.Periods, m.s.Sets
		m.realTold = [3]told{m.toldSusp, m.toldCards, m.toldSubs}
		m.demoStop = make(chan struct{})
		start(m.demoStop)
	} else {
		close(m.demoStop)
		// The demo's periods and scores go; the game's periods come back,
		// and the box's next frame starts the live values afresh. So do the
		// timeline's marks, which the demo's suspensions and cards raised.
		m.s.Periods = append([]Period{}, m.realPeriods...)
		m.s.Sets = append([]SetScore{}, m.realSets...)
		m.toldSusp, m.toldCards, m.toldSubs = m.realTold[0], m.realTold[1], m.realTold[2]
		m.s.Live = Live{}
		m.started, m.lastTenths, m.zeroSince = false, -1, time.Time{}
		m.finalDone, m.matchOver = 0, false
	}
	m.changed()
}

func shotText(s int) string {
	if s < 10 {
		return fmt.Sprintf("%d.0", s)
	}
	return strconv.Itoa(s)
}

const (
	nothing = iota
	homeScores
	awayScores
	homeFoul
	awayFoul
)

// demoEvent picks what happens in one game second. The club scores a bit
// more often; in a close finish (comeback) only the club scores, and often.
func demoEvent(x float64, comeback bool) int {
	home, away := 0.0175, 0.034
	if comeback {
		home, away = 0.06, 0.06
	}
	switch {
	case x < home:
		return homeScores
	case x < away:
		return awayScores
	case x < 0.040:
		return homeFoul
	case x < 0.046:
		return awayFoul
	}
	return nothing
}

// demo plays the game's sport at a compressed pace, each sport on its own
// scripted timeline (basketball: playDemo; the others: demo_sports.go). It
// follows the game's sport: if that changes while it runs, the next frame
// starts that sport's game.
func (m *Machine) demo(stop <-chan struct{}) {
	tick := time.NewTicker(100 * time.Millisecond) // one tick
	defer tick.Stop()
	m.demoLoop(stop, func(n int) bool {
		for range n {
			select {
			case <-stop:
				return false
			case <-tick.C:
			}
		}
		return true
	})
}

// demoLoop plays demo games until stop closes; wait(n) waits n ticks and
// reports false once the demo is stopped.
func (m *Machine) demoLoop(stop <-chan struct{}, wait func(n int) bool) {
	r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 7))
	newGame := func() {
		m.mu.Lock()
		if m.s.Demo {
			m.s.Periods, m.s.Sets = []Period{}, []SetScore{}
			m.finalDone, m.matchOver, m.zeroSince = 0, false, time.Time{}
			// The last game's final is not this game's.
			if m.s.Screen.Current == "final" {
				m.auto("marcador")
			}
		}
		m.mu.Unlock()
	}
	for {
		m.mu.Lock()
		p := m.rules()
		m.mu.Unlock()
		same := func(n int) bool {
			if !wait(n) {
				return false
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			return m.rules().ID == p.ID
		}
		show := func(l Live) { m.update(l, true) }
		if p.ID == sport.Basketball.ID || !scriptable(p) {
			playDemo(r, show, same, newGame)
		} else {
			playSport(p, show, same, newGame)
		}
		select {
		case <-stop:
			return
		default:
		}
	}
}

// playDemo runs demo games until hold says stop; hold(n) waits n game seconds.
func playDemo(r *rand.Rand, show func(Live), hold func(int) bool, newGame func()) {
	points := func() int { return []int{1, 2, 2, 2, 3}[r.IntN(5)] }
	for {
		newGame()
		l := Live{HomeName: i18n.T("jogo.casa"), AwayName: i18n.T("jogo.visitante"), Sport: "basketball"}
		for p := 1; ; p++ {
			length := 600
			if p > 4 {
				length = 300
			}
			l.Period, l.Clock, l.Shot = strconv.Itoa(p), ClockText(length*10), "24"
			if p <= 4 {
				l.HomeFouls, l.AwayFouls = 0, 0
			}
			show(l)
			if !hold(30) {
				return
			}
			shot := 24
			for left := length - 1; left >= 0; left-- {
				shot--
				switch demoEvent(r.Float64(), p >= 4 && left < 300 && l.Home <= l.Away) {
				case homeScores:
					l.Home += points()
					shot = 24
				case awayScores:
					l.Away += points()
					shot = 24
				case homeFoul:
					l.HomeFouls++
				case awayFoul:
					l.AwayFouls++
				}
				if left == 0 && p >= 4 && l.Home < l.Away {
					l.Home = l.Away // the club still wins the demo: level, and overtime decides
				}
				if shot <= 0 {
					shot = 24
				}
				l.Clock, l.Shot = ClockText(left*10), shotText(min(shot, left))
				show(l)
				if !hold(1) {
					return
				}
			}
			if p >= 4 && l.Home != l.Away {
				// Long enough for the final screen, then a new game.
				if !hold(int(FinalHold/(100*time.Millisecond)) + 100) {
					return
				}
				break
			}
			pause := 50
			if p == 2 {
				pause = 200
			}
			if !hold(pause) {
				return
			}
		}
	}
}
