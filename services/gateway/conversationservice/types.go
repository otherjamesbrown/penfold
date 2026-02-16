// Package conversationservice types
package conversationservice

import "time"

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
