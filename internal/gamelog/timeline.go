// Package gamelog keeps dados/timeline.jsonl and sends each stream's logs to
// the club's server (fpb-calendar, POST /api/marcador/logs).
package gamelog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// TimeLayout is the timeline's ts: RFC 3339 with milliseconds and the local offset.
const TimeLayout = "2006-01-02T15:04:05.000Z07:00"

// rawLayout is raw.log's stamp: local time, no zone.
const rawLayout = "2006-01-02T15:04:05.000"

// Timeline appends one JSON line per change the video-stats side can't get
// from raw.log. A nil Timeline drops everything.
type Timeline struct {
	mu  sync.Mutex
	w   io.Writer
	Now func() time.Time
}

func OpenTimeline(path string) (*Timeline, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return NewTimeline(f), nil
}

func NewTimeline(w io.Writer) *Timeline { return &Timeline{w: w, Now: time.Now} }

// Add writes {"ts", "event", ...fields}.
func (t *Timeline) Add(event string, fields map[string]any) {
	if t == nil {
		return
	}
	t.AddAt(t.Now(), event, fields)
}

// AddAt is Add stamped at.
func (t *Timeline) AddAt(at time.Time, event string, fields map[string]any) {
	if t == nil {
		return
	}
	head, _ := json.Marshal(map[string]string{"ts": at.Format(TimeLayout)})
	ev, _ := json.Marshal(event)
	line := append(head[:len(head)-1], `,"event":`...)
	line = append(line, ev...)
	if rest, err := json.Marshal(fields); err == nil && len(fields) > 0 {
		line = append(line, ',')
		line = append(line, rest[1:len(rest)-1]...)
	}
	line = append(line, "}\n"...)
	t.mu.Lock()
	t.w.Write(line)
	t.mu.Unlock()
}

// Cut copies the lines of r stamped from from to to, both included. Lines
// without a readable stamp are skipped.
func Cut(w io.Writer, r io.Reader, from, to time.Time, stamp func([]byte) (time.Time, bool)) error {
	return cut(w, r, from, to, stamp, false)
}

// CutFollow is Cut, but a line without a stamp goes with the line before it
// (a message written over several lines).
func CutFollow(w io.Writer, r io.Reader, from, to time.Time, stamp func([]byte) (time.Time, bool)) error {
	return cut(w, r, from, to, stamp, true)
}

func cut(w io.Writer, r io.Reader, from, to time.Time, stamp func([]byte) (time.Time, bool), follow bool) error {
	br := bufio.NewReaderSize(r, 64<<10)
	keep := false
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			t, ok := stamp(line)
			if ok {
				keep = !t.Before(from) && !t.After(to)
			} else if !follow {
				keep = false
			}
			if keep {
				if !bytes.HasSuffix(line, []byte("\n")) {
					line = append(line, '\n')
				}
				if _, werr := w.Write(line); werr != nil {
					return werr
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// RawStamp reads a raw.log line: "2006-01-02T15:04:05.000 <hex>".
func RawStamp(line []byte) (time.Time, bool) { return localStamp(line, rawLayout) }

// LogStamp reads a marcador.log line: "2006-01-02 15:04:05 <message>".
func LogStamp(line []byte) (time.Time, bool) { return localStamp(line, "2006-01-02 15:04:05") }

// BluetoothStamp reads a bluetooth.log line: "2006-01-02T15:04:05 <address> ...".
func BluetoothStamp(line []byte) (time.Time, bool) { return localStamp(line, "2006-01-02T15:04:05") }

// localStamp reads a local time in layout at the start of line, followed by a space.
func localStamp(line []byte, layout string) (time.Time, bool) {
	if len(line) < len(layout)+1 || line[len(layout)] != ' ' {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(layout, string(line[:len(layout)]), time.Local)
	return t, err == nil
}

// TimelineStamp reads the ts of a timeline line.
func TimelineStamp(line []byte) (time.Time, bool) {
	var v struct {
		TS string `json:"ts"`
	}
	if json.Unmarshal(line, &v) != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v.TS)
	return t, err == nil
}
