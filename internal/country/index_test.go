package country

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// dataSets are the documents the index tests run on: the small fixture with
// a shared alias, and the real data set.
func dataSets(t *testing.T) map[string][]byte {
	t.Helper()
	data, err := os.ReadFile("data/countries.json")
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"fixture": fixtureJSON, "countries.json": data}
}

func decodeFile(t *testing.T, data []byte) File {
	t.Helper()
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestBuildIndexSortedAndUnique(t *testing.T) {
	for name, data := range dataSets(t) {
		f := decodeFile(t, data)
		index, err := BuildIndex(f.Countries)
		if err != nil {
			t.Fatalf("%s: BuildIndex: %v", name, err)
		}
		if len(index) < len(f.Countries) {
			t.Errorf("%s: %d entries for %d countries", name, len(index), len(f.Countries))
		}
		for i, e := range index {
			if e.Key == "" {
				t.Errorf("%s: entry %d has an empty key", name, i)
			}
			if e.Index < 0 || e.Index >= len(f.Countries) {
				t.Errorf("%s: entry %q has index %d out of range", name, e.Key, e.Index)
			}
			if i > 0 && strings.Compare(index[i-1].Key, e.Key) >= 0 {
				t.Errorf("%s: keys not strictly ascending: %q then %q", name, index[i-1].Key, e.Key)
			}
		}
	}
}

func TestBuildIndexRejectsSharedName(t *testing.T) {
	_, err := BuildIndex([]*Country{{Code: "JP", NameJA: "日本"}, {Code: "JQ", NameJA: " 日本"}})
	if err == nil {
		t.Error("err = nil, want an error for two names with the same key")
	}
}

// ownerCodes collects, for each normalized non-empty key of names(c), the
// codes of the countries that have it.
func ownerCodes(countries []*Country, names func(c *Country) []string) map[string]map[string]bool {
	owners := map[string]map[string]bool{}
	for _, c := range countries {
		for _, n := range names(c) {
			k := NormalizeKey(n)
			if k == "" {
				continue
			}
			if owners[k] == nil {
				owners[k] = map[string]bool{}
			}
			owners[k][c.Code] = true
		}
	}
	return owners
}

// TestLookupEveryNameAndAlias checks every display name, alias and capital
// against an expectation built here without BuildIndex: a display name always
// finds its country, an alias finds its only owner unless it is also a
// display name, a capital finds its only owner unless it is also a display
// name or an alias, and an alias or capital owned by several countries finds
// nothing.
func TestLookupEveryNameAndAlias(t *testing.T) {
	for name, data := range dataSets(t) {
		f := decodeFile(t, data)
		s, err := Load(data)
		if err != nil {
			t.Fatalf("%s: Load: %v", name, err)
		}

		nameCode := map[string]string{} // key -> code
		for _, c := range f.Countries {
			nameCode[NormalizeKey(c.NameJA)] = c.Code
		}
		aliasCodes := ownerCodes(f.Countries, func(c *Country) []string {
			return append(append([]string{}, c.AliasesJA...), c.AliasesEN...)
		})
		capitalCodes := ownerCodes(f.Countries, func(c *Country) []string {
			return []string{c.CapitalJA, c.CapitalEN}
		})
		want := map[string]string{} // key -> code, "" for no match
		for k, code := range nameCode {
			want[k] = code
		}
		shared := 0
		for k, owners := range aliasCodes {
			if _, isName := nameCode[k]; isName {
				continue
			}
			if len(owners) > 1 {
				want[k] = ""
				shared++
				continue
			}
			for code := range owners {
				want[k] = code
			}
		}
		capitalKeys, sharedCapitals, shadowedCapitals := 0, 0, 0
		for k, owners := range capitalCodes {
			if _, taken := want[k]; taken {
				shadowedCapitals++
				continue
			}
			if len(owners) > 1 {
				want[k] = ""
				sharedCapitals++
				continue
			}
			for code := range owners {
				want[k] = code
			}
			capitalKeys++
		}
		t.Logf("%s: %d names, %d alias keys, %d shared; %d capital keys added, %d shadowed by a name or alias, %d shared",
			name, len(nameCode), len(aliasCodes), shared, capitalKeys, shadowedCapitals, sharedCapitals)
		if name == "fixture" && shared == 0 {
			t.Errorf("fixture: no shared alias, the nil case is not covered")
		}

		check := func(in string) {
			t.Helper()
			code := want[NormalizeKey(in)]
			got := s.Lookup(in)
			switch {
			case code == "" && got != nil:
				t.Errorf("%s: Lookup(%q) = %s, want nil", name, in, got.Code)
			case code != "" && got == nil:
				t.Errorf("%s: Lookup(%q) = nil, want %s", name, in, code)
			case code != "" && got.Code != code:
				t.Errorf("%s: Lookup(%q) = %s, want %s", name, in, got.Code, code)
			}
		}
		for _, c := range f.Countries {
			check(c.NameJA)
			for _, a := range c.AliasesJA {
				check(a)
			}
			for _, a := range c.AliasesEN {
				check(a)
				check(strings.ToUpper(a))
			}
			if c.CapitalJA != "" {
				check(c.CapitalJA)
			}
			if c.CapitalEN != "" {
				check(c.CapitalEN)
				check(strings.ToUpper(c.CapitalEN))
			}
		}

		// The index has exactly the expected keys.
		entries := 0
		for _, code := range want {
			if code != "" {
				entries++
			}
		}
		index, err := BuildIndex(f.Countries)
		if err != nil {
			t.Fatal(err)
		}
		if len(index) != entries {
			t.Errorf("%s: index has %d entries, want %d", name, len(index), entries)
		}
	}
}

// TestByNameMatchesLookup fixes ByName to the exact display-name match: it
// finds a country only when the input is the display name as is, even if a
// normalized form or an alias finds it with Lookup.
func TestByNameMatchesLookup(t *testing.T) {
	for name, data := range dataSets(t) {
		f := decodeFile(t, data)
		s, err := Load(data)
		if err != nil {
			t.Fatalf("%s: Load: %v", name, err)
		}
		byName := map[string]*Country{}
		for _, c := range f.Countries {
			byName[c.NameJA] = c
		}

		inputs := []string{"", " ", "Atlantis"}
		for _, c := range f.Countries {
			inputs = append(inputs,
				c.NameJA,
				" "+c.NameJA,
				c.NameJA+"　",
				strings.Join(strings.Split(c.NameJA, ""), " "),
				strings.ToUpper(c.NameJA),
			)
			inputs = append(inputs, c.AliasesJA...)
			inputs = append(inputs, c.AliasesEN...)
		}
		for _, in := range inputs {
			got, want := s.ByName(in), byName[in]
			if want == nil {
				if got != nil {
					t.Errorf("%s: ByName(%q) = %s, want nil", name, in, got.Code)
				}
				continue
			}
			if got == nil || got.Code != want.Code {
				t.Errorf("%s: ByName(%q) = %v, want %s", name, in, got, want.Code)
				continue
			}
			// A display-name match is also what Lookup finds.
			if l := s.Lookup(in); l != got {
				t.Errorf("%s: ByName(%q) = %s but Lookup = %v", name, in, got.Code, l)
			}
		}
	}
}

// capitalPriorityCountries has capitals that clash with other countries'
// names and aliases, and with each other:
//
//   - AA's capital "ビーランド" is BB's display name.
//   - BB's capital "Cee" is CC's English alias.
//   - CC and DD share the capital "ツインシティ".
//   - DD's capital_en "Dee City" is its own alias.
//   - EE's capital "Shared" is an alias of both AA and BB (ambiguous alias).
func capitalPriorityCountries() []*Country {
	return []*Country{
		{Code: "AA", NameJA: "エーランド", AliasesEN: []string{"Shared"}, CapitalJA: "ビーランド", CapitalEN: "Aa City"},
		{Code: "BB", NameJA: "ビーランド", AliasesEN: []string{"Shared"}, CapitalJA: "ビー市", CapitalEN: "Cee"},
		{Code: "CC", NameJA: "シーランド", AliasesEN: []string{"Cee"}, CapitalJA: "ツインシティ", CapitalEN: "Cee Town"},
		{Code: "DD", NameJA: "ディーランド", AliasesEN: []string{"Dee City"}, CapitalJA: "ツインシティ", CapitalEN: "Dee City"},
		{Code: "EE", NameJA: "イーランド", CapitalJA: "イー市", CapitalEN: "Shared"},
	}
}

func lookupCode(s *Store, in string) string {
	if c := s.Lookup(in); c != nil {
		return c.Code
	}
	return ""
}

func TestBuildIndexCapitals(t *testing.T) {
	countries := capitalPriorityCountries()
	index, err := BuildIndex(countries)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	s := newStore(countries, index, "", "")
	for in, want := range map[string]string{
		"Aa City":  "AA", // capital_en
		"AA CITY":  "AA", // normalized like names
		"ビー市":      "BB", // capital_ja
		"Cee Town": "CC",
		"イー市":      "EE",
		// A display name wins over another country's capital.
		"ビーランド": "BB",
		// An alias wins over another country's capital.
		"Cee": "CC",
		// A capital shared by two countries is ambiguous and left out.
		"ツインシティ": "",
		// A capital equal to the country's own alias finds the country.
		"Dee City": "DD",
		// An alias shared by two countries stays ambiguous even though it is
		// also a third country's capital: aliases come before capitals.
		"Shared": "",
	} {
		if got := lookupCode(s, in); got != want {
			t.Errorf("Lookup(%q) = %q, want %q", in, got, want)
		}
	}
	for _, e := range index {
		if e.Key == NormalizeKey("ツインシティ") || e.Key == "shared" {
			t.Errorf("ambiguous key %q is in the index", e.Key)
		}
	}
	// 5 names, the aliases cee and deecity, and the capitals aacity, ビー市,
	// ceetown and イー市.
	if len(index) != 11 {
		t.Errorf("index has %d entries, want 11: %v", len(index), index)
	}
}

func TestBuildIndexCapitalClashIsNotAnError(t *testing.T) {
	// Capitals never fail the build, even when one equals another country's
	// display name or is shared; only display names must be unique.
	countries := []*Country{
		{Code: "AA", NameJA: "エーランド", CapitalJA: "ビーランド"},
		{Code: "BB", NameJA: "ビーランド", CapitalJA: "エーランド"},
		{Code: "CC", NameJA: "シーランド", CapitalJA: "同じ", CapitalEN: "same"},
		{Code: "DD", NameJA: "ディーランド", CapitalJA: "同じ", CapitalEN: "Same"},
	}
	index, err := BuildIndex(countries)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(index) != 4 {
		t.Errorf("index has %d entries, want only the 4 names: %v", len(index), index)
	}
}

// TestEmbeddedCapitalsFindTheirCountry is a data invariant: every capital in
// the bundled data finds its own country, so no capital is shared with or
// shadowed by another country.
func TestEmbeddedCapitalsFindTheirCountry(t *testing.T) {
	data, err := os.ReadFile("data/countries.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*Store{s, Default()} {
		for _, c := range st.All() {
			for _, capital := range []string{c.CapitalJA, c.CapitalEN} {
				if capital == "" {
					t.Errorf("%s: capital missing (ja %q, en %q)", c.Code, c.CapitalJA, c.CapitalEN)
					continue
				}
				if got := lookupCode(st, capital); got != c.Code {
					t.Errorf("%s: Lookup(%q) = %q, want %s", c.Code, capital, got, c.Code)
				}
			}
		}
	}
	for in, want := range map[string]string{"東京": "JP", "Tokyo": "JP", "tokyo": "JP", "シンガポール": "SG", "パリ": "FR"} {
		if got := lookupCode(s, in); got != want {
			t.Errorf("Lookup(%q) = %q, want %s", in, got, want)
		}
	}
}
