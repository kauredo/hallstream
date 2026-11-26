package obsapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func readKeys(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEnableNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obs-websocket", "config.json")
	ws, err := ReadWebSocket(path)
	if err != nil || ws.Enabled || ws.Port != DefaultPort {
		t.Fatalf("defaults %+v %v", ws, err)
	}
	ws, err = EnableWebSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Enabled || !ws.Auth || ws.Port != 4455 || len(ws.Password) != 24 {
		t.Fatalf("got %+v", ws)
	}
	m := readKeys(t, path)
	if m["server_enabled"] != true || m["auth_required"] != true || m["server_port"] != 4455.0 ||
		m["server_password"] != ws.Password || m["first_load"] != false {
		t.Errorf("file %v", m)
	}
	if got, _ := ReadWebSocket(path); got != ws {
		t.Errorf("read back %+v", got)
	}
}

func TestEnableKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"alerts_enabled": true, "first_load": false, "server_enabled": false,
		"server_port": 4460, "auth_required": false, "server_password": "", "future_key": {"a": [1, 2]}}`), 0o644)
	ws, err := EnableWebSocket(path)
	if err != nil {
		t.Fatal(err)
	}
	m := readKeys(t, path)
	if m["alerts_enabled"] != true || m["future_key"].(map[string]any)["a"].([]any)[1] != 2.0 {
		t.Errorf("lost keys: %v", m)
	}
	if m["server_port"] != 4455.0 || ws.Port != 4455 || m["auth_required"] != true || ws.Password == "" {
		t.Errorf("file %v, ws %+v", m, ws)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("left a temporary file: %v", entries)
	}
}

func TestEnableKeepsPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"server_enabled": false, "server_password": "abc123"}`), 0o644)
	ws, err := EnableWebSocket(path)
	if err != nil || ws.Password != "abc123" {
		t.Fatalf("%+v %v", ws, err)
	}
}

func TestEnableBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"server_enabled": fal`), 0o644)
	if _, err := EnableWebSocket(path); err == nil {
		t.Fatal("want an error")
	}
	if b, _ := os.ReadFile(path); string(b) != `{"server_enabled": fal` {
		t.Errorf("rewrote a file it could not read: %s", b)
	}
}

func TestPassword(t *testing.T) {
	a, _ := Password()
	b, _ := Password()
	if a == b || len(a) != 24 {
		t.Errorf("%q %q", a, b)
	}
}

func TestSystem(t *testing.T) {
	app := System()
	if app.ConfigPath == "" || filepath.Base(app.ConfigPath) != "config.json" {
		t.Errorf("config path %q", app.ConfigPath)
	}
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if app.Running == nil || app.Start == nil {
			t.Error("this system finds and starts OBS")
		}
	default:
		if app.Running != nil || app.Start != nil || app.Close != nil {
			t.Error("only Windows, macOS and Linux start OBS")
		}
	}
}
