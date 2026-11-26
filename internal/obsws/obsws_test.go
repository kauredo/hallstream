package obsws

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/net/websocket"
)

// fake is the OBS side of the tests: one program scene with some items.
type fake struct {
	mu      sync.Mutex
	scenes  []string
	program string
	items   []map[string]any            // as GetSceneItemList sends them
	byScene map[string][]map[string]any // each scene's items instead, when set
	canvasW int
	canvasH int
	sets    []map[string]any // sceneItemTransform of each SetSceneItemTransform
	asked   []string         // request types in order
	setIn   []string         // the sceneName of each SetSceneItemTransform
	conns   []*websocket.Conn
	subs    []int // each connection's eventSubscriptions
}

func (f *fake) joined(ws *websocket.Conn, events int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conns = append(f.conns, ws)
	f.subs = append(f.subs, events)
}

// emit sends an event to every connection.
func (f *fake) emit(typ string, data map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ws := range f.conns {
		send(ws, 5, map[string]any{"eventType": typ, "eventIntent": EventScenes, "eventData": data})
	}
}

func (f *fake) answer(typ string, d map[string]any) (bool, int, any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, typ)
	switch typ {
	case "SetCurrentProgramScene":
		f.program, _ = d["sceneName"].(string)
		return true, 100, nil
	case "GetSceneList":
		var list []map[string]string
		for _, s := range f.scenes {
			list = append(list, map[string]string{"sceneName": s})
		}
		return true, 100, map[string]any{"scenes": list}
	case "GetCurrentProgramScene":
		return true, 100, map[string]any{"sceneName": f.program, "currentProgramSceneName": f.program}
	case "GetVideoSettings":
		return true, 100, map[string]any{"baseWidth": f.canvasW, "baseHeight": f.canvasH}
	case "GetSceneItemList":
		if f.byScene != nil {
			return true, 100, map[string]any{"sceneItems": f.byScene[d["sceneName"].(string)]}
		}
		return true, 100, map[string]any{"sceneItems": f.items}
	case "SetSceneItemTransform":
		id := int(d["sceneItemId"].(float64))
		tr := d["sceneItemTransform"].(map[string]any)
		scene, _ := d["sceneName"].(string)
		f.setIn = append(f.setIn, scene)
		items := f.items
		if f.byScene != nil {
			items = f.byScene[scene]
		}
		for _, it := range items {
			if it["sceneItemId"] == id {
				cur := it["sceneItemTransform"].(map[string]any)
				for k, v := range tr {
					cur[k] = v
				}
				f.sets = append(f.sets, tr)
				return true, 100, nil
			}
		}
		return false, NotFound, nil
	}
	return false, 204, nil
}

func (f *fake) scene() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.program
}

func (f *fake) transforms() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any{}, f.sets...)
}

type answerer interface {
	answer(typ string, d map[string]any) (bool, int, any)
}

// watched is a fake that keeps its connections, to send events on them.
type watched interface {
	joined(ws *websocket.Conn, events int)
}

// fakeOBS asks for auth, checks it, and answers requests from f.
func fakeOBS(t *testing.T, password string, f answerer) Config {
	srv := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		send(ws, 0, map[string]any{"rpcVersion": 1,
			"authentication": map[string]string{"challenge": "ch", "salt": "sa"}})
		var id struct {
			Authentication string `json:"authentication"`
			Events         int    `json:"eventSubscriptions"`
		}
		if receive(ws, 1, &id) != nil || id.Authentication != Auth(password, "sa", "ch") {
			return // OBS closes on bad auth
		}
		if w, ok := f.(watched); ok {
			w.joined(ws, id.Events)
		}
		send(ws, 2, map[string]any{"negotiatedRpcVersion": 1})
		for {
			var req struct {
				RequestType string          `json:"requestType"`
				RequestID   string          `json:"requestId"`
				RequestData json.RawMessage `json:"requestData"`
			}
			if receive(ws, 6, &req) != nil {
				return
			}
			var d map[string]any
			json.Unmarshal(req.RequestData, &d)
			ok, code, data := f.answer(req.RequestType, d)
			if code == hangUp {
				return
			}
			if code == silent {
				continue
			}
			resp := map[string]any{"requestType": req.RequestType, "requestId": req.RequestID,
				"requestStatus": map[string]any{"result": ok, "code": code, "comment": "fake"}}
			if data != nil {
				resp["responseData"] = data
			}
			send(ws, 7, resp)
		}
	}))
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return Config{Host: host, Port: p, Password: password}
}

func TestSetScene(t *testing.T) {
	f := &fake{}
	cfg := fakeOBS(t, "segredo", f)
	var c Client
	if err := c.SetScene(cfg, "Desconto"); err != nil {
		t.Fatal(err)
	}
	if p := f.scene(); p != "Desconto" {
		t.Errorf("program %q", p)
	}
	if err := c.SetScene(cfg, "Jogo"); err != nil {
		t.Fatal(err)
	}
	if p := f.scene(); p != "Jogo" {
		t.Errorf("program %q", p)
	}
	if ok, e := c.Status(); !ok || e != "" {
		t.Errorf("status %v %q", ok, e)
	}
}

func TestWrongPassword(t *testing.T) {
	cfg := fakeOBS(t, "segredo", &fake{})
	cfg.Password = "errada"
	var c Client
	if c.SetScene(cfg, "Jogo") == nil {
		t.Fatal("want an error")
	}
	if ok, e := c.Status(); ok || e == "" {
		t.Errorf("status %v %q", ok, e)
	}
}

func TestRequestError(t *testing.T) {
	cfg := fakeOBS(t, "", &fake{})
	var c Client
	err := c.Request(cfg, "SetSceneItemTransform", map[string]any{"sceneItemId": 9, "sceneItemTransform": map[string]any{}}, nil)
	if re, ok := err.(*RequestError); !ok || re.Code != NotFound {
		t.Fatalf("err %v", err)
	}
	if ok, _ := c.Status(); !ok {
		t.Error("a refused request must keep the connection")
	}
}

// hangUp makes the fake close the connection instead of answering.
const hangUp = -1

// silent makes the fake send no answer and keep the connection.
const silent = -2

type dropFirst struct {
	mu   sync.Mutex
	seen map[string]int
}

func (d *dropFirst) answer(typ string, _ map[string]any) (bool, int, any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[typ]++
	if d.seen[typ] == 1 {
		return false, hangUp, nil
	}
	return true, 100, map[string]any{}
}

func (d *dropFirst) count(typ string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seen[typ]
}

// A read is sent again on a new connection; a write that may have reached
// OBS is not, so it can't create a scene twice.
func TestRetryOnlyReads(t *testing.T) {
	d := &dropFirst{seen: map[string]int{}}
	cfg := fakeOBS(t, "", d)
	var c Client
	if err := c.Request(cfg, "GetSceneList", nil, nil); err != nil {
		t.Errorf("read: %v", err)
	}
	if err := c.Request(cfg, "CreateScene", map[string]string{"sceneName": "X"}, nil); err == nil {
		t.Error("want the write's error")
	}
	if d.count("GetSceneList") != 2 || d.count("CreateScene") != 1 {
		t.Errorf("sent %d reads, %d writes", d.count("GetSceneList"), d.count("CreateScene"))
	}
	if err := c.Request(cfg, "CreateScene", map[string]string{"sceneName": "X"}, nil); err != nil {
		t.Errorf("next request on a new connection: %v", err)
	}
}
