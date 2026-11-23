// Package state holds the live state the overlay reads, plus the rules that
// change it on their own: period scores, automatic screens and the lower
// third timeout.
package state

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Box struct {
	Connected   bool       `json:"connected"`
	LastFrameAt *time.Time `json:"last_frame_at"`
	BoxType     string     `json:"box_type"`
	Scan        Scan       `json:"scan"`
}

// Scan is what the Bluetooth reader is doing, for the panel.
type Scan struct {
	Scanning    bool   `json:"scanning"`
	DevicesSeen int    `json:"devices_seen"`
	Bluetooth   string `json:"bluetooth"` // ok, off or unavailable
	Message     string `json:"message"`
	Address     string `json:"address"` // the box connected now
	Name        string `json:"name"`
	Saved       string `json:"saved"` // settings.box_address
}

type Live struct {
	HomeName  string `json:"home_name"`
	AwayName  string `json:"away_name"`
	Home      int    `json:"home"`
	Away      int    `json:"away"`
	Clock     string `json:"clock"`
	Period    string `json:"period"`
	HomeFouls int    `json:"home_fouls"`
	AwayFouls int    `json:"away_fouls"`
	Shot      string `json:"shot"`
	Sport     string `json:"sport"`
}

// Period is one completed period: the score when it ended (Home, Away) and
// the points each team scored in it.
type Period struct {
	Period     int `json:"period"`
	Home       int `json:"home"`
	Away       int `json:"away"`
	HomePoints int `json:"home_points"`
	AwayPoints int `json:"away_points"`
}

type Group struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Coach string `json:"coach"`
}

type Team struct {
	Name     string  `json:"name"`
	Short    string  `json:"short"`
	CrestURL *string `json:"crest_url"`
	Color    string  `json:"color"`
	Initials string  `json:"initials"`
}

type SquadPlayer struct {
	Number   string `json:"number"`
	Name     string `json:"name"`
	PhotoURL string `json:"photo_url"`
}

// Photos says where the overlay shows player photos.
type Photos struct {
	Squad      bool `json:"squad"`
	LowerThird bool `json:"lower_third"`
}

type Sponsor struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type Sponsors struct {
	Principal *Sponsor  `json:"principal"`
	Others    []Sponsor `json:"others"`
}

type Game struct {
	Group       *Group        `json:"group"`
	Home        Team          `json:"home"`
	Away        Team          `json:"away"`
	Start       string        `json:"start"`
	Venue       string        `json:"venue"`
	Competition string        `json:"competition"`
	Round       string        `json:"round"`
	Squad       []SquadPlayer `json:"squad"`
	Sponsors    Sponsors      `json:"sponsors"`
	Instagram   string        `json:"instagram"`
	Photos      Photos        `json:"photos"`
	Overlay     Overlay       `json:"overlay"`
	// Sport is the club's sport id; SportLabel the word on air ("BASKET").
	Sport      string `json:"sport"`
	SportLabel string `json:"sport_label"`
	// Theme is the overlay's colours from the club's, keyed by CSS custom
	// property name without the dashes (club.Theme).
	Theme map[string]string `json:"theme"`
	// AtHome is whether the club plays at home, null when the FPB doesn't say.
	AtHome *bool `json:"at_home"`
	// Next is the club's game after this one, for proximo; null with the
	// reason in NextNote.
	Next     *NextGame `json:"next"`
	NextNote string    `json:"next_note"`
}

type NextGame struct {
	AtHome      bool   `json:"at_home"`
	Opponent    Team   `json:"opponent"`
	FullName    string `json:"full_name"` // the opponent's FPB name
	Start       string `json:"start"`
	Venue       string `json:"venue"`
	Competition string `json:"competition"`
	Round       string `json:"round"`
}

// Overlay is how the overlay is laid out (copy of settings.overlay).
type Overlay struct {
	BugPosition string `json:"bug_position"`
	// SponsorCorner shows one sponsor at a time in the top-right corner of
	// the scoreboard screen, every SponsorEvery minutes.
	SponsorCorner bool   `json:"sponsor_corner"`
	SponsorEvery  int    `json:"sponsor_every"`
	BackLine      string `json:"back_line"` // the line under VOLTAMOS JÁ
}

type Screen struct {
	Current string    `json:"current"`
	Auto    bool      `json:"auto"`
	Since   time.Time `json:"since"`
}

type LowerThird struct {
	Visible   bool       `json:"visible"`
	Type      string     `json:"type"`
	Number    string     `json:"number"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Label     string     `json:"label"`
	PhotoURL  string     `json:"photo_url"`
	Until     *time.Time `json:"until"`
}

// PromoClip is a club video as the overlay plays it.
type PromoClip struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Orientation string  `json:"orientation"`
	Kicker      string  `json:"kicker"`
	Headline    string  `json:"headline"`
	Line        string  `json:"line"`
	Link        string  `json:"link"`
	QRURL       string  `json:"qr_url"` // "" = no QR
	Sound       bool    `json:"sound"`
	Volume      int     `json:"volume"`
	End         string  `json:"end"` // "back" or "loop"
	Duration    float64 `json:"duration"`
}

// Promo is the clip on the promo screen and the one on desconto.
type Promo struct {
	Clip     *PromoClip `json:"clip"`
	Previous string     `json:"previous"` // the screen to go back to
	Seq      int        `json:"seq"`      // one per "Mostrar promo"
	Mini     *PromoClip `json:"mini"`     // small on desconto; nil = none
}

type State struct {
	Box        Box        `json:"box"`
	Live       Live       `json:"live"`
	Periods    []Period   `json:"periods"`
	Game       Game       `json:"game"`
	Screen     Screen     `json:"screen"`
	LowerThird LowerThird `json:"lower_third"`
	Camera     *Camera    `json:"camera"`
	Promo      Promo      `json:"promo"`
	Demo       bool       `json:"demo"`
}

// Rect is a box on the 1920x1080 overlay, in pixels.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Camera is where the camera picture sits at full screen and on desconto,
// when the program moves it in OBS.
type Camera struct {
	Full   Rect `json:"full"`
	Window Rect `json:"window"`
}

var Screens = []string{"marcador", "abertura", "plantel", "desconto", "intervalo", "final", "apoiantes", "proximo", "voltamos", "nada"}

// PromoGrace is how long past a clip's length the promo screen waits for the
// overlay to say the clip ended before it goes back by itself.
const PromoGrace = 5 * time.Second

// PromoUnknown stands in for the length of a clip the panel has not measured.
const PromoUnknown = 10 * time.Minute

func promoLength(sec float64) time.Duration {
	if sec <= 0 {
		return PromoUnknown
	}
	return time.Duration(sec * float64(time.Second))
}

// FinalHold is how long the clock must sit at 0 in period 4+ before the
// game counts as over.
const FinalHold = 60 * time.Second

type Machine struct {
	mu     sync.Mutex
	now    func() time.Time
	s      State
	picked bool
	subs   map[chan struct{}]bool

	// OnPeriods runs (outside the lock) each time a period score is recorded.
	OnPeriods func(State)
	// SavePeriods runs (outside the lock) each time the periods change,
	// reset included, so they can outlive a restart. Never during the demo.
	SavePeriods func([]Period)
	realPeriods []Period // the game's periods while the demo plays
	saveSeq     int      // periods snapshots taken for SavePeriods
	saveMu      sync.Mutex
	savedSeq    int // the newest snapshot SavePeriods got

	// OnEvent gets each change the timeline keeps (screen, lower_third, box,
	// period). It runs with the machine locked, so it must not call it.
	OnEvent    func(event string, fields map[string]any)
	seen       seen
	autoSwitch bool // the program picked the screen, not the volunteer

	zeroSince  time.Time // clock at 0 in period >= 4 since then
	finalDone  int       // period whose end already fired "final"
	started    bool      // the clock has run in the current period
	clockRanAt time.Time // when the clock value last changed; zero before any
	lastTenths int

	demoStop chan struct{}
}

// New returns a machine using now as its clock (time.Now outside tests).
func New(now func() time.Time) *Machine {
	m := &Machine{
		now:        now,
		subs:       map[chan struct{}]bool{},
		lastTenths: -1,
		s: State{
			Box:        Box{Scan: Scan{Bluetooth: "ok"}},
			Periods:    []Period{},
			Screen:     Screen{Current: "marcador", Auto: true, Since: now().UTC()},
			LowerThird: LowerThird{Type: "jogador"},
		},
	}
	m.seen = m.seenNow()
	return m
}

// Start runs the timers (final hold, lower third) until the program ends.
func (m *Machine) Start() {
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			m.Tick()
		}
	}()
}

// Subscribe returns a channel that receives a value after each change.
func (m *Machine) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	m.mu.Lock()
	m.subs[ch] = true
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		delete(m.subs, ch)
		m.mu.Unlock()
	}
}

// changed must be called with the lock held.
func (m *Machine) changed() {
	m.emit()
	for ch := range m.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// seen is what the timeline last wrote down.
type seen struct {
	screen     string
	lowerThird string
	box        bool
	periods    int
	demo       bool
}

func (m *Machine) seenNow() seen {
	lt := m.s.LowerThird
	shown := ""
	if lt.Visible {
		shown = lt.Type + ":" + lt.Number
	}
	return seen{m.s.Screen.Current, shown, m.s.Box.Connected, len(m.s.Periods), m.s.Demo}
}

// EmitCurrent tells OnEvent the screen, lower third and box as they are now
// (with initial true), so a stream's timeline starts from them.
func (m *Machine) EmitCurrent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.OnEvent == nil || m.s.Demo {
		return
	}
	lt := m.s.LowerThird
	m.OnEvent("screen", map[string]any{"name": m.s.Screen.Current, "auto": false, "initial": true})
	m.OnEvent("lower_third", map[string]any{"shown": lt.Visible, "type": lt.Type, "number": lt.Number, "initial": true})
	m.OnEvent("box", map[string]any{"connected": m.s.Box.Connected, "initial": true})
}

// emit tells OnEvent what changed since the last call. The demo is no game,
// so nothing is told while it runs.
func (m *Machine) emit() {
	old, cur, auto := m.seen, m.seenNow(), m.autoSwitch
	m.seen, m.autoSwitch = cur, false
	if m.OnEvent == nil || old.demo || cur.demo {
		return
	}
	if cur.screen != old.screen {
		m.OnEvent("screen", map[string]any{"name": cur.screen, "auto": auto})
	}
	if cur.lowerThird != old.lowerThird {
		lt := m.s.LowerThird
		m.OnEvent("lower_third", map[string]any{"shown": lt.Visible, "type": lt.Type, "number": lt.Number})
	}
	if cur.box != old.box {
		m.OnEvent("box", map[string]any{"connected": cur.box})
	}
	for i := old.periods; i < cur.periods; i++ {
		p := m.s.Periods[i]
		m.OnEvent("period", map[string]any{"period": p.Period, "home": p.Home, "away": p.Away})
	}
}

// Snapshot returns a copy of the state with the display rules applied.
func (m *Machine) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot()
}

func (m *Machine) snapshot() State {
	s := m.s
	s.Periods = append([]Period{}, m.s.Periods...)
	s.Promo.Clip, s.Promo.Mini = clipCopy(m.s.Promo.Clip), clipCopy(m.s.Promo.Mini)
	g := m.s.Game
	g.Squad = append([]SquadPlayer{}, g.Squad...)
	g.Sponsors.Others = append([]Sponsor{}, g.Sponsors.Others...)
	if !m.picked {
		if s.Live.HomeName != "" {
			g.Home.Name = s.Live.HomeName
		}
		if s.Live.AwayName != "" {
			g.Away.Name, g.Away.Short, g.Away.Initials = s.Live.AwayName, "", ""
		}
	}
	// A picked opponent may have up to 20 characters (opponentName in web);
	// the club's name and the console's names have 12.
	awayMax := 12
	if m.picked {
		awayMax = 20
	}
	g.Home = tidy(g.Home, 12)
	g.Away = tidy(g.Away, awayMax)
	s.Game = g
	return s
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func tidy(t Team, max int) Team {
	t.Name = cut(strings.TrimSpace(t.Name), max)
	if t.Short == "" {
		t.Short = strings.ToUpper(cut(strings.ReplaceAll(t.Name, " ", ""), 3))
	}
	t.Short = cut(t.Short, 4)
	if t.Initials == "" {
		t.Initials = t.Short
	}
	return t
}

func (m *Machine) JSON() []byte {
	b, _ := json.Marshal(m.Snapshot())
	return b
}

// ClockRanWithin reports whether the game clock changed in the last d.
func (m *Machine) ClockRanWithin(d time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.clockRanAt.IsZero() && m.now().Sub(m.clockRanAt) < d
}

// SetGame sets the game info. picked is false when no game was chosen, so the
// console names are shown instead.
func (m *Machine) SetGame(g Game, picked bool) {
	m.mu.Lock()
	m.s.Game, m.picked = g, picked
	m.changed()
	m.mu.Unlock()
}

func (m *Machine) SetBox(connected bool) {
	m.mu.Lock()
	if m.s.Box.Connected != connected {
		m.s.Box.Connected = connected
		m.changed()
	}
	m.mu.Unlock()
}

// SetCamera sets the camera boxes; nil when the program does not move the camera.
func (m *Machine) SetCamera(c *Camera) {
	m.mu.Lock()
	if (c == nil) != (m.s.Camera == nil) || c != nil && *c != *m.s.Camera {
		m.s.Camera = c
		m.changed()
	}
	m.mu.Unlock()
}

func (m *Machine) SetScan(s Scan) {
	m.mu.Lock()
	if m.s.Box.Scan != s {
		m.s.Box.Scan = s
		m.changed()
	}
	m.mu.Unlock()
}

// SetScreen switches the screen by hand. An empty name only sets auto.
func (m *Machine) SetScreen(name string, auto bool) error {
	if name != "" && !valid(name) {
		return errors.New("ecrã desconhecido")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Screen.Auto = auto
	if name != "" {
		m.switchTo(name)
	}
	m.changed()
	return nil
}

// switchTo changes the screen; leaving promo drops its clip. Lock held.
func (m *Machine) switchTo(name string) {
	if name == m.s.Screen.Current {
		return
	}
	if m.s.Screen.Current == "promo" {
		m.s.Promo.Clip = nil
	}
	m.s.Screen.Current, m.s.Screen.Since, m.autoSwitch = name, m.now().UTC(), false
}

// ShowPromo puts a clip on the promo screen, remembering the screen to go back to.
func (m *Machine) ShowPromo(c PromoClip) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur := m.s.Screen.Current; cur != "promo" {
		m.s.Promo.Previous = cur
	}
	m.s.Promo.Clip = &c
	m.s.Promo.Seq++
	m.s.Screen.Current, m.s.Screen.Since = "promo", m.now().UTC()
	m.changed()
}

// EndPromo goes back to the screen before the promo. seq 0 stops any clip;
// another seq only ends the clip it belongs to, so a late "ended" from the
// overlay can't stop the next one. It reports whether the screen changed.
func (m *Machine) EndPromo(seq int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s.Screen.Current != "promo" || seq != 0 && seq != m.s.Promo.Seq {
		return false
	}
	m.endPromo()
	m.autoSwitch = seq != 0 // the clip ended by itself
	m.changed()
	return true
}

func (m *Machine) endPromo() {
	prev := m.s.Promo.Previous
	if prev == "" || prev == "promo" {
		prev = "marcador"
	}
	m.switchTo(prev)
}

// SyncPromo refreshes the clips from the library: get returns a clip by id
// (false when it was deleted, which ends it) and mini is the desconto clip.
func (m *Machine) SyncPromo(get func(id string) (PromoClip, bool), mini *PromoClip) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := &m.s.Promo
	changed := !sameClip(p.Mini, mini)
	p.Mini = clipCopy(mini)
	if p.Clip != nil {
		c, ok := get(p.Clip.ID)
		switch {
		case !ok && m.s.Screen.Current == "promo":
			m.endPromo()
			changed = true
		case !ok:
			p.Clip, changed = nil, true
		case c != *p.Clip:
			p.Clip, changed = &c, true
		}
	}
	if changed {
		m.changed()
	}
}

func clipCopy(c *PromoClip) *PromoClip {
	if c == nil {
		return nil
	}
	v := *c
	return &v
}

func sameClip(a, b *PromoClip) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func valid(name string) bool {
	for _, s := range Screens {
		if s == name {
			return true
		}
	}
	return false
}

func (m *Machine) ShowLowerThird(lt LowerThird, d time.Duration) {
	until := m.now().Add(d).UTC()
	lt.Visible, lt.Until = true, &until
	m.mu.Lock()
	m.s.LowerThird = lt
	m.changed()
	m.mu.Unlock()
}

func (m *Machine) HideLowerThird() {
	m.mu.Lock()
	m.s.LowerThird.Visible, m.s.LowerThird.Until = false, nil
	m.changed()
	m.mu.Unlock()
}

// ResetPeriods starts a new game: no recorded periods, also for after the demo.
func (m *Machine) ResetPeriods() {
	m.mu.Lock()
	m.s.Periods, m.realPeriods = []Period{}, nil
	m.finalDone, m.zeroSince = 0, time.Time{}
	m.changed()
	save := m.saver(true)
	m.mu.Unlock()
	save()
}

// RestorePeriods puts back the periods saved before a restart.
func (m *Machine) RestorePeriods(ps []Period) {
	m.mu.Lock()
	m.s.Periods = append([]Period{}, ps...)
	m.seen.periods = len(ps) // restored, not recorded now
	m.changed()
	m.mu.Unlock()
}

// saver takes a snapshot of the periods (the lock held) and returns the save
// to run after unlocking. Saves race outside the lock, so an older snapshot
// that arrives after a newer one is dropped. Only a reset saves in the demo.
func (m *Machine) saver(reset bool) func() {
	if m.SavePeriods == nil || m.s.Demo && !reset {
		return func() {}
	}
	m.saveSeq++
	seq, ps := m.saveSeq, append([]Period{}, m.s.Periods...)
	return func() {
		m.saveMu.Lock()
		defer m.saveMu.Unlock()
		if seq > m.savedSeq {
			m.savedSeq = seq
			m.SavePeriods(ps)
		}
	}
}

// ApplyConsole takes the decoder's values from one box frame.
// It is ignored while the demo runs.
func (m *Machine) ApplyConsole(v map[string]string) {
	num := func(k string) int { n, _ := strconv.Atoi(v[k]); return n }
	l := Live{
		HomeName: v["home_name"], AwayName: v["away_name"],
		Home: num("home"), Away: num("away"),
		Clock: v["clock"], Period: v["period"],
		HomeFouls: num("home_fouls"), AwayFouls: num("away_fouls"),
		Shot: v["shot_clock"], Sport: v["sport"],
	}
	m.mu.Lock()
	now := m.now().UTC()
	m.s.Box.LastFrameAt = &now
	m.s.Box.Connected = true
	if v["box_type"] != "" {
		m.s.Box.BoxType = v["box_type"]
	}
	m.mu.Unlock()
	m.update(l, false)
}

func (m *Machine) update(l Live, fromDemo bool) {
	m.mu.Lock()
	if m.s.Demo != fromDemo {
		m.mu.Unlock()
		return
	}
	if fromDemo {
		// The demo stands in for a connected box, so the overlay shows no "disconnected" dot.
		now := m.now().UTC()
		m.s.Box.Connected, m.s.Box.LastFrameAt = true, &now
	}
	recorded := m.setLive(l)
	m.changed()
	snap := m.snapshot()
	save := m.recordedSaver(recorded)
	m.mu.Unlock()
	m.recorded(recorded, snap, save)
}

// recordedSaver must be called with the lock held.
func (m *Machine) recordedSaver(rec bool) func() {
	if !rec {
		return func() {}
	}
	return m.saver(false)
}

// recorded runs the hooks after a period was recorded, outside the lock.
func (m *Machine) recorded(rec bool, snap State, save func()) {
	if rec && m.OnPeriods != nil {
		m.OnPeriods(snap)
	}
	save()
}

// setLive applies new live values and runs the period and screen rules.
// It reports whether a period score was recorded.
func (m *Machine) setLive(l Live) bool {
	old := m.s.Live
	m.s.Live = l
	if old.Clock != "" && old.Clock != l.Clock {
		m.clockRanAt = m.now()
	}
	oldP, _ := strconv.Atoi(old.Period)
	newP, _ := strconv.Atoi(l.Period)
	recorded := false

	if newP != oldP {
		if oldP >= 1 && newP == oldP+1 {
			recorded = m.record(oldP, old.Home, old.Away)
			if oldP == 2 {
				m.auto("intervalo")
			}
		}
		m.started, m.lastTenths, m.zeroSince = false, -1, time.Time{}
	}

	t := tenths(l.Clock)
	if !m.started && t >= 0 && m.lastTenths > 0 && t < m.lastTenths {
		m.started = true
		cur := m.s.Screen.Current
		if newP >= 3 && (cur == "intervalo" || cur == "final") {
			m.auto("marcador")
		}
	}
	m.lastTenths = t

	if newP >= 4 && t == 0 {
		if m.zeroSince.IsZero() {
			m.zeroSince = m.now()
		}
	} else {
		m.zeroSince = time.Time{}
	}
	return m.checkFinal() || recorded
}

// checkFinal fires the end of the game once the clock has sat at 0 long enough.
func (m *Machine) checkFinal() bool {
	p, _ := strconv.Atoi(m.s.Live.Period)
	if m.zeroSince.IsZero() || m.finalDone == p || m.now().Sub(m.zeroSince) < FinalHold {
		return false
	}
	m.finalDone = p
	m.auto("final")
	return m.record(p, m.s.Live.Home, m.s.Live.Away)
}

// record stores period p ending at home-away. Its points are the score minus
// the score at the end of the last recorded period, never below 0 (a
// correction can take the score under it).
func (m *Machine) record(p, home, away int) bool {
	prevH, prevA := 0, 0
	for _, r := range m.s.Periods {
		if r.Period >= p {
			return false
		}
		prevH, prevA = r.Home, r.Away
	}
	m.s.Periods = append(m.s.Periods, Period{p, home, away, max(home-prevH, 0), max(away-prevA, 0)})
	return true
}

func (m *Machine) auto(screen string) {
	if m.s.Screen.Auto && screen != m.s.Screen.Current {
		m.switchTo(screen)
		m.autoSwitch = true
	}
}

// Tick runs the time-based rules. Start calls it 4 times a second.
func (m *Machine) Tick() {
	m.mu.Lock()
	lt := &m.s.LowerThird
	changed := false
	if lt.Visible && lt.Until != nil && !m.now().Before(*lt.Until) {
		lt.Visible, lt.Until = false, nil
		changed = true
	}
	if c := m.s.Promo.Clip; m.s.Screen.Current == "promo" && c != nil && c.End != "loop" &&
		m.now().Sub(m.s.Screen.Since) > promoLength(c.Duration)+PromoGrace {
		m.endPromo()
		m.autoSwitch, changed = true, true
	}
	recorded := m.checkFinal()
	if changed || recorded {
		m.changed()
	}
	snap := m.snapshot()
	save := m.recordedSaver(recorded)
	m.mu.Unlock()
	m.recorded(recorded, snap, save)
}

// tenths parses "MM:SS" or "S.T" into tenths of a second, -1 if empty.
func tenths(clock string) int {
	if a, b, ok := strings.Cut(clock, ":"); ok {
		mm, _ := strconv.Atoi(strings.TrimSpace(a))
		ss, _ := strconv.Atoi(strings.TrimSpace(b))
		return (mm*60 + ss) * 10
	}
	if a, b, ok := strings.Cut(clock, "."); ok {
		ss, _ := strconv.Atoi(strings.TrimSpace(a))
		t, _ := strconv.Atoi(strings.TrimSpace(b))
		return ss*10 + t
	}
	return -1
}
