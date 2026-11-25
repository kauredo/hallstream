package provider

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/store"
)

// Event is one VEVENT of an iCalendar feed (RFC 5545), the parts a fixture needs.
type Event struct {
	UID      string
	Start    time.Time
	Timed    bool // false for an all-day event (DTSTART;VALUE=DATE)
	Summary  string
	Location string
}

// ParseICS reads the events of an iCalendar feed. A start in UTC ("…Z") is
// taken as is, one with a TZID Go knows is read in that zone, and a floating
// one (no zone, or a TZID Go doesn't know) is read in zone, the club's.
// Events with no start, cancelled ones and recurring ones (RRULE) are left out.
func ParseICS(data []byte, zone *time.Location) []Event {
	var out []Event
	var ev *Event
	skip := false
	for _, line := range unfold(data) {
		name, params, value := property(line)
		switch {
		case name == "BEGIN" && value == "VEVENT":
			ev, skip = &Event{}, false
		case name == "END" && value == "VEVENT":
			if ev != nil && !skip && !ev.Start.IsZero() {
				out = append(out, *ev)
			}
			ev = nil
		case ev == nil:
		case name == "UID":
			ev.UID = value
		case name == "SUMMARY":
			ev.Summary = text(value)
		case name == "LOCATION":
			ev.Location = text(value)
		case name == "DTSTART":
			ev.Start, ev.Timed = icsTime(value, params, zone)
		case name == "RRULE":
			skip = true
		case name == "STATUS" && strings.EqualFold(value, "CANCELLED"):
			skip = true
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// unfold joins the continuation lines (a leading space or tab) to the line before.
func unfold(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	var lines []string
	for _, l := range strings.Split(string(data), "\n") {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// property splits "DTSTART;TZID=Europe/Madrid:20261220T183000" into its name,
// parameters and value. A colon inside a quoted parameter value is kept.
func property(line string) (string, map[string]string, string) {
	quoted, cut := false, -1
	for i, r := range line {
		if r == '"' {
			quoted = !quoted
		}
		if r == ':' && !quoted {
			cut = i
			break
		}
	}
	if cut < 0 {
		return "", nil, ""
	}
	head, value := line[:cut], strings.TrimSpace(line[cut+1:])
	parts := strings.Split(head, ";")
	params := map[string]string{}
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(p, "="); ok {
			params[strings.ToUpper(k)] = strings.Trim(v, `"`)
		}
	}
	return strings.ToUpper(parts[0]), params, value
}

// text undoes the TEXT escapes: \n, \, \; and \\.
func text(s string) string {
	r := strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`)
	return strings.Join(strings.Fields(r.Replace(s)), " ")
}

func icsTime(value string, params map[string]string, zone *time.Location) (time.Time, bool) {
	if params["VALUE"] == "DATE" || len(value) == 8 {
		t, err := time.ParseInLocation("20060102", value, zone)
		if err != nil {
			return time.Time{}, false
		}
		return t, false
	}
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		return t, err == nil
	}
	loc := zone
	if tz := params["TZID"]; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	t, err := time.ParseInLocation("20060102T150405", value, loc)
	return t, err == nil
}

// Matchup splits a game's title into the opponent and whether the club plays
// at home, from the shapes calendars use: "Club vs Rival", "Club - Rival",
// "Club x Rival" (the first team at home), "Rival @ Club", and the team apps'
// "vs Rival" or "at Rival". names are the club's name and short name; when
// neither appears, the whole title is the opponent and the club is at home.
func Matchup(summary string, names ...string) (opponent string, home bool) {
	s := strings.TrimSpace(summary)
	if m := teamApp.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[2]), !strings.EqualFold(m[1], "at") && m[1] != "@"
	}
	for _, sep := range []string{" @ ", " at "} {
		if a, b, ok := cutFold(s, sep); ok {
			if isClub(a, names) {
				return b, false
			}
			return a, true // "Rival @ Club"
		}
	}
	for _, sep := range []string{" vs. ", " vs ", " v ", " x ", " - ", " – "} {
		if a, b, ok := cutFold(s, sep); ok {
			if isClub(b, names) && !isClub(a, names) {
				return a, false
			}
			return b, true
		}
	}
	return s, true
}

// teamApp is TeamSnap's and SportsEngine's "vs Rival" / "at Rival" / "@ Rival".
var teamApp = regexp.MustCompile(`(?i)^(?:game\s+)?(vs\.?|at|@)\s+(.+)$`)

func cutFold(s, sep string) (string, string, bool) {
	i := strings.Index(strings.ToLower(s), strings.ToLower(sep))
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):]), true
}

func isClub(side string, names []string) bool {
	side = strings.ToLower(side)
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" && strings.Contains(side, n) {
			return true
		}
	}
	return false
}

// Calendar is an escalão's calendar link (store.Group.CalendarURL) as a
// provider. Fetch returns the feed, cached; Club gives the club's names and zone.
type Calendar struct {
	Fetch func(url string) ([]byte, error)
	Club  func() store.Club
	Zone  func() *time.Location
}

func (Calendar) ID() string { return "ics" }

func (c Calendar) Fixtures(g store.Group) ([]Fixture, error) {
	if g.CalendarURL == "" || c.Fetch == nil {
		return nil, nil
	}
	data, err := c.Fetch(g.CalendarURL)
	if err != nil {
		return nil, err
	}
	club := c.Club()
	var out []Fixture
	for i, ev := range ParseICS(data, c.Zone()) {
		opp, home := Matchup(ev.Summary, club.Name, club.Short)
		uid := ev.UID
		if uid == "" {
			uid = ev.Start.UTC().Format("20060102T150405") + "-" + strconv.Itoa(i)
		}
		out = append(out, Fixture{
			Ref: "ics/" + uid, Start: ev.Start, Timed: ev.Timed, AtHome: home,
			Opponent: Side{Name: opp, FullName: opp},
			Venue:    ev.Location,
		})
	}
	return out, nil
}
