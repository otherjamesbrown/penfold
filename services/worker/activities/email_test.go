package activities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckOutboundWhitelist(t *testing.T) {
	whitelist := []string{"james@brown.chat", "alice@example.com"}

	t.Run("whitelisted address passes", func(t *testing.T) {
		err := checkOutboundWhitelist(whitelist, "james@brown.chat")
		assert.NoError(t, err)
	})

	t.Run("non-whitelisted address fails", func(t *testing.T) {
		err := checkOutboundWhitelist(whitelist, "stranger@example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in outbound whitelist")
	})

	t.Run("empty whitelist rejects all", func(t *testing.T) {
		err := checkOutboundWhitelist([]string{}, "james@brown.chat")
		require.Error(t, err)
	})
}

func TestCheckOutboundWhitelist_CaseInsensitive(t *testing.T) {
	whitelist := []string{"James@Brown.Chat"}

	t.Run("uppercase recipient matches lowercase whitelist entry", func(t *testing.T) {
		err := checkOutboundWhitelist(whitelist, "james@brown.chat")
		assert.NoError(t, err)
	})

	t.Run("mixed case recipient matches", func(t *testing.T) {
		err := checkOutboundWhitelist(whitelist, "JAMES@BROWN.CHAT")
		assert.NoError(t, err)
	})

	t.Run("different address still rejected", func(t *testing.T) {
		err := checkOutboundWhitelist(whitelist, "other@brown.chat")
		require.Error(t, err)
	})
}

func TestSendEmail_NilCreds(t *testing.T) {
	// Zero-value credentials should cause an OAuth failure when attempting to send.
	creds := EmailCredentials{}
	input := SendEmailInput{
		From:    "sender@example.com",
		To:      "recipient@example.com",
		Subject: "Test",
		Body:    "<p>Hello</p>",
	}

	_, err := SendEmail(context.Background(), creds, input)
	require.Error(t, err, "SendEmail with empty credentials should return an error")
}

func TestRenderDigestHTML(t *testing.T) {
	body := json.RawMessage(`{"summary": "Line one\nLine two", "items_count": 4, "model_used": "gpt-4"}`)
	got := renderDigestHTML(body, "Weekly Digest", "2026-03-01", "2026-03-07")

	assert.Contains(t, got, "<h2")
	assert.Contains(t, got, "Weekly Digest")
	assert.Contains(t, got, "2026-03-01")
	assert.Contains(t, got, "2026-03-07")
	assert.Contains(t, got, "Line one")
	assert.Contains(t, got, "Line two")
	assert.Contains(t, got, "font-family")
	assert.Contains(t, got, "<br>")
	assert.NotContains(t, got, "Line one\nLine two", "newlines in summary should be replaced with <br>")
}

func TestRenderDigestHTML_Fallback(t *testing.T) {
	body := json.RawMessage(`not valid json`)
	got := renderDigestHTML(body, "My Digest", "2026-03-01", "2026-03-07")

	assert.Contains(t, got, "<pre>")
	assert.Contains(t, got, "not valid json")
}

func TestRenderDigestHTML_MissingSummary(t *testing.T) {
	body := json.RawMessage(`{"items_count": 4, "model_used": "gpt-4"}`)
	got := renderDigestHTML(body, "My Digest", "2026-03-01", "2026-03-07")

	assert.Contains(t, got, "<div")
	assert.Contains(t, got, "My Digest")
}

func TestRFC2822MessageConstruction(t *testing.T) {
	input := SendEmailInput{
		From:    "sender@example.com",
		To:      "recipient@example.com",
		Subject: "Weekly Digest — 2026-03-01 to 2026-03-07",
		Body:    "<p>Hello <strong>world</strong></p>",
	}

	raw, err := buildRFC2822Message(input)
	require.NoError(t, err)

	msg := string(raw)

	t.Run("contains From header", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "From: sender@example.com"))
	})

	t.Run("contains To header", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "To: recipient@example.com"))
	})

	t.Run("contains Subject header", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "Subject: Weekly Digest"))
	})

	t.Run("MIME-Version header present", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "MIME-Version: 1.0"))
	})

	t.Run("Content-Type is multipart/alternative", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "Content-Type: multipart/alternative"))
	})

	t.Run("HTML body is included", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "<p>Hello <strong>world</strong></p>"))
	})

	t.Run("HTML part Content-Type is text/html", func(t *testing.T) {
		assert.True(t, strings.Contains(msg, "Content-Type: text/html"))
	})
}
