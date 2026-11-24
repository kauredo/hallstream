// Package web is the HTTP server: overlay, panel, SSE and the panel API.
package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/gamelog"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsapp"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/youtube"
)

type Server struct {
	Store   *store.Store
	State   *state.Machine
	FPB     *fpb.Client
	OBS     *obsws.Client
	Camera  *obsws.Camera // set by New
	BLE     *ble.Reader   // nil in demo mode
	Files   fs.FS         // holds overlay/, painel/, fonts/, assets/
	Version string
	Port    int
	Log     func(string)
	// Shortcut makes the desktop shortcut and returns its path; nil where
	// the system has none (anything but Windows).
	Shortcut func() (string, error)
	// Autostart adds or removes the shortcut in the Startup folder; nil
	// where the system has none.
	Autostart func(on bool) error
	OBSApp    obsapp.App
	// UploadToken lets the program send each stream's logs; "" never sends.
	UploadToken string
	Timeline    *gamelog.Timeline // nil writes nothing
	// UseInput switches the score input to settings.input's new value; nil
	// in tests.
	UseInput func(name string)
	// Manual is the score kept by hand from /marcar; nil answers 503.
	Manual *source.Manual

	mux        *http.ServeMux
	failMu     sync.Mutex
	setupMu    sync.Mutex     // one club setup or daily update at a time
	bg         sync.WaitGroup // background updates (tests wait on it)
	progMu     sync.Mutex
	prog       setupProgress
	camErr     error      // overlay.css lacks the time-out window
	promoMu    sync.Mutex // SyncPromo
	nextMu     sync.Mutex // one RefreshNext at a time
	nextQueued atomic.Bool
	fixMu      sync.Mutex
	fix        fixtures
	syncMu     sync.Mutex // SyncGame reads and publishes as one step
	setupOBSMu sync.Mutex // one Configurar OBS or health check at a time
	launchMu   sync.Mutex // never start OBS twice
	obsMu      sync.Mutex
	health     obsHealth
	stream     *obsws.Stream // nil when OBS can't be asked

	ytMu        sync.Mutex
	yt          ytSaved
	ytRecs      []ytRecord
	ytLogin     *youtube.Login // a sign-in waiting for the browser
	ytErr       string
	ytEnded     string          // the broadcast of the post-stream card
	ytBusy      map[string]bool // broadcasts being finished
	ytAgain     map[string]bool // finish once more when the run ends
	ytRemote    []youtube.Broadcast
	ytRemoteAt  time.Time
	ytRemoteErr string
	ytSleep     func(time.Duration) // tests skip the waits

	uploads  *gamelog.Uploader
	logMu    sync.Mutex
	logStart time.Time // when the stream on air started; zero off air
	lastGame string    // the game last put in the timeline (under syncMu)
}

func New(s *Server) *Server {
	if s.Log == nil {
		s.Log = func(string) {}
	}
	s.mux = http.NewServeMux()
	s.newCamera()
	s.health.Problems = []string{}
	s.ytBusy, s.ytAgain = map[string]bool{}, map[string]bool{}
	if s.ytSleep == nil {
		s.ytSleep = time.Sleep
	}
	s.loadYouTube()
	s.newUploads()
	if s.OBS != nil {
		s.OBS.OnConnect = s.refreshOBS
	}
	s.routes()
	s.bleRoutes()
	s.cameraRoutes()
	s.appRoutes()
	s.promoRoutes()
	s.obsRoutes()
	s.youtubeRoutes()
	s.setupRoutes()
	return s
}

// once runs fn in the background, counted in bg. A panic in fn goes to the
// log and the program carries on; fn is not started again.
func (s *Server) once(name string, fn func()) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		defer func() {
			if r := recover(); r != nil {
				s.Log(i18n.T("log.erro_inesperado", "tarefa", name, "erro", r) + "\n" + string(debug.Stack()))
			}
		}()
		fn()
	}()
}

// The ports the server tries, in order.
const FirstPort, LastPort = 8765, 8775

// Listen opens 127.0.0.1 (or every interface with phone access) on the
// first free port from 8765 to 8775.
func Listen(phone bool) (net.Listener, int, error) {
	host := "127.0.0.1"
	if phone {
		host = "0.0.0.0"
	}
	for p := FirstPort; p <= LastPort; p++ {
		if l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p))); err == nil {
			return l, p, nil
		}
	}
	return nil, 0, i18n.Err("log.portas_ocupadas")
}

func (s *Server) routes() {
	m := s.mux
	files := http.FileServerFS(s.Files)
	// A new exe brings new scripts: browsers and OBS must ask again each time.
	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	page := func(file string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, s.Files, file)
		}
	}
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/painel", http.StatusFound)
	})
	m.HandleFunc("GET /overlay", page("overlay/index.html"))
	m.HandleFunc("GET /painel", page("painel/index.html"))
	s.marcarRoutes(m, page)
	for _, dir := range []string{"/overlay/", "/painel/", "/marcar/", "/i18n/", "/fonts/", "/assets/"} {
		m.Handle("GET "+dir, static)
	}
	m.HandleFunc("GET /i18n.js", words)
	media := http.StripPrefix("/media/", http.FileServer(http.Dir(s.Store.MediaDir())))
	m.HandleFunc("GET /media/", func(w http.ResponseWriter, r *http.Request) {
		// Logos come from outside (FPB, uploads): an SVG must never run scripts.
		w.Header().Set("Content-Security-Policy", "script-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		media.ServeHTTP(w, r)
	})

	m.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.State.JSON())
	})
	m.HandleFunc("GET /api/events", s.events)
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) { reply(w, forCaller(s.Store.Settings(), r)) })
	m.HandleFunc("PUT /api/settings", s.putSettings)
	m.HandleFunc("POST /api/screen", s.screen)
	m.HandleFunc("POST /api/lower-third", s.lowerThird)
	m.HandleFunc("DELETE /api/lower-third", func(w http.ResponseWriter, r *http.Request) {
		s.State.HideLowerThird()
		ok(w)
	})
	m.HandleFunc("POST /api/obs/test", func(w http.ResponseWriter, r *http.Request) {
		var o store.OBS
		if !decode(w, r, &o) {
			return
		}
		if o.Host == "" || o.Port == 0 || !isLoopback(r.RemoteAddr) {
			// Nothing sent, or a phone, which never sees the password: test the saved settings.
			o = s.Store.Settings().OBS
		}
		res := map[string]any{"connected": true, "error": ""}
		if err := obsws.Test(obsws.Config{Host: o.Host, Port: o.Port, Password: o.Password}); err != nil {
			res["connected"], res["error"] = false, err.Error()
		}
		reply(w, res)
	})
	m.HandleFunc("POST /api/demo", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &req) {
			return
		}
		s.State.SetDemo(req.On)
		ok(w)
	})

	m.HandleFunc("GET /api/groups", s.listGroups)
	m.HandleFunc("POST /api/groups", s.addGroup)
	m.HandleFunc("PUT /api/groups/{id}", s.putGroup)
	m.HandleFunc("DELETE /api/groups/{id}", s.deleteGroup)
	m.HandleFunc("GET /api/groups/{id}/players", s.listPlayers)
	m.HandleFunc("POST /api/groups/{id}/players", s.addPlayer)
	m.HandleFunc("PUT /api/groups/{id}/players/{pid}", s.putPlayer)
	m.HandleFunc("DELETE /api/groups/{id}/players/{pid}", s.deletePlayer)
	m.HandleFunc("POST /api/groups/{id}/players/import", s.importPlayers)
	m.HandleFunc("POST /api/groups/{id}/players/{pid}/link", s.linkPlayer)
	m.HandleFunc("DELETE /api/groups/{id}/players/{pid}/link", s.unlinkPlayer)
	m.HandleFunc("POST /api/groups/{id}/players/from-fpb", s.playerFromFPB)
	m.HandleFunc("GET /api/groups/{id}/sponsors", s.listSponsors)
	m.HandleFunc("POST /api/groups/{id}/sponsors", s.addSponsor)
	m.HandleFunc("PUT /api/groups/{id}/sponsors", s.orderSponsors)
	m.HandleFunc("DELETE /api/groups/{id}/sponsors/{sid}", s.deleteSponsor)

	m.HandleFunc("GET /api/fpb/teams", func(w http.ResponseWriter, r *http.Request) {
		s.proxy(w, fpb.TeamsPath(r.URL.Query().Get("name")))
	})
	m.HandleFunc("GET /api/fpb/teams/{id}/next-games", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := teamID(w, r); ok {
			s.proxy(w, fpb.NextGamesPath(id))
		}
	})
	m.HandleFunc("GET /api/fpb/teams/{id}/roster", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := teamID(w, r); ok {
			s.proxy(w, fpb.RosterPath(id))
		}
	})

	m.HandleFunc("POST /api/club/setup", s.clubSetup)
	m.HandleFunc("POST /api/club/crest", s.clubCrest)
	m.HandleFunc("DELETE /api/club/crest", func(w http.ResponseWriter, r *http.Request) { s.setCrest(w, store.DefaultCrest) })

	m.HandleFunc("GET /api/game", func(w http.ResponseWriter, r *http.Request) { reply(w, s.Store.Game()) })
	m.HandleFunc("PUT /api/game", s.putGame)
	m.HandleFunc("POST /api/game/opponent-logo", s.opponentLogo)
	m.HandleFunc("POST /api/game/reset", func(w http.ResponseWriter, r *http.Request) {
		s.State.ResetPeriods()
		ok(w)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.localHost(r) {
		// Any other Host is a page that pointed its own name at this PC (DNS rebinding).
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !s.sameSite(w, r) || !s.allowed(w, r) {
		return
	}
	s.mux.ServeHTTP(w, r)
	// Settings, groups and the game feed the overlay: refresh after any write.
	if r.Method != http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") {
		s.SyncGame()
		s.SyncPromo()
	}
}

// allowed applies the PIN rule: /api/* from other devices needs the PIN
// (cookie "pin" or header X-PIN), even after phone access is turned off,
// since the server keeps listening on every interface until it restarts.
// The pages hold no data and the PC itself never needs the PIN.
func (s *Server) allowed(w http.ResponseWriter, r *http.Request) bool {
	set := s.Store.Settings()
	if isLoopback(r.RemoteAddr) || !strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	pin := r.Header.Get("X-PIN")
	if c, err := r.Cookie("pin"); err == nil && pin == "" {
		pin = c.Value
	}
	if set.PIN != "" && subtle.ConstantTimeCompare([]byte(pin), []byte(set.PIN)) == 1 {
		return true
	}
	if pin != "" {
		// One wrong PIN per second, across all requests, so 4 digits can't be guessed quickly.
		s.failMu.Lock()
		time.Sleep(time.Second)
		s.failMu.Unlock()
	}
	fail(w, http.StatusUnauthorized, i18n.T("api.pin_errado"))
	return false
}

// localHost reports whether the Host names this PC: localhost, a loopback
// address or, with phone access on, an address of this PC.
func (s *Server) localHost(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	return host == "localhost" || net.ParseIP(host).IsLoopback() || s.Store.Settings().PhoneAccess && isLocalIP(host)
}

// sameSite refuses API writes another web page could make: an Origin of
// another site and body types a plain HTML form can send (multipart only
// where uploads use it).
func (s *Server) sameSite(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || !strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != r.Host {
			fail(w, http.StatusForbidden, i18n.T("api.outra_pagina"))
			return false
		}
	}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	upload := r.Method == http.MethodPost && (r.URL.Path == "/api/game/opponent-logo" || r.URL.Path == "/api/club/crest" || r.URL.Path == "/api/promo" || r.URL.Path == "/api/youtube/thumbnail" ||
		strings.HasPrefix(r.URL.Path, "/api/groups/") && strings.HasSuffix(r.URL.Path, "/sponsors"))
	if ct != "application/json" && !(upload && ct == "multipart/form-data") {
		fail(w, http.StatusUnsupportedMediaType, i18n.T("api.tipo_invalido"))
		return false
	}
	return true
}

var interfaceAddrs = net.InterfaceAddrs // tests stand in a LAN address

// isLocalIP reports whether ip is loopback or an address of this PC.
func isLocalIP(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	if p.IsLoopback() {
		return true
	}
	addrs, _ := interfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(p) {
			return true
		}
	}
	return false
}

// LanIP is the first private IPv4 address, for the phone link.
func LanIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.IsPrivate() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	return ""
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, okf := w.(http.Flusher)
	if !okf {
		fail(w, http.StatusInternalServerError, i18n.T("api.sem_streaming"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := s.State.Subscribe()
	defer cancel()
	send := func() {
		fmt.Fprintf(w, "event: state\ndata: %s\n\n", s.State.JSON())
		fl.Flush()
	}
	send()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			send()
			// At most 20 messages a second; changes meanwhile wait in ch.
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	reachable, cachedAt := s.FPB.Status()
	var cached *time.Time
	if !cachedAt.IsZero() {
		cached = &cachedAt
	}
	lan := ""
	if s.Store.Settings().PhoneAccess {
		if ip := LanIP(); ip != "" {
			lan = fmt.Sprintf("http://%s:%d/painel", ip, s.Port)
		}
	}
	reply(w, map[string]any{
		"version":     s.Version,
		"port":        s.Port,
		"overlay_url": fmt.Sprintf("http://127.0.0.1:%d/overlay", s.Port),
		"lan_url":     lan,
		"box":         s.State.Snapshot().Box,
		"obs":         s.obsStatus(),
		"youtube":     s.ytStatus(isLoopback(r.RemoteAddr)),
		"fpb":         map[string]any{"reachable": reachable, "cached_at": cached},
		"club_setup":  s.progress(),
		"data_dir":    s.Store.Dir,
		// The panel offers the desktop shortcut on the PC itself only.
		"shortcut":  s.Shortcut != nil && isLoopback(r.RemoteAddr),
		"autostart": s.Autostart != nil && isLoopback(r.RemoteAddr),
		// This system can start and close OBS by itself.
		"obs_app": s.OBSApp.Start != nil,
		"uploads": s.uploads.Status(),
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	req := s.Store.Settings()
	// Decoding would merge into the steps map the store shares with req.
	req.FirstSetup = store.FirstSetup{}
	if !decode(w, r, &req) {
		return
	}
	if req.PhoneAccess && !fourDigits(req.PIN) {
		fail(w, http.StatusBadRequest, i18n.T("api.pin_4"))
		return
	}
	if req.FPBBaseURL == "" {
		req.FPBBaseURL = store.DefaultSettings().FPBBaseURL
	}
	if req.Overlay.SponsorEvery < store.SponsorEveryMin || req.Overlay.SponsorEvery > store.SponsorEveryMax {
		req.Overlay.SponsorEvery = store.SponsorEvery
	}
	req.Lang = i18n.Clean(req.Lang)
	if err := checkClub(&req.Club); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var old store.Settings
	set, err := s.Store.UpdateSettings(func(cur *store.Settings) {
		old = *cur
		// The panel owns the rest; these change elsewhere meanwhile.
		req.BoxAddress, req.ClubSetup, req.ClubTeams, req.ClubSeason = cur.BoxAddress, cur.ClubSetup, cur.ClubTeams, cur.ClubSeason
		if !slices.Contains(store.BugPositions, req.Overlay.BugPosition) {
			req.Overlay.BugPosition = store.BugPositions[0]
		}
		if !slices.Contains(store.Inputs, req.Input) {
			req.Input = store.Inputs[0]
		}
		req.Overlay.BackLine = cutRunes(strings.Join(strings.Fields(req.Overlay.BackLine), " "), maxBackLine)
		req.DesktopShortcut, req.StartWithWindows, req.FirstSetup = cur.DesktopShortcut, cur.StartWithWindows, cur.FirstSetup
		req.OBS.SetupAt = cur.OBS.SetupAt
		req.Club.Crest = cur.Club.Crest // /api/club/crest owns it
		if !isLoopback(r.RemoteAddr) {
			// What starts programs on the PC, or reaches OBS, is set on the PC only.
			req.OBS.Host, req.OBS.Port, req.OBS.Password = cur.OBS.Host, cur.OBS.Port, cur.OBS.Password
			req.OBS.Launch, req.OBS.CloseOnExit = cur.OBS.Launch, cur.OBS.CloseOnExit
		}
		req.OBS.CameraFull = cur.OBS.CameraFull
		if req.OBS.CameraSource != cur.OBS.CameraSource {
			req.OBS.CameraFull = nil // refit the new camera
		}
		if req.OBS.CameraFit != "fill" {
			req.OBS.CameraFit = "contain"
		}
		*cur = req
	})
	if !s.save(w, err) {
		return
	}
	if old.Lang != set.Lang {
		i18n.SetLang(set.Lang)
	}
	if old.AutoScreens != set.AutoScreens {
		s.State.SetScreen("", set.AutoScreens)
	}
	if old.Input != set.Input && s.UseInput != nil {
		s.UseInput(set.Input)
	}
	if old.PhoneAccess != set.PhoneAccess {
		s.Log(i18n.T("log.telemovel_depois"))
	}
	reply(w, forCaller(set, r))
}

// words is /i18n.js: window.I18N, every string of the panel and the overlay
// in settings.lang, with pt under it, so the pages have them before they draw.
func words(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(i18n.Words())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, "window.I18N = %s;\n", b)
}

// forCaller hides the OBS password from other devices: OBS's server listens
// on the whole network, and the password is all that guards it.
func forCaller(set store.Settings, r *http.Request) store.Settings {
	if !isLoopback(r.RemoteAddr) {
		set.OBS.Password = ""
	}
	return set
}

// maxBackLine fits the line under VOLTAMOS JÁ on one row.
const maxBackLine = 60

func fourDigits(pin string) bool {
	if len(pin) != 4 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Screen string `json:"screen"`
		Auto   *bool  `json:"auto"`
	}
	if !decode(w, r, &req) {
		return
	}
	auto := s.State.Snapshot().Screen.Auto
	if req.Auto != nil {
		auto = *req.Auto
	}
	if g := s.State.Snapshot().Game; req.Screen == "proximo" && g.Next == nil {
		fail(w, http.StatusBadRequest, i18n.T("api.sem_proximo", "motivo", g.NextNote))
		return
	}
	if err := s.State.SetScreen(req.Screen, auto); err != nil {
		fail(w, http.StatusBadRequest, i18n.T("api.ecra_desconhecido", "ecra", req.Screen))
		return
	}
	if req.Auto != nil {
		s.Store.UpdateSettings(func(set *store.Settings) { set.AutoScreens = auto })
	}
	resp := map[string]any{"ok": true}
	if s.Store.Settings().OBS.Enabled && req.Screen != "" {
		if err := s.followCamera(); err != nil {
			resp["obs_error"] = err.Error()
		}
	}
	reply(w, resp)
}

func (s *Server) lowerThird(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  string `json:"player_id"`
		Type      string `json:"type"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Label     string `json:"label"`
		Seconds   int    `json:"seconds"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Seconds <= 0 {
		req.Seconds = 8
	}
	lt := state.LowerThird{Type: "comentador", FirstName: req.FirstName, LastName: req.LastName, Label: req.Label}
	if req.PlayerID != "" {
		game := s.Store.Game()
		g, err := s.Store.Group(game.GroupID)
		p := findPlayer(g.Players, req.PlayerID)
		if err != nil || p == nil {
			fail(w, http.StatusNotFound, i18n.T("api.jogador_fora_escalao"))
			return
		}
		number := p.Number
		for _, e := range game.Squad {
			if e.PlayerID == p.ID && e.Number != "" {
				number = e.Number
			}
		}
		first, last, _ := strings.Cut(strings.TrimSpace(p.DisplayName), " ")
		lt = state.LowerThird{Type: "jogador", Number: number, FirstName: first, LastName: strings.TrimSpace(last), PhotoURL: p.PhotoURL}
	} else if req.Type != "comentador" {
		fail(w, http.StatusBadRequest, i18n.T("api.rodape_sem_nome"))
		return
	}
	s.State.ShowLowerThird(lt, time.Duration(req.Seconds)*time.Second)
	ok(w)
}

// Helpers.

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func replyCode(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter) { reply(w, map[string]bool{"ok": true}) }

func fail(w http.ResponseWriter, code int, msg string) {
	replyCode(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, i18n.T("api.pedido_invalido", "err", err))
		return false
	}
	return true
}

// save turns a store error into a reply; true when there was none.
func (s *Server) save(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, i18n.T("api.nao_encontrado"))
	case errors.As(err, new(userError)):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		s.Log(i18n.T("log.erro_a_guardar", "err", err))
		fail(w, http.StatusInternalServerError, i18n.T("api.nao_guardei", "err", err))
	}
	return false
}

// userError is a mistake in the request, shown to the volunteer as is.
type userError string

func (e userError) Error() string { return string(e) }
