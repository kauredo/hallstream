package youtube

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeGoogle is the token endpoint: it answers only a code exchange whose
// verifier matches the challenge in the consent link.
func fakeGoogle(t *testing.T, challenge *string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "the-code" ||
			r.Form.Get("client_secret") != "segredo" || base64.RawURLEncoding.EncodeToString(sum[:]) != *challenge ||
			!strings.HasPrefix(r.Form.Get("redirect_uri"), "http://127.0.0.1:") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "a1", "refresh_token": "r1", "token_type": "Bearer", "expires_in": 3600})
	}))
	t.Cleanup(srv.Close)
	old := Google
	Google.TokenURL = srv.URL
	t.Cleanup(func() { Google = old })
}

type result struct {
	tok *oauth2.Token
	err error
}

func start(t *testing.T, wait time.Duration) (*Login, url.Values, chan result) {
	t.Helper()
	got := make(chan result, 2)
	l, err := StartLogin(Credentials{"id.apps.googleusercontent.com", "segredo"}, wait, func(_ *Login, tok *oauth2.Token, err error) { got <- result{tok, err} })
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(l.URL)
	return l, u.Query(), got
}

func get(t *testing.T, redirect string, q url.Values, host string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", redirect+"/?"+q.Encode(), nil)
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestLoginStateAndPKCE(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	_, q, got := start(t, time.Minute)
	challenge = q.Get("code_challenge")
	redirect := q.Get("redirect_uri")
	if q.Get("code_challenge_method") != "S256" || challenge == "" || q.Get("access_type") != "offline" ||
		q.Get("scope") != Scope || len(q.Get("state")) < 40 || !strings.HasPrefix(redirect, "http://127.0.0.1:") {
		t.Fatalf("consent link %v", q)
	}
	// Another page guessing at the callback: refused, and the login waits on.
	if code := get(t, redirect, url.Values{"state": {"adivinha"}, "code": {"the-code"}}, ""); code != 400 {
		t.Errorf("wrong state: %d", code)
	}
	// A page that points its own name at 127.0.0.1.
	if code := get(t, redirect, url.Values{"state": {q.Get("state")}, "code": {"the-code"}}, "evil.example:80"); code != 404 {
		t.Errorf("other host: %d", code)
	}
	select {
	case r := <-got:
		t.Fatalf("finished early: %+v", r)
	default:
	}
	if code := get(t, redirect, url.Values{"state": {q.Get("state")}, "code": {"the-code"}}, ""); code != 200 {
		t.Errorf("good callback: %d", code)
	}
	r := <-got
	if r.err != nil || r.tok.RefreshToken != "r1" {
		t.Fatalf("%+v", r)
	}
	// One shot: the listener is gone.
	if _, err := http.Get(redirect + "/"); err == nil {
		t.Error("still listening")
	}
}

func TestLoginWrongVerifier(t *testing.T) {
	challenge := "not-the-one"
	fakeGoogle(t, &challenge)
	_, q, got := start(t, time.Minute)
	get(t, q.Get("redirect_uri"), url.Values{"state": {q.Get("state")}, "code": {"the-code"}}, "")
	if r := <-got; r.err == nil || r.tok != nil {
		t.Fatalf("%+v", r)
	}
}

func TestLoginDenied(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	_, q, got := start(t, time.Minute)
	get(t, q.Get("redirect_uri"), url.Values{"state": {q.Get("state")}, "error": {"access_denied"}}, "")
	if r := <-got; r.err == nil || !strings.Contains(r.err.Error(), "cancelado") {
		t.Fatalf("%+v", r)
	}
}

func TestLoginTimeout(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	_, _, got := start(t, 20*time.Millisecond)
	if r := <-got; !errors.Is(r.err, ErrTimeout) {
		t.Fatalf("%+v", r)
	}
}

func TestLoginCancel(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	l, q, got := start(t, 50*time.Millisecond)
	l.Cancel()
	time.Sleep(100 * time.Millisecond)
	select {
	case r := <-got:
		t.Fatalf("done after Cancel: %+v", r)
	default:
	}
	if _, err := http.Get(q.Get("redirect_uri") + "/"); err == nil {
		t.Error("still listening")
	}
}

func TestLoginErrorEscaped(t *testing.T) {
	var challenge string
	fakeGoogle(t, &challenge)
	_, q, got := start(t, time.Minute)
	resp, err := http.Get(q.Get("redirect_uri") + "/?" + url.Values{"state": {q.Get("state")}, "error": {"<script>x</script>"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	<-got
	if strings.Contains(string(b), "<script>") {
		t.Errorf("%s", b)
	}
}
