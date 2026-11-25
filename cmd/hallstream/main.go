// Hallstream reads the live score from a Stramatel SL Stream Box over
// Bluetooth LE and serves the OBS overlay and the control panel in the
// browser. It also writes each value to dados/score/*.txt for older OBS setups.
//
// On Windows it runs without a console: messages go to dados/marcador.log,
// startup errors to a message box, and the icon by the clock opens the
// panel and closes the program. Messages are in Portuguese for the people
// running it at games.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	assets "github.com/kauredo/hallstream/web"

	"github.com/kauredo/hallstream/internal/applog"
	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/desktop"
	"github.com/kauredo/hallstream/internal/fpb"
	"github.com/kauredo/hallstream/internal/gamelog"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsapp"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/source"
	"github.com/kauredo/hallstream/internal/sport"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/web"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

// uploadToken lets the program send each stream's logs to the club's
// server; set by build.sh from HALLSTREAM_UPLOAD_TOKEN. Empty never sends.
var uploadToken = ""

// feedbackURL is the feedback worker's address (worker/README.md), set at
// build time; "" leaves only the GitHub link in Ajuda.
var feedbackURL = ""

// logOut gets every message: dados/marcador.log, and the console when there
// is one.
var logOut io.Writer = os.Stdout

func say(s string) { fmt.Fprintln(logOut, time.Now().Format("2006-01-02 15:04:05"), s) }

// fatal logs why the program can't go on, tells the volunteer and exits.
func fatal(msg string) {
	say(msg)
	desktop.Fatal(msg)
	os.Exit(1)
}

// How long safely waits before it starts a loop again: restartMin after the
// first panic, twice as long after each one in a row, up to restartMax.
var (
	restartMin = 5 * time.Second
	restartMax = 5 * time.Minute
)

// safely runs fn in the background and keeps a panic in it from closing the
// program in the middle of a game: the panic goes to the log and fn starts
// again after a pause. A fn that returns is not started again.
func safely(name string, fn func()) {
	go func() {
		pause := restartMin
		for {
			began := time.Now()
			if !panicked(name, fn) {
				return
			}
			if time.Since(began) > restartMax {
				pause = restartMin
			}
			time.Sleep(pause)
			pause = min(pause*2, restartMax)
		}
	}()
}

func panicked(name string, fn func()) (yes bool) {
	defer func() {
		if r := recover(); r != nil {
			yes = true
			say(i18n.T("log.erro_inesperado_repete", "tarefa", name, "erro", r) + "\n" + string(debug.Stack()))
		}
	}()
	fn()
	return false
}

// writer keeps the old outputs: raw.log with every frame and score/*.txt.
type writer struct {
	dir     string
	raw     *os.File
	written map[string]string
}

func newWriter(dataDir string) (*writer, error) {
	raw, err := os.OpenFile(filepath.Join(dataDir, "raw.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &writer{dir: filepath.Join(dataDir, "score"), raw: raw, written: map[string]string{}}, nil
}

func (w *writer) frame(f []byte) {
	fmt.Fprintf(w.raw, "%s %s\n", time.Now().Format("2006-01-02T15:04:05.000"), hex.EncodeToString(f))
}

func (w *writer) state(s map[string]string) {
	for k, v := range s {
		if w.written[k] == v {
			continue
		}
		if store.WriteFile(filepath.Join(w.dir, k+".txt"), []byte(v)) == nil {
			w.written[k] = v
		}
	}
}

func main() {
	demo := flag.Bool("demo", false, i18n.T("app.flag.demo"))
	noBrowser := flag.Bool("nobrowser", false, i18n.T("app.flag.nobrowser"))
	flag.Parse()

	// A second double-click opens the panel of the one already running.
	for p := web.FirstPort; p <= web.LastPort; p++ {
		if web.Ours(fmt.Sprintf("127.0.0.1:%d", p)) {
			say(i18n.T("log.ja_aberto", "porta", p))
			openBrowser(fmt.Sprintf("http://127.0.0.1:%d/painel", p))
			return
		}
	}

	dir := store.DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(i18n.T("log.nao_criei_pasta", "pasta", dir, "err", err))
	}
	logFile, err := applog.Open(filepath.Join(dir, "marcador.log"), 1<<20)
	switch {
	case err == nil && desktop.Console:
		logOut = io.MultiWriter(os.Stdout, logFile)
	case err == nil:
		logOut = logFile
	case !desktop.Console:
		logOut = io.Discard
	}
	say("Hallstream " + version)
	if err != nil {
		say(i18n.T("log.nao_criei_marcador_log", "err", err))
	}

	st, err := store.Open(dir)
	if err != nil {
		fatal(i18n.T("log.nao_abri_pasta", "pasta", dir, "err", err))
	}
	w, err := newWriter(dir)
	if err != nil {
		fatal(i18n.T("log.nao_criei_raw_log", "pasta", dir, "err", err))
	}
	set := st.Settings()
	i18n.SetLang(set.Lang)

	tl, err := gamelog.OpenTimeline(filepath.Join(dir, "timeline.jsonl"))
	if err != nil {
		say(i18n.T("log.nao_criei_timeline", "err", err))
	}

	m := state.New(time.Now)
	m.OnEvent = tl.Add
	m.SetScreen("", set.AutoScreens)
	m.OnPeriods = func(s state.State) { saveHistory(st, s) }
	restorePeriods(st, m)
	m.Start()

	// The score comes from one source at a time, settings.input: the box
	// (console) or the phone (manual). The Bluetooth reader runs either way,
	// and the server answers /api/ble with it; it starts last.
	console := source.NewConsole(&source.Console{Raw: w.frame})
	manual := source.NewManual(&source.Manual{Save: func(s source.Score) error { return st.SaveManual(s) }, Log: say,
		Rules: func() sport.Profile { return sport.Get(st.GameSport()) }})
	restoreManual(st, manual)
	inputs := &source.Switch{Sources: []source.Source{console, manual}, State: m, Values: w.state, Go: safely}
	var reader *ble.Reader
	if !*demo {
		reader = ble.New(&ble.Reader{
			Radio:     ble.Adapter(),
			Frame:     console.Frame,
			Connected: console.Connected,
			Log:       say,
			Status:    m.SetScan,
			Saved:     func() string { return st.Settings().BoxAddress },
			Save: func(addr string) {
				if err := st.SetBoxAddress(addr); err != nil {
					say(i18n.T("log.nao_guardei_endereco_caixa", "err", err))
				}
			},
		})
		if f, err := os.OpenFile(filepath.Join(dir, "bluetooth.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			reader.DevLog = f
		} else {
			say(i18n.T("log.nao_criei_bluetooth_log", "err", err))
		}
	}

	srv := web.New(&web.Server{
		Store:     st,
		State:     m,
		FPB:       fpb.New(func() string { return st.Settings().FPBBaseURL }, st.CacheDir(), version),
		OBS:       &obsws.Client{},
		BLE:       reader,
		Files:     assets.FS,
		Version:   version,
		Log:       say,
		Shortcut:  desktop.Shortcut,
		Autostart: desktop.Autostart,
		OBSApp:    obsapp.System(),

		UploadToken: uploadToken,
		FeedbackURL: feedbackURL,
		Timeline:    tl,
		UseInput:    inputs.Use,
		Manual:      manual,
	})
	srv.SyncGame()
	srv.SyncPromo()
	// A copy unzipped on another PC brings the switch but not the Startup
	// shortcut, and a moved program needs it pointing at the new exe.
	if set.StartWithWindows && desktop.Autostart != nil {
		if err := desktop.Autostart(true); err != nil {
			say(i18n.T("log.nao_pus_arranque", "err", err))
		}
	}
	safely(i18n.T("log.tarefa.camara"), srv.RunCamera)
	safely("OBS", srv.RunOBS)
	safely("YouTube", srv.RunYouTube)
	safely(i18n.T("log.tarefa.proximo"), srv.WatchNext)
	safely(i18n.T("log.tarefa.envios"), srv.RunUploads)
	safely(i18n.T("log.tarefa.atualizacoes"), srv.RunUpdates)
	safely(i18n.T("log.tarefa.fpb"), func() {
		srv.FirstRun(*demo)
		srv.Daily(*demo)
	})
	l, port, err := web.Listen(set.PhoneAccess)
	if err != nil {
		fatal(i18n.T("log.nao_abri_servidor", "err", err))
	}
	srv.Port = port
	go func() {
		hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
		if err := hs.Serve(l); err != nil {
			fatal(i18n.T("log.servidor_parou", "err", err))
		}
	}()

	panel := fmt.Sprintf("http://127.0.0.1:%d/painel", port)
	overlay := fmt.Sprintf("http://127.0.0.1:%d/overlay", port)
	say(i18n.T("log.painel", "url", panel))
	say(i18n.T("log.overlay", "url", overlay))
	if set.PhoneAccess {
		if ip := web.LanIP(); ip != "" {
			say(i18n.T("log.telemovel", "url", fmt.Sprintf("http://%s:%d/painel", ip, port)))
		}
	}
	say(i18n.T("log.dados_em", "pasta", dir))
	if port != web.FirstPort {
		go desktop.Warn(i18n.T("log.porta_ocupada_aviso", "pedida", web.FirstPort, "porta", port, "overlay", overlay))
	}
	if !*noBrowser {
		openBrowser(panel)
	}

	inputs.Use(set.Input)
	if *demo {
		say(i18n.T("log.modo_demo"))
		m.SetDemo(true)
	} else {
		go reader.Run()
	}

	icon, _ := assets.FS.ReadFile("assets/icon/hallstream.ico")
	changed, _ := m.Subscribe()
	desktop.Tray{
		Icon:    icon,
		Tooltip: "Hallstream",
		Panel:   func() { openBrowser(panel) },
		Overlay: overlay,
		Status:  func() string { return trayStatus(m.Snapshot()) },
		Changed: changed,
		Log:     say,
		ConfirmQuit: func() bool {
			if !srv.Streaming() {
				return true
			}
			msg := i18n.T("app.sair.no_ar")
			if st.Settings().OBS.CloseOnExit {
				msg = i18n.T("app.sair.no_ar_fecha_obs")
			}
			return desktop.Ask(msg)
		},
		Quit: func() {
			say(i18n.T("log.a_fechar"))
			srv.Camera.Stop()
			srv.CloseOBS()
			if reader != nil {
				// The Bluetooth stack may not answer; don't let that keep the program open.
				done := make(chan struct{})
				go func() { reader.Close(); close(done) }()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
				}
			}
			st.Close()
			if logFile != nil {
				logFile.Close()
			}
		},
	}.Run()
}

// trayStatus is the grey line in the tray menu.
func trayStatus(s state.State) string {
	switch {
	case s.Demo:
		return i18n.T("app.bandeja.demo")
	case s.Input == "manual" && s.Box.Connected:
		return i18n.T("app.bandeja.manual_ligada")
	case s.Input == "manual":
		return i18n.T("app.bandeja.manual_espera")
	case s.Box.Connected:
		return i18n.T("app.bandeja.ligada")
	case s.Box.Scan.Bluetooth == "off" || s.Box.Scan.Bluetooth == "unavailable":
		return i18n.T("app.bandeja.bluetooth_desligado")
	default:
		return i18n.T("app.bandeja.a_procura")
	}
}

// restorePeriods brings back the periods of the game that was on before a
// restart, and keeps periods.json up to date from now on.
func restorePeriods(st *store.Store, m *state.Machine) {
	var ps []state.Period
	if err := st.LoadPeriods(&ps); err != nil {
		say(i18n.T("log.nao_li_periodos", "err", err))
		ps = nil
	}
	m.RestorePeriods(ps)
	m.SavePeriods = func(ps []state.Period) {
		if err := st.SavePeriods(ps); err != nil {
			say(i18n.T("log.nao_guardei_periodos", "err", err))
		}
	}
	var ss []state.SetScore
	if err := st.LoadSets(&ss); err != nil {
		say(i18n.T("log.nao_li_sets", "err", err))
		ss = nil
	}
	m.RestoreSets(ss)
	m.SaveSets = func(ss []state.SetScore) {
		if err := st.SaveSets(ss); err != nil {
			say(i18n.T("log.nao_guardei_sets", "err", err))
		}
	}
}

// restoreManual brings back the score kept by hand before a restart.
func restoreManual(st *store.Store, mn *source.Manual) {
	sc := mn.Score()
	if err := st.LoadManual(&sc); err != nil {
		say(i18n.T("log.nao_li_marcacao", "err", err))
		return
	}
	mn.Restore(sc)
}

// saveHistory writes the game's periods to dados/jogos each time one ends.
func saveHistory(st *store.Store, s state.State) {
	if s.Demo {
		return
	}
	date := time.Now().Format("2006-01-02")
	if len(s.Game.Start) >= 10 {
		date = s.Game.Start[:10]
	}
	group := ""
	if s.Game.Group != nil {
		group = s.Game.Group.ID
	}
	final := map[string]int{"home": s.Live.Home, "away": s.Live.Away}
	if n := len(s.Periods); n > 0 {
		final = map[string]int{"home": s.Periods[n-1].Home, "away": s.Periods[n-1].Away}
	}
	if s.Game.Rules.Sets() {
		final = map[string]int{"home": s.Live.HomeSets, "away": s.Live.AwaySets}
	}
	err := st.SaveHistory(date, group, map[string]any{
		"saved_at": time.Now().Format(time.RFC3339),
		"game":     s.Game,
		"periods":  s.Periods,
		"sets":     s.Sets,
		"final":    final,
	})
	if err != nil {
		say(i18n.T("log.nao_guardei_historico", "err", err))
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		say(i18n.T("log.nao_abri_browser", "err", err, "url", url))
		return
	}
	go cmd.Wait()
}
