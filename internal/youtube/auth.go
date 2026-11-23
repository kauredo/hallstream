// Package youtube signs in to the club's YouTube channel (OAuth for desktop
// apps: a loopback redirect with PKCE) and makes the few YouTube Data API v3
// calls the program needs, as plain REST.
package youtube

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// Scope covers broadcasts, streams, video privacy and thumbnails.
const Scope = "https://www.googleapis.com/auth/youtube"

// Google is the OAuth endpoint for installed apps; tests stand in their own.
var Google = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// RevokeURL takes a token and ends the program's access to the channel.
var RevokeURL = "https://oauth2.googleapis.com/revoke"

// Credentials are the club's own OAuth client, a "Desktop app" in Google Cloud.
type Credentials struct {
	ClientID     string
	ClientSecret string
}

func (c Credentials) config(redirect string) *oauth2.Config {
	return &oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret,
		Endpoint: Google, RedirectURL: redirect, Scopes: []string{Scope}}
}

var ErrTimeout = errors.New("o login no YouTube demorou demasiado. Carregue outra vez em Ligar ao YouTube.")

// Login is one sign-in waiting for Google to send the browser back.
type Login struct {
	URL      string // the consent page to open
	state    string
	verifier string
	conf     *oauth2.Config
	ln       net.Listener
	srv      *http.Server
	once     sync.Once
	done     func(*Login, *oauth2.Token, error)
}

// StartLogin listens on 127.0.0.1 on a free port for one redirect with the
// right state, for at most wait. done runs once, with the token or why not.
func StartLogin(c Credentials, wait time.Duration, done func(*Login, *oauth2.Token, error)) (*Login, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	l := &Login{state: random(32), verifier: oauth2.GenerateVerifier(), ln: ln, done: done,
		conf: c.config("http://127.0.0.1:" + strconv.Itoa(port))}
	l.URL = l.conf.AuthCodeURL(l.state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(l.verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))
	l.srv = &http.Server{Handler: http.HandlerFunc(l.callback), ReadHeaderTimeout: 10 * time.Second}
	go l.srv.Serve(ln)
	time.AfterFunc(wait, func() { l.finish(nil, ErrTimeout) })
	return l, nil
}

// Cancel stops waiting; done is not called.
func (l *Login) Cancel() { l.once.Do(l.close) }

// close stops listening; the page being sent still goes out.
func (l *Login) close() {
	l.ln.Close()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		l.srv.Shutdown(ctx)
	}()
}

func (l *Login) finish(tok *oauth2.Token, err error) {
	l.once.Do(func() {
		l.close()
		l.done(l, tok, err)
	})
}

func (l *Login) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Only the browser coming back from Google, on this address, counts.
	if r.Method != http.MethodGet || r.URL.Path != "/" || r.Host != l.ln.Addr().String() {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(l.state)) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, page("Este pedido não veio do Hallstream. Volte ao painel e carregue em Ligar ao YouTube."))
		return
	}
	if e := q.Get("error"); e != "" || q.Get("code") == "" {
		msg := "O login no YouTube foi cancelado."
		if e != "" && e != "access_denied" {
			msg = "O Google não deixou ligar (" + e + ")."
		}
		fmt.Fprint(w, page(msg+" Volte ao painel para tentar outra vez."))
		l.finish(nil, errors.New(msg))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tok, err := l.conf.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(l.verifier))
	if err == nil && tok.RefreshToken == "" {
		err = errors.New("o Google não deu acesso duradouro. Tire o acesso do Hallstream em myaccount.google.com/permissions e ligue outra vez")
	}
	if err != nil {
		fmt.Fprint(w, page("Não consegui acabar o login. Volte ao painel para ver o que aconteceu."))
		l.finish(nil, loginError(err))
		return
	}
	fmt.Fprint(w, page("Ligado ao YouTube. Pode fechar esta janela e voltar ao painel."))
	l.finish(tok, nil)
}

// loginError keeps Google's error code, never the request it answered.
func loginError(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		code := re.ErrorCode
		if code == "" {
			code = strconv.Itoa(re.Response.StatusCode)
		}
		if code == "invalid_client" {
			return errors.New("o Google não aceitou o ID ou o segredo do cliente OAuth. Confirme-os em Definições > YouTube")
		}
		return errors.New("o Google recusou o login (" + code + ")")
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return errors.New("sem ligação ao Google: " + ue.Err.Error())
	}
	return err
}

func page(msg string) string {
	return `<!doctype html><html lang="pt-PT"><meta charset="utf-8"><title>Hallstream</title>` +
		`<body style="font:18px system-ui,sans-serif;margin:48px;color:#181841"><p>` + html.EscapeString(msg) + `</p></body></html>`
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Revoke ends the program's access with tok (the refresh token).
func Revoke(ctx context.Context, hc *http.Client, tok string) error {
	// In the body, never the URL, which ends up in errors.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RevokeURL, strings.NewReader(url.Values{"token": {tok}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 400 is a token Google no longer knows: nothing left to revoke.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("o Google respondeu %d", resp.StatusCode)
	}
	return nil
}
