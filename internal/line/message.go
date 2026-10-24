package line

import (
	"encoding/json"
	"unicode/utf8"
)

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
}

// Template is the template of a TemplateMessage.
type Template interface {
	isTemplate()
}

// Action is an action of a template.
type Action interface {
	isAction()
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

// Each MarshalJSON adds the "type" field the API requires. Embedding a local
// alias type flattens the fields and avoids recursing into MarshalJSON.

func (m TextMessage) MarshalJSON() ([]byte, error) {
	type alias TextMessage
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"text", alias(m)})
}

func (m StickerMessage) MarshalJSON() ([]byte, error) {
	type alias StickerMessage
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"sticker", alias(m)})
}

func (m TemplateMessage) MarshalJSON() ([]byte, error) {
	type alias TemplateMessage
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"template", alias(m)})
}

func (t ButtonsTemplate) MarshalJSON() ([]byte, error) {
	type alias ButtonsTemplate
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"buttons", alias(t)})
}

func (t ConfirmTemplate) MarshalJSON() ([]byte, error) {
	type alias ConfirmTemplate
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"confirm", alias(t)})
}

func (a PostbackAction) MarshalJSON() ([]byte, error) {
	type alias PostbackAction
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"postback", alias(a)})
}

func (a MessageAction) MarshalJSON() ([]byte, error) {
	type alias MessageAction
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"message", alias(a)})
}

func (a URIAction) MarshalJSON() ([]byte, error) {
	type alias URIAction
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{"uri", alias(a)})
}

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
