// Package conversationservice types
package conversationservice

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// generateShortID returns a random 8-character hex string for conversation IDs.
func generateShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ConversationSummary represents a conversation in the list view.
type ConversationSummary struct {
	ID               string
	TenantID         string
	Topic            string
	ThreadKey        *string
	FirstSeen        *time.Time
	LastSeen         *time.Time
	ParticipantCount int32
	ItemCount        int32
	State            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ConversationDetail represents a full conversation with items and participants.
type ConversationDetail struct {
	ID               string
	TenantID         string
	Topic            string
	ThreadKey        *string
	FirstSeen        *time.Time
	LastSeen         *time.Time
	ParticipantCount int32
	ItemCount        int32
	StateSummary     *string
	SummaryVersion   int32
	SummaryUpdatedAt *time.Time
	State            string
	StateReason      *string
	StateChangedAt   *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Items            []ConversationItem
	Participants     []ConversationParticipant
}

// ConversationItem represents a content item in a conversation.
type ConversationItem struct {
	ConversationID string
	ContentID      string
	SourceID       *int64
	AddedAt        time.Time
	TenantID       string
}

// ConversationParticipant represents a participant in a conversation.
type ConversationParticipant struct {
	ConversationID string
	Name           *string
	Address        *string
	TenantID       string
}

// Conversation is the full model used for inserts/updates.
type Conversation struct {
	ID               string
	TenantID         string
	Topic            string
	ThreadKey        *string
	FirstSeen        *time.Time
	LastSeen         *time.Time
	ParticipantCount int32
	ItemCount        int32
	Metadata         map[string]interface{}
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// StateHistoryEntry represents a single state transition in conversation history.
type StateHistoryEntry struct {
	ID             int64
	ConversationID string
	OldState       string
	NewState       string
	Reason         string
	CreatedAt      time.Time
}

// AuditOrphan represents a content item that should be in a conversation but isn't.
type AuditOrphan struct {
	ContentID              string
	Reason                 string
	SuggestedConversationID string
}

// AuditDuplicate represents an item linked to multiple conversations.
type AuditDuplicate struct {
	ContentID       string
	ConversationIDs []string
}

// AuditMergeCandidate represents two conversations that may need merging.
type AuditMergeCandidate struct {
	ConversationIDA string
	ConversationIDB string
	TopicA          string
	TopicB          string
	Reason          string
	SharedItems     []string
}
