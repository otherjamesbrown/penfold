// Package teams provides parsing for Microsoft Teams channel messages.
// It converts Teams message threads (root + replies) into plain text
// suitable for pipeline ingestion.
package teams

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/graph"
)

var (
	// htmlTagRe matches HTML tags for stripping.
	htmlTagRe = regexp.MustCompile(`<[^>]*>`)
	// whitespaceRe collapses multiple whitespace into single spaces.
	whitespaceRe = regexp.MustCompile(`\s+`)
	// htmlEntities maps common HTML entities to their text equivalents.
	htmlEntities = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
	)
)

// StripHTML removes HTML tags and decodes common entities, returning plain text.
func StripHTML(html string) string {
	// Replace line-breaking tags with newlines before stripping
	text := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
	).Replace(html)
	// Strip HTML tags first (before entity decoding, so decoded < > aren't treated as tags)
	text = htmlTagRe.ReplaceAllString(text, "")
	// Then decode HTML entities
	text = htmlEntities.Replace(text)
	// Collapse excessive whitespace but preserve paragraph breaks
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(whitespaceRe.ReplaceAllString(line, " "))
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}

// ParseThread converts a Teams thread (root message + replies) into plain text.
// The output format preserves conversation structure:
//
//	[Subject: <subject>]
//	<sender> (<timestamp>):
//	<body text>
//
//	--- Reply ---
//	<sender> (<timestamp>):
//	<body text>
func ParseThread(thread graph.TeamsThread) string {
	var b strings.Builder

	// Subject line if present
	if thread.Root.Subject != "" {
		fmt.Fprintf(&b, "Subject: %s\n\n", thread.Root.Subject)
	}

	// Root message
	writeMessage(&b, thread.Root)

	// Replies
	for _, reply := range thread.Replies {
		b.WriteString("\n--- Reply ---\n")
		writeMessage(&b, reply)
	}

	return strings.TrimSpace(b.String())
}

func writeMessage(b *strings.Builder, msg graph.TeamsMessage) {
	// Header: sender and timestamp
	timestamp := msg.CreatedAt.Format("2006-01-02 15:04")
	if msg.SenderName != "" {
		fmt.Fprintf(b, "%s (%s):\n", msg.SenderName, timestamp)
	} else {
		fmt.Fprintf(b, "(%s):\n", timestamp)
	}

	// Body: strip HTML if needed
	body := msg.BodyContent
	if strings.EqualFold(msg.BodyContentType, "html") {
		body = StripHTML(body)
	}
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
}

// ThreadMetadata extracts metadata fields from a thread for storage in the sources table.
func ThreadMetadata(thread graph.TeamsThread, channelName string) map[string]interface{} {
	meta := map[string]interface{}{
		"team_id":      thread.Root.TeamID,
		"channel_id":   thread.Root.ChannelID,
		"channel_name": channelName,
		"message_id":   thread.Root.ID,
		"sender":       thread.Root.SenderName,
		"reply_count":  len(thread.Replies),
		"source":       "teams_channel",
	}
	if thread.Root.Subject != "" {
		meta["subject"] = thread.Root.Subject
	}
	if thread.Root.WebURL != "" {
		meta["web_url"] = thread.Root.WebURL
	}

	// Collect unique participants
	participants := map[string]bool{}
	participants[thread.Root.SenderName] = true
	for _, r := range thread.Replies {
		if r.SenderName != "" {
			participants[r.SenderName] = true
		}
	}
	names := make([]string, 0, len(participants))
	for name := range participants {
		if name != "" {
			names = append(names, name)
		}
	}
	meta["participants"] = names

	return meta
}
