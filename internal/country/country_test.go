package country

import (
	_ "embed"
	"math/rand/v2"
	"strings"
	"testing"
)

//go:embed testdata/fixture.json
var fixtureJSON []byte

func loadFixture(t *testing.T) *Store {
	t.Helper()
	s, err := Load(fixtureJSON)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestLoadMetadata(t *testing.T) {
	s := loadFixture(t)
	if s.Len() != 6 {
		t.Errorf("Len = %d, want 6", s.Len())
	}
	if s.GeneratedAt() != "2026-01-01" || s.Source() != "test fixture" {
		t.Errorf("metadata = %q %q", s.GeneratedAt(), s.Source())
	}
	codes := []string{}
	for _, c := range s.All() {
		codes = append(codes, c.Code)
	}
	if got := strings.Join(codes, ","); got != "CD,CG,FR,JP,NR,US" {
		t.Errorf("codes = %s", got)
	}
}

func TestLookup(t *testing.T) {
	s := loadFixture(t)
	tests := []struct {
		in   string
		want string // expected code, "" for no match
	}{
		{"日本", "JP"},
		{"日本国", "JP"},
		{"にほん", "JP"},
		{"Japan", "JP"},
		{"japan", "JP"},
		{"JAPAN", "JP"},
		{" 日 本 ", "JP"},
		{"日　本", "JP"},
		{"アメリカ", "US"},
		{"米国", "US"},
		{"usa", "US"},
		{"unitedstatesofamerica", "US"},
		{"United States of America", "US"},
		{"フランス共和国", "FR"},
		{"コンゴ民主共和国", "CD"},
		{"コンゴ共和国", "CG"},
		// Shared alias between two countries is ambiguous and not indexed.
		{"コンゴ", ""},
		{"日", ""},
		{"", ""},
		{"Atlantis", ""},
		// Full-width ASCII is not folded (only ASCII is lowercased).
		{"ＪＡＰＡＮ", ""},
	}
	for _, tt := range tests {
		got := s.Lookup(tt.in)
		switch {
		case tt.want == "" && got != nil:
			t.Errorf("Lookup(%q) = %s, want nil", tt.in, got.Code)
		case tt.want != "" && got == nil:
			t.Errorf("Lookup(%q) = nil, want %s", tt.in, tt.want)
		case tt.want != "" && got.Code != tt.want:
			t.Errorf("Lookup(%q) = %s, want %s", tt.in, got.Code, tt.want)
		}
	}
}

func TestByName(t *testing.T) {
	s := loadFixture(t)
	if c := s.ByName("アメリカ合衆国"); c == nil || c.Code != "US" {
		t.Errorf("ByName(アメリカ合衆国) = %v", c)
	}
	// ByName matches only display names, not aliases.
	if c := s.ByName("アメリカ"); c != nil {
		t.Errorf("ByName(アメリカ) = %s, want nil", c.Code)
	}
}

func TestNormalizeKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Japan", "japan"},
		{" a\tb\nc\r　D ", "abcd"},
		{"日本", "日本"},
		{"ÄB", "Äb"},
	}
	for _, tt := range tests {
		if got := NormalizeKey(tt.in); got != tt.want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPickN(t *testing.T) {
	s := loadFixture(t)
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 50; i++ {
		got := s.PickN(r, 4)
		if len(got) != 4 {
			t.Fatalf("len = %d, want 4", len(got))
		}
		seen := map[string]bool{}
		for _, c := range got {
			if seen[c.Code] {
				t.Fatalf("duplicate %s in %v", c.Code, got)
			}
			seen[c.Code] = true
		}
	}
	all := s.PickN(r, 10)
	if len(all) != s.Len() {
		t.Errorf("PickN(10) len = %d, want %d", len(all), s.Len())
	}
	if got := s.PickN(r, 0); len(got) != 0 {
		t.Errorf("PickN(0) = %v", got)
	}
	// The store order is not changed by picking.
	if s.All()[0].Code != "CD" {
		t.Errorf("All()[0] = %s, store order changed", s.All()[0].Code)
	}
}

func TestPickNDeterministic(t *testing.T) {
	s := loadFixture(t)
	a := s.PickN(rand.New(rand.NewPCG(7, 7)), 4)
	b := s.PickN(rand.New(rand.NewPCG(7, 7)), 4)
	for i := range a {
		if a[i].Code != b[i].Code {
			t.Fatalf("same seed gave %v and %v", a, b)
		}
	}
}

func TestURLs(t *testing.T) {
	s := loadFixture(t)
	jp := s.Lookup("日本")
	if got := jp.FlagURL("https://bot.example.com"); got != "https://bot.example.com/flags/jp.png" {
		t.Errorf("FlagURL = %s", got)
	}
	if got := jp.FlagURL("https://bot.example.com/"); got != "https://bot.example.com/flags/jp.png" {
		t.Errorf("FlagURL trailing slash = %s", got)
	}
	if got := jp.MapURL(); got != "https://www.google.com/maps/search/?api=1&query=35.689,139.692" {
		t.Errorf("MapURL = %s", got)
	}
	us := s.Lookup("usa")
	if got := us.MapURL(); got != "https://www.google.com/maps/search/?api=1&query=38.895,-77.036667" {
		t.Errorf("MapURL = %s", got)
	}
	if got := jp.WikipediaURL(); got != "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC" {
		t.Errorf("WikipediaURL = %s", got)
	}
	noURL := &Country{Code: "XX", NameJA: "架空 国/テスト"}
	if got := noURL.WikipediaURL(); got != "https://ja.wikipedia.org/wiki/%E6%9E%B6%E7%A9%BA%20%E5%9B%BD%2F%E3%83%86%E3%82%B9%E3%83%88" {
		t.Errorf("WikipediaURL fallback = %s", got)
	}
}

func TestCapitalText(t *testing.T) {
	s := loadFixture(t)
	if got := s.Lookup("日本").CapitalText(); got != "東京 / Tokyo" {
		t.Errorf("JP CapitalText = %q", got)
	}
	nr := s.Lookup("ナウル")
	if nr.HasCapital() {
		t.Error("NR HasCapital = true")
	}
	if got := nr.CapitalText(); got != "首都情報なし" {
		t.Errorf("NR CapitalText = %q", got)
	}
	if got := (&Country{CapitalJA: "東京"}).CapitalText(); got != "東京" {
		t.Errorf("ja only = %q", got)
	}
	if got := (&Country{CapitalEN: "Tokyo"}).CapitalText(); got != "Tokyo" {
		t.Errorf("en only = %q", got)
	}
}

func TestLoadValidation(t *testing.T) {
	tests := map[string]string{
		"invalid json":   `{`,
		"no countries":   `{"countries":[]}`,
		"lowercase code": `{"countries":[{"code":"jp","name_ja":"日本"}]}`,
		"long code":      `{"countries":[{"code":"JPN","name_ja":"日本"}]}`,
		"duplicate code": `{"countries":[{"code":"JP","name_ja":"日本"},{"code":"JP","name_ja":"日本国"}]}`,
		"empty name":     `{"countries":[{"code":"JP","name_ja":""}]}`,
		"duplicate name": `{"countries":[{"code":"JP","name_ja":"日本"},{"code":"JQ","name_ja":"日本"}]}`,
	}
	for name, body := range tests {
		if _, err := Load([]byte(body)); err == nil {
			t.Errorf("%s: err = nil, want error", name)
		}
	}
}
