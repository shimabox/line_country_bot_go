package line

// The message types are encoded by hand (appendJSON) instead of with
// encoding/json, because the Worker starts a new wasm instance for every
// request and encoding/json would build its reflection-based encoders for
// each type on every request. The output is the JSON encoding/json gave: the
// "type" field first, then the fields in declaration order, without spaces.
// The tests check it byte for byte against a reference encoder built on
// encoding/json.

import "unicode/utf8"

// Limits of the Messaging API, counted in UTF-16 code units.
const (
	MaxReplyMessages       = 5
	MaxButtonsTitle        = 40
	MaxButtonsTextImage    = 60  // buttons text when an image or title is set
	MaxButtonsTextPlain    = 160 // buttons text without image and title
	MaxConfirmText         = 240
	MaxActionLabel         = 20
	MaxPostbackData        = 300
	MaxPostbackDisplayText = 300
	MaxAltText             = 400
	MaxTextMessageLength   = 5000
)

// Values for ButtonsTemplate image fields.
const (
	ImageAspectRatioRectangle = "rectangle"
	ImageAspectRatioSquare    = "square"
	ImageSizeCover            = "cover"
	ImageSizeContain          = "contain"
)

// Message is a message object that can be sent with the reply API.
type Message interface {
	isMessage()
	appendJSON(dst []byte) []byte
}

// Template is the template of a TemplateMessage.
type Template interface {
	isTemplate()
	appendJSON(dst []byte) []byte
}

// Action is an action of a template.
type Action interface {
	isAction()
	appendJSON(dst []byte) []byte
}

// TextMessage is a plain text message.
type TextMessage struct {
	Text string `json:"text"`
}

// StickerMessage sends a sticker identified by package and sticker IDs.
type StickerMessage struct {
	PackageID string `json:"packageId"`
	StickerID string `json:"stickerId"`
}

// TemplateMessage sends a template; AltText is shown where templates are not
// supported, such as in notifications.
type TemplateMessage struct {
	AltText  string   `json:"altText"`
	Template Template `json:"template"`
}

// ButtonsTemplate shows an optional image, title, text and up to four actions.
type ButtonsTemplate struct {
	ThumbnailImageURL    string   `json:"thumbnailImageUrl,omitempty"`
	ImageAspectRatio     string   `json:"imageAspectRatio,omitempty"`
	ImageSize            string   `json:"imageSize,omitempty"`
	ImageBackgroundColor string   `json:"imageBackgroundColor,omitempty"`
	Title                string   `json:"title,omitempty"`
	Text                 string   `json:"text"`
	Actions              []Action `json:"actions"`
}

// ConfirmTemplate shows text and exactly two actions.
type ConfirmTemplate struct {
	Text    string   `json:"text"`
	Actions []Action `json:"actions"`
}

// PostbackAction returns Data in a postback event when tapped. A non-empty
// DisplayText is shown in the chat as the user's message; it is not sent in
// the event. The deprecated "text" field is not supported.
type PostbackAction struct {
	Label       string `json:"label"`
	Data        string `json:"data"`
	DisplayText string `json:"displayText,omitempty"`
}

// MessageAction makes the user send Text when tapped.
type MessageAction struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// URIAction opens URI when tapped.
type URIAction struct {
	Label string `json:"label"`
	URI   string `json:"uri"`
}

func (TextMessage) isMessage()      {}
func (StickerMessage) isMessage()   {}
func (TemplateMessage) isMessage()  {}
func (ButtonsTemplate) isTemplate() {}
func (ConfirmTemplate) isTemplate() {}
func (PostbackAction) isAction()    {}
func (MessageAction) isAction()     {}
func (URIAction) isAction()         {}

// Each appendJSON writes the "type" field the API requires, then the fields.
// Fields with omitempty are left out when empty.

func (m TextMessage) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"text","text":`...)
	dst = appendJSONString(dst, m.Text)
	return append(dst, '}')
}

func (m StickerMessage) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"sticker","packageId":`...)
	dst = appendJSONString(dst, m.PackageID)
	dst = append(dst, `,"stickerId":`...)
	dst = appendJSONString(dst, m.StickerID)
	return append(dst, '}')
}

func (m TemplateMessage) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"template","altText":`...)
	dst = appendJSONString(dst, m.AltText)
	dst = append(dst, `,"template":`...)
	dst = appendTemplate(dst, m.Template)
	return append(dst, '}')
}

func (t ButtonsTemplate) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"buttons"`...)
	dst = appendOmitEmpty(dst, `,"thumbnailImageUrl":`, t.ThumbnailImageURL)
	dst = appendOmitEmpty(dst, `,"imageAspectRatio":`, t.ImageAspectRatio)
	dst = appendOmitEmpty(dst, `,"imageSize":`, t.ImageSize)
	dst = appendOmitEmpty(dst, `,"imageBackgroundColor":`, t.ImageBackgroundColor)
	dst = appendOmitEmpty(dst, `,"title":`, t.Title)
	dst = append(dst, `,"text":`...)
	dst = appendJSONString(dst, t.Text)
	dst = append(dst, `,"actions":`...)
	dst = appendArray(dst, t.Actions, appendAction)
	return append(dst, '}')
}

func (t ConfirmTemplate) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"confirm","text":`...)
	dst = appendJSONString(dst, t.Text)
	dst = append(dst, `,"actions":`...)
	dst = appendArray(dst, t.Actions, appendAction)
	return append(dst, '}')
}

func (a PostbackAction) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"postback","label":`...)
	dst = appendJSONString(dst, a.Label)
	dst = append(dst, `,"data":`...)
	dst = appendJSONString(dst, a.Data)
	dst = appendOmitEmpty(dst, `,"displayText":`, a.DisplayText)
	return append(dst, '}')
}

func (a MessageAction) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"message","label":`...)
	dst = appendJSONString(dst, a.Label)
	dst = append(dst, `,"text":`...)
	dst = appendJSONString(dst, a.Text)
	return append(dst, '}')
}

func (a URIAction) appendJSON(dst []byte) []byte {
	dst = append(dst, `{"type":"uri","label":`...)
	dst = appendJSONString(dst, a.Label)
	dst = append(dst, `,"uri":`...)
	dst = appendJSONString(dst, a.URI)
	return append(dst, '}')
}

// appendOmitEmpty appends key (a comma, the quoted name and a colon) and s,
// unless s is empty.
func appendOmitEmpty(dst []byte, key, s string) []byte {
	if s == "" {
		return dst
	}
	dst = append(dst, key...)
	return appendJSONString(dst, s)
}

// appendMessage, appendTemplate and appendAction write a nil interface or a
// nil pointer as null, as encoding/json does. The value receivers of
// appendJSON make the pointer types satisfy the interfaces too, and calling
// appendJSON on a nil pointer would panic.

func appendMessage(dst []byte, m Message) []byte {
	switch v := m.(type) {
	case nil:
		return append(dst, "null"...)
	case *TextMessage:
		if v == nil {
			return append(dst, "null"...)
		}
	case *StickerMessage:
		if v == nil {
			return append(dst, "null"...)
		}
	case *TemplateMessage:
		if v == nil {
			return append(dst, "null"...)
		}
	}
	return m.appendJSON(dst)
}

func appendTemplate(dst []byte, t Template) []byte {
	switch v := t.(type) {
	case nil:
		return append(dst, "null"...)
	case *ButtonsTemplate:
		if v == nil {
			return append(dst, "null"...)
		}
	case *ConfirmTemplate:
		if v == nil {
			return append(dst, "null"...)
		}
	}
	return t.appendJSON(dst)
}

func appendAction(dst []byte, a Action) []byte {
	switch v := a.(type) {
	case nil:
		return append(dst, "null"...)
	case *PostbackAction:
		if v == nil {
			return append(dst, "null"...)
		}
	case *MessageAction:
		if v == nil {
			return append(dst, "null"...)
		}
	case *URIAction:
		if v == nil {
			return append(dst, "null"...)
		}
	}
	return a.appendJSON(dst)
}

// MarshalJSON keeps json.Marshal of a message working; it gives the same
// bytes as appendJSON.

func (m TextMessage) MarshalJSON() ([]byte, error)     { return m.appendJSON(nil), nil }
func (m StickerMessage) MarshalJSON() ([]byte, error)  { return m.appendJSON(nil), nil }
func (m TemplateMessage) MarshalJSON() ([]byte, error) { return m.appendJSON(nil), nil }
func (t ButtonsTemplate) MarshalJSON() ([]byte, error) { return t.appendJSON(nil), nil }
func (t ConfirmTemplate) MarshalJSON() ([]byte, error) { return t.appendJSON(nil), nil }
func (a PostbackAction) MarshalJSON() ([]byte, error)  { return a.appendJSON(nil), nil }
func (a MessageAction) MarshalJSON() ([]byte, error)   { return a.appendJSON(nil), nil }
func (a URIAction) MarshalJSON() ([]byte, error)       { return a.appendJSON(nil), nil }

// UTF16Len returns the length of s in UTF-16 code units, which is how the
// Messaging API counts characters.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Units(r)
	}
	return n
}

// TruncateUTF16 returns the longest prefix of s that fits in max UTF-16 code
// units without splitting a character (surrogate pairs stay intact).
func TruncateUTF16(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i, r := range s {
		n += utf16Units(r)
		if n > max {
			return s[:i]
		}
	}
	return s
}

func utf16Units(r rune) int {
	if r >= 0x10000 && r <= utf8.MaxRune {
		return 2
	}
	return 1
}
