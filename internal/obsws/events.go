package obsws

import (
	"encoding/json"
	"time"

	"golang.org/x/net/websocket"
)

// EventScenes is EventSubscription::Scenes.
const EventScenes = 1 << 2

// The watch asks for the program scene every WatchPing, which also keeps the
// connection alive; with nothing heard from OBS for WatchSilence the
// connection counts as lost.
var (
	WatchPing    = 10 * time.Second
	WatchSilence = 30 * time.Second
)

// Watch opens its own connection to OBS, subscribed to scene events only,
// and calls scene with the program scene's name: from every
// CurrentProgramSceneChanged (event true), and from the answer to
// GetCurrentProgramScene, asked when connected and every WatchPing (event
// false). It returns when the connection fails or goes silent, or when stop
// closes (then nil); the caller reconnects.
func Watch(cfg Config, stop <-chan struct{}, scene func(name string, event bool)) error {
	ping, silence := WatchPing, WatchSilence
	conn, err := dial(cfg, EventScenes)
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Time{})
	done := make(chan struct{})
	defer close(done)
	ask := func() error {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return send(conn, 6, map[string]any{"requestType": "GetCurrentProgramScene", "requestId": "watch"})
	}
	go func() {
		tick := time.NewTicker(ping)
		defer tick.Stop()
		for {
			select {
			case <-stop:
			case <-done:
			case <-tick.C:
				if ask() == nil {
					continue
				}
			}
			conn.Close()
			return
		}
	}()
	err = ask()
	for err == nil {
		conn.SetReadDeadline(time.Now().Add(silence))
		var m message
		if err = websocket.JSON.Receive(conn, &m); err != nil {
			break
		}
		var d struct {
			RequestID string `json:"requestId"`
			Type      string `json:"eventType"`
			Reply     struct {
				Name string `json:"sceneName"`
				Old  string `json:"currentProgramSceneName"`
			} `json:"responseData"`
			Event struct {
				Name string `json:"sceneName"`
			} `json:"eventData"`
		}
		if json.Unmarshal(m.D, &d) != nil {
			continue
		}
		switch {
		case m.Op == 7 && d.RequestID == "watch":
			name := d.Reply.Name
			if name == "" {
				name = d.Reply.Old
			}
			scene(name, false)
		case m.Op == 5 && d.Type == "CurrentProgramSceneChanged":
			scene(d.Event.Name, true)
		}
	}
	select {
	case <-stop:
		return nil
	default:
		return err
	}
}
