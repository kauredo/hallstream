package provider

import (
	"testing"
	"time"

	"github.com/kauredo/hallstream/internal/store"
)

var madrid, _ = time.LoadLocation("Europe/Madrid")

const feed = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:a1@example\r\nDTSTART:20301220T173000Z\r\nSUMMARY:CB Vale vs Ribeira\r\n  Basket\r\nLOCATION:Pabellón Municipal\\, Vale\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a2@example\r\nDTSTART;TZID=America/New_York:20301221T190000\r\nSUMMARY:at Harbor Hawks\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a3@example\r\nDTSTART:20301222T120000\r\nSUMMARY:Norte - CB Vale\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a4@example\r\nDTSTART;VALUE=DATE:20301223\r\nSUMMARY:Torneio\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a5@example\r\nDTSTART:20301224T120000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Training\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a6@example\r\nDTSTART:20301225T120000Z\r\nSTATUS:CANCELLED\r\nSUMMARY:Off\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseICS(t *testing.T) {
	evs := ParseICS([]byte(feed), madrid)
	if len(evs) != 4 {
		t.Fatalf("%d events, want 4 (no recurring, no cancelled): %+v", len(evs), evs)
	}
	if e := evs[0]; e.UID != "a1@example" || !e.Start.Equal(time.Date(2030, 12, 20, 17, 30, 0, 0, time.UTC)) || !e.Timed ||
		e.Summary != "CB Vale vs Ribeira Basket" || e.Location != "Pabellón Municipal, Vale" {
		t.Errorf("UTC event, folded and escaped: %+v", e)
	}
	if e := evs[1]; !e.Start.Equal(time.Date(2030, 12, 22, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("TZID event at %v, want 19:00 New York", e.Start.UTC())
	}
	if e := evs[2]; !e.Start.Equal(time.Date(2030, 12, 22, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("floating event at %v, want 12:00 Madrid", e.Start.UTC())
	}
	if e := evs[3]; e.Timed {
		t.Errorf("an all-day event counts as timed: %+v", e)
	}
}

func TestMatchup(t *testing.T) {
	for _, c := range []struct {
		summary, opp string
		home         bool
	}{
		{"CB Vale vs Ribeira Basket", "Ribeira Basket", true},
		{"Ribeira Basket vs. CB Vale", "Ribeira Basket", false},
		{"Norte - CB Vale", "Norte", false},
		{"CB Vale x Sul", "Sul", true},
		{"Harbor Hawks @ CB Vale", "Harbor Hawks", true},
		{"CB Vale at Harbor Hawks", "Harbor Hawks", false},
		{"at Harbor Hawks", "Harbor Hawks", false},
		{"Game vs Harbor Hawks", "Harbor Hawks", true},
		{"@ Harbor Hawks", "Harbor Hawks", false},
		{"Torneio de Natal", "Torneio de Natal", true},
	} {
		opp, home := Matchup(c.summary, "CB Vale", "CBV")
		if opp != c.opp || home != c.home {
			t.Errorf("Matchup(%q) = %q, %v; want %q, %v", c.summary, opp, home, c.opp, c.home)
		}
	}
}

func TestCalendarProvider(t *testing.T) {
	c := Calendar{
		Fetch: func(string) ([]byte, error) { return []byte(feed), nil },
		Club:  func() store.Club { return store.Club{Name: "CB Vale", Short: "CBV"} },
		Zone:  func() *time.Location { return madrid },
	}
	if got, err := c.Fixtures(store.Group{}); got != nil || err != nil {
		t.Errorf("a group without a calendar: %v %v", got, err)
	}
	got, err := c.Fixtures(store.Group{CalendarURL: "https://example.com/team.ics"})
	if err != nil || len(got) != 4 {
		t.Fatalf("%d fixtures, %v", len(got), err)
	}
	if f := got[0]; f.Ref != "ics/a1@example" || f.Opponent.Name != "Ribeira Basket" || !f.AtHome || f.Venue != "Pabellón Municipal, Vale" {
		t.Errorf("first fixture %+v", f)
	}
	if f := got[2]; f.AtHome || f.Opponent.Name != "Norte" {
		t.Errorf("away fixture %+v", f)
	}
}
