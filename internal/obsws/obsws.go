// Package obsws is a minimal obs-websocket v5 client: connect, identify,
// switch the program scene and move the camera for the time-out. OBS 28+ has
// the server built in.
package obsws

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type Config struct {
	Host     string
	Port     int
	Password string
}

type Client struct {
	// OnConnect runs, in its own goroutine, after each new connection.
	OnConnect func()

	mu   sync.Mutex
	conn *websocket.Conn
	cfg  Config
	err  string
	next int
}

type message struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

// Auth computes the obs-websocket v5 authentication string.
func Auth(password, salt, challenge string) string {
	h := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(h[:])
	h = sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(h[:])
}

func dial(cfg Config) (*websocket.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	wc, err := websocket.NewConfig("ws://"+addr, "http://localhost/")
	if err != nil {
		return nil, err
	}
	wc.Protocol = []string{"obswebsocket.json"}
	wc.Dialer = &net.Dialer{Timeout: 3 * time.Second}
	conn, err := websocket.DialConfig(wc)
	if err != nil {
		return nil, errors.New("o OBS não responde em " + addr + " (o servidor WebSocket está ligado?)")
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var hello struct {
		Authentication *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	if err := receive(conn, 0, &hello); err != nil {
		conn.Close()
		return nil, err
	}
	id := map[string]any{"rpcVersion": 1, "eventSubscriptions": 0}
	if a := hello.Authentication; a != nil {
		id["authentication"] = Auth(cfg.Password, a.Salt, a.Challenge)
	}
	if err := send(conn, 1, id); err != nil {
		conn.Close()
		return nil, err
	}
	if err := receive(conn, 2, nil); err != nil {
		conn.Close()
		return nil, errors.New("o OBS recusou a ligação (palavra-passe errada?)")
	}
	return conn, nil
}

func send(conn *websocket.Conn, op int, d any) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return websocket.JSON.Send(conn, message{Op: op, D: b})
}

// receive reads messages until one with op arrives.
func receive(conn *websocket.Conn, op int, into any) error {
	for {
		var m message
		if err := websocket.JSON.Receive(conn, &m); err != nil {
			return err
		}
		if m.Op != op {
			continue
		}
		if into == nil {
			return nil
		}
		return json.Unmarshal(m.D, into)
	}
}

// Test opens one connection with cfg and closes it.
func Test(cfg Config) error {
	conn, err := dial(cfg)
	if err != nil {
		return err
	}
	return conn.Close()
}

// SetScene switches the OBS program scene, connecting first if needed.
func (c *Client) SetScene(cfg Config, scene string) error {
	err := c.Request(cfg, "SetCurrentProgramScene", map[string]string{"sceneName": scene}, nil)
	if re, ok := err.(*RequestError); ok {
		return fmt.Errorf("o OBS não mudou para a cena %q: %s", scene, re.Comment)
	}
	return err
}

// RequestError is a request OBS answered with a failure status.
type RequestError struct {
	Type    string
	Code    int
	Comment string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("o OBS recusou %s: %s", e.Type, e.Comment)
}

// NotFound is the obs-websocket status for a missing scene, source or item.
const NotFound = 600

// Request sends one request and decodes its responseData into into (when
// not nil), connecting first if needed.
func (c *Client) Request(cfg Config, typ string, data, into any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	sent, err := c.request(cfg, typ, data, into)
	if _, status := err.(*RequestError); err != nil && !status && c.conn != nil {
		// The connection may have gone stale: drop it, and try once more on
		// a new one unless OBS may have done it already.
		c.conn.Close()
		c.conn = nil
		if !sent || again(typ) {
			_, err = c.request(cfg, typ, data, into)
		}
	}
	c.err = ""
	if err != nil {
		c.err = err.Error()
	}
	return err
}

// again: the request gives the same result when sent twice.
func again(typ string) bool {
	return strings.HasPrefix(typ, "Get") || typ == "SetSceneItemTransform" || typ == "SetCurrentProgramScene"
}

// request reports whether the request went out, so a request that fails
// after that is not sent twice.
func (c *Client) request(cfg Config, typ string, data, into any) (bool, error) {
	if c.conn != nil && c.cfg != cfg {
		c.conn.Close()
		c.conn = nil
	}
	if c.conn == nil {
		conn, err := dial(cfg)
		if err != nil {
			return false, err
		}
		c.conn, c.cfg = conn, cfg
		if c.OnConnect != nil {
			go c.OnConnect()
		}
	}
	c.next++
	reqID := strconv.Itoa(c.next)
	// Adding a camera or changing the canvas can take OBS a while.
	wait := 5 * time.Second
	if !again(typ) {
		wait = 20 * time.Second
	}
	c.conn.SetDeadline(time.Now().Add(wait))
	req := map[string]any{"requestType": typ, "requestId": reqID}
	if data != nil {
		req["requestData"] = data
	}
	if err := send(c.conn, 6, req); err != nil {
		return false, err
	}
	for {
		var resp struct {
			RequestID     string `json:"requestId"`
			RequestStatus struct {
				Result  bool   `json:"result"`
				Code    int    `json:"code"`
				Comment string `json:"comment"`
			} `json:"requestStatus"`
			ResponseData json.RawMessage `json:"responseData"`
		}
		if err := receive(c.conn, 7, &resp); err != nil {
			return true, err
		}
		if resp.RequestID != reqID {
			continue
		}
		if !resp.RequestStatus.Result {
			return true, &RequestError{typ, resp.RequestStatus.Code, resp.RequestStatus.Comment}
		}
		if into == nil || len(resp.ResponseData) == 0 {
			return true, nil
		}
		return true, json.Unmarshal(resp.ResponseData, into)
	}
}

// Status reports whether a connection is open and the last error.
func (c *Client) Status() (connected bool, err string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil, c.err
}
