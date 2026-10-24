package main

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

func loadResults(t *testing.T) *SPARQLResults {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sparql.json"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ParseResults(data)
	if err != nil {
		t.Fatalf("ParseResults: %v", err)
	}
	return res
}

func parseOverrides(t *testing.T, s string) *Overrides {
	t.Helper()
	o, err := ParseOverrides([]byte(s))
	if err != nil {
		t.Fatalf("ParseOverrides: %v", err)
	}
	return o
}

// fixtureOverrides resolves every problem in testdata/sparql.json.
const fixtureOverrides = `{
  "exclude": [],
  "countries": {
    "ZA": {"capital": "Q3926", "note": "行政首都プレトリアを採用"},
    "XB": {"name_ja": "長い連邦", "aliases_ja": ["ロングランド"], "note": "表示名がボタンの上限を超えるため短縮"}
  }
}`

func byCode(f *country.File) map[string]*country.Country {
	m := map[string]*country.Country{}
	for _, c := range f.Countries {
		m[c.Code] = c
	}
	return m
}

func TestBuild(t *testing.T) {
	f, err := Build(loadResults(t), parseOverrides(t, fixtureOverrides), "2026-10-24")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if f.GeneratedAt != "2026-10-24" || f.Source != Source {
		t.Errorf("metadata = %q %q", f.GeneratedAt, f.Source)
	}
	codes := []string{}
	for _, c := range f.Countries {
		codes = append(codes, c.Code)
	}
	if got := strings.Join(codes, ","); got != "JP,KP,KR,NR,XA,XB,ZA" {
		t.Errorf("codes = %s (want sorted, aggregated)", got)
	}
	m := byCode(f)

	jp := m["JP"]
	want := &country.Country{
		Code:           "JP",
		NameJA:         "日本",
		AliasesJA:      []string{"にっぽん", "日本国"},
		AliasesEN:      []string{"Japan"},
		CapitalJA:      "東京",
		CapitalEN:      "Tokyo",
		Lat:            35.689,
		Lon:            139.692,
		WikipediaJAURL: "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC",
	}
	if !reflect.DeepEqual(jp, want) {
		t.Errorf("JP =\n %+v\nwant\n %+v", jp, want)
	}

	if got := m["KR"].CapitalJA; got != "ソウル" {
		t.Errorf("KR capital = %q (特別市 suffix)", got)
	}
	if got := m["KP"].CapitalJA; got != "平壌" {
		t.Errorf("KP capital = %q (市 suffix)", got)
	}
	if got := m["XB"].CapitalJA; got != "長" {
		t.Errorf("XB capital = %q (都 suffix)", got)
	}

	za := m["ZA"]
	if za.CapitalJA != "プレトリア" || za.CapitalEN != "Pretoria" || za.Lat != -25.746 || za.Lon != 28.188 {
		t.Errorf("ZA = %+v", za)
	}

	nr := m["NR"]
	if nr.HasCapital() || nr.Lat != -0.5228 || nr.Lon != 166.9315 {
		t.Errorf("NR (no capital, country coords) = %+v", nr)
	}

	xa := m["XA"]
	if xa.NameJA != "架空国" {
		t.Errorf("XA name (no article → ja label) = %q", xa.NameJA)
	}
	if xa.WikipediaJAURL != "https://ja.wikipedia.org/wiki/%E6%9E%B6%E7%A9%BA%E5%9B%BD" {
		t.Errorf("XA wikipedia fallback = %q", xa.WikipediaJAURL)
	}

	xb := m["XB"]
	if xb.NameJA != "長い連邦" {
		t.Errorf("XB name override = %q", xb.NameJA)
	}
	// The original label stays searchable and the override alias is added.
	if !reflect.DeepEqual(xb.AliasesJA, []string{"とても長い名前を持つ架空の連邦共和国と諸島の連合", "ロングランド"}) {
		t.Errorf("XB aliases = %v", xb.AliasesJA)
	}

	// The output must load in the runtime package.
	out, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := country.Load(out); err != nil {
		t.Errorf("country.Load(output): %v", err)
	}
}

func TestBuildValidationErrors(t *testing.T) {
	_, err := Build(loadResults(t), parseOverrides(t, `{}`), "2026-10-24")
	if err == nil {
		t.Fatal("Build without overrides: err = nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"ZA: 3 capitals",
		"Q3926", "Q5465", "Q37701",
		"XB: name_ja",
		"exceeds 20",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "JP:") || strings.Contains(msg, "NR:") {
		t.Errorf("error mentions valid countries:\n%s", msg)
	}
}

func TestBuildOverrideErrors(t *testing.T) {
	tests := map[string]struct {
		overrides string
		want      string
	}{
		"unknown capital qid": {
			`{"countries":{"ZA":{"capital":"Q1"},"XB":{"name_ja":"短い"}}}`,
			"ZA: capital Q1 is not",
		},
		"stale code": {
			`{"countries":{"ZA":{"capital":"Q3926"},"XB":{"name_ja":"短い"},"QQ":{"name_ja":"x"}}}`,
			"QQ: override for a code",
		},
		"stale exclude": {
			`{"exclude":["QQ"],"countries":{"ZA":{"capital":"Q3926"},"XB":{"name_ja":"短い"}}}`,
			"QQ: excluded code",
		},
		"long override name": {
			`{"countries":{"ZA":{"capital":"Q3926"},"XB":{"name_ja":"あいうえおかきくけこさしすせそたちつてとな"}}}`,
			"XB: name_ja",
		},
		"duplicate display name": {
			`{"countries":{"ZA":{"capital":"Q3926"},"XB":{"name_ja":"日本"}}}`,
			"display name 日本",
		},
		"several capitals with manual name and coordinates but no qid": {
			`{"countries":{"ZA":{"capital_ja":"プレトリア","capital_en":"Pretoria","lat":-25.74,"lon":28.19},"XB":{"name_ja":"短い"}}}`,
			"ZA: 3 capitals",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Build(loadResults(t), parseOverrides(t, tt.overrides), "2026-10-24")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBuildExcludeAndManualCapital(t *testing.T) {
	o := parseOverrides(t, `{
	  "exclude": ["XB"],
	  "countries": {
	    "ZA": {"capital": "Q3926"},
	    "NR": {"capital_ja": "ヤレン", "capital_en": "Yaren", "lat": -0.547, "lon": 166.92, "note": "事実上の首都"}
	  }
	}`)
	f, err := Build(loadResults(t), o, "2026-10-24")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	m := byCode(f)
	if _, ok := m["XB"]; ok {
		t.Error("XB not excluded")
	}
	nr := m["NR"]
	if nr.CapitalJA != "ヤレン" || nr.CapitalEN != "Yaren" || nr.Lat != -0.547 || nr.Lon != 166.92 {
		t.Errorf("NR = %+v", nr)
	}
}

func TestBuildCapitalQIDWithManualName(t *testing.T) {
	o := parseOverrides(t, `{
	  "countries": {
	    "ZA": {"capital": "Q3926", "capital_ja": "プレトリア（行政首都）", "lat": -25.74, "lon": 28.19},
	    "XB": {"name_ja": "短い"}
	  }
	}`)
	f, err := Build(loadResults(t), o, "2026-10-24")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	za := byCode(f)["ZA"]
	if za.CapitalJA != "プレトリア（行政首都）" || za.Lat != -25.74 || za.Lon != 28.19 {
		t.Errorf("ZA = %+v", za)
	}
}

func TestBuildRemoveAliasesAndWikipediaURL(t *testing.T) {
	o := parseOverrides(t, `{
	  "countries": {
	    "ZA": {"capital": "Q3926"},
	    "XB": {"name_ja": "長い連邦"},
	    "JP": {"remove_aliases": ["にっぽん", "存在しない別名"], "wikipedia_ja_url": "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC%E5%9B%BD"}
	  }
	}`)
	f, err := Build(loadResults(t), o, "2026-10-24")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	jp := byCode(f)["JP"]
	if !reflect.DeepEqual(jp.AliasesJA, []string{"日本国"}) {
		t.Errorf("JP aliases = %v", jp.AliasesJA)
	}
	if jp.WikipediaJAURL != "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC%E5%9B%BD" {
		t.Errorf("JP wikipedia = %s", jp.WikipediaJAURL)
	}
}

func TestSharedAliases(t *testing.T) {
	f := &country.File{Countries: []*country.Country{
		{Code: "CN", NameJA: "中華人民共和国", AliasesJA: []string{"中国"}},
		{Code: "TW", NameJA: "中華民国", AliasesJA: []string{"中国", "台湾"}},
		{Code: "US", NameJA: "アメリカ合衆国", AliasesEN: []string{"America"}},
		{Code: "XA", NameJA: "アメリカ連合", AliasesEN: []string{"AMERICA"}},
		// An alias equal to another country's display name is not ambiguous:
		// the display name wins at lookup time.
		{Code: "XB", NameJA: "台湾", AliasesJA: []string{"中華民国"}},
	}}
	got := SharedAliases(f)
	want := []string{"america (US, XA)", "中国 (CN, TW)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SharedAliases = %q, want %q", got, want)
	}
}

func TestBuildDuplicateCountryItems(t *testing.T) {
	res := loadResults(t)
	extra := cloneBinding(res.Results.Bindings[0])
	extra["country"] = SPARQLValue{Type: "uri", Value: "http://www.wikidata.org/entity/Q999"}
	res.Results.Bindings = append(res.Results.Bindings, extra)

	_, err := Build(res, parseOverrides(t, fixtureOverrides), "2026-10-24")
	if err == nil || !strings.Contains(err.Error(), "JP: 2 items") {
		t.Fatalf("err = %v, want duplicate items error", err)
	}

	o := parseOverrides(t, `{"countries":{"JP":{"qid":"Q17"},"ZA":{"capital":"Q3926"},"XB":{"name_ja":"短い"}}}`)
	if _, err := Build(res, o, "2026-10-24"); err != nil {
		t.Errorf("Build with qid override: %v", err)
	}
}

func cloneBinding(b map[string]SPARQLValue) map[string]SPARQLValue {
	out := make(map[string]SPARQLValue, len(b))
	for k, v := range b {
		out[k] = v
	}
	return out
}

func TestBuildDeterministic(t *testing.T) {
	o := parseOverrides(t, fixtureOverrides)
	res := loadResults(t)
	f1, err := Build(res, o, "2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	out1, err := Encode(f1)
	if err != nil {
		t.Fatal(err)
	}

	r := rand.New(rand.NewPCG(3, 4))
	for i := 0; i < 10; i++ {
		shuffled := loadResults(t)
		b := shuffled.Results.Bindings
		r.Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })
		f2, err := Build(shuffled, o, "2026-10-24")
		if err != nil {
			t.Fatal(err)
		}
		out2, err := Encode(f2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out1, out2) {
			t.Fatalf("output differs after shuffling rows:\n%s\n---\n%s", out1, out2)
		}
	}
}

func TestEncodeFormat(t *testing.T) {
	f, err := Build(loadResults(t), parseOverrides(t, fixtureOverrides), "2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("{\n  \"generated_at\": \"2026-10-24\",\n  \"source\": ")) {
		t.Errorf("unexpected header:\n%s", out[:80])
	}
	if !bytes.HasSuffix(out, []byte("}\n")) {
		t.Error("output does not end with a newline")
	}
	if bytes.Contains(out, []byte(`\u0026`)) {
		t.Error("output escapes HTML characters")
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatal(err)
	}
}

func TestTrimCapitalSuffix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"東京都", "東京"},
		{"平壌市", "平壌"},
		{"ソウル特別市", "ソウル"},
		{"パリ", "パリ"},
		{"市", "市"},
		{"都", "都"},
		{"特別市", "特別市"},
	}
	for _, tt := range tests {
		if got := trimCapitalSuffix(tt.in); got != tt.want {
			t.Errorf("trimCapitalSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParsePoint(t *testing.T) {
	lat, lon, ok := parsePoint("Point(139.692 35.689)")
	if !ok || lat != 35.689 || lon != 139.692 {
		t.Errorf("parsePoint = %v %v %v", lat, lon, ok)
	}
	for _, bad := range []string{"", "Point()", "Point(1)", "Point(a b)", "<http://www.wikidata.org/entity/Q405> Point(1 2)"} {
		if _, _, ok := parsePoint(bad); ok {
			t.Errorf("parsePoint(%q) ok = true", bad)
		}
	}
}

func TestParseOverridesRejectsUnknownFields(t *testing.T) {
	if _, err := ParseOverrides([]byte(`{"countries":{"JP":{"nmae_ja":"x"}}}`)); err == nil {
		t.Error("typo field accepted")
	}
}

func TestCheckedInOverridesParse(t *testing.T) {
	data, err := os.ReadFile("overrides.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOverrides(data); err != nil {
		t.Errorf("overrides.json: %v", err)
	}
}
