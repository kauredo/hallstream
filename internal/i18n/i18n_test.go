package i18n

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// root is the repository, two folders up from this package.
const root = "../.."

func TestFill(t *testing.T) {
	SetLang(Default)
	if got := T("log.porta_ocupada_aviso", "pedida", 8765, "porta", 8766, "overlay", "http://x"); !strings.Contains(got, "8766") || strings.Contains(got, "{porta}") {
		t.Errorf("placeholders not filled: %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q, want the key", got)
	}
	base := errors.New("dial tcp: timeout")
	err := Err("log.nao_abri_servidor", "err", base)
	if !errors.Is(err, base) || !strings.Contains(err.Error(), "dial tcp: timeout") {
		t.Errorf("Err does not wrap or show its error: %v", err)
	}
}

func TestLanguageFallsBackToPortuguese(t *testing.T) {
	defer SetLang(Default)
	if got := SetLang("xx"); got != Default {
		t.Errorf("SetLang(xx) = %q, want %q", got, Default)
	}
	if got := SetLang("../pt"); got != Default {
		t.Errorf("SetLang(../pt) = %q, want %q", got, Default)
	}
	if T("log.a_fechar") == "log.a_fechar" {
		t.Error("pt.json not read")
	}
}

func TestLanguagesNameThemselves(t *testing.T) {
	got := Languages()
	if len(got) < 2 || got[0].Code != Default {
		t.Fatalf("Languages() = %v, want pt first and at least one more", got)
	}
	for _, l := range got {
		if l.Name == "" {
			t.Errorf("%s.json has no data.nome", l.Code)
		}
	}
}

// keysInOrder reads a catalog with its keys in file order.
func keysInOrder(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("%s is not a JSON object", path)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var v string
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%s: %q is not a string: %v", path, k, err)
		}
		keys = append(keys, k.(string))
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		t.Fatalf("%s: bad end", path)
	}
	if _, err := dec.Token(); err == nil {
		t.Fatalf("%s: text after the object", path)
	}
	return keys
}

func TestCatalogIsSortedJSON(t *testing.T) {
	keys := keysInOrder(t, filepath.Join(root, "web/i18n/pt.json"))
	if len(keys) == 0 {
		t.Fatal("pt.json is empty")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Errorf("pt.json: %q comes before %q (keys must be sorted, each once)", keys[i-1], keys[i])
		}
	}
	key := regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)+$`)
	for _, k := range keys {
		if !key.MatchString(k) {
			t.Errorf("pt.json: key %q is not dotted lower-case words", k)
		}
	}
}

func TestOtherLanguagesHaveTheSameKeys(t *testing.T) {
	want := keysInOrder(t, filepath.Join(root, "web/i18n/pt.json"))
	files, _ := filepath.Glob(filepath.Join(root, "web/i18n/*.json"))
	for _, f := range files {
		if filepath.Base(f) == "pt.json" {
			continue
		}
		got := keysInOrder(t, f)
		sort.Strings(got)
		for _, k := range want {
			if _, found := slices.BinarySearch(got, k); !found {
				t.Errorf("%s lacks %q", filepath.Base(f), k)
			}
		}
		for _, k := range got {
			if _, found := slices.BinarySearch(want, k); !found {
				t.Errorf("%s has %q, which pt.json does not", filepath.Base(f), k)
			}
		}
	}
}

// source is a file the key scan reads: every .go file, and the .js and
// .html under web/.
type source struct {
	path string // slash-separated, from the repository root
	text string
}

func sources(t *testing.T) []source {
	t.Helper()
	var out []source
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dados", "dist", "node_modules", ".hallmark":
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(p)
		if ext == ".go" || strings.HasPrefix(rel, "web/") && (ext == ".js" || ext == ".html") {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out = append(out, source{rel, string(b)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	jsKey     = regexp.MustCompile(`\bt\(\s*['"]([^'"]+)['"]`)
	jsDynamic = regexp.MustCompile(`\bt\(\s*[^'"\s)]`)
	htmlKey   = regexp.MustCompile(`data-i18n(?:-html)?="([^"]*)"`)
	htmlAttr  = regexp.MustCompile(`data-i18n-attr="([^"]*)"`)
	goKey     = regexp.MustCompile(`\bi18n\.(?:T|Err|Key)\(\s*"([^"]+)"`)
	goDynamic = regexp.MustCompile(`\bi18n\.(?:T|Err|Key)\(\s*[^"\s]`)
)

// uses finds every key the code names, with where it is named.
func uses(t *testing.T) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	add := func(k, where string) { found[k] = append(found[k], where) }
	for _, s := range sources(t) {
		if strings.HasPrefix(s.path, "internal/i18n/") || s.path == "web/i18n/i18n.js" {
			continue // the lookup itself, not a use
		}
		if strings.HasSuffix(s.path, ".go") {
			for _, m := range goKey.FindAllStringSubmatch(s.text, -1) {
				add(m[1], s.path)
			}
			if m := goDynamic.FindString(s.text); m != "" {
				t.Errorf("%s: %q builds a key at run time; name every key in full", s.path, m)
			}
			continue
		}
		for _, m := range jsKey.FindAllStringSubmatch(s.text, -1) {
			add(m[1], s.path)
		}
		for _, m := range htmlKey.FindAllStringSubmatch(s.text, -1) {
			add(m[1], s.path)
		}
		for _, m := range htmlAttr.FindAllStringSubmatch(s.text, -1) {
			for _, pair := range strings.Split(m[1], ";") {
				_, k, ok := strings.Cut(pair, ":")
				if !ok {
					t.Errorf("%s: data-i18n-attr %q is not attr:key", s.path, pair)
					continue
				}
				add(strings.TrimSpace(k), s.path)
			}
		}
		if m := jsDynamic.FindString(s.text); m != "" {
			t.Errorf("%s: %q builds a key at run time; name every key in full", s.path, m)
		}
	}
	return found
}

func TestEveryKeyIsDefinedAndUsed(t *testing.T) {
	SetLang(Default)
	words := Words()
	used := uses(t)
	for k, where := range used {
		if _, ok := words[k]; !ok {
			t.Errorf("key %q is used in %s but missing from web/i18n/pt.json", k, strings.Join(slices.Compact(where), ", "))
		}
	}
	for k := range words {
		if _, ok := used[k]; !ok {
			t.Errorf("key %q in web/i18n/pt.json is not used anywhere", k)
		}
	}
}

// looksPortuguese is what copy looks like: a Portuguese accent or a common word.
var looksPortuguese = regexp.MustCompile(`[ãõçáéíóúâêôàÃÕÇÁÉÍÓÚÂÊÔ]|\b(não|Carregue|Não)\b| (de|para|com|uma|está|sem) `)

// notCopy are literals that look Portuguese but are data, with the reason.
var notCopy = map[string]string{
	"ASSOCIAÇÃO": "a club word opponentName drops from FPB names (internal/web/game.go)",
	"Sénior":     "an FPB age group, in the order the club setup sorts escalões",
	"Hallstream (à procura de câmaras)": "the name of a source the program adds to OBS and removes; " +
		"OBS knows it by this name, so it never changes with the language",
	"AÉÇ0º":  "glyphs resultado.js asks the browser to load the fonts for",
	"AÉÇÁ0º": "glyphs miniatura.js asks the browser to load the fonts for",
}

// mockFiles stand in for the server and its data, like the fixtures.
var mockFiles = map[string]bool{"web/painel/mock.js": true}

// asciiLetter finds a literal with a word in it; one without (an accent
// table such as "àáâã") is not copy.
var asciiLetter = regexp.MustCompile(`[A-Za-z]`)

func TestNoCopyLeftInCode(t *testing.T) {
	check := func(path string, line int, s string) {
		if !asciiLetter.MatchString(s) || !looksPortuguese.MatchString(s) {
			return
		}
		if _, ok := notCopy[s]; ok {
			return
		}
		t.Errorf("%s:%d: %q looks like copy; move it to web/i18n/pt.json", path, line, s)
	}
	for _, s := range sources(t) {
		p := s.path
		switch {
		case strings.HasSuffix(p, ".go"):
			if strings.HasSuffix(p, "_test.go") || strings.HasPrefix(p, "internal/i18n/") {
				continue
			}
			goStrings(t, p, s.text, check)
		case mockFiles[p]:
		case strings.HasSuffix(p, ".js") && (strings.HasPrefix(p, "web/painel/") || strings.HasPrefix(p, "web/overlay/") || strings.HasPrefix(p, "web/marcar/")) && strings.Count(p, "/") == 2:
			jsStrings(s.text, func(line int, lit string) { check(p, line, lit) })
		case strings.HasSuffix(p, ".html") && (strings.HasPrefix(p, "web/painel/") || strings.HasPrefix(p, "web/overlay/") || strings.HasPrefix(p, "web/marcar/")) && strings.Count(p, "/") == 2:
			htmlText(s.text, func(line int, txt string) { check(p, line, txt) })
		}
	}
}

func goStrings(t *testing.T, path, src string, f func(string, int, string)) {
	fset := token.NewFileSet()
	file := fset.AddFile(path, -1, len(src))
	var sc scanner.Scanner
	sc.Init(file, []byte(src), func(pos token.Position, msg string) { t.Errorf("%s: %s", pos, msg) }, 0)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			return
		}
		if tok != token.STRING {
			continue
		}
		s, err := strconv.Unquote(lit)
		if err != nil {
			s = lit
		}
		f(path, fset.Position(pos).Line, s)
	}
}

// jsStrings calls f with every string and template literal outside comments
// and regular expressions.
func jsStrings(src string, f func(int, string)) {
	line := 1
	prev := func(i int) (byte, string) {
		k := i - 1
		for k >= 0 && (src[k] == ' ' || src[k] == '\t' || src[k] == '\n' || src[k] == '\r') {
			k--
		}
		if k < 0 {
			return 0, ""
		}
		j := k
		for j >= 0 && (src[j] >= 'a' && src[j] <= 'z') {
			j--
		}
		return src[k], src[j+1 : k+1]
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			line++
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			e := strings.Index(src[i+2:], "*/") + i + 2
			line += strings.Count(src[i:e], "\n")
			i = e + 1
		case c == '\'' || c == '"' || c == '`':
			start, j := line, i+1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				} else if src[j] == '\n' {
					line++
				}
				j++
			}
			f(start, src[i+1:j])
			i = j
		case c == '/':
			p, word := prev(i)
			if p == 0 || strings.IndexByte("(,=:[!&|?{};+-*%<>~^", p) >= 0 || word == "return" || word == "typeof" {
				j, class := i+1, false
				for j < len(src) && (src[j] != '/' || class) {
					switch src[j] {
					case '\\':
						j++
					case '[':
						class = true
					case ']':
						class = false
					}
					j++
				}
				i = j
			}
		}
	}
}

var (
	htmlSkip = regexp.MustCompile(`(?s)<!--.*?-->|<script\b.*?</script>|<style\b.*?</style>`)
	htmlTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlVal  = regexp.MustCompile(`="([^"]*)"`)
)

// htmlText calls f with the text between tags and every attribute value.
func htmlText(src string, f func(int, string)) {
	src = htmlSkip.ReplaceAllStringFunc(src, func(m string) string { return strings.Repeat("\n", strings.Count(m, "\n")) })
	last := 0
	lineAt := func(i int) int { return 1 + strings.Count(src[:i], "\n") }
	for _, loc := range htmlTag.FindAllStringIndex(src, -1) {
		if txt := strings.TrimSpace(src[last:loc[0]]); txt != "" {
			f(lineAt(last), txt)
		}
		for _, v := range htmlVal.FindAllStringSubmatch(src[loc[0]:loc[1]], -1) {
			f(lineAt(loc[0]), v[1])
		}
		last = loc[1]
	}
	if txt := strings.TrimSpace(src[last:]); txt != "" {
		f(lineAt(last), txt)
	}
}
