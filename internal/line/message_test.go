package line

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// assertGolden compares the JSON encoding of v with testdata/messages/<name>
// structurally, so that key order and whitespace do not matter but missing or
// extra keys do.
func assertGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "messages", name))
	if err != nil {
		t.Fatal(err)
	}
	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("golden %s: %v", name, err)
	}
	if !reflect.DeepEqual(gotV, wantV) {
		t.Errorf("%s mismatch\n got: %s\nwant: %s", name, got, want)
	}
}

func TestMessageGolden(t *testing.T) {
	tests := []struct {
		golden string
		msg    Message
	}{
		{"text.json", TextMessage{Text: "[日本] ですね🤔"}},
		{"sticker.json", StickerMessage{PackageID: "446", StickerID: "1999"}},
		{"buttons_image.json", TemplateMessage{
			AltText: "日本",
			Template: ButtonsTemplate{
				ThumbnailImageURL:    "https://example.com/flags/jp.png",
				ImageAspectRatio:     ImageAspectRatioRectangle,
				ImageSize:            ImageSizeContain,
				ImageBackgroundColor: "#FFFFFF",
				Title:                "日本",
				Text:                 "東京 / Tokyo",
				Actions: []Action{
					URIAction{Label: "Wikipedia", URI: "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC"},
					URIAction{Label: "Google Maps", URI: "https://www.google.com/maps/search/?api=1&query=35.68,139.76"},
				},
			},
		}},
		{"buttons_no_image.json", TemplateMessage{
			AltText: "クイズです",
			Template: ButtonsTemplate{
				Text: "この国はどこでしょうか？",
				Actions: []Action{
					PostbackAction{Label: "日本", Data: "answer=%E6%97%A5%E6%9C%AC&result=1"},
				},
			},
		}},
		{"confirm.json", TemplateMessage{
			AltText: "もう一度やりますか？",
			Template: ConfirmTemplate{
				Text: "もう一度やりますか？",
				Actions: []Action{
					MessageAction{Label: "いいえ", Text: "いいえ"},
					MessageAction{Label: "はい", Text: "クイズ"},
				},
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			assertGolden(t, tt.golden, tt.msg)
		})
	}
}

func TestActionGolden(t *testing.T) {
	assertGolden(t, "actions.json", []Action{
		PostbackAction{Label: "フランス", Data: "answer=%E3%83%95%E3%83%A9%E3%83%B3%E3%82%B9&result=0"},
		MessageAction{Label: "はい", Text: "クイズ"},
		URIAction{Label: "Wikipedia", URI: "https://ja.wikipedia.org/wiki/%E3%83%95%E3%83%A9%E3%83%B3%E3%82%B9"},
		PostbackAction{Label: "日本", Data: "answer=%E6%97%A5%E6%9C%AC&result=1", DisplayText: "日本"},
	})
}

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"日本", 2},
		{"😀", 2},
		{"a😀b", 4},
	}
	for _, tt := range tests {
		if got := UTF16Len(tt.in); got != tt.want {
			t.Errorf("UTF16Len(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestTruncateUTF16(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 3, "abc"},
		{"abc", 2, "ab"},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"日本国", 2, "日本"},
		// A surrogate pair is never split: the emoji needs 2 units.
		{"😀😀", 3, "😀"},
		{"😀😀", 4, "😀😀"},
		{"a😀", 2, "a"},
		{"a😀", 3, "a😀"},
		{"😀", 1, ""},
	}
	for _, tt := range tests {
		got := TruncateUTF16(tt.in, tt.max)
		if got != tt.want {
			t.Errorf("TruncateUTF16(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
		}
		if tt.max >= 0 && UTF16Len(got) > tt.max {
			t.Errorf("TruncateUTF16(%q, %d) length %d exceeds max", tt.in, tt.max, UTF16Len(got))
		}
	}
}
