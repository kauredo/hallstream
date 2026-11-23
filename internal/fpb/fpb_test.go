package fpb

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pngOf(t *testing.T, fill func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 40 {
			img.Set(x, y, fill(x, y))
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestAPIRedirectsStayOnCalendar(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":1,"name":"Clube Exemplo"}]`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/teams" {
			http.Redirect(w, r, other.URL+"/api/teams", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/api/teams", http.StatusFound) // same host
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")
	if _, err := c.Teams("Casa"); err == nil {
		t.Error("followed a redirect to another host")
	}
	if _, err := c.Get("/api/old"); err == nil || !strings.Contains(err.Error(), "fora do calendário") {
		t.Errorf("same-host redirect then off-host: %v", err)
	}
}

func TestPickColor(t *testing.T) {
	white := color.NRGBA{255, 255, 255, 255}
	tests := []struct {
		name string
		img  []byte
		want string // "" = any colour that passes contrast
	}{
		{"dark red stays", pngOf(t, func(x, y int) color.Color {
			if x < 30 {
				return color.NRGBA{0xBD, 0x15, 0x22, 255}
			}
			return white
		}), "#BD1522"},
		{"yellow gets darkened", pngOf(t, func(x, y int) color.Color {
			return color.NRGBA{255, 220, 0, 255}
		}), ""},
		{"most common wins over white and black", pngOf(t, func(x, y int) color.Color {
			switch {
			case x < 15:
				return white
			case x < 25:
				return color.NRGBA{0, 0, 0, 255}
			case x < 35:
				return color.NRGBA{0x2E, 0x6F, 0xD8, 255}
			}
			return color.NRGBA{0, 0x80, 0, 255}
		}), ""},
		// Atlético do Vale: mostly gold, then purple. The purple, not a brown.
		{"dark second colour beats a light first", pngOf(t, func(x, y int) color.Color {
			if x < 15 {
				return color.NRGBA{0x5B, 0x2A, 0x86, 255}
			}
			return color.NRGBA{0xF5, 0xC5, 0x18, 255}
		}), "#5B2A86"},
		{"greys only", pngOf(t, func(x, y int) color.Color { return color.NRGBA{120, 120, 120, 255} }), Neutral},
		{"transparent only", pngOf(t, func(x, y int) color.Color { return color.NRGBA{255, 0, 0, 0} }), Neutral},
		{"svg", []byte("<svg/>"), Neutral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PickColor(tt.img)
			if tt.want != "" && got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if c := Contrast(got); c < 4.5 {
				t.Errorf("%s has contrast %.2f", got, c)
			}
		})
	}
	// Blue (the 3rd case) covers 10 columns, green only 5.
	blue := PickColor(tests[2].img)
	if c, _ := parseHex(blue); !(c.b > c.r && c.b > c.g) {
		t.Errorf("want a blue, got %s", blue)
	}
}

func TestEnsureContrast(t *testing.T) {
	for _, in := range []string{"#FFFFFF", "#ff0", "#2E6FD8", "nada", ""} {
		got := EnsureContrast(in)
		if Contrast(got) < 4.5 {
			t.Errorf("%q -> %s contrast %.2f", in, got, Contrast(got))
		}
	}
	if got := EnsureContrast("#BD1522"); got != "#BD1522" {
		t.Errorf("good colour changed to %s", got)
	}
}

func TestGetCachesAndHandles404(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "hallstream/test" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		switch {
		case !up:
			w.WriteHeader(http.StatusBadGateway)
		case r.URL.Path == "/api/teams/1/roster":
			w.Write([]byte(`{"team":{"id":1,"name":"CASA"},"season":"2026/27","players":[{"player_id":5,"name":"Rui","jersey_number":7,"games":2}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")

	r, err := c.Roster(1)
	if err != nil || len(r.Players) != 1 || r.Players[0].JerseyNumber != "7" {
		t.Fatalf("roster %+v, %v", r, err)
	}
	if _, err := c.NextGames(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}

	up = false
	r, err = c.Roster(1)
	if err != nil || len(r.Players) != 1 {
		t.Fatalf("cached roster %+v, %v", r, err)
	}
	if ok, at := c.Status(); ok || at.IsZero() {
		t.Errorf("status reachable=%v cached_at=%v", ok, at)
	}
	if _, err := c.Roster(2); err == nil {
		t.Error("want an error with no cache")
	}
}

func TestRosterPhotoFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"players":[{"player_id":1,"fpb_athlete_id":296222,"photo_url":"https://sav2.fpb.pt/a.png"},{"player_id":2,"photo_url":null}]}`))
	}))
	defer srv.Close()
	r, err := New(func() string { return srv.URL }, t.TempDir(), "test").Roster(1)
	if err != nil || r.Players[0].FPBAthleteID != 296222 || r.Players[0].PhotoURL != "https://sav2.fpb.pt/a.png" || r.Players[1].PhotoURL != "" {
		t.Fatalf("roster %+v, %v", r, err)
	}
}

func TestDownloadPhotoHosts(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpg"))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/away" {
			http.Redirect(w, r, other.URL+"/x.jpg", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpg"))
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")

	if _, _, err := c.DownloadPhoto(context.Background(), srv.URL+"/p.jpg"); err == nil {
		t.Error("http and an unlisted host accepted by default")
	}
	c.AllowHTTP = true
	c.PhotoHosts = []string{strings.TrimPrefix(srv.URL, "http://")}
	if b, ext, err := c.DownloadPhoto(context.Background(), srv.URL+"/p.jpg"); err != nil || string(b) != "jpg" || ext != ".jpg" {
		t.Errorf("listed host: %q %q %v", b, ext, err)
	}
	if _, _, err := c.DownloadPhoto(context.Background(), other.URL+"/p.jpg"); err == nil {
		t.Error("unlisted host accepted")
	}
	if _, _, err := c.DownloadPhoto(context.Background(), srv.URL+"/away"); err == nil {
		t.Error("redirect to an unlisted host followed")
	}
	if _, _, err := c.DownloadPhoto(context.Background(), "https://sav2.fpb.pt.evil.com/p.jpg"); err == nil {
		t.Error("look-alike host accepted")
	}
}

func TestDownloadTooBig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(make([]byte, 5<<20+1))
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")
	c.AllowHTTP = true
	c.PhotoHosts = []string{strings.TrimPrefix(srv.URL, "http://")}
	if b, _, err := c.DownloadPhoto(context.Background(), srv.URL+"/p.jpg"); err == nil || b != nil {
		t.Errorf("oversized photo: %d bytes, %v", len(b), err)
	}
}

func TestRealPhoto(t *testing.T) {
	if got := RealPhoto("https://www.fpb.pt/wp-content/themes/fpbasquetebol/assets/images/noplayer.png"); got != "" {
		t.Errorf("placeholder kept: %q", got)
	}
	if got := RealPhoto("https://sav2.fpb.pt/uploads/utilizadores/1.png"); got == "" {
		t.Error("real photo dropped")
	}
}

func TestGetKeepsCacheWhenAnswerIsNotJSON(t *testing.T) {
	good := `{"team":{"id":1,"name":"CASA"},"season":"2026/27","players":[]}`
	body, ctype := good, "application/json"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")

	if got, err := c.Get("/api/teams/1/roster"); err != nil || string(got) != good {
		t.Fatalf("first get %q, %v", got, err)
	}
	body, ctype = "<html>Entre na rede</html>", "text/html"
	got, err := c.Get("/api/teams/1/roster")
	if err != nil || string(got) != good {
		t.Fatalf("portal page: got %q, %v", got, err)
	}
	if ok, _ := c.Status(); ok {
		t.Error("a non-JSON answer should count as unreachable")
	}
	if cached, _ := os.ReadFile(filepath.Join(c.CacheDir, cacheName("/api/teams/1/roster"))); string(cached) != good {
		t.Errorf("cache overwritten with %q", cached)
	}
}

func TestCacheNameDiffersForQueriesThatSanitiseAlike(t *testing.T) {
	a, b := cacheName("/api/teams?name=Basket A"), cacheName("/api/teams?name=Basket_A")
	if a == b {
		t.Errorf("both queries share %q", a)
	}
	if cacheName("/api/teams?name=Basket A") != a {
		t.Error("name is not stable")
	}
}

func TestGetReadsCacheSavedUnderOldName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	dir := t.TempDir()
	old := `{"team":{"id":1,"name":"CASA"},"players":[]}`
	if err := os.WriteFile(filepath.Join(dir, "api_teams_1_roster.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(func() string { return srv.URL }, dir, "test")
	got, err := c.Get("/api/teams/1/roster")
	if err != nil || string(got) != old {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, at := c.Status(); at.IsZero() {
		t.Error("cached_at not set from the old file")
	}
}

// noSleep makes c take its turns without waiting and returns the waits it asked for.
func noSleep(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.sleep = func(d time.Duration) { waits = append(waits, d) }
	return &waits
}

func near(got, want time.Duration) bool { return got <= want && got > want-time.Second }

func TestAPIRequestsStayUnderTheCalendarLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")
	if perMinute := apiBurst + int(time.Minute/c.Every); perMinute >= 30 {
		t.Errorf("up to %d requests in a minute, the calendar answers 429 past 30", perMinute)
	}
	waits := noSleep(c)
	for range apiBurst + 2 {
		if _, err := c.Get("/api/teams"); err != nil {
			t.Fatal(err)
		}
	}
	// The first apiBurst go at once, then one every c.Every.
	if w := *waits; len(w) != 2 || !near(w[0], c.Every) || !near(w[1], 2*c.Every) {
		t.Errorf("waits %v, want %v then %v", w, c.Every, 2*c.Every)
	}
}

func TestBusyFPBIsNotOffline(t *testing.T) {
	status, retry, hits := http.StatusOK, "", 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		w.WriteHeader(status)
		w.Write([]byte(`[1]`))
	}))
	defer srv.Close()
	c := New(func() string { return srv.URL }, t.TempDir(), "test")
	waits := noSleep(c)
	if _, err := c.Get("/api/a"); err != nil || c.Busy() {
		t.Fatalf("a fresh answer: %v, busy %v", err, c.Busy())
	}

	status, retry = http.StatusTooManyRequests, "7"
	if body, err := c.Get("/api/a"); err != nil || string(body) != `[1]` {
		t.Fatalf("a 429 with a saved copy: %q, %v", body, err)
	}
	if ok, _ := c.Status(); !ok {
		t.Error("a 429 reads as no connection")
	}
	if !c.Busy() {
		t.Error("the saved copy served on a 429 reads as a fresh answer")
	}
	// The next request waits the 7 s the calendar asked for; its own 429 comes with no Retry-After.
	retry = ""
	if _, err := c.Get("/api/b"); !errors.Is(err, ErrBusy) {
		t.Errorf("a 429 with no saved copy: %v, want ErrBusy", err)
	}
	if w := *waits; len(w) != 1 || !near(w[0], 7*time.Second) {
		t.Errorf("waits %v after Retry-After: 7, want one of 7s", w)
	}
	// That pause is a minute, too long to wait: the saved copy at once, and nothing sent.
	before := hits
	if body, err := c.Get("/api/a"); err != nil || string(body) != `[1]` || hits != before || len(*waits) != 1 {
		t.Errorf("inside the pause: %q, %v, %d requests sent, waits %v", body, err, hits-before, *waits)
	}
}

func TestRetryAfter(t *testing.T) {
	date := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(http.TimeFormat) }
	for h, want := range map[string]time.Duration{
		"": busyPause, "soon": busyPause, "-4": busyPause,
		"7": 7 * time.Second, "0": 0,
		"100000": maxPause, "99999999999999999": maxPause,
		date(40 * time.Second): 40 * time.Second, date(-time.Hour): 0, date(24 * time.Hour): maxPause,
	} {
		// A date is whole seconds from now, so it reads up to 2 s short.
		if got := retryAfter(h); got > want || got < want-2*time.Second {
			t.Errorf("Retry-After %q: %v, want %v", h, got, want)
		}
	}
}
