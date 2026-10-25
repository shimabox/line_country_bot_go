package line

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertJSONString(t *testing.T, s string) {
	t.Helper()
	want, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal(%q): %v", s, err)
	}
	if got := appendJSONString(nil, s); string(got) != string(want) {
		t.Errorf("appendJSONString(%q)\n got: %s\nwant: %s", s, got, want)
	}
}

func TestAppendJSONString(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"ascii", "Hello, world! 0123 ~`'"},
		{"japanese", "日本 / 東京 / クイズです"},
		{"emoji", "😀🤔🇯🇵👨‍👩‍👧"},
		{"quote and backslash", `"a\b"\\`},
		{"short escapes", "\b\f\n\r\t"},
		{"DEL", "\x7f"},
		{"html", "<script>&amp;</script>"},
		{"line and paragraph separators", "a\xe2\x80\xa8b\xe2\x80\xa9c"},
		{"U+2027 and U+202A", "\xe2\x80\xa7\xe2\x80\xaa"},
		{"valid U+FFFD", "\xef\xbf\xbd"},
		{"invalid byte", "\xff"},
		{"invalid bytes inside text", "a\xffb\xfe\xc0c"},
		{"truncated multibyte", "日本"[:4]},
		{"lone continuation", "\x80\xbf"},
		{"overlong", "\xc0\xaf"},
		{"surrogate half", "\xed\xa0\x80"},
		{"beyond U+10FFFF", "\xf4\x90\x80\x80"},
		{"special text", specialText},
		{"long", strings.Repeat("あa\n<", 1250)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONString(t, tt.in)
		})
	}
	t.Run("control characters", func(t *testing.T) {
		for c := range 0x20 {
			assertJSONString(t, string(rune(c)))
			assertJSONString(t, "x"+string(rune(c))+"y")
		}
	})
	t.Run("every byte", func(t *testing.T) {
		for c := range 0x100 {
			assertJSONString(t, string([]byte{byte(c)}))
		}
	})
}

func TestAppendJSONStringAppends(t *testing.T) {
	got := appendJSONString([]byte("prefix:"), "a<b")
	if want := `prefix:"a\` + `u003cb"`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func FuzzAppendJSONString(f *testing.F) {
	for _, s := range []string{"", "a", "日本😀", specialText, "\xff\xfe", "\xe2\x80\xa8\xe2\x80\xa9", "\xef\xbf\xbd", "\xed\xa0\x80"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		want, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal(%q): %v", s, err)
		}
		if got := appendJSONString(nil, s); string(got) != string(want) {
			t.Errorf("appendJSONString(%q)\n got: %s\nwant: %s", s, got, want)
		}
	})
}
