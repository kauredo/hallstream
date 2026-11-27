// Package cable reads the score from consoles on a serial cable (RS-232):
// it cuts the byte stream into checked frames and decodes them into the
// console values state.Machine.ApplyConsole reads. It does no I/O.
//
// The layouts come from docs/research/scoreboard-serial-protocols.md.
package cable

import (
	"slices"
	"strconv"
	"strings"
)

// Protocol is one console family. A value keeps what the console sent
// before, so use one per connection, from Get.
type Protocol interface {
	Name() string
	Baud() int
	// Split returns the whole frames at the start of buf whose checksum
	// holds, and what is left of a frame still arriving.
	Split(buf []byte) (frames [][]byte, rest []byte)
	// Decode updates vals from one frame of Split's. It returns false when
	// the protocol has no layout for sport, and then leaves vals alone
	// (Daktronics still writes the frame into its buffer).
	Decode(frame []byte, sport string, vals map[string]string) bool
}

// Names are the values of settings.serial.protocol, the default first.
var Names = []string{"daktronics", "bodet"}

// Get returns a new Protocol for name; an unknown name gets the first.
func Get(name string) Protocol {
	if Known(name) == "bodet" {
		return &Bodet{}
	}
	return NewDaktronics()
}

// Known is name if it is one of Names, else the first.
func Known(name string) string {
	if slices.Contains(Names, name) {
		return name
	}
	return Names[0]
}

// maxRest is how much of an unfinished frame Split keeps.
const maxRest = 4096

func keep(rest []byte) []byte {
	if len(rest) > maxRest {
		rest = rest[len(rest)-maxRest:]
	}
	return append([]byte(nil), rest...)
}

// score is a team's points without padding; blank reads 0.
func score(s string) string {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return "0"
	}
	return strconv.Itoa(n)
}

// clock trims a game clock to the box's shape: "9:59", "12:34", "45.6".
func clock(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 5 && s[0] == '0' && s[2] == ':' {
		s = s[1:]
	}
	return s
}

// seconds is a whole number of seconds without a leading zero, or "".
func seconds(s string) string {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return strconv.Itoa(n)
}
