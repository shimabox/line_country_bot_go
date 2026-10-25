package bot

import (
	"math/rand/v2"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
	"github.com/shimabox/line_country_bot_go/internal/line"
)

const baseURL = "https://bot.example.com"

var testProfile = Profile{
	ImageURL: "https://example.com/profile.png",
	Title:    "国名bot",
	Text:     "国名を送ると国旗と首都を返します",
	LinkText: "ソースコード",
	LinkURL:  "https://example.com/source",
}

func newTestBot(t *testing.T, seed uint64) *Bot {
	t.Helper()
	store := country.Default()
	return New(store, rand.New(rand.NewPCG(seed, seed)))
}

func textEvent(text string) line.Event {
	return line.Event{
		Type:       line.EventTypeMessage,
		ReplyToken: "rt",
		Message:    &line.EventMessage{ID: "1", Type: line.MessageTypeText, Text: text},
	}
}

func postbackEvent(data string) line.Event {
	return line.Event{Type: line.EventTypePostback, ReplyToken: "rt", Postback: &line.Postback{Data: data}}
}

func opts(profile Profile) Options {
	return Options{FlagBaseURL: baseURL, Profile: profile}
}

func asText(t *testing.T, m line.Message) string {
	t.Helper()
	tm, ok := m.(line.TextMessage)
	if !ok {
		t.Fatalf("message %#v is not text", m)
	}
	return tm.Text
}

func asButtons(t *testing.T, m line.Message) (line.TemplateMessage, line.ButtonsTemplate) {
	t.Helper()
	tm, ok := m.(line.TemplateMessage)
	if !ok {
		t.Fatalf("message %#v is not a template", m)
	}
	b, ok := tm.Template.(line.ButtonsTemplate)
	if !ok {
		t.Fatalf("template %#v is not buttons", tm.Template)
	}
	return tm, b
}

// hasEmojiSuffix reports whether s is prefix followed by one emoji of group.
func hasEmojiSuffix(s, prefix string, group []string) bool {
	rest, ok := strings.CutPrefix(s, prefix)
	return ok && slices.Contains(group, rest)
}

func TestNonText(t *testing.T) {
	b := newTestBot(t, 1)
	for _, typ := range []string{"image", "sticker", "video", "audio", "location", "file", "unknown-type"} {
		ev := line.Event{Type: line.EventTypeMessage, ReplyToken: "rt", Message: &line.EventMessage{Type: typ}}
		got := b.Respond(ev, opts(testProfile))
		want := []line.Message{
			line.TextMessage{Text: "テキストメッセージ以外わからないです"},
			line.StickerMessage{PackageID: "446", StickerID: "1999"},
		}
		if !reflect.DeepEqual(got.Messages, want) {
			t.Errorf("%s: messages = %#v", typ, got.Messages)
		}
		if got.Kind != KindNonText {
			t.Errorf("%s: kind = %s", typ, got.Kind)
		}
	}
}

func TestIgnoredEvents(t *testing.T) {
	b := newTestBot(t, 1)
	for _, ev := range []line.Event{
		{Type: "follow", ReplyToken: "rt"},
		{Type: "join", ReplyToken: "rt"},
		{Type: "unsend"},
		{Type: "somethingNew", ReplyToken: "rt"},
		{Type: line.EventTypeMessage, ReplyToken: "rt"}, // message without body
		{Type: line.EventTypePostback, ReplyToken: "rt"},
	} {
		got := b.Respond(ev, opts(testProfile))
		if got.Messages != nil {
			t.Errorf("%s: messages = %#v, want none", ev.Type, got.Messages)
		}
	}
}

func TestProfileReply(t *testing.T) {
	b := newTestBot(t, 1)
	want := []line.Message{line.TemplateMessage{
		AltText: "国名bot",
		Template: line.ButtonsTemplate{
			ThumbnailImageURL: "https://example.com/profile.png",
			Title:             "国名bot",
			Text:              "国名を送ると国旗と首都を返します",
			Actions:           []line.Action{line.URIAction{Label: "ソースコード", URI: "https://example.com/source"}},
		},
	}}
	for _, in := range []string{"君の名は", "who", "WHO", " Who "} {
		got := b.Respond(textEvent(in), opts(testProfile))
		if got.Kind != KindProfile || !reflect.DeepEqual(got.Messages, want) {
			t.Errorf("%q: %s %#v", in, got.Kind, got.Messages)
		}
	}
}

func TestProfileDisabledFallsBackToCountrySearch(t *testing.T) {
	b := newTestBot(t, 1)
	got := b.Respond(textEvent("君の名は"), opts(Profile{}))
	if got.Kind != KindCountryMiss || len(got.Messages) != 2 {
		t.Fatalf("kind = %s, messages = %#v", got.Kind, got.Messages)
	}
	if s := asText(t, got.Messages[0]); s != "[君の名は] ですね🤔" {
		t.Errorf("first = %q", s)
	}
}

// wantImageSize is the imageSize expected for a country's flag.
func wantImageSize(c *country.Country) string {
	if c.FlagFitsFrame() {
		return "cover"
	}
	return "contain"
}

// wantBackground is the imageBackgroundColor expected for a country's flag:
// its corner colour when it fills the frame, otherwise white.
func wantBackground(c *country.Country) string {
	if c.FlagFitsFrame() {
		return c.FlagCornerColor()
	}
	return "#FFFFFF"
}

func TestQuiz(t *testing.T) {
	imageSizes := map[string]bool{}
	backgrounds := map[string]bool{}
	for seed := uint64(0); seed < 30; seed++ {
		b := newTestBot(t, seed)
		for _, in := range []string{"クイズ", "quiz", "QUIZ"} {
			got := b.Respond(textEvent(in), opts(Profile{}))
			if got.Kind != KindQuiz || len(got.Messages) != 1 {
				t.Fatalf("%q: kind = %s, %d messages", in, got.Kind, len(got.Messages))
			}
			tm, bt := asButtons(t, got.Messages[0])
			if tm.AltText != "この国はどこでしょうか？" || bt.Title != "クイズです" || bt.Text != "この国はどこでしょうか？" {
				t.Errorf("texts = %q %q %q", tm.AltText, bt.Title, bt.Text)
			}
			if bt.ImageAspectRatio != "rectangle" {
				t.Errorf("imageAspectRatio = %q", bt.ImageAspectRatio)
			}
			if len(bt.Actions) != 4 {
				t.Fatalf("%d actions", len(bt.Actions))
			}
			labels := map[string]bool{}
			correct := ""
			for _, a := range bt.Actions {
				pa, ok := a.(line.PostbackAction)
				if !ok {
					t.Fatalf("action %#v is not postback", a)
				}
				if line.UTF16Len(pa.Label) > line.MaxActionLabel || line.UTF16Len(pa.Data) > line.MaxPostbackData {
					t.Errorf("action too long: %#v", pa)
				}
				labels[pa.Label] = true
				v, err := url.ParseQuery(pa.Data)
				if err != nil {
					t.Fatalf("data %q: %v", pa.Data, err)
				}
				if want := (url.Values{"answer": {v.Get("answer")}, "result": {v.Get("result")}}).Encode(); pa.Data != want {
					t.Errorf("data = %q, want form %q", pa.Data, want)
				}
				if v.Get("answer") != pa.Label {
					t.Errorf("answer %q != label %q", v.Get("answer"), pa.Label)
				}
				if pa.DisplayText != v.Get("answer") {
					t.Errorf("displayText %q != answer %q", pa.DisplayText, v.Get("answer"))
				}
				if line.UTF16Len(pa.DisplayText) > line.MaxPostbackDisplayText {
					t.Errorf("displayText too long: %#v", pa)
				}
				switch v.Get("result") {
				case "1":
					if correct != "" {
						t.Errorf("two correct answers: %s and %s", correct, pa.Label)
					}
					correct = pa.Label
				case "0":
				default:
					t.Errorf("result = %q", v.Get("result"))
				}
			}
			if len(labels) != 4 {
				t.Errorf("duplicate choices: %v", labels)
			}
			if correct == "" {
				t.Fatal("no correct answer")
			}
			c := b.countries.ByName(correct)
			if c == nil {
				t.Fatalf("correct answer %q not found", correct)
			}
			if bt.ThumbnailImageURL != c.FlagURL(baseURL) {
				t.Errorf("image = %s, want flag of %s", bt.ThumbnailImageURL, c.Code)
			}
			if got.Code != c.Code {
				t.Errorf("Code = %s, want %s", got.Code, c.Code)
			}
			if want := wantImageSize(c); bt.ImageSize != want {
				t.Errorf("%s: imageSize = %q, want %q", c.Code, bt.ImageSize, want)
			}
			if want := wantBackground(c); bt.ImageBackgroundColor != want {
				t.Errorf("%s: imageBackgroundColor = %q, want %q", c.Code, bt.ImageBackgroundColor, want)
			}
			imageSizes[bt.ImageSize] = true
			if bt.ImageSize == "cover" {
				backgrounds[bt.ImageBackgroundColor] = true
			}
		}
	}
	if !imageSizes["cover"] || !imageSizes["contain"] {
		t.Errorf("quizzes used only imageSize %v", imageSizes)
	}
	// Cover flags use their own corner colour, not only white.
	delete(backgrounds, "#FFFFFF")
	if len(backgrounds) == 0 {
		t.Error("cover quizzes used only a white background")
	}
}

func TestQuizShufflesCorrectPosition(t *testing.T) {
	positions := map[int]bool{}
	for seed := uint64(0); seed < 40; seed++ {
		got := newTestBot(t, seed).Respond(textEvent("クイズ"), opts(Profile{}))
		_, bt := asButtons(t, got.Messages[0])
		for i, a := range bt.Actions {
			if strings.HasSuffix(a.(line.PostbackAction).Data, "result=1") {
				positions[i] = true
			}
		}
	}
	if len(positions) != 4 {
		t.Errorf("correct answer appeared only at positions %v", positions)
	}
}

func TestCountryHit(t *testing.T) {
	b := newTestBot(t, 1)
	got := b.Respond(textEvent(" 日本\n"), opts(Profile{}))
	if got.Kind != KindCountryHit || got.Code != "JP" || len(got.Messages) != 3 {
		t.Fatalf("kind = %s code = %s, %d messages", got.Kind, got.Code, len(got.Messages))
	}
	if s := asText(t, got.Messages[0]); s != "[日本] ですね🤔" {
		t.Errorf("first = %q", s)
	}
	if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "わかります", EmojiSmile) {
		t.Errorf("second = %q", s)
	}
	want := line.TemplateMessage{
		AltText: "日本",
		Template: line.ButtonsTemplate{
			ThumbnailImageURL:    "https://bot.example.com/flags/jp.png",
			ImageAspectRatio:     "rectangle",
			ImageSize:            "cover",
			ImageBackgroundColor: "#FFFFFF",
			Title:                "日本",
			Text:                 "東京 / Tokyo",
			Actions: []line.Action{
				line.URIAction{Label: "Wikipedia", URI: b.countries.Lookup("日本").WikipediaURL()},
				line.URIAction{Label: "Google Maps", URI: b.countries.Lookup("日本").MapURL()},
			},
		},
	}
	if !reflect.DeepEqual(got.Messages[2], want) {
		t.Errorf("third =\n %#v\nwant\n %#v", got.Messages[2], want)
	}
}

func TestCountryHitImageSize(t *testing.T) {
	b := newTestBot(t, 1)
	fr := b.countries.Lookup("フランス")
	for name, want := range map[string]struct{ size, background string }{
		"日本":   {"cover", "#FFFFFF"},   // 3:2, white field
		"ブラジル": {"cover", "#009440"},   // 10:7, green field
		"ドイツ":  {"cover", "#000000"},   // 5:3, black top band
		"フランス": {"cover", "#671C3D"},   // 3:2, mean of blue and red
		"イタリア": {"cover", "#675F3F"},   // 3:2, mean of green and red
		"イギリス": {"contain", "#FFFFFF"}, // 2:1
		"スイス":  {"contain", "#FFFFFF"}, // 1:1
		"ネパール": {"contain", "#FFFFFF"}, // portrait
		"アメリカ": {"contain", "#FFFFFF"}, // 19:10
	} {
		got := b.Respond(textEvent(name), opts(Profile{}))
		if got.Kind != KindCountryHit {
			t.Fatalf("%s: kind = %s", name, got.Kind)
		}
		_, bt := asButtons(t, got.Messages[2])
		if bt.ImageSize != want.size || bt.ImageBackgroundColor != want.background {
			t.Errorf("%s: imageSize, imageBackgroundColor = %q, %q, want %q, %q",
				name, bt.ImageSize, bt.ImageBackgroundColor, want.size, want.background)
		}
	}
	if got := fr.FlagCornerColor(); got == "#FFFFFF" || got == "#002654" || got == "#CE1126" {
		t.Errorf("France corner colour = %q, want the mean of blue and red", got)
	}
	// Every country follows FlagFitsFrame: "cover" on its corner colour and
	// "contain" on white. The frame is always set.
	for _, c := range b.countries.All() {
		got := b.Respond(textEvent(c.NameJA), opts(Profile{}))
		if got.Kind != KindCountryHit || got.Code != c.Code {
			t.Fatalf("%s: kind = %s code = %s", c.Code, got.Kind, got.Code)
		}
		_, bt := asButtons(t, got.Messages[2])
		if bt.ImageSize != wantImageSize(c) || bt.ImageAspectRatio != "rectangle" || bt.ImageBackgroundColor != wantBackground(c) {
			t.Errorf("%s: image options = %q %q %q", c.Code, bt.ImageAspectRatio, bt.ImageSize, bt.ImageBackgroundColor)
		}
	}
}

func TestCountryHitEnglishUsesInputAsTitle(t *testing.T) {
	got := newTestBot(t, 1).Respond(textEvent("JAPAN"), opts(Profile{}))
	if got.Kind != KindCountryHit || got.Code != "JP" {
		t.Fatalf("kind = %s code = %s", got.Kind, got.Code)
	}
	if s := asText(t, got.Messages[0]); s != "[JAPAN] ですね🤔" {
		t.Errorf("first = %q", s)
	}
	tm, bt := asButtons(t, got.Messages[2])
	if bt.Title != "JAPAN" || tm.AltText != "JAPAN" {
		t.Errorf("title = %q alt = %q", bt.Title, tm.AltText)
	}
}

func TestCountryMiss(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		got := newTestBot(t, seed).Respond(textEvent("アトランティス"), opts(Profile{}))
		if got.Kind != KindCountryMiss || len(got.Messages) != 2 {
			t.Fatalf("kind = %s, %d messages", got.Kind, len(got.Messages))
		}
		if s := asText(t, got.Messages[0]); s != "[アトランティス] ですね🤔" {
			t.Errorf("first = %q", s)
		}
		if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、わかりませんでした", EmojiSad) {
			t.Errorf("second = %q", s)
		}
	}
}

func TestCancel(t *testing.T) {
	got := newTestBot(t, 1).Respond(textEvent("いいえ"), opts(testProfile))
	if got.Kind != KindCancel || got.Messages != nil {
		t.Errorf("kind = %s messages = %#v", got.Kind, got.Messages)
	}
}

func answerData(name, result string) string {
	return url.Values{"answer": {name}, "result": {result}}.Encode()
}

func TestAnswerCorrect(t *testing.T) {
	b := newTestBot(t, 1)
	got := b.Respond(postbackEvent(answerData("日本", "1")), opts(Profile{}))
	if got.Kind != KindAnswerCorrect || got.Code != "JP" || len(got.Messages) != 4 {
		t.Fatalf("kind = %s code = %s, %d messages", got.Kind, got.Code, len(got.Messages))
	}
	if s := asText(t, got.Messages[0]); s != "[日本] ですね?" {
		t.Errorf("first = %q", s)
	}
	if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、\n正解です", EmojiCelebrate) {
		t.Errorf("second = %q", s)
	}
	jp := b.countries.Lookup("日本")
	wantDetail := line.TemplateMessage{
		AltText: "日本",
		Template: line.ButtonsTemplate{
			Title: "日本",
			Text:  "東京 / Tokyo",
			Actions: []line.Action{
				line.URIAction{Label: "Wikipedia", URI: jp.WikipediaURL()},
				line.URIAction{Label: "Google Maps", URI: jp.MapURL()},
			},
		},
	}
	if !reflect.DeepEqual(got.Messages[2], wantDetail) {
		t.Errorf("third = %#v", got.Messages[2])
	}
	wantConfirm := line.TemplateMessage{
		AltText: "もう一度やりますか？",
		Template: line.ConfirmTemplate{
			Text: "もう一度やりますか？",
			Actions: []line.Action{
				line.MessageAction{Label: "いいえ", Text: "いいえ"},
				line.MessageAction{Label: "はい", Text: "クイズ"},
			},
		},
	}
	if !reflect.DeepEqual(got.Messages[3], wantConfirm) {
		t.Errorf("fourth = %#v", got.Messages[3])
	}
}

func TestAnswerWrong(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		got := newTestBot(t, seed).Respond(postbackEvent(answerData("フランス", "0")), opts(Profile{}))
		if got.Kind != KindAnswerWrong || len(got.Messages) != 2 {
			t.Fatalf("kind = %s, %d messages", got.Kind, len(got.Messages))
		}
		if s := asText(t, got.Messages[0]); s != "[フランス] ですね?" {
			t.Errorf("first = %q", s)
		}
		if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、\n残念！はずれです", EmojiRegret) {
			t.Errorf("second = %q", s)
		}
	}
}

func TestAnswerUnknownCountryStillConfirms(t *testing.T) {
	got := newTestBot(t, 1).Respond(postbackEvent(answerData("ムー大陸", "1")), opts(Profile{}))
	if got.Kind != KindAnswerCorrect || len(got.Messages) != 3 {
		t.Fatalf("kind = %s, %d messages", got.Kind, len(got.Messages))
	}
	tm, ok := got.Messages[2].(line.TemplateMessage)
	if !ok {
		t.Fatalf("third = %#v", got.Messages[2])
	}
	if _, ok := tm.Template.(line.ConfirmTemplate); !ok {
		t.Errorf("third template = %#v, want confirm", tm.Template)
	}
}

func TestAnswerInvalidData(t *testing.T) {
	b := newTestBot(t, 1)
	for _, data := range []string{
		"",
		"%zz",
		"result=1",
		"answer=&result=1",
		"answer=%E6%97%A5%E6%9C%AC",
		"answer=%E6%97%A5%E6%9C%AC&result=2",
		"something=else",
	} {
		got := b.Respond(postbackEvent(data), opts(Profile{}))
		if got.Messages != nil || got.Kind != KindInvalidPostback {
			t.Errorf("%q: kind = %s messages = %#v", data, got.Kind, got.Messages)
		}
	}
}

func TestReplyCountNeverExceedsLimit(t *testing.T) {
	events := []line.Event{
		textEvent("日本"), textEvent("アトランティス"), textEvent("クイズ"), textEvent("who"),
		{Type: line.EventTypeMessage, ReplyToken: "rt", Message: &line.EventMessage{Type: "image"}},
	}
	b := newTestBot(t, 1)
	for _, c := range b.countries.All() {
		events = append(events, postbackEvent(answerData(c.NameJA, "1")), postbackEvent(answerData(c.NameJA, "0")))
	}
	for _, ev := range events {
		got := b.Respond(ev, opts(testProfile))
		if len(got.Messages) > line.MaxReplyMessages {
			t.Errorf("%+v: %d messages", ev, len(got.Messages))
		}
	}
}

func TestDeterministicWithSeed(t *testing.T) {
	a := newTestBot(t, 42).Respond(textEvent("クイズ"), opts(Profile{}))
	b := newTestBot(t, 42).Respond(textEvent("クイズ"), opts(Profile{}))
	if !reflect.DeepEqual(a, b) {
		t.Error("same seed produced different quizzes")
	}
}

func TestEmojiGroups(t *testing.T) {
	groups := map[string]struct {
		got  []string
		want []rune
	}{
		"celebrate": {EmojiCelebrate, []rune{0x1F60A, 0x1F60D, 0x1F31F, 0x2728, 0x1F389, 0x1F38A}},
		"regret":    {EmojiRegret, []rune{0x1F61B, 0x1F61C, 0x1F61D, 0x274C}},
		"smile":     {EmojiSmile, []rune{0x1F600, 0x1F60E, 0x1F917, 0x1F60F, 0x1F609}},
		"sad":       {EmojiSad, []rune{0x1F622, 0x1F62D, 0x1F631, 0x1F635, 0x1F616}},
	}
	for name, g := range groups {
		var want []string
		for _, r := range g.want {
			want = append(want, string(r))
		}
		if !reflect.DeepEqual(g.got, want) {
			t.Errorf("%s = %q, want %q", name, g.got, want)
		}
	}
	if EmojiThinking != "\U0001F914" {
		t.Errorf("thinking = %q", EmojiThinking)
	}
}

func TestEchoTruncationKeepsBrackets(t *testing.T) {
	b := newTestBot(t, 1)
	in := strings.Repeat("あ", line.MaxTextMessageLength)
	got := b.Respond(textEvent(in), opts(Profile{}))
	s := asText(t, got.Messages[0])
	// "[" + "]" + " ですね🤔" take 8 UTF-16 units.
	want := "[" + strings.Repeat("あ", line.MaxTextMessageLength-8) + "] ですね🤔"
	if s != want {
		t.Errorf("echo = %d units, prefix %q, suffix %q", line.UTF16Len(s), s[:10], s[len(s)-20:])
	}
	if line.UTF16Len(s) != line.MaxTextMessageLength {
		t.Errorf("echo length = %d", line.UTF16Len(s))
	}

	got = b.Respond(postbackEvent(answerData(in, "0")), opts(Profile{}))
	s = asText(t, got.Messages[0])
	// "[" + "]" + " ですね?" take 7 UTF-16 units.
	if want := "[" + strings.Repeat("あ", line.MaxTextMessageLength-7) + "] ですね?"; s != want {
		t.Errorf("answer echo = %d units", line.UTF16Len(s))
	}
}

func TestTruncation(t *testing.T) {
	long40 := strings.Repeat("あ", 45)
	data := `{"countries":[{"code":"XA","name_ja":"長い国","aliases_ja":["` + long40 + `"],` +
		`"capital_ja":"` + strings.Repeat("い", 40) + `","capital_en":"` + strings.Repeat("u", 40) + `",` +
		`"lat":1,"lon":2,"wikipedia_ja_url":"https://ja.wikipedia.org/wiki/x"}]}`
	store, err := country.Load([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	b := New(store, rand.New(rand.NewPCG(1, 1)))

	got := b.Respond(textEvent(long40), opts(Profile{}))
	if got.Kind != KindCountryHit {
		t.Fatalf("kind = %s", got.Kind)
	}
	tm, bt := asButtons(t, got.Messages[2])
	if bt.Title != strings.Repeat("あ", 40) {
		t.Errorf("title = %q (%d)", bt.Title, line.UTF16Len(bt.Title))
	}
	if line.UTF16Len(bt.Text) != 60 || !strings.HasPrefix(bt.Text, strings.Repeat("い", 40)+" / ") {
		t.Errorf("text = %q (%d)", bt.Text, line.UTF16Len(bt.Text))
	}
	if tm.AltText != long40 {
		t.Errorf("altText = %q", tm.AltText)
	}

	longProfile := testProfile
	longProfile.Title = strings.Repeat("t", 50)
	longProfile.Text = strings.Repeat("x", 70)
	longProfile.LinkText = strings.Repeat("l", 30)
	got = b.Respond(textEvent("who"), opts(longProfile))
	_, bt = asButtons(t, got.Messages[0])
	if len(bt.Title) != 40 || len(bt.Text) != 60 || len(bt.Actions[0].(line.URIAction).Label) != 20 {
		t.Errorf("profile lengths = %d %d %d", len(bt.Title), len(bt.Text), len(bt.Actions[0].(line.URIAction).Label))
	}
}
