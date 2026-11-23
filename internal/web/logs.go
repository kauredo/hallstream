package web

import (
	"os"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/gamelog"
	"github.com/kauredo/hallstream/internal/i18n"
	"github.com/kauredo/hallstream/internal/state"
	"github.com/kauredo/hallstream/internal/store"
)

func (s *Server) newUploads() {
	s.uploads = &gamelog.Uploader{Dir: s.Store.Dir, Token: s.UploadToken, Log: s.Log,
		Post: func(token, contentType string, body []byte) (int, []byte, error) {
			return s.FPB.Post(gamelog.Path, token, contentType, body)
		}}
}

// RunUploads sends the batches left from before at start, then every 10 minutes.
func (s *Server) RunUploads() {
	for {
		s.uploads.Send()
		time.Sleep(gamelog.Every)
	}
}

// streamStarted notes when the stream went on air (once per stream).
func (s *Server) streamStarted(at time.Time) {
	s.logMu.Lock()
	if !s.logStart.IsZero() {
		s.logMu.Unlock()
		return
	}
	s.logStart = at
	s.logMu.Unlock()
	id, test := "", false
	if r := s.ytOnAir(); r != nil {
		id, test = r.BroadcastID, r.Test
	}
	s.Timeline.Add("stream_start", map[string]any{"broadcast_id": id, "test": test})
	// Each batch starts from the state on air, not only its changes.
	s.syncMu.Lock()
	if g := strings.Split(s.lastGame, "\x00"); len(g) == 3 {
		s.Timeline.Add("game", map[string]any{"key": g[0], "home": g[1], "away": g[2], "initial": true})
	}
	s.syncMu.Unlock()
	s.State.EmitCurrent()
}

// maxWindow bounds how far before the stream's end its logs reach (plus Before).
const maxWindow = 6 * time.Hour

// windowStart is when the stream's logs start: OBS's start, or a reused
// broadcast's first start when that is earlier and at most maxWindow before
// it. Never more than maxWindow before the end.
func windowStart(obs time.Time, startedAt string, end time.Time) time.Time {
	start := obs
	if t, err := time.Parse(time.RFC3339, startedAt); err == nil && t.Before(obs) && obs.Sub(t) <= maxWindow {
		start = t
	}
	if floor := end.Add(-maxWindow); start.Before(floor) {
		start = floor
	}
	return start
}

// logsEnd runs once when the stream stops: the timeline line, then the
// batch with this stream's logs, written and sent in the background. rec is
// the broadcast that was on air, if any.
func (s *Server) logsEnd(rec *ytRecord) {
	s.logMu.Lock()
	start := s.logStart
	s.logStart = time.Time{}
	s.logMu.Unlock()
	if start.IsZero() {
		return
	}
	end := time.Now()
	r := ytRecord{}
	if rec != nil {
		r = *rec
	}
	// Stamped with the cut's end, so the batch always has it.
	s.Timeline.AddAt(end, "stream_stop", map[string]any{"broadcast_id": r.BroadcastID, "test": r.Test})
	if s.uploads.Off() {
		return
	}
	start = windowStart(start, r.StartedAt, end)
	snap := s.State.Snapshot()
	meta := gamelog.Meta{Version: s.Version, BroadcastID: r.BroadcastID, VideoID: r.VideoID, Title: r.Title, Test: r.Test,
		Escalao: r.Escalao, Opponent: r.Opponent, StreamStart: start.Format(time.RFC3339), StreamEnd: end.Format(time.RFC3339),
		Score: gamelog.Score{Home: snap.Live.Home, Away: snap.Live.Away}, BoxAddress: snap.Box.Scan.Address, BoxName: snap.Box.Scan.Name}
	if rec == nil {
		meta.Title, _ = broadcastText(snap.Game, start)
		meta.Opponent = teamName(snap.Game.Away.Name)
		if snap.Game.Group != nil {
			meta.Escalao = snap.Game.Group.Name
		}
	}
	if meta.BoxAddress == "" {
		meta.BoxAddress = s.Store.Settings().BoxAddress
	}
	name := gamelog.BatchName(r.BroadcastID, start)
	scrub := s.scrubber()
	s.once(i18n.T("log.tarefa.envios"), func() {
		if err := s.uploads.Write(name, meta, start, end, scrub); err != nil {
			s.Log(i18n.T("log.envio.nao_preparei", "err", err))
			return
		}
		s.uploads.Send()
	})
}

// scrubber takes out of marcador.log what is personal: the Windows user
// and the players' names.
func (s *Server) scrubber() func([]byte) []byte {
	var names []string
	for _, g := range s.Store.Groups() {
		for _, p := range g.Players {
			names = append(names, p.DisplayName)
			if p.FPBName != nil {
				names = append(names, *p.FPBName)
			}
		}
	}
	home, _ := os.UserHomeDir()
	return gamelog.NewScrubber(home, names)
}

// timelineGame puts the game in the timeline when it changes; call with syncMu held.
func (s *Server) timelineGame(g store.Game, out state.Game) {
	key := ""
	if g.Picked() {
		key = gameKey(g)
	}
	line := key + "\x00" + out.Home.Name + "\x00" + out.Away.Name
	if line == s.lastGame {
		return
	}
	s.lastGame = line
	s.Timeline.Add("game", map[string]any{"key": key, "home": out.Home.Name, "away": out.Away.Name})
}
