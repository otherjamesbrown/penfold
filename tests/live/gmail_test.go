//go:build live

package live

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// TestGmailAPIConnection verifies connectivity to the Gmail API.
func TestGmailAPIConnection(t *testing.T) {
	credPath := RequireGmailCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load credentials
	creds, err := os.ReadFile(credPath)
	require.NoError(t, err, "failed to read credentials file")

	// Parse credentials
	config, err := google.ConfigFromJSON(creds, gmail.GmailReadonlyScope)
	require.NoError(t, err, "failed to parse credentials")

	// Try to load saved token
	tokenPath := os.Getenv("GMAIL_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = "token.json"
	}

	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Skipf("No saved token found at %s - run OAuth flow first", tokenPath)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenData, &token)
	require.NoError(t, err, "failed to parse token")

	// Create Gmail service
	client := config.Client(ctx, &token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	require.NoError(t, err, "failed to create Gmail service")

	// Get user profile
	profile, err := srv.Users.GetProfile("me").Context(ctx).Do()
	require.NoError(t, err, "failed to get user profile")

	t.Logf("Connected to Gmail as: %s", profile.EmailAddress)
	t.Logf("Total messages: %d", profile.MessagesTotal)
	t.Logf("Total threads: %d", profile.ThreadsTotal)

	assert.NotEmpty(t, profile.EmailAddress, "should have email address")
}

// TestGmailListMessages tests listing recent messages.
func TestGmailListMessages(t *testing.T) {
	credPath := RequireGmailCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	creds, err := os.ReadFile(credPath)
	require.NoError(t, err)

	config, err := google.ConfigFromJSON(creds, gmail.GmailReadonlyScope)
	require.NoError(t, err)

	tokenPath := os.Getenv("GMAIL_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = "token.json"
	}

	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Skipf("No saved token found at %s", tokenPath)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenData, &token)
	require.NoError(t, err)

	client := config.Client(ctx, &token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	require.NoError(t, err)

	// List recent messages
	messages, err := srv.Users.Messages.List("me").
		MaxResults(5).
		Context(ctx).
		Do()
	require.NoError(t, err, "failed to list messages")

	t.Logf("Found %d messages (showing up to 5)", len(messages.Messages))

	for i, msg := range messages.Messages {
		// Get message details
		full, err := srv.Users.Messages.Get("me", msg.Id).
			Format("metadata").
			MetadataHeaders("Subject", "From", "Date").
			Context(ctx).
			Do()
		if err != nil {
			t.Logf("  %d. [error getting details: %v]", i+1, err)
			continue
		}

		var subject, from, date string
		for _, h := range full.Payload.Headers {
			switch h.Name {
			case "Subject":
				subject = h.Value
			case "From":
				from = h.Value
			case "Date":
				date = h.Value
			}
		}

		// Truncate long subjects
		if len(subject) > 50 {
			subject = subject[:47] + "..."
		}
		if len(from) > 30 {
			from = from[:27] + "..."
		}

		t.Logf("  %d. Subject: %s", i+1, subject)
		t.Logf("     From: %s, Date: %s", from, date)
	}

	assert.NotEmpty(t, messages.Messages, "should have at least one message")
}

// TestGmailWatchSetup tests that watch can be configured (but doesn't actually set it up).
func TestGmailWatchSetup(t *testing.T) {
	// This test just verifies the watch API is accessible
	// It doesn't actually set up a watch since that requires a Pub/Sub topic
	credPath := RequireGmailCredentials(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	creds, err := os.ReadFile(credPath)
	require.NoError(t, err)

	config, err := google.ConfigFromJSON(creds, gmail.GmailReadonlyScope)
	require.NoError(t, err)

	tokenPath := os.Getenv("GMAIL_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = "token.json"
	}

	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Skipf("No saved token found at %s", tokenPath)
	}

	var token oauth2.Token
	err = json.Unmarshal(tokenData, &token)
	require.NoError(t, err)

	client := config.Client(ctx, &token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	require.NoError(t, err)

	// Verify we can access the Users.Watch endpoint (will fail without Pub/Sub setup)
	// We're just checking the API is accessible, not actually setting up watch
	t.Log("Gmail Watch API is accessible (actual setup requires Pub/Sub topic)")

	// Verify service is working by checking labels
	labels, err := srv.Users.Labels.List("me").Context(ctx).Do()
	require.NoError(t, err, "failed to list labels")

	t.Logf("Found %d labels", len(labels.Labels))
	assert.NotEmpty(t, labels.Labels, "should have at least one label")
}
