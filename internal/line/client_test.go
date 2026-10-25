package line

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeDoer struct {
	req    *http.Request
	body   []byte
	status int
	resp   string
	err    error
	calls  int
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	f.req = req
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		f.body = b
	}
	if f.err != nil {
		return nil, f.err
	}
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(f.resp)),
		Header:     make(http.Header),
	}, nil
}

func TestReplySendsRequest(t *testing.T) {
	doer := &fakeDoer{resp: `{}`}
	c := NewClient("", "test-access-token", doer)
	msgs := []Message{
		TextMessage{Text: "[日本] ですね🤔"},
		StickerMessage{PackageID: "446", StickerID: "1999"},
	}
	if err := c.Reply(context.Background(), "reply-token", msgs); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if doer.req.Method != http.MethodPost {
		t.Errorf("Method = %s", doer.req.Method)
	}
	if got := doer.req.URL.String(); got != "https://api.line.me/v2/bot/message/reply" {
		t.Errorf("URL = %s", got)
	}
	if got := doer.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := doer.req.Header.Get("Authorization"); got != "Bearer test-access-token" {
		t.Errorf("Authorization = %q", got)
	}
	var body struct {
		ReplyToken string           `json:"replyToken"`
		Messages   []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(doer.body, &body); err != nil {
		t.Fatalf("body %s: %v", doer.body, err)
	}
	if body.ReplyToken != "reply-token" {
		t.Errorf("replyToken = %q", body.ReplyToken)
	}
	if len(body.Messages) != 2 || body.Messages[0]["type"] != "text" || body.Messages[1]["stickerId"] != "1999" {
		t.Errorf("messages = %v", body.Messages)
	}
}

func TestReplyBaseURLOverride(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:9999", "http://127.0.0.1:9999/"} {
		doer := &fakeDoer{}
		c := NewClient(base, "token", doer)
		if err := c.Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}}); err != nil {
			t.Fatalf("Reply: %v", err)
		}
		if got := doer.req.URL.String(); got != "http://127.0.0.1:9999/v2/bot/message/reply" {
			t.Errorf("base %q: URL = %s", base, got)
		}
	}
}

func TestReplySuccessStatuses(t *testing.T) {
	for _, status := range []int{200, 201, 204, 299} {
		doer := &fakeDoer{status: status}
		if err := NewClient("", "t", doer).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}}); err != nil {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
}

func TestReplyErrorStatuses(t *testing.T) {
	long := strings.Repeat("x", 2000)
	tests := []struct {
		status int
		resp   string
	}{
		{400, `{"message":"Invalid reply token"}`},
		{401, `{"message":"Authentication failed"}`},
		{429, `{"message":"rate limit"}`},
		{500, long},
	}
	for _, tt := range tests {
		doer := &fakeDoer{status: tt.status, resp: tt.resp}
		err := NewClient("", "t", doer).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}})
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("status %d: err = %v, want *APIError", tt.status, err)
		}
		if apiErr.StatusCode != tt.status {
			t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
		}
		if len(apiErr.Body) > 512 {
			t.Errorf("Body length = %d, want <= 512", len(apiErr.Body))
		}
		if !strings.HasPrefix(tt.resp, apiErr.Body) || apiErr.Body == "" {
			t.Errorf("Body = %q", apiErr.Body)
		}
		if !strings.Contains(err.Error(), "400") && tt.status == 400 {
			t.Errorf("Error() = %q, want status", err.Error())
		}
	}
}

func TestReplyTransportError(t *testing.T) {
	doer := &fakeDoer{err: errors.New("connection refused")}
	err := NewClient("", "t", doer).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("err = %v", err)
	}
	if !errors.Is(err, ErrSend) {
		t.Errorf("err = %v, want ErrSend", err)
	}
	if err := NewClient("", "", doer).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}}); errors.Is(err, ErrSend) {
		t.Errorf("err = %v; an unsent request must not be ErrSend", err)
	}
}

func TestReplyMessageCount(t *testing.T) {
	five := make([]Message, MaxReplyMessages)
	for i := range five {
		five[i] = TextMessage{Text: "x"}
	}
	doer := &fakeDoer{}
	if err := NewClient("", "t", doer).Reply(context.Background(), "rt", five); err != nil {
		t.Errorf("5 messages: err = %v", err)
	}

	six := append(five, TextMessage{Text: "y"})
	doer = &fakeDoer{}
	if err := NewClient("", "t", doer).Reply(context.Background(), "rt", six); err == nil {
		t.Error("6 messages: err = nil, want error")
	}
	if doer.calls != 0 {
		t.Errorf("6 messages: calls = %d, want 0", doer.calls)
	}

	doer = &fakeDoer{}
	if err := NewClient("", "t", doer).Reply(context.Background(), "rt", nil); err == nil {
		t.Error("0 messages: err = nil, want error")
	}
	if doer.calls != 0 {
		t.Errorf("0 messages: calls = %d, want 0", doer.calls)
	}
}

func TestReplyRequiresToken(t *testing.T) {
	doer := &fakeDoer{}
	if err := NewClient("", "t", doer).Reply(context.Background(), "", []Message{TextMessage{Text: "x"}}); err == nil {
		t.Error("empty reply token: err = nil")
	}
	if err := NewClient("", "", doer).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}}); err == nil {
		t.Error("empty access token: err = nil")
	}
	if doer.calls != 0 {
		t.Errorf("calls = %d, want 0", doer.calls)
	}
}

func TestReplyErrorNamesTheAPI(t *testing.T) {
	err := NewClient("", "t", &fakeDoer{status: 400}).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "reply") {
		t.Errorf("API error = %v, want it to name the reply API", err)
	}
	err = NewClient("", "t", &fakeDoer{err: errors.New("down")}).Reply(context.Background(), "rt", []Message{TextMessage{Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "reply") {
		t.Errorf("send error = %v, want it to name the reply API", err)
	}
}

func TestShowLoadingSendsRequest(t *testing.T) {
	doer := &fakeDoer{status: http.StatusAccepted, resp: `{}`}
	c := NewClient("", "test-access-token", doer)
	if err := c.ShowLoading(context.Background(), "U0123456789abcdef", MinLoadingSeconds); err != nil {
		t.Fatalf("ShowLoading: %v", err)
	}
	if doer.req.Method != http.MethodPost {
		t.Errorf("Method = %s", doer.req.Method)
	}
	if got := doer.req.URL.String(); got != "https://api.line.me/v2/bot/chat/loading/start" {
		t.Errorf("URL = %s", got)
	}
	if got := doer.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := doer.req.Header.Get("Authorization"); got != "Bearer test-access-token" {
		t.Errorf("Authorization = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(doer.body, &body); err != nil {
		t.Fatalf("body %s: %v", doer.body, err)
	}
	if len(body) != 2 || body["chatId"] != "U0123456789abcdef" || body["loadingSeconds"] != float64(5) {
		t.Errorf("body = %s", doer.body)
	}
}

func TestShowLoadingBaseURLOverride(t *testing.T) {
	doer := &fakeDoer{}
	if err := NewClient("http://127.0.0.1:9999/", "t", doer).ShowLoading(context.Background(), "U1", 5); err != nil {
		t.Fatalf("ShowLoading: %v", err)
	}
	if got := doer.req.URL.String(); got != "http://127.0.0.1:9999/v2/bot/chat/loading/start" {
		t.Errorf("URL = %s", got)
	}
}

func TestShowLoadingSeconds(t *testing.T) {
	for _, s := range []int{5, 10, 55, 60} {
		doer := &fakeDoer{}
		if err := NewClient("", "t", doer).ShowLoading(context.Background(), "U1", s); err != nil {
			t.Errorf("%d seconds: err = %v", s, err)
		}
	}
	for _, s := range []int{-5, 0, 1, 4, 6, 59, 65, 120} {
		doer := &fakeDoer{}
		if err := NewClient("", "t", doer).ShowLoading(context.Background(), "U1", s); err == nil {
			t.Errorf("%d seconds: err = nil, want error", s)
		}
		if doer.calls != 0 {
			t.Errorf("%d seconds: calls = %d, want 0", s, doer.calls)
		}
	}
}

func TestShowLoadingRequiresChatAndToken(t *testing.T) {
	doer := &fakeDoer{}
	if err := NewClient("", "t", doer).ShowLoading(context.Background(), "", 5); err == nil {
		t.Error("empty chat ID: err = nil")
	}
	if err := NewClient("", "", doer).ShowLoading(context.Background(), "U1", 5); err == nil || errors.Is(err, ErrSend) {
		t.Errorf("empty access token: err = %v, want an error that is not ErrSend", err)
	}
	if doer.calls != 0 {
		t.Errorf("calls = %d, want 0", doer.calls)
	}
}

func TestShowLoadingErrorStatus(t *testing.T) {
	doer := &fakeDoer{status: 400, resp: `{"message":"The property, 'chatId', in the request body is invalid"}`}
	err := NewClient("", "t", doer).ShowLoading(context.Background(), "C-group", 5)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != 400 || !strings.HasPrefix(doer.resp, apiErr.Body) || apiErr.Body == "" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if !strings.Contains(err.Error(), "loading") || !strings.Contains(err.Error(), "400") {
		t.Errorf("Error() = %q, want the API and the status", err.Error())
	}
}

func TestShowLoadingTransportError(t *testing.T) {
	doer := &fakeDoer{err: errors.New("connection refused")}
	err := NewClient("", "t", doer).ShowLoading(context.Background(), "U1", 5)
	if !errors.Is(err, ErrSend) {
		t.Errorf("err = %v, want ErrSend", err)
	}
	if err == nil || !strings.Contains(err.Error(), "connection refused") || !strings.Contains(err.Error(), "loading") {
		t.Errorf("err = %v", err)
	}
}
