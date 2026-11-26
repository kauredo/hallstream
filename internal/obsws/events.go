package obsws

import (
	"encoding/json"
	"time"

	"golang.org/x/net/websocket"
)

// EventScenes is EventSubscription::Scenes.
const EventScenes = 1 << 2

// Watch opens its own connection to OBS, subscribed to scene events only,
// and calls scene with the program scene's name: once when connected and
// again on every CurrentProgramSceneChanged. It returns when the
// connection fails or stop closes (then nil); the caller reconnects.
func Watch(cfg Config, stop <-chan struct{}, scene func(name string)) error {
	conn, err := dial(cfg, EventScenes)
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Time{})
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-stop:
		case <-done:
		}
		conn.Close()
	}()
	err = send(conn, 6, map[string]any{"requestType": "GetCurrentProgramScene", "requestId": "watch"})
	for err == nil {
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
			scene(name)
		case m.Op == 5 && d.Type == "CurrentProgramSceneChanged":
			scene(d.Event.Name)
		}
	}
	select {
	case <-stop:
		return nil
	default:
		return err
	}
}
