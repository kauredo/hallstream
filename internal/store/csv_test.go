package store

import (
	"testing"
	"time"
)

func TestParseSquad(t *testing.T) {
	for name, in := range map[string]string{
		"comma, header":      "numero,nome,foto\n7,Hélio Vaz,\n12,Rui Mota,https://exemplo.pt/rui.jpg\n",
		"semicolon, no head": "7;Hélio Vaz\n\n12;Rui Mota;https://exemplo.pt/rui.jpg\n",
		"bom and crlf":       "\xef\xbb\xbfNº;Nome;Foto\r\n7;Hélio Vaz;\r\n12;Rui Mota;https://exemplo.pt/rui.jpg\r\n",
		"windows-1252":       "7;H\xe9lio Vaz\n12;Rui Mota;https://exemplo.pt/rui.jpg\n",
	} {
		rows, bad, err := ParseSquad([]byte(in))
		if err != nil || len(bad) != 0 || len(rows) != 2 {
			t.Fatalf("%s: %+v %+v %v", name, rows, bad, err)
		}
		if rows[0].Number != "7" || rows[0].Name != "Hélio Vaz" || rows[1].Photo != "https://exemplo.pt/rui.jpg" {
			t.Errorf("%s: %+v", name, rows)
		}
	}
	rows, bad, _ := ParseSquad([]byte(",Ana Lima\n7,\nabc,Rui\n9,Tó,ficheiro.jpg\n"))
	if len(rows) != 1 || rows[0].Name != "Ana Lima" || rows[0].Number != "" {
		t.Errorf("rows %+v", rows)
	}
	if len(bad) != 3 || bad[0].Line != 2 || bad[1].Line != 3 || bad[2].Line != 4 {
		t.Errorf("bad lines %+v", bad)
	}
}

func TestMergeSquad(t *testing.T) {
	fpb := int64(5)
	players := []Player{
		{ID: "a", DisplayName: "Hélio Vaz", Number: "4", NumberFromFPB: true},
		{ID: "b", DisplayName: "Rui Mota", Number: "12", FPBPlayerID: &fpb, PhotoSource: "https://fpb/x.jpg"},
	}
	out, added, updated := MergeSquad(players, []SquadRow{
		{Number: "7", Name: "helio vaz"},
		{Number: "12", Name: "Rui Mota", Photo: "https://outro/y.jpg"},
		{Number: "9", Name: "Ana Lima", Photo: "https://exemplo.pt/ana.jpg"},
	})
	if added != 1 || updated != 1 || len(out) != 3 {
		t.Fatalf("added %d updated %d: %+v", added, updated, out)
	}
	if out[0].Number != "7" || out[0].NumberFromFPB {
		t.Errorf("number by hand: %+v", out[0])
	}
	if out[1].PhotoSource != "https://fpb/x.jpg" {
		t.Errorf("an FPB player's photo changed: %+v", out[1])
	}
	if out[2].DisplayName != "Ana Lima" || out[2].ID == "" || out[2].PhotoSource == "" {
		t.Errorf("new player %+v", out[2])
	}
	if players[0].Number != "4" {
		t.Error("the players passed in changed")
	}
}

func TestParseFixtures(t *testing.T) {
	lisbon, _ := time.LoadLocation("Europe/Lisbon")
	in := "data;hora;adversario;casa_fora;local;competicao;jornada\n" +
		"20/12/2026;18h30;Ribeira Basket;fora;Pavilhão da Ribeira;Regional;7\n" +
		"2027-01-09;11:00;GD Monte;C\n" +
		"31/02/2027;10:00;Ninguém;casa\n" +
		"10/01/2027;;Alguém;casa\n" +
		"11/01/2027;10:00;;casa\n" +
		"12/01/2027;10:00;Outro;talvez\n"
	rows, bad, err := ParseFixtures([]byte(in), lisbon)
	if err != nil || len(rows) != 2 || len(bad) != 4 {
		t.Fatalf("%+v %+v %v", rows, bad, err)
	}
	f := rows[0].Fixture
	if f.Start != "2026-12-20T18:30:00Z" || f.Home || f.Opponent.Name != "Ribeira Basket" || f.Venue != "Pavilhão da Ribeira" || f.Round != "7" {
		t.Errorf("first %+v", f)
	}
	if g := rows[1].Fixture; g.Start != "2027-01-09T11:00:00Z" || !g.Home {
		t.Errorf("second %+v", g)
	}
}

func TestValidFixture(t *testing.T) {
	f := Fixture{Start: "2026-12-20T18:30:00Z", Opponent: Opponent{Name: "  Ribeira   Basket ", Short: "rib"}}
	if err := ValidFixture(&f); err != nil || f.Opponent.Name != "Ribeira Basket" || f.Opponent.Short != "RIB" {
		t.Errorf("%v %+v", err, f)
	}
	for _, bad := range []Fixture{{Start: "amanhã", Opponent: Opponent{Name: "X"}}, {Start: "2026-12-20T18:30:00Z"}, {Start: "2026-12-20T18:30:00Z", Opponent: Opponent{Name: "X", Short: "ABCDE"}}} {
		if ValidFixture(&bad) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
