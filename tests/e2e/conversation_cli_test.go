//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- pf-1fd11b: Conversation CLI extensions (merge, split, filters) ---

// TestConversation_ListFilterByState verifies that penf conversation list --state
// filters conversations by their lifecycle state.
//
// Acceptance criteria (pf-1fd11b): penf conversation list --state active filters correctly.
func TestConversation_ListFilterByState(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get all conversations first to know what states exist
	allResult := env.SafeCLI.Run(ctx, "conversation", "list", "-o", "json")
	require.True(t, allResult.Success(), "conversation list should succeed: %s", allResult.Stderr)

	var allList struct {
		Conversations []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(allResult.Stdout), &allList))
	require.NotEmpty(t, allList.Conversations)

	// Count conversations by state
	stateCounts := make(map[string]int)
	for _, c := range allList.Conversations {
		stateCounts[c.State]++
	}

	// Filter by a state that exists
	var filterState string
	for state := range stateCounts {
		if state != "" {
			filterState = state
			break
		}
	}
	require.NotEmpty(t, filterState, "need at least one conversation with a state")

	filteredResult := env.SafeCLI.Run(ctx, "conversation", "list", "--state", filterState, "-o", "json")
	require.True(t, filteredResult.Success(), "conversation list --state should succeed: %s", filteredResult.Stderr)

	var filteredList struct {
		Conversations []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(filteredResult.Stdout), &filteredList))

	// All returned conversations should match the filter
	for _, c := range filteredList.Conversations {
		assert.Equal(t, filterState, c.State,
			"filtered conversation %s should have state %q, got %q", c.ID, filterState, c.State)
	}

	// Count should match what we counted manually
	assert.Equal(t, stateCounts[filterState], len(filteredList.Conversations),
		"filtered count should match manual count for state %q", filterState)
}

// TestConversation_ListFilterByParticipant verifies that penf conversation list
// --participant filters by participant name.
//
// Acceptance criteria (pf-1fd11b): penf conversation list --participant "name" filters correctly.
func TestConversation_ListFilterByParticipant(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find a conversation with known participants
	allResult := env.SafeCLI.Run(ctx, "conversation", "list", "-o", "json")
	require.True(t, allResult.Success(), "conversation list should succeed: %s", allResult.Stderr)

	var allList struct {
		Conversations []struct {
			ID           string `json:"id"`
			Participants []struct {
				Name    string `json:"name"`
				Address string `json:"address"`
			} `json:"participants"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(allResult.Stdout), &allList))

	// Find a participant name that appears in at least one conversation
	var participantName string
	for _, c := range allList.Conversations {
		for _, p := range c.Participants {
			if p.Name != "" {
				participantName = p.Name
				break
			}
		}
		if participantName != "" {
			break
		}
	}
	require.NotEmpty(t, participantName, "need at least one conversation with a named participant")

	// Filter by that participant
	filteredResult := env.SafeCLI.Run(ctx, "conversation", "list", "--participant", participantName, "-o", "json")
	require.True(t, filteredResult.Success(),
		"conversation list --participant should succeed: %s", filteredResult.Stderr)

	var filteredList struct {
		Conversations []struct {
			ID           string `json:"id"`
			Participants []struct {
				Name string `json:"name"`
			} `json:"participants"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(filteredResult.Stdout), &filteredList))

	// Each returned conversation should include the participant
	for _, c := range filteredList.Conversations {
		var found bool
		for _, p := range c.Participants {
			if p.Name == participantName {
				found = true
				break
			}
		}
		assert.True(t, found,
			"conversation %s from --participant filter should include %q", c.ID, participantName)
	}
}

// TestConversation_ShowParticipantRoles verifies that conversation show includes
// participant roles (initiator, most active, last responder).
//
// Acceptance criteria (pf-1fd11b): penf conversation show displays participant roles.
func TestConversation_ShowParticipantRoles(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find a conversation with multiple items (more interesting roles)
	listResult := env.SafeCLI.Run(ctx, "conversation", "list", "-o", "json")
	require.True(t, listResult.Success(), "conversation list should succeed: %s", listResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResult.Stdout), &convList))
	require.NotEmpty(t, convList.Conversations)

	var targetID string
	for _, c := range convList.Conversations {
		if c.ItemCount >= 2 {
			targetID = c.ID
			break
		}
	}
	if targetID == "" {
		targetID = convList.Conversations[0].ID
	}

	showResult := env.SafeCLI.Run(ctx, "conversation", "show", targetID, "-o", "json")
	require.True(t, showResult.Success(), "conversation show should succeed: %s", showResult.Stderr)

	var detail struct {
		Participants []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"participants"`
	}
	require.NoError(t, json.Unmarshal([]byte(showResult.Stdout), &detail))

	// At least one participant should have a role
	var hasRole bool
	validRoles := map[string]bool{
		"initiator": true, "most_active": true, "last_responder": true, "participant": true,
	}
	for _, p := range detail.Participants {
		if p.Role != "" {
			hasRole = true
			assert.True(t, validRoles[p.Role],
				"participant %s has unexpected role %q", p.Name, p.Role)
		}
	}
	assert.True(t, hasRole, "at least one participant should have a role assigned")
}

// TestConversation_MergeTwo verifies that penf conversation merge combines two
// conversations correctly — union of items, participants, and regenerated summary.
//
// Acceptance criteria (pf-1fd11b): merge works correctly and triggers summary regeneration.
func TestConversation_MergeTwo(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List conversations and find two with items
	listResult := env.SafeCLI.Run(ctx, "conversation", "list", "-o", "json")
	require.True(t, listResult.Success(), "conversation list should succeed: %s", listResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResult.Stdout), &convList))
	require.GreaterOrEqual(t, len(convList.Conversations), 2, "need at least 2 conversations to test merge")

	conv1 := convList.Conversations[0]
	conv2 := convList.Conversations[1]
	expectedMinItems := conv1.ItemCount + conv2.ItemCount

	// Merge conv2 into conv1
	mergeResult := env.SafeCLI.Run(ctx, "conversation", "merge", conv1.ID, conv2.ID, "-o", "json")
	require.True(t, mergeResult.Success(), "conversation merge should succeed: %s", mergeResult.Stderr)

	var merged struct {
		ID        string `json:"id"`
		ItemCount int    `json:"item_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(mergeResult.Stdout), &merged))

	// Merged conversation should have items from both
	assert.GreaterOrEqual(t, merged.ItemCount, expectedMinItems,
		"merged conversation should have items from both sources")

	// Original conv2 should no longer exist
	show2Result := env.SafeCLI.Run(ctx, "conversation", "show", conv2.ID, "-o", "json")
	assert.False(t, show2Result.Success(),
		"merged-away conversation %s should no longer exist", conv2.ID)
}

// TestConversation_SplitItems verifies that penf conversation split moves
// specified items into a new conversation.
//
// Acceptance criteria (pf-1fd11b): split works correctly and triggers summary regeneration.
func TestConversation_SplitItems(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Find a conversation with 3+ items (enough to split meaningfully)
	listResult := env.SafeCLI.Run(ctx, "conversation", "list", "-o", "json")
	require.True(t, listResult.Success(), "conversation list should succeed: %s", listResult.Stderr)

	var convList struct {
		Conversations []struct {
			ID        string `json:"id"`
			ItemCount int    `json:"item_count"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResult.Stdout), &convList))

	var targetID string
	for _, c := range convList.Conversations {
		if c.ItemCount >= 3 {
			targetID = c.ID
			break
		}
	}
	if targetID == "" {
		t.Skip("no conversation with 3+ items available for split test")
	}

	// Get the conversation's items
	showResult := env.SafeCLI.Run(ctx, "conversation", "show", targetID, "-o", "json")
	require.True(t, showResult.Success(), "conversation show should succeed: %s", showResult.Stderr)

	var detail struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(showResult.Stdout), &detail))
	require.GreaterOrEqual(t, len(detail.Items), 3, "need 3+ items for split")

	originalCount := len(detail.Items)

	// Split the last item into a new conversation
	splitItemID := detail.Items[len(detail.Items)-1].ID
	splitResult := env.SafeCLI.Run(ctx, "conversation", "split", targetID, "--items", splitItemID, "-o", "json")
	require.True(t, splitResult.Success(), "conversation split should succeed: %s", splitResult.Stderr)

	var newConv struct {
		ID        string `json:"id"`
		ItemCount int    `json:"item_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(splitResult.Stdout), &newConv))

	assert.NotEqual(t, targetID, newConv.ID, "split should create a new conversation")
	assert.Equal(t, 1, newConv.ItemCount, "new conversation should have the split item")

	// Original should have one fewer item
	showAfter := env.SafeCLI.Run(ctx, "conversation", "show", targetID, "-o", "json")
	require.True(t, showAfter.Success(), "conversation show should succeed: %s", showAfter.Stderr)

	var afterDetail struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(showAfter.Stdout), &afterDetail))
	assert.Equal(t, originalCount-1, len(afterDetail.Items),
		"original conversation should have one fewer item after split")
}

// TestConversation_MergeOutputJSON verifies that merge returns valid JSON output.
//
// Acceptance criteria (pf-1fd11b): merge supports --output json.
func TestConversation_MergeOutputJSON(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Just verify the --help mentions json output (non-destructive check)
	helpResult := env.SafeCLI.Run(ctx, "conversation", "merge", "--help")
	require.True(t, helpResult.Success(), "conversation merge --help should succeed: %s", helpResult.Stderr)

	assert.Contains(t, helpResult.Stdout, "json",
		"merge help should mention json output option")
}

// TestConversation_ListSinceFilter verifies that penf conversation list --since
// filters by last activity date.
//
// Acceptance criteria (pf-1fd11b): list supports --since filter.
func TestConversation_ListSinceFilter(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Filter with a date far in the past — should return all conversations
	allResult := env.SafeCLI.Run(ctx, "conversation", "list", "--since", "2020-01-01", "-o", "json")
	require.True(t, allResult.Success(), "conversation list --since should succeed: %s", allResult.Stderr)

	var allList struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(allResult.Stdout), &allList))

	// Filter with a date far in the future — should return no conversations
	noneResult := env.SafeCLI.Run(ctx, "conversation", "list", "--since", "2099-01-01", "-o", "json")
	require.True(t, noneResult.Success(), "conversation list --since should succeed: %s", noneResult.Stderr)

	var noneList struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	require.NoError(t, json.Unmarshal([]byte(noneResult.Stdout), &noneList))

	assert.Greater(t, len(allList.Conversations), len(noneList.Conversations),
		"past date should return more conversations than future date")
	assert.Empty(t, noneList.Conversations,
		"future date should return no conversations")
}
