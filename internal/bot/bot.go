package bot

import (
	"math/rand/v2"
	"net/url"

	"github.com/shimabox/line_country_bot_go/internal/country"
	"github.com/shimabox/line_country_bot_go/internal/line"
)

// Reply texts.
const (
	textNonText      = "テキストメッセージ以外わからないです"
	echoOpen         = "["
	echoClose        = "]"
	suffixEcho       = " ですね" + EmojiThinking
	suffixAnswerEcho = " ですね?"
	textFound        = "わかります"
	textNotFound     = "む、むむっ、、わかりませんでした"
	textCorrect      = "む、むむっ、、\n正解です"
	textWrong        = "む、むむっ、、\n残念！はずれです"
	textQuizTitle    = "クイズです"
	textQuizQuestion = "この国はどこでしょうか？"
	textRetry        = "もう一度やりますか？"
	labelNo          = "いいえ"
	labelYes         = "はい"
	labelWikipedia   = "Wikipedia"
	labelMaps        = "Google Maps"

	// Sticker sent with textNonText.
	stickerPackageID = "446"
	stickerID        = "1999"

	quizChoices    = 4
	flagBackground = "#FFFFFF"
	postbackAnswer = "answer"
	postbackResult = "result"
	resultCorrect  = "1"
	resultWrong    = "0"
)

// Kind says which reply was chosen. It is meant for logs, which must not
// contain what the user wrote.
type Kind string

const (
	KindIgnored         Kind = "ignored"
	KindNonText         Kind = "non-text"
	KindProfile         Kind = "profile"
	KindQuiz            Kind = "quiz"
	KindCancel          Kind = "cancel"
	KindCountryHit      Kind = "country-hit"
	KindCountryMiss     Kind = "country-miss"
	KindAnswerCorrect   Kind = "answer-correct"
	KindAnswerWrong     Kind = "answer-wrong"
	KindInvalidPostback Kind = "invalid-postback"
)

// Options are per-request settings.
type Options struct {
	// FlagBaseURL is the origin that serves /flags/*.png.
	FlagBaseURL string
	// Profile is the self-introduction; it is used only when enabled.
	Profile Profile
}

// Reply is the bot's answer to one event. Messages is nil when nothing should
// be sent.
type Reply struct {
	Messages []line.Message
	Kind     Kind
	// Code is the ISO code of the country involved, if any.
	Code string
}

// Bot builds replies from the country data. A Bot is not safe for concurrent
// use because it owns the random source.
type Bot struct {
	countries *country.Store
	rand      *rand.Rand
}

// New returns a Bot. r drives quiz choices and emoji; pass a fixed seed in
// tests.
func New(countries *country.Store, r *rand.Rand) *Bot {
	return &Bot{countries: countries, rand: r}
}

// Respond decides the reply for an event. It never returns more than
// line.MaxReplyMessages messages.
func (b *Bot) Respond(ev line.Event, opts Options) Reply {
	switch ev.Type {
	case line.EventTypeMessage:
		if ev.Message == nil {
			return Reply{Kind: KindIgnored}
		}
		if ev.Message.Type != line.MessageTypeText {
			return Reply{Kind: KindNonText, Messages: []line.Message{
				line.TextMessage{Text: textNonText},
				line.StickerMessage{PackageID: stickerPackageID, StickerID: stickerID},
			}}
		}
		return b.respondText(Normalize(ev.Message.Text), opts)
	case line.EventTypePostback:
		if ev.Postback == nil {
			return Reply{Kind: KindIgnored}
		}
		return b.respondAnswer(ev.Postback.Data)
	default:
		return Reply{Kind: KindIgnored}
	}
}

func (b *Bot) respondText(in Input, opts Options) Reply {
	switch Classify(in, opts.Profile.Enabled()) {
	case TriggerProfile:
		return Reply{Kind: KindProfile, Messages: []line.Message{profileMessage(opts.Profile)}}
	case TriggerQuiz:
		return b.quiz(opts.FlagBaseURL)
	case TriggerCancel:
		return Reply{Kind: KindCancel}
	}

	echo := textMessage(in.Text, suffixEcho)
	c := b.countries.Lookup(in.Text)
	if c == nil {
		return Reply{Kind: KindCountryMiss, Messages: []line.Message{
			echo,
			line.TextMessage{Text: textNotFound + b.pick(EmojiSad)},
		}}
	}
	return Reply{Kind: KindCountryHit, Code: c.Code, Messages: []line.Message{
		echo,
		line.TextMessage{Text: textFound + b.pick(EmojiSmile)},
		detailMessage(c, in.Text, c.FlagURL(opts.FlagBaseURL)),
	}}
}

func (b *Bot) quiz(flagBaseURL string) Reply {
	choices := b.countries.PickN(b.rand, quizChoices)
	if len(choices) == 0 {
		return Reply{Kind: KindQuiz}
	}
	answer := choices[b.rand.IntN(len(choices))]
	b.rand.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })

	actions := make([]line.Action, len(choices))
	for i, c := range choices {
		result := resultWrong
		if c == answer {
			result = resultCorrect
		}
		actions[i] = line.PostbackAction{
			Label: line.TruncateUTF16(c.NameJA, line.MaxActionLabel),
			Data:  url.Values{postbackAnswer: {c.NameJA}, postbackResult: {result}}.Encode(),
		}
	}
	return Reply{Kind: KindQuiz, Code: answer.Code, Messages: []line.Message{line.TemplateMessage{
		AltText: textQuizQuestion,
		Template: line.ButtonsTemplate{
			ThumbnailImageURL:    answer.FlagURL(flagBaseURL),
			ImageAspectRatio:     line.ImageAspectRatioRectangle,
			ImageSize:            line.ImageSizeContain,
			ImageBackgroundColor: flagBackground,
			Title:                textQuizTitle,
			Text:                 textQuizQuestion,
			Actions:              actions,
		},
	}}}
}

// respondAnswer handles the postback of a quiz button. Data that is not ours
// gets no reply.
func (b *Bot) respondAnswer(data string) Reply {
	v, err := url.ParseQuery(data)
	name, result := v.Get(postbackAnswer), v.Get(postbackResult)
	if err != nil || name == "" || (result != resultCorrect && result != resultWrong) {
		return Reply{Kind: KindInvalidPostback}
	}

	echo := textMessage(name, suffixAnswerEcho)
	if result == resultWrong {
		return Reply{Kind: KindAnswerWrong, Messages: []line.Message{
			echo,
			line.TextMessage{Text: textWrong + b.pick(EmojiRegret)},
		}}
	}

	msgs := []line.Message{echo, line.TextMessage{Text: textCorrect + b.pick(EmojiCelebrate)}}
	r := Reply{Kind: KindAnswerCorrect}
	// The name may be missing after a data update; skip the details then.
	if c := b.countries.ByName(name); c != nil {
		r.Code = c.Code
		msgs = append(msgs, detailMessage(c, c.NameJA, ""))
	}
	r.Messages = append(msgs, retryMessage())
	return r
}

// detailMessage shows the capital and links for a country. imageURL may be
// empty to omit the flag.
func detailMessage(c *country.Country, title, imageURL string) line.Message {
	bt := line.ButtonsTemplate{
		Title: line.TruncateUTF16(title, line.MaxButtonsTitle),
		Text:  line.TruncateUTF16(c.CapitalText(), line.MaxButtonsTextImage),
		Actions: []line.Action{
			line.URIAction{Label: labelWikipedia, URI: c.WikipediaURL()},
			line.URIAction{Label: labelMaps, URI: c.MapURL()},
		},
	}
	if imageURL != "" {
		bt.ThumbnailImageURL = imageURL
		bt.ImageAspectRatio = line.ImageAspectRatioRectangle
		bt.ImageSize = line.ImageSizeContain
		bt.ImageBackgroundColor = flagBackground
	}
	return line.TemplateMessage{AltText: line.TruncateUTF16(title, line.MaxAltText), Template: bt}
}

func retryMessage() line.Message {
	return line.TemplateMessage{
		AltText: textRetry,
		Template: line.ConfirmTemplate{
			Text: textRetry,
			Actions: []line.Action{
				line.MessageAction{Label: labelNo, Text: wordCancel},
				line.MessageAction{Label: labelYes, Text: wordQuizJA},
			},
		},
	}
}

func profileMessage(p Profile) line.Message {
	return line.TemplateMessage{
		AltText: line.TruncateUTF16(p.Title, line.MaxAltText),
		Template: line.ButtonsTemplate{
			ThumbnailImageURL: p.ImageURL,
			Title:             line.TruncateUTF16(p.Title, line.MaxButtonsTitle),
			Text:              line.TruncateUTF16(p.Text, line.MaxButtonsTextImage),
			Actions: []line.Action{
				line.URIAction{Label: line.TruncateUTF16(p.LinkText, line.MaxActionLabel), URI: p.LinkURL},
			},
		},
	}
}

// textMessage echoes user input in brackets followed by a suffix, keeping the
// message within the API's length limit.
func textMessage(input, suffix string) line.TextMessage {
	limit := line.MaxTextMessageLength - line.UTF16Len(echoOpen+echoClose+suffix)
	return line.TextMessage{Text: echoOpen + line.TruncateUTF16(input, limit) + echoClose + suffix}
}

func (b *Bot) pick(group []string) string {
	return group[b.rand.IntN(len(group))]
}
