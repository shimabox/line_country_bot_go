// Package country holds the country data set and the lookups the bot needs:
// name search, random picks for the quiz and the URLs shown in replies.
package country

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
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
	countries   []*Country   // data order
	index       []IndexEntry // sorted by Key
	generatedAt string
	source      string
}

// IndexEntry maps a normalized name or alias to a country by its position
// in the data.
type IndexEntry struct {
	Key   string
	Index int
}

func newStore(countries []*Country, index []IndexEntry, generatedAt, source string) *Store {
	return &Store{countries: countries, index: index, generatedAt: generatedAt, source: source}
}

// Load parses and validates a countries document and builds the index.
func Load(data []byte) (*Store, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("country: parse data: %w", err)
	}
	if err := validate(f.Countries); err != nil {
		return nil, err
	}
	index, err := BuildIndex(f.Countries)
	if err != nil {
		return nil, err
	}
	return newStore(f.Countries, index, f.GeneratedAt, f.Source), nil
}

func validate(countries []*Country) error {
	if len(countries) == 0 {
		return errors.New("country: data has no countries")
	}
	codes := make(map[string]bool, len(countries))
	for _, c := range countries {
		if c == nil {
			return errors.New("country: null record")
		}
		if !validCode(c.Code) {
			return fmt.Errorf("country: invalid code %q", c.Code)
		}
		if codes[c.Code] {
			return fmt.Errorf("country: duplicate code %s", c.Code)
		}
		codes[c.Code] = true
	}
	return nil
}

// BuildIndex returns the lookup index of countries, sorted by Key in byte
// order (strings.Compare) without duplicates. The normalized display name
// always finds its country. Aliases never override a display name, and an
// alias shared by different countries is ambiguous and is left out.
//
// It fails when a display name normalizes to nothing or to the same key as
// another display name.
func BuildIndex(countries []*Country) ([]IndexEntry, error) {
	byKey := make(map[string]int, len(countries)*4)
	for i, c := range countries {
		if c == nil {
			return nil, errors.New("country: null record")
		}
		key := NormalizeKey(c.NameJA)
		if key == "" {
			return nil, fmt.Errorf("country: %s has no name_ja", c.Code)
		}
		if j, ok := byKey[key]; ok {
			return nil, fmt.Errorf("country: %s and %s share the name %q", countries[j].Code, c.Code, c.NameJA)
		}
		byKey[key] = i
	}

	owners := map[string]int{}
	ambiguous := map[string]bool{}
	for i, c := range countries {
		for _, alias := range slices.Concat(c.AliasesJA, c.AliasesEN) {
			key := NormalizeKey(alias)
			if key == "" {
				continue
			}
			if _, isName := byKey[key]; isName {
				continue
			}
			if prev, ok := owners[key]; ok && prev != i {
				ambiguous[key] = true
				continue
			}
			owners[key] = i
		}
	}
	for key, i := range owners {
		if !ambiguous[key] {
			byKey[key] = i
		}
	}

	index := make([]IndexEntry, 0, len(byKey))
	for key, i := range byKey {
		index = append(index, IndexEntry{Key: key, Index: i})
	}
	slices.SortFunc(index, func(a, b IndexEntry) int { return strings.Compare(a.Key, b.Key) })
	return index, nil
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
	i, ok := slices.BinarySearchFunc(s.index, key, func(e IndexEntry, key string) int {
		return strings.Compare(e.Key, key)
	})
	if !ok {
		return nil
	}
	return s.countries[s.index[i].Index]
}

// ByName finds a country by its exact display name (name_ja).
//
// The normalized display name always finds its own country in the index, so
// the display name matches exactly only when Lookup finds that country.
func (s *Store) ByName(name string) *Country {
	if c := s.Lookup(name); c != nil && c.NameJA == name {
		return c
	}
	return nil
}

// All returns the countries in data order. The slice must not be modified.
func (s *Store) All() []*Country { return s.countries }

// Len returns the number of countries.
func (s *Store) Len() int { return len(s.countries) }

// GeneratedAt returns the generation date recorded in the data.
func (s *Store) GeneratedAt() string { return s.generatedAt }

// Source returns the data source recorded in the data.
func (s *Store) Source() string { return s.source }

// PickN returns n distinct countries chosen uniformly at random. When the
// store has fewer than n countries, all of them are returned in random order.
func (s *Store) PickN(r *rand.Rand, n int) []*Country {
	all := s.countries
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
