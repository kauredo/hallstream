// Package fpb talks to the FPB Calendar API (calendariofpb.pt) and keeps a
// copy of every answer on disk, so the panel works offline after one load.
package fpb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

var ErrNotFound error = i18n.Key("fpb.sem_dados")

// ErrBusy is the calendar answering 429, or a request held back so it does not.
var ErrBusy error = i18n.Key("fpb.ocupada")

// The calendar answers 429 past 30 API requests a minute from one address:
// apiBurst at once and then one every Client.Every stays under that, and a
// request waits up to maxWait for its turn. After a 429 the client stays away
// for the Retry-After: busyPause without one (the calendar counts by the
// minute), maxPause at most.
const (
	apiBurst  = 5
	maxWait   = 10 * time.Second
	busyPause = time.Minute
	maxPause  = 5 * time.Minute
)

type Client struct {
	BaseURL   func() string
	CacheDir  string
	UserAgent string
	HTTP      *http.Client
	// PhotoHosts are the only hosts DownloadPhoto fetches from, over https
	// (AllowHTTP is for tests).
	PhotoHosts []string
	AllowHTTP  bool
	// UploadHost is the only host Post sends the token to (tests change it).
	UploadHost string
	// Every is the pause between API requests once apiBurst have gone; 0
	// sends them all at once (tests).
	Every time.Duration

	photoHTTP *http.Client
	logoHTTP  *http.Client // opponent logos live on other hosts
	sleep     func(time.Duration)
	mu        sync.Mutex
	reachable bool
	busy      bool // the last answer was a 429, or the request was held back
	cachedAt  time.Time
	next      time.Time // when the next API request may go
}

func New(baseURL func() string, cacheDir, version string) *Client {
	c := &Client{
		BaseURL:    baseURL,
		CacheDir:   cacheDir,
		UserAgent:  "hallstream/" + version,
		PhotoHosts: []string{"sav2.fpb.pt", "www.fpb.pt"},
		UploadHost: "calendariofpb.pt",
		Every:      3 * time.Second,
		logoHTTP:   &http.Client{Timeout: 10 * time.Second},
		sleep:      time.Sleep,
	}
	// API answers only come from the calendar itself, redirects included.
	c.HTTP = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		base, err := url.Parse(c.BaseURL())
		if err != nil || r.URL.Host != base.Host {
			return i18n.Err("fpb.redirecionou_fora", "url", r.URL.Redacted())
		}
		if len(via) >= 5 {
			return i18n.Err("fpb.redirecionamentos")
		}
		return nil
	}}
	c.photoHTTP = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return i18n.Err("fpb.redirecionamentos")
		}
		return c.photoAllowed(r.URL)
	}}
	return c
}

// Status reports whether the last request reached the FPB and when the data
// last served from disk was saved.
func (c *Client) Status() (reachable bool, cachedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reachable, c.cachedAt
}

// Busy reports whether the last Get met a 429 or was held back, so that what
// it returned was the saved copy.
func (c *Client) Busy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy
}

func (c *Client) url(p string) string {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	return strings.TrimRight(c.BaseURL(), "/") + p
}

// turn waits for this request's place under the limit, or refuses it with
// ErrBusy when that place is more than maxWait away.
func (c *Client) turn() error {
	if c.Every == 0 {
		return nil
	}
	c.mu.Lock()
	now := time.Now()
	// A client left alone has apiBurst requests in hand.
	if idle := now.Add(-(apiBurst - 1) * c.Every); c.next.Before(idle) {
		c.next = idle
	}
	wait := c.next.Sub(now)
	if wait > maxWait {
		c.mu.Unlock()
		return ErrBusy
	}
	c.next = c.next.Add(c.Every)
	c.mu.Unlock()
	if wait > 0 {
		c.sleep(wait)
	}
	return nil
}

// retryAfter reads a 429's Retry-After, in seconds or as a date: busyPause
// when it is neither, and never more than maxPause.
func retryAfter(h string) time.Duration {
	if n, err := strconv.ParseInt(h, 10, 64); err == nil && n >= 0 {
		return time.Duration(min(n, int64(maxPause/time.Second))) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		return min(max(time.Until(t), 0), maxPause)
	}
	return busyPause
}

func (c *Client) fetch(u string) (*http.Response, error) {
	if err := c.turn(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err == nil && resp.StatusCode == http.StatusTooManyRequests {
		c.mu.Lock()
		if t := time.Now().Add(retryAfter(resp.Header.Get("Retry-After"))); t.After(c.next) {
			c.next = t
		}
		c.mu.Unlock()
	}
	return resp, err
}

// ErrUploadHost refuses to send the token anywhere but UploadHost over https.
var ErrUploadHost error = i18n.Key("fpb.envio_fora")

// uploadAllowed is https on UploadHost or one of its subdomains.
func (c *Client) uploadAllowed(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && c.UploadHost != "" && (h == c.UploadHost || strings.HasSuffix(h, "."+c.UploadHost))
}

// Post sends body to an API path with a bearer token, only to UploadHost
// over https (redirects included), and returns the status and the start of
// the answer.
func (c *Client) Post(p, token, contentType string, body []byte) (int, []byte, error) {
	target, err := url.Parse(c.url(p))
	if err != nil || !c.uploadAllowed(target) {
		return 0, nil, ErrUploadHost
	}
	req, err := http.NewRequest("POST", target.String(), bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	hc := http.Client{Transport: c.HTTP.Transport, Timeout: 5 * time.Minute, // up to 25 MB
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if !c.uploadAllowed(r.URL) {
				return ErrUploadHost
			}
			if len(via) >= 5 {
				return i18n.Err("fpb.redirecionamentos")
			}
			return nil
		}}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, answer, nil
}

func cacheName(p string) string {
	sum := sha256.Sum256([]byte(p))
	return legacyCacheName(p, "-"+hex.EncodeToString(sum[:4]))
}

// legacyCacheName is the file name used before the hash was added.
func legacyCacheName(p, suffix string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '_'
	}, strings.TrimPrefix(p, "/")) + suffix + ".json"
}

// Get fetches an API path ("/api/teams?name=x"). When the FPB cannot be
// reached it returns the copy saved on disk, if there is one.
func (c *Client) Get(p string) ([]byte, error) {
	file := filepath.Join(c.CacheDir, cacheName(p))
	resp, err := c.fetch(c.url(p))
	if err == nil {
		defer resp.Body.Close()
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		switch {
		case rerr != nil:
			err = rerr
		case resp.StatusCode == http.StatusOK && !json.Valid(body):
			err = i18n.Err("fpb.resposta_inesperada")
		case resp.StatusCode == http.StatusOK:
			c.set(true, false, time.Now())
			os.WriteFile(file+".tmp", body, 0o644)
			os.Rename(file+".tmp", file)
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			c.set(true, false, time.Time{})
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests:
			err = ErrBusy
		default:
			err = i18n.Err("fpb.respondeu", "codigo", resp.StatusCode)
		}
	}
	// A busy FPB is still there, so the saved copy goes out without "sem ligação".
	busy := errors.Is(err, ErrBusy)
	c.set(busy, busy, time.Time{})
	body, rerr := os.ReadFile(file)
	if os.IsNotExist(rerr) {
		file = filepath.Join(c.CacheDir, legacyCacheName(p, ""))
		body, rerr = os.ReadFile(file)
	}
	if rerr != nil && busy {
		return nil, ErrBusy
	}
	if rerr != nil {
		return nil, i18n.Err("fpb.sem_ligacao", "err", err)
	}
	if st, err := os.Stat(file); err == nil {
		c.set(busy, busy, st.ModTime())
	}
	return body, nil
}

func (c *Client) set(reachable, busy bool, cachedAt time.Time) {
	c.mu.Lock()
	c.reachable, c.busy = reachable, busy
	if !cachedAt.IsZero() {
		c.cachedAt = cachedAt
	}
	c.mu.Unlock()
}

// Flex is a JSON value that may come as a string, a number or null.
type Flex string

func (f *Flex) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		return json.Unmarshal(b, (*string)(f))
	}
	*f = Flex(s)
	return nil
}

type Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	LogoURL     string `json:"logo_url"`
	ClubLogoURL string `json:"club_logo_url"`
}

type NextGame struct {
	ID              int64  `json:"id"`
	Date            string `json:"date"`
	Time            string `json:"time"`
	Location        string `json:"location"`
	CompetitionName string `json:"competition_name"`
	CompetitionSlug string `json:"competition_slug"`
	GameWeek        Flex   `json:"game_week"`
	PhaseName       string `json:"phase_name"`
	Home            Team   `json:"home"`
	Away            Team   `json:"away"`
}

type NextGames struct {
	Team  Team       `json:"team"`
	Games []NextGame `json:"games"`
}

type RosterPlayer struct {
	PlayerID     int64  `json:"player_id"`
	FPBAthleteID int64  `json:"fpb_athlete_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	JerseyNumber Flex   `json:"jersey_number"`
	PhotoURL     string `json:"photo_url"`
	Games        int    `json:"games"`
}

type Roster struct {
	Team    Team           `json:"team"`
	Season  Flex           `json:"season"`
	Players []RosterPlayer `json:"players"`
}

func getJSON[T any](c *Client, p string) (T, error) {
	var v T
	b, err := c.Get(p)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, i18n.Err("fpb.resposta_inesperada_erro", "err", err)
	}
	return v, nil
}

// TeamInfo is one result of the FPB team search.
type TeamInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Age         string `json:"age"`
	Gender      string `json:"gender"`
	Slug        string `json:"slug"` // ends with the season: "-2026-2027"
	SquadLabel  string `json:"squad_label"`
	HasFixtures bool   `json:"has_fixtures"`
}

func TeamsPath(name string) string { return "/api/teams?name=" + url.QueryEscape(name) }

func (c *Client) Teams(name string) ([]TeamInfo, error) {
	return getJSON[[]TeamInfo](c, TeamsPath(name))
}

func NextGamesPath(teamID int64) string {
	return fmt.Sprintf("/api/teams/%d/next-games?days=14", teamID)
}
func RosterPath(teamID int64) string { return fmt.Sprintf("/api/teams/%d/roster", teamID) }

func (c *Client) NextGames(teamID int64) (NextGames, error) {
	return getJSON[NextGames](c, NextGamesPath(teamID))
}

func (c *Client) Roster(teamID int64) (Roster, error) {
	return getJSON[Roster](c, RosterPath(teamID))
}

// Download fetches a logo (up to 5 MB) and returns it with a file extension.
func (c *Client) Download(u string) ([]byte, string, error) {
	return c.image(context.Background(), c.logoHTTP, c.url(u), i18n.T("fpb.o_logotipo"))
}

// DownloadPhoto fetches a player photo, only from PhotoHosts, redirects
// included, and gives up when ctx ends.
func (c *Client) DownloadPhoto(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	if err := c.photoAllowed(u); err != nil {
		return nil, "", err
	}
	return c.image(ctx, c.photoHTTP, raw, i18n.T("fpb.a_fotografia"))
}

func (c *Client) photoAllowed(u *url.URL) error {
	if (u.Scheme == "https" || c.AllowHTTP && u.Scheme == "http") && slices.Contains(c.PhotoHosts, u.Host) {
		return nil
	}
	return i18n.Err("fpb.foto_fora", "url", u.Redacted())
}

const maxImage = 5 << 20

func (c *Client) image(ctx context.Context, hc *http.Client, u, what string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", i18n.Err("fpb.imagem_respondeu", "o_que", what, "codigo", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > maxImage {
		return nil, "", i18n.Err("fpb.mais_5mb", "o_que", what)
	}
	return b, ImageExt(resp.Header.Get("Content-Type"), u), nil
}

// ImageExt picks a file extension from a content type, else from the name.
func ImageExt(contentType, name string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "svg"):
		return ".svg"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	}
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	switch ext := strings.ToLower(path.Ext(name)); ext {
	case ".png", ".jpg", ".svg", ".webp", ".gif":
		return ext
	case ".jpeg":
		return ".jpg"
	}
	return ".png"
}

// RealPhoto drops FPB's "no photo" placeholder, so those players show initials.
func RealPhoto(u string) string {
	if strings.HasSuffix(u, "/noplayer.png") {
		return ""
	}
	return u
}
