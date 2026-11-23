package fpb

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strconv"
	"strings"
)

// Neutral is the accent used when a logo gives no usable colour.
const Neutral = "#5C6170"

type rgb struct{ r, g, b float64 } // 0..255

// PickColor returns the most common saturated colour in a logo, darkened
// until white text on it reaches 4.5:1. When that colour is too light, a
// colour at least a quarter as common that already reaches 4.5:1 wins.
// Neutral when the image can't be read (SVG, WebP) or has no saturated colour.
func PickColor(data []byte) string {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Neutral
	}
	type bucket struct {
		n       int
		r, g, b float64
	}
	buckets := map[int]*bucket{}
	best := -1
	bounds := img.Bounds()
	step := max(1, max(bounds.Dx(), bounds.Dy())/200) // sample big logos
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x8000 {
				continue
			}
			// RGBA is alpha-premultiplied; undo it.
			c := rgb{float64(r16) / float64(a16) * 255, float64(g16) / float64(a16) * 255, float64(b16) / float64(a16) * 255}
			if !saturated(c) {
				continue
			}
			key := int(c.r)>>4<<8 | int(c.g)>>4<<4 | int(c.b)>>4
			bk := buckets[key]
			if bk == nil {
				bk = &bucket{}
				buckets[key] = bk
			}
			bk.n++
			bk.r += c.r
			bk.g += c.g
			bk.b += c.b
			if best < 0 || bk.n > buckets[best].n {
				best = key
			}
		}
	}
	if best < 0 {
		return Neutral
	}
	mean := func(bk *bucket) rgb { return rgb{bk.r / float64(bk.n), bk.g / float64(bk.n), bk.b / float64(bk.n)} }
	pick := buckets[best]
	// A light main colour (gold, yellow) would darken to brown: a second
	// colour of the logo that is dark enough already looks more like the club.
	if ContrastWithWhite(mean(pick)) < 4.5 {
		alt := -1
		for k, bk := range buckets {
			if bk.n*4 >= pick.n && ContrastWithWhite(mean(bk)) >= 4.5 &&
				(alt < 0 || bk.n > buckets[alt].n || bk.n == buckets[alt].n && k < alt) {
				alt = k
			}
		}
		if alt >= 0 {
			pick = buckets[alt]
		}
	}
	return darken(mean(pick))
}

// saturated skips greys, near-white and near-black.
func saturated(c rgb) bool {
	hi := math.Max(c.r, math.Max(c.g, c.b))
	lo := math.Min(c.r, math.Min(c.g, c.b))
	return hi >= 40 && (hi-lo)/hi >= 0.35
}

// EnsureContrast returns hex darkened until white text passes 4.5:1, or
// Neutral if hex is not a colour.
func EnsureContrast(hex string) string {
	c, ok := parseHex(hex)
	if !ok {
		return Neutral
	}
	return darken(c)
}

func darken(c rgb) string {
	for {
		// Check the rounded colour, the one that is actually written out.
		r := rgb{math.Round(c.r), math.Round(c.g), math.Round(c.b)}
		if ContrastWithWhite(r) >= 4.5 {
			return fmt.Sprintf("#%02X%02X%02X", int(r.r), int(r.g), int(r.b))
		}
		c = rgb{c.r * 0.95, c.g * 0.95, c.b * 0.95}
	}
}

func parseHex(s string) (rgb, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
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

// ContrastWithWhite is the WCAG contrast ratio of white text on c.
func ContrastWithWhite(c rgb) float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
	return 1.05 / (l + 0.05)
}

// Contrast is ContrastWithWhite for a hex colour (0 if invalid).
func Contrast(hex string) float64 {
	c, ok := parseHex(hex)
	if !ok {
		return 0
	}
	return ContrastWithWhite(c)
}
