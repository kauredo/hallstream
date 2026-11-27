package cable

import (
	"bytes"
	"strings"
)

// Bodet is the Scorepad's protocol output (Bodet manual 608264):
//
//	SOH address STX CTRL message ETX LRC
//
// The message opens with its number in two digits. The LRC is the XOR of
// the bytes after SOH up to ETX, & 0x7F, plus 32 when under 32. The manual's
// own shot-clock samples leave ETX out of it, so either form is accepted.
type Bodet struct {
	// shotSport is the sport of the last clock message: the shot clock
	// (message 50) carries none.
	shotSport string
}

func (*Bodet) Name() string { return "bodet" }
func (*Bodet) Baud() int    { return 9600 }

func (*Bodet) Split(buf []byte) (frames [][]byte, rest []byte) {
	for {
		i := bytes.IndexByte(buf, soh)
		if i < 0 {
			return frames, nil
		}
		buf = buf[i:]
		j := bytes.IndexByte(buf, etx)
		if j < 0 || j+1 >= len(buf) {
			return frames, keep(buf)
		}
		if f := buf[:j+2]; bodetValid(f) {
			frames = append(frames, append([]byte(nil), f...))
		}
		buf = buf[j+2:]
	}
}

func bodetValid(f []byte) bool {
	n := len(f)
	if n < 7 || f[2] != stx {
		return false
	}
	return f[n-1] == lrc(f[1:n-1]) || f[n-1] == lrc(f[1:n-2])
}

func lrc(b []byte) byte {
	var x byte
	for _, c := range b {
		x ^= c
	}
	x &= 0x7f
	if x < 32 {
		x += 32
	}
	return x
}

var bodetSports = map[byte]string{'5': "basketball", '4': "handball"}

func (b *Bodet) Decode(f []byte, sport string, vals map[string]string) bool {
	if sport != "basketball" && sport != "handball" {
		return false
	}
	m := f[4 : len(f)-2] // after SOH, address, STX, CTRL; before ETX, LRC
	if len(m) < 3 {
		return true
	}
	of := func(i int) bool { return len(m) > i && bodetSports[m[i]] == sport }
	switch string(m[:2]) {
	case "18": // basketball: clock, period
		if !of(3) || len(m) < 13 {
			return true
		}
		b.shotSport = sport
		vals["clock"] = bodetClock(m[4:8])
		vals["period"] = period(m[12], "5")
	case "01": // handball: clock, period
		if !of(3) || len(m) < 11 {
			return true
		}
		b.shotSport = sport
		vals["clock"] = bodetClock(m[4:8])
		vals["period"] = period(m[10], "3")
	case "30", "02": // basketball and handball scores, 3 characters each
		if !of(2) || len(m) < 9 {
			return true
		}
		vals["home"], vals["away"] = score(string(m[3:6])), score(string(m[6:9]))
	case "31": // basketball team fouls
		if !of(2) || len(m) < 7 {
			return true
		}
		vals["home_fouls"], vals["away_fouls"] = score(string(m[4])), score(string(m[6]))
	case "50": // shot clock: status word, then two characters
		if b.shotSport != "basketball" || sport != "basketball" || len(m) < 5 {
			return true
		}
		status := m[2]
		switch {
		case status&0x08 != 0: // blanked
			vals["shot_clock"] = ""
		case status&0x10 != 0: // seconds and tenths
			vals["shot_clock"] = string(m[3]) + "." + string(m[4])
		default:
			vals["shot_clock"] = seconds(string(m[3:5]))
		}
	default:
		return true
	}
	vals["sport"] = sport
	return true
}

// bodetClock reads "mmss", or "ssDt" in the last minute.
func bodetClock(c []byte) string {
	if c[2] == 'D' {
		return seconds(string(c[:2])) + "." + string(c[3])
	}
	return clock(string(c[:2]) + ":" + string(c[2:]))
}

// period is the period digit, or extra for 'E' (extra time).
func period(c byte, extra string) string {
	if c == 'E' {
		return extra
	}
	return strings.TrimSpace(string(rune(c)))
}
