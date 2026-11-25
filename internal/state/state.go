// Package state holds the live state the overlay reads, plus the rules that
// change it on their own: period scores, automatic screens and the lower
// third timeout.
package state

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/sport"
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
	// Timeouts taken; only the manual scorer counts them, the console sends 0.
	HomeTimeouts int `json:"home_timeouts"`
	AwayTimeouts int `json:"away_timeouts"`
	// Volleyball: the sets each side has won and who serves ("home", "away" or "").
	HomeSets int    `json:"home_sets"`
	AwaySets int    `json:"away_sets"`
	Serve    string `json:"serve"`
	// Sets is the phone scorer's finished sets, "25-20h,18-25a" (home-away and
	// the winner's side; the winner may be missing). The machine records them
	// into State.Sets; the console sends nothing here.
	Sets string `json:"sets"`
	// PeriodEnd is where the period ends on a clock counting up, in tenths
	// (the phone scorer sends it; 0 for a countdown or a console frame).
	PeriodEnd int `json:"period_end"`
	// Suspensions are the 2-minute suspensions running, home first (the phone
	// scorer sends them; the console sends none).
	Suspensions []Suspension `json:"suspensions"`
	// Soccer (the phone scorer sends these when the sport has cards). Over is
	// the scorer's "Fim do jogo"; Stoppage the minutes of added time announced
	// for this half; Cards and Subs are the game's, in the order they happened.
	Over     bool   `json:"over"`
	Stoppage int    `json:"stoppage"`
	Cards    []Card `json:"cards"`
	Subs     []Sub  `json:"subs"`
	// setCounts is true when the frame carried home_sets or away_sets.
	setCounts bool
}

// Suspension is one running suspension; Left is "MM:SS" and Number is empty
// until the scorer adds it.
type Suspension struct {
	Team   string `json:"team"` // "home" or "away"
	ID     int    `json:"id"`
	Number string `json:"number"`
	Left   string `json:"left"`
}

// Card is one card shown: Kind is "yellow", "second_yellow" or "red", Minute
// the match minute as written ("23'", "45+2'"). ID is never reused in a run.
type Card struct {
	ID     int    `json:"id"`
	Team   string `json:"team"` // "home" or "away"
	Number string `json:"number"`
	Kind   string `json:"kind"`
	Minute string `json:"minute"`
}

// Sub is one substitution; Out and In are shirt numbers.
type Sub struct {
	ID     int    `json:"id"`
	Team   string `json:"team"`
	Out    string `json:"out"`
	In     string `json:"in"`
	Minute string `json:"minute"`
}

// SetScore is one finished set of a sets sport (volleyball). A set seeded
// from a console has no points (0-0) and only its winner.
type SetScore struct {
	Set    int    `json:"set"`
	Home   int    `json:"home"`
	Away   int    `json:"away"`
	Winner string `json:"winner"`
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
	// Credit shows the Hallstream mark bottom-right (settings.credit).
	Credit bool `json:"credit"`
	// Sport is the club's sport id; SportLabel the word on air ("BASKET").
	Sport      string `json:"sport"`
	SportLabel string `json:"sport_label"`
	// Rules is the sport's profile; the pages read the numbers from it.
	Rules sport.Profile `json:"rules"`
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
// Overlay is how the club set up the graphics; store.Overlay has the meaning
// and the values of each field.
type Overlay struct {
	Style       string   `json:"style"`
	BugPosition string   `json:"bug_position"`
	LowerThird  string   `json:"lower_third"`
	Margin      int      `json:"margin"`
	Scale       int      `json:"scale"`
	Accent      string   `json:"accent"`
	AwayColor   string   `json:"away_color"`
	Hide        []string `json:"hide"`
	Photo       string   `json:"photo"`
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
	// Soccer: Card is the kind of a "cartao" lower third; OutNumber and OutName
	// are the player going off in a "substituicao" one.
	Card      string `json:"card"`
	OutNumber string `json:"out_number"`
	OutName   string `json:"out_name"`
	// Team is "away" for an opponent's card or substitution, which the overlay
	// draws under that team's tab; "" is the home team, as for every other type.
	Team string `json:"team"`
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
	Sets       []SetScore `json:"sets"`
	Game       Game       `json:"game"`
	Screen     Screen     `json:"screen"`
	LowerThird LowerThird `json:"lower_third"`
	Camera     *Camera    `json:"camera"`
	Promo      Promo      `json:"promo"`
	Demo       bool       `json:"demo"`
	// Input is where the score comes from: "console" or "manual" (settings.input).
	Input string `json:"input"`
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
var FinalHold = time.Duration(sport.Basketball.FinalHold) * 100 * time.Millisecond

type Machine struct {
	mu     sync.Mutex
	now    func() time.Time
	s      State
	picked bool
	// rulesFor is the profile lookup; tests set it, nil means sport.Get.
	rulesFor func(id string) sport.Profile
	subs     map[chan struct{}]bool

	// OnPeriods runs (outside the lock) each time a period score is recorded.
	OnPeriods func(State)
	// SavePeriods runs (outside the lock) each time the periods change,
	// reset included, so they can outlive a restart. Never during the demo.
	SavePeriods func([]Period)
	realPeriods []Period // the game's periods while the demo plays
	// SaveSets runs with SavePeriods, from the same snapshot, so the two
	// files come from one moment.
	SaveSets func([]SetScore)
	realSets []SetScore // the game's sets while the demo plays
	saveSeq  int        // periods and sets snapshots taken for the save hooks
	saveMu   sync.Mutex
	savedSeq int // the newest snapshot the save hooks got

	// OnEvent gets each change the timeline keeps (screen, lower_third, box,
	// period). It runs with the machine locked, so it must not call it.
	OnEvent    func(event string, fields map[string]any)
	seen       seen
	autoSwitch bool // the program picked the screen, not the volunteer
	// What the timeline has told of the suspensions, cards and substitutions.
	toldSusp, toldCards, toldSubs told

	zeroSince  time.Time // clock at 0 in the last regulation period or later since then
	finalDone  int       // period whose end already fired "final"
	matchOver  bool      // a sets sport: a side has won the match and "final" fired
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
			Sets:       []SetScore{},
			Screen:     Screen{Current: "marcador", Auto: true, Since: now().UTC()},
			LowerThird: LowerThird{Type: "jogador"},
			Input:      "console",
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
	sets       int
	demo       bool
}

func (m *Machine) seenNow() seen {
	lt := m.s.LowerThird
	shown := ""
	if lt.Visible {
		shown = lt.Type + ":" + lt.Number
	}
	return seen{m.s.Screen.Current, shown, m.s.Box.Connected, len(m.s.Periods), len(m.s.Sets), m.s.Demo}
}

// told is the highest ID of one list (suspensions, cards or substitutions) the
// timeline has told. It only goes up, so an ID that goes away and comes back (an
// undo) is not news twice. A new game numbers from 1 again, which restart
// allows once the old game's entries are gone from the live state.
type told struct {
	high     int
	renumber bool
}

func suspID(x Suspension) int { return x.ID }
func cardID(x Card) int       { return x.ID }
func subID(x Sub) int         { return x.ID }

// highestID is the highest ID in list, 0 when it is empty.
func highestID[T any](list []T, id func(T) int) int {
	n := 0
	for _, x := range list {
		n = max(n, id(x))
	}
	return n
}

// news is the entries of live above the mark, and moves the mark up to the
// highest ID live holds. The scorer never gives an ID out twice in a run, so a
// card entered after an undo is news.
func news[T any](k *told, live []T, id func(T) int) []T {
	top := highestID(live, id)
	if k.renumber && top < k.high {
		k.high, k.renumber = 0, false
	}
	var fresh []T
	for _, x := range live {
		if id(x) > k.high {
			fresh = append(fresh, x)
		}
	}
	k.high = max(k.high, top)
	return fresh
}

// restart is a new game: what the live state holds still is the last game's.
func (k *told) restart(top int) { k.high, k.renumber = top, top > 0 }

// prime marks what a restored scorer holds as told.
func (k *told) prime(top int) { k.high, k.renumber = top, false }

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
	live := m.s.Live
	newSusp := news(&m.toldSusp, live.Suspensions, suspID)
	newCards := news(&m.toldCards, live.Cards, cardID)
	newSubs := news(&m.toldSubs, live.Subs, subID)
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
	for i := old.sets; i < cur.sets; i++ {
		s := m.s.Sets[i]
		m.OnEvent("set", map[string]any{"set": s.Set, "home": s.Home, "away": s.Away})
	}
	for _, x := range newSusp {
		m.OnEvent("suspension", map[string]any{"team": x.Team, "number": x.Number})
	}
	for _, x := range newCards {
		m.OnEvent("card", map[string]any{"team": x.Team, "number": x.Number, "kind": x.Kind, "minute": x.Minute})
	}
	for _, x := range newSubs {
		m.OnEvent("sub", map[string]any{"team": x.Team, "out": x.Out, "in": x.In, "minute": x.Minute})
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
	s.Sets = append([]SetScore{}, m.s.Sets...)
	s.Promo.Clip, s.Promo.Mini = clipCopy(m.s.Promo.Clip), clipCopy(m.s.Promo.Mini)
	g := m.s.Game
	if g.Rules.ID == "" {
		g.Rules = m.rules()
	}
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

// SetInput says where the score comes from now. The new input is not
// connected until it says so.
func (m *Machine) SetInput(name string) {
	m.mu.Lock()
	if m.s.Input != name {
		m.s.Input, m.s.Box.Connected = name, false
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
		return i18n.Err("estado.ecra_desconhecido")
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

// ResetPeriods starts a new game: no recorded periods or sets, the match not
// over, also for after the demo. The phone scorer keeps its own sets, so a
// caller that resets a sets game resets the scorer too (Server.newGame).
func (m *Machine) ResetPeriods() {
	m.mu.Lock()
	m.s.Periods, m.realPeriods = []Period{}, nil
	m.s.Sets, m.realSets = []SetScore{}, nil
	m.finalDone, m.matchOver, m.zeroSince = 0, false, time.Time{}
	// The last game's final is not this game's: a program-picked screen goes back to the scoreboard.
	if m.s.Screen.Current == "final" {
		m.auto("marcador")
	}
	// A new game numbers its suspensions from 1 again.
	l := m.s.Live
	m.toldSusp.restart(highestID(l.Suspensions, suspID))
	m.toldCards.restart(highestID(l.Cards, cardID))
	m.toldSubs.restart(highestID(l.Subs, subID))
	m.changed()
	save := m.saver(true)
	m.mu.Unlock()
	save()
}

// RestoreSets puts back the sets saved before a restart.
func (m *Machine) RestoreSets(ss []SetScore) {
	m.mu.Lock()
	m.s.Sets = append([]SetScore{}, ss...)
	m.seen.sets = len(ss) // restored, not recorded now
	m.changed()
	m.mu.Unlock()
}

// Prime loads the live values the scorer had before a restart, without telling
// the timeline anything: what it holds now is not news. SetGame calls changed()
// before the scorer's first frame, so what Prime loads goes into seen as well.
func (m *Machine) Prime(l Live) {
	m.mu.Lock()
	m.s.Live = l
	m.seen = m.seenNow()
	m.toldSusp.prime(highestID(l.Suspensions, suspID))
	m.toldCards.prime(highestID(l.Cards, cardID))
	m.toldSubs.prime(highestID(l.Subs, subID))
	m.mu.Unlock()
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
	if m.SavePeriods == nil && m.SaveSets == nil || m.s.Demo && !reset {
		return func() {}
	}
	m.saveSeq++
	seq, ps, ss := m.saveSeq, append([]Period{}, m.s.Periods...), append([]SetScore{}, m.s.Sets...)
	return func() {
		m.saveMu.Lock()
		defer m.saveMu.Unlock()
		if seq > m.savedSeq {
			m.savedSeq = seq
			if m.SavePeriods != nil {
				m.SavePeriods(ps)
			}
			if m.SaveSets != nil {
				m.SaveSets(ss)
			}
		}
	}
}

// maxLiveSusp is the most suspensions a frame can carry (three a team).
const maxLiveSusp = 6

// MaxLiveList is the most cards, and the most substitutions, a frame can carry
// (the scorer refuses more).
const MaxLiveList = 100

// decodeList reads a JSON list out of a frame's value, keeping up to limit entries
// whose team is home or away. A value that is not JSON reads as none.
func decodeList[T any](raw string, team func(T) string, limit int) []T {
	var list, out []T
	if json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	for _, x := range list {
		if t := team(x); (t == "home" || t == "away") && len(out) < limit {
			out = append(out, x)
		}
	}
	return out
}

// LiveOf reads one frame's values (the keys ApplyConsole documents) into a Live.
// A suspensions, cards or subs list that is not JSON reads as none; entries
// beyond the sixth suspension (the hundredth card or sub) and entries for a team
// that is neither home nor away are dropped.
func LiveOf(v map[string]string) Live {
	num := func(k string) int { n, _ := strconv.Atoi(v[k]); return n }
	l := Live{
		HomeName: v["home_name"], AwayName: v["away_name"],
		Home: num("home"), Away: num("away"),
		Clock: v["clock"], Period: v["period"],
		HomeFouls: num("home_fouls"), AwayFouls: num("away_fouls"),
		Shot: v["shot_clock"], Sport: v["sport"],
		HomeTimeouts: num("home_timeouts"), AwayTimeouts: num("away_timeouts"),
		HomeSets: num("home_sets"), AwaySets: num("away_sets"), Serve: v["serve"], Sets: v["sets"],
		PeriodEnd: num("period_end"),
	}
	l.Suspensions = decodeList(v["suspensions"], func(x Suspension) string { return x.Team }, maxLiveSusp)
	l.Over, l.Stoppage = v["over"] == "1", num("stoppage")
	l.Cards = decodeList(v["cards"], func(x Card) string { return x.Team }, MaxLiveList)
	l.Subs = decodeList(v["subs"], func(x Sub) string { return x.Team }, MaxLiveList)
	_, hasHome := v["home_sets"]
	_, hasAway := v["away_sets"]
	l.setCounts = hasHome || hasAway
	return l
}

// ApplyConsole takes the values of one frame from the score input: the
// decoder's for the box, the scorer's for manual. It is ignored while the
// demo runs.
func (m *Machine) ApplyConsole(v map[string]string) {
	l := LiveOf(v)
	m.mu.Lock()
	now := m.now().UTC()
	m.s.Box.LastFrameAt = &now
	// A box frame shows the box is there. The manual scorer's frames come
	// from this program, so only the phone calling in counts (SetBox).
	if m.s.Input != "manual" {
		m.s.Box.Connected = true
	}
	if v["box_type"] != "" {
		m.s.Box.BoxType = v["box_type"]
	}
	m.mu.Unlock()
	m.update(l, false)
}

// parseSets reads the phone scorer's finished sets, "25-20h,18-25a": home-away
// points and, after them, the winner's side (h or a). Without the letter the
// side with more points won, and a tie has no winner yet (setSets takes it
// from the counts). A part that is not two numbers is skipped.
func parseSets(s string) []SetScore {
	var out []SetScore
	for _, part := range strings.Split(s, ",") {
		win := ""
		switch {
		case strings.HasSuffix(part, "h"):
			win, part = "home", strings.TrimSuffix(part, "h")
		case strings.HasSuffix(part, "a"):
			win, part = "away", strings.TrimSuffix(part, "a")
		}
		a, b, ok := strings.Cut(part, "-")
		h, errH := strconv.Atoi(a)
		w, errW := strconv.Atoi(b)
		if !ok || errH != nil || errW != nil {
			continue
		}
		switch {
		case win != "":
		case h > w:
			win = "home"
		case w > h:
			win = "away"
		}
		out = append(out, SetScore{Set: len(out) + 1, Home: h, Away: w, Winner: win})
	}
	return out
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
// It reports whether a period or set score was recorded.
func (m *Machine) setLive(l Live) bool {
	old := m.s.Live
	m.s.Live = l
	if old.Clock != "" && old.Clock != l.Clock {
		m.clockRanAt = m.now()
	}
	oldP, _ := strconv.Atoi(old.Period)
	newP, _ := strconv.Atoi(l.Period)
	r := m.rules()
	if r.Sets() {
		return m.setSets(old, l)
	}
	recorded := false

	if newP != oldP {
		if oldP >= 1 && newP == oldP+1 {
			recorded = m.record(oldP, old.Home, old.Away)
			if oldP == r.Half {
				m.auto("intervalo")
			}
		}
		m.started, m.lastTenths, m.zeroSince = false, -1, time.Time{}
	}

	t := Tenths(l.Clock)
	up := r.CountUp()
	began := t >= 0 && m.lastTenths > 0 && t < m.lastTenths
	if up {
		began = t >= 0 && m.lastTenths >= 0 && t > m.lastTenths
	}
	if !m.started && began {
		m.started = true
		cur := m.s.Screen.Current
		if newP > r.Half && (cur == "intervalo" || cur == "final") {
			m.auto("marcador")
		}
	}
	m.lastTenths = t

	if m.holding(l, newP, t, r) {
		if m.zeroSince.IsZero() {
			m.zeroSince = m.now()
		}
	} else {
		m.zeroSince = time.Time{}
	}
	if r.Cards && m.over(l, newP) {
		recorded = true
	}
	return m.checkFinal() || recorded
}

// over is setLive's rule for a sport whose scorer ends the game itself (soccer):
// "Fim do jogo" shows "final" and records the last period, once. Taking it back
// (an undo, the clock started again, a period back) puts "marcador" on again and
// drops the period it recorded, so ending the game again records it once. It
// reports whether the recorded periods changed.
func (m *Machine) over(l Live, p int) bool {
	switch {
	case l.Over && m.finalDone != p:
		m.finalDone = p
		m.auto("final")
		return m.record(p, l.Home, l.Away)
	case !l.Over && m.finalDone != 0:
		last := m.finalDone
		m.finalDone = 0
		if m.s.Screen.Current == "final" {
			m.auto("marcador")
		}
		kept := make([]Period, 0, len(m.s.Periods))
		for _, x := range m.s.Periods {
			if x.Period != last {
				kept = append(kept, x)
			}
		}
		dropped := len(kept) != len(m.s.Periods)
		m.s.Periods = kept
		return dropped
	}
	return false
}

// holding reports whether the clock sits at the end of the game: a countdown
// at 0 in the last period or later; a count-up that stops at the end of its
// period at (or past) that end in the last period or later. A count-up that
// runs on (soccer) never holds.
func (m *Machine) holding(l Live, period, t int, r sport.Profile) bool {
	if period < r.Periods {
		return false
	}
	if !r.CountUp() {
		return t == 0
	}
	if !r.StopAtEnd {
		return false
	}
	end := l.PeriodEnd
	if end == 0 {
		end = r.End(period, r.Length)
	}
	return end > 0 && t >= end
}

// setSets is setLive for a sets sport: it records a set when the count of sets
// won rises, drops sets when it falls (an undo) and ends the match when a side
// has won SetsToWin sets. A frame from the phone scorer carries every finished
// set's score in l.Sets; the console only sends the counts, so a set it ends is
// recorded with the points that were on the board before the count rose. A
// frame with no set counts at all (a console that has not sent them) leaves
// everything as it is. Reports whether the recorded sets changed.
func (m *Machine) setSets(old, l Live) bool {
	if !l.setCounts {
		return false
	}
	r := m.rules()
	most := 2*r.SetsToWin - 1
	hs, as := min(max(l.HomeSets, 0), most), min(max(l.AwaySets, 0), most)
	m.s.Live.HomeSets, m.s.Live.AwaySets = hs, as
	total, have := min(hs+as, most), len(m.s.Sets)
	finished := parseSets(l.Sets)
	changed := false
	if total > have {
		home := 0
		for _, s := range m.s.Sets {
			if s.Winner == "home" {
				home++
			}
		}
		for n := have + 1; n <= total; n++ {
			s := SetScore{Set: n}
			if n <= len(finished) {
				s = finished[n-1]
				s.Set = n
			} else if n == total && total == have+1 {
				s.Home, s.Away = old.Home, old.Away
			}
			if s.Winner == "" { // a tie, or no word on the winner: the counts say who
				s.Winner = "away"
				if home < hs {
					s.Winner = "home"
				}
			}
			if s.Winner == "home" {
				home++
			}
			m.s.Sets = append(m.s.Sets, s)
		}
		changed = true
	} else if total < have {
		m.s.Sets = append([]SetScore{}, m.s.Sets[:total]...)
		changed = true
	}
	won := hs >= r.SetsToWin || as >= r.SetsToWin
	switch {
	case won && !m.matchOver:
		m.matchOver = true
		m.auto("final")
	case !won && m.matchOver:
		m.matchOver = false
		if m.s.Screen.Current == "final" {
			m.auto("marcador")
		}
	}
	return changed
}

// rules is the profile of the game's sport. Call it with the lock held.
func (m *Machine) rules() sport.Profile {
	if m.rulesFor != nil {
		return m.rulesFor(m.s.Game.Sport)
	}
	return sport.Get(m.s.Game.Sport)
}

// checkFinal fires the end of the game once the clock has sat at 0 long enough.
func (m *Machine) checkFinal() bool {
	p, _ := strconv.Atoi(m.s.Live.Period)
	r := m.rules()
	if r.FinalHold == 0 || m.zeroSince.IsZero() || m.finalDone == p || m.now().Sub(m.zeroSince) < time.Duration(r.FinalHold)*100*time.Millisecond {
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

// ClockText formats tenths the way the box does: "MM:SS", or "S.T" under a
// minute. A clock counting down shows a second until it has all gone, so
// 9:59.5 left is "10:00".
func ClockText(t int) string {
	if t >= 600 {
		s := (t + 9) / 10
		return fmt.Sprintf("%02d:%02d", s/60, s%60)
	}
	return fmt.Sprintf("%d.%d", t/10, t%10)
}

// ClockTextUp formats the tenths of a clock counting up: the whole seconds
// played, always "MM:SS" ("00:12", "30:00", "59:59").
func ClockTextUp(t int) string {
	s := max(t, 0) / 10
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// Tenths parses "MM:SS" or "S.T" into tenths of a second, -1 if empty.
func Tenths(clock string) int {
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
