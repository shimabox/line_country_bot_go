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

// TestQuiz checks the flag quiz built by quizFlag; which kind of quiz
// Respond asks is tested in TestQuizPicksBothKinds.
func TestQuiz(t *testing.T) {
	imageSizes := map[string]bool{}
	backgrounds := map[string]bool{}
	for seed := uint64(0); seed < 30; seed++ {
		b := newTestBot(t, seed)
		for i := 0; i < 3; i++ {
			got := b.quizFlag(baseURL)
			if got.Kind != KindQuiz || len(got.Messages) != 1 {
				t.Fatalf("seed %d #%d: kind = %s, %d messages", seed, i, got.Kind, len(got.Messages))
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
			correctCode := ""
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
				if want := (url.Values{"answer": {v.Get("answer")}, "code": {v.Get("code")}, "result": {v.Get("result")}}).Encode(); pa.Data != want {
					t.Errorf("data = %q, want form %q", pa.Data, want)
				}
				if v.Get("answer") != pa.Label {
					t.Errorf("answer %q != label %q", v.Get("answer"), pa.Label)
				}
				// code is the country of the button.
				if c := b.countries.ByCode(v.Get("code")); c == nil || c.NameJA != v.Get("answer") {
					t.Errorf("code %q is not the country of answer %q", v.Get("code"), v.Get("answer"))
				}
				if v.Get("result") == "1" {
					correctCode = v.Get("code")
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
			if correctCode != c.Code {
				t.Errorf("code of the correct button = %s, want %s", correctCode, c.Code)
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
		got := newTestBot(t, seed).quizFlag(baseURL)
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

// capitalQuizButtons checks the shape of a capital quiz reply and returns the
// country of the correct button and its position.
func capitalQuizButtons(t *testing.T, b *Bot, got Reply) (*country.Country, int) {
	t.Helper()
	if got.Kind != KindCapitalQuiz || len(got.Messages) != 1 {
		t.Fatalf("kind = %s, %d messages", got.Kind, len(got.Messages))
	}
	tm, bt := asButtons(t, got.Messages[0])
	if tm.AltText != "この国の首都はどこでしょうか？" || bt.Title != "クイズです" || bt.Text != "この国の首都はどこでしょうか？" {
		t.Errorf("texts = %q %q %q", tm.AltText, bt.Title, bt.Text)
	}
	if bt.ImageAspectRatio != "rectangle" {
		t.Errorf("imageAspectRatio = %q", bt.ImageAspectRatio)
	}
	if len(bt.Actions) != 4 {
		t.Fatalf("%d actions", len(bt.Actions))
	}
	labels := map[string]bool{}
	var correct *country.Country
	position := -1
	for i, a := range bt.Actions {
		pa, ok := a.(line.PostbackAction)
		if !ok {
			t.Fatalf("action %#v is not postback", a)
		}
		if line.UTF16Len(pa.Label) > line.MaxActionLabel || line.UTF16Len(pa.Data) > line.MaxPostbackData ||
			line.UTF16Len(pa.DisplayText) > line.MaxPostbackDisplayText {
			t.Errorf("action too long: %#v", pa)
		}
		labels[pa.Label] = true
		v, err := url.ParseQuery(pa.Data)
		if err != nil {
			t.Fatalf("data %q: %v", pa.Data, err)
		}
		want := url.Values{"answer": {v.Get("answer")}, "code": {v.Get("code")}, "q": {"capital"}, "result": {v.Get("result")}}.Encode()
		if pa.Data != want {
			t.Errorf("data = %q, want form %q", pa.Data, want)
		}
		c := b.countries.ByCode(v.Get("code"))
		if c == nil {
			t.Fatalf("data %q: unknown code", pa.Data)
		}
		if pa.Label != c.CapitalJA || pa.DisplayText != c.CapitalJA || v.Get("answer") != c.CapitalJA {
			t.Errorf("%s: label %q, displayText %q, answer %q, want capital_ja %q", c.Code, pa.Label, pa.DisplayText, v.Get("answer"), c.CapitalJA)
		}
		switch v.Get("result") {
		case "1":
			if correct != nil {
				t.Errorf("two correct answers: %s and %s", correct.Code, c.Code)
			}
			correct, position = c, i
		case "0":
		default:
			t.Errorf("result = %q", v.Get("result"))
		}
	}
	if len(labels) != 4 {
		t.Errorf("duplicate choices: %v", labels)
	}
	if correct == nil {
		t.Fatal("no correct answer")
	}
	if bt.ThumbnailImageURL != correct.FlagURL(baseURL) {
		t.Errorf("image = %s, want flag of %s", bt.ThumbnailImageURL, correct.Code)
	}
	if bt.ImageSize != wantImageSize(correct) || bt.ImageBackgroundColor != wantBackground(correct) {
		t.Errorf("%s: imageSize, imageBackgroundColor = %q, %q", correct.Code, bt.ImageSize, bt.ImageBackgroundColor)
	}
	if got.Code != correct.Code {
		t.Errorf("Code = %s, want %s", got.Code, correct.Code)
	}
	return correct, position
}

func TestCapitalQuiz(t *testing.T) {
	positions := map[int]bool{}
	answers := map[string]bool{}
	for seed := uint64(0); seed < 40; seed++ {
		b := newTestBot(t, seed)
		for i := 0; i < 3; i++ {
			c, pos := capitalQuizButtons(t, b, b.quizCapital(baseURL))
			positions[pos] = true
			answers[c.Code] = true
		}
	}
	if len(positions) != 4 {
		t.Errorf("correct answer appeared only at positions %v", positions)
	}
	if len(answers) < 20 {
		t.Errorf("only %d different answers", len(answers))
	}
}

// TestCapitalQuizFitsEveryCountry builds capital quizzes over all the
// countries, so every capital is checked against the label and data limits.
func TestCapitalQuizFitsEveryCountry(t *testing.T) {
	b := newTestBot(t, 1)
	all := b.countries.All()
	for i := 0; i < len(all); i += 4 {
		choices := make([]*country.Country, 0, 4)
		for j := 0; j < 4; j++ {
			choices = append(choices, all[(i+j)%len(all)])
		}
		if !capitalChoicesOK(choices) {
			t.Fatalf("choices %s %s %s %s cannot make a capital quiz", choices[0].Code, choices[1].Code, choices[2].Code, choices[3].Code)
		}
		capitalQuizButtons(t, b, capitalQuizReply(choices, choices[i%4], baseURL))
	}
}

// fallbackStore is a store of four countries, so every quiz uses all of them.
func fallbackStore(t *testing.T, capitals [4]string) *country.Store {
	t.Helper()
	var sb strings.Builder
	sb.WriteString(`{"countries":[`)
	for i, capital := range capitals {
		if i > 0 {
			sb.WriteString(",")
		}
		code := string(rune('A'+i)) + "A"
		sb.WriteString(`{"code":"` + code + `","name_ja":"国` + code + `","capital_ja":"` + capital + `","capital_en":"City ` + code + `",` +
			`"lat":1,"lon":2,"wikipedia_ja_url":"https://ja.wikipedia.org/wiki/x"}`)
	}
	sb.WriteString(`]}`)
	s, err := country.Load([]byte(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCapitalQuizFallsBackToFlagQuiz(t *testing.T) {
	long := strings.Repeat("あ", line.MaxActionLabel)
	for name, tt := range map[string]struct {
		capitals [4]string
		want     Kind
	}{
		"distinct capitals":     {[4]string{"一の市", "二の市", "三の市", "四の市"}, KindCapitalQuiz},
		"missing capital_ja":    {[4]string{"一の市", "", "三の市", "四の市"}, KindQuiz},
		"same capital_ja":       {[4]string{"一の市", "同じ市", "同じ市", "四の市"}, KindQuiz},
		"same truncated labels": {[4]string{"一の市", long + "い", long + "う", "四の市"}, KindQuiz},
	} {
		for seed := uint64(0); seed < 10; seed++ {
			b := New(fallbackStore(t, tt.capitals), rand.New(rand.NewPCG(seed, seed)))
			got := b.quizCapital(baseURL)
			if got.Kind != tt.want || len(got.Messages) != 1 {
				t.Fatalf("%s: kind = %s, %d messages, want %s", name, got.Kind, len(got.Messages), tt.want)
			}
			tm, bt := asButtons(t, got.Messages[0])
			wantText := "この国はどこでしょうか？"
			if tt.want == KindCapitalQuiz {
				wantText = "この国の首都はどこでしょうか？"
			}
			if tm.AltText != wantText || bt.Text != wantText || len(bt.Actions) != 4 {
				t.Errorf("%s: alt %q text %q, %d actions", name, tm.AltText, bt.Text, len(bt.Actions))
			}
			for _, a := range bt.Actions {
				pa := a.(line.PostbackAction)
				v, _ := url.ParseQuery(pa.Data)
				c := b.countries.ByCode(v.Get("code"))
				if c == nil {
					t.Fatalf("%s: data %q has no known code", name, pa.Data)
				}
				wantLabel, wantQ := c.NameJA, ""
				if tt.want == KindCapitalQuiz {
					wantLabel, wantQ = c.CapitalJA, "capital"
				}
				if pa.Label != wantLabel || v.Get("q") != wantQ {
					t.Errorf("%s: label %q q %q, want %q %q", name, pa.Label, v.Get("q"), wantLabel, wantQ)
				}
			}
			// Respond falls back the same way, so it only asks flag quizzes.
			if tt.want == KindQuiz {
				if got := b.Respond(textEvent("クイズ"), opts(Profile{})); got.Kind != KindQuiz {
					t.Errorf("%s: Respond kind = %s, want %s", name, got.Kind, KindQuiz)
				}
			}
		}
	}
}

func TestQuizPicksBothKinds(t *testing.T) {
	for _, in := range []string{"クイズ", "quiz", "QUIZ"} {
		kinds := map[Kind]int{}
		for seed := uint64(0); seed < 30; seed++ {
			b := newTestBot(t, seed)
			got := b.Respond(textEvent(in), opts(Profile{}))
			kinds[got.Kind]++
			if len(got.Messages) != 1 {
				t.Fatalf("%q seed %d: %d messages", in, seed, len(got.Messages))
			}
			tm, _ := asButtons(t, got.Messages[0])
			switch got.Kind {
			case KindQuiz:
				if tm.AltText != "この国はどこでしょうか？" {
					t.Errorf("%q seed %d: flag quiz alt = %q", in, seed, tm.AltText)
				}
			case KindCapitalQuiz:
				capitalQuizButtons(t, b, got)
			default:
				t.Errorf("%q seed %d: kind = %s", in, seed, got.Kind)
			}
		}
		if kinds[KindQuiz] == 0 || kinds[KindCapitalQuiz] == 0 {
			t.Errorf("%q: kinds over 30 seeds = %v, want both quizzes", in, kinds)
		}
		t.Logf("%q: %v", in, kinds)
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

func TestCapitalHitUsesCountryNameAsTitle(t *testing.T) {
	b := newTestBot(t, 1)
	jp := b.countries.ByCode("JP")
	for _, in := range []string{"東京", "Tokyo", " tokyo ", "TOKYO"} {
		got := b.Respond(textEvent(in), opts(Profile{}))
		if got.Kind != KindCapitalHit || got.Code != "JP" || len(got.Messages) != 3 {
			t.Fatalf("%q: kind = %s code = %s, %d messages", in, got.Kind, got.Code, len(got.Messages))
		}
		echo := Normalize(in).Text
		if s := asText(t, got.Messages[0]); s != "["+echo+"] ですね🤔" {
			t.Errorf("%q: first = %q", in, s)
		}
		if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "わかります", EmojiSmile) {
			t.Errorf("%q: second = %q", in, s)
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
					line.URIAction{Label: "Wikipedia", URI: jp.WikipediaURL()},
					line.URIAction{Label: "Google Maps", URI: jp.MapURL()},
				},
			},
		}
		if !reflect.DeepEqual(got.Messages[2], want) {
			t.Errorf("%q: third =\n %#v\nwant\n %#v", in, got.Messages[2], want)
		}
	}
}

// TestCapitalHitEveryCountry: every capital finds its country with the
// country's name as the title, unless the capital is also the country's name
// or alias, which is a country hit titled with the input.
func TestCapitalHitEveryCountry(t *testing.T) {
	b := newTestBot(t, 1)
	capitalHits := 0
	for _, c := range b.countries.All() {
		for _, capital := range []string{c.CapitalJA, c.CapitalEN} {
			got := b.Respond(textEvent(capital), opts(Profile{}))
			if got.Code != c.Code || len(got.Messages) != 3 {
				t.Fatalf("%s %q: kind = %s code = %s", c.Code, capital, got.Kind, got.Code)
			}
			_, bt := asButtons(t, got.Messages[2])
			in := Normalize(capital)
			wantKind, wantTitle := KindCapitalHit, line.TruncateUTF16(c.NameJA, line.MaxButtonsTitle)
			if c.NamedBy(in.Lower) {
				wantKind, wantTitle = KindCountryHit, line.TruncateUTF16(in.Text, line.MaxButtonsTitle)
			} else {
				capitalHits++
			}
			if got.Kind != wantKind || bt.Title != wantTitle || bt.ThumbnailImageURL != c.FlagURL(baseURL) {
				t.Errorf("%s %q: kind = %s title = %q image = %q, want %s %q", c.Code, capital, got.Kind, bt.Title, bt.ThumbnailImageURL, wantKind, wantTitle)
			}
		}
	}
	if capitalHits == 0 {
		t.Error("no capital hit")
	}
}

func TestCapitalSameAsCountryNameIsCountryHit(t *testing.T) {
	for _, in := range []string{"シンガポール", "Singapore"} {
		got := newTestBot(t, 1).Respond(textEvent(in), opts(Profile{}))
		if got.Kind != KindCountryHit || got.Code != "SG" {
			t.Fatalf("%q: kind = %s code = %s", in, got.Kind, got.Code)
		}
		tm, bt := asButtons(t, got.Messages[2])
		if bt.Title != in || tm.AltText != in {
			t.Errorf("%q: title = %q alt = %q, want the input", in, bt.Title, tm.AltText)
		}
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

// codeData is the postback data of a quiz button with a country code; q is
// "" for the flag quiz.
func codeData(answer, code, q, result string) string {
	v := url.Values{"answer": {answer}, "code": {code}, "result": {result}}
	if q != "" {
		v.Set("q", q)
	}
	return v.Encode()
}

func TestAnswerWithCodeFindsCountryByCode(t *testing.T) {
	b := newTestBot(t, 1)
	old := b.Respond(postbackEvent(answerData("日本", "1")), opts(Profile{}))
	// The code decides the country; the answer is only echoed.
	for _, data := range []string{codeData("日本", "JP", "", "1"), codeData("ニッポン", "JP", "", "1")} {
		got := b.Respond(postbackEvent(data), opts(Profile{}))
		if got.Kind != KindAnswerCorrect || got.Code != "JP" || len(got.Messages) != 4 {
			t.Fatalf("%q: kind = %s code = %s, %d messages", data, got.Kind, got.Code, len(got.Messages))
		}
		answer, _ := url.ParseQuery(data)
		if s := asText(t, got.Messages[0]); s != "["+answer.Get("answer")+"] ですね?" {
			t.Errorf("%q: first = %q", data, s)
		}
		// Same details and confirm as the old form without code.
		if !reflect.DeepEqual(got.Messages[2:], old.Messages[2:]) {
			t.Errorf("%q: messages = %#v, want %#v", data, got.Messages[2:], old.Messages[2:])
		}
	}
}

func TestAnswerUnknownCodeStillConfirms(t *testing.T) {
	// A well-formed code that is not in the data skips the details, even when
	// the answer is a known name.
	for _, data := range []string{codeData("日本", "ZZ", "", "1"), codeData("東京", "ZZ", "capital", "1")} {
		got := newTestBot(t, 1).Respond(postbackEvent(data), opts(Profile{}))
		if len(got.Messages) != 3 || got.Code != "" {
			t.Fatalf("%q: kind = %s code = %s, %d messages", data, got.Kind, got.Code, len(got.Messages))
		}
		if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、\n正解です", EmojiCelebrate) {
			t.Errorf("%q: second = %q", data, s)
		}
		tm, ok := got.Messages[2].(line.TemplateMessage)
		if !ok {
			t.Fatalf("%q: third = %#v", data, got.Messages[2])
		}
		if _, ok := tm.Template.(line.ConfirmTemplate); !ok {
			t.Errorf("%q: third template = %#v, want confirm", data, tm.Template)
		}
	}
}

func TestAnswerMalformedCodeFallsBackToName(t *testing.T) {
	b := newTestBot(t, 1)
	want := b.Respond(postbackEvent(answerData("日本", "1")), opts(Profile{}))
	// A code that is not two uppercase letters counts as no code, so the
	// answer is looked up by name as in the old form.
	for _, code := range []string{"", "jp", "Jp", "JPN", "J", "日本", "J1"} {
		got := b.Respond(postbackEvent(codeData("日本", code, "", "1")), opts(Profile{}))
		if got.Kind != KindAnswerCorrect || got.Code != "JP" || len(got.Messages) != 4 {
			t.Fatalf("code %q: kind = %s code = %s, %d messages", code, got.Kind, got.Code, len(got.Messages))
		}
		if !reflect.DeepEqual(got.Messages[2:], want.Messages[2:]) {
			t.Errorf("code %q: messages differ from the old form", code)
		}
	}
}

func TestCapitalAnswerCorrect(t *testing.T) {
	b := newTestBot(t, 1)
	got := b.Respond(postbackEvent(codeData("東京", "JP", "capital", "1")), opts(Profile{}))
	if got.Kind != KindCapitalAnswerCorrect || got.Code != "JP" || len(got.Messages) != 4 {
		t.Fatalf("kind = %s code = %s, %d messages", got.Kind, got.Code, len(got.Messages))
	}
	if s := asText(t, got.Messages[0]); s != "[東京] ですね?" {
		t.Errorf("first = %q", s)
	}
	if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、\n正解です", EmojiCelebrate) {
		t.Errorf("second = %q", s)
	}
	// The details are titled with the country, without the flag, as after a
	// flag quiz; the confirm is the same too.
	flag := b.Respond(postbackEvent(answerData("日本", "1")), opts(Profile{}))
	if !reflect.DeepEqual(got.Messages[2:], flag.Messages[2:]) {
		t.Errorf("details and confirm = %#v, want %#v", got.Messages[2:], flag.Messages[2:])
	}
	_, bt := asButtons(t, got.Messages[2])
	if bt.Title != "日本" || bt.Text != "東京 / Tokyo" || bt.ThumbnailImageURL != "" {
		t.Errorf("details = %q %q %q", bt.Title, bt.Text, bt.ThumbnailImageURL)
	}
}

func TestCapitalAnswerWrong(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		got := newTestBot(t, seed).Respond(postbackEvent(codeData("パリ", "FR", "capital", "0")), opts(Profile{}))
		if got.Kind != KindCapitalAnswerWrong || len(got.Messages) != 2 {
			t.Fatalf("kind = %s, %d messages", got.Kind, len(got.Messages))
		}
		if s := asText(t, got.Messages[0]); s != "[パリ] ですね?" {
			t.Errorf("first = %q", s)
		}
		if s := asText(t, got.Messages[1]); !hasEmojiSuffix(s, "む、むむっ、、\n残念！はずれです", EmojiRegret) {
			t.Errorf("second = %q", s)
		}
	}
}

func TestCapitalAnswerKinds(t *testing.T) {
	b := newTestBot(t, 1)
	for data, want := range map[string]Kind{
		codeData("東京", "ZZ", "capital", "1"): KindCapitalAnswerCorrect, // unknown code
		codeData("東京", "ZZ", "capital", "0"): KindCapitalAnswerWrong,
		codeData("日本", "JP", "flag", "1"):    KindAnswerCorrect, // only "capital" is the capital quiz
		codeData("日本", "JP", "", "0"):        KindAnswerWrong,
		answerData("日本", "1"):                KindAnswerCorrect,
	} {
		if got := b.Respond(postbackEvent(data), opts(Profile{})); got.Kind != want {
			t.Errorf("%q: kind = %s, want %s", data, got.Kind, want)
		}
	}
}

// TestCapitalQuizButtonsAnswer taps every button of capital quizzes.
func TestCapitalQuizButtonsAnswer(t *testing.T) {
	for seed := uint64(0); seed < 10; seed++ {
		b := newTestBot(t, seed)
		quiz := b.quizCapital(baseURL)
		_, bt := asButtons(t, quiz.Messages[0])
		for _, a := range bt.Actions {
			pa := a.(line.PostbackAction)
			got := b.Respond(postbackEvent(pa.Data), opts(Profile{}))
			if s := asText(t, got.Messages[0]); s != "["+pa.DisplayText+"] ですね?" {
				t.Errorf("echo = %q, want %q", s, pa.DisplayText)
			}
			if strings.HasSuffix(pa.Data, "result=1") {
				if got.Kind != KindCapitalAnswerCorrect || got.Code != quiz.Code || len(got.Messages) != 4 {
					t.Errorf("correct %q: kind = %s code = %s, %d messages", pa.Data, got.Kind, got.Code, len(got.Messages))
				}
			} else if got.Kind != KindCapitalAnswerWrong || len(got.Messages) != 2 {
				t.Errorf("wrong %q: kind = %s, %d messages", pa.Data, got.Kind, len(got.Messages))
			}
		}
	}
}

func TestReplyCountNeverExceedsLimit(t *testing.T) {
	events := []line.Event{
		textEvent("日本"), textEvent("アトランティス"), textEvent("クイズ"), textEvent("who"),
		textEvent("東京"), textEvent("Tokyo"), textEvent("シンガポール"),
		{Type: line.EventTypeMessage, ReplyToken: "rt", Message: &line.EventMessage{Type: "image"}},
	}
	b := newTestBot(t, 1)
	for _, c := range b.countries.All() {
		events = append(events, postbackEvent(answerData(c.NameJA, "1")), postbackEvent(answerData(c.NameJA, "0")))
		events = append(events, postbackEvent(codeData(c.NameJA, c.Code, "", "1")), postbackEvent(codeData(c.NameJA, c.Code, "", "0")))
		events = append(events,
			postbackEvent(codeData(c.CapitalJA, c.Code, "capital", "1")), postbackEvent(codeData(c.CapitalJA, c.Code, "capital", "0")))
		events = append(events, textEvent(c.CapitalJA), textEvent(c.CapitalEN))
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
