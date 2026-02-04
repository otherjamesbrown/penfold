// Package activities provides tests for parse activities.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestParseEmail_ValidInput(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseEmailInput{
		TenantID: "test-tenant",
		SourceID: 123,
		BodyText: "This is a test email.\n\nOn 2024-01-01, someone wrote:\n> Quoted text here",
		BodyHTML: "",
	}

	output, err := activities.ParseEmail(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CleanBody)
	require.NotEmpty(t, output.NewContent)
	require.True(t, output.IsReply)
	require.Contains(t, output.QuotedContent, "Quoted text")
}

func TestParseEmail_EmptyTenantID(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseEmailInput{
		TenantID: "",
		SourceID: 123,
		BodyText: "Test email",
		BodyHTML: "",
	}

	output, err := activities.ParseEmail(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.True(t, IsValidationError(err))
}

func TestParseEmail_EmptyBody(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseEmailInput{
		TenantID: "test-tenant",
		SourceID: 123,
		BodyText: "",
		BodyHTML: "",
	}

	output, err := activities.ParseEmail(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.CleanBody)
	require.Empty(t, output.NewContent)
	require.Empty(t, output.QuotedContent)
	require.False(t, output.IsReply)
}

func TestParseEmail_HTMLBody(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseEmailInput{
		TenantID: "test-tenant",
		SourceID: 123,
		BodyText: "",
		BodyHTML: "<p>Hello <strong>world</strong></p><p>This is a test.</p>",
	}

	output, err := activities.ParseEmail(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CleanBody)
	require.Contains(t, output.CleanBody, "Hello world")
	require.Contains(t, output.CleanBody, "This is a test")
	require.NotContains(t, output.CleanBody, "<p>")
	require.NotContains(t, output.CleanBody, "<strong>")
}

func TestParseEmail_TextBodyWithQuotes(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseEmailInput{
		TenantID: "test-tenant",
		SourceID: 123,
		BodyText: "New message here.\n\n> Quoted line 1\n> Quoted line 2\n> Quoted line 3",
		BodyHTML: "",
	}

	output, err := activities.ParseEmail(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.NewContent)
	require.Contains(t, output.NewContent, "New message")
	require.True(t, output.IsReply)
	require.NotEmpty(t, output.QuotedContent)
}

func TestParseTranscript_ValidInput(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	// Simple VTT format with proper segment headers
	vttContent := `WEBVTT

1 "Speaker One" (123)
00:00:00.000 --> 00:00:05.000
Hello everyone, welcome to the meeting.

2 "Speaker Two" (456)
00:00:05.000 --> 00:00:10.000
Thanks for having me.
`

	input := ParseTranscriptInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Content:  vttContent,
		Format:   "vtt",
	}

	output, err := activities.ParseTranscript(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CleanText)
	require.Equal(t, "vtt", output.Format)
	require.NotEmpty(t, output.Speakers, "Expected at least one speaker")
	require.Greater(t, output.DurationMs, 0)
}

func TestParseTranscript_EmptyTenantID(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseTranscriptInput{
		TenantID: "",
		SourceID: 123,
		Content:  "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\n<v Speaker>Test",
		Format:   "vtt",
	}

	output, err := activities.ParseTranscript(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.True(t, IsValidationError(err))
}

func TestParseTranscript_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	input := ParseTranscriptInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Content:  "",
		Format:   "",
	}

	output, err := activities.ParseTranscript(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.CleanText)
	require.Empty(t, output.Speakers)
	require.Equal(t, 0, output.DurationMs)
	require.Empty(t, output.Format)
}

func TestParseTranscript_SRTContent(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)

	// Simple SRT format
	srtContent := `1
00:00:00,000 --> 00:00:05,000
Speaker 1: Hello everyone

2
00:00:05,000 --> 00:00:10,000
Speaker 2: Thanks for having me
`

	input := ParseTranscriptInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Content:  srtContent,
		Format:   "",
	}

	output, err := activities.ParseTranscript(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.NotEmpty(t, output.CleanText)
	require.Equal(t, "srt", output.Format)
	require.NotEmpty(t, output.Speakers)
	require.Greater(t, output.DurationMs, 0)
}
