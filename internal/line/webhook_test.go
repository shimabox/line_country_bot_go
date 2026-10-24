package line

import (
	"os"
	"path/filepath"
	"testing"
)

func loadWebhook(t *testing.T, name string) *WebhookRequest {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "webhook", name))
	if err != nil {
		t.Fatal(err)
	}
	req, err := ParseWebhook(body)
	if err != nil {
		t.Fatalf("ParseWebhook(%s): %v", name, err)
	}
	return req
}

func TestParseWebhookEmptyEvents(t *testing.T) {
	req := loadWebhook(t, "empty.json")
	if req.Destination != "Udestination0000000000000000000000" {
		t.Errorf("Destination = %q", req.Destination)
	}
	if req.Events == nil || len(req.Events) != 0 {
		t.Errorf("Events = %#v, want empty non-nil slice", req.Events)
	}
}

func TestParseWebhookText(t *testing.T) {
	req := loadWebhook(t, "text.json")
	if len(req.Events) != 1 {
		t.Fatalf("len(Events) = %d", len(req.Events))
	}
	ev := req.Events[0]
	if ev.Type != EventTypeMessage {
		t.Errorf("Type = %q", ev.Type)
	}
	if ev.ReplyToken != "reply-token-text" {
		t.Errorf("ReplyToken = %q", ev.ReplyToken)
	}
	if ev.Timestamp != 1700000000000 {
		t.Errorf("Timestamp = %d", ev.Timestamp)
	}
	if ev.Source.Type != "user" || ev.Source.UserID != "Uuser00000000000000000000000000000" {
		t.Errorf("Source = %#v", ev.Source)
	}
	if ev.DeliveryContext.IsRedelivery {
		t.Error("IsRedelivery = true")
	}
	if ev.Message == nil {
		t.Fatal("Message is nil")
	}
	if ev.Message.ID != "100001" || ev.Message.Type != MessageTypeText || ev.Message.Text != "日本" {
		t.Errorf("Message = %#v", ev.Message)
	}
	if ev.Postback != nil {
		t.Errorf("Postback = %#v, want nil", ev.Postback)
	}
}

func TestParseWebhookImage(t *testing.T) {
	ev := loadWebhook(t, "image.json").Events[0]
	if ev.Type != EventTypeMessage || ev.Message == nil || ev.Message.Type != "image" {
		t.Fatalf("event = %#v", ev)
	}
	if ev.Message.Text != "" {
		t.Errorf("Text = %q", ev.Message.Text)
	}
}

func TestParseWebhookPostback(t *testing.T) {
	ev := loadWebhook(t, "postback.json").Events[0]
	if ev.Type != EventTypePostback {
		t.Errorf("Type = %q", ev.Type)
	}
	if ev.Postback == nil || ev.Postback.Data != "answer=%E6%97%A5%E6%9C%AC&result=1" {
		t.Errorf("Postback = %#v", ev.Postback)
	}
	if ev.Source.Type != "group" || ev.Source.GroupID != "Cgroup0000000000000000000000000000" {
		t.Errorf("Source = %#v", ev.Source)
	}
	if ev.Message != nil {
		t.Errorf("Message = %#v, want nil", ev.Message)
	}
}

func TestParseWebhookFollow(t *testing.T) {
	ev := loadWebhook(t, "follow.json").Events[0]
	if ev.Type != "follow" || ev.ReplyToken != "reply-token-follow" {
		t.Errorf("event = %#v", ev)
	}
	if ev.Message != nil || ev.Postback != nil {
		t.Errorf("unexpected payload: %#v", ev)
	}
}

func TestParseWebhookUnknownTypesAreKept(t *testing.T) {
	req := loadWebhook(t, "unknown.json")
	if len(req.Events) != 2 {
		t.Fatalf("len(Events) = %d", len(req.Events))
	}
	if req.Events[0].Type != "somethingNew" {
		t.Errorf("Events[0].Type = %q", req.Events[0].Type)
	}
	if m := req.Events[1].Message; m == nil || m.Type != "hologram" {
		t.Errorf("Events[1].Message = %#v", m)
	}
}

func TestParseWebhookRedelivery(t *testing.T) {
	ev := loadWebhook(t, "redelivery.json").Events[0]
	if !ev.DeliveryContext.IsRedelivery {
		t.Error("IsRedelivery = false, want true")
	}
	if ev.ReplyToken != "reply-token-redelivery" || ev.Message.Text != "quiz" {
		t.Errorf("event = %#v", ev)
	}
}

func TestParseWebhookInvalid(t *testing.T) {
	for _, body := range []string{``, `{`, `[]`, `{"events":{}}`, `null`} {
		if _, err := ParseWebhook([]byte(body)); err == nil {
			t.Errorf("ParseWebhook(%q) error = nil, want error", body)
		}
	}
}
