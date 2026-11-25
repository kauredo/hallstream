package web

import (
	"net/http/httptest"
	"testing"

	"github.com/kauredo/hallstream/internal/store"
)

func TestStreamDestination(t *testing.T) {
	s := newServer(t)
	for _, c := range []struct {
		body store.Stream
		code int
	}{
		{store.Stream{Service: "rtmp", Server: "http://live.example.com/app", Key: "k"}, 400},
		{store.Stream{Service: "rtmp", Server: "rtmp://live.example.com/app", Key: " "}, 400},
		{store.Stream{Service: "twitch"}, 400},
	} {
		if code := call(t, s, "PUT", "/api/stream", c.body, nil); code != c.code {
			t.Errorf("PUT %+v: %d, want %d", c.body, code, c.code)
		}
	}
	if got := s.Store.Settings().Stream; got != (store.Stream{}) {
		t.Fatalf("a refused PUT saved %+v", got)
	}

	want := store.Stream{Service: "rtmp", Server: "rtmps://live.example.com/app", Key: "abc-123"}
	if code := call(t, s, "PUT", "/api/stream", store.Stream{Service: "rtmp", Server: " " + want.Server, Key: want.Key + " "}, nil); code != 200 {
		t.Fatalf("PUT rtmp: %d", code)
	}
	if got := s.Store.Settings().Stream; got != want || !s.rtmp() {
		t.Fatalf("saved %+v, want %+v", got, want)
	}

	// The settings form keeps the destination, whatever it sends.
	set := s.Store.Settings()
	set.Stream = store.Stream{}
	call(t, s, "PUT", "/api/settings", set, nil)
	if got := s.Store.Settings().Stream; got != want {
		t.Errorf("after PUT /api/settings: %+v", got)
	}

	// Another device never sees the key.
	req := httptest.NewRequest("GET", "/api/settings", nil)
	req.RemoteAddr = "192.168.1.20:5000"
	if got := forCaller(s.Store.Settings(), req).Stream; got.Key != "" || got.Server != want.Server {
		t.Errorf("for another device: %+v", got)
	}

	call(t, s, "PUT", "/api/stream", store.Stream{Service: "youtube", Server: "rtmp://x", Key: "y"}, nil)
	if got := s.Store.Settings().Stream; got != (store.Stream{Service: "youtube"}) || s.rtmp() {
		t.Errorf("back to YouTube: %+v", got)
	}
}
