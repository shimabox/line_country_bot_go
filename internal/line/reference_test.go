package line

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The reference encoder produces the JSON the message types encoded to when
// each MarshalJSON passed an alias-embedding struct to encoding/json: a "type"
// field first, then the fields in declaration order with the same tags.
//
// It converts a value into the ref* mirror types below and marshals them with
// encoding/json. The mirror types have no methods, so encoding/json never
// calls MarshalJSON or appendJSON of the message types; TestReferenceHasNo
// BodyTypes checks that no message type survives the conversion.

type refTextMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type refStickerMessage struct {
	Type      string `json:"type"`
	PackageID string `json:"packageId"`
	StickerID string `json:"stickerId"`
}

type refTemplateMessage struct {
	Type     string `json:"type"`
	AltText  string `json:"altText"`
	Template any    `json:"template"`
}

type refButtonsTemplate struct {
	Type                 string `json:"type"`
	ThumbnailImageURL    string `json:"thumbnailImageUrl,omitempty"`
	ImageAspectRatio     string `json:"imageAspectRatio,omitempty"`
	ImageSize            string `json:"imageSize,omitempty"`
	ImageBackgroundColor string `json:"imageBackgroundColor,omitempty"`
	Title                string `json:"title,omitempty"`
	Text                 string `json:"text"`
	Actions              []any  `json:"actions"`
}

type refConfirmTemplate struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Actions []any  `json:"actions"`
}

type refPostbackAction struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Data        string `json:"data"`
	DisplayText string `json:"displayText,omitempty"`
}

type refMessageAction struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

type refURIAction struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	URI   string `json:"uri"`
}

type refReplyBody struct {
	ReplyToken string `json:"replyToken"`
	Messages   []any  `json:"messages"`
}

type refLoadingBody struct {
	ChatID         string `json:"chatId"`
	LoadingSeconds int    `json:"loadingSeconds"`
}

// refMessage converts m into a mirror value. A nil interface stays nil and a
// typed nil pointer becomes a nil pointer of the mirror type, so encoding/json
// treats both as it treated the original. Unknown dynamic types fail the test.
func refMessage(tb testing.TB, m Message) any {
	tb.Helper()
	switch v := m.(type) {
	case nil:
		return nil
	case TextMessage:
		return refTextMessage{Type: "text", Text: v.Text}
	case *TextMessage:
		if v == nil {
			return (*refTextMessage)(nil)
		}
		return refMessage(tb, *v)
	case StickerMessage:
		return refStickerMessage{Type: "sticker", PackageID: v.PackageID, StickerID: v.StickerID}
	case *StickerMessage:
		if v == nil {
			return (*refStickerMessage)(nil)
		}
		return refMessage(tb, *v)
	case TemplateMessage:
		return refTemplateMessage{Type: "template", AltText: v.AltText, Template: refTemplate(tb, v.Template)}
	case *TemplateMessage:
		if v == nil {
			return (*refTemplateMessage)(nil)
		}
		return refMessage(tb, *v)
	}
	tb.Fatalf("reference encoder: unknown Message type %T", m)
	return nil
}

func refTemplate(tb testing.TB, t Template) any {
	tb.Helper()
	switch v := t.(type) {
	case nil:
		return nil
	case ButtonsTemplate:
		return refButtonsTemplate{
			Type:                 "buttons",
			ThumbnailImageURL:    v.ThumbnailImageURL,
			ImageAspectRatio:     v.ImageAspectRatio,
			ImageSize:            v.ImageSize,
			ImageBackgroundColor: v.ImageBackgroundColor,
			Title:                v.Title,
			Text:                 v.Text,
			Actions:              refActions(tb, v.Actions),
		}
	case *ButtonsTemplate:
		if v == nil {
			return (*refButtonsTemplate)(nil)
		}
		return refTemplate(tb, *v)
	case ConfirmTemplate:
		return refConfirmTemplate{Type: "confirm", Text: v.Text, Actions: refActions(tb, v.Actions)}
	case *ConfirmTemplate:
		if v == nil {
			return (*refConfirmTemplate)(nil)
		}
		return refTemplate(tb, *v)
	}
	tb.Fatalf("reference encoder: unknown Template type %T", t)
	return nil
}

func refAction(tb testing.TB, a Action) any {
	tb.Helper()
	switch v := a.(type) {
	case nil:
		return nil
	case PostbackAction:
		return refPostbackAction{Type: "postback", Label: v.Label, Data: v.Data, DisplayText: v.DisplayText}
	case *PostbackAction:
		if v == nil {
			return (*refPostbackAction)(nil)
		}
		return refAction(tb, *v)
	case MessageAction:
		return refMessageAction{Type: "message", Label: v.Label, Text: v.Text}
	case *MessageAction:
		if v == nil {
			return (*refMessageAction)(nil)
		}
		return refAction(tb, *v)
	case URIAction:
		return refURIAction{Type: "uri", Label: v.Label, URI: v.URI}
	case *URIAction:
		if v == nil {
			return (*refURIAction)(nil)
		}
		return refAction(tb, *v)
	}
	tb.Fatalf("reference encoder: unknown Action type %T", a)
	return nil
}

// refActions keeps the difference between a nil slice (null) and an empty one
// ([]).
func refActions(tb testing.TB, actions []Action) []any {
	tb.Helper()
	if actions == nil {
		return nil
	}
	out := make([]any, len(actions))
	for i, a := range actions {
		out[i] = refAction(tb, a)
	}
	return out
}

func refMessages(tb testing.TB, messages []Message) []any {
	tb.Helper()
	if messages == nil {
		return nil
	}
	out := make([]any, len(messages))
	for i, m := range messages {
		out[i] = refMessage(tb, m)
	}
	return out
}

func refReply(tb testing.TB, replyToken string, messages []Message) refReplyBody {
	tb.Helper()
	return refReplyBody{ReplyToken: replyToken, Messages: refMessages(tb, messages)}
}

func refLoading(chatID string, seconds int) refLoadingBody {
	return refLoadingBody{ChatID: chatID, LoadingSeconds: seconds}
}

// refJSON marshals a mirror value after checking that it holds no message
// type.
func refJSON(tb testing.TB, v any) []byte {
	tb.Helper()
	assertNoBodyTypes(tb, reflect.ValueOf(v), "value")
	b, err := json.Marshal(v)
	if err != nil {
		tb.Fatalf("reference encoder: %v", err)
	}
	return b
}

var (
	marshalerType = reflect.TypeFor[json.Marshaler]()
	messageType   = reflect.TypeFor[Message]()
	templateType  = reflect.TypeFor[Template]()
	actionType    = reflect.TypeFor[Action]()
)

// assertNoBodyTypes walks v and fails if it reaches a type that encoding/json
// would hand over to the package's own encoder: a message type, or anything
// with a MarshalJSON method.
func assertNoBodyTypes(tb testing.TB, v reflect.Value, path string) {
	tb.Helper()
	if !v.IsValid() {
		return
	}
	t := v.Type()
	for _, it := range []reflect.Type{marshalerType, messageType, templateType, actionType} {
		if t.Implements(it) || (t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(it)) {
			tb.Fatalf("reference encoder: %s has type %v, which implements %v", path, t, it)
		}
	}
	switch v.Kind() {
	case reflect.String, reflect.Int:
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			assertNoBodyTypes(tb, v.Elem(), path)
		}
	case reflect.Slice:
		for i := range v.Len() {
			assertNoBodyTypes(tb, v.Index(i), path+"["+t.Elem().String()+"]")
		}
	case reflect.Struct:
		if !strings.HasPrefix(t.Name(), "ref") {
			tb.Fatalf("reference encoder: %s has non-mirror struct type %v", path, t)
		}
		for i := range v.NumField() {
			assertNoBodyTypes(tb, v.Field(i), path+"."+t.Field(i).Name)
		}
	default:
		tb.Fatalf("reference encoder: %s has unexpected kind %v", path, v.Kind())
	}
}

// specialText holds every class of character whose escaping matters,
// including U+2028, U+2029 and an invalid byte.
const specialText = "a\"b\\c\nd\te\x01f<g>h&i\xe2\x80\xa8j\xe2\x80\xa9k\x7fl日本😀\xffm"

// messageCases covers every message type, the omitempty fields, and nil
// interfaces, typed nil pointers and non-nil pointers in each position that
// holds a Message, a Template or an Action.
func messageCases() []struct {
	name string
	msg  Message
} {
	type tc = struct {
		name string
		msg  Message
	}
	var cases []tc
	for _, g := range goldenMessages {
		cases = append(cases, tc{"golden/" + g.golden, g.msg})
	}
	cases = append(cases,
		tc{"golden/actions.json in buttons", TemplateMessage{AltText: "a", Template: ButtonsTemplate{Text: "t", Actions: goldenActions}}},
		tc{"text/empty", TextMessage{}},
		tc{"text/special", TextMessage{Text: specialText}},
		tc{"sticker/empty", StickerMessage{}},
		tc{"sticker/special", StickerMessage{PackageID: specialText, StickerID: specialText}},
		tc{"template/empty", TemplateMessage{}},
		tc{"template/special", TemplateMessage{AltText: specialText, Template: ConfirmTemplate{Text: specialText, Actions: []Action{
			PostbackAction{Label: specialText, Data: specialText, DisplayText: specialText},
			MessageAction{Label: specialText, Text: specialText},
			URIAction{Label: specialText, URI: specialText},
		}}}},
	)

	// Every combination of the five omitempty fields of ButtonsTemplate.
	for mask := range 1 << 5 {
		bt := ButtonsTemplate{Text: "text", Actions: []Action{URIAction{Label: "l", URI: "https://example.com/?a=1&b=<2>"}}}
		if mask&1 != 0 {
			bt.ThumbnailImageURL = "https://example.com/flag.png"
		}
		if mask&2 != 0 {
			bt.ImageAspectRatio = ImageAspectRatioSquare
		}
		if mask&4 != 0 {
			bt.ImageSize = ImageSizeCover
		}
		if mask&8 != 0 {
			bt.ImageBackgroundColor = "#000000"
		}
		if mask&16 != 0 {
			bt.Title = specialText
		}
		cases = append(cases, tc{fmt.Sprintf("buttons/omitempty %05b", mask), TemplateMessage{AltText: "alt", Template: bt}})
	}
	cases = append(cases,
		tc{"postback/no displayText", TemplateMessage{Template: ConfirmTemplate{Actions: []Action{PostbackAction{Label: "l", Data: "d"}}}}},
		tc{"postback/displayText", TemplateMessage{Template: ConfirmTemplate{Actions: []Action{PostbackAction{Label: "l", Data: "d", DisplayText: "x"}}}}},
	)

	// The Template position: nil interface, typed nil, non-nil pointer.
	buttons := ButtonsTemplate{Title: "title", Text: "text", Actions: []Action{MessageAction{Label: "l", Text: "t"}}}
	confirm := ConfirmTemplate{Text: "text", Actions: []Action{MessageAction{Label: "no", Text: "no"}, MessageAction{Label: "yes", Text: "yes"}}}
	cases = append(cases,
		tc{"template/nil interface", TemplateMessage{AltText: "a", Template: nil}},
		tc{"template/typed nil buttons", TemplateMessage{AltText: "a", Template: (*ButtonsTemplate)(nil)}},
		tc{"template/typed nil confirm", TemplateMessage{AltText: "a", Template: (*ConfirmTemplate)(nil)}},
		tc{"template/pointer buttons", TemplateMessage{AltText: "a", Template: &buttons}},
		tc{"template/pointer confirm", TemplateMessage{AltText: "a", Template: &confirm}},
	)

	// The actions slice: nil, empty, and each kind of element, in both
	// templates.
	postback := PostbackAction{Label: "l", Data: "d", DisplayText: "x"}
	message := MessageAction{Label: "l", Text: "t"}
	uri := URIAction{Label: "l", URI: "https://example.com/"}
	actionLists := []struct {
		name    string
		actions []Action
	}{
		{"nil", nil},
		{"empty", []Action{}},
		{"nil interface", []Action{nil}},
		{"typed nil postback", []Action{(*PostbackAction)(nil)}},
		{"typed nil message", []Action{(*MessageAction)(nil)}},
		{"typed nil uri", []Action{(*URIAction)(nil)}},
		{"pointer postback", []Action{&postback}},
		{"pointer message", []Action{&message}},
		{"pointer uri", []Action{&uri}},
		{"mixed", []Action{postback, nil, &message, (*URIAction)(nil), uri, (*PostbackAction)(nil), &uri}},
	}
	for _, al := range actionLists {
		cases = append(cases,
			tc{"buttons actions/" + al.name, TemplateMessage{AltText: "a", Template: ButtonsTemplate{Text: "t", Actions: al.actions}}},
			tc{"confirm actions/" + al.name, TemplateMessage{AltText: "a", Template: ConfirmTemplate{Text: "t", Actions: al.actions}}},
			tc{"pointer confirm actions/" + al.name, TemplateMessage{AltText: "a", Template: &ConfirmTemplate{Text: "t", Actions: al.actions}}},
		)
	}

	// Message values that are pointers or typed nil themselves.
	text := TextMessage{Text: "text"}
	sticker := StickerMessage{PackageID: "446", StickerID: "1999"}
	tmpl := TemplateMessage{AltText: "a", Template: &buttons}
	cases = append(cases,
		tc{"message/nil interface", nil},
		tc{"message/typed nil text", (*TextMessage)(nil)},
		tc{"message/typed nil sticker", (*StickerMessage)(nil)},
		tc{"message/typed nil template", (*TemplateMessage)(nil)},
		tc{"message/pointer text", &text},
		tc{"message/pointer sticker", &sticker},
		tc{"message/pointer template", &tmpl},
	)
	return cases
}

// messageListCases holds lists of one to five messages, with nil interfaces,
// typed nil pointers and non-nil pointers as elements.
func messageListCases() []struct {
	name string
	msgs []Message
} {
	type tc = struct {
		name string
		msgs []Message
	}
	all := messageCases()
	cases := []tc{
		{"nil", nil},
		{"empty", []Message{}},
		{"element nil interface", []Message{TextMessage{Text: "a"}, nil}},
		{"element typed nil", []Message{(*TextMessage)(nil), (*StickerMessage)(nil), (*TemplateMessage)(nil)}},
		{"element pointers", []Message{&TextMessage{Text: "a"}, &StickerMessage{PackageID: "1", StickerID: "2"}, &TemplateMessage{AltText: "a", Template: &ConfirmTemplate{Text: "t"}}}},
	}
	for n := 1; n <= MaxReplyMessages; n++ {
		msgs := make([]Message, n)
		for i := range msgs {
			msgs[i] = all[(i*7+n)%len(all)].msg
		}
		cases = append(cases, tc{fmt.Sprintf("length %d", n), msgs})
	}
	return cases
}

func TestReferenceGolden(t *testing.T) {
	for _, tt := range goldenMessages {
		t.Run(tt.golden, func(t *testing.T) {
			assertGolden(t, tt.golden, refMessage(t, tt.msg))
		})
	}
	t.Run("actions.json", func(t *testing.T) {
		assertGolden(t, "actions.json", refActions(t, goldenActions))
	})
}

func TestReferenceHasNoBodyTypes(t *testing.T) {
	for _, v := range []any{
		refTextMessage{}, refStickerMessage{}, refTemplateMessage{}, refButtonsTemplate{},
		refConfirmTemplate{}, refPostbackAction{}, refMessageAction{}, refURIAction{},
		refReplyBody{}, refLoadingBody{},
	} {
		for _, it := range []reflect.Type{marshalerType, messageType, templateType, actionType} {
			if reflect.TypeOf(v).Implements(it) || reflect.PointerTo(reflect.TypeOf(v)).Implements(it) {
				t.Errorf("%T implements %v", v, it)
			}
		}
	}
	for _, tt := range messageCases() {
		assertNoBodyTypes(t, reflect.ValueOf(refMessage(t, tt.msg)), tt.name)
	}
	for _, tt := range messageListCases() {
		assertNoBodyTypes(t, reflect.ValueOf(refReply(t, "rt", tt.msgs)), tt.name)
	}

	// The check itself must catch a message type left in the output.
	ft := &fatalRecorder{TB: t}
	func() {
		defer func() { _ = recover() }()
		assertNoBodyTypes(ft, reflect.ValueOf(refTemplateMessage{Template: ButtonsTemplate{}}), "leak")
	}()
	if !ft.failed {
		t.Error("assertNoBodyTypes accepted a ButtonsTemplate")
	}
}

// fatalRecorder records a Fatalf call and stops the caller with a panic
// instead of failing the test.
type fatalRecorder struct {
	testing.TB
	failed bool
}

func (f *fatalRecorder) Fatalf(string, ...any) {
	f.failed = true
	panic("fatal")
}

// TestJSONMarshalMatchesReference checks that json.Marshal of a message gives
// the reference output. On the alias-embedding implementation this anchors
// the reference encoder to it; on the hand-written encoder it checks that
// MarshalJSON still gives the same output.
func TestJSONMarshalMatchesReference(t *testing.T) {
	for _, tt := range messageCases() {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if want := refJSON(t, refMessage(t, tt.msg)); string(got) != string(want) {
				t.Errorf("mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
	for _, tt := range messageListCases() {
		t.Run("list/"+tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.msgs)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if want := refJSON(t, refMessages(t, tt.msgs)); string(got) != string(want) {
				t.Errorf("mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
