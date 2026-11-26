package obsws

import (
	"slices"
	"testing"
	"time"
)

func next(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("no scene came")
		return ""
	}
}

func TestWatchSeesProgramScene(t *testing.T) {
	f := &fake{program: "Jogo"}
	cfg := fakeOBS(t, "segredo", f)
	stop := make(chan struct{})
	defer close(stop)
	got := make(chan string, 4)
	go Watch(cfg, stop, func(name string) { got <- name })
	if s := next(t, got); s != "Jogo" {
		t.Fatalf("first %q", s)
	}
	f.emit("SceneCreated", map[string]any{"sceneName": "Outra"})
	f.emit("CurrentProgramSceneChanged", map[string]any{"sceneName": "Câmara 2"})
	if s := next(t, got); s != "Câmara 2" {
		t.Errorf("after the change %q", s)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.subs, []int{4}) {
		t.Errorf("eventSubscriptions %v", f.subs)
	}
}

func TestWatchStops(t *testing.T) {
	f := &fake{program: "Jogo"}
	cfg := fakeOBS(t, "", f)
	stop := make(chan struct{})
	got := make(chan string, 1)
	res := make(chan error, 1)
	go func() { res <- Watch(cfg, stop, func(name string) { got <- name }) }()
	next(t, got)
	close(stop)
	select {
	case err := <-res:
		if err != nil {
			t.Errorf("stopped with %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch did not return")
	}
}

func TestRequestsStillSubscribeToNothing(t *testing.T) {
	f := &fake{}
	cfg := fakeOBS(t, "", f)
	var c Client
	if err := c.Request(cfg, "GetSceneList", nil, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.subs, []int{0}) {
		t.Errorf("eventSubscriptions %v", f.subs)
	}
}
