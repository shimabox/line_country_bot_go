// Package bot decides how the bot answers each webhook event: input
// normalization, trigger words, emoji and the reply messages.
package bot

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

// Input is a normalized user message.
type Input struct {
	// Text has every whitespace character (including U+3000) removed.
	Text string
	// Lower is Text with ASCII letters lowercased.
	Lower string
}

// Normalize removes whitespace and builds the lowercase form used for
// trigger words and searches.
func Normalize(s string) Input {
	text := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '　' {
			return -1
		}
		return r
	}, s)
	return Input{Text: text, Lower: country.NormalizeKey(text)}
}

// Trigger is what a text message asks for.
type Trigger int

const (
	// TriggerCountry searches the text as a country name.
	TriggerCountry Trigger = iota
	// TriggerProfile shows the self-introduction.
	TriggerProfile
	// TriggerQuiz starts a flag quiz.
	TriggerQuiz
	// TriggerCancel ends the conversation without a reply.
	TriggerCancel
)

func (t Trigger) String() string {
	switch t {
	case TriggerProfile:
		return "profile"
	case TriggerQuiz:
		return "quiz"
	case TriggerCancel:
		return "cancel"
	default:
		return "country"
	}
}

// Trigger words, compared with Input.Lower (cancel with Input.Text).
const (
	wordProfileJA = "君の名は"
	wordProfileEN = "who"
	wordQuizJA    = "クイズ"
	wordQuizEN    = "quiz"
	wordCancel    = "いいえ"
)

// Classify decides what the input asks for, in this order: self-introduction
// (only when the profile is enabled), quiz, cancel, country search.
func Classify(in Input, profileEnabled bool) Trigger {
	switch {
	case profileEnabled && (in.Lower == wordProfileJA || in.Lower == wordProfileEN):
		return TriggerProfile
	case in.Lower == wordQuizJA || in.Lower == wordQuizEN:
		return TriggerQuiz
	case in.Text == wordCancel:
		return TriggerCancel
	default:
		return TriggerCountry
	}
}

// Profile is the self-introduction shown for "君の名は" / "who". It is
// disabled when ImageURL is empty or any other field is missing.
type Profile struct {
	ImageURL string
	Title    string
	Text     string
	LinkText string
	LinkURL  string
}

// Enabled reports whether every field is set.
func (p Profile) Enabled() bool {
	return p.ImageURL != "" && len(p.missing()) == 0
}

// Validate returns an error when ImageURL is set (the profile is meant to be
// enabled) but other fields are missing. An empty ImageURL disables the
// profile on purpose and is not an error.
func (p Profile) Validate() error {
	if p.ImageURL == "" {
		return nil
	}
	if m := p.missing(); len(m) > 0 {
		return fmt.Errorf("bot: profile is disabled because %s is empty", strings.Join(m, ", "))
	}
	return nil
}

func (p Profile) missing() []string {
	var m []string
	for _, f := range []struct{ name, value string }{
		{"PROFILE_TITLE", p.Title},
		{"PROFILE_TEXT", p.Text},
		{"PROFILE_LINK_TEXT", p.LinkText},
		{"PROFILE_LINK_URL", p.LinkURL},
	} {
		if f.value == "" {
			m = append(m, f.name)
		}
	}
	return m
}
