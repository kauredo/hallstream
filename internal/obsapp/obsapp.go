// Package obsapp is OBS Studio as a program on this PC: obs-websocket's
// config file, and on Windows finding, starting and closing obs64.exe.
package obsapp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// App is what the program can do with OBS here. System() gives the real
// one; nil funcs mean this system can't (anything but Windows).
type App struct {
	// ConfigPath is obs-websocket's config.json; "" when unknown.
	ConfigPath string
	// Running reports whether obs64.exe is running.
	Running func() (bool, error)
	// Start launches OBS minimised to the tray.
	Start func() error
	// Close asks OBS to close, the way its window's X would; with force it
	// ends OBS when that does not work.
	Close func(force bool) error
}

// The defaults of obs-websocket 5 (src/Config.h).
const DefaultPort = 4455

// WebSocket is the part of obs-websocket's config.json the program uses.
type WebSocket struct {
	Enabled  bool
	Port     int
	Auth     bool
	Password string
}

// configPath is obs-studio/plugin_config/obs-websocket/config.json in the
// user's config folder (%APPDATA% on Windows).
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "obs-studio", "plugin_config", "obs-websocket", "config.json")
}

// ReadWebSocket reads the server settings, with obs-websocket's defaults for
// what the file lacks. A missing file is the defaults (server off).
func ReadWebSocket(path string) (WebSocket, error) {
	ws := WebSocket{Port: DefaultPort, Auth: true}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ws, nil
	}
	if err != nil {
		return ws, err
	}
	var c struct {
		Enabled  *bool   `json:"server_enabled"`
		Port     *int    `json:"server_port"`
		Auth     *bool   `json:"auth_required"`
		Password *string `json:"server_password"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return ws, errors.New("o ficheiro " + path + " não se lê: " + err.Error())
	}
	if c.Enabled != nil {
		ws.Enabled = *c.Enabled
	}
	if c.Port != nil && *c.Port > 0 && *c.Port < 65536 {
		ws.Port = *c.Port
	}
	if c.Auth != nil {
		ws.Auth = *c.Auth
	}
	if c.Password != nil {
		ws.Password = *c.Password
	}
	return ws, nil
}

// EnableWebSocket turns the server on with the default port and auth on,
// keeping every other key in the file. It keeps a password already there and
// otherwise sets a new random one; it returns the settings now in the file.
// OBS reads the file only when it starts, and rewrites it from what it holds
// when its settings change, so call this only while OBS is closed.
func EnableWebSocket(path string) (WebSocket, error) {
	keys := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return WebSocket{}, err
	case len(bytes.TrimSpace(b)) > 0:
		if err := json.Unmarshal(b, &keys); err != nil {
			// Not ours to repair: OBS would lose whatever is in it.
			return WebSocket{}, errors.New("o ficheiro " + path + " não se lê: " + err.Error())
		}
	}
	cur, _ := ReadWebSocket(path)
	pass := cur.Password
	if pass == "" {
		if pass, err = Password(); err != nil {
			return WebSocket{}, err
		}
	}
	set := func(k string, v any) {
		keys[k], _ = json.Marshal(v)
	}
	set("server_enabled", true)
	set("auth_required", true)
	set("server_password", pass)
	set("server_port", DefaultPort)
	set("first_load", false)
	out, err := json.MarshalIndent(keys, "", "    ")
	if err != nil {
		return WebSocket{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WebSocket{}, err
	}
	if err := writeFile(path, out); err != nil {
		return WebSocket{}, err
	}
	return WebSocket{Enabled: true, Port: DefaultPort, Auth: true, Password: pass}, nil
}

// Password is a new random server password.
func Password() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writeFile replaces path through a temporary file in the same folder, so
// OBS never reads half a file.
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
