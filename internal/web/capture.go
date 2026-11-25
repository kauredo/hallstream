package web

import (
	"net/http"
	"path/filepath"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/i18n"
)

// captureDir is where recordings of the console go, inside dados/.
func (s *Server) captureDir() string { return filepath.Join(s.Store.Dir, "captura") }

func (s *Server) captureRoutes() {
	m := s.mux
	recorder := func(w http.ResponseWriter) *ble.Recorder {
		if s.BLE == nil || s.BLE.Capture == nil {
			fail(w, http.StatusServiceUnavailable, noReader.Error())
			return nil
		}
		return s.BLE.Capture
	}
	m.HandleFunc("GET /api/ble/capture", func(w http.ResponseWriter, r *http.Request) {
		if s.BLE == nil || s.BLE.Capture == nil { // demo mode: nothing to record
			reply(w, ble.CaptureStatus{Max: ble.DefaultCaptureMax})
			return
		}
		reply(w, s.BLE.Capture.Status())
	})
	m.HandleFunc("POST /api/ble/capture", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &req) {
			return
		}
		rec := recorder(w)
		if rec == nil {
			return
		}
		if !req.On {
			reply(w, rec.Stop("off"))
			return
		}
		linked, name := s.BLE.Linked()
		if !linked {
			fail(w, http.StatusConflict, i18n.T("api.captura.sem_caixa"))
			return
		}
		st, err := rec.Start(name, s.once)
		if err != nil {
			fail(w, http.StatusInternalServerError, i18n.T("api.captura.nao_gravei", "err", err))
			return
		}
		reply(w, st)
	})
	m.HandleFunc("POST /api/system/capture-folder", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.RemoteAddr) {
			fail(w, http.StatusForbidden, i18n.T("api.captura.so_no_pc"))
			return
		}
		if s.OpenFolder == nil {
			fail(w, http.StatusNotImplemented, i18n.T("api.captura.sem_pasta"))
			return
		}
		if err := s.OpenFolder(s.captureDir()); err != nil {
			fail(w, http.StatusInternalServerError, i18n.T("api.captura.nao_abri_pasta", "err", err))
			return
		}
		ok(w)
	})
}
