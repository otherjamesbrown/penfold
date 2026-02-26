// Package ledger provides types and repository for session ledger management.
package ledger

import "time"

// Entry represents a session ledger entry.
type Entry struct {
	ID         int64             `json:"id,omitempty"`
	TenantID   string            `json:"tenant_id"`
	SessionID  string            `json:"session_id"`
	EntryType  string            `json:"entry_type"`
	Title      string            `json:"title"`
	Body       *string           `json:"body,omitempty"`
	Source     string            `json:"source"`
	Agent      string            `json:"agent"`
	Labels     []string          `json:"labels"`
	ShardRefs  []string          `json:"shard_refs"`
	Metadata   map[string]string `json:"metadata"`
	CreatedAt  time.Time         `json:"created_at"`
}

// SessionSummary is an aggregated view of a session's entries.
type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	Agent        string    `json:"agent"`
	EntryCount   int64     `json:"entry_count"`
	FirstEntry   time.Time `json:"first_entry"`
	LastEntry    time.Time `json:"last_entry"`
	EntryTypes   []string  `json:"entry_types"`
	Labels       []string  `json:"labels"`
	LatestTitle  *string   `json:"latest_title,omitempty"`
}

// ConsolidationDecision represents a decision extracted during consolidation.
type ConsolidationDecision struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	SessionID string `json:"session_id"`
}

// ConsolidationPattern represents a pattern identified during consolidation.
type ConsolidationPattern struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Evidence []string `json:"evidence"`
}

// Consolidation represents a session ledger consolidation record.
type Consolidation struct {
	ID             int64                   `json:"id,omitempty"`
	TenantID       string                  `json:"tenant_id"`
	Title          string                  `json:"title"`
	Body           string                  `json:"body"`
	SourceEntryIDs []int64                 `json:"source_entry_ids"`
	SessionIDs     []string                `json:"session_ids"`
	Decisions      []ConsolidationDecision `json:"decisions"`
	Patterns       []ConsolidationPattern  `json:"patterns"`
	TimeStart      time.Time               `json:"time_start"`
	TimeEnd        time.Time               `json:"time_end"`
	ModelID        *string                 `json:"model_id,omitempty"`
	Metadata       map[string]string       `json:"metadata"`
	CreatedAt      time.Time               `json:"created_at"`
}

// EntryFilter holds filter parameters for ListEntries.
type EntryFilter struct {
	TenantID  *string
	SessionID *string
	EntryType *string
	Source    *string
	Agent     *string
	Labels    []string
	Limit     int32
	Offset    int32
}

// SessionFilter holds filter parameters for ListSessions.
type SessionFilter struct {
	TenantID string
	Agent    *string
	Limit    int32
	Offset   int32
}

// SearchFilter holds filter parameters for SearchEntries.
type SearchFilter struct {
	TenantID string
	Query    string
	Limit    int32
	Offset   int32
}
