package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/shimabox/line_country_bot_go/internal/country"
	"github.com/shimabox/line_country_bot_go/internal/line"
)

// Source is recorded in the generated file.
const Source = "Wikidata (https://www.wikidata.org/), CC0 1.0"

const (
	entityPrefix    = "http://www.wikidata.org/entity/"
	wikipediaJABase = "https://ja.wikipedia.org/wiki/"
	maxNameJA       = line.MaxActionLabel // quiz buttons use name_ja as labels
)

// SPARQLResults is the subset of the SPARQL 1.1 JSON results format we read.
type SPARQLResults struct {
	Results struct {
		Bindings []map[string]SPARQLValue `json:"bindings"`
	} `json:"results"`
}

// SPARQLValue is one bound value of a result row.
type SPARQLValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ParseResults decodes a SPARQL JSON response.
func ParseResults(data []byte) (*SPARQLResults, error) {
	var r SPARQLResults
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse SPARQL results: %w", err)
	}
	if r.Results.Bindings == nil {
		return nil, errors.New("parse SPARQL results: no bindings")
	}
	return &r, nil
}

// Overrides are manual corrections kept in overrides.json.
type Overrides struct {
	// Exclude lists codes to leave out of the output.
	Exclude []string `json:"exclude,omitempty"`
	// Countries maps an ISO code to its corrections.
	Countries map[string]Override `json:"countries,omitempty"`
}

// Override corrects one country. Empty fields keep the generated value.
type Override struct {
	// QID selects the Wikidata item when several items share the code.
	QID string `json:"qid,omitempty"`
	// NameJA replaces the display name.
	NameJA string `json:"name_ja,omitempty"`
	// AliasesJA adds search words.
	AliasesJA []string `json:"aliases_ja,omitempty"`
	// RemoveAliases drops generated aliases (Japanese or English) that are
	// misleading or shared with another country.
	RemoveAliases []string `json:"remove_aliases,omitempty"`
	// WikipediaJAURL replaces the article URL.
	WikipediaJAURL string `json:"wikipedia_ja_url,omitempty"`
	// Capital selects the capital item (QID) when Wikidata lists several.
	Capital string `json:"capital,omitempty"`
	// CapitalJA and CapitalEN replace the capital names.
	CapitalJA string `json:"capital_ja,omitempty"`
	CapitalEN string `json:"capital_en,omitempty"`
	// Lat and Lon replace the map coordinates.
	Lat *float64 `json:"lat,omitempty"`
	Lon *float64 `json:"lon,omitempty"`
	// Note records why the override exists. It is not used by the generator.
	Note string `json:"note,omitempty"`
}

// ParseOverrides decodes overrides.json, rejecting unknown fields so typos
// are not silently ignored.
func ParseOverrides(data []byte) (*Overrides, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var o Overrides
	if err := dec.Decode(&o); err != nil {
		return nil, fmt.Errorf("parse overrides: %w", err)
	}
	return &o, nil
}

// ValidationError lists every problem found while building, one per line, so
// all of them can be fixed in a single pass.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d problem(s); fix them in overrides.json:\n  %s",
		len(e.Problems), strings.Join(e.Problems, "\n  "))
}

type capitalAgg struct {
	ja, en, coords set
}

type itemAgg struct {
	labelJA, labelEN, altJA, officialJA set
	articles                            map[string]set // article URL -> titles
	capitals                            map[string]*capitalAgg
	coords                              set
}

func newItemAgg() *itemAgg {
	return &itemAgg{
		labelJA: set{}, labelEN: set{}, altJA: set{}, officialJA: set{}, coords: set{},
		articles: map[string]set{},
		capitals: map[string]*capitalAgg{},
	}
}

// aggregate groups result rows by code and then by Wikidata item.
func aggregate(res *SPARQLResults) map[string]map[string]*itemAgg {
	byCode := map[string]map[string]*itemAgg{}
	for _, row := range res.Results.Bindings {
		code := strings.ToUpper(strings.TrimSpace(row["code"].Value))
		qid := strings.TrimPrefix(row["country"].Value, entityPrefix)
		if code == "" || qid == "" {
			continue
		}
		items := byCode[code]
		if items == nil {
			items = map[string]*itemAgg{}
			byCode[code] = items
		}
		it := items[qid]
		if it == nil {
			it = newItemAgg()
			items[qid] = it
		}
		it.labelJA.add(row["labelJa"].Value)
		it.labelEN.add(row["labelEn"].Value)
		it.altJA.add(row["altJa"].Value)
		it.officialJA.add(row["officialJa"].Value)
		it.coords.add(row["countryCoord"].Value)
		if a := row["article"].Value; a != "" {
			if it.articles[a] == nil {
				it.articles[a] = set{}
			}
			it.articles[a].add(row["articleTitle"].Value)
		}
		if c := strings.TrimPrefix(row["capital"].Value, entityPrefix); c != "" {
			capital := it.capitals[c]
			if capital == nil {
				capital = &capitalAgg{ja: set{}, en: set{}, coords: set{}}
				it.capitals[c] = capital
			}
			capital.ja.add(row["capitalJa"].Value)
			capital.en.add(row["capitalEn"].Value)
			capital.coords.add(row["capitalCoord"].Value)
		}
	}
	return byCode
}

// Build turns SPARQL rows and overrides into the countries document. Output
// does not depend on row order. All problems are reported together.
func Build(res *SPARQLResults, ov *Overrides, generatedAt string) (*country.File, error) {
	if ov == nil {
		ov = &Overrides{}
	}
	byCode := aggregate(res)
	var problems []string
	addf := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	excluded := map[string]bool{}
	for _, code := range ov.Exclude {
		excluded[code] = true
		if byCode[code] == nil {
			addf("%s: excluded code is not in the query results; remove it from exclude", code)
		}
	}
	for _, code := range sortedKeys(ov.Countries) {
		if byCode[code] == nil {
			addf("%s: override for a code that is not in the query results; remove it", code)
		}
	}

	out := &country.File{GeneratedAt: generatedAt, Source: Source}
	names := map[string]string{} // display name -> code
	for _, code := range sortedKeys(byCode) {
		if excluded[code] {
			continue
		}
		o := ov.Countries[code]
		c, errs := buildCountry(code, byCode[code], o)
		problems = append(problems, errs...)
		if c == nil {
			continue
		}
		if other, ok := names[c.NameJA]; ok {
			addf("%s: display name %s is also used by %s; set countries.%s.name_ja", code, c.NameJA, other, code)
			continue
		}
		names[c.NameJA] = code
		out.Countries = append(out.Countries, c)
	}
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return out, nil
}

func buildCountry(code string, items map[string]*itemAgg, o Override) (*country.Country, []string) {
	var problems []string
	addf := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if len(code) != 2 || strings.ToUpper(code) != code || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		addf("%s: not an ISO 3166-1 alpha-2 code; add it to exclude", code)
		return nil, problems
	}

	qids := sortedKeys(items)
	var it *itemAgg
	switch {
	case o.QID != "":
		it = items[o.QID]
		if it == nil {
			addf("%s: qid %s is not one of the items %s", code, o.QID, strings.Join(qids, ", "))
			return nil, problems
		}
	case len(qids) > 1:
		addf("%s: %d items share the code (%s); set countries.%s.qid", code, len(qids), strings.Join(qids, ", "), code)
		return nil, problems
	default:
		it = items[qids[0]]
	}

	c := &country.Country{Code: code}

	// Display name: ja.wikipedia article title, then ja label, then override.
	articleURL, articleTitle := "", ""
	if urls := sortedKeys(it.articles); len(urls) > 0 {
		articleURL = urls[0]
		articleTitle = it.articles[articleURL].first()
	}
	c.NameJA = articleTitle
	if c.NameJA == "" {
		c.NameJA = it.labelJA.first()
	}
	if o.NameJA != "" {
		c.NameJA = o.NameJA
	}
	switch {
	case c.NameJA == "":
		addf("%s: no ja.wikipedia article and no ja label; set countries.%s.name_ja", code, code)
	case line.UTF16Len(c.NameJA) > maxNameJA:
		addf("%s: name_ja %q is %d UTF-16 units and exceeds %d; set countries.%s.name_ja",
			code, c.NameJA, line.UTF16Len(c.NameJA), maxNameJA, code)
	}

	switch {
	case o.WikipediaJAURL != "":
		c.WikipediaJAURL = o.WikipediaJAURL
	case articleURL != "":
		c.WikipediaJAURL = articleURL
	case c.NameJA != "":
		c.WikipediaJAURL = wikipediaJABase + url.PathEscape(c.NameJA)
	}

	// Aliases: generated words minus remove_aliases, plus aliases_ja.
	aliases, aliasesEN := set{}, set{}
	for _, s := range [][]string{it.labelJA.sorted(), it.altJA.sorted(), it.officialJA.sorted(), {articleTitle}} {
		for _, a := range s {
			aliases.add(a)
		}
	}
	for _, a := range it.labelEN.sorted() {
		aliasesEN.add(a)
	}
	for _, a := range o.RemoveAliases {
		delete(aliases, a)
		delete(aliasesEN, a)
	}
	for _, a := range o.AliasesJA {
		aliases.add(a)
	}
	delete(aliases, c.NameJA)
	if len(aliases) > 0 {
		c.AliasesJA = aliases.sorted()
	}
	if len(aliasesEN) > 0 {
		c.AliasesEN = aliasesEN.sorted()
	}

	// Capital.
	var capital *capitalAgg
	capQIDs := sortedKeys(it.capitals)
	switch {
	case o.Capital != "":
		capital = it.capitals[o.Capital]
		if capital == nil {
			addf("%s: capital %s is not one of the capitals %s", code, o.Capital, describeCapitals(it.capitals))
		}
	case len(capQIDs) == 1:
		capital = it.capitals[capQIDs[0]]
	case len(capQIDs) > 1:
		// Name and coordinate overrides do not say which capital is meant, so
		// the QID is required even when they are all set.
		addf("%s: %d capitals (%s); set countries.%s.capital to one QID and record the reason in note",
			code, len(capQIDs), describeCapitals(it.capitals), code)
	}
	if capital != nil {
		c.CapitalJA = trimCapitalSuffix(capital.ja.first())
		c.CapitalEN = capital.en.first()
	}
	if o.CapitalJA != "" {
		c.CapitalJA = o.CapitalJA
	}
	if o.CapitalEN != "" {
		c.CapitalEN = o.CapitalEN
	}

	// Coordinates: capital, then country, then override.
	lat, lon, ok := 0.0, 0.0, false
	if capital != nil {
		lat, lon, ok = firstPoint(capital.coords)
	}
	if !ok {
		lat, lon, ok = firstPoint(it.coords)
	}
	if o.Lat != nil && o.Lon != nil {
		lat, lon, ok = *o.Lat, *o.Lon, true
	}
	if !ok {
		addf("%s: no coordinates for the capital or the country; set countries.%s.lat and lon", code, code)
	}
	c.Lat, c.Lon = lat, lon

	if len(problems) > 0 {
		return nil, problems
	}
	return c, nil
}

// SharedAliases lists aliases that several countries share and that no
// country uses as its display name. The runtime index leaves them out, so a
// user typing one gets "not found"; remove_aliases or name_ja can fix that.
// Each entry is "normalized alias (CODE, CODE...)", sorted.
func SharedAliases(f *country.File) []string {
	names := map[string]bool{}
	for _, c := range f.Countries {
		names[country.NormalizeKey(c.NameJA)] = true
	}
	owners := map[string]set{}
	for _, c := range f.Countries {
		for _, a := range append(append([]string{}, c.AliasesJA...), c.AliasesEN...) {
			key := country.NormalizeKey(a)
			if key == "" || names[key] {
				continue
			}
			if owners[key] == nil {
				owners[key] = set{}
			}
			owners[key].add(c.Code)
		}
	}
	var out []string
	for _, key := range sortedKeys(owners) {
		if codes := owners[key].sorted(); len(codes) > 1 {
			out = append(out, fmt.Sprintf("%s (%s)", key, strings.Join(codes, ", ")))
		}
	}
	return out
}

func describeCapitals(caps map[string]*capitalAgg) string {
	parts := []string{}
	for _, q := range sortedKeys(caps) {
		parts = append(parts, fmt.Sprintf("%s %s", q, caps[q].ja.first()))
	}
	return strings.Join(parts, ", ")
}

// trimCapitalSuffix removes 特別市, 市 or 都 from the end of a capital name
// (東京都 → 東京). A name that is only the suffix is kept.
func trimCapitalSuffix(s string) string {
	for _, suffix := range []string{"特別市", "市", "都"} {
		if rest, ok := strings.CutSuffix(s, suffix); ok {
			if rest == "" {
				return s
			}
			return rest
		}
	}
	return s
}

// firstPoint returns the first parsable Earth coordinate in sorted order.
func firstPoint(coords set) (lat, lon float64, ok bool) {
	for _, p := range coords.sorted() {
		if lat, lon, ok := parsePoint(p); ok {
			return lat, lon, true
		}
	}
	return 0, 0, false
}

// parsePoint parses a WKT literal "Point(lon lat)". Coordinates on other
// globes are prefixed by an IRI and rejected.
func parsePoint(s string) (lat, lon float64, ok bool) {
	inner, found := strings.CutPrefix(s, "Point(")
	if !found {
		return 0, 0, false
	}
	inner, found = strings.CutSuffix(inner, ")")
	if !found {
		return 0, 0, false
	}
	fields := strings.Fields(inner)
	if len(fields) != 2 {
		return 0, 0, false
	}
	lon, err1 := strconv.ParseFloat(fields[0], 64)
	lat, err2 := strconv.ParseFloat(fields[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// Encode writes the document as indented JSON without HTML escaping.
func Encode(f *country.File) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type set map[string]struct{}

func (s set) add(v string) {
	if v = strings.TrimSpace(v); v != "" {
		s[v] = struct{}{}
	}
}

func (s set) sorted() []string {
	out := make([]string, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func (s set) first() string {
	if v := s.sorted(); len(v) > 0 {
		return v[0]
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
