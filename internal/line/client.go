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
	maxErrorBodySize = 512
)

// Doer sends an HTTP request. *http.Client satisfies it; the Workers entry
// point supplies an implementation backed by the runtime's fetch.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// ErrSend is wrapped by the error Reply returns when the request could not be
// sent or no response arrived.
var ErrSend = errors.New("line: send reply")

// APIError is returned when the Messaging API answers with a non-2xx status.
// Body holds at most the first 512 bytes of the response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("line: reply API returned status %d: %s", e.StatusCode, e.Body)
}

// Client calls the reply API.
type Client struct {
	baseURL     string
	accessToken string
	doer        Doer
}

// NewClient returns a reply API client. An empty baseURL selects
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

	body, err := json.Marshal(replyRequest{ReplyToken: replyToken, Messages: messages})
	if err != nil {
		return fmt.Errorf("line: encode reply: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+replyPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("line: build reply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.doer.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		head, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		return &APIError{StatusCode: resp.StatusCode, Body: string(head)}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
