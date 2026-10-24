package app

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

// discardDoer answers every reply API call with 200 after reading the body,
// so the benchmarks measure the handler and not the fake.
type discardDoer struct{}

func (discardDoer) Do(req *http.Request) (*http.Response, error) {
	io.Copy(io.Discard, req.Body)
	return response(200, `{}`), nil
}

// benchmarkWebhook measures one signed webhook request with a text message,
// from signature verification to sending the reply. Profile it with
//
//	go test -run '^$' -bench Webhook -cpuprofile cpu.out ./internal/app
func benchmarkWebhook(b *testing.B, text string) {
	vars := map[string]string{EnvChannelSecret: testSecret, EnvChannelAccessToken: testToken}
	h := NewHandler(Deps{
		Getenv:    func(k string) string { return vars[k] },
		Doer:      discardDoer{},
		Countries: country.Default(),
		Logf:      log.New(io.Discard, "", log.LstdFlags).Printf,
	})
	body := eventsJSON(textEventJSON("bench-token", text))
	signature := sign(testSecret, body)
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, DefaultWebhookPath, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Line-Signature", signature)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}

func BenchmarkWebhookCountry(b *testing.B) { benchmarkWebhook(b, "日本") }

func BenchmarkWebhookQuiz(b *testing.B) { benchmarkWebhook(b, "クイズ") }
