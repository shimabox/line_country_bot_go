// Package country holds the country data set and the lookups the bot needs:
// name search, random picks for the quiz and the URLs shown in replies.
package country

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// NoCapitalText is shown instead of the capital when the data has none.
const NoCapitalText = "首都情報なし"

const (
	wikipediaJABase = "https://ja.wikipedia.org/wiki/"
	mapsSearchBase  = "https://www.google.com/maps/search/?api=1&query="
)

// File is the JSON document stored in data/countries.json.
type File struct {
	GeneratedAt string     `json:"generated_at"`
	Source      string     `json:"source"`
	Countries   []*Country `json:"countries"`
}

// Country is one country record.
type Country struct {
	Code           string   `json:"code"`
	NameJA         string   `json:"name_ja"`
	AliasesJA      []string `json:"aliases_ja,omitempty"`
	AliasesEN      []string `json:"aliases_en,omitempty"`
	CapitalJA      string   `json:"capital_ja,omitempty"`
	CapitalEN      string   `json:"capital_en,omitempty"`
	Lat            float64  `json:"lat"`
	Lon            float64  `json:"lon"`
	WikipediaJAURL string   `json:"wikipedia_ja_url"`
}

// HasCapital reports whether a capital name is known.
func (c *Country) HasCapital() bool {
	return c.CapitalJA != "" || c.CapitalEN != ""
}

// CapitalText returns "capital_ja / capital_en", or NoCapitalText.
func (c *Country) CapitalText() string {
	switch {
	case c.CapitalJA != "" && c.CapitalEN != "":
		return c.CapitalJA + " / " + c.CapitalEN
	case c.CapitalJA != "":
		return c.CapitalJA
	case c.CapitalEN != "":
		return c.CapitalEN
	default:
		return NoCapitalText
	}
}

// FlagURL returns the URL of the flag image served from baseURL.
func (c *Country) FlagURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/flags/" + strings.ToLower(c.Code) + ".png"
}

// MapURL returns a Google Maps search URL for the capital (or the country).
func (c *Country) MapURL() string {
	return mapsSearchBase + formatCoord(c.Lat) + "," + formatCoord(c.Lon)
}

// WikipediaURL returns the Japanese Wikipedia article URL.
func (c *Country) WikipediaURL() string {
	if c.WikipediaJAURL != "" {
		return c.WikipediaJAURL
	}
	return wikipediaJABase + url.PathEscape(c.NameJA)
}

func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Store is an immutable, indexed set of countries.
type Store struct {
	file   File
	byKey  map[string]*Country // normalized name or alias
	byName map[string]*Country // exact display name
}

// Load parses and validates a countries document and builds the indexes.
func Load(data []byte) (*Store, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("country: parse data: %w", err)
	}
	if len(f.Countries) == 0 {
		return nil, errors.New("country: data has no countries")
	}

	s := &Store{
		file:   f,
		byKey:  make(map[string]*Country, len(f.Countries)*4),
		byName: make(map[string]*Country, len(f.Countries)),
	}
	codes := make(map[string]bool, len(f.Countries))
	for _, c := range f.Countries {
		if c == nil {
			return nil, errors.New("country: null record")
		}
		if !validCode(c.Code) {
			return nil, fmt.Errorf("country: invalid code %q", c.Code)
		}
		if codes[c.Code] {
			return nil, fmt.Errorf("country: duplicate code %s", c.Code)
		}
		codes[c.Code] = true
		if c.NameJA == "" {
			return nil, fmt.Errorf("country: %s has no name_ja", c.Code)
		}
		key := NormalizeKey(c.NameJA)
		if other, ok := s.byKey[key]; ok {
			return nil, fmt.Errorf("country: %s and %s share the name %q", other.Code, c.Code, c.NameJA)
		}
		s.byKey[key] = c
		s.byName[c.NameJA] = c
	}

	// Aliases never override a display name. An alias shared by different
	// countries is ambiguous and is left out of the index.
	owners := map[string]*Country{}
	ambiguous := map[string]bool{}
	for _, c := range f.Countries {
		for _, alias := range append(append([]string{}, c.AliasesJA...), c.AliasesEN...) {
			key := NormalizeKey(alias)
			if key == "" {
				continue
			}
			if _, isName := s.byKey[key]; isName {
				continue
			}
			if prev, ok := owners[key]; ok && prev != c {
				ambiguous[key] = true
				continue
			}
			owners[key] = c
		}
	}
	for key, c := range owners {
		if !ambiguous[key] {
			s.byKey[key] = c
		}
	}
	return s, nil
}

func validCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	for i := 0; i < 2; i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}
	return true
}

// NormalizeKey removes every whitespace character (including U+3000) and
// lowercases ASCII letters. Non-ASCII letters are kept as is.
func NormalizeKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || r == '　':
			continue
		case 'A' <= r && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Lookup finds a country by display name, Japanese alias or English name.
// The input is normalized with NormalizeKey. It returns nil when nothing
// matches exactly.
func (s *Store) Lookup(name string) *Country {
	key := NormalizeKey(name)
	if key == "" {
		return nil
	}
	return s.byKey[key]
}

// ByName finds a country by its exact display name (name_ja).
func (s *Store) ByName(name string) *Country {
	return s.byName[name]
}

// All returns the countries in data order. The slice must not be modified.
func (s *Store) All() []*Country { return s.file.Countries }

// Len returns the number of countries.
func (s *Store) Len() int { return len(s.file.Countries) }

// GeneratedAt returns the generation date recorded in the data.
func (s *Store) GeneratedAt() string { return s.file.GeneratedAt }

// Source returns the data source recorded in the data.
func (s *Store) Source() string { return s.file.Source }

// PickN returns n distinct countries chosen uniformly at random. When the
// store has fewer than n countries, all of them are returned in random order.
func (s *Store) PickN(r *rand.Rand, n int) []*Country {
	all := s.file.Countries
	if n <= 0 {
		return nil
	}
	if n > len(all) {
		n = len(all)
	}
	idx := make([]int, len(all))
	for i := range idx {
		idx[i] = i
	}
	out := make([]*Country, n)
	for i := 0; i < n; i++ {
		j := i + r.IntN(len(idx)-i)
		idx[i], idx[j] = idx[j], idx[i]
		out[i] = all[idx[i]]
	}
	return out
}
