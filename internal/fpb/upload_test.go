package fpb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// tokenSeen counts the requests that carried the token.
func tokenSeen(n *atomic.Int32, then http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), "tok") {
			n.Add(1)
		}
		then(w, r)
	}
}

func ok(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) }

func TestPostOnlyToUploadHost(t *testing.T) {
	var seen atomic.Int32
	tlsSrv := httptest.NewTLSServer(tokenSeen(&seen, ok))
	defer tlsSrv.Close()
	plain := httptest.NewServer(tokenSeen(&seen, ok))
	defer plain.Close()

	client := func(base string) *Client {
		c := New(func() string { return base }, t.TempDir(), "t")
		c.HTTP.Transport = tlsSrv.Client().Transport
		c.UploadHost = "127.0.0.1"
		return c
	}
	if code, _, err := client(tlsSrv.URL).Post("/api/marcador/logs", "tok", "text/plain", nil); err != nil || code != 200 || seen.Load() != 1 {
		t.Fatalf("https to the host: %d %v, seen %d", code, err, seen.Load())
	}
	// http, and another host, never get the token.
	if _, _, err := client(plain.URL).Post("/api/marcador/logs", "tok", "text/plain", nil); !errors.Is(err, ErrUploadHost) {
		t.Errorf("http: %v", err)
	}
	other := client(tlsSrv.URL)
	other.UploadHost = "calendariofpb.pt"
	if _, _, err := other.Post("/api/marcador/logs", "tok", "text/plain", nil); !errors.Is(err, ErrUploadHost) {
		t.Errorf("other host: %v", err)
	}
	if seen.Load() != 1 {
		t.Errorf("the token went out %d times", seen.Load())
	}
}

func TestPostRedirectStaysOnUploadHost(t *testing.T) {
	var seen atomic.Int32
	plain := httptest.NewServer(tokenSeen(&seen, ok))
	defer plain.Close()
	otherTLS := httptest.NewTLSServer(tokenSeen(&seen, ok))
	defer otherTLS.Close()
	// localhost is another host to the pin, though the same machine.
	elsewhere := strings.Replace(otherTLS.URL, "127.0.0.1", "localhost", 1)
	for _, to := range []string{plain.URL, elsewhere} {
		src := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, to+r.URL.Path, http.StatusTemporaryRedirect)
		}))
		c := New(func() string { return src.URL }, t.TempDir(), "t")
		c.HTTP.Transport = src.Client().Transport
		c.UploadHost = "127.0.0.1"
		if _, _, err := c.Post("/api/marcador/logs", "tok", "text/plain", nil); !errors.Is(err, ErrUploadHost) {
			t.Errorf("redirect to %s: %v", to, err)
		}
		src.Close()
	}
	if seen.Load() != 0 {
		t.Errorf("the token went out %d times", seen.Load())
	}
}

func TestUploadAllowed(t *testing.T) {
	c := New(func() string { return "" }, t.TempDir(), "t")
	for u, want := range map[string]bool{
		"https://calendariofpb.pt/x":         true,
		"https://API.calendariofpb.pt:443/x": true,
		"http://calendariofpb.pt/x":          false,
		"https://calendariofpb.pt.evil.pt/":  false,
		"https://evilcalendariofpb.pt/":      false,
	} {
		p, _ := url.Parse(u)
		if c.uploadAllowed(p) != want {
			t.Errorf("%s: want %v", u, want)
		}
	}
}
