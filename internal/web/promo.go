package web

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"rsc.io/qr"

	"github.com/kauredo/tribuna/internal/state"
	"github.com/kauredo/tribuna/internal/store"
)

// promoMax is the largest video the panel can upload and promoTotal the
// whole library, so uploads can't fill the club PC's disk (variables for tests).
var promoMax, promoTotal int64 = 200 << 20, 2 << 30

// promoUpload lets one upload run at a time.
var promoUpload = make(chan struct{}, 1)

// The default text of a new clip: the club's first use is selling the kit.
const promoKicker, promoHeadline, promoLine = "LOJA DO CLUBE", "Equipamento oficial", "Disponível na receção do clube"

// promoFile is the only shape a stored video name has.
var promoFile = regexp.MustCompile(`^[0-9a-f]{8}\.mp4$`)

func (s *Server) promoRoutes() {
	m := s.mux
	m.HandleFunc("GET /promo/{file}", s.promoVideo)
	m.HandleFunc("GET /promo/{id}/qr.svg", s.promoQR)
	m.HandleFunc("GET /api/promo", func(w http.ResponseWriter, r *http.Request) { reply(w, s.Store.Promo()) })
	m.HandleFunc("PUT /api/promo", s.putPromoLibrary)
	m.HandleFunc("POST /api/promo", s.addPromo)
	m.HandleFunc("PUT /api/promo/{id}", s.putPromo)
	m.HandleFunc("DELETE /api/promo/{id}", s.deletePromo)
	m.HandleFunc("POST /api/promo/{id}/show", s.showPromo)
	m.HandleFunc("POST /api/promo/stop", func(w http.ResponseWriter, r *http.Request) {
		s.State.EndPromo(0)
		ok(w)
	})
	m.HandleFunc("POST /api/promo/ended", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Seq int `json:"seq"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Seq > 0 {
			s.State.EndPromo(req.Seq)
		}
		ok(w)
	})
}

func findClip(p store.Promo, id string) *store.Clip {
	for i := range p.Clips {
		if p.Clips[i].ID == id {
			return &p.Clips[i]
		}
	}
	return nil
}

// addPromo streams one MP4 from the multipart field "file" into dados/promo.
func (s *Server) addPromo(w http.ResponseWriter, r *http.Request) {
	select {
	case promoUpload <- struct{}{}:
		defer func() { <-promoUpload }()
	default:
		fail(w, http.StatusConflict, "Já está a carregar um vídeo. Espere que acabe.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, promoMax+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		fail(w, http.StatusBadRequest, "Pedido inválido: "+err.Error())
		return
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			fail(w, http.StatusBadRequest, "Escolha um vídeo MP4.")
			return
		}
		if err != nil {
			s.promoUploadFail(w, err)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		clip, err := s.savePromo(part)
		part.Close()
		if err != nil {
			s.promoUploadFail(w, err)
			return
		}
		if _, err := s.Store.UpdatePromo(func(p *store.Promo) error {
			total := clip.Size
			for _, c := range p.Clips {
				total += c.Size
			}
			if total > promoTotal {
				return userError("Já há muitos vídeos (máximo 2 GB no total). Apague um antes de carregar outro.")
			}
			p.Clips = append(p.Clips, clip)
			return nil
		}); err != nil {
			s.Store.RemovePromoFile(clip.File)
			s.save(w, err)
			return
		}
		replyCode(w, http.StatusCreated, clip)
		return
	}
}

var errTooBig = userError("O vídeo passa dos 200 MB.")

func (s *Server) promoUploadFail(w http.ResponseWriter, err error) {
	var ue userError
	var mb *http.MaxBytesError
	switch {
	case errors.As(err, &mb) || errors.Is(err, errTooBig):
		fail(w, http.StatusRequestEntityTooLarge, errTooBig.Error())
	case errors.As(err, &ue):
		fail(w, http.StatusBadRequest, ue.Error())
	default:
		s.Log("Promo: " + err.Error())
		fail(w, http.StatusInternalServerError, "Não consegui guardar o vídeo. Veja se o disco tem espaço.")
	}
}

// savePromo writes the part to a temp file in dados/promo, checking the MP4
// signature first and the size as it goes, then names it <random id>.mp4.
func (s *Server) savePromo(part *multipart.Part) (store.Clip, error) {
	name := part.FileName()
	if !strings.EqualFold(filepath.Ext(name), ".mp4") {
		return store.Clip{}, userError("O vídeo tem de ser MP4.")
	}
	head := make([]byte, 64)
	n, err := io.ReadFull(part, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		if err == io.EOF {
			return store.Clip{}, userError("O ficheiro está vazio.")
		}
		return store.Clip{}, err
	}
	head = head[:n]
	if !isMP4(head) {
		return store.Clip{}, userError("Este ficheiro não é um vídeo MP4.")
	}
	dir := s.Store.PromoDir()
	tmp, err := os.CreateTemp(dir, ".up-*")
	if err != nil {
		return store.Clip{}, err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	size, err := io.Copy(tmp, io.MultiReader(bytes.NewReader(head), io.LimitReader(part, promoMax-int64(n)+1)))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && size > promoMax {
		err = errTooBig
	}
	if err != nil {
		return store.Clip{}, err
	}
	id := store.NewID()
	for exists(filepath.Join(dir, id+".mp4")) {
		id = store.NewID()
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, id+".mp4")); err != nil {
		return store.Clip{}, err
	}
	return store.Clip{
		ID: id, Name: clipName(name), File: id + ".mp4", Size: size,
		Kicker: promoKicker, Headline: promoHeadline, Line: promoLine,
		Volume: 60, End: "back", Added: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// mp4Brands are the ftyp brands of MP4 files (QuickTime's "qt  " is not one).
var mp4Brands = map[string]bool{
	"isom": true, "iso2": true, "iso3": true, "iso4": true, "iso5": true, "iso6": true,
	"mp41": true, "mp42": true, "avc1": true, "M4V ": true, "M4VH": true, "M4VP": true,
	"dash": true, "mmp4": true, "MSNV": true, "f4v ": true,
}

// isMP4 checks the file starts with an ftyp box whose major or a compatible brand is MP4.
func isMP4(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}
	size := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if size < 16 || size%4 != 0 {
		return false
	}
	if mp4Brands[string(b[8:12])] {
		return true
	}
	for i := 16; i+4 <= min(size, len(b)); i += 4 {
		if mp4Brands[string(b[i:i+4])] {
			return true
		}
	}
	return false
}

// clipName is the upload's file name without folders, extension or control characters.
func clipName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, name)
	if name = strings.TrimSpace(cutRunes(name, 60)); name == "" || name == "." {
		return "Vídeo"
	}
	return name
}

// promoLink tidies the link typed in the panel: "" stays "", a link without
// a scheme gets https://, and anything but an http(s) address is refused.
func promoLink(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		len(s) > 300 || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", userError("O link tem de ser um endereço da internet, por exemplo https://loja.exemplo.pt.")
	}
	return s, nil
}

func (s *Server) putPromo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cur := findClip(s.Store.Promo(), id)
	if cur == nil {
		fail(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	req := *cur
	if !decode(w, r, &req) {
		return
	}
	link, err := promoLink(req.Link)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	var out store.Clip
	_, err = s.Store.UpdatePromo(func(p *store.Promo) error {
		c := findClip(*p, id)
		if c == nil {
			return store.ErrNotFound
		}
		// The file and its facts are the server's; the panel owns the rest.
		c.Name = strings.TrimSpace(cutRunes(req.Name, 60))
		if c.Name == "" {
			c.Name = "Vídeo"
		}
		c.Kicker = strings.TrimSpace(cutRunes(req.Kicker, 40))
		c.Headline = strings.TrimSpace(cutRunes(req.Headline, 60))
		c.Line = strings.TrimSpace(cutRunes(req.Line, 120))
		c.Link, c.ShowQR = link, req.ShowQR && link != ""
		c.Sound, c.Volume = req.Sound, min(max(req.Volume, 0), 100)
		c.End = "back"
		if req.End == "loop" {
			c.End = "loop"
		}
		if req.Orientation == "vertical" || req.Orientation == "horizontal" {
			c.Orientation = req.Orientation
		}
		if req.Width > 0 && req.Width <= 16384 && req.Height > 0 && req.Height <= 16384 {
			c.Width, c.Height = req.Width, req.Height
		}
		if req.Duration > 0 && req.Duration < 24*3600 {
			c.Duration = req.Duration
		}
		out = *c
		return nil
	})
	if s.save(w, err) {
		reply(w, out)
	}
}

func (s *Server) putPromoLibrary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TimeoutClip string `json:"timeout_clip"`
	}
	if !decode(w, r, &req) {
		return
	}
	p, err := s.Store.UpdatePromo(func(p *store.Promo) error {
		if req.TimeoutClip != "" {
			c := findClip(*p, req.TimeoutClip)
			if c == nil {
				return store.ErrNotFound
			}
			if c.Orientation != "vertical" {
				return userError("No desconto de tempo só cabem vídeos na vertical.")
			}
		}
		p.TimeoutClip = req.TimeoutClip
		return nil
	})
	if s.save(w, err) {
		reply(w, p)
	}
}

func (s *Server) deletePromo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var file string
	_, err := s.Store.UpdatePromo(func(p *store.Promo) error {
		for i, c := range p.Clips {
			if c.ID == id {
				file = c.File
				p.Clips = append(p.Clips[:i], p.Clips[i+1:]...)
				if p.TimeoutClip == id {
					p.TimeoutClip = ""
				}
				return nil
			}
		}
		return store.ErrNotFound
	})
	if !s.save(w, err) {
		return
	}
	s.SyncPromo() // stop it before the file goes
	s.Store.RemovePromoFile(file)
	ok(w)
}

func (s *Server) showPromo(w http.ResponseWriter, r *http.Request) {
	c := findClip(s.Store.Promo(), r.PathValue("id"))
	if c == nil {
		fail(w, http.StatusNotFound, "Vídeo não encontrado.")
		return
	}
	s.State.ShowPromo(promoClip(*c))
	ok(w)
}

// promoClip is a library clip as the live state carries it.
func promoClip(c store.Clip) state.PromoClip {
	out := state.PromoClip{
		ID: c.ID, URL: "/promo/" + c.File, Orientation: c.Orientation,
		Kicker: c.Kicker, Headline: c.Headline, Line: c.Line, Link: c.Link,
		Sound: c.Sound, Volume: c.Volume, End: c.End, Duration: c.Duration,
	}
	if c.ShowQR && c.Link != "" {
		// The link's checksum in the address makes the overlay load a new code when it changes.
		out.QRURL = fmt.Sprintf("/promo/%s/qr.svg?v=%08x", c.ID, crc32.ChecksumIEEE([]byte(c.Link)))
	}
	return out
}

// SyncPromo brings the live state's clips up to date with the library.
func (s *Server) SyncPromo() {
	// One at a time, so an older read of the library never lands after a newer one.
	s.promoMu.Lock()
	defer s.promoMu.Unlock()
	p := s.Store.Promo()
	var mini *state.PromoClip
	if c := findClip(p, p.TimeoutClip); c != nil && c.Orientation == "vertical" {
		v := promoClip(*c)
		mini = &v
	}
	s.State.SyncPromo(func(id string) (state.PromoClip, bool) {
		c := findClip(p, id)
		if c == nil {
			return state.PromoClip{}, false
		}
		return promoClip(*c), true
	}, mini)
}

// promoVideo serves a clip with Range support, so the video element can seek and stream.
func (s *Server) promoVideo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	known := false
	for _, c := range s.Store.Promo().Clips {
		known = known || c.File == name
	}
	if !promoFile.MatchString(name) || !known {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.Store.PromoDir(), name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// promoQR draws the clip's link as an SVG QR code with the standard 4-module margin.
func (s *Server) promoQR(w http.ResponseWriter, r *http.Request) {
	c := findClip(s.Store.Promo(), r.PathValue("id"))
	if c == nil || c.Link == "" {
		http.NotFound(w, r)
		return
	}
	code, err := qr.Encode(c.Link, qr.M)
	if err != nil {
		fail(w, http.StatusInternalServerError, "Não consegui fazer o código QR: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Security-Policy", "script-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(qrSVG(code))
}

func qrSVG(code *qr.Code) []byte {
	n := code.Size + 8
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#14060A" d="`, n, n, n, n)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+4, y+4)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.Bytes()
}
