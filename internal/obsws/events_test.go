package obsws

import (
	"slices"
	"sync"
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
	go Watch(cfg, stop, func(name string, _ bool) { got <- name })
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
	go func() { res <- Watch(cfg, stop, func(name string, _ bool) { got <- name }) }()
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

// drop closes every connection, as OBS closing does.
func (f *fake) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ws := range f.conns {
		ws.Close()
	}
}

// raw sends a message with op to every connection.
func (f *fake) raw(op int, d map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ws := range f.conns {
		send(ws, op, d)
	}
}

func watchFor(t *testing.T, cfg Config) (<-chan string, <-chan error) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	got, res := make(chan string, 64), make(chan error, 1)
	go func() { res <- Watch(cfg, stop, func(name string, _ bool) { got <- name }) }()
	return got, res
}

func ended(t *testing.T, res <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-res:
		return err
	case <-time.After(within):
		t.Fatal("Watch did not return")
		return nil
	}
}

func TestWatchDropped(t *testing.T) {
	f := &fake{program: "Jogo"}
	got, res := watchFor(t, fakeOBS(t, "", f))
	next(t, got)
	f.drop()
	if err := ended(t, res, 2*time.Second); err == nil {
		t.Error("a dropped connection returned nil")
	}
}

func TestWatchWrongPassword(t *testing.T) {
	cfg := fakeOBS(t, "segredo", &fake{program: "Jogo"})
	cfg.Password = "errada"
	_, res := watchFor(t, cfg)
	if err := ended(t, res, 7*time.Second); err == nil {
		t.Error("a wrong password returned nil")
	}
}

// oldReply answers GetCurrentProgramScene with currentProgramSceneName only,
// as OBS before 30 does.
type oldReply struct{ *fake }

func (o oldReply) answer(typ string, d map[string]any) (bool, int, any) {
	if typ == "GetCurrentProgramScene" {
		return true, 100, map[string]any{"currentProgramSceneName": o.scene()}
	}
	return o.fake.answer(typ, d)
}

func TestWatchOldReply(t *testing.T) {
	got, _ := watchFor(t, fakeOBS(t, "", oldReply{&fake{program: "Jogo"}}))
	if s := next(t, got); s != "Jogo" {
		t.Errorf("scene %q", s)
	}
}

func TestWatchIgnoresOtherReplies(t *testing.T) {
	f := &fake{program: "Jogo"}
	got, _ := watchFor(t, fakeOBS(t, "", f))
	next(t, got)
	f.raw(7, map[string]any{"requestType": "GetCurrentProgramScene", "requestId": "7",
		"requestStatus": map[string]any{"result": true, "code": 100}, "responseData": map[string]any{"sceneName": "Outra"}})
	f.emit("CurrentProgramSceneChanged", map[string]any{"sceneName": "Câmara 2"})
	if s := next(t, got); s != "Câmara 2" {
		t.Errorf("took %q", s)
	}
}

// silentAfter answers GetCurrentProgramScene once, then never again, like
// an OBS that froze with the connection still open.
type silentAfter struct {
	*fake
	mu    sync.Mutex
	asked int
}

func (s *silentAfter) answer(typ string, d map[string]any) (bool, int, any) {
	s.mu.Lock()
	s.asked++
	n := s.asked
	s.mu.Unlock()
	if n > 1 {
		return false, silent, nil
	}
	return s.fake.answer(typ, d)
}

func shortWatch(t *testing.T, ping, silence time.Duration) {
	p, q := WatchPing, WatchSilence
	WatchPing, WatchSilence = ping, silence
	t.Cleanup(func() { WatchPing, WatchSilence = p, q })
}

func TestWatchSilenceIsAnError(t *testing.T) {
	shortWatch(t, 20*time.Millisecond, 150*time.Millisecond)
	got, res := watchFor(t, fakeOBS(t, "", &silentAfter{fake: &fake{program: "Jogo"}}))
	next(t, got)
	if err := ended(t, res, 2*time.Second); err == nil {
		t.Error("a silent OBS returned nil")
	}
}

func TestWatchPingKeepsItAlive(t *testing.T) {
	shortWatch(t, 20*time.Millisecond, 150*time.Millisecond)
	got, res := watchFor(t, fakeOBS(t, "", &fake{program: "Jogo"}))
	for range 15 { // 300 ms of answers, twice the silence
		next(t, got)
	}
	select {
	case err := <-res:
		t.Fatalf("Watch ended with %v", err)
	default:
	}
}
