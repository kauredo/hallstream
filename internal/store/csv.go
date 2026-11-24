package store

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kauredo/hallstream/internal/i18n"
)

// SquadRow is one line of a squad CSV: number,name,photo.
type SquadRow struct {
	Line   int
	Number string
	Name   string
	Photo  string // an http(s) address, or ""
}

// LineError is a CSV line that was left out, and why.
type LineError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// csvRecords reads a CSV a club made in a spreadsheet: comma or semicolon
// (Portuguese Excel writes ;), an optional BOM, blank lines skipped. Each
// record keeps its line number.
func csvRecords(data []byte) ([][]string, []int, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		data = latin1(data) // Excel on Windows saves "CSV" in Windows-1252
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	var recs [][]string
	var lines []int
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, i18n.Err("csv.ilegivel", "err", err)
		}
		line, _ := r.FieldPos(0)
		if blank(rec) {
			continue
		}
		recs, lines = append(recs, rec), append(lines, line)
	}
	return recs, lines, nil
}

func blank(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

func latin1(b []byte) []byte {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return []byte(string(r))
}

func field(rec []string, i int) string {
	if i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}

// header reports whether the first record names its columns: its first
// field is one of the names a club would give the column.
func header(rec []string) bool {
	switch strings.Trim(Plain(field(rec, 0)), " .º°#") {
	case "numero", "n", "no", "num", "number", "data", "date", "dia":
		return true
	}
	return false
}

var shirt = regexp.MustCompile(`^\d{1,3}$`)

// ParseSquad reads number,name,photo lines. A first line with no number in
// it is a header. A line without a name, or with a number that is not 0-999,
// is left out with its reason; the number may be empty.
func ParseSquad(data []byte) ([]SquadRow, []LineError, error) {
	recs, lines, err := csvRecords(data)
	if err != nil {
		return nil, nil, err
	}
	if len(recs) > 0 && header(recs[0]) {
		recs, lines = recs[1:], lines[1:]
	}
	var rows []SquadRow
	var bad []LineError
	for i, rec := range recs {
		row := SquadRow{Line: lines[i], Number: field(rec, 0), Name: strings.Join(strings.Fields(field(rec, 1)), " "), Photo: field(rec, 2)}
		switch {
		case row.Name == "":
			bad = append(bad, LineError{row.Line, i18n.T("csv.sem_nome")})
		case row.Number != "" && !shirt.MatchString(row.Number):
			bad = append(bad, LineError{row.Line, i18n.T("csv.numero", "numero", row.Number)})
		case row.Photo != "" && !strings.HasPrefix(row.Photo, "http://") && !strings.HasPrefix(row.Photo, "https://"):
			bad = append(bad, LineError{row.Line, i18n.T("csv.foto")})
		default:
			rows = append(rows, row)
		}
	}
	return rows, bad, nil
}

// MergeSquad adds the rows to players: a row whose name matches a player
// (ignoring case and accents) updates that player's number, and a photo
// address the player had not, the others are new players. Nobody is removed.
// It returns the players and how many were added and updated.
func MergeSquad(players []Player, rows []SquadRow) (out []Player, added, updated int) {
	out = append([]Player(nil), players...)
	byName := map[string]int{}
	for i, p := range out {
		byName[Plain(p.DisplayName)] = i
	}
	for _, r := range rows {
		if i, ok := byName[Plain(r.Name)]; ok {
			p := &out[i]
			changed := false
			if r.Number != "" && r.Number != p.Number {
				p.Number, p.NumberFromFPB, changed = r.Number, false, true
			}
			if r.Photo != "" && r.Photo != p.PhotoSource && p.FPBPlayerID == nil && !p.PhotoByHand {
				p.PhotoSource, changed = r.Photo, true
			}
			if changed {
				updated++
			}
			continue
		}
		out = append(out, Player{ID: NewID(), DisplayName: r.Name, Number: r.Number, PhotoSource: r.Photo})
		byName[Plain(r.Name)] = len(out) - 1
		added++
	}
	return out, added, updated
}

// FixtureRow is one line of a fixtures CSV:
// data,hora,adversario,casa_fora,local,competicao,jornada.
type FixtureRow struct {
	Line    int
	Fixture Fixture
}

// ParseFixtures reads fixture lines: the date as 2026-11-20 or 20/11/2026,
// the time as 18:30 or 18h30, the opponent's name, "casa" or "fora" (C/F,
// home/away), then the venue, competition and round, which may be empty.
func ParseFixtures(data []byte, loc *time.Location) ([]FixtureRow, []LineError, error) {
	recs, lines, err := csvRecords(data)
	if err != nil {
		return nil, nil, err
	}
	if len(recs) > 0 && header(recs[0]) {
		recs, lines = recs[1:], lines[1:]
	}
	var rows []FixtureRow
	var bad []LineError
	for i, rec := range recs {
		start, ok := fixtureAt(field(rec, 0), field(rec, 1), loc)
		name := strings.Join(strings.Fields(field(rec, 2)), " ")
		home, okSide := side(field(rec, 3))
		switch {
		case !ok:
			bad = append(bad, LineError{lines[i], i18n.T("csv.data", "data", strings.TrimSpace(field(rec, 0)+" "+field(rec, 1)))})
		case name == "":
			bad = append(bad, LineError{lines[i], i18n.T("csv.sem_adversario")})
		case !okSide:
			bad = append(bad, LineError{lines[i], i18n.T("csv.casa_fora", "valor", field(rec, 3))})
		default:
			rows = append(rows, FixtureRow{lines[i], Fixture{ID: NewID(), Start: start.Format(time.RFC3339),
				Opponent: Opponent{Name: name}, Home: home,
				Venue: field(rec, 4), Competition: field(rec, 5), Round: field(rec, 6)}})
		}
	}
	return rows, bad, nil
}

var (
	isoDate = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	ptDate  = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$`)
	hhmm    = regexp.MustCompile(`^(\d{1,2})\s*[:hH]\s*(\d{2})$`)
)

func fixtureAt(date, hour string, loc *time.Location) (time.Time, bool) {
	var y, m, d string
	if g := isoDate.FindStringSubmatch(date); g != nil {
		y, m, d = g[1], g[2], g[3]
	} else if g := ptDate.FindStringSubmatch(date); g != nil {
		y, m, d = g[3], g[2], g[1]
	} else {
		return time.Time{}, false
	}
	c := hhmm.FindStringSubmatch(hour)
	if c == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-1-2 15:4", y+"-"+m+"-"+d+" "+c[1]+":"+c[2], loc)
	return t, err == nil
}

func side(s string) (home, ok bool) {
	switch Plain(strings.TrimSpace(s)) {
	case "casa", "c", "home", "h", "":
		return true, true
	case "fora", "f", "away", "a", "visitante":
		return false, true
	}
	return false, false
}

// ValidFixture checks a fixture typed in the panel: a start, an opponent.
func ValidFixture(f *Fixture) error {
	f.Opponent.Name = strings.Join(strings.Fields(f.Opponent.Name), " ")
	f.Opponent.Short = strings.ToUpper(strings.TrimSpace(f.Opponent.Short))
	if _, err := time.Parse(time.RFC3339, f.Start); err != nil {
		return i18n.Key("calendario.data")
	}
	if f.Opponent.Name == "" {
		return i18n.Key("csv.sem_adversario")
	}
	if utf8.RuneCountInString(f.Opponent.Short) > 4 {
		return i18n.Key("calendario.sigla")
	}
	return nil
}
