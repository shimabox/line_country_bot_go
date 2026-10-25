package line

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultAPIBaseURL is the Messaging API endpoint used when no base URL is set.
const DefaultAPIBaseURL = "https://api.line.me"

const (
	replyPath        = "/v2/bot/message/reply"
	loadingPath      = "/v2/bot/chat/loading/start"
	maxErrorBodySize = 512
)

// Names of the APIs, used in errors.
const (
	apiReply   = "reply"
	apiLoading = "loading"
)

// Bounds of the loading animation's duration. The API accepts multiples of 5
// between them.
const (
	MinLoadingSeconds = 5
	MaxLoadingSeconds = 60
)

// Doer sends an HTTP request. *http.Client satisfies it; the Workers entry
// point supplies an implementation backed by the runtime's fetch.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// ErrSend is wrapped by the error Reply and ShowLoading return when the
// request could not be sent or no response arrived.
var ErrSend = errors.New("line: send request")

// APIError is returned when the Messaging API answers with a non-2xx status.
// API names the API that answered ("reply" or "loading"). Body holds at most
// the first 512 bytes of the response.
type APIError struct {
	API        string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("line: %s API returned status %d: %s", e.API, e.StatusCode, e.Body)
}

// Client calls the reply and loading animation APIs.
type Client struct {
	baseURL     string
	accessToken string
	doer        Doer
}

// NewClient returns a Messaging API client. An empty baseURL selects
// DefaultAPIBaseURL.
func NewClient(baseURL, accessToken string, doer Doer) *Client {
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		accessToken: accessToken,
		doer:        doer,
	}
}

type replyRequest struct {
	ReplyToken string    `json:"replyToken"`
	Messages   []Message `json:"messages"`
}

// Reply sends between one and five messages using a reply token.
func (c *Client) Reply(ctx context.Context, replyToken string, messages []Message) error {
	switch {
	case replyToken == "":
		return errors.New("line: reply token is empty")
	case c.accessToken == "":
		return errors.New("line: channel access token is empty")
	case len(messages) == 0:
		return errors.New("line: no messages to reply")
	case len(messages) > MaxReplyMessages:
		return fmt.Errorf("line: %d messages exceed the limit of %d", len(messages), MaxReplyMessages)
	}

	return c.post(ctx, apiReply, replyPath, replyRequest{ReplyToken: replyToken, Messages: messages})
}

type loadingRequest struct {
	ChatID         string `json:"chatId"`
	LoadingSeconds int    `json:"loadingSeconds"`
}

// ShowLoading shows the loading animation in a one-on-one chat for the given
// number of seconds, or until the bot's next message arrives. seconds must be
// a multiple of 5 between MinLoadingSeconds and MaxLoadingSeconds.
func (c *Client) ShowLoading(ctx context.Context, chatID string, seconds int) error {
	switch {
	case chatID == "":
		return errors.New("line: chat ID is empty")
	case c.accessToken == "":
		return errors.New("line: channel access token is empty")
	case seconds < MinLoadingSeconds || seconds > MaxLoadingSeconds || seconds%5 != 0:
		return fmt.Errorf("line: loading seconds %d is not a multiple of 5 between %d and %d", seconds, MinLoadingSeconds, MaxLoadingSeconds)
	}
	return c.post(ctx, apiLoading, loadingPath, loadingRequest{ChatID: chatID, LoadingSeconds: seconds})
}

// post sends payload as JSON to path. api names the API in errors.
func (c *Client) post(ctx context.Context, api, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("line: encode %s request: %w", api, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("line: build %s request: %w", api, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.doer.Do(req)
	if err != nil {
		return fmt.Errorf("%w to the %s API: %w", ErrSend, api, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		head, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return &APIError{API: api, StatusCode: resp.StatusCode, Body: string(head)}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
