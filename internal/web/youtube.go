package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/obsws"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
	"github.com/kauredo/hallstream/internal/youtube"
)

// ytSaved is dados/youtube.json: the club's OAuth client, the login and the
// reusable stream. It never leaves the PC: the API answers with ytStatus.
type ytSaved struct {
	ClientID     string           `json:"client_id"`
	ClientSecret string           `json:"client_secret"`
	Token        *oauth2.Token    `json:"token"`
	Channel      *youtube.Channel `json:"channel"`
	Stream       *youtube.Stream  `json:"stream"`
	// Expired: Google refused the saved login; only a new one helps.
	Expired bool `json:"expired"`
}

func (y ytSaved) creds() youtube.Credentials {
	return youtube.Credentials{ClientID: y.ClientID, ClientSecret: y.ClientSecret}
}
func (y ytSaved) connected() bool { return y.Token != nil && y.Token.RefreshToken != "" }

// ytRecord is one stream in dados/transmissoes.json.
type ytRecord struct {
	BroadcastID string `json:"broadcast_id"`
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	GroupID     string `json:"group_id"`
	Escalao     string `json:"escalao"`
	Opponent    string `json:"opponent"`
	Date        string `json:"date"`   // RFC 3339
	Score       string `json:"score"`  // "82-76", home first
	Result      string `json:"result"` // "Casa 82-76 Atlético Vale"
	Privacy     string `json:"privacy"`
	Link        string `json:"link"`
	GameKey     string `json:"game_key"`
	Status      string `json:"status"` // the broadcast's lifeCycleStatus when last seen
	// OnAir is from Iniciar to Terminar; Ended from Terminar on.
	OnAir          bool   `json:"on_air"`
	Ended          bool   `json:"ended"`
	PrivacyError   string `json:"privacy_error"`
	ThumbnailSet   bool   `json:"thumbnail_set"`
	ThumbnailError string `json:"thumbnail_error"`
	External       bool   `json:"external,omitempty"` // made outside the program (history only)
	// Test is a private rehearsal: it stays private and has no thumbnail.
	Test bool `json:"test,omitempty"`
	// StartedAt is the first start (RFC 3339), kept when the broadcast is reused.
	StartedAt string `json:"started_at,omitempty"`
}

const (
	ytFile     = "youtube.json"
	ytHistFile = "transmissoes.json"
	ytStream   = "Hallstream"
	// After Terminar: the first look, the most between looks, and how many.
	ytFirstPoll = 30 * time.Second
	ytMaxWait   = 5 * time.Minute
	ytMaxPolls  = 10
	// The channel's finished broadcasts are read again after this.
	ytRemoteTTL = 10 * time.Minute
)

// ytLoginWait is how long a login waits for Google; tests shorten it.
var ytLoginWait = 5 * time.Minute

var ytReuse = []string{"created", "ready", "testStarting", "testing", "liveStarting", "live"}

func (s *Server) loadYouTube() {
	// A program closed mid-save leaves a temp file, maybe with the login in it.
	left, _ := filepath.Glob(filepath.Join(s.Store.Dir, ".tmp-*"))
	for _, f := range left {
		os.Remove(f)
	}
	for name, v := range map[string]any{ytFile: &s.yt, ytHistFile: &s.ytRecs} {
		b, err := os.ReadFile(filepath.Join(s.Store.Dir, name))
		if err == nil && name == ytFile {
			b, err = youtube.Unseal(b)
		}
		if err == nil {
			err = json.Unmarshal(b, v)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.Log(i18n.T("log.yt.estragado", "ficheiro", name, "err", err))
		}
	}
	if s.ytRecs == nil {
		s.ytRecs = []ytRecord{}
	}
}

// ytWrite saves both files; call with ytMu held. youtube.json holds the
// secret and the tokens, so it is sealed for this Windows user (DPAPI); on
// other systems the file is 0600 (from store.WriteFile's temp file).
func (s *Server) ytWrite() {
	for name, v := range map[string]any{ytFile: s.yt, ytHistFile: s.ytRecs} {
		b, _ := json.MarshalIndent(v, "", "  ")
		var err error
		if name == ytFile {
			b, err = youtube.Seal(b)
		}
		if err == nil {
			err = store.WriteFile(filepath.Join(s.Store.Dir, name), b)
		}
		if err != nil {
			s.Log(i18n.T("log.yt.nao_guardei", "ficheiro", name, "err", err))
		}
	}
}

func (s *Server) ytGet() ytSaved {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	return s.yt
}

func (s *Server) ytConnected() bool { return s.ytGet().connected() }

func (s *Server) youtubeRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/youtube", func(w http.ResponseWriter, r *http.Request) { reply(w, s.ytStatus(isLoopback(r.RemoteAddr))) })
	m.HandleFunc("PUT /api/youtube/client", s.ytSetClient)
	m.HandleFunc("POST /api/youtube/connect", s.ytConnect)
	m.HandleFunc("POST /api/youtube/disconnect", s.ytDisconnect)
	m.HandleFunc("POST /api/youtube/thumbnail", s.ytThumbnail)
	m.HandleFunc("POST /api/youtube/unlist", s.ytUnlist)
	m.HandleFunc("GET /api/youtube/history", s.ytHistory)
}

// ytStatus is what the panel may know: never the secret or the token, and
// the client id only on the PC.
func (s *Server) ytStatus(here bool) map[string]any {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	y := s.yt
	m := map[string]any{
		"setup": here, "configured": y.ClientID != "" && y.ClientSecret != "", "has_secret": y.ClientSecret != "",
		"connected": y.connected(), "expired": y.Expired, "channel": y.Channel, "stream_ready": y.Stream != nil,
		"connecting": s.ytLogin != nil, "error": s.ytErr, "client_id": "",
	}
	if here {
		m["client_id"] = y.ClientID
	}
	return m
}

// pcOnly refuses a request from another device.
func pcOnly(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopback(r.RemoteAddr) {
		fail(w, http.StatusForbidden, i18n.T("api.yt.so_no_pc"))
		return false
	}
	return true
}

func (s *Server) ytSetClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if !decode(w, r, &req) || !pcOnly(w, r) {
		return
	}
	id, secret := strings.TrimSpace(req.ClientID), strings.TrimSpace(req.ClientSecret)
	if !strings.HasSuffix(id, ".apps.googleusercontent.com") || strings.ContainsAny(id, " /") {
		fail(w, http.StatusBadRequest, i18n.T("api.yt.id_cliente"))
		return
	}
	s.ytMu.Lock()
	if s.yt.connected() && id != s.yt.ClientID {
		s.ytMu.Unlock()
		fail(w, http.StatusConflict, i18n.T("api.yt.desligue_primeiro"))
		return
	}
	if secret == "" && id == s.yt.ClientID {
		secret = s.yt.ClientSecret // left blank: keep it
	}
	if secret == "" {
		s.ytMu.Unlock()
		fail(w, http.StatusBadRequest, i18n.T("api.yt.falta_segredo"))
		return
	}
	s.yt.ClientID, s.yt.ClientSecret, s.ytErr = id, secret, ""
	s.ytWrite()
	old := s.ytLogin // a login with the old client would save a token it can't refresh
	s.ytLogin = nil
	s.ytMu.Unlock()
	if old != nil {
		old.Cancel()
	}
	reply(w, s.ytStatus(true))
}

func (s *Server) ytConnect(w http.ResponseWriter, r *http.Request) {
	if !decode(w, r, &struct{}{}) || !pcOnly(w, r) {
		return
	}
	s.ytMu.Lock()
	creds, old := s.yt.creds(), s.ytLogin
	s.ytLogin = nil
	s.ytMu.Unlock()
	// Cancelled outside ytMu: a login finishing now waits for ytMu in
	// ytLoggedIn, and Cancel waits for it. With ytLogin nil it is dropped.
	if old != nil {
		old.Cancel()
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		fail(w, http.StatusBadRequest, i18n.T("api.yt.falta_cliente"))
		return
	}
	l, err := youtube.StartLogin(creds, ytLoginWait, s.ytLoggedIn)
	if err != nil {
		fail(w, http.StatusInternalServerError, i18n.T("api.yt.nao_preparei_login", "err", err))
		return
	}
	s.ytMu.Lock()
	s.ytLogin, s.ytErr = l, ""
	s.ytMu.Unlock()
	reply(w, map[string]string{"auth_url": l.URL})
}

func (s *Server) ytLoggedIn(l *youtube.Login, tok *oauth2.Token, err error) {
	s.ytMu.Lock()
	if s.ytLogin != l {
		s.ytMu.Unlock()
		return
	}
	s.ytLogin = nil
	if err != nil {
		s.ytErr = err.Error()
		s.ytMu.Unlock()
		s.Log(i18n.T("log.yt.erro", "err", err))
		return
	}
	s.yt.Token, s.yt.Expired, s.yt.Channel, s.yt.Stream = tok, false, nil, nil
	s.ytWrite()
	s.ytMu.Unlock()
	s.Log(i18n.T("log.yt.ligado"))
	s.once("YouTube", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		ch, err := s.ytClient().Channel(ctx)
		if err == nil {
			s.ytMu.Lock()
			s.yt.Channel = &ch
			s.ytWrite()
			s.ytMu.Unlock()
			_, _, err = s.ytPrepare(ctx)
		}
		if err != nil {
			s.ytFailed(err)
		}
	})
}

func (s *Server) ytDisconnect(w http.ResponseWriter, r *http.Request) {
	if !decode(w, r, &struct{}{}) || !pcOnly(w, r) {
		return
	}
	y := s.ytGet()
	if y.Token != nil && y.Token.RefreshToken != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		if err := youtube.Revoke(ctx, &http.Client{}, y.Token.RefreshToken); err != nil {
			s.Log(i18n.T("log.yt.nao_cancelei", "err", err))
		}
		cancel()
	}
	s.ytMu.Lock()
	old := s.ytLogin
	s.ytLogin = nil
	s.ytMu.Unlock()
	if old != nil {
		old.Cancel() // outside ytMu, as in ytConnect
	}
	s.ytMu.Lock()
	s.yt.Token, s.yt.Channel, s.yt.Stream, s.yt.Expired, s.ytErr = nil, nil, nil, false, ""
	s.ytWrite()
	s.ytMu.Unlock()
	s.Log(i18n.T("log.yt.desligado"))
	reply(w, s.ytStatus(true))
}

// ytClient calls YouTube with the saved login, saving each new access token.
func (s *Server) ytClient() *youtube.Client {
	y := s.ytGet()
	tok := y.Token
	if tok == nil {
		// Disconnected meanwhile: every call then fails, with no refresh token.
		tok = &oauth2.Token{}
	}
	return youtube.NewClient(y.creds(), tok, func(t *oauth2.Token) {
		s.ytMu.Lock()
		defer s.ytMu.Unlock()
		if tok.RefreshToken != "" && s.yt.Token != nil && s.yt.Token.RefreshToken == tok.RefreshToken {
			s.yt.Token = t
			s.ytWrite()
		}
	})
}

// ytFailed notes an error; a refused login marks the connection expired.
func (s *Server) ytFailed(err error) error {
	s.ytMu.Lock()
	if errors.Is(err, youtube.ErrReconnect) && !s.yt.Expired {
		s.yt.Expired = true
		s.ytWrite()
	}
	s.ytErr = err.Error()
	s.ytMu.Unlock()
	s.Log(i18n.T("log.yt.erro", "err", err))
	return err
}

// ytPrepare finds or makes the reusable stream and points OBS at it. It
// reports whether OBS changed, and "later" when OBS is live.
func (s *Server) ytPrepare(ctx context.Context) (youtube.Stream, string, error) {
	y := s.ytGet()
	if !y.connected() {
		return youtube.Stream{}, "", nil
	}
	st := youtube.Stream{}
	if y.Stream != nil {
		st = *y.Stream
	} else {
		var err error
		if st, err = s.ytClient().EnsureStream(ctx, ytStream); err != nil {
			return st, "", err
		}
		s.ytMu.Lock()
		s.yt.Stream = &st
		s.ytWrite()
		s.ytMu.Unlock()
	}
	o := s.Store.Settings().OBS
	if !o.Enabled {
		return st, "", nil
	}
	changed, err := s.OBS.SetCustomService(obsConfig(o), st.Server, st.Key)
	switch {
	case errors.Is(err, obsws.ErrStreaming):
		return st, "later", nil
	case err != nil:
		return st, "", i18n.Err("yt.obs_nao_envia", "err", err)
	case changed:
		s.Log(i18n.T("log.yt.obs_envia", "stream", ytStream))
		return st, "fixed", nil
	}
	return st, "ok", nil
}

// ytSetupStep is the YouTube line of Configurar OBS.
func (s *Server) ytSetupStep() (step obsws.Step, ok bool) {
	if !s.ytConnected() {
		return step, false
	}
	step.ID, step.Label = "youtube", i18n.T("obs.setup.youtube")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, res, err := s.ytPrepare(ctx)
	switch {
	case err != nil:
		s.ytFailed(err)
		step.Result, step.Detail = "failed", err.Error()
	case res == "fixed":
		step.Result, step.Detail = "fixed", i18n.T("obs.setup.youtube_pus")
	case res == "later":
		step.Result, step.Detail = "later", i18n.T("obs.setup.youtube_depois")
	default:
		step.Result, step.Detail = "ok", i18n.T("obs.setup.youtube_ok")
	}
	return step, true
}

func gameKey(g store.Game) string {
	if g.FPBGameID != nil {
		return "fpb:" + strconv.FormatInt(*g.FPBGameID, 10)
	}
	day := time.Now().In(lisbon).Format("2006-01-02")
	if len(g.Start) >= 10 {
		day = g.Start[:10]
	}
	return g.GroupID + ":" + day + ":" + g.Away.Name
}

// ytGoLive makes (or finds, after a restart) the broadcast for the game and
// ties it to the stream, before OBS starts. Without YouTube it does nothing.
// A test makes a private broadcast, kept apart from the game's real one.
func (s *Server) ytGoLive(ctx context.Context, test bool) error {
	y := s.ytGet()
	if !y.connected() {
		if test {
			return errNoTest
		}
		return nil
	}
	if y.Expired {
		return youtube.ErrReconnect
	}
	st, _, err := s.ytPrepare(ctx)
	if err != nil {
		return s.ytFailed(err)
	}
	c := s.ytClient()
	game := s.Store.Game()
	key, privacy := gameKey(game), "public"
	if test {
		key, privacy = "teste:"+key, "private"
	}
	if rec := s.ytLatest(key); rec != nil {
		bs, err := c.Broadcasts(ctx, rec.BroadcastID)
		if err != nil {
			return s.ytFailed(err)
		}
		status := "gone"
		if len(bs) == 1 {
			status = bs[0].Status
		}
		if slices.Contains(ytReuse, status) {
			if bs[0].StreamID != st.ID {
				if err := s.ytBind(ctx, c, rec.BroadcastID, st); err != nil {
					return s.ytFailed(err)
				}
			}
			s.ytUpdate(rec.BroadcastID, func(r *ytRecord) {
				if r.StartedAt == "" || r.Ended {
					r.StartedAt = time.Now().Format(time.RFC3339)
				}
				r.Status, r.OnAir, r.Ended, r.PrivacyError = status, true, false, ""
			})
			s.ytMu.Lock()
			s.ytErr = ""
			s.ytMu.Unlock()
			return nil
		}
		s.ytUpdate(rec.BroadcastID, func(r *ytRecord) { r.Status = status })
	}
	snap := s.State.Snapshot()
	title, desc := broadcastText(snap.Game, time.Now())
	if test {
		r := []rune(i18n.T("yt.titulo_teste", "titulo", title))
		title = string(r[:min(len(r), 100)])
	}
	b, err := c.NewBroadcast(ctx, title, desc, privacy, time.Now())
	if err != nil {
		return s.ytFailed(err)
	}
	date := time.Now().Format(time.RFC3339)
	if _, err := time.Parse(time.RFC3339, game.Start); err == nil {
		date = game.Start
	}
	group := ""
	if snap.Game.Group != nil {
		group = snap.Game.Group.Name
	}
	s.ytMu.Lock()
	for i := range s.ytRecs {
		s.ytRecs[i].OnAir = false
	}
	s.ytRecs = append(s.ytRecs, ytRecord{BroadcastID: b.ID, VideoID: b.ID, Title: b.Title, GroupID: game.GroupID, Escalao: group,
		Opponent: teamName(snap.Game.Away.Name), Date: date, Privacy: privacy, Link: "https://youtu.be/" + b.ID,
		GameKey: key, Status: b.Status, OnAir: true, Test: test, StartedAt: time.Now().Format(time.RFC3339)})
	s.ytWrite()
	s.ytMu.Unlock()
	s.Log(i18n.T("log.yt.transmissao_nova", "id", b.ID, "titulo", title))
	if err := s.ytBind(ctx, c, b.ID, st); err != nil {
		return s.ytFailed(err)
	}
	s.ytMu.Lock()
	s.ytErr = ""
	s.ytMu.Unlock()
	return nil
}

// ytBind ties broadcast id to the stream. When the stream was deleted on
// YouTube, it finds or makes it again (and points OBS at it) once.
func (s *Server) ytBind(ctx context.Context, c *youtube.Client, id string, st youtube.Stream) error {
	err := c.Bind(ctx, id, st.ID)
	var ae *youtube.APIError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		return err
	}
	s.ytMu.Lock()
	s.yt.Stream = nil
	s.ytMu.Unlock()
	if st, _, err = s.ytPrepare(ctx); err == nil {
		err = c.Bind(ctx, id, st.ID)
	}
	return err
}

var errNoTest error = i18n.Key("yt.teste_precisa")

// ytOnAir is the record on air, if any.
func (s *Server) ytOnAir() *ytRecord {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	for _, r := range s.ytRecs {
		if r.OnAir {
			return &r
		}
	}
	return nil
}

// ytAbort: OBS didn't start, so no broadcast is on air.
func (s *Server) ytAbort() {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	changed := false
	for i := range s.ytRecs {
		if s.ytRecs[i].OnAir {
			s.ytRecs[i].OnAir, s.ytRecs[i].StartedAt, changed = false, "", true
		}
	}
	if changed {
		s.ytWrite()
	}
}

// ytLatest is the newest stream of the game with key.
func (s *Server) ytLatest(key string) *ytRecord {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	for i := len(s.ytRecs) - 1; i >= 0; i-- {
		if s.ytRecs[i].GameKey == key {
			r := s.ytRecs[i]
			return &r
		}
	}
	return nil
}

func (s *Server) ytRecord(id string) *ytRecord {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	for _, r := range s.ytRecs {
		if r.BroadcastID == id {
			return &r
		}
	}
	return nil
}

func (s *Server) ytUpdate(id string, fn func(*ytRecord)) {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	for i := range s.ytRecs {
		if s.ytRecs[i].BroadcastID == id {
			fn(&s.ytRecs[i])
			s.ytWrite()
			return
		}
	}
}

// ytEnd runs after OBS stops: the result goes in the record, and the video
// turns unlisted once YouTube calls the broadcast complete.
func (s *Server) ytEnd() {
	snap := s.State.Snapshot()
	id, test := "", false
	var ended *ytRecord
	s.ytMu.Lock()
	for i := range s.ytRecs {
		if r := &s.ytRecs[i]; r.OnAir {
			r.OnAir, r.Ended, r.PrivacyError = false, true, ""
			r.Score = fmt.Sprintf("%d-%d", snap.Live.Home, snap.Live.Away)
			r.Result = fmt.Sprintf("%s %d-%d %s", teamName(snap.Game.Home.Name), snap.Live.Home, snap.Live.Away, teamName(snap.Game.Away.Name))
			id, test = r.BroadcastID, r.Test
			s.ytEnded = id
			c := *r
			ended = &c
		}
	}
	if id != "" {
		s.ytWrite()
	}
	s.ytMu.Unlock()
	s.logsEnd(ended)
	if id != "" && !test {
		s.ytFinishLater(id)
	}
}

// ytFinishLater runs ytFinish for id in the background; asked again while
// it runs, it runs once more after.
func (s *Server) ytFinishLater(id string) {
	s.ytMu.Lock()
	if s.ytBusy[id] {
		s.ytAgain[id] = true
		s.ytMu.Unlock()
		return
	}
	s.ytBusy[id] = true
	s.ytMu.Unlock()
	s.once("YouTube", func() {
		for {
			s.ytFinish(id)
			s.ytMu.Lock()
			if !s.ytAgain[id] {
				delete(s.ytBusy, id)
				s.ytMu.Unlock()
				return
			}
			delete(s.ytAgain, id)
			s.ytMu.Unlock()
		}
	})
}

// ytFinish waits for YouTube to end the broadcast (enableAutoStop does it
// about a minute after the stream stops), then makes the video unlisted. At
// most ytMaxPolls looks of 1 unit each, a minute to 5 apart, and one
// transition if it stays live.
func (s *Server) ytFinish(id string) {
	wait, done := ytFirstPoll, false
	var last error
	for i := 0; i < ytMaxPolls; i++ {
		s.ytSleep(wait)
		wait = min(wait*2, ytMaxWait)
		if r := s.ytRecord(id); r == nil || !r.Ended {
			return // started again
		}
		if !s.ytConnected() {
			s.ytUpdate(id, func(r *ytRecord) {
				r.PrivacyError = i18n.T("yt.desligado_antes")
			})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		bs, err := s.ytClient().Broadcasts(ctx, id)
		status := "gone"
		if len(bs) == 1 {
			status = bs[0].Status
		}
		switch {
		case err != nil:
			last = s.ytFailed(err)
			if errors.Is(err, youtube.ErrReconnect) {
				s.ytUpdate(id, func(r *ytRecord) { r.PrivacyError = err.Error() })
				cancel()
				return
			}
			cancel()
			continue
		case status == "complete":
			err = s.ytClient().SetPrivacy(ctx, id, "unlisted")
			s.ytPrivacy(id, status, err)
			cancel()
			return
		case status == "live" || status == "liveStarting":
			if i >= 3 && !done {
				done = true
				if err := s.ytClient().Complete(ctx, id); err != nil {
					s.ytFailed(err)
				}
			}
		default: // never went live, or gone
			s.ytUpdate(id, func(r *ytRecord) {
				r.Status, r.PrivacyError = status, i18n.T("yt.nunca_no_ar")
			})
			cancel()
			return
		}
		s.ytUpdate(id, func(r *ytRecord) { r.Status = status })
		cancel()
	}
	msg := i18n.T("yt.ainda_nao_acabou")
	if last != nil {
		msg = i18n.T("yt.nao_sei_se_acabou", "err", last)
	}
	s.ytUpdate(id, func(r *ytRecord) { r.PrivacyError = msg })
}

func (s *Server) ytPrivacy(id, status string, err error) {
	s.ytUpdate(id, func(r *ytRecord) {
		r.Status, r.PrivacyError = status, ""
		if err != nil {
			r.PrivacyError = i18n.T("yt.nao_listado_falhou", "err", err)
			return
		}
		r.Privacy = "unlisted"
	})
	if err != nil {
		s.ytFailed(err)
		return
	}
	s.ytMu.Lock()
	for i, b := range s.ytRemote {
		if b.ID == id {
			s.ytRemote[i].Privacy = "unlisted"
		}
	}
	s.ytMu.Unlock()
	s.Log(i18n.T("log.yt.nao_listado", "id", id))
}

// RunYouTube finishes streams that ended while the program was closed.
func (s *Server) RunYouTube() {
	s.ytMu.Lock()
	var ids []string
	for _, r := range s.ytRecs {
		if r.Ended && !r.Test && r.Privacy != "unlisted" && r.PrivacyError == "" {
			ids = append(ids, r.BroadcastID)
		}
	}
	s.ytMu.Unlock()
	for _, id := range ids {
		s.ytFinishLater(id)
	}
}

// ytSummary goes with the stream state, for the stream bar.
func (s *Server) ytSummary() map[string]any {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	if !s.yt.connected() && s.ytEnded == "" {
		return nil
	}
	m := map[string]any{"connected": s.yt.connected(), "expired": s.yt.Expired, "live": nil, "ended": nil,
		"waiting": s.ytBusy[s.ytEnded], "error": s.ytErr}
	for _, r := range s.ytRecs {
		if r.OnAir {
			m["live"] = r
		}
		if r.BroadcastID == s.ytEnded {
			m["ended"] = r
		}
	}
	return m
}

func (s *Server) ytThumbnail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	id := r.FormValue("broadcast_id")
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, i18n.T("api.yt.falta_imagem"))
		return
	}
	defer f.Close()
	png, err := io.ReadAll(io.LimitReader(f, ytThumbMax+1))
	if err == nil && len(png) > ytThumbMax {
		fail(w, http.StatusRequestEntityTooLarge, i18n.T("api.yt.miniatura_8mb"))
		return
	}
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")) {
		fail(w, http.StatusBadRequest, i18n.T("api.yt.miniatura_png"))
		return
	}
	if s.ytRecord(id) == nil || !s.ytConnected() {
		fail(w, http.StatusNotFound, i18n.T("api.yt.nao_e_nossa"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	err = s.ytClient().SetThumbnail(ctx, id, png)
	s.ytUpdate(id, func(rec *ytRecord) {
		rec.ThumbnailSet, rec.ThumbnailError = err == nil, ""
		if err != nil {
			rec.ThumbnailError = err.Error()
		}
	})
	if err != nil {
		s.ytFailed(err)
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	ok(w)
}

// ytUnlist is "Tornar não listado": the retry after a failure, and the
// button in Transmissões.
func (s *Server) ytUnlist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BroadcastID string `json:"broadcast_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.BroadcastID == "" || !s.ytConnected() {
		fail(w, http.StatusBadRequest, i18n.T("api.yt.ligue_primeiro"))
		return
	}
	if !s.ytKnown(req.BroadcastID) {
		fail(w, http.StatusNotFound, i18n.T("api.yt.fora_da_lista"))
		return
	}
	if rec := s.ytRecord(req.BroadcastID); rec != nil && rec.Test {
		fail(w, http.StatusConflict, i18n.T("api.yt.teste_privado"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	err := s.ytClient().SetPrivacy(ctx, req.BroadcastID, "unlisted")
	if rec := s.ytRecord(req.BroadcastID); rec != nil {
		s.ytPrivacy(req.BroadcastID, rec.Status, err)
	} else if err != nil {
		s.ytFailed(err)
	}
	if err != nil {
		fail(w, http.StatusBadGateway, i18n.T("yt.nao_listado_falhou", "err", err))
		return
	}
	if s.ytRecord(req.BroadcastID) == nil {
		s.ytMu.Lock()
		for i, b := range s.ytRemote {
			if b.ID == req.BroadcastID {
				s.ytRemote[i].Privacy = "unlisted"
			}
		}
		s.ytMu.Unlock()
	}
	reply(w, map[string]any{"ok": true, "privacy": "unlisted"})
}

// ytKnown: id is in the record or in the channel's list read for Transmissões.
func (s *Server) ytKnown(id string) bool {
	s.ytMu.Lock()
	defer s.ytMu.Unlock()
	for _, r := range s.ytRecs {
		if r.BroadcastID == id {
			return true
		}
	}
	for _, b := range s.ytRemote {
		if b.ID == id {
			return true
		}
	}
	return false
}

// ytThumbMax is what the thumbnail upload takes (the 1280x720 PNG is far less).
const ytThumbMax = 8 << 20

// ytExternal is the escalão filter value for streams made outside the program.
func ytExternal() string { return i18n.T("yt.fora_hallstream") }

func ytSeason(date string) string {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		return ""
	}
	t = t.In(lisbon)
	y := t.Year()
	if t.Month() < time.August {
		y--
	}
	return fmt.Sprintf("%d/%02d", y, (y+1)%100)
}

// ytHistory is Transmissões: the local record with the channel's finished
// broadcasts, newest first.
func (s *Server) ytHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.ytMu.Lock()
	stale := time.Since(s.ytRemoteAt) > ytRemoteTTL || q.Get("refresh") == "1" && time.Since(s.ytRemoteAt) > time.Minute
	s.ytMu.Unlock()
	if stale && s.ytConnected() && !s.ytGet().Expired {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		list, err := s.ytClient().Completed(ctx, 100)
		cancel()
		s.ytMu.Lock()
		s.ytRemoteAt, s.ytRemoteErr = time.Now(), ""
		if err != nil {
			s.ytRemoteErr = err.Error()
		} else {
			s.ytRemote = list
		}
		s.ytMu.Unlock()
		if err != nil {
			s.ytFailed(err)
		}
	}
	s.ytMu.Lock()
	items := slices.Clone(s.ytRecs)
	at := map[string]int{}
	for i, rec := range items {
		at[rec.BroadcastID] = i
	}
	for _, b := range s.ytRemote {
		if i, ok := at[b.ID]; ok {
			items[i].Privacy, items[i].Status = b.Privacy, b.Status
			continue
		}
		items = append(items, ytRecord{BroadcastID: b.ID, VideoID: b.ID, Title: b.Title, Date: b.Start.Format(time.RFC3339),
			Privacy: b.Privacy, Link: "https://youtu.be/" + b.ID, Status: b.Status, External: true})
	}
	remoteErr := s.ytRemoteErr
	s.ytMu.Unlock()

	escalao, seas := q.Get("escalao"), q.Get("season")
	var groups, seasons []string
	out := []ytRecord{}
	for _, it := range items {
		g := it.Escalao
		if it.External {
			g = ytExternal()
		}
		if g != "" && !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
		se := ytSeason(it.Date)
		if se != "" && !slices.Contains(seasons, se) {
			seasons = append(seasons, se)
		}
		if (escalao == "" || g == escalao) && (seas == "" || se == seas) {
			out = append(out, it)
		}
	}
	slices.SortStableFunc(out, func(a, b ytRecord) int { return strings.Compare(utcOf(b.Date), utcOf(a.Date)) })
	slices.Sort(groups)
	slices.Sort(seasons)
	slices.Reverse(seasons)
	reply(w, map[string]any{"items": out, "escaloes": groups, "seasons": seasons, "remote_error": remoteErr,
		"connected": s.ytConnected()})
}

func utcOf(date string) string {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// teamName turns a scoreboard name ("CASA") into "Casa"; short
// words stay as they are (CAS, VDB).
func teamName(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		lw := strings.ToLower(w)
		switch {
		case i > 0 && slices.Contains([]string{"de", "da", "do", "dos", "das", "e"}, lw):
			words[i] = lw
		case utf8.RuneCountInString(w) > 3:
			r, n := utf8.DecodeRuneInString(lw)
			words[i] = string(unicode.ToUpper(r)) + lw[n:]
		}
	}
	return strings.Join(words, " ")
}

// broadcastText is the broadcast's title (at most 100 characters, no < or >,
// as YouTube asks) and description.
func broadcastText(g state.Game, now time.Time) (string, string) {
	home, away := teamName(g.Home.Name), teamName(g.Away.Name)
	if g.AtHome != nil && !*g.AtHome {
		home, away = away, home
	}
	match := i18n.T("yt.titulo_jogo", "casa", home, "fora", away)
	if home == "" || away == "" {
		match = home + away
	}
	round := g.Round
	if n, ok := strings.CutPrefix(round, i18n.T("jogo.jornada", "n", "")); ok {
		round = i18n.T("yt.jornada_curta", "n", n)
	}
	group := ""
	if g.Group != nil {
		group = g.Group.Name
	}
	comp := strings.TrimSpace(g.Competition + " " + round)
	title := joinDot(match, group, comp)
	if utf8.RuneCountInString(ytClean(title)) > 100 {
		title = joinDot(match, group)
	}
	title = ytClean(title)
	if r := []rune(title); len(r) > 100 {
		title = string(r[:100])
	}
	if strings.TrimSpace(title) == "" {
		title = i18n.T("yt.titulo_omissao")
	}
	when := now
	if t, err := time.Parse(time.RFC3339, g.Start); err == nil {
		when = t
	}
	when = when.In(lisbon)
	date := i18n.T("yt.data", "dia_semana", weekday(when.Weekday()), "dia", when.Day(), "mes", month(when.Month()), "ano", when.Year(), "hora", when.Format("15:04"))
	desc := ytClean(strings.Join(nonEmpty(joinDot(g.Competition, g.Round, group), g.Venue, date), "\n"))
	return title, desc
}

func weekday(d time.Weekday) string {
	return [...]string{i18n.T("data.dia.0"), i18n.T("data.dia.1"), i18n.T("data.dia.2"), i18n.T("data.dia.3"),
		i18n.T("data.dia.4"), i18n.T("data.dia.5"), i18n.T("data.dia.6")}[d]
}

func month(m time.Month) string {
	return [...]string{i18n.T("data.mes.1"), i18n.T("data.mes.2"), i18n.T("data.mes.3"), i18n.T("data.mes.4"),
		i18n.T("data.mes.5"), i18n.T("data.mes.6"), i18n.T("data.mes.7"), i18n.T("data.mes.8"),
		i18n.T("data.mes.9"), i18n.T("data.mes.10"), i18n.T("data.mes.11"), i18n.T("data.mes.12")}[m-1]
}

func joinDot(parts ...string) string { return strings.Join(nonEmpty(parts...), " · ") }

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ytClean(s string) string { return strings.NewReplacer("<", "", ">", "").Replace(s) }
