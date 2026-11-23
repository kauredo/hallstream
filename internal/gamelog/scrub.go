package gamelog

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// homeBackstop catches a Windows user folder the home path missed.
var homeBackstop = regexp.MustCompile(`(?i)[A-Za-z]:[\\/]Users[\\/][^\\/\s]+`)

// NewScrubber returns what cleans marcador.log of the personal: home (any
// case, either slash) becomes "~", the bare user name "(utilizador)" and each
// name "(jogador)". Names match whole words only, in any case, with or
// without accents.
func NewScrubber(home string, names []string) func([]byte) []byte {
	type term struct {
		folded []rune
		with   string
	}
	var terms []term
	add := func(s, with string) {
		if f := fold(strings.TrimSpace(s)); len(f) > 2 {
			terms = append(terms, term{f, with})
		}
	}
	if len(home) > 3 {
		add(home, "~")
		add(filepath.Base(strings.ReplaceAll(home, `\`, "/")), "(utilizador)")
	}
	for _, n := range names {
		add(n, "(jogador)")
	}
	// The longer first, so "Rui Costa" goes whole before "Rui".
	slices.SortStableFunc(terms, func(a, b term) int { return len(b.folded) - len(a.folded) })
	return func(b []byte) []byte {
		src := []rune(string(b))
		folded := fold(string(src))
		with := make([]string, len(src)) // replacement at the start of a match
		taken := make([]bool, len(src))
		for _, t := range terms {
			n := len(t.folded)
			for i := 0; i+n <= len(folded); i++ {
				if folded[i] != t.folded[0] || !slices.Equal(folded[i:i+n], t.folded) || slices.Contains(taken[i:i+n], true) ||
					i > 0 && wordRune(folded[i-1]) || i+n < len(folded) && wordRune(folded[i+n]) {
					continue
				}
				with[i] = t.with
				for j := i; j < i+n; j++ {
					taken[j] = true
				}
				i += n - 1
			}
		}
		var out strings.Builder
		for i, r := range src {
			switch {
			case with[i] != "":
				out.WriteString(with[i])
			case !taken[i]:
				out.WriteRune(r)
			}
		}
		return []byte(homeBackstop.ReplaceAllString(out.String(), "~"))
	}
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// fold lowers s, takes the accents off and turns \ into /, one rune for one.
func fold(s string) []rune {
	rs := []rune(s)
	for i, r := range rs {
		r = unicode.ToLower(r)
		if f, ok := plain[r]; ok {
			r = f
		}
		rs[i] = r
	}
	return rs
}

var plain = func() map[rune]rune {
	m := map[rune]rune{'\\': '/'}
	for to, from := range map[rune]string{'a': "àáâãäå", 'e': "èéêë", 'i': "ìíîï", 'o': "òóôõö", 'u': "ùúûü", 'c': "ç", 'n': "ñ", 'y': "ýÿ"} {
		for _, r := range from {
			m[r] = to
		}
	}
	return m
}()
