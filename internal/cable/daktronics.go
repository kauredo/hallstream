package cable

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Daktronics is the All Sport 5000's real-time data (RTD) output:
//
//	SYN "00000000" SOH "004210" offset(4) STX data EOT checksum(2 hex) ETB
//
// Each frame writes its data into one buffer at the zero-based offset, and
// the console sends only what changed. Fields are read at the manual's
// one-based positions (ED-12483).
type Daktronics struct {
	buf [1024]byte
}

const (
	syn = 0x16
	soh = 0x01
	stx = 0x02
	etx = 0x03
	eot = 0x04
	etb = 0x17
)

// NewDaktronics returns a protocol with a blank buffer.
func NewDaktronics() *Daktronics {
	d := &Daktronics{}
	for i := range d.buf {
		d.buf[i] = ' '
	}
	return d
}

func (*Daktronics) Name() string { return "daktronics" }
func (*Daktronics) Baud() int    { return 19200 }

func (*Daktronics) Split(buf []byte) (frames [][]byte, rest []byte) {
	for {
		i := bytes.IndexByte(buf, syn)
		if i < 0 {
			return frames, nil
		}
		for i+1 < len(buf) && buf[i+1] == syn { // sync idle
			i++
		}
		buf = buf[i:]
		j := bytes.IndexByte(buf, etb)
		if j < 0 {
			return frames, keep(buf)
		}
		if k := bytes.LastIndexByte(buf[:j], syn); k > 0 { // a stray SYN before the frame's own
			buf, j = buf[k:], j-k
		}
		if f := buf[:j+1]; dakValid(f) {
			frames = append(frames, append([]byte(nil), f...))
		}
		buf = buf[j+1:]
	}
}

// dakValid checks the sum of every byte after SYN up to EOT against the
// two hex digits after EOT.
func dakValid(f []byte) bool {
	e := bytes.IndexByte(f, eot)
	if e < 1 || e+3 != len(f)-1 {
		return false
	}
	var sum byte
	for _, b := range f[1 : e+1] {
		sum += b
	}
	return fmt.Sprintf("%02X", sum) == string(f[e+1:e+3])
}

func (d *Daktronics) Decode(f []byte, sport string, vals map[string]string) bool {
	d.write(f)
	switch sport {
	case "basketball":
		d.shared(vals)
		vals["clock"] = clock(d.field(1, 5))
		vals["home_fouls"], vals["away_fouls"] = score(d.field(236, 2)), score(d.field(238, 2))
		vals["shot_clock"] = shot(d.field(201, 8))
	case "volleyball":
		d.shared(vals)
		vals["home_sets"], vals["away_sets"] = score(d.field(215, 2)), score(d.field(217, 2))
		switch {
		case d.field(201, 1) == "<":
			vals["serve"] = "home"
		case d.field(208, 1) == ">":
			vals["serve"] = "away"
		default:
			vals["serve"] = ""
		}
	default:
		return false
	}
	vals["sport"] = sport
	return true
}

// write copies a data frame into the buffer; other frames carry nothing.
func (d *Daktronics) write(f []byte) {
	s, x, e := bytes.IndexByte(f, soh), bytes.IndexByte(f, stx), bytes.IndexByte(f, eot)
	if s < 0 || x < s || e < x {
		return
	}
	head := string(f[s+1 : x])
	if len(head) != 10 || !strings.HasPrefix(head, "004210") {
		return
	}
	off, err := strconv.Atoi(head[6:])
	if err != nil || off < 0 || off >= len(d.buf) { // Atoi takes a sign: "-001" is a frame too
		return
	}
	copy(d.buf[off:], f[x+1:e])
}

func (d *Daktronics) shared(vals map[string]string) {
	vals["home"], vals["away"] = score(d.field(108, 4)), score(d.field(112, 4))
	vals["period"] = strings.TrimSpace(d.field(142, 2))
}

// field is n characters at the manual's one-based item.
func (d *Daktronics) field(item, n int) string {
	return string(d.buf[item-1 : item-1+n])
}

// shot is the shot clock's "mm:ss" (or "ss.t") as seconds: "24", "4.5", "".
func shot(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ".") {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "00:"), "0:")
		for len(s) > 1 && s[0] == '0' && s[1] != '.' {
			s = s[1:]
		}
		return s
	}
	if m, sec, ok := strings.Cut(s, ":"); ok {
		mins, _ := strconv.Atoi(strings.TrimSpace(m))
		n, err := strconv.Atoi(sec)
		if err != nil {
			return ""
		}
		return strconv.Itoa(mins*60 + n)
	}
	return seconds(s)
}
