// Package i18n holds every word the program shows or writes:
// web/i18n/<lang>.json, a flat map of dotted keys to strings with {name}
// placeholders. pt.json is the source and the fallback: a language without a
// file, or a key a language lacks, reads the Portuguese.
package i18n

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kauredo/hallstream/web"
)

// Default is the language of the source file, web/i18n/pt.json.
const Default = "pt"

type catalog struct {
	lang  string
	words map[string]string // pt with the language's own words over it
}

var (
	baseOnce sync.Once
	base     map[string]string
	cur      atomic.Pointer[catalog]
)

func load(lang string) (map[string]string, error) {
	b, err := web.FS.ReadFile("i18n/" + lang + ".json")
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("i18n/%s.json: %w", lang, err)
	}
	return m, nil
}

func portuguese() map[string]string {
	baseOnce.Do(func() {
		m, err := load(Default)
		if err != nil {
			// The test fails on a broken pt.json; at run time keys show as they are.
			m = map[string]string{}
		}
		base = m
	})
	return base
}

func current() *catalog {
	if c := cur.Load(); c != nil {
		return c
	}
	c := &catalog{lang: Default, words: portuguese()}
	cur.CompareAndSwap(nil, c)
	return cur.Load()
}

var langName = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

// Clean is lang as settings.lang keeps it: lower case, Default when it is
// empty or not a language name.
func Clean(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if !langName.MatchString(lang) {
		return Default
	}
	return lang
}

// SetLang switches to web/i18n/<lang>.json and returns the language in use:
// Default when that file is missing or broken.
func SetLang(lang string) string {
	lang = Clean(lang)
	words := portuguese()
	if lang != Default {
		own, err := load(lang)
		if err != nil {
			lang = Default
		} else {
			merged := make(map[string]string, len(words))
			for k, v := range words {
				merged[k] = v
			}
			for k, v := range own {
				merged[k] = v
			}
			words = merged
		}
	}
	cur.Store(&catalog{lang: lang, words: words})
	return lang
}

// Lang is the language in use.
func Lang() string { return current().lang }

// Words is every key with its string in the language in use, for the browser.
// The map is shared: callers must not change it.
func Words() map[string]string { return current().words }

var placeholder = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// T is the string for key with its placeholders filled from args, given as
// name, value pairs: T("log.porta", "porta", 8766). A value is written as
// fmt.Sprint writes it (an error as its message). A key nobody defined comes
// back as the key itself; a placeholder without a value stays as it is.
func T(key string, args ...any) string {
	s, ok := current().words[key]
	if !ok {
		return key
	}
	if len(args) == 0 || !strings.Contains(s, "{") {
		return s
	}
	vals := make(map[string]string, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		vals[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
	}
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// Key is an error whose message is the string for that key, read each time
// it is shown. Being comparable, it serves as a sentinel for errors.Is.
type Key string

func (k Key) Error() string { return T(string(k)) }

// Err is an error with the message T(key, args...). It wraps every error
// among the values, so errors.Is and errors.As still find them.
func Err(key string, args ...any) error { return &message{key: key, args: args} }

type message struct {
	key  string
	args []any
}

func (m *message) Error() string { return T(m.key, m.args...) }

func (m *message) Unwrap() []error {
	var errs []error
	for i := 1; i < len(m.args); i += 2 {
		if e, ok := m.args[i].(error); ok {
			errs = append(errs, e)
		}
	}
	return errs
}
