package teams

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/graph"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text passthrough",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "simple paragraph tags",
			input:    "<p>Hello</p><p>World</p>",
			expected: "HelloWorld",
		},
		{
			name:     "br tags become newlines",
			input:    "Line one<br>Line two<br/>Line three",
			expected: "Line one\nLine two\nLine three",
		},
		{
			name:     "HTML entities decoded",
			input:    "A &amp; B &lt; C &gt; D &quot;E&quot;",
			expected: `A & B < C > D "E"`,
		},
		{
			name:     "nested tags stripped",
			input:    "<div><span style='color:red'>Bold <b>text</b></span></div>",
			expected: "Bold text",
		},
		{
			name:     "whitespace collapsed",
			input:    "  lots   of   spaces  ",
			expected: "lots of spaces",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "real Teams HTML message",
			input:    `<div><div itemprop="copy-paste-block"><p>Hey team, just wanted to flag that the deployment is blocked.</p></div></div>`,
			expected: "Hey team, just wanted to flag that the deployment is blocked.",
		},
		{
			name:     "no raw p tags in output",
			input:    "<p>First paragraph</p><p>Second paragraph</p>",
			expected: "First paragraphSecond paragraph",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.expected {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseThread(t *testing.T) {
	baseTime := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)

	t.Run("single message thread", func(t *testing.T) {
		thread := graph.TeamsThread{
			Root: graph.TeamsMessage{
				ID:              "msg-1",
				TeamID:          "team-1",
				ChannelID:       "channel-1",
				Subject:         "Project Update",
				BodyContentType: "text",
				BodyContent:     "The project is on track.",
				SenderName:      "Alice",
				CreatedAt:       baseTime,
			},
		}

		result := ParseThread(thread)

		if result == "" {
			t.Fatal("ParseThread returned empty string")
		}
		if !contains(result, "Subject: Project Update") {
			t.Error("missing subject line")
		}
		if !contains(result, "Alice (2026-03-04 10:00)") {
			t.Error("missing sender/timestamp header")
		}
		if !contains(result, "The project is on track.") {
			t.Error("missing body content")
		}
	})

	t.Run("thread with replies", func(t *testing.T) {
		thread := graph.TeamsThread{
			Root: graph.TeamsMessage{
				ID:              "msg-1",
				BodyContentType: "text",
				BodyContent:     "Root message",
				SenderName:      "Alice",
				CreatedAt:       baseTime,
			},
			Replies: []graph.TeamsMessage{
				{
					ID:              "msg-2",
					BodyContentType: "text",
					BodyContent:     "First reply",
					SenderName:      "Bob",
					CreatedAt:       baseTime.Add(5 * time.Minute),
					ReplyToID:       "msg-1",
				},
				{
					ID:              "msg-3",
					BodyContentType: "text",
					BodyContent:     "Second reply",
					SenderName:      "Charlie",
					CreatedAt:       baseTime.Add(10 * time.Minute),
					ReplyToID:       "msg-1",
				},
			},
		}

		result := ParseThread(thread)

		if !contains(result, "Root message") {
			t.Error("missing root message body")
		}
		if !contains(result, "--- Reply ---") {
			t.Error("missing reply separator")
		}
		if !contains(result, "Bob") {
			t.Error("missing reply sender")
		}
		if !contains(result, "First reply") {
			t.Error("missing first reply body")
		}
		if !contains(result, "Second reply") {
			t.Error("missing second reply body")
		}
	})

	t.Run("HTML body stripped to plain text", func(t *testing.T) {
		thread := graph.TeamsThread{
			Root: graph.TeamsMessage{
				ID:              "msg-1",
				BodyContentType: "html",
				BodyContent:     "<p>Hello <b>world</b></p>",
				SenderName:      "Alice",
				CreatedAt:       baseTime,
			},
		}

		result := ParseThread(thread)

		if contains(result, "<p>") || contains(result, "<b>") {
			t.Errorf("HTML tags not stripped: %q", result)
		}
		if !contains(result, "Hello world") {
			t.Error("plain text content missing after HTML strip")
		}
	})

	t.Run("no subject line when empty", func(t *testing.T) {
		thread := graph.TeamsThread{
			Root: graph.TeamsMessage{
				ID:              "msg-1",
				BodyContentType: "text",
				BodyContent:     "Message without subject",
				SenderName:      "Alice",
				CreatedAt:       baseTime,
			},
		}

		result := ParseThread(thread)

		if contains(result, "Subject:") {
			t.Error("unexpected Subject: line for empty subject")
		}
	})
}

func TestThreadMetadata(t *testing.T) {
	baseTime := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)

	thread := graph.TeamsThread{
		Root: graph.TeamsMessage{
			ID:         "msg-1",
			TeamID:     "team-1",
			ChannelID:  "channel-1",
			Subject:    "Test Thread",
			SenderName: "Alice",
			CreatedAt:  baseTime,
			WebURL:     "https://teams.microsoft.com/messages/123",
		},
		Replies: []graph.TeamsMessage{
			{
				ID:         "msg-2",
				SenderName: "Bob",
				CreatedAt:  baseTime.Add(5 * time.Minute),
			},
			{
				ID:         "msg-3",
				SenderName: "Alice",
				CreatedAt:  baseTime.Add(10 * time.Minute),
			},
		},
	}

	meta := ThreadMetadata(thread, "General")

	if meta["team_id"] != "team-1" {
		t.Errorf("team_id = %v, want team-1", meta["team_id"])
	}
	if meta["channel_id"] != "channel-1" {
		t.Errorf("channel_id = %v, want channel-1", meta["channel_id"])
	}
	if meta["channel_name"] != "General" {
		t.Errorf("channel_name = %v, want General", meta["channel_name"])
	}
	if meta["reply_count"] != 2 {
		t.Errorf("reply_count = %v, want 2", meta["reply_count"])
	}
	if meta["source"] != "teams_channel" {
		t.Errorf("source = %v, want teams_channel", meta["source"])
	}
	if meta["subject"] != "Test Thread" {
		t.Errorf("subject = %v, want Test Thread", meta["subject"])
	}
	if meta["web_url"] != "https://teams.microsoft.com/messages/123" {
		t.Errorf("web_url = %v, want https://teams.microsoft.com/messages/123", meta["web_url"])
	}

	participants := meta["participants"].([]string)
	if len(participants) != 2 {
		t.Errorf("expected 2 unique participants, got %d", len(participants))
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
