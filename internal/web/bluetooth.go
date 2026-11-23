package web

import (
	"errors"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

const noReader = "O Bluetooth não está a correr (modo demo)."

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
			fail(w, http.StatusBadRequest, "Falta o endereço do dispositivo.")
			return
		}
		if s.BLE == nil {
			fail(w, http.StatusServiceUnavailable, noReader)
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
			fail(w, http.StatusServiceUnavailable, noReader)
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
		return errors.New("Abra as definições de Bluetooth do computador à mão.")
	}
	if err := cmd.Start(); err != nil {
		return errors.New("Não consegui abrir as definições de Bluetooth: " + err.Error())
	}
	go cmd.Wait()
	return nil
}
