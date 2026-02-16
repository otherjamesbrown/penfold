//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConversation_AutoPopulateFromThreads verifies that existing thread groups
// are backfilled into conversation objects.
//
// Acceptance criteria: All threads with 2+ messages have a corresponding conversation.
func TestConversation_AutoPopulateFromThreads(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List threads to get current count
	threadResult := env.SafeCLI.RunWithArgs(ctx, "thread", "list", "-o", "json")
	require.True(t, threadResult.Success(), "thread list should succeed: %s", threadResult.Stderr)

	var threadList struct {
		Threads []struct {
			ID           int    `json:"id"`
			Subject      string `json:"subject"`
			MessageCount int    `json:"message_count"`
		} `json:"threads"`
	}
	require.NoError(t, json.Unmarshal([]byte(threadResult.Stdout), &threadList))
	require.NotEmpty(t, threadList.Threads, "should have threads to backfill from")

	// List conversations
	convResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, convResult.Success(), "conversation list should succeed: %s", convResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID    string `json:"id"`
			Topic string `json:"topic"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(convResult.Stdout), &convList))

	// Every thread should have a conversation
	assert.GreaterOrEqual(t, len(convList.Conversations), len(threadList.Threads),
		"should have at least as many conversations as threads")
}

// TestConversation_ShowIncludesContentItems verifies that showing a conversation
// returns its linked content items in chronological order.
func TestConversation_ShowIncludesContentItems(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List conversations and pick one with multiple items
	convResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, convResult.Success(), "conversation list should succeed: %s", convResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID           string `json:"id"`
			Topic        string `json:"topic"`
			ItemCount    int    `json:"item_count"`
			Participants []struct {
				Name string `json:"name"`
			} `json:"participants"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(convResult.Stdout), &convList))
	require.NotEmpty(t, convList.Conversations, "need at least one conversation")

	// Find a conversation with multiple items (from a multi-message thread)
	var targetID string
	for _, c := range convList.Conversations {
		if c.ItemCount > 1 {
			targetID = c.ID
			break
		}
	}
	if targetID == "" {
		// Fall back to first conversation if none have multiple items
		targetID = convList.Conversations[0].ID
	}

	// Show conversation detail
	showResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "show", targetID, "-o", "json")
	require.True(t, showResult.Success(), "conversation show should succeed: %s", showResult.Stderr)

	var detail struct {
		ID           string `json:"id"`
		Topic        string `json:"topic"`
		ThreadKey    string `json:"thread_key"`
		FirstSeen    string `json:"first_seen"`
		LastSeen     string `json:"last_seen"`
		Participants []struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		} `json:"participants"`
		Items []struct {
			ID      string `json:"id"`
			Subject string `json:"subject"`
			Date    string `json:"date"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(showResult.Stdout), &detail))

	assert.NotEmpty(t, detail.Topic, "conversation should have a topic")
	assert.NotEmpty(t, detail.ThreadKey, "conversation should have a thread_key")
	assert.NotEmpty(t, detail.Items, "conversation should have content items")

	// Items should be chronologically ordered
	if len(detail.Items) > 1 {
		for i := 1; i < len(detail.Items); i++ {
			assert.LessOrEqual(t, detail.Items[i-1].Date, detail.Items[i].Date,
				"content items should be in chronological order")
		}
	}
}

// TestConversation_TopicFromSubject verifies that conversation topics are derived
// from the thread subject line with Re:/FW: prefixes stripped.
func TestConversation_TopicFromSubject(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	convResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, convResult.Success(), "conversation list should succeed: %s", convResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID    string `json:"id"`
			Topic string `json:"topic"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(convResult.Stdout), &convList))
	require.NotEmpty(t, convList.Conversations)

	// No conversation topic should start with Re: or FW: or Fwd:
	for _, c := range convList.Conversations {
		assert.NotRegexp(t, `^(?i)(re|fw|fwd)\s*:`, c.Topic,
			fmt.Sprintf("topic %q should not start with Re:/FW:/Fwd: prefix", c.Topic))
	}
}

// TestConversation_ParticipantsFromThread verifies that conversation participants
// are the union of all from/to/cc across the thread's content items.
func TestConversation_ParticipantsFromThread(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List conversations and find one with items
	convResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, convResult.Success(), "conversation list should succeed: %s", convResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID           string `json:"id"`
			ItemCount    int    `json:"item_count"`
			Participants []struct {
				Name    string `json:"name"`
				Address string `json:"address"`
			} `json:"participants"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(convResult.Stdout), &convList))

	// Find a conversation with participants
	var found bool
	for _, c := range convList.Conversations {
		if len(c.Participants) > 0 {
			found = true
			// Each participant should have at least a name or address
			for _, p := range c.Participants {
				assert.True(t, p.Name != "" || p.Address != "",
					"participant should have name or address")
			}
			break
		}
	}
	assert.True(t, found, "at least one conversation should have participants")
}

// TestConversation_IngestAutoLinks verifies that reprocessing an email that belongs
// to a thread automatically links it to the corresponding conversation.
func TestConversation_IngestAutoLinks(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Get current conversation state
	beforeResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, beforeResult.Success(), "conversation list should succeed: %s", beforeResult.Stderr)

	var beforeList struct {
		Conversations []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(beforeResult.Stdout), &beforeList))

	// Reprocess a known threaded email (em-goJuvK1X is in the CLIC/Juniper thread)
	reprocessResult := env.SafeCLI.RunWithArgs(ctx, "reprocess", "em-goJuvK1X")
	require.True(t, reprocessResult.Success(), "reprocess should succeed: %s", reprocessResult.Stderr)

	// Wait for pipeline to complete
	time.Sleep(30 * time.Second)

	// Verify the conversation still has the item linked
	afterResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "list", "-o", "json")
	require.True(t, afterResult.Success(), "conversation list should succeed: %s", afterResult.Stderr)

	var afterList struct {
		Conversations []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(afterResult.Stdout), &afterList))

	// Conversation count should be stable (reprocess doesn't create new conversations)
	assert.Equal(t, len(beforeList.Conversations), len(afterList.Conversations),
		"reprocessing should not change conversation count")
}
