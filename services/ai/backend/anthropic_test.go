package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestAnthropicBackend creates an AnthropicBackend pointed at a test server.
func newTestAnthropicBackend(t *testing.T, serverURL string) *AnthropicBackend {
	t.Helper()
	be, err := NewAnthropicBackend(&AnthropicConfig{
		APIKey:   "test-key",
		Endpoint: serverURL,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewAnthropicBackend: %v", err)
	}
	return be
}

// anthropicTestResponse builds a minimal anthropicResponse JSON body.
func anthropicTestResponse(content []anthropicContent, stopReason string) []byte {
	resp := anthropicResponse{
		ID:         "msg_test",
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      "claude-haiku-4-5-20251001",
		StopReason: stopReason,
		Usage:      anthropicUsage{InputTokens: 10, OutputTokens: 20},
	}
	b, _ := json.Marshal(resp)
	return b
}

// serveAnthropicResponse returns an httptest.Server that always responds with
// the provided JSON body.
func serveAnthropicResponse(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

// =============================================================================
// Content extraction — tool_use always wins regardless of block order
// =============================================================================

func TestAnthropicBackend_ContentExtraction_TextThenToolUse(t *testing.T) {
	// Blocks: [text, tool_use] — tool_use must win even though it comes last.
	toolJSON := json.RawMessage(`{"key":"from_tool"}`)
	body := anthropicTestResponse([]anthropicContent{
		{Type: "text", Text: "some thinking text"},
		{Type: "tool_use", ID: "tu_1", Name: "respond", Input: toolJSON},
	}, "tool_use")

	srv := serveAnthropicResponse(t, body)
	defer srv.Close()

	be := newTestAnthropicBackend(t, srv.URL)
	result, err := be.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "hello"},
	}, CompletionOptions{Model: "claude-haiku-4-5-20251001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != `{"key":"from_tool"}` {
		t.Errorf("expected tool_use content, got %q", result.Content)
	}
}

func TestAnthropicBackend_ContentExtraction_ToolUseThenText(t *testing.T) {
	// Blocks: [tool_use, text] — tool_use must still win.
	toolJSON := json.RawMessage(`{"key":"from_tool"}`)
	body := anthropicTestResponse([]anthropicContent{
		{Type: "tool_use", ID: "tu_1", Name: "respond", Input: toolJSON},
		{Type: "text", Text: "trailing text"},
	}, "tool_use")

	srv := serveAnthropicResponse(t, body)
	defer srv.Close()

	be := newTestAnthropicBackend(t, srv.URL)
	result, err := be.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "hello"},
	}, CompletionOptions{Model: "claude-haiku-4-5-20251001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != `{"key":"from_tool"}` {
		t.Errorf("expected tool_use content, got %q", result.Content)
	}
}

func TestAnthropicBackend_ContentExtraction_TextOnly(t *testing.T) {
	// No tool_use block — text content is returned.
	body := anthropicTestResponse([]anthropicContent{
		{Type: "text", Text: "plain text response"},
	}, "end_turn")

	srv := serveAnthropicResponse(t, body)
	defer srv.Close()

	be := newTestAnthropicBackend(t, srv.URL)
	result, err := be.ChatCompletion(context.Background(), []Message{
		{Role: "user", Content: "hello"},
	}, CompletionOptions{Model: "claude-haiku-4-5-20251001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "plain text response" {
		t.Errorf("expected text content, got %q", result.Content)
	}
}

// =============================================================================
// stop_reason normalisation
// =============================================================================

func TestAnthropicBackend_StopReason_Normalisation(t *testing.T) {
	tests := []struct {
		stopReason string
		want       string
	}{
		{"end_turn", "stop"},
		{"tool_use", "stop"},
		{"stop_sequence", "stop"},
		{"max_tokens", "length"},
		{"unknown_reason", "unknown_reason"}, // pass-through for unrecognised values
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.stopReason, func(t *testing.T) {
			body := anthropicTestResponse([]anthropicContent{
				{Type: "text", Text: "response"},
			}, tt.stopReason)

			srv := serveAnthropicResponse(t, body)
			defer srv.Close()

			be := newTestAnthropicBackend(t, srv.URL)
			result, err := be.ChatCompletion(context.Background(), []Message{
				{Role: "user", Content: "hello"},
			}, CompletionOptions{Model: "claude-haiku-4-5-20251001"})
			if err != nil {
				t.Fatalf("stop_reason=%q: unexpected error: %v", tt.stopReason, err)
			}
			if result.FinishReason != tt.want {
				t.Errorf("stop_reason=%q: got FinishReason=%q, want %q",
					tt.stopReason, result.FinishReason, tt.want)
			}
		})
	}
}
