package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// The API addresses; tests point them at a fake.
var (
	APIBase    = "https://www.googleapis.com/youtube/v3"
	UploadBase = "https://www.googleapis.com/upload/youtube/v3"
)

// ErrReconnect: Google no longer takes the saved login (revoked, expired or
// the password changed), so only a new "Ligar ao YouTube" helps.
var ErrReconnect = errors.New("A ligação ao YouTube expirou. Em Definições > YouTube, carregue em Ligar ao YouTube outra vez.")

// APIError is an answer from YouTube with an error status.
type APIError struct {
	Code    int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("o YouTube respondeu %d %s: %s", e.Code, e.Reason, e.Message)
}

// Client calls the API as the signed-in channel.
type Client struct {
	hc *http.Client
}

// NewClient refreshes tok when it runs out; save gets every new token.
func NewClient(c Credentials, tok *oauth2.Token, save func(*oauth2.Token)) *Client {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 30 * time.Second})
	src := &saving{src: c.config("").TokenSource(ctx, tok), last: tok.AccessToken, save: save}
	hc := oauth2.NewClient(ctx, oauth2.ReuseTokenSource(tok, src))
	hc.Timeout = 60 * time.Second
	return &Client{hc: hc}
}

type saving struct {
	src  oauth2.TokenSource
	last string
	save func(*oauth2.Token)
}

func (s *saving) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err == nil && t.AccessToken != s.last {
		s.last = t.AccessToken
		s.save(t)
	}
	return t, err
}

// do sends one request; body is JSON (any) or a raw upload ([]byte, with ctype).
func (c *Client) do(ctx context.Context, method, base, path string, q url.Values, body any, ctype string, out any) error {
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			return err
		}
		rd, ctype = bytes.NewReader(j), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path+"?"+q.Encode(), rd)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return netError(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Errors  []struct {
					Reason string `json:"reason"`
				} `json:"errors"`
			} `json:"error"`
		}
		json.Unmarshal(data, &e)
		ae := &APIError{Code: resp.StatusCode, Message: e.Error.Message}
		if len(e.Error.Errors) > 0 {
			ae.Reason = e.Error.Errors[0].Reason
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return ErrReconnect
		}
		return ae
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// netError turns a failed refresh into ErrReconnect and drops the URL from
// other errors.
func netError(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode == "invalid_grant" || re.ErrorCode == "unauthorized_client" || re.Response != nil && re.Response.StatusCode == http.StatusUnauthorized {
			return ErrReconnect
		}
		return errors.New("o Google recusou renovar o acesso (" + re.ErrorCode + ")")
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return errors.New("sem ligação ao YouTube: " + ue.Err.Error())
	}
	return err
}

func vals(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return q
}

// Channel is the signed-in channel, for "Ligado como".
type Channel struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
}

func (c *Client) Channel(ctx context.Context) (Channel, error) {
	var r struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				Thumbnails map[string]struct {
					URL string `json:"url"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := c.do(ctx, "GET", APIBase, "/channels", vals("part", "snippet", "mine", "true"), nil, "", &r); err != nil {
		return Channel{}, err
	}
	if len(r.Items) == 0 {
		return Channel{}, errors.New("esta conta Google não tem canal do YouTube")
	}
	it := r.Items[0]
	return Channel{it.ID, it.Snippet.Title, it.Snippet.Thumbnails["default"].URL}, nil
}

// ChannelCheck is what the API says about the channel's live streaming and
// phone verification; "" where it doesn't say.
type ChannelCheck struct {
	Live     string `json:"live"`     // "on" or "off"
	Verified string `json:"verified"` // "yes", "no" or "blocked"
}

// CheckChannel reads channels.list status.longUploadsStatus (allowed once
// the phone is verified, eligible before) and tries liveBroadcasts.list,
// which answers liveStreamingNotEnabled while live is off. 2 quota units.
func (c *Client) CheckChannel(ctx context.Context) (ChannelCheck, error) {
	var out ChannelCheck
	var r struct {
		Items []struct {
			Status struct {
				LongUploads string `json:"longUploadsStatus"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := c.do(ctx, "GET", APIBase, "/channels", vals("part", "status", "mine", "true"), nil, "", &r); err != nil {
		return out, err
	}
	if len(r.Items) == 0 {
		return out, errors.New("esta conta Google não tem canal do YouTube")
	}
	out.Verified = map[string]string{"allowed": "yes", "eligible": "no", "disallowed": "blocked"}[r.Items[0].Status.LongUploads]
	err := c.do(ctx, "GET", APIBase, "/liveBroadcasts", vals("part", "id", "mine", "true", "maxResults", "1"), nil, "", nil)
	var ae *APIError
	switch {
	case err == nil:
		out.Live = "on"
	case errors.As(err, &ae) && ae.Reason == "liveStreamingNotEnabled":
		out.Live = "off"
	case errors.Is(err, ErrReconnect):
		return out, err
	}
	return out, nil
}

// Stream is the club's one reusable stream: OBS sends to Server with Key.
type Stream struct {
	ID     string `json:"id"`
	Server string `json:"server"`
	Key    string `json:"key"`
}

type liveStream struct {
	ID      string `json:"id"`
	Snippet struct {
		Title string `json:"title"`
	} `json:"snippet"`
	CDN struct {
		IngestionType string `json:"ingestionType"`
		Info          struct {
			StreamName       string `json:"streamName"`
			IngestionAddress string `json:"ingestionAddress"`
		} `json:"ingestionInfo"`
	} `json:"cdn"`
	ContentDetails struct {
		IsReusable bool `json:"isReusable"`
	} `json:"contentDetails"`
}

func (s liveStream) stream() Stream {
	return Stream{s.ID, s.CDN.Info.IngestionAddress, s.CDN.Info.StreamName}
}

// StreamByID reads a saved stream back; ok false when it is gone.
func (c *Client) StreamByID(ctx context.Context, id string) (Stream, bool, error) {
	var r struct {
		Items []liveStream `json:"items"`
	}
	if err := c.do(ctx, "GET", APIBase, "/liveStreams", vals("part", "id,snippet,cdn,contentDetails", "id", id), nil, "", &r); err != nil {
		return Stream{}, false, err
	}
	if len(r.Items) == 0 {
		return Stream{}, false, nil
	}
	return r.Items[0].stream(), true, nil
}

// EnsureStream finds the reusable RTMP stream called title, or makes it.
func (c *Client) EnsureStream(ctx context.Context, title string) (Stream, error) {
	q := vals("part", "id,snippet,cdn,contentDetails", "mine", "true", "maxResults", "50")
	for page := 0; page < 4; page++ {
		var r struct {
			Items []liveStream `json:"items"`
			Next  string       `json:"nextPageToken"`
		}
		if err := c.do(ctx, "GET", APIBase, "/liveStreams", q, nil, "", &r); err != nil {
			return Stream{}, err
		}
		for _, s := range r.Items {
			if s.Snippet.Title == title && s.ContentDetails.IsReusable && s.CDN.IngestionType == "rtmp" {
				return s.stream(), nil
			}
		}
		if r.Next == "" {
			break
		}
		q.Set("pageToken", r.Next)
	}
	body := map[string]any{
		"snippet":        map[string]any{"title": title},
		"cdn":            map[string]any{"ingestionType": "rtmp", "resolution": "variable", "frameRate": "variable"},
		"contentDetails": map[string]any{"isReusable": true},
	}
	var s liveStream
	if err := c.do(ctx, "POST", APIBase, "/liveStreams", vals("part", "snippet,cdn,contentDetails"), body, "", &s); err != nil {
		return Stream{}, err
	}
	return s.stream(), nil
}

// Broadcast is one live event (its id is the video's id too).
type Broadcast struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Start       time.Time `json:"start"`
	Status      string    `json:"status"`  // lifeCycleStatus: created, ready, testing, live, complete…
	Privacy     string    `json:"privacy"` // public, unlisted, private
	StreamID    string    `json:"stream_id"`
}

type liveBroadcast struct {
	ID      string `json:"id"`
	Snippet struct {
		Title              string `json:"title"`
		Description        string `json:"description"`
		ScheduledStartTime string `json:"scheduledStartTime"`
		ActualStartTime    string `json:"actualStartTime"`
	} `json:"snippet"`
	Status struct {
		LifeCycleStatus string `json:"lifeCycleStatus"`
		PrivacyStatus   string `json:"privacyStatus"`
	} `json:"status"`
	ContentDetails struct {
		BoundStreamID string `json:"boundStreamId"`
	} `json:"contentDetails"`
}

func (b liveBroadcast) broadcast() Broadcast {
	t, err := time.Parse(time.RFC3339, b.Snippet.ActualStartTime)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, b.Snippet.ScheduledStartTime)
	}
	return Broadcast{b.ID, b.Snippet.Title, b.Snippet.Description, t, b.Status.LifeCycleStatus, b.Status.PrivacyStatus, b.ContentDetails.BoundStreamID}
}

// NewBroadcast makes a broadcast that starts and ends with the stream;
// privacy is public, unlisted or private.
func (c *Client) NewBroadcast(ctx context.Context, title, description, privacy string, start time.Time) (Broadcast, error) {
	body := map[string]any{
		"snippet": map[string]any{"title": title, "description": description,
			"scheduledStartTime": start.UTC().Format(time.RFC3339)},
		"status": map[string]any{"privacyStatus": privacy, "selfDeclaredMadeForKids": false},
		"contentDetails": map[string]any{"enableAutoStart": true, "enableAutoStop": true,
			"monitorStream": map[string]any{"enableMonitorStream": false}},
	}
	var b liveBroadcast
	if err := c.do(ctx, "POST", APIBase, "/liveBroadcasts", vals("part", "snippet,status,contentDetails"), body, "", &b); err != nil {
		return Broadcast{}, err
	}
	return b.broadcast(), nil
}

// Bind ties the broadcast to the stream, so OBS's signal goes to it.
func (c *Client) Bind(ctx context.Context, broadcastID, streamID string) error {
	return c.do(ctx, "POST", APIBase, "/liveBroadcasts/bind",
		vals("id", broadcastID, "streamId", streamID, "part", "id,contentDetails"), nil, "", nil)
}

// Broadcasts reads broadcasts by id; missing ones are left out.
func (c *Client) Broadcasts(ctx context.Context, ids ...string) ([]Broadcast, error) {
	var r struct {
		Items []liveBroadcast `json:"items"`
	}
	q := vals("part", "id,snippet,status,contentDetails", "id", strings.Join(ids, ","), "maxResults", "50")
	if err := c.do(ctx, "GET", APIBase, "/liveBroadcasts", q, nil, "", &r); err != nil {
		return nil, err
	}
	out := make([]Broadcast, 0, len(r.Items))
	for _, b := range r.Items {
		out = append(out, b.broadcast())
	}
	return out, nil
}

// Completed lists the channel's finished broadcasts, newest first, up to max.
func (c *Client) Completed(ctx context.Context, max int) ([]Broadcast, error) {
	q := vals("part", "id,snippet,status,contentDetails", "broadcastStatus", "completed", "broadcastType", "all", "maxResults", "50")
	var out []Broadcast
	for len(out) < max {
		var r struct {
			Items []liveBroadcast `json:"items"`
			Next  string          `json:"nextPageToken"`
		}
		if err := c.do(ctx, "GET", APIBase, "/liveBroadcasts", q, nil, "", &r); err != nil {
			return nil, err
		}
		for _, b := range r.Items {
			out = append(out, b.broadcast())
		}
		if r.Next == "" {
			break
		}
		q.Set("pageToken", r.Next)
	}
	return out, nil
}

// Complete ends a broadcast that is still live after the stream stopped.
func (c *Client) Complete(ctx context.Context, id string) error {
	return c.do(ctx, "POST", APIBase, "/liveBroadcasts/transition",
		vals("broadcastStatus", "complete", "id", id, "part", "id,status"), nil, "", nil)
}

// SetPrivacy changes a video's privacy. videos.update clears any status field
// left out (bar the made-for-kids ones), so it reads the status first and
// sends the rest back as it was.
func (c *Client) SetPrivacy(ctx context.Context, videoID, privacy string) error {
	var r struct {
		Items []struct {
			ID     string         `json:"id"`
			Status map[string]any `json:"status"`
		} `json:"items"`
	}
	if err := c.do(ctx, "GET", APIBase, "/videos", vals("part", "status", "id", videoID), nil, "", &r); err != nil {
		return err
	}
	if len(r.Items) == 0 {
		return errors.New("o YouTube não encontrou o vídeo " + videoID)
	}
	st := map[string]any{"privacyStatus": privacy}
	for _, k := range []string{"embeddable", "license", "publicStatsViewable"} {
		if v, ok := r.Items[0].Status[k]; ok {
			st[k] = v
		}
	}
	return c.do(ctx, "PUT", APIBase, "/videos", vals("part", "status"), map[string]any{"id": videoID, "status": st}, "", nil)
}

// ErrThumbnail: the channel can't have custom thumbnails yet.
var ErrThumbnail = errors.New("O YouTube não aceitou a miniatura, porque o canal ainda não está verificado em youtube.com/verify. O direto continua com a imagem que o YouTube escolher.")

// SetThumbnail uploads a PNG as the video's thumbnail.
func (c *Client) SetThumbnail(ctx context.Context, videoID string, png []byte) error {
	err := c.do(ctx, "POST", UploadBase, "/thumbnails/set", vals("videoId", videoID, "uploadType", "media"), png, "image/png", nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Code == http.StatusForbidden {
		return ErrThumbnail
	}
	return err
}
