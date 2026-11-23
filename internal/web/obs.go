package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsapp"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/youtube"
)

// errNotLive: OBS took StartStream but never sent, as when a YouTube account
// linked in OBS has no broadcast and OBS shows a message on the PC.
var errNotLive error = i18n.Key("api.obs.nao_transmitiu")

// startPolls times 300 ms is how long a start waits for OBS to send: the
// connection to YouTube comes first.
var startPolls = 100

// obsHealth is the last health check: what is missing or wrong in OBS.
type obsHealth struct {
	Checked  bool     `json:"checked"`
	Ready    bool     `json:"ready"`
	Problems []string `json:"problems"`
}

func obsConfig(o store.OBS) obsws.Config {
	return obsws.Config{Host: o.Host, Port: o.Port, Password: o.Password}
}

// local: OBS runs on this PC, so its files and process are the program's to touch.
func local(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// want is the setup's aim; old is the camera before ("" for the saved one).
func (s *Server) want(o store.OBS, camera, old string) obsws.Want {
	if camera == "" && o.CameraSource != "" {
		camera = "source:" + o.CameraSource
	}
	if old == "" {
		old = o.CameraSource
	}
	return obsws.Want{OverlayURL: obsws.OverlayURL(s.Port), OverlayPorts: [2]int{FirstPort, LastPort},
		Camera: camera, Old: old}
}

func (s *Server) obsRoutes() {
	m := s.mux
	m.HandleFunc("POST /api/obs/setup", s.obsSetup)
	// POST: listing devices may add and remove a hidden source in OBS.
	m.HandleFunc("POST /api/obs/cameras", func(w http.ResponseWriter, r *http.Request) {
		// Not during a setup, which may add the same hidden source.
		s.setupOBSMu.Lock()
		list, err := s.OBS.Cameras(obsConfig(s.Store.Settings().OBS))
		s.setupOBSMu.Unlock()
		if err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
		if list == nil {
			list = []obsws.Choice{}
		}
		reply(w, map[string]any{"cameras": list})
	})
	m.HandleFunc("GET /api/obs/stream", func(w http.ResponseWriter, r *http.Request) { reply(w, s.streamState()) })
	m.HandleFunc("POST /api/obs/stream/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Test bool `json:"test"`
		}
		if !decode(w, r, &req) {
			return
		}
		// Without YouTube, OBS sends to whatever it has: that is no test.
		if y := s.ytGet(); req.Test && (!y.connected() || y.Expired) {
			fail(w, http.StatusConflict, errNoTest.Error())
			return
		}
		// A test over the game (or the reverse) would lose the one on air.
		if rec := s.ytOnAir(); rec != nil && rec.Test != req.Test {
			msg := i18n.T("api.obs.jogo_no_ar")
			if rec.Test {
				msg = i18n.T("api.obs.teste_no_ar")
			}
			fail(w, http.StatusConflict, msg)
			return
		}
		// With YouTube connected, the broadcast is ready before OBS sends.
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		err := s.ytGoLive(ctx, req.Test)
		cancel()
		if err != nil {
			code := http.StatusBadGateway
			if errors.Is(err, youtube.ErrReconnect) {
				code = http.StatusConflict
			}
			fail(w, code, err.Error())
			return
		}
		if !s.streamCall(w, s.OBS.StartStream, true, nil) {
			s.ytAbort()
		}
	})
	m.HandleFunc("POST /api/obs/stream/stop", func(w http.ResponseWriter, r *http.Request) {
		s.streamCall(w, s.OBS.StopStream, false, s.ytEnd)
	})
	m.HandleFunc("POST /api/system/autostart", s.autostart)
}

// streamCall runs call on OBS, then done (when set), and answers with the
// new stream state. It reports whether OBS took the call.
func (s *Server) streamCall(w http.ResponseWriter, call func(obsws.Config) error, live bool, done func()) bool {
	cfg := obsConfig(s.Store.Settings().OBS)
	err := call(cfg)
	if errors.Is(err, obsws.ErrNoStreamKey) {
		fail(w, http.StatusConflict, err.Error())
		return false
	}
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return false
	}
	if done != nil {
		done()
	}
	if live {
		return s.waitLive(w, cfg)
	}
	// OBS takes a moment to report the output as stopped.
	for range 10 {
		st, err := s.OBS.StreamStatus(cfg)
		if err != nil {
			break
		}
		s.setStream(&st)
		if !st.Active {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	reply(w, s.streamState())
	return true
}

// waitLive waits for OBS to send after StartStream. A status read can fail
// while OBS connects, so only reads that worked count. Inactive reads are not
// kept meanwhile: a stale active status would end the new broadcast.
func (s *Server) waitLive(w http.ResponseWriter, cfg obsws.Config) bool {
	var last *obsws.Stream
	for range startPolls {
		if st, err := s.OBS.StreamStatus(cfg); err == nil {
			last = &st
			if st.Active {
				s.setStream(&st)
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if last != nil && !last.Active {
		if err := s.OBS.StopStream(cfg); err != nil {
			s.Log(i18n.T("log.obs.nao_parei", "err", err))
		}
		fail(w, http.StatusBadGateway, errNotLive.Error())
		return false
	}
	reply(w, s.streamState())
	return true
}

func (s *Server) streamState() map[string]any {
	s.obsMu.Lock()
	defer s.obsMu.Unlock()
	return map[string]any{"known": s.stream != nil, "stream": s.stream, "youtube": s.ytSummary()}
}

// setStream keeps OBS's stream status. A stream that stops without
// Terminar (in OBS, or the connection lost) still ends the broadcast.
func (s *Server) setStream(st *obsws.Stream) {
	s.obsMu.Lock()
	was := s.stream != nil && s.stream.Active
	s.stream = st
	s.obsMu.Unlock()
	switch {
	case was && st != nil && !st.Active:
		s.ytEnd()
	case st != nil && !st.Active:
		s.logsEnd(s.ytOnAir()) // it stopped while OBS couldn't be asked
	case !was && st != nil && st.Active:
		s.streamStarted(time.Now().Add(-time.Duration(st.DurationMS) * time.Millisecond))
	}
}

type setupReply struct {
	obsws.Report
	OK bool `json:"ok"`
}

// obsSetup is "Configurar OBS" (and "Reparar"): it reaches OBS, turning
// its WebSocket server on and starting it when needed, then sets it up.
func (s *Server) obsSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Camera string `json:"camera"`
		// Old is the camera before the panel saved its form, to hide it.
		Old string `json:"old"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !s.setupOBSMu.TryLock() {
		fail(w, http.StatusConflict, i18n.T("api.obs.a_configurar"))
		return
	}
	defer s.setupOBSMu.Unlock()
	cfg, st, ok := s.reachOBS(isLoopback(r.RemoteAddr))
	if !ok {
		reply(w, setupReply{Report: obsws.Report{Steps: []obsws.Step{st}}})
		return
	}
	o := s.Store.Settings().OBS
	rep, err := s.OBS.Setup(cfg, s.want(o, req.Camera, req.Old), true, s.fitCamera)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	rep.Steps = append([]obsws.Step{st}, rep.Steps...)
	if yt, on := s.ytSetupStep(); on {
		rep.Steps = append(rep.Steps, yt)
	}
	if rep.Ready {
		_, err := s.Store.UpdateSettings(func(set *store.Settings) {
			if set.OBS.SetupAt == "" {
				set.OBS.Launch = true
			}
			set.OBS.SetupAt = time.Now().Format(time.RFC3339)
			set.OBS.Enabled = true
		})
		if !s.save(w, err) {
			return
		}
	}
	s.checkOBS()
	reply(w, setupReply{rep, rep.Ready})
}

// fitCamera keeps cam as the camera and fits it. A camera just added may
// take a few seconds to send a picture.
func (s *Server) fitCamera(cam string) error {
	_, err := s.Store.UpdateSettings(func(set *store.Settings) {
		if set.OBS.CameraSource != cam {
			set.OBS.CameraSource, set.OBS.CameraFull = cam, nil
		}
		set.OBS.Enabled = true
	})
	if err != nil {
		return err
	}
	if s.camErr != nil {
		return s.camErr
	}
	for i := 0; ; i++ {
		_, err = s.Camera.FitNow()
		if err == nil || i == 5 {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

// reachOBS connects with the saved settings, or with the server settings in
// obs-websocket's own file. When OBS is closed and its server off, it turns
// the server on and starts OBS, but only for a request from the PC itself.
func (s *Server) reachOBS(here bool) (obsws.Config, obsws.Step, bool) {
	st := obsws.Step{ID: "websocket", Label: i18n.T("obs.setup.ligacao"), Result: "ok", Detail: i18n.T("obs.setup.ligacao_ok")}
	o := s.Store.Settings().OBS
	cfg := obsConfig(o)
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	app := s.OBSApp
	mine := local(cfg.Host) && app.ConfigPath != ""
	var file obsapp.WebSocket
	if mine {
		var err error
		if file, err = obsapp.ReadWebSocket(app.ConfigPath); err != nil {
			s.Log(i18n.T("log.obs.erro", "err", err))
		} else if file.Enabled {
			cfg.Port, cfg.Password = file.Port, ""
			if file.Auth {
				cfg.Password = file.Password
			}
		}
	}
	keep := func() bool {
		_, err := s.Store.UpdateSettings(func(set *store.Settings) {
			set.OBS.Host, set.OBS.Port, set.OBS.Password, set.OBS.Enabled = cfg.Host, cfg.Port, cfg.Password, true
		})
		if err != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_guardei_ligacao", "err", err)
		}
		return err == nil
	}
	err := obsws.Test(cfg)
	if err == nil {
		return cfg, st, keep()
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	byHand := i18n.T("obs.setup.a_mao")
	if !mine || !here || app.Running == nil || app.Start == nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_responde", "endereco", addr, "a_mao", byHand)
		return cfg, st, false
	}
	// Holding launchMu, so RunOBS can't start OBS between the check and the
	// file write.
	started, ok := func() (bool, bool) {
		s.launchMu.Lock()
		defer s.launchMu.Unlock()
		running, rerr := app.Running()
		switch {
		case rerr != nil:
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_vi_aberto", "err", rerr)
			return false, false
		case running && file.Enabled:
			return false, true
		case running:
			st.Result, st.Detail = "failed", i18n.T("obs.setup.websocket_desligado", "a_mao", byHand)
			return false, false
		}
		if !file.Enabled {
			ws, err := obsapp.EnableWebSocket(app.ConfigPath)
			if err != nil {
				st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_liguei_websocket", "err", err)
				return false, false
			}
			cfg.Port, cfg.Password = ws.Port, ws.Password
			st.Result, st.Detail = "fixed", i18n.T("obs.setup.liguei_websocket")
			s.Log(i18n.T("log.obs.liguei_websocket", "ficheiro", app.ConfigPath))
		}
		if !keep() {
			return false, false
		}
		s.Log(i18n.T("log.obs.a_abrir"))
		if err := app.Start(); err != nil {
			st.Result, st.Detail = "failed", err.Error()
			return false, false
		}
		return true, true
	}()
	if !ok {
		return cfg, st, false
	}
	if !started {
		// OBS is open with its server on: it may still be starting, as when
		// it opened with the program a moment ago.
		if err := waitOBS(cfg, 20*time.Second); err != nil {
			st.Result, st.Detail = "failed", i18n.T("obs.setup.nao_aceita", "err", err)
			return cfg, st, false
		}
		return cfg, st, keep()
	}
	if err := waitOBS(cfg, time.Minute); err != nil {
		st.Result, st.Detail = "failed", i18n.T("obs.setup.abriu_sem_resposta")
		return cfg, st, false
	}
	if st.Result == "ok" {
		st.Result = "fixed"
		st.Detail = ""
	} else {
		st.Detail += " "
	}
	st.Detail += i18n.T("obs.setup.abri")
	return cfg, st, true
}

// launchOBS starts OBS unless it is running; never two at once.
func (s *Server) launchOBS() error {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	app := s.OBSApp
	if app.Running == nil || app.Start == nil {
		return i18n.Err("obs.nao_abre_sozinho")
	}
	running, err := app.Running()
	if err != nil || running {
		return err
	}
	s.Log(i18n.T("log.obs.a_abrir"))
	return app.Start()
}

// waitOBS tries to connect, waiting longer each time, until OBS answers.
func waitOBS(cfg obsws.Config, max time.Duration) error {
	end := time.Now().Add(max)
	wait := time.Second
	for {
		err := obsws.Test(cfg)
		if err == nil || time.Now().After(end) {
			return err
		}
		time.Sleep(wait)
		wait = min(wait*2, 8*time.Second)
	}
}

// checkOBS runs the health check now. Call it with setupOBSMu held.
func (s *Server) checkOBS() {
	o := s.Store.Settings().OBS
	h := obsHealth{Problems: []string{}}
	if o.Enabled {
		if rep, err := s.OBS.Setup(obsConfig(o), s.want(o, "", ""), false, nil); err == nil {
			h = obsHealth{true, rep.Ready, rep.Problems()}
		}
	}
	s.obsMu.Lock()
	s.health = h
	s.obsMu.Unlock()
}

// refreshOBS is checkOBS unless a setup is running, which checks at its end.
func (s *Server) refreshOBS() {
	if s.setupOBSMu.TryLock() {
		s.checkOBS()
		s.setupOBSMu.Unlock()
	}
}

func (s *Server) obsStatus() map[string]any {
	connected, obsErr := s.OBS.Status()
	s.obsMu.Lock()
	defer s.obsMu.Unlock()
	h := s.health
	if !connected {
		h = obsHealth{Problems: []string{}}
	}
	return map[string]any{"connected": connected, "error": obsErr, "health": h, "stream": s.stream}
}

// RunOBS starts OBS with the program when set to, then keeps the stream
// status and the health check up to date. It never returns.
func (s *Server) RunOBS() {
	if o := s.Store.Settings().OBS; o.Enabled && o.Launch && local(o.Host) && s.OBSApp.Start != nil {
		if err := s.launchOBS(); err != nil {
			s.Log(i18n.T("log.obs.nao_abri", "err", err))
		}
	}
	wait, n := time.Second, 0
	for {
		o := s.Store.Settings().OBS
		if !o.Enabled {
			s.setStream(nil)
			time.Sleep(5 * time.Second)
			continue
		}
		st, err := s.OBS.StreamStatus(obsConfig(o))
		if err != nil {
			s.setStream(nil)
			time.Sleep(wait)
			wait = min(wait*2, 10*time.Second)
			continue
		}
		wait = time.Second
		s.setStream(&st)
		if n++; n%10 == 0 {
			s.refreshOBS()
		}
		time.Sleep(3 * time.Second)
	}
}

// Streaming asks OBS whether the stream is live now.
func (s *Server) Streaming() bool {
	o := s.Store.Settings().OBS
	if !o.Enabled {
		return false
	}
	st, err := s.OBS.StreamStatus(obsConfig(o))
	if err != nil {
		s.obsMu.Lock()
		defer s.obsMu.Unlock()
		return s.stream != nil && s.stream.Active
	}
	return st.Active
}

// CloseOBS closes OBS when "Fechar o OBS ao sair" is on, ending the stream first.
func (s *Server) CloseOBS() {
	o := s.Store.Settings().OBS
	if !o.CloseOnExit || !local(o.Host) || s.OBSApp.Close == nil {
		return
	}
	cfg := obsConfig(o)
	live := false
	if st, err := s.OBS.StreamStatus(cfg); err == nil && st.Active {
		s.OBS.StopStream(cfg)
		for range 10 {
			st, err := s.OBS.StreamStatus(cfg)
			if live = err == nil && st.Active; !live {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	// Ending OBS by force is only for when no one is watching.
	if err := s.OBSApp.Close(!live); err != nil {
		s.Log(i18n.T("log.obs.nao_fechei", "err", err))
	}
}

// autostart is "Abrir com o Windows": a shortcut in the Startup folder.
func (s *Server) autostart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.so_no_pc"))
		return
	}
	if s.Autostart == nil {
		fail(w, http.StatusNotImplemented, i18n.T("api.arranque_so_windows"))
		return
	}
	if err := s.Autostart(req.On); err != nil {
		s.Log(i18n.T("log.arranque_erro", "err", err))
		fail(w, http.StatusInternalServerError, i18n.T("api.nao_mudei_arranque", "err", err))
		return
	}
	_, err := s.Store.UpdateSettings(func(set *store.Settings) { set.StartWithWindows = req.On })
	if !s.save(w, err) {
		return
	}
	reply(w, map[string]any{"ok": true, "on": req.On})
}
