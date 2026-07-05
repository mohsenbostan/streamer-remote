package kick

import (
	"log/slog"
	"os"
	"testing"
)

// realChatMessagePayload is a trimmed but field-accurate example of the
// data Kick's Pusher feed sends inside an App\Events\ChatMessageEvent
// envelope, matching what production Kick chat actually emits.
const realChatMessagePayload = `{
	"id": "abc123",
	"chatroom_id": 668,
	"content": "rc!w+shift",
	"type": "message",
	"created_at": "2026-07-05T12:00:00.000000Z",
	"sender": {
		"id": 42,
		"username": "SomeViewer",
		"slug": "someviewer",
		"identity": {
			"color": "#FF0000",
			"badges": [
				{"type": "moderator", "text": "Moderator"},
				{"type": "subscriber", "text": "Subscriber", "count": 3}
			]
		}
	}
}`

func TestHandleChatMessage(t *testing.T) {
	client := &Client{Channel: "test", Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	out := make(chan ChatEvent, 1)

	client.handleChatMessage(realChatMessagePayload, out)

	select {
	case e := <-out:
		if e.Username != "SomeViewer" {
			t.Errorf("expected username %q, got %q", "SomeViewer", e.Username)
		}
		if e.Text != "rc!w+shift" {
			t.Errorf("expected text %q, got %q", "rc!w+shift", e.Text)
		}
		if !e.Badges["moderator"] || !e.Badges["subscriber"] {
			t.Errorf("expected moderator and subscriber badges, got %v", e.Badges)
		}
		if e.Badges["vip"] {
			t.Errorf("did not expect vip badge, got %v", e.Badges)
		}
	default:
		t.Fatal("expected a chat event to be emitted")
	}
}

func TestHandleChatMessageIgnoresMalformedPayload(t *testing.T) {
	client := &Client{Channel: "test", Logger: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	out := make(chan ChatEvent, 1)

	client.handleChatMessage("not json", out)

	select {
	case e := <-out:
		t.Fatalf("expected no event for malformed payload, got %+v", e)
	default:
	}
}
