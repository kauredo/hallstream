package web

import (
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/kauredo/hallstream/internal/i18n"
)

var noReader error = i18n.Key("api.ble.sem_leitor")

func (s *Server) bleRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/ble/devices", func(w http.ResponseWriter, r *http.Request) {
		if s.BLE == nil {
			reply(w, []any{})
			return
		}
		reply(w, s.BLE.Devices())
	})
	m.HandleFunc("POST /api/ble/try", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address string `json:"address"`
		}
		if !decode(w, r, &req) {
			return
		}
		req.Address = strings.TrimSpace(req.Address)
		if req.Address == "" {
			fail(w, http.StatusBadRequest, i18n.T("api.ble.falta_endereco"))
			return
		}
		if s.BLE == nil {
			fail(w, http.StatusServiceUnavailable, noReader.Error())
			return
		}
		if err := s.BLE.Try(req.Address); err != nil {
			reply(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		ok(w)
	})
	m.HandleFunc("POST /api/ble/forget", func(w http.ResponseWriter, r *http.Request) {
		if s.BLE == nil {
			fail(w, http.StatusServiceUnavailable, noReader.Error())
			return
		}
		if err := s.BLE.Forget(); err != nil {
			fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		ok(w)
	})
	m.HandleFunc("POST /api/system/bluetooth-settings", func(w http.ResponseWriter, r *http.Request) {
		if err := openBluetoothSettings(); err != nil {
			fail(w, http.StatusNotImplemented, err.Error())
			return
		}
		ok(w)
	})
}

func openBluetoothSettings() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", "ms-settings:bluetooth")
	case "darwin":
		cmd = exec.Command("open", "x-apple.systempreferences:com.apple.BluetoothSettings")
	default:
		return i18n.Err("api.ble.abra_definicoes")
	}
	if err := cmd.Start(); err != nil {
		return i18n.Err("api.ble.nao_abri_definicoes", "err", err)
	}
	go cmd.Wait()
	return nil
}
