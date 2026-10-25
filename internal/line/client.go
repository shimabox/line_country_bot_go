package line

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// Initial capacities of the request body buffers. A reply body is a few
// hundred bytes to about 1KB (a quiz with four choices), so it is usually
// written without growing the buffer.
const (
	replyBodySize   = 2048
	loadingBodySize = 64
)

// appendReplyBody appends the reply API request body,
// {"replyToken":...,"messages":[...]}.
func appendReplyBody(dst []byte, replyToken string, messages []Message) []byte {
	dst = append(dst, `{"replyToken":`...)
	dst = appendJSONString(dst, replyToken)
	dst = append(dst, `,"messages":`...)
	dst = appendArray(dst, messages, appendMessage)
	return append(dst, '}')
}

// appendLoadingBody appends the loading animation API request body,
// {"chatId":...,"loadingSeconds":N}.
func appendLoadingBody(dst []byte, chatID string, seconds int) []byte {
	dst = append(dst, `{"chatId":`...)
	dst = appendJSONString(dst, chatID)
	dst = append(dst, `,"loadingSeconds":`...)
	dst = strconv.AppendInt(dst, int64(seconds), 10)
	return append(dst, '}')
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

	return c.post(ctx, apiReply, replyPath, appendReplyBody(make([]byte, 0, replyBodySize), replyToken, messages))
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
	return c.post(ctx, apiLoading, loadingPath, appendLoadingBody(make([]byte, 0, loadingBodySize), chatID, seconds))
}

// post sends the JSON body to path. api names the API in errors.
func (c *Client) post(ctx context.Context, api, path string, body []byte) error {
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
