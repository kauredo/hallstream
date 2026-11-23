// Package club holds what the club profile derives: the on-air colours that
// come from the club's two colours, and the sports a club can pick.
package club

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// The colours of a club that has not set its own: a slate blue and the
// Hallstream ink, both with white text well over 4.5:1.
const (
	DefaultPrimary   = "#2F4B7C"
	DefaultSecondary = "#1C1A17"
)

// Sport is one sport a club can pick. Only Active ones work today; the rest
// are listed in the panel as coming soon.
type Sport struct {
	ID     string
	Name   string // BASQUETEBOL
	Short  string // the word on air: BASKET
	Active bool
}

var Sports = []Sport{
	{"basketball", "BASQUETEBOL", "BASKET", true},
	{"futsal", "FUTSAL", "FUTSAL", false},
	{"handball", "ANDEBOL", "ANDEBOL", false},
	{"volleyball", "VOLEIBOL", "VOLEIBOL", false},
	{"soccer", "FUTEBOL", "FUTEBOL", false},
}

// SportOf finds a sport by its id.
func SportOf(id string) (Sport, bool) {
	for _, s := range Sports {
		if s.ID == id {
			return s, true
		}
	}
	return Sport{}, false
}

// Hex returns s as "#RRGGBB" when it is a "#RGB" or "#RRGGBB" colour.
func Hex(s string) (string, bool) {
	c, ok := parse(s)
	if !ok {
		return "", false
	}
	return c.hex(), true
}

// Theme is the overlay's colour set, keyed by CSS custom property name
// without the dashes. The old red family (red, kit, blood, ink, red-hi,
// red-lo, red-text, tint, tint-dim) comes from primary, navy from secondary,
// and on-red and on-kit are white or ink, whichever reads better. The -rgb
// keys are "r,g,b" for rgba(). A colour that is not one falls back to the
// default.
func Theme(primary, secondary string) map[string]string {
	p, ok := parse(primary)
	if !ok {
		p, _ = parse(DefaultPrimary)
	}
	s, ok := parse(secondary)
	if !ok {
		s, _ = parse(DefaultSecondary)
	}
	h, sat, l := p.hsl()
	ink := hsl(h, math.Min(sat, 0.6), 0.05)
	kit := hsl(h, sat, l*0.72)
	navy := darken(s, white, 12)
	hi := lighten(p, ink)
	return map[string]string{
		"red":      p.hex(),
		"on-red":   on(p, ink).hex(),
		"kit":      kit.hex(),
		"on-kit":   on(kit, ink).hex(),
		"blood":    darken(hsl(h, sat, math.Min(l*0.45, 0.2)), white, 7).hex(),
		"ink":      ink.hex(),
		"navy":     navy.hex(),
		"red-hi":   hi.hex(),
		"red-lo":   lighten(p, navy).hex(),
		"red-text": darken(p, white, 4.5).hex(),
		"tint":     hsl(h, sat, math.Min(l, 0.38)).hex(),
		"tint-dim": hsl(h, sat, math.Min(l, 0.345)).hex(),
		"red-rgb":  p.triplet(),
		"hi-rgb":   hi.triplet(),
		"ink-rgb":  ink.triplet(),
	}
}

type rgb struct{ r, g, b float64 } // 0..255, whole numbers

var white = rgb{255, 255, 255}

func parse(s string) (rgb, bool) {
	s, ok := strings.CutPrefix(strings.TrimSpace(s), "#")
	if !ok {
		return rgb{}, false
	}
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return rgb{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{float64(v >> 16), float64(v >> 8 & 0xff), float64(v & 0xff)}, true
}

func (c rgb) hex() string {
	return fmt.Sprintf("#%02X%02X%02X", int(c.r), int(c.g), int(c.b))
}

func (c rgb) triplet() string {
	return fmt.Sprintf("%d,%d,%d", int(c.r), int(c.g), int(c.b))
}

// lum is the WCAG relative luminance.
func (c rgb) lum() float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

func contrast(a, b rgb) float64 {
	x, y := a.lum(), b.lum()
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}

// on is white text when it reaches 4.5:1 on bg, else whichever of white
// and dark reads better.
func on(bg, dark rgb) rgb {
	if w := contrast(white, bg); w >= 4.5 || w >= contrast(dark, bg) {
		return white
	}
	return dark
}

// darken lowers c's lightness until it reaches ratio against bg.
func darken(c, bg rgb, ratio float64) rgb {
	h, s, l := c.hsl()
	for ; l > 0; l -= 0.01 {
		if x := hsl(h, s, l); contrast(x, bg) >= ratio {
			return x
		}
	}
	return hsl(h, s, 0)
}

// lighten raises c's lightness until it reaches 5.5:1 against the dark bg,
// for small text on it.
func lighten(c, bg rgb) rgb {
	h, s, l := c.hsl()
	for ; l < 1; l += 0.01 {
		if x := hsl(h, s, l); contrast(x, bg) >= 5.5 {
			return x
		}
	}
	return white
}

func (c rgb) hsl() (h, s, l float64) {
	r, g, b := c.r/255, c.g/255, c.b/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	if l > 0.5 {
		s = d / (2 - hi - lo)
	} else {
		s = d / (hi + lo)
	}
	switch hi {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

// hsl builds a colour from hue, saturation and lightness (0..1), rounded to
// whole channels so the contrast checks see the colour that is written out.
func hsl(h, s, l float64) rgb {
	l = math.Max(0, math.Min(1, l))
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	ch := func(t float64) float64 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return math.Round(v * 255)
	}
	return rgb{ch(h + 1.0/3), ch(h), ch(h - 1.0/3)}
}
