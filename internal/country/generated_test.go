package country

import (
	"os"
	"reflect"
	"testing"
)

const regenerate = "countries_gen.go is out of date with data/countries.json; run: go generate ./internal/country"

// TestGeneratedMatchesJSON fails when countries_gen.go was not regenerated
// after data/countries.json changed.
func TestGeneratedMatchesJSON(t *testing.T) {
	data, err := os.ReadFile("data/countries.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Load(data)
	if err != nil {
		t.Fatalf("Load(data/countries.json): %v", err)
	}
	got := Default()

	if got.GeneratedAt() != want.GeneratedAt() || got.Source() != want.Source() {
		t.Errorf("metadata = %q %q, want %q %q\n%s",
			got.GeneratedAt(), got.Source(), want.GeneratedAt(), want.Source(), regenerate)
	}
	if got.Len() != want.Len() {
		t.Fatalf("%d countries, want %d\n%s", got.Len(), want.Len(), regenerate)
	}
	for i, c := range want.All() {
		if g := got.All()[i]; !reflect.DeepEqual(g, c) {
			t.Errorf("country %d = %+v, want %+v\n%s", i, *g, *c, regenerate)
		}
	}
	if !reflect.DeepEqual(got.index, want.index) {
		t.Errorf("index differs (%d entries, want %d)\n%s", len(got.index), len(want.index), regenerate)
	}
}

func TestDefaultIsShared(t *testing.T) {
	if Default() != Default() {
		t.Error("Default returned different stores")
	}
	if a, b := Default().All()[0], &countryTable[0]; a != b {
		t.Error("Default does not point into countryTable")
	}
}
