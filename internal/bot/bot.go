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

	quizChoices = 4
	// flagBackgroundContain fills the frame around a flag shown whole.
	flagBackgroundContain = "#FFFFFF"

	// Postback data of a quiz button, encoded with url.Values (keys sorted):
	// answer=<label>&code=<ISO code>[&q=capital]&result=<0|1>. answer is
	// echoed back, code finds the country of the button and q tells the
	// quiz. Buttons sent before code existed have only answer and result;
	// their country is found by answer, the display name.
	postbackAnswer = "answer"
	postbackCode   = "code"
	postbackQuiz   = "q"
	postbackResult = "result"
	resultCorrect  = "1"
	resultWrong    = "0"
)

// Kind says which reply was chosen. It is meant for logs, which must not
// contain what the user wrote.
type Kind string

const (
	KindIgnored Kind = "ignored"
	KindNonText Kind = "non-text"
	KindProfile Kind = "profile"
	KindCancel  Kind = "cancel"

	// Searches: KindCountryHit is found by a display name or alias,
	// KindCapitalHit by a capital only.
	KindCountryHit  Kind = "country-hit"
	KindCapitalHit  Kind = "capital-hit"
	KindCountryMiss Kind = "country-miss"

	// Quizzes: KindQuiz asks for the country of a flag, KindCapitalQuiz for
	// its capital. The answers to each have their own kinds.
	KindQuiz                 Kind = "quiz"
	KindCapitalQuiz          Kind = "capital-quiz"
	KindAnswerCorrect        Kind = "answer-correct"
	KindAnswerWrong          Kind = "answer-wrong"
	KindCapitalAnswerCorrect Kind = "capital-answer-correct"
	KindCapitalAnswerWrong   Kind = "capital-answer-wrong"
	KindInvalidPostback      Kind = "invalid-postback"
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
	// A name or alias is shown as typed. A capital alone would put the
	// capital above "capital / capital_en" without the country, so the card
	// is titled with the country's name instead.
	kind, title := KindCountryHit, in.Text
	if !c.NamedBy(in.Lower) {
		kind, title = KindCapitalHit, c.NameJA
	}
	return Reply{Kind: kind, Code: c.Code, Messages: []line.Message{
		echo,
		line.TextMessage{Text: textFound + b.pick(EmojiSmile)},
		detailMessage(c, title, c.FlagURL(opts.FlagBaseURL)),
	}}
}

func (b *Bot) quiz(flagBaseURL string) Reply {
	return b.quizFlag(flagBaseURL)
}

// quizFlag asks for the country of a flag, with four country names to
// choose from.
func (b *Bot) quizFlag(flagBaseURL string) Reply {
	choices, answer := b.pickQuiz()
	if answer == nil {
		return Reply{Kind: KindQuiz}
	}
	return flagQuizReply(choices, answer, flagBaseURL)
}

// pickQuiz picks the choices of a quiz in random order and the answer among
// them. answer is nil when there is no country.
func (b *Bot) pickQuiz() (choices []*country.Country, answer *country.Country) {
	choices = b.countries.PickN(b.rand, quizChoices)
	if len(choices) == 0 {
		return nil, nil
	}
	answer = choices[b.rand.IntN(len(choices))]
	b.rand.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
	return choices, answer
}

func flagQuizReply(choices []*country.Country, answer *country.Country, flagBaseURL string) Reply {
	return quizReply(KindQuiz, textQuizQuestion, "", choices, answer, flagBaseURL,
		func(c *country.Country) string { return c.NameJA })
}

// quizReply shows the answer's flag with a button per choice, labelled with
// name(choice). q is the postback's quiz value, "" for the flag quiz.
func quizReply(kind Kind, question, q string, choices []*country.Country, answer *country.Country,
	flagBaseURL string, name func(*country.Country) string) Reply {
	actions := make([]line.Action, len(choices))
	for i, c := range choices {
		result := resultWrong
		if c == answer {
			result = resultCorrect
		}
		data := url.Values{postbackAnswer: {name(c)}, postbackCode: {c.Code}, postbackResult: {result}}
		if q != "" {
			data.Set(postbackQuiz, q)
		}
		// DisplayText shows the full name in the chat so the user can see
		// which choice they tapped; the label may be truncated.
		actions[i] = line.PostbackAction{
			Label:       line.TruncateUTF16(name(c), line.MaxActionLabel),
			Data:        data.Encode(),
			DisplayText: name(c),
		}
	}
	size, background := flagImage(answer)
	return Reply{Kind: kind, Code: answer.Code, Messages: []line.Message{line.TemplateMessage{
		AltText: question,
		Template: line.ButtonsTemplate{
			ThumbnailImageURL:    answer.FlagURL(flagBaseURL),
			ImageAspectRatio:     line.ImageAspectRatioRectangle,
			ImageSize:            size,
			ImageBackgroundColor: background,
			Title:                textQuizTitle,
			Text:                 question,
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
	kindCorrect, kindWrong := KindAnswerCorrect, KindAnswerWrong

	echo := textMessage(name, suffixAnswerEcho)
	if result == resultWrong {
		return Reply{Kind: kindWrong, Messages: []line.Message{
			echo,
			line.TextMessage{Text: textWrong + b.pick(EmojiRegret)},
		}}
	}

	msgs := []line.Message{echo, line.TextMessage{Text: textCorrect + b.pick(EmojiCelebrate)}}
	r := Reply{Kind: kindCorrect}
	// The country may be missing after a data update; skip the details then.
	if c := b.answerCountry(v.Get(postbackCode), name); c != nil {
		r.Code = c.Code
		msgs = append(msgs, detailMessage(c, c.NameJA, ""))
	}
	r.Messages = append(msgs, retryMessage())
	return r
}

// answerCountry finds the country of a quiz button: by its code, or by the
// display name in answer for buttons without a well-formed code (sent before
// the code was added).
func (b *Bot) answerCountry(code, answer string) *country.Country {
	if isCountryCode(code) {
		return b.countries.ByCode(code)
	}
	return b.countries.ByName(answer)
}

// isCountryCode reports whether code has the form of the data's codes: two
// uppercase ASCII letters.
func isCountryCode(code string) bool {
	return len(code) == 2 && 'A' <= code[0] && code[0] <= 'Z' && 'A' <= code[1] && code[1] <= 'Z'
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
		bt.ImageSize, bt.ImageBackgroundColor = flagImage(c)
	}
	return line.TemplateMessage{AltText: line.TruncateUTF16(title, line.MaxAltText), Template: bt}
}

// flagImage decides how a flag is shown. Flags close to the frame's shape
// fill it ("cover"), so no thin white bars appear beside them, on a
// background of their own corner colour, so the rounded corners do not show
// a light rim; other flags are shown whole on white, which joins the white
// body below.
func flagImage(c *country.Country) (size, background string) {
	if c.FlagFitsFrame() {
		if col := c.FlagCornerColor(); col != "" {
			return line.ImageSizeCover, col
		}
		return line.ImageSizeCover, flagBackgroundContain
	}
	return line.ImageSizeContain, flagBackgroundContain
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
