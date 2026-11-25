package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.9.0", "1.10.0", true},
		{"1.2.3", "2.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.3.0", "1.2.9", false},
		{"dev", "9.9.9", false},
		{"0.0.0-dev.abc1234", "0.1.0", false},
		{"1.0.0", "1.0", false},
	} {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestLatest(t *testing.T) {
	body := `{"tag_name":"v1.4.0","html_url":"https://github.com/kauredo/hallstream/releases/tag/v1.4.0"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "Hallstream/1.0.0" {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()
	got, err := Latest(context.Background(), srv.Client(), srv.URL, "Hallstream/1.0.0")
	if err != nil || got.Version != "1.4.0" || got.URL != "https://github.com/kauredo/hallstream/releases/tag/v1.4.0" {
		t.Fatalf("Latest = %+v, %v", got, err)
	}
	body = `{"tag_name":"nightly","html_url":"https://github.com/kauredo/hallstream/releases/tag/nightly"}`
	if _, err := Latest(context.Background(), srv.Client(), srv.URL, "Hallstream/1.0.0"); err == nil {
		t.Error("a tag that is not a version passed")
	}
	body = `{"tag_name":"v2.0.0","html_url":"https://evil.example/x"}`
	if _, err := Latest(context.Background(), srv.Client(), srv.URL, "Hallstream/1.0.0"); err == nil {
		t.Error("a page outside GitHub passed")
	}
}
