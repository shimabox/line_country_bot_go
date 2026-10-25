package line

import "testing"

// Messages shaped like the bot's replies: a country name, and a quiz.
var (
	benchCountryReply = []Message{
		TextMessage{Text: "[日本] ですね🤔"},
		TextMessage{Text: "見つかりました😄"},
		TemplateMessage{
			AltText: "日本",
			Template: ButtonsTemplate{
				ThumbnailImageURL:    "https://line-country-bot.example.workers.dev/flags/jp.png",
				ImageAspectRatio:     ImageAspectRatioRectangle,
				ImageSize:            ImageSizeContain,
				ImageBackgroundColor: "#FFFFFF",
				Title:                "日本",
				Text:                 "首都: 東京 / Tokyo",
				Actions: []Action{
					URIAction{Label: "Wikipedia", URI: "https://ja.wikipedia.org/wiki/%E6%97%A5%E6%9C%AC"},
					URIAction{Label: "Google Maps", URI: "https://www.google.com/maps/search/?api=1&query=35.6812,139.7671"},
				},
			},
		},
	}
	benchQuizReply = []Message{
		TemplateMessage{
			AltText: "この国はどこでしょうか？",
			Template: ButtonsTemplate{
				ThumbnailImageURL:    "https://line-country-bot.example.workers.dev/flags/fr.png",
				ImageAspectRatio:     ImageAspectRatioRectangle,
				ImageSize:            ImageSizeCover,
				ImageBackgroundColor: "#FFFFFF",
				Title:                "国旗クイズ",
				Text:                 "この国はどこでしょうか？",
				Actions: []Action{
					PostbackAction{Label: "フランス", Data: "answer=%E3%83%95%E3%83%A9%E3%83%B3%E3%82%B9&result=1", DisplayText: "フランス"},
					PostbackAction{Label: "イタリア", Data: "answer=%E3%82%A4%E3%82%BF%E3%83%AA%E3%82%A2&result=0", DisplayText: "イタリア"},
					PostbackAction{Label: "ベルギー", Data: "answer=%E3%83%99%E3%83%AB%E3%82%AE%E3%83%BC&result=0", DisplayText: "ベルギー"},
					PostbackAction{Label: "アイルランド", Data: "answer=%E3%82%A2%E3%82%A4%E3%83%AB%E3%83%A9%E3%83%B3%E3%83%89&result=0", DisplayText: "アイルランド"},
				},
			},
		},
	}
)

// benchmarkReplyBody measures building a reply body the way Reply does.
func benchmarkReplyBody(b *testing.B, msgs []Message) {
	var n int
	b.ReportAllocs()
	for b.Loop() {
		n = len(appendReplyBody(make([]byte, 0, replyBodySize), "nHuyWiB7yP5Zw52FIkcQobQuGDXCTA", msgs))
	}
	b.ReportMetric(float64(n), "bytes")
}

func BenchmarkReplyBodyCountry(b *testing.B) { benchmarkReplyBody(b, benchCountryReply) }

func BenchmarkReplyBodyQuiz(b *testing.B) { benchmarkReplyBody(b, benchQuizReply) }
