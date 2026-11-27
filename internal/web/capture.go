package web

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/kauredo/hallstream/internal/ble"
	"github.com/kauredo/hallstream/internal/i18n"
)

// captureDir is where recordings of the console go, inside dados/.
func (s *Server) captureDir() string { return filepath.Join(s.Store.Dir, "captura") }

// captureLink is the link Gravar a consola records: the cable while it is
// the input, else the Bluetooth box. rec is nil when there is nothing to
// record (demo mode); noLink is the refusal while it is not linked.
func (s *Server) captureLink() (rec *ble.Recorder, linked func() (bool, string), noLink string) {
	if s.Serial != nil && s.State.Snapshot().Input == "serial" {
		return s.Serial.Capture, s.Serial.Linked, i18n.T("api.captura.sem_cabo")
	}
	if s.BLE == nil || s.BLE.Capture == nil {
		return nil, nil, ""
	}
	return s.BLE.Capture, s.BLE.Linked, i18n.T("api.captura.sem_caixa")
}

func (s *Server) captureRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/ble/capture", func(w http.ResponseWriter, r *http.Request) {
		if rec, _, _ := s.captureLink(); rec != nil {
			reply(w, rec.Status())
			return
		}
		reply(w, ble.CaptureStatus{Max: ble.DefaultCaptureMax}) // demo mode: nothing to record
	})
	m.HandleFunc("POST /api/ble/capture", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			On bool `json:"on"`
		}
		if !decode(w, r, &req) {
			return
		}
		rec, linkedNow, noLink := s.captureLink()
		if rec == nil {
			fail(w, http.StatusServiceUnavailable, noReader.Error())
			return
		}
		if !req.On {
			reply(w, rec.Stop("off"))
			return
		}
		linked, name := linkedNow()
		if !linked {
			fail(w, http.StatusConflict, noLink)
			return
		}
		st, err := rec.Start(name, s.once)
		if errors.Is(err, ble.ErrFolderFull) {
			fail(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			// The disk's message names folders: the log gets it, the panel does not.
			s.Log(i18n.T("log.captura.nao_gravei", "err", err))
			fail(w, http.StatusInternalServerError, i18n.T("api.captura.nao_gravei"))
			return
		}
		// The link may have dropped while the file was being made.
		if linked, _ := linkedNow(); !linked {
			rec.Stop("box")
			fail(w, http.StatusConflict, noLink)
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
			s.Log(i18n.T("log.captura.nao_abri_pasta", "err", err))
			fail(w, http.StatusInternalServerError, i18n.T("api.captura.nao_abri_pasta"))
			return
		}
		ok(w)
	})
}
