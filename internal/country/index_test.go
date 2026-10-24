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

// TestLookupEveryNameAndAlias checks every display name and alias against an
// expectation built here without BuildIndex: a display name always finds its
// country, an alias finds its only owner unless it is also a display name,
// and an alias owned by several countries finds nothing.
func TestLookupEveryNameAndAlias(t *testing.T) {
	for name, data := range dataSets(t) {
		f := decodeFile(t, data)
		s, err := Load(data)
		if err != nil {
			t.Fatalf("%s: Load: %v", name, err)
		}

		nameCode := map[string]string{}            // key -> code
		aliasCodes := map[string]map[string]bool{} // key -> owner codes
		for _, c := range f.Countries {
			nameCode[NormalizeKey(c.NameJA)] = c.Code
		}
		for _, c := range f.Countries {
			for _, a := range append(append([]string{}, c.AliasesJA...), c.AliasesEN...) {
				k := NormalizeKey(a)
				if k == "" {
					continue
				}
				if aliasCodes[k] == nil {
					aliasCodes[k] = map[string]bool{}
				}
				aliasCodes[k][c.Code] = true
			}
		}
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
		t.Logf("%s: %d names, %d alias keys, %d shared", name, len(nameCode), len(aliasCodes), shared)
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
