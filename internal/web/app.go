package web

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kauredo/tribuna/internal/store"
)

const appID = "tribuna"

func (s *Server) appRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"app": appID, "version": s.Version})
	})
	typed := func(file, ctype string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, s.Files, file)
		}
	}
	m.HandleFunc("GET /favicon.ico", typed("assets/icon/tribuna.ico", "image/x-icon"))
	m.HandleFunc("GET /painel/manifest.webmanifest", typed("painel/manifest.webmanifest", "application/manifest+json"))
	m.HandleFunc("POST /api/system/desktop-shortcut", s.desktopShortcut)
	m.HandleFunc("GET /api/system/backup", s.backup)
}

// backup is "Guardar cópia": the dados folder as a zip, to unzip beside the
// program on another PC. raw.log is left out, as it grows all season.
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	// Another site can't read the zip, but could still drop one in Downloads.
	if site := r.Header.Get("Sec-Fetch-Site"); !isLoopback(r.RemoteAddr) || (site != "" && site != "same-origin" && site != "none") {
		fail(w, http.StatusForbidden, "A cópia só se guarda no próprio computador.")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="Tribuna-copia-`+time.Now().Format("2006-01-02")+`.zip"`)
	zw := zip.NewWriter(w)
	err := filepath.WalkDir(s.Store.Dir, func(path string, d fs.DirEntry, err error) error {
		// The program may delete a file (a sent game log, a removed video)
		// between the listing and the copy.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if strings.HasPrefix(name, "raw.log") || strings.HasPrefix(name, ".tmp-") {
			return nil
		}
		rel, err := filepath.Rel(s.Store.Dir, path)
		if err != nil {
			return err
		}
		h := &zip.FileHeader{Name: "dados/" + filepath.ToSlash(rel), Method: zip.Deflate, Modified: time.Now()}
		if info, err := d.Info(); err == nil {
			h.Modified = info.ModTime()
		}
		// Videos and pictures are compressed already.
		switch strings.ToLower(filepath.Ext(name)) {
		case ".mp4", ".png", ".jpg", ".jpeg", ".webp":
			h.Method = zip.Store
		}
		f, err := os.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer f.Close()
		out, err := zw.CreateHeader(h)
		if err == nil {
			_, err = io.Copy(out, f)
		}
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		s.Log("Não consegui guardar a cópia dos dados: " + err.Error())
		// The 200 is sent: cutting the connection makes the browser show a
		// failed download instead of a broken zip.
		panic(http.ErrAbortHandler)
	}
}

// desktopShortcut answers the panel's shortcut question: create it, or
// remember that the volunteer said no.
func (s *Server) desktopShortcut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Create bool `json:"create"`
	}
	if !decode(w, r, &req) {
		return
	}
	answer, path := "declined", ""
	if req.Create {
		if !isLoopback(r.RemoteAddr) {
			fail(w, http.StatusForbidden, "O atalho só se cria no próprio computador.")
			return
		}
		if s.Shortcut == nil {
			fail(w, http.StatusNotImplemented, "O atalho no ambiente de trabalho só se cria no Windows.")
			return
		}
		var err error
		if path, err = s.Shortcut(); err != nil {
			s.Log("Não consegui criar o atalho: " + err.Error())
			fail(w, http.StatusInternalServerError, "Não consegui criar o atalho: "+err.Error())
			return
		}
		answer = "created"
	}
	_, err := s.Store.UpdateSettings(func(set *store.Settings) { set.DesktopShortcut = answer })
	if !s.save(w, err) {
		return
	}
	reply(w, map[string]any{"ok": true, "path": path})
}

// Ours reports whether another Tribuna answers at addr (host:port).
func Ours(addr string) bool {
	c := &http.Client{
		Timeout:       500 * time.Millisecond, // loopback: a real answer takes a few ms
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := c.Get("http://" + addr + "/api/hello")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var hello struct {
		App string `json:"app"`
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&hello) == nil && hello.App == appID
}
