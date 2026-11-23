package gamelog

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

// Before is how much of the logs before the stream goes with it.
const Before = 30 * time.Minute

// Every is how often pending batches are tried again.
const Every = 10 * time.Minute

// Path is the server's endpoint, under settings.fpb_base_url.
const Path = "/api/marcador/logs"

// KeepFailed is how many refused batches stay in falhados; MaxAge is how
// long a batch waits before it goes there unsent.
const (
	KeepFailed = 20
	MaxAge     = 60 * 24 * time.Hour
)

// The server's limits: per gzipped file and per request.
var (
	maxFile int64 = 10 << 20
	maxBody int64 = 25 << 20
)

// bodyMargin covers the multipart headers, batch and meta.
const bodyMargin = 256 << 10

// Meta goes with a batch as the meta field. Nothing secret goes in it.
type Meta struct {
	Version     string `json:"version"`
	BroadcastID string `json:"broadcast_id"`
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	Test        bool   `json:"test"`
	Escalao     string `json:"escalao"`
	Opponent    string `json:"opponent"`
	StreamStart string `json:"stream_start"` // RFC 3339 with offset
	StreamEnd   string `json:"stream_end"`
	Score       Score  `json:"score"`
	BoxAddress  string `json:"box_address"`
	BoxName     string `json:"box_name"`
	// Truncated says raw.log lost its oldest lines to fit the server's limits.
	Truncated bool `json:"truncated,omitempty"`
}

type Score struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

const failedDir = "falhados"

func goodName(name string) bool { return validName.MatchString(name) && name != failedDir }

// BatchName is <broadcast id>-<start>, or local-<start> without a usable id.
// The start keeps two streams on one reused broadcast in separate batches.
func BatchName(broadcastID string, start time.Time) string {
	stamp := start.Format("20060102-150405")
	if name := broadcastID + "-" + stamp; goodName(broadcastID) && goodName(name) {
		return name
	}
	return "local-" + stamp
}

// Uploader writes batches to <Dir>/envios/<batch>/ and sends them.
type Uploader struct {
	Dir   string // dados
	Token string
	// Post sends the multipart body and returns the status and the answer.
	Post func(token, contentType string, body []byte) (int, []byte, error)
	Log  func(string)

	sendMu  sync.Mutex // one round at a time, and no batch swapped under it
	mu      sync.Mutex
	lastOK  time.Time
	lastErr string
	logged  map[string]bool // errors already logged since the last 200
	warned  bool
}

func (u *Uploader) queue() string { return filepath.Join(u.Dir, "envios") }

// Off reports a build without a token, saying so once.
func (u *Uploader) Off() bool {
	if u.Token != "" {
		return false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.warned {
		u.warned = true
		u.Log(i18n.T("envio.sem_chave"))
	}
	return true
}

// Write puts a batch on disk: raw.log, timeline.jsonl, marcador.log (+ .1)
// and bluetooth.log (likely boxes only), each cut from Before the start to
// the end and gzipped, and meta.json. scrub cleans marcador.log of what is
// personal. raw.log loses its oldest lines when the batch would pass the
// server's limits. A batch of the same name is replaced.
func (u *Uploader) Write(name string, meta Meta, start, end time.Time, scrub func([]byte) []byte) error {
	if !goodName(name) {
		return i18n.Err("envio.nome_invalido", "nome", strconv.Quote(name))
	}
	q := u.queue()
	dst := filepath.Join(q, name)
	if filepath.Dir(dst) != q {
		return i18n.Err("envio.nome_invalido", "nome", strconv.Quote(name))
	}
	if err := os.MkdirAll(q, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(q, ".tmp-"+name)
	os.RemoveAll(tmp)
	if err := os.Mkdir(tmp, 0o755); err != nil {
		return err
	}
	if err := u.fill(tmp, &meta, start.Add(-Before), end, scrub); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	u.sendMu.Lock()
	defer u.sendMu.Unlock()
	os.RemoveAll(dst)
	return os.Rename(tmp, dst)
}

func (u *Uploader) fill(tmp string, meta *Meta, from, to time.Time, scrub func([]byte) []byte) error {
	scrubbed := func(w io.Writer, r io.Reader) error {
		var b bytes.Buffer
		if err := CutFollow(&b, r, from, to, LogStamp); err != nil {
			return err
		}
		_, err := w.Write(scrub(b.Bytes()))
		return err
	}
	files := []struct {
		name string
		copy func(io.Writer, io.Reader) error
	}{
		{"timeline.jsonl", func(w io.Writer, r io.Reader) error { return Cut(w, r, from, to, TimelineStamp) }},
		{"marcador.log", scrubbed},
		{"marcador.log.1", scrubbed},
		{"bluetooth.log", func(w io.Writer, r io.Reader) error {
			return Cut(likelyOnly{w}, r, from, to, BluetoothStamp)
		}},
	}
	var others int64
	for _, f := range files {
		n, err := gzipFile(filepath.Join(u.Dir, f.name), filepath.Join(tmp, f.name+".gz"), f.copy)
		if err != nil {
			return err
		}
		others += n
	}
	truncated, err := u.rawLog(tmp, from, to, min(maxFile, maxBody-others-bodyMargin))
	if err != nil {
		return err
	}
	meta.Truncated = truncated
	b, _ := json.MarshalIndent(meta, "", "  ")
	return os.WriteFile(filepath.Join(tmp, "meta.json"), b, 0o644)
}

// rawLog writes raw.log.gz with the window's lines, keeping only the newest
// that fit in budget bytes gzipped. It reports whether it cut any.
func (u *Uploader) rawLog(tmp string, from, to time.Time, budget int64) (bool, error) {
	in, err := os.Open(filepath.Join(u.Dir, "raw.log"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer in.Close()
	plain := filepath.Join(tmp, ".raw.log")
	defer os.Remove(plain)
	n, err := writePlain(plain, func(w io.Writer) error { return Cut(w, in, from, to, RawStamp) })
	if err != nil {
		return false, err
	}
	dst := filepath.Join(tmp, "raw.log.gz")
	keep, truncated := n, false
	for range 10 {
		size, err := gzipTail(plain, dst, n-keep)
		if err != nil {
			return truncated, err
		}
		if size <= budget || keep == 0 {
			return truncated, nil
		}
		keep = max(0, int64(float64(keep)*float64(budget)/float64(size)*0.9))
		truncated = true
	}
	return truncated, i18n.Err("envio.raw_nao_cabe")
}

// gzipTail gzips src from offset on (from the next line when offset cuts
// one) into dst and returns dst's size.
func gzipTail(src, dst string, offset int64) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	var r io.Reader = in
	if offset > 0 {
		if _, err := in.Seek(offset-1, io.SeekStart); err != nil {
			return 0, err
		}
		br := bufio.NewReader(in)
		if _, err := br.ReadBytes('\n'); err != nil && err != io.EOF {
			return 0, err
		}
		r = br
	}
	return writeGzip(dst, func(w io.Writer) error { _, err := io.Copy(w, r); return err })
}

// likelyOnly passes the bluetooth.log lines of likely boxes: the other
// devices are the phones and earphones of the people in the hall. Cut
// writes one line at a time.
type likelyOnly struct{ w io.Writer }

func (l likelyOnly) Write(line []byte) (int, error) {
	if bytes.HasSuffix(bytes.TrimRight(line, "\r\n"), []byte(" likely=true")) {
		return l.w.Write(line)
	}
	return len(line), nil
}

// gzipFile writes src through copy into dst, gzipped, and returns dst's
// size. A missing src writes nothing.
func gzipFile(src, dst string, copy func(io.Writer, io.Reader) error) (int64, error) {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer in.Close()
	return writeGzip(dst, func(w io.Writer) error { return copy(w, in) })
}

func writeGzip(dst string, fill func(io.Writer) error) (int64, error) {
	return writePlain(dst, func(w io.Writer) error {
		zw := gzip.NewWriter(w)
		err := fill(zw)
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
		return err
	})
}

func writePlain(dst string, fill func(io.Writer) error) (int64, error) {
	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	cw := &counter{w: out}
	err = fill(cw)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return cw.n, err
}

type counter struct {
	w io.Writer
	n int64
}

func (c *counter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}

// batches lists the batches in dir, oldest first by meta.json.
func batches(dir string) []string {
	entries, _ := os.ReadDir(dir)
	type batch struct {
		name string
		at   time.Time
	}
	var bs []batch
	for _, e := range entries {
		if !e.IsDir() || !goodName(e.Name()) {
			continue
		}
		fi, err := os.Stat(filepath.Join(dir, e.Name(), "meta.json"))
		if err != nil {
			continue
		}
		bs = append(bs, batch{e.Name(), fi.ModTime()})
	}
	slices.SortStableFunc(bs, func(a, b batch) int { return a.at.Compare(b.at) })
	names := make([]string, len(bs))
	for i, b := range bs {
		names[i] = b.name
	}
	return names
}

func (u *Uploader) pending() []string { return batches(u.queue()) }

// toFailed moves a pending batch to falhados, dated now so it counts as the
// newest there.
func (u *Uploader) toFailed(name string) {
	failed := filepath.Join(u.queue(), failedDir)
	os.MkdirAll(failed, 0o755)
	os.RemoveAll(filepath.Join(failed, name))
	if os.Rename(filepath.Join(u.queue(), name), filepath.Join(failed, name)) == nil {
		now := time.Now()
		os.Chtimes(filepath.Join(failed, name, "meta.json"), now, now)
	}
}

// prune sends batches older than MaxAge to falhados, and keeps only the
// newest KeepFailed there.
func (u *Uploader) prune() {
	for _, name := range u.pending() {
		fi, err := os.Stat(filepath.Join(u.queue(), name, "meta.json"))
		if err == nil && time.Since(fi.ModTime()) > MaxAge {
			u.toFailed(name)
			u.Log(i18n.T("envio.esperaram_60_dias", "nome", name))
		}
	}
	failed := filepath.Join(u.queue(), failedDir)
	old := batches(failed)
	for _, name := range old[:max(0, len(old)-KeepFailed)] {
		os.RemoveAll(filepath.Join(failed, name))
	}
}

// retried are the refusals that may pass later: a token not yet known, the
// endpoint not yet deployed, too many requests.
func retried(code int) bool {
	return code < 400 || code >= 500 || code == http.StatusUnauthorized || code == http.StatusNotFound || code == http.StatusTooManyRequests
}

// Send tries every pending batch, oldest first. A 200 deletes it; 401, 404,
// 429 and 5xx leave it for the next round; any other 4xx moves it to
// falhados. No network ends the round.
func (u *Uploader) Send() {
	if u.Off() {
		return
	}
	u.sendMu.Lock()
	defer u.sendMu.Unlock()
	u.prune()
	defer u.prune()
	for _, name := range u.pending() {
		code, answer, err := u.send(name, filepath.Join(u.queue(), name))
		switch {
		case err != nil:
			u.failed(i18n.T("envio.nao_enviei", "nome", name, "err", err), false)
			return
		case code == http.StatusOK:
			os.RemoveAll(filepath.Join(u.queue(), name))
			u.mu.Lock()
			u.lastOK, u.lastErr, u.logged = time.Now(), "", nil
			u.mu.Unlock()
			u.Log(i18n.T("envio.enviados", "nome", name))
		case retried(code):
			u.failed(i18n.T("envio.nao_enviei_codigo", "nome", name, "codigo", code, "motivo", reason(answer)), false)
		default:
			u.toFailed(name)
			u.failed(i18n.T("envio.recusados", "nome", name, "codigo", code, "motivo", reason(answer)), true)
		}
	}
}

// failed notes an error, logging it unless it was logged since the last 200.
func (u *Uploader) failed(msg string, always bool) {
	u.mu.Lock()
	u.lastErr = msg
	seen := u.logged[msg]
	if u.logged == nil {
		u.logged = map[string]bool{}
	}
	u.logged[msg] = true
	u.mu.Unlock()
	if always || !seen {
		u.Log(msg)
	}
}

func reason(answer []byte) string {
	var v struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(answer, &v) == nil && v.Error != "" {
		return ": " + v.Error
	}
	return ""
}

func (u *Uploader) send(name, dir string) (int, []byte, error) {
	meta, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return 0, nil, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("batch", name)
	mw.WriteField("meta", string(meta))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil, err
	}
	for _, e := range entries {
		if e.Name() == "meta.json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return 0, nil, err
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[]"; filename=%s`, strconv.Quote(e.Name())))
		h.Set("Content-Type", "application/gzip")
		pw, err := mw.CreatePart(h)
		if err != nil {
			return 0, nil, err
		}
		pw.Write(b)
	}
	mw.Close()
	return u.Post(u.Token, mw.FormDataContentType(), body.Bytes())
}

// Status is the uploads part of GET /api/status.
func (u *Uploader) Status() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	ok := ""
	if !u.lastOK.IsZero() {
		ok = u.lastOK.Format(time.RFC3339)
	}
	return map[string]any{"pending": len(u.pending()), "last_ok": ok, "last_error": u.lastErr}
}
