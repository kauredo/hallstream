package cable

import (
	"encoding/hex"
	"fmt"
	"maps"
	"testing"
)

// dakFrame builds an All Sport 5000 data frame for offset with its checksum.
func dakFrame(offset int, data string) []byte {
	body := fmt.Sprintf("00000000\x01004210%04d\x02%s\x04", offset, data)
	var sum byte
	for i := 0; i < len(body); i++ {
		sum += body[i]
	}
	return []byte(fmt.Sprintf("\x16%s%02X\x17", body, sum))
}

// bodetFrame builds a Scorepad frame with the LRC over ETX included.
func bodetFrame(msg string) []byte {
	f := append([]byte{soh, 0x7f, stx, 'G'}, msg...)
	f = append(f, etx)
	return append(f, lrc(f[1:]))
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The first frames of raw-J5-output.txt, a real All Sport 5000 capture in
// github.com/JoshuaCarroll/scoredata (MIT).
const j5 = "16303030303030303001303034323130303030300231363a303931363a303920202031363a303931363a3039202020207320202004343917" +
	"16303030303030303001303034323130303030300231363a303931363a303920202031363a303931363a3039202020202020202004463617"

func TestDaktronicsSplitRealFrames(t *testing.T) {
	d := NewDaktronics()
	frames, rest := d.Split(unhex(t, j5))
	if len(frames) != 2 || len(rest) != 0 {
		t.Fatalf("frames %d, rest %q", len(frames), rest)
	}
	vals := map[string]string{}
	d.Decode(frames[0], "basketball", vals)
	if vals["clock"] != "16:09" {
		t.Errorf("clock %q", vals["clock"])
	}
}

func TestDaktronicsSplitDropsBadAndKeepsPartial(t *testing.T) {
	d := NewDaktronics()
	good := dakFrame(107, "  12")
	bad := dakFrame(111, "  34")
	bad[len(bad)-2] = 'Z'
	in := append([]byte("noise\x16\x16\x16"), bad...)
	in = append(in, good...)
	in = append(in, good[:20]...)
	frames, rest := d.Split(in)
	if len(frames) != 1 || string(frames[0]) != string(good) {
		t.Fatalf("frames %q", frames)
	}
	frames, rest = d.Split(append(rest, good[20:]...))
	if len(frames) != 1 || len(rest) != 0 {
		t.Fatalf("after the rest: %d frames, rest %q", len(frames), rest)
	}
}

func TestDaktronicsRestIsCapped(t *testing.T) {
	_, rest := NewDaktronics().Split(append([]byte{syn}, make([]byte, 3*maxRest)...))
	if len(rest) != maxRest {
		t.Errorf("rest %d", len(rest))
	}
}

func TestDaktronicsBasketball(t *testing.T) {
	d := NewDaktronics()
	vals := map[string]string{}
	for _, f := range [][]byte{
		dakFrame(0, " 9:59"),
		dakFrame(107, "  52  48"), // offset 107 is item 108, home score
		dakFrame(141, " 3"),
		dakFrame(200, "   00:14"),
		dakFrame(235, " 4 5"),
		dakFrame(111, " 101"), // offset 0111 lands on the guest score
	} {
		frames, _ := d.Split(f)
		if len(frames) != 1 || !d.Decode(frames[0], "basketball", vals) {
			t.Fatalf("frame %q", f)
		}
	}
	want := map[string]string{"clock": "9:59", "home": "52", "away": "101", "period": "3",
		"shot_clock": "14", "home_fouls": "4", "away_fouls": "5", "sport": "basketball"}
	if !maps.Equal(vals, want) {
		t.Errorf("got %v\nwant %v", vals, want)
	}
}

func TestShotClock(t *testing.T) {
	for in, want := range map[string]string{"   00:24": "24", "    0:09": "9", "     4.5": "4.5", "        ": "", " 1:00   ": "60"} {
		if got := shot(in); got != want {
			t.Errorf("shot(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDaktronicsVolleyball(t *testing.T) {
	d := NewDaktronics()
	vals := map[string]string{}
	for _, f := range [][]byte{dakFrame(107, "  21  19"), dakFrame(141, " 4"), dakFrame(200, "<      "), dakFrame(214, " 2 1")} {
		frames, _ := d.Split(f)
		d.Decode(frames[0], "volleyball", vals)
	}
	want := map[string]string{"home": "21", "away": "19", "period": "4", "home_sets": "2", "away_sets": "1", "serve": "home", "sport": "volleyball"}
	if !maps.Equal(vals, want) {
		t.Errorf("got %v\nwant %v", vals, want)
	}
	frames, _ := d.Split(dakFrame(200, "       >"))
	d.Decode(frames[0], "volleyball", vals)
	if vals["serve"] != "away" {
		t.Errorf("serve %q", vals["serve"])
	}
}

func TestDaktronicsOtherSportKeepsTheBuffer(t *testing.T) {
	d := NewDaktronics()
	vals := map[string]string{}
	frames, _ := d.Split(dakFrame(107, "   7"))
	if d.Decode(frames[0], "futsal", vals) || len(vals) != 0 {
		t.Fatalf("futsal decoded %v", vals)
	}
	frames, _ = d.Split(dakFrame(141, " 1"))
	d.Decode(frames[0], "basketball", vals)
	if vals["home"] != "7" {
		t.Errorf("home %q: the futsal frame was not kept", vals["home"])
	}
}

func TestDaktronicsSkipsOtherHeaders(t *testing.T) {
	d := NewDaktronics()
	body := "00000000\x01003010006\x04"
	var sum byte
	for i := 0; i < len(body); i++ {
		sum += body[i]
	}
	frames, _ := d.Split([]byte(fmt.Sprintf("\x16%s%02X\x17", body, sum)))
	vals := map[string]string{}
	if len(frames) != 1 || !d.Decode(frames[0], "basketball", vals) || vals["home"] != "0" {
		t.Errorf("frames %d, vals %v", len(frames), vals)
	}
}

func TestBodetManualSample(t *testing.T) {
	// Revision A, page 18: a shot clock frame whose LRC leaves ETX out.
	b := &Bodet{}
	frames, rest := b.Split(unhex(t, "017f02473530803234033901"))
	if len(frames) != 1 || len(rest) != 1 {
		t.Fatalf("frames %d, rest %q", len(frames), rest)
	}
	vals := map[string]string{}
	b.Decode(bodetFrame("18\x805165413  2 "), "basketball", vals) // the clock first: message 50 has no sport
	b.Decode(frames[0], "basketball", vals)
	if vals["shot_clock"] != "24" {
		t.Errorf("shot clock %q", vals["shot_clock"])
	}
}

func TestBodetBasketball(t *testing.T) {
	b := &Bodet{}
	vals := map[string]string{}
	in := []byte("garbage")
	for _, m := range []string{
		"18\x805165413  2 ", // 16:54, period 2
		"305 12 18 ",        // 12 - 18, padded on either side
		"315 3 5 071",       // fouls 3 and 5
		"50\x90" + "45",     // status b4: 4.5
		"18\x80556D41300E ", // 56.4 in extra time
		"305104118",
	} {
		in = append(in, bodetFrame(m)...)
	}
	frames, _ := b.Split(in)
	for _, f := range frames {
		b.Decode(f, "basketball", vals)
	}
	want := map[string]string{"clock": "56.4", "period": "5", "home": "104", "away": "118",
		"home_fouls": "3", "away_fouls": "5", "shot_clock": "4.5", "sport": "basketball"}
	if len(frames) != 6 || !maps.Equal(vals, want) {
		t.Errorf("%d frames, got %v\nwant %v", len(frames), vals, want)
	}
	b.Decode(bodetFrame("50\x88  "), "basketball", vals)
	if vals["shot_clock"] != "" {
		t.Errorf("blanked shot clock %q", vals["shot_clock"])
	}
}

func TestBodetHandball(t *testing.T) {
	b := &Bodet{}
	vals := map[string]string{}
	for _, m := range []string{"01\x8040912312", "024021019", "305 12 18 ", "01\x804300012E"} {
		b.Decode(bodetFrame(m), "handball", vals)
	}
	want := map[string]string{"clock": "30:00", "period": "3", "home": "21", "away": "19", "sport": "handball"}
	if !maps.Equal(vals, want) {
		t.Errorf("got %v\nwant %v", vals, want)
	}
	if b.Decode(bodetFrame("024021019"), "volleyball", vals) {
		t.Error("volleyball has a layout")
	}
}

func TestBodetDropsBadLRC(t *testing.T) {
	f := bodetFrame("305 12 18 ")
	f[len(f)-1] ^= 0x01
	if frames, _ := (&Bodet{}).Split(f); len(frames) != 0 {
		t.Errorf("kept a bad frame")
	}
}

func TestGet(t *testing.T) {
	if Get("bodet").Name() != "bodet" || Get("x").Name() != "daktronics" || Get("daktronics") == Get("daktronics") {
		t.Error("Get")
	}
}

// Atoi reads a sign, so a frame whose checksum holds can carry offset "-001".
func TestDaktronicsSignedOffset(t *testing.T) {
	d := NewDaktronics()
	for _, head := range []string{"-001", "+107"} {
		body := "00000000\x01004210" + head + "\x02  99\x04"
		var sum byte
		for i := 0; i < len(body); i++ {
			sum += body[i]
		}
		frames, _ := d.Split([]byte(fmt.Sprintf("\x16%s%02X\x17", body, sum)))
		if len(frames) != 1 {
			t.Fatalf("%s: %d frames", head, len(frames))
		}
		d.Decode(frames[0], "basketball", map[string]string{}) // must not panic
	}
}

func TestShotClockUnderASecond(t *testing.T) {
	for in, want := range map[string]string{"    00.5": "0.5", "     0.0": "0.0", "   04.5": "4.5", " 0:09.8": "9.8"} {
		if got := shot(in); got != want {
			t.Errorf("shot(%q) = %q, want %q", in, got, want)
		}
	}
}

// Noise holding a SYN with no end in front of a good frame must not swallow it.
func TestDaktronicsStraySYN(t *testing.T) {
	good := dakFrame(107, "  12")
	frames, _ := NewDaktronics().Split(append([]byte("\x16000\x01junk"), good...))
	if len(frames) != 1 || string(frames[0]) != string(good) {
		t.Fatalf("frames %q", frames)
	}
}

// Revision J of the Bodet manual sends 'O' for basketball extra time; revision A sent 'E'.
func TestBodetExtraTimeLetters(t *testing.T) {
	for _, m := range []string{"18\x80556D41300E ", "18\x80556D41300O "} {
		vals := map[string]string{}
		(&Bodet{}).Decode(bodetFrame(m), "basketball", vals)
		if vals["period"] != "5" {
			t.Errorf("%q: period %q, want 5", m, vals["period"])
		}
	}
}
