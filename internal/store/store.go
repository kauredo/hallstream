// Package store keeps settings, age groups, the current game and history as
// JSON files in the dados folder next to the program.
package store

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kauredo/hallstream/internal/club"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
)

type OBS struct {
	Enabled      bool   `json:"enabled"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Password     string `json:"password"`
	TimeoutScene string `json:"timeout_scene"`
	GameScene    string `json:"game_scene"`
	// CameraSource is the camera input in the program scene; "" picks it.
	CameraSource string `json:"camera_source"`
	CameraFit    string `json:"camera_fit"` // "contain" or "fill"
	// CameraFull is the last full-screen placement; only the camera changes it.
	CameraFull *obsws.Full `json:"camera_full"`
	// SetupAt is when "Configurar OBS" last got OBS ready; only the server sets it.
	SetupAt string `json:"setup_at"`
	// Launch starts OBS with the program (Windows); it turns on with the
	// first good setup.
	Launch      bool `json:"launch"`
	CloseOnExit bool `json:"close_on_exit"`
}

type Settings struct {
	FPBBaseURL  string `json:"fpb_base_url"`
	PhoneAccess bool   `json:"phone_access"`
	PIN         string `json:"pin"`
	OBS         OBS    `json:"obs"`
	AutoScreens bool   `json:"auto_screens"`
	Instagram   string `json:"instagram"`
	Club        Club   `json:"club"`
	Photos      Photos `json:"photos"`
	// DesktopShortcut is the answer to the first-run shortcut question: ""
	// (not asked yet), "created" or "declined". Only the server changes it.
	DesktopShortcut string `json:"desktop_shortcut"`
	// BoxAddress is the Bluetooth address of the box, saved after the first
	// good connection. Only the reader changes it.
	BoxAddress string `json:"box_address"`
	// ClubSetup is "" before the first-run club setup, "pending" until the
	// FPB has answered once, then the time it finished.
	ClubSetup string `json:"club_setup"`
	// ClubTeams are the club's FPB team ids seen in the last team list; an id
	// not seen before is a new team.
	ClubTeams []int64 `json:"club_teams"`
	// ClubSeason is the newest season in the club's team slugs
	// ("2026-2027"); a newer one means the FPB moved to a new season.
	ClubSeason string `json:"club_season"`
	// ClubSearch is the club's name on the FPB ("Clube Exemplo"). The club
	// setup searches the FPB teams with it and takes the teams whose name
	// starts with it. Empty skips the club setup.
	ClubSearch string  `json:"club_search"`
	Overlay    Overlay `json:"overlay"`
	// Credit shows the small Hallstream mark on air; on by default.
	Credit bool `json:"credit"`
	// StartWithWindows is whether the Startup folder has the shortcut; only
	// the server changes it.
	StartWithWindows bool `json:"start_with_windows"`
	// FirstSetup is the panel's "Primeira configuração"; only /api/setup changes it.
	FirstSetup FirstSetup `json:"first_setup"`
	// Lang is the language of the panel, the overlay and the log: the name
	// of a file in web/i18n ("pt"). A language without a file reads pt.
	Lang string `json:"lang"`
	// Input is where the score comes from: one of Inputs.
	Input string `json:"input"`
	// UpdateCheck lets the program ask GitHub once a day for a newer release
	// (off until the volunteer turns it on). It never downloads.
	UpdateCheck bool `json:"update_check"`
	// Stream is where OBS sends the picture; only PUT /api/stream changes it.
	Stream Stream `json:"stream"`
}

// Stream is where OBS sends the picture: the YouTube stream the program
// manages, or a custom RTMP server with its key.
type Stream struct {
	Service string `json:"service"` // ServiceYouTube or ServiceRTMP
	Server  string `json:"server"`  // rtmp:// or rtmps://, for ServiceRTMP
	Key     string `json:"key"`     // the stream key, for ServiceRTMP
}

// The values of Stream.Service.
const (
	ServiceYouTube = "youtube"
	ServiceRTMP    = "rtmp"
)

// Inputs are the values of Settings.Input, the default first: the box over
// Bluetooth, or the score kept by hand on a phone.
var Inputs = []string{"console", "manual"}

// Club is the club's profile: what the overlay shows for the home side and
// the colours it is drawn in.
type Club struct {
	Name      string `json:"name"`      // up to 12 characters
	Short     string `json:"short"`     // the sigla, up to 4
	Primary   string `json:"primary"`   // "#RRGGBB"
	Secondary string `json:"secondary"` // "#RRGGBB"
	// Crest is the crest's URL: a /media upload, or DefaultCrest. Only
	// /api/club/crest changes it.
	Crest   string `json:"crest"`
	Sport   string `json:"sport"`   // an id from club.Sports
	Country string `json:"country"` // ISO 3166 alpha-2, "PT"
	// TimeZone is an IANA zone ("Europe/Madrid"); "" takes the country's
	// (club.Zone). Fixture times typed or imported are read in it.
	TimeZone string `json:"timezone"`
}

// DefaultCrest is the neutral stand-in shield.
const DefaultCrest = "/assets/crest.svg"

// FirstSetup remembers the guided first-run steps.
type FirstSetup struct {
	Done  string `json:"done"`  // RFC 3339 when finished, "" before
	Later bool   `json:"later"` // "Agora não": no banner, the flow stays in Definições
	// Steps maps a step id to "done" (ticked by hand) or "skipped". Replace
	// the map to change it: Settings() hands out copies that share it.
	Steps map[string]string `json:"steps"`
}

// Overlay holds how the overlay is laid out.
type Overlay struct {
	// Style is the on-air look, one of Styles.
	Style string `json:"style"`
	// BugPosition is where the scoreboard sits, one of BugPositions.
	BugPosition string `json:"bug_position"`
	// LowerThird is the lower third's side, one of LowerThirds. The overlay
	// moves it out of the way of a bug at the bottom.
	LowerThird string `json:"lower_third"`
	// Margin is the gap in px between the frame's edge and the bug, the
	// lower third, the sponsor corner and the credit, for TVs that crop.
	Margin int `json:"margin"`
	// Scale is the size of the bug and the lower third, in percent.
	Scale int `json:"scale"`
	// Accent lights the bonus, the points just scored and a running-out
	// shot clock, one of Accents.
	Accent string `json:"accent"`
	// AwayColor is "own" (the opponent's colour) or "neutral" (grey).
	AwayColor string `json:"away_color"`
	// Hide lists what stays off air: "fouls", "shot" (the shot clock),
	// "strip" (the sponsor strip on the full screens).
	Hide []string `json:"hide"`
	// Photo is the hall photo behind the full screens: a /media upload, or
	// "" for the stand-in. Only /api/overlay/photo changes it.
	Photo string `json:"photo"`
	// SponsorCorner shows one sponsor at a time top-right on the scoreboard
	// screen, for 10 s every SponsorEvery minutes.
	SponsorCorner bool `json:"sponsor_corner"`
	SponsorEvery  int  `json:"sponsor_every"`
	// BackLine is the short line under "VOLTAMOS JÁ"; empty by default.
	BackLine string `json:"back_line"`
}

// SponsorEvery is the default and the limits of Overlay.SponsorEvery, in minutes.
const SponsorEvery, SponsorEveryMin, SponsorEveryMax = 4, 1, 30

// The values each Overlay setting takes, the default first.
var (
	Styles       = []string{"classico", "lombada", "cor", "placa", "pavilhao"}
	BugPositions = []string{"top-left", "top-center", "top-right", "bottom-left", "bottom-center", "bottom-right"}
	LowerThirds  = []string{"left", "center", "right"}
	Accents      = []string{"gold", "white", "orange", "lime", "cyan"}
	AwayColors   = []string{"own", "neutral"}
	Hideable     = []string{"fouls", "shot", "strip"}
)

// The default and the limits of Overlay.Margin (px) and Overlay.Scale (%).
const (
	Margin, MarginMin, MarginMax = 64, 32, 96
	Scale, ScaleMin, ScaleMax    = 100, 85, 125
)

// Clean puts every value of o back inside what the overlay knows: an
// unknown name becomes the default and a number outside its limits the
// default too. Hide keeps the known names once each, in Hideable's order.
func (o *Overlay) Clean() {
	pick := func(v *string, from []string) {
		if !slices.Contains(from, *v) {
			*v = from[0]
		}
	}
	pick(&o.Style, Styles)
	pick(&o.BugPosition, BugPositions)
	pick(&o.LowerThird, LowerThirds)
	pick(&o.Accent, Accents)
	pick(&o.AwayColor, AwayColors)
	if o.Margin < MarginMin || o.Margin > MarginMax {
		o.Margin = Margin
	}
	if o.Scale < ScaleMin || o.Scale > ScaleMax {
		o.Scale = Scale
	}
	if o.SponsorEvery < SponsorEveryMin || o.SponsorEvery > SponsorEveryMax {
		o.SponsorEvery = SponsorEvery
	}
	hide := []string{}
	for _, h := range Hideable {
		if slices.Contains(o.Hide, h) {
			hide = append(hide, h)
		}
	}
	o.Hide = hide
}

// Photos switches player photos on the squad screen and the lower third.
type Photos struct {
	Squad      bool `json:"squad"`
	LowerThird bool `json:"lower_third"`
}

func DefaultSettings() Settings {
	return Settings{
		FPBBaseURL: "https://calendariofpb.pt",
		OBS: OBS{Host: "127.0.0.1", Port: 4455,
			GameScene: "Jogo", CameraFit: "contain"},
		AutoScreens: true,
		Club: Club{Name: "CASA", Short: "CAS", Primary: club.DefaultPrimary, Secondary: club.DefaultSecondary,
			Crest: DefaultCrest, Sport: "basketball", Country: "PT"},
		Overlay: Overlay{Style: Styles[0], BugPosition: BugPositions[0], LowerThird: LowerThirds[0],
			Margin: Margin, Scale: Scale, Accent: Accents[0], AwayColor: AwayColors[0], Hide: []string{},
			SponsorCorner: true, SponsorEvery: SponsorEvery},
		Credit: true,
		Lang:   "pt",
		Input:  Inputs[0],
	}
}

type Player struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Number      string  `json:"number"`
	FPBPlayerID *int64  `json:"fpb_player_id"`
	FPBName     *string `json:"fpb_name"`
	// NumberFromFPB is true while the number came from an import, so the
	// next import may update it. Editing the number by hand clears it.
	NumberFromFPB bool `json:"number_from_fpb"`
	// PhotoURL is the local copy (/media/pl-<id>.<ext>) of the FPB photo at PhotoSource.
	PhotoURL    string `json:"photo_url"`
	PhotoSource string `json:"photo_source"`
	// FPBAthleteID follows the player into next season's team, where the
	// roster may give another player id.
	FPBAthleteID int64 `json:"fpb_athlete_id"`
	// Inactive: linked, but not in the squad of the team the group moved to.
	Inactive bool `json:"inactive"`
	// PhotoByHand: the club uploaded PhotoURL; no FPB import or CSV replaces it.
	PhotoByHand bool `json:"photo_by_hand"`
}

// SyncChanges counts what the last FPB update changed in a group.
type SyncChanges struct {
	NewPlayers int `json:"new_players"`
	Numbers    int `json:"numbers"`
	Photos     int `json:"photos"`
	Left       int `json:"left"`
	Returned   int `json:"returned"`
}

type Sponsor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Principal bool   `json:"principal"`
}

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Coach       string    `json:"coach"`
	FPBTeamID   int64     `json:"fpb_team_id"` // 0 = not linked
	FPBTeamName string    `json:"fpb_team_name"`
	Players     []Player  `json:"players"`
	Sponsors    []Sponsor `json:"sponsors"`
	// FPBTeamKey is the escalão name rule applied to the linked team
	// ("Sub 16 Masculino B"), used to find next season's team.
	FPBTeamKey string `json:"fpb_team_key"`
	// NeedsTeam: the linked team left the FPB list and no single new team matches.
	NeedsTeam   bool        `json:"needs_team"`
	LastSync    string      `json:"last_sync"` // RFC 3339, "" = never
	LastChanges SyncChanges `json:"last_changes"`
	SyncedTeam  int64       `json:"synced_team"` // FPBTeamID at the last sync
	// Fixtures are the games the club typed or imported for this escalão;
	// with an FPB team linked they add to the FPB's list.
	Fixtures []Fixture `json:"fixtures"`
	// CalendarURL is the escalão's calendar link (an .ics feed, http, https
	// or webcal); its games add to the others. "" = none.
	CalendarURL string `json:"calendar_url"`
	// Sport is an id from club.Sports; Open and AddGroup never leave it empty.
	Sport string `json:"sport"`
}

// Fixture is one game the club entered by hand.
type Fixture struct {
	ID          string   `json:"id"`
	Start       string   `json:"start"` // RFC 3339, with the offset of the club's zone
	Opponent    Opponent `json:"opponent"`
	Home        bool     `json:"home"` // the club plays at home
	Venue       string   `json:"venue"`
	Competition string   `json:"competition"`
	Round       string   `json:"round"`
}

// Shared is the group whose sponsors show for every age group.
const Shared = "geral"

type Opponent struct {
	Name     string `json:"name"`
	Short    string `json:"short"`
	Color    string `json:"color"`
	CrestURL string `json:"crest_url"`
}

type SquadEntry struct {
	PlayerID string `json:"player_id"`
	Number   string `json:"number"`
}

type Game struct {
	GroupID     string       `json:"group_id"`
	FPBGameID   *int64       `json:"fpb_game_id"` // null for a manual game
	Away        Opponent     `json:"away"`
	Start       string       `json:"start"`
	Venue       string       `json:"venue"`
	Competition string       `json:"competition"`
	Round       string       `json:"round"`
	Squad       []SquadEntry `json:"squad"`
}

// Picked reports whether a game was chosen in the panel.
func (g Game) Picked() bool { return g.GroupID != "" || g.Away.Name != "" }

var ErrNotFound error = i18n.Key("store.nao_encontrado")

type Store struct {
	Dir      string
	mu       sync.Mutex
	settings Settings
	groups   []Group
	game     Game
	promo    Promo
}

// DataDir is the dados folder next to the executable, or in the working
// directory under `go run`.
func DataDir() string {
	dir, _ := os.Getwd()
	if exe, err := os.Executable(); err == nil && !strings.Contains(exe, "go-build") {
		dir = filepath.Dir(exe)
	}
	return filepath.Join(dir, "dados")
}

func Open(dir string) (*Store, error) {
	for _, d := range []string{"media", "cache/fpb", "jogos", "score", "promo"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	s := &Store{Dir: dir, settings: DefaultSettings(), game: Game{Squad: []SquadEntry{}}, promo: Promo{Clips: []Clip{}}}
	for name, v := range map[string]any{"settings.json": &s.settings, "groups.json": &s.groups, "game.json": &s.game, "promo.json": &s.promo} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return nil, i18n.Err("store.ficheiro_estragado", "ficheiro", name, "err", err)
		}
		if name == "settings.json" {
			oldHome(b, &s.settings)
		}
	}
	// Uploads cut short by a closed program leave temp files behind.
	left, _ := filepath.Glob(filepath.Join(dir, "promo", ".up-*"))
	for _, f := range left {
		os.Remove(f)
	}
	if _, i := s.find(Shared); i < 0 {
		s.groups = append(s.groups, Group{ID: Shared, Name: i18n.T("store.grupo_geral")})
	}
	// Escalões saved before each had its own sport take the club's.
	for i := range s.groups {
		if s.groups[i].Sport == "" {
			s.groups[i].Sport = cmp.Or(s.settings.Club.Sport, "basketball")
		}
	}
	return s, nil
}

// SportOf is the sport of an escalão, else the club's main sport (no escalão,
// an unknown one, or the shared group).
func (s *Store) SportOf(groupID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, i := s.find(groupID); i >= 0 && g.ID != Shared && g.Sport != "" {
		return g.Sport
	}
	return s.settings.Club.Sport
}

// GameSport is the sport of the game's escalão, else the club's.
func (s *Store) GameSport() string { return s.SportOf(s.Game().GroupID) }

// oldHome moves home_name and home_short, from a settings.json written before
// the club profile, into the club. The next save drops the old keys.
func oldHome(b []byte, set *Settings) {
	var old struct {
		Club      *json.RawMessage `json:"club"`
		HomeName  *string          `json:"home_name"`
		HomeShort *string          `json:"home_short"`
	}
	if json.Unmarshal(b, &old) != nil || old.Club != nil {
		return
	}
	if old.HomeName != nil {
		set.Club.Name = *old.HomeName
	}
	if old.HomeShort != nil {
		set.Club.Short = *old.HomeShort
	}
}

// WriteFile writes atomically: a temp file next to it, then rename.
func WriteFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// Close waits for a save in progress and holds back any later one, so the
// program can exit without cutting a write short. It is the last call before
// exit: every Store method blocks after it.
func (s *Store) Close() { s.mu.Lock() }

func (s *Store) writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(filepath.Join(s.Dir, name), b)
}

// copyOf deep-copies through JSON so callers never share slices with the store.
func copyOf[T any](v T) T {
	var out T
	b, _ := json.Marshal(v)
	json.Unmarshal(b, &out)
	return out
}

func NewID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *Store) SaveSettings(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeJSON("settings.json", v); err != nil {
		return err
	}
	s.settings = v
	return nil
}

// SetBoxAddress saves the box address ("" forgets it).
func (s *Store) SetBoxAddress(addr string) error {
	_, err := s.UpdateSettings(func(v *Settings) { v.BoxAddress = addr })
	return err
}

// SetClubSetup saves settings.club_setup.
func (s *Store) SetClubSetup(v string) error {
	_, err := s.UpdateSettings(func(set *Settings) { set.ClubSetup = v })
	return err
}

// SetClubTeams saves settings.club_teams.
func (s *Store) SetClubTeams(ids []int64) error {
	_, err := s.UpdateSettings(func(set *Settings) { set.ClubTeams = ids })
	return err
}

// SetClubSeason saves settings.club_season.
func (s *Store) SetClubSeason(v string) error {
	_, err := s.UpdateSettings(func(set *Settings) { set.ClubSeason = v })
	return err
}

// UpdateSettings changes the settings with fn and saves, under one lock, so
// fields fn leaves alone keep their newest value.
func (s *Store) UpdateSettings(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	fn(&v)
	if err := s.writeJSON("settings.json", v); err != nil {
		return Settings{}, err
	}
	s.settings = v
	return v, nil
}

func (s *Store) Game() Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.game)
}

func (s *Store) SaveGame(g Game) error {
	if g.Squad == nil {
		g.Squad = []SquadEntry{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeJSON("game.json", g); err != nil {
		return err
	}
	s.game = copyOf(g)
	return nil
}

// Groups returns every group, the shared one included.
func (s *Store) Groups() []Group {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.groups)
}

func (s *Store) Group(id string) (Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, i := s.find(id)
	if i < 0 {
		return Group{}, ErrNotFound
	}
	return copyOf(*g), nil
}

func (s *Store) find(id string) (*Group, int) {
	for i := range s.groups {
		if s.groups[i].ID == id {
			return &s.groups[i], i
		}
	}
	return nil, -1
}

// AddGroup creates a group with an id made from its name.
func (s *Store) AddGroup(name string) (Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base := Slug(name)
	if base == "" {
		base = "escalao"
	}
	id := base
	for n := 2; ; n++ {
		if _, i := s.find(id); i < 0 {
			break
		}
		id = base + "-" + strconv.Itoa(n)
	}
	g := Group{ID: id, Name: name, Players: []Player{}, Sponsors: []Sponsor{}, Sport: cmp.Or(s.settings.Club.Sport, "basketball")}
	s.groups = append(s.groups, g)
	if err := s.writeJSON("groups.json", s.groups); err != nil {
		s.groups = s.groups[:len(s.groups)-1]
		return Group{}, err
	}
	return g, nil
}

// UpdateGroup changes one group with fn and saves. If fn fails nothing changes.
func (s *Store) UpdateGroup(id string, fn func(*Group) error) (Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, i := s.find(id)
	if i < 0 {
		return Group{}, ErrNotFound
	}
	work := copyOf(*g)
	if err := fn(&work); err != nil {
		return Group{}, err
	}
	old := *g
	s.groups[i] = work
	if err := s.writeJSON("groups.json", s.groups); err != nil {
		s.groups[i] = old
		return Group{}, err
	}
	return copyOf(work), nil
}

func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, i := s.find(id)
	if i < 0 || id == Shared {
		return ErrNotFound
	}
	players := g.Players
	groups := append(append([]Group{}, s.groups[:i]...), s.groups[i+1:]...)
	if err := s.writeJSON("groups.json", groups); err != nil {
		return err
	}
	s.groups = groups
	for _, p := range players {
		s.RemoveMedia(p.PhotoURL)
	}
	return nil
}

func (s *Store) MediaDir() string { return filepath.Join(s.Dir, "media") }
func (s *Store) CacheDir() string { return filepath.Join(s.Dir, "cache", "fpb") }

// SaveMedia stores an image in dados/media and returns its /media URL.
func (s *Store) SaveMedia(name string, data []byte) (string, error) {
	if err := WriteFile(filepath.Join(s.MediaDir(), name), data); err != nil {
		return "", err
	}
	return "/media/" + name, nil
}

// ReadMedia reads a file saved with SaveMedia, given its URL.
func (s *Store) ReadMedia(url string) ([]byte, error) {
	name, ok := strings.CutPrefix(url, "/media/")
	if !ok || name == "" || strings.ContainsAny(name, `/\`) {
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(s.MediaDir(), name))
}

// RemoveMedia deletes a file saved with SaveMedia, given its URL.
func (s *Store) RemoveMedia(url string) {
	if name, ok := strings.CutPrefix(url, "/media/"); ok && !strings.ContainsAny(name, `/\`) {
		os.Remove(filepath.Join(s.MediaDir(), name))
	}
}

// SavePeriods writes periods.json, the current game's periods, so they
// outlive a restart.
func (s *Store) SavePeriods(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON("periods.json", v)
}

// LoadPeriods reads periods.json into v. A missing file leaves v alone.
func (s *Store) LoadPeriods(v any) error { return s.load("periods.json", v) }

// SaveSets writes sets.json, the current game's finished sets (volleyball), so
// they outlive a restart.
func (s *Store) SaveSets(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON("sets.json", v)
}

// LoadSets reads sets.json into v. A missing file leaves v alone.
func (s *Store) LoadSets(v any) error { return s.load("sets.json", v) }

// SaveManual writes manual.json, the score kept by hand, so it outlives a restart.
func (s *Store) SaveManual(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON("manual.json", v)
}

// LoadManual reads manual.json into v. A missing file leaves v alone.
func (s *Store) LoadManual(v any) error { return s.load("manual.json", v) }

// HasManual reports whether manual.json exists, so a first start can tell a
// fresh install from a restart.
func (s *Store) HasManual() bool {
	_, err := os.Stat(filepath.Join(s.Dir, "manual.json"))
	return err == nil
}

func (s *Store) load(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(s.Dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SaveHistory writes jogos/<date>-<group>.json.
func (s *Store) SaveHistory(date, group string, v any) error {
	if group == "" {
		group = "jogo"
	}
	return s.writeJSON(filepath.Join("jogos", date+"-"+group+".json"), v)
}
