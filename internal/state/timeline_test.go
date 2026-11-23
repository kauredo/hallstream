package state

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestTimelineEvents(t *testing.T) {
	m, c := newTest()
	var got []string
	m.OnEvent = func(ev string, f map[string]any) {
		switch ev {
		case "screen":
			got = append(got, fmt.Sprintf("screen %v auto=%v", f["name"], f["auto"]))
		case "lower_third":
			got = append(got, fmt.Sprintf("lower_third %v %v %v", f["shown"], f["type"], f["number"]))
		case "box":
			got = append(got, fmt.Sprintf("box %v", f["connected"]))
		case "period":
			got = append(got, fmt.Sprintf("period %v %v-%v", f["period"], f["home"], f["away"]))
		default:
			got = append(got, ev)
		}
	}
	// Restored periods were recorded before the restart: no line.
	m.RestorePeriods(nil)
	m.SetBox(true)
	m.SetScreen("abertura", true)
	m.ShowLowerThird(LowerThird{Type: "jogador", Number: "7", FirstName: "Nome", LastName: "Apelido"}, 8*time.Second)
	m.HideLowerThird()
	run(t, m, c, []step{
		{0, "1", "10:00", 0, 0, "abertura", 0},
		{1, "1", "00:00", 20, 18, "abertura", 0},
		{1, "2", "10:00", 20, 18, "", 1},
		{1, "2", "00:00", 40, 41, "", 1},
		{1, "3", "10:00", 40, 41, "intervalo", 2},
	})
	m.SetDemo(true) // the demo is no game
	m.SetScreen("plantel", true)
	m.SetDemo(false)
	m.SetBox(false)
	want := []string{
		"box true",
		"screen abertura auto=false",
		"lower_third true jogador 7",
		"lower_third false jogador 7",
		"period 1 20-18",
		"screen intervalo auto=true",
		"period 2 40-41",
		"box false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}
