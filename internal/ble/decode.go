package ble

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

// Frame layout taken from the SL Live Stream Android app v1.8
// (MainActivity.displayData). Every notification is 20 bytes and the first
// byte says what it holds.

var sports = map[byte]string{
	'1': "hockey", // minute-based exclusion timers
	'3': "basketball",
	'6': "net", // sets, no games
	'7': "timer",
	'8': "timer",
	'9': "tennis",
	';': "volleyball",
	'=': "handball",
}

func isNet(sport string) bool {
	return sport == "net" || sport == "tennis" || sport == "volleyball"
}

func text(f []byte, start, n int) string {
	return strings.TrimSpace(string(f[start : start+n]))
}

// teamName decodes bytes 1..18: nine UTF-16 big-endian characters.
func teamName(f []byte) string {
	units := make([]uint16, 9)
	for i := range units {
		units[i] = uint16(f[1+2*i])<<8 | uint16(f[2+2*i])
	}
	return strings.Trim(string(utf16.Decode(units)), " \x00")
}

func clock(f []byte) string {
	sep := ":"
	if f[4] == ' ' { // under a minute the box sends seconds and tenths
		sep = "."
	}
	return text(f, 1, 2) + sep + text(f, 3, 2)
}

// Decode updates state (field name -> text) from one 20-byte frame.
func Decode(f []byte, state map[string]string) {
	if len(f) < 20 {
		return
	}
	switch f[0] {
	case 'w':
		state["home_name"] = teamName(f)
		return
	case 'b':
		state["away_name"] = teamName(f)
		return
	case 't': // new match
		for _, k := range []string{"home", "away", "clock", "period", "shot_clock"} {
			state[k] = ""
		}
		return
	case 'c': // box info: P = Pro, anything else = Lite
		state["box_type"] = "Lite"
		if f[1] == 'P' {
			state["box_type"] = "Pro"
		}
		return
	}

	sport, ok := sports[f[0]]
	if !ok {
		return
	}
	state["sport"] = sport
	num := func(b byte) string { return strconv.Itoa(int(b)) }

	if isNet(sport) {
		state["home"], state["away"] = num(f[5]), num(f[6])
		if f[5] == 100 { // advantage
			state["home"], state["away"] = "A", ""
		}
		if f[6] == 100 {
			state["home"], state["away"] = "", "A"
		}
		state["home_sets"], state["away_sets"] = text(f, 9, 1), text(f, 10, 1)
		switch f[19] {
		case '1':
			state["serve"] = "home"
		case '2', '3':
			state["serve"] = "away"
		default:
			state["serve"] = ""
		}
		if sport == "tennis" {
			state["home_games"], state["away_games"] = num(f[7]), num(f[8])
		}
		return
	}

	state["clock"] = clock(f)
	state["home"], state["away"] = num(f[5]), num(f[6])
	state["period"] = text(f, 7, 1)
	if sport == "basketball" {
		state["home_fouls"], state["away_fouls"] = num(f[9]), num(f[10])
		if f[16] >= 64 && f[16] <= 73 { // shot clock under 10s, with tenths
			state["shot_clock"] = string([]byte{f[15], '.', f[16] - 16})
		} else {
			state["shot_clock"] = text(f, 15, 2)
		}
	}
}
