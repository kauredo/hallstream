package ble

import (
	"testing"
	"unicode/utf16"
)

func frame(parts ...any) []byte {
	f := make([]byte, 20)
	for i := 0; i < len(parts); i += 2 {
		copy(f[parts[i].(int):], parts[i+1].([]byte))
	}
	return f
}

func utf16be(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u>>8), byte(u))
	}
	return out
}

func expect(t *testing.T, state map[string]string, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if state[k] != v {
			t.Errorf("%s = %q, want %q", k, state[k], v)
		}
	}
}

func TestBasketball(t *testing.T) {
	s := map[string]string{}
	Decode(frame(0, []byte("3"), 1, []byte("0745"), 5, []byte{82, 79}, 7, []byte("3"),
		9, []byte{4, 2}, 15, []byte("14")), s)
	expect(t, s, map[string]string{"clock": "07:45", "home": "82", "away": "79",
		"period": "3", "home_fouls": "4", "away_fouls": "2", "shot_clock": "14"})
}

func TestTenths(t *testing.T) {
	s := map[string]string{}
	Decode(frame(0, []byte("3"), 1, []byte(" 94 "), 15, []byte{'3', '2' + 16}), s)
	expect(t, s, map[string]string{"clock": "9.4", "shot_clock": "3.2"})
}

func TestTeamNames(t *testing.T) {
	s := map[string]string{}
	Decode(frame(0, []byte("w"), 1, utf16be("MONTEALTO  ")), s)
	Decode(frame(0, []byte("b"), 1, utf16be("PORTO    ")), s)
	expect(t, s, map[string]string{"home_name": "MONTEALTO", "away_name": "PORTO"})
}

func TestVolleyball(t *testing.T) {
	s := map[string]string{}
	Decode(frame(0, []byte(";"), 5, []byte{21, 18}, 9, []byte("21"), 19, []byte("2")), s)
	expect(t, s, map[string]string{"home": "21", "away": "18",
		"home_sets": "2", "away_sets": "1", "serve": "away"})
}

func TestBoxInfo(t *testing.T) {
	s := map[string]string{}
	Decode(frame(0, []byte("cP")), s)
	expect(t, s, map[string]string{"box_type": "Pro"})
	Decode(frame(0, []byte("cl")), s)
	expect(t, s, map[string]string{"box_type": "Lite"})
}
