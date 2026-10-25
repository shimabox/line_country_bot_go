package app

import (
	"bytes"
	"context"
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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shimabox/line_country_bot_go/internal/country"
)

const (
	testSecret = "test-channel-secret"
	testToken  = "test-access-token"
	testUserID = "Uuser00000000000000000000000000000"
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

type loadingRequest struct {
	URL           string
	Authorization string
	ContentType   string
	Raw           string
	Body          loadingBody
}

type loadingBody struct {
	ChatID         string `json:"chatId"`
	LoadingSeconds int    `json:"loadingSeconds"`
}

// fakeDoer records reply and loading API calls. respond decides the result of
// reply call n and respondLoading that of loading call n (0-based); nil means
// 200 and 202. calls lists the API calls and the handler's sleeps in order:
// "loading", "sleep <duration>" and "reply <reply token>".
type fakeDoer struct {
	mu             sync.Mutex
	sent           []sentRequest
	loading        []loadingRequest
	calls          []string
	respond        func(n int) (*http.Response, error)
	respondLoading func(n int) (*http.Response, error)
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := io.ReadAll(req.Body)
	if strings.HasSuffix(req.URL.Path, "/v2/bot/chat/loading/start") {
		var body loadingBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("bad loading body %s: %w", raw, err)
		}
		f.loading = append(f.loading, loadingRequest{
			URL:           req.URL.String(),
			Authorization: req.Header.Get("Authorization"),
			ContentType:   req.Header.Get("Content-Type"),
			Raw:           string(raw),
			Body:          body,
		})
		f.calls = append(f.calls, "loading")
		if f.respondLoading != nil {
			if resp, err := f.respondLoading(len(f.loading) - 1); resp != nil || err != nil {
				return resp, err
			}
		}
		return response(202, `{}`), nil
	}
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
	f.calls = append(f.calls, "reply "+body.ReplyToken)
	n := len(f.sent) - 1
	if f.respond != nil {
		if resp, err := f.respond(n); resp != nil || err != nil {
			return resp, err
		}
	}
	return response(200, `{}`), nil
}

// sleep records the handler's wait instead of waiting.
func (f *fakeDoer) sleep(_ context.Context, d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "sleep "+d.String())
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
		Sleep: doer.sleep,
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
	return sourcedTextEventJSON(token, text, map[string]string{"type": "user", "userId": testUserID})
}

func sourcedTextEventJSON(token, text string, source map[string]string) string {
	b, _ := json.Marshal(map[string]any{
		"type":            "message",
		"replyToken":      token,
		"timestamp":       1700000000000,
		"source":          source,
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
	// Loading for each replied event, one wait per webhook.
	want := []string{"loading", "sleep 1s", "reply rt-text", "loading", "reply rt-image"}
	if !slices.Equal(e.doer.calls, want) {
		t.Errorf("calls = %q, want %q", e.doer.calls, want)
	}
}

func TestLogsDoNotContainUserTextOrTokens(t *testing.T) {
	e := newTestEnv(t, nil)
	e.doer.respond = func(int) (*http.Response, error) { return response(500, `{"message":"error"}`), nil }
	// The loading API's error body may echo the chat ID.
	e.doer.respondLoading = func(n int) (*http.Response, error) {
		if n == 0 {
			return response(400, `{"message":"invalid chatId `+testUserID+`"}`), nil
		}
		return nil, errors.New("dial tcp: chatId=" + testUserID)
	}
	e.webhook("/webhook", eventsJSON(textEventJSON("secret-reply-token", "ひみつの国名テキスト"), textEventJSON("secret-reply-token-2", "日本")))
	logs := e.logs.String()
	if logs == "" {
		t.Fatal("nothing logged")
	}
	for _, s := range []string{"secret-reply-token", "ひみつの国名テキスト", "日本", testToken, testSecret, testUserID, "Uuser"} {
		if strings.Contains(logs, s) {
			t.Errorf("logs contain %q:\n%s", s, logs)
		}
	}
	if !strings.Contains(logs, "JP") {
		t.Errorf("logs miss the country code:\n%s", logs)
	}
	if len(e.doer.loading) != 2 {
		t.Errorf("loading calls = %d, want 2", len(e.doer.loading))
	}
}

func TestLogsDoNotContainUserIDOnSuccess(t *testing.T) {
	e := newTestEnv(t, nil)
	e.webhook("/webhook", eventsJSON(textEventJSON("rt", "日本")))
	logs := e.logs.String()
	if strings.Contains(logs, testUserID) || strings.Contains(logs, "rt ") {
		t.Errorf("logs contain the user ID or the token:\n%s", logs)
	}
	if !strings.Contains(logs, "loading=ok") {
		t.Errorf("logs miss loading=ok:\n%s", logs)
	}
}

func TestWebhookLoadingThenWaitThenReply(t *testing.T) {
	for _, tt := range []struct {
		env  map[string]string
		wait string
	}{
		{nil, "sleep 1s"},
		{map[string]string{EnvReplyDelayMS: "800"}, "sleep 800ms"},
		{map[string]string{EnvReplyDelayMS: "1200"}, "sleep 1.2s"},
	} {
		e := newTestEnv(t, tt.env)
		rec := e.webhook("/webhook", eventsJSON(textEventJSON("rt-1", "日本")))
		if rec.Code != 200 {
			t.Fatalf("status = %d", rec.Code)
		}
		want := []string{"loading", tt.wait, "reply rt-1"}
		if !slices.Equal(e.doer.calls, want) {
			t.Errorf("env %v: calls = %q, want %q", tt.env, e.doer.calls, want)
		}
		if len(e.doer.loading) != 1 {
			t.Fatalf("loading calls = %d", len(e.doer.loading))
		}
		l := e.doer.loading[0]
		if l.URL != "https://api.line.me/v2/bot/chat/loading/start" || l.Authorization != "Bearer "+testToken || l.ContentType != "application/json" {
			t.Errorf("loading request = %+v", l)
		}
		if l.Raw != `{"chatId":"`+testUserID+`","loadingSeconds":5}` {
			t.Errorf("loading body = %s", l.Raw)
		}
		if len(e.doer.sent) != 1 || len(e.doer.sent[0].Body.Messages) != 3 {
			t.Errorf("reply = %+v", e.doer.sent)
		}
	}
}

func TestWebhookPostbackShowsLoading(t *testing.T) {
	e := newTestEnv(t, nil)
	postback := `{"type":"postback","replyToken":"rt-pb","source":{"type":"user","userId":"` + testUserID + `"},"deliveryContext":{"isRedelivery":false},"postback":{"data":"answer=%E6%97%A5%E6%9C%AC&result=1"}}`
	e.webhook("/webhook", eventsJSON(postback))
	if len(e.doer.sent) != 1 || len(e.doer.sent[0].Body.Messages) != 4 {
		t.Fatalf("reply = %+v, want the correct-answer reply", e.doer.sent)
	}
	want := []string{"loading", "sleep 1s", "reply rt-pb"}
	if !slices.Equal(e.doer.calls, want) {
		t.Errorf("calls = %q, want %q", e.doer.calls, want)
	}
}

func TestWebhookGroupSourceRepliesWithoutLoading(t *testing.T) {
	e := newTestEnv(t, nil)
	rec := e.webhook("/webhook", eventsJSON(
		sourcedTextEventJSON("rt-group", "日本", map[string]string{"type": "group", "groupId": "Cgroup", "userId": testUserID}),
		sourcedTextEventJSON("rt-room", "日本", map[string]string{"type": "room", "roomId": "Rroom", "userId": testUserID}),
		sourcedTextEventJSON("rt-no-user", "日本", map[string]string{"type": "user"}),
	))
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	want := []string{"reply rt-group", "reply rt-room", "reply rt-no-user"}
	if !slices.Equal(e.doer.calls, want) {
		t.Errorf("calls = %q, want %q", e.doer.calls, want)
	}
	if got := strings.Count(e.logs.String(), "loading=skipped"); got != 3 {
		t.Errorf("loading=skipped logged %d times, want 3:\n%s", got, e.logs)
	}
}

func TestWebhookNoReplyNoLoading(t *testing.T) {
	e := newTestEnv(t, nil)
	follow := `{"type":"follow","replyToken":"rt-follow","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false}}`
	rec := e.webhook("/webhook", eventsJSON(
		textEventJSON("rt-cancel", "いいえ"),
		follow,
		textEventJSON("", "日本"),
	))
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	if len(e.doer.calls) != 0 {
		t.Errorf("calls = %q, want none", e.doer.calls)
	}
}

func TestWebhookLoadingFailureStillReplies(t *testing.T) {
	for name, tt := range map[string]struct {
		resp *http.Response
		err  error
		log  string
	}{
		"api error": {response(400, `{"message":"body-only-marker"}`), nil, "loading=failed loading_status=400 loading_error=loading_api_error"},
		"transport": {nil, errors.New("transport-only-marker"), "loading=failed loading_status=0 loading_error=transport_error"},
	} {
		e := newTestEnv(t, nil)
		e.doer.respondLoading = func(int) (*http.Response, error) { return tt.resp, tt.err }
		rec := e.webhook("/webhook", eventsJSON(textEventJSON("rt-1", "日本")))
		if rec.Code != 200 {
			t.Errorf("%s: status = %d", name, rec.Code)
		}
		// No wait after a failed loading animation.
		want := []string{"loading", "reply rt-1"}
		if !slices.Equal(e.doer.calls, want) {
			t.Errorf("%s: calls = %q, want %q", name, e.doer.calls, want)
		}
		logs := e.logs.String()
		if !strings.Contains(logs, tt.log) || !strings.Contains(logs, "replied=true") {
			t.Errorf("%s: logs miss %q:\n%s", name, tt.log, logs)
		}
		for _, s := range []string{"body-only-marker", "transport-only-marker"} {
			if strings.Contains(logs, s) {
				t.Errorf("%s: logs contain %q:\n%s", name, s, logs)
			}
		}
	}
}

func TestWebhookWaitsOncePerWebhook(t *testing.T) {
	e := newTestEnv(t, nil)
	e.webhook("/webhook", eventsJSON(
		textEventJSON("rt-1", "日本"),
		sourcedTextEventJSON("rt-group", "日本", map[string]string{"type": "group", "groupId": "Cgroup"}),
		textEventJSON("rt-2", "クイズ"),
	))
	want := []string{"loading", "sleep 1s", "reply rt-1", "reply rt-group", "loading", "reply rt-2"}
	if !slices.Equal(e.doer.calls, want) {
		t.Errorf("calls = %q, want %q", e.doer.calls, want)
	}
}

func TestWebhookNoWaitWhenFirstReplySkipsIt(t *testing.T) {
	// The wait goes only before the first replied event. When that event does
	// not wait (group source, or the loading API failed), later one-on-one
	// replies do not wait either, but still show the animation.
	failFirst := func(n int) (*http.Response, error) {
		if n == 0 {
			return response(500, `{}`), nil
		}
		return nil, nil
	}
	for name, tt := range map[string]struct {
		respondLoading func(int) (*http.Response, error)
		first          string
		want           []string
	}{
		"group": {
			nil,
			sourcedTextEventJSON("rt-0", "日本", map[string]string{"type": "group", "groupId": "Cgroup"}),
			[]string{"reply rt-0", "loading", "reply rt-1", "loading", "reply rt-2"},
		},
		"loading failed": {
			failFirst,
			textEventJSON("rt-0", "日本"),
			[]string{"loading", "reply rt-0", "loading", "reply rt-1", "loading", "reply rt-2"},
		},
	} {
		e := newTestEnv(t, nil)
		e.doer.respondLoading = tt.respondLoading
		unfollow := `{"type":"unfollow","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false}}`
		e.webhook("/webhook", eventsJSON(
			unfollow,
			tt.first,
			textEventJSON("rt-1", "日本"),
			textEventJSON("rt-2", "クイズ"),
		))
		if !slices.Equal(e.doer.calls, tt.want) {
			t.Errorf("%s: calls = %q, want %q", name, e.doer.calls, tt.want)
		}
	}
}

func TestWebhookReplyDelayZeroKeepsImmediateReplies(t *testing.T) {
	e := newTestEnv(t, map[string]string{EnvReplyDelayMS: "0"})
	follow := `{"type":"follow","replyToken":"rt-follow","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false}}`
	image := `{"type":"message","replyToken":"rt-image","source":{"type":"user","userId":"U1"},"deliveryContext":{"isRedelivery":false},"message":{"id":"2","type":"image"}}`
	rec := e.webhook("/webhook", eventsJSON(textEventJSON("rt-text", "日本"), follow, image))
	if rec.Code != 200 {
		t.Errorf("status = %d", rec.Code)
	}
	want := []string{"reply rt-text", "reply rt-image"}
	if !slices.Equal(e.doer.calls, want) {
		t.Errorf("calls = %q, want %q", e.doer.calls, want)
	}
	if strings.Contains(e.logs.String(), "config warning") {
		t.Errorf("unexpected warning:\n%s", e.logs)
	}
}

func TestWebhookReplyDelayWarnings(t *testing.T) {
	for _, tt := range []struct{ value, wait string }{
		{"abc", "sleep 1s"},
		{"-5", "sleep 1s"},
		{"5000", "sleep 1.2s"},
	} {
		e := newTestEnv(t, map[string]string{EnvReplyDelayMS: tt.value})
		e.webhook("/webhook", eventsJSON(textEventJSON("rt-1", "日本")))
		want := []string{"loading", tt.wait, "reply rt-1"}
		if !slices.Equal(e.doer.calls, want) {
			t.Errorf("%s=%q: calls = %q, want %q", EnvReplyDelayMS, tt.value, e.doer.calls, want)
		}
		if !strings.Contains(e.logs.String(), "config warning: "+EnvReplyDelayMS) {
			t.Errorf("%s=%q: no config warning:\n%s", EnvReplyDelayMS, tt.value, e.logs)
		}
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
