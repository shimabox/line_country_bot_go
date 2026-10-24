package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

const (
	testSecret = "test-channel-secret"
	testToken  = "test-access-token"
)

type sentRequest struct {
	URL           string
	Authorization string
	ContentType   string
	Body          replyBody
}

type replyBody struct {
	ReplyToken string           `json:"replyToken"`
	Messages   []map[string]any `json:"messages"`
}

// fakeDoer records reply API calls. respond decides the result of call n
// (0-based); nil means 200.
type fakeDoer struct {
	mu      sync.Mutex
	sent    []sentRequest
	respond func(n int) (*http.Response, error)
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := io.ReadAll(req.Body)
	var body replyBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("bad reply body %s: %w", raw, err)
	}
	f.sent = append(f.sent, sentRequest{
		URL:           req.URL.String(),
		Authorization: req.Header.Get("Authorization"),
		ContentType:   req.Header.Get("Content-Type"),
		Body:          body,
	})
	n := len(f.sent) - 1
	if f.respond != nil {
		if resp, err := f.respond(n); resp != nil || err != nil {
			return resp, err
		}
	}
	return response(200, `{}`), nil
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

type testEnv struct {
	handler http.Handler
	doer    *fakeDoer
	logs    *bytes.Buffer
}

func newTestEnv(t *testing.T, env map[string]string) *testEnv {
	t.Helper()
	store := country.Default()
	vars := map[string]string{
		EnvChannelSecret:      testSecret,
		EnvChannelAccessToken: testToken,
	}
	for k, v := range env {
		vars[k] = v
	}
	doer := &fakeDoer{}
	logs := &bytes.Buffer{}
	var logMu sync.Mutex
	h := NewHandler(Deps{
		Getenv:    func(k string) string { return vars[k] },
		Doer:      doer,
		Countries: store,
		NewRand:   func() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) },
		Logf: func(format string, args ...any) {
			logMu.Lock()
			defer logMu.Unlock()
			fmt.Fprintf(logs, format+"\n", args...)
		},
	})
	return &testEnv{handler: h, doer: doer, logs: logs}
}

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (e *testEnv) do(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) webhook(path, body string) *httptest.ResponseRecorder {
	return e.do(http.MethodPost, path, body, map[string]string{
		"Content-Type":     "application/json",
		"X-Line-Signature": sign(testSecret, body),
	})
}

func textEventJSON(token, text string) string {
	b, _ := json.Marshal(map[string]any{
		"type":            "message",
		"replyToken":      token,
		"timestamp":       1700000000000,
		"source":          map[string]string{"type": "user", "userId": "Uuser00000000000000000000000000000"},
		"deliveryContext": map[string]bool{"isRedelivery": false},
		"message":         map[string]string{"id": "1", "type": "text", "text": text},
	})
	return string(b)
}

func eventsJSON(events ...string) string {
	return `{"destination":"Udestination","events":[` + strings.Join(events, ",") + `]}`
}

func TestRoot(t *testing.T) {
	e := newTestEnv(t, nil)
	rec := e.do(http.MethodGet, "/", "", nil)
	if rec.Code != 200 || rec.Body.Len() == 0 {
		t.Errorf("GET / = %d %q", rec.Code, rec.Body.String())
	}
	if rec := e.do(http.MethodPost, "/", "", nil); rec.Code != 405 {
		t.Errorf("POST / = %d", rec.Code)
	}
}

func TestNotFound(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, tt := range []struct{ method, path string }{
		{"GET", "/other"},
		{"POST", "/other"},
		{"GET", "/webhook/extra"},
		{"GET", "/flags/zz.png"},
	} {
		if rec := e.do(tt.method, tt.path, "", nil); rec.Code != 404 {
			t.Errorf("%s %s = %d", tt.method, tt.path, rec.Code)
		}
	}
}

func TestWebhookMethodNotAllowed(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		rec := e.do(m, "/webhook", "", nil)
		if rec.Code != 405 || rec.Header().Get("Allow") != "POST" {
			t.Errorf("%s /webhook = %d Allow=%q", m, rec.Code, rec.Header().Get("Allow"))
		}
	}
}

func TestWebhookInvalidSignature(t *testing.T) {
	e := newTestEnv(t, nil)
	body := eventsJSON(textEventJSON("rt", "日本"))
	for name, headers := range map[string]map[string]string{
		"missing":      {},
		"wrong secret": {"X-Line-Signature": sign("other-secret", body)},
		"not base64":   {"X-Line-Signature": "!!!"},
	} {
		rec := e.do(http.MethodPost, "/webhook", body, headers)
		if rec.Code != 400 || strings.TrimSpace(rec.Body.String()) != "Invalid signature." {
			t.Errorf("%s: %d %q", name, rec.Code, rec.Body.String())
		}
	}
	if len(e.doer.sent) != 0 {
		t.Errorf("reply sent for an invalid signature: %d", len(e.doer.sent))
	}
}

func TestWebhookInvalidJSON(t *testing.T) {
	e := newTestEnv(t, nil)
	for _, body := range []string{`{`, `[]`, `{"events":{}}`, ``} {
		if rec := e.webhook("/webhook", body); rec.Code != 400 {
			t.Errorf("%q: %d", body, rec.Code)
		}
	}
}

func TestWebhookEmptyEvents(t *testing.T) {
	e := newTestEnv(t, nil)
	rec := e.webhook("/webhook", `{"destination":"Udestination","events":[]}`)
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	if len(e.doer.sent) != 0 {
		t.Errorf("sent = %d", len(e.doer.sent))
	}
}

func TestWebhookTextReply(t *testing.T) {
	e := newTestEnv(t, nil)
	rec := e.webhook("/webhook", eventsJSON(textEventJSON("reply-token-1", "日本")))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(e.doer.sent) != 1 {
		t.Fatalf("sent = %d", len(e.doer.sent))
	}
	s := e.doer.sent[0]
	if s.URL != "https://api.line.me/v2/bot/message/reply" || s.Authorization != "Bearer "+testToken || s.ContentType != "application/json" {
		t.Errorf("request = %+v", s)
	}
	if s.Body.ReplyToken != "reply-token-1" || len(s.Body.Messages) != 3 {
		t.Fatalf("body = %+v", s.Body)
	}
	if got := s.Body.Messages[0]["text"]; got != "[日本] ですね🤔" {
		t.Errorf("first text = %v", got)
	}
	tmpl := s.Body.Messages[2]["template"].(map[string]any)
	// httptest requests use Host example.com.
	if got := tmpl["thumbnailImageUrl"]; got != "https://example.com/flags/jp.png" {
		t.Errorf("thumbnail = %v", got)
	}
}

func TestWebhookConfigOverrides(t *testing.T) {
	e := newTestEnv(t, map[string]string{
		EnvWebhookPath:    "/hook",
		EnvPublicBaseURL:  "https://bot.example.org/",
		EnvLineAPIBaseURL: "http://127.0.0.1:9999",
	})
	if rec := e.webhook("/webhook", eventsJSON()); rec.Code != 404 {
		t.Errorf("/webhook = %d, want 404 when WEBHOOK_PATH is /hook", rec.Code)
	}
	rec := e.webhook("/hook", eventsJSON(textEventJSON("rt", "日本")))
	if rec.Code != 200 || len(e.doer.sent) != 1 {
		t.Fatalf("status = %d sent = %d", rec.Code, len(e.doer.sent))
	}
	s := e.doer.sent[0]
	if s.URL != "http://127.0.0.1:9999/v2/bot/message/reply" {
		t.Errorf("URL = %s", s.URL)
	}
	tmpl := s.Body.Messages[2]["template"].(map[string]any)
	if got := tmpl["thumbnailImageUrl"]; got != "https://bot.example.org/flags/jp.png" {
		t.Errorf("thumbnail = %v", got)
	}
}

func TestWebhookProfileFromEnv(t *testing.T) {
	profile := map[string]string{
		EnvProfileImageURL: "https://example.com/p.png",
		EnvProfileTitle:    "国名bot",
		EnvProfileText:     "国名を送ってください",
		EnvProfileLinkText: "ソース",
		EnvProfileLinkURL:  "https://example.com/",
	}
	e := newTestEnv(t, profile)
	e.webhook("/webhook", eventsJSON(textEventJSON("rt", "who")))
	if len(e.doer.sent) != 1 || e.doer.sent[0].Body.Messages[0]["type"] != "template" {
		t.Fatalf("sent = %+v", e.doer.sent)
	}

	delete(profile, EnvProfileLinkURL)
	e = newTestEnv(t, profile)
	e.webhook("/webhook", eventsJSON(textEventJSON("rt", "who")))
	if got := e.doer.sent[0].Body.Messages[0]["text"]; got != "[who] ですね🤔" {
		t.Errorf("incomplete profile: first = %v, want country search", got)
	}
	if !strings.Contains(e.logs.String(), "PROFILE_LINK_URL") {
		t.Errorf("no warning about the missing profile field:\n%s", e.logs)
	}
}

func TestWebhookReplyFailureStill200(t *testing.T) {
	e := newTestEnv(t, nil)
	e.doer.respond = func(int) (*http.Response, error) {
		return response(400, `{"message":"Invalid reply token"}`), nil
	}
	rec := e.webhook("/webhook", eventsJSON(textEventJSON("used-token", "日本")))
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(e.logs.String(), "400") {
		t.Errorf("status not logged:\n%s", e.logs)
	}
}

func TestReplyFailureLogsOnlyStatusAndClass(t *testing.T) {
	e := newTestEnv(t, nil)
	e.doer.respond = func(n int) (*http.Response, error) {
		if n == 0 {
			return response(400, `{"message":"Invalid reply token","details":"body-only-marker"}`), nil
		}
		return nil, errors.New("dial tcp: transport-only-marker")
	}
	e.webhook("/webhook", eventsJSON(textEventJSON("rt-1", "日本"), textEventJSON("rt-2", "日本")))
	logs := e.logs.String()
	for _, s := range []string{"body-only-marker", "Invalid reply token", "transport-only-marker", "dial tcp"} {
		if strings.Contains(logs, s) {
			t.Errorf("logs contain %q:\n%s", s, logs)
		}
	}
	for _, s := range []string{"status=400 error=reply_api_error", "status=0 error=transport_error"} {
		if !strings.Contains(logs, s) {
			t.Errorf("logs miss %q:\n%s", s, logs)
		}
	}
}

func TestReplyErrorWithoutTokenIsRequestError(t *testing.T) {
	e := newTestEnv(t, map[string]string{EnvChannelAccessToken: ""})
	e.webhook("/webhook", eventsJSON(textEventJSON("rt", "日本")))
	if !strings.Contains(e.logs.String(), "status=0 error=request_error") {
		t.Errorf("logs:\n%s", e.logs)
	}
}

func TestWebhookEventsAreIndependent(t *testing.T) {
	e := newTestEnv(t, nil)
	e.doer.respond = func(n int) (*http.Response, error) {
		if n == 0 {
			return nil, errors.New("network down")
		}
		return nil, nil
	}
	follow := `{"type":"follow","replyToken":"rt-follow","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false}}`
	unknown := `{"type":"somethingNew","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false}}`
	image := `{"type":"message","replyToken":"rt-image","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":true},"message":{"id":"2","type":"image"}}`
	rec := e.webhook("/webhook", eventsJSON(textEventJSON("rt-text", "日本"), follow, unknown, image))
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	if len(e.doer.sent) != 2 {
		t.Fatalf("sent = %d, want 2 (text and image; follow and unknown ignored)", len(e.doer.sent))
	}
	if e.doer.sent[0].Body.ReplyToken != "rt-text" || e.doer.sent[1].Body.ReplyToken != "rt-image" {
		t.Errorf("tokens = %s, %s", e.doer.sent[0].Body.ReplyToken, e.doer.sent[1].Body.ReplyToken)
	}
	if got := e.doer.sent[1].Body.Messages[1]["stickerId"]; got != "1999" {
		t.Errorf("image reply = %v", e.doer.sent[1].Body.Messages)
	}
}

func TestLogsDoNotContainUserTextOrTokens(t *testing.T) {
	e := newTestEnv(t, nil)
	e.doer.respond = func(int) (*http.Response, error) { return response(500, `{"message":"error"}`), nil }
	e.webhook("/webhook", eventsJSON(textEventJSON("secret-reply-token", "ひみつの国名テキスト"), textEventJSON("secret-reply-token-2", "日本")))
	logs := e.logs.String()
	if logs == "" {
		t.Fatal("nothing logged")
	}
	for _, s := range []string{"secret-reply-token", "ひみつの国名テキスト", "日本", testToken, testSecret} {
		if strings.Contains(logs, s) {
			t.Errorf("logs contain %q:\n%s", s, logs)
		}
	}
	if !strings.Contains(logs, "JP") {
		t.Errorf("logs miss the country code:\n%s", logs)
	}
}

func TestWebhookMissingSecret(t *testing.T) {
	e := newTestEnv(t, map[string]string{EnvChannelSecret: ""})
	body := eventsJSON()
	rec := e.do(http.MethodPost, "/webhook", body, map[string]string{"X-Line-Signature": sign("", body)})
	if rec.Code != 500 {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg := LoadConfig(func(string) string { return "" })
	if cfg.WebhookPath != "/webhook" || cfg.LineAPIBaseURL != "https://api.line.me" || cfg.PublicBaseURL != "" {
		t.Errorf("defaults = %+v", cfg)
	}
	if cfg.Profile.Enabled() {
		t.Error("profile enabled by default")
	}
}
