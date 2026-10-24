package country

import (
	"strings"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/line"
)

// TestEmbeddedData checks the generated data set against the limits the bot
// relies on, so a regeneration that breaks them fails here.
func TestEmbeddedData(t *testing.T) {
	s, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if s.Len() < 190 {
		t.Errorf("Len = %d, want >= 190", s.Len())
	}
	if s.GeneratedAt() == "" || !strings.Contains(s.Source(), "Wikidata") {
		t.Errorf("metadata = %q %q", s.GeneratedAt(), s.Source())
	}
	prev := ""
	for _, c := range s.All() {
		if c.Code <= prev {
			t.Errorf("codes not strictly ascending: %s after %s", c.Code, prev)
		}
		prev = c.Code
		if n := line.UTF16Len(c.NameJA); n == 0 || n > line.MaxActionLabel {
			t.Errorf("%s: name_ja %q has %d UTF-16 units", c.Code, c.NameJA, n)
		}
		if !strings.HasPrefix(c.WikipediaJAURL, "https://ja.wikipedia.org/wiki/") {
			t.Errorf("%s: wikipedia_ja_url = %q", c.Code, c.WikipediaJAURL)
		}
		if n := line.UTF16Len(c.CapitalText()); n > line.MaxButtonsTextImage {
			t.Errorf("%s: capital text %q has %d UTF-16 units", c.Code, c.CapitalText(), n)
		}
		if c.Lat < -90 || c.Lat > 90 || c.Lon < -180 || c.Lon > 180 || (c.Lat == 0 && c.Lon == 0) {
			t.Errorf("%s: coordinates %v,%v", c.Code, c.Lat, c.Lon)
		}
	}
}

func TestEmbeddedDataLookups(t *testing.T) {
	s, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"日本":     "JP",
		"japan":  "JP",
		"アメリカ":   "US",
		"中国":     "CN",
		"台湾":     "TW",
		"韓国":     "KR",
		"北朝鮮":    "KP",
		"オランダ":   "NL",
		"イギリス":   "GB",
		"ジョージア":  "GE",
		"パレスチナ":  "PS",
		"フランス":   "FR",
		"France": "FR",
	}
	for in, want := range tests {
		got := s.Lookup(in)
		if got == nil || got.Code != want {
			t.Errorf("Lookup(%q) = %v, want %s", in, got, want)
		}
	}
	if jp := s.Lookup("日本"); jp.CapitalJA != "東京" {
		t.Errorf("JP capital = %q", jp.CapitalJA)
	}
}
