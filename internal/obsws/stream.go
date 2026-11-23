package obsws

import "errors"

// Stream is OBS's stream output.
type Stream struct {
	Active       bool  `json:"active"`
	Reconnecting bool  `json:"reconnecting"`
	DurationMS   int64 `json:"duration_ms"`
}

// The obs-websocket statuses for an output already running or not running.
const (
	OutputRunning    = 500
	OutputNotRunning = 501
)

// ErrNoStreamKey: OBS has no stream key, so it can't go live.
var ErrNoStreamKey = errors.New("Configure a conta do YouTube no OBS (Definições > Transmissão) uma vez.")

// ErrStreaming: OBS doesn't take new stream settings while live.
var ErrStreaming = errors.New("o OBS está a transmitir")

func (c *Client) StreamStatus(cfg Config) (Stream, error) {
	var r struct {
		Active       bool    `json:"outputActive"`
		Reconnecting bool    `json:"outputReconnecting"`
		Duration     float64 `json:"outputDuration"`
	}
	if err := c.Request(cfg, "GetStreamStatus", nil, &r); err != nil {
		return Stream{}, err
	}
	return Stream{r.Active, r.Reconnecting, int64(r.Duration)}, nil
}

// StartStream goes live, after checking OBS has somewhere to send it.
func (c *Client) StartStream(cfg Config) error {
	var r struct {
		Type     string `json:"streamServiceType"`
		Settings struct {
			Key string `json:"key"`
		} `json:"streamServiceSettings"`
	}
	if err := c.Request(cfg, "GetStreamServiceSettings", nil, &r); err != nil {
		return err
	}
	// A YouTube account linked in OBS may keep no key here, so only a
	// custom server must have one.
	if r.Type == "rtmp_custom" && r.Settings.Key == "" {
		return ErrNoStreamKey
	}
	err := c.Request(cfg, "StartStream", nil, nil)
	re, ok := err.(*RequestError)
	switch {
	case ok && re.Code == OutputRunning:
		return nil
	case ok:
		return errors.New("O OBS não começou a transmissão (" + re.Comment + "). Confirme a conta do YouTube no OBS, em Definições > Transmissão.")
	}
	return err
}

// SetCustomService points OBS at server with key (a custom RTMP service),
// unless it already is. OBS refuses the change while streaming, so it waits
// for the next stop. It reports whether it changed.
func (c *Client) SetCustomService(cfg Config, server, key string) (bool, error) {
	var r struct {
		Type     string `json:"streamServiceType"`
		Settings struct {
			Server string `json:"server"`
			Key    string `json:"key"`
		} `json:"streamServiceSettings"`
	}
	if err := c.Request(cfg, "GetStreamServiceSettings", nil, &r); err != nil {
		return false, err
	}
	if r.Type == "rtmp_custom" && r.Settings.Server == server && r.Settings.Key == key {
		return false, nil
	}
	if st, err := c.StreamStatus(cfg); err != nil || st.Active {
		if err == nil {
			err = ErrStreaming
		}
		return false, err
	}
	err := c.Request(cfg, "SetStreamServiceSettings", map[string]any{
		"streamServiceType":     "rtmp_custom",
		"streamServiceSettings": map[string]any{"server": server, "key": key},
	}, nil)
	return err == nil, err
}

func (c *Client) StopStream(cfg Config) error {
	err := c.Request(cfg, "StopStream", nil, nil)
	if re, ok := err.(*RequestError); ok && re.Code == OutputNotRunning {
		return nil
	}
	return err
}
