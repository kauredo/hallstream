package store

import (
	"strings"
	"unicode"
)

// RosterPlayer is one player from the FPB roster.
type RosterPlayer struct {
	ID        int64
	AthleteID int64
	Name      string
	Number    string
	Games     int
	PhotoURL  string
}

type Suggestion struct {
	FPBPlayerID  int64   `json:"fpb_player_id"`
	FPBAthleteID int64   `json:"fpb_athlete_id"`
	Name         string  `json:"name"`
	Number       string  `json:"number"`
	Games        int     `json:"games"`
	PhotoURL     string  `json:"photo_url"`
	BestMatch    *string `json:"best_match"`
}

// Import merges an FPB roster into local players. Linked players get the FPB
// name, the FPB photo address and, unless the number was set by hand, the FPB
// number. display_name and the local photo are never touched; a new photo
// address alone does not count as an update. A linked player is found by
// player id, else by athlete id (next season's roster), and stops being
// inactive. FPB players linked to nobody come back as suggestions.
func Import(players []Player, roster []RosterPlayer) (updated []Player, suggestions []Suggestion) {
	byID := map[int64]RosterPlayer{}
	byAthlete := map[int64]RosterPlayer{}
	for _, r := range roster {
		byID[r.ID] = r
		if r.AthleteID != 0 {
			byAthlete[r.AthleteID] = r
		}
	}
	// Direct links claim their roster entry first, so an athlete-id match can
	// never give a player the id another one already has.
	claimed := map[int64]bool{}
	for _, p := range players {
		if p.FPBPlayerID != nil {
			claimed[*p.FPBPlayerID] = true
		}
	}
	linked := map[int64]bool{}
	for i := range players {
		p := &players[i]
		if p.FPBPlayerID == nil {
			continue
		}
		r, ok := byID[*p.FPBPlayerID]
		if !ok && p.FPBAthleteID != 0 {
			if a, found := byAthlete[p.FPBAthleteID]; found && !claimed[a.ID] {
				id := a.ID
				claimed[id] = true
				r, ok, p.FPBPlayerID = a, true, &id
			}
		}
		linked[*p.FPBPlayerID] = true
		if !ok {
			continue
		}
		if r.AthleteID != 0 {
			p.FPBAthleteID = r.AthleteID
		}
		p.Inactive = false
		p.PhotoSource = r.PhotoURL
		changed := false
		if p.FPBName == nil || *p.FPBName != r.Name {
			name := r.Name
			p.FPBName, changed = &name, true
		}
		if r.Number != "" && (p.Number == "" || p.NumberFromFPB) && p.Number != r.Number {
			p.Number, p.NumberFromFPB, changed = r.Number, true, true
		}
		if changed {
			updated = append(updated, *p)
		}
	}
	for _, r := range roster {
		if linked[r.ID] {
			continue
		}
		suggestions = append(suggestions, Suggestion{
			FPBPlayerID: r.ID, FPBAthleteID: r.AthleteID, Name: r.Name, Number: r.Number, Games: r.Games, PhotoURL: r.PhotoURL,
			BestMatch: BestMatch(r.Name, players),
		})
	}
	return updated, suggestions
}

// BestMatch picks the unlinked local player whose name looks most like name,
// ignoring accents, case and word order. nil when nothing is close.
func BestMatch(name string, players []Player) *string {
	want := words(name)
	var best *string
	bestScore := 0
	for i := range players {
		p := &players[i]
		if p.FPBPlayerID != nil {
			continue
		}
		have := words(p.DisplayName)
		common := 0
		for w := range have {
			if want[w] {
				common++
			}
		}
		// Every word of the shorter name, or at least two words, must match.
		short := min(len(have), len(want))
		if common == 0 || (common < short && common < 2) {
			continue
		}
		score := common*100 - abs(len(have)-len(want))
		if score > bestScore {
			bestScore, best = score, &p.ID
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var particles = map[string]bool{"de": true, "da": true, "do": true, "dos": true, "das": true, "e": true}

// words returns the set of plain lowercase words in a name.
func words(name string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(Plain(name), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if !particles[w] {
			out[w] = true
		}
	}
	return out
}

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// Plain lowercases and strips Portuguese accents.
func Plain(s string) string { return accents.Replace(strings.ToLower(s)) }

// Slug makes an id like "senior-m" from a name.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range Plain(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return b.String()
}
