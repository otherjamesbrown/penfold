//go:build live

// Package live provides helpers for live API tests.
// These tests make real API calls to external services and may incur costs.
package live

import (
	"os"
	"testing"
)

// RequireGeminiAPIKey skips the test if GEMINI_API_KEY is not set.
func RequireGeminiAPIKey(t *testing.T) string {
	t.Helper()

	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set - skipping live test")
	}
	return key
}

// RequireGmailCredentials skips the test if Gmail OAuth credentials are not set.
func RequireGmailCredentials(t *testing.T) string {
	t.Helper()

	path := os.Getenv("GMAIL_OAUTH_CREDENTIALS")
	if path == "" {
		t.Skip("GMAIL_OAUTH_CREDENTIALS not set - skipping live test")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("Gmail credentials file not found: %s", path)
	}

	return path
}

// RequireOpenAIAPIKey skips the test if OPENAI_API_KEY is not set.
func RequireOpenAIAPIKey(t *testing.T) string {
	t.Helper()

	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set - skipping live test")
	}
	return key
}

// RequireAnthropicAPIKey skips the test if ANTHROPIC_API_KEY is not set.
func RequireAnthropicAPIKey(t *testing.T) string {
	t.Helper()

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set - skipping live test")
	}
	return key
}
