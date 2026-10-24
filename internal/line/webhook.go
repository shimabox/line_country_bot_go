package line

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Event types handled by the bot. Other types are parsed and kept as is.
const (
	EventTypeMessage  = "message"
	EventTypePostback = "postback"
)

// MessageTypeText is the message type of a text message.
const MessageTypeText = "text"

// WebhookRequest is the body of a webhook request.
type WebhookRequest struct {
	Destination string  `json:"destination"`
	Events      []Event `json:"events"`
}

// Event is a single webhook event. Unknown event and message types are not
// errors; callers decide what to ignore.
type Event struct {
	Type            string          `json:"type"`
	ReplyToken      string          `json:"replyToken"`
	Timestamp       int64           `json:"timestamp"`
	Source          Source          `json:"source"`
	DeliveryContext DeliveryContext `json:"deliveryContext"`
	Message         *EventMessage   `json:"message,omitempty"`
	Postback        *Postback       `json:"postback,omitempty"`
}

// Source identifies where an event came from.
type Source struct {
	Type    string `json:"type"`
	UserID  string `json:"userId,omitempty"`
	GroupID string `json:"groupId,omitempty"`
	RoomID  string `json:"roomId,omitempty"`
}

// DeliveryContext tells whether the event is a redelivery.
type DeliveryContext struct {
	IsRedelivery bool `json:"isRedelivery"`
}

// EventMessage is the message carried by a message event.
type EventMessage struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Postback is the payload of a postback event.
type Postback struct {
	Data string `json:"data"`
}

var errNoEvents = errors.New("line: webhook body has no events field")

// ParseWebhook decodes a webhook request body. An empty events array (sent by
// the console's verify button) is valid; a body without an events array is not.
func ParseWebhook(body []byte) (*WebhookRequest, error) {
	var req WebhookRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("line: parse webhook: %w", err)
	}
	if req.Events == nil {
		return nil, errNoEvents
	}
	return &req, nil
}
