// Package watchlist provides types and repository for watch list and priority management.
package watchlist

import "time"

// WatchItem represents a watch list entry.
type WatchItem struct {
	ID                   int64     `json:"id,omitempty"`
	UserID               string    `json:"user_id"`
	AssertionRootID      *int64    `json:"assertion_root_id,omitempty"`
	ProjectID            *int64    `json:"project_id,omitempty"`
	Notes                string    `json:"notes"`
	CreatedAt            time.Time `json:"created_at"`
	AssertionDescription string    `json:"assertion_description,omitempty"` // denormalized
	ProjectName          string    `json:"project_name,omitempty"`          // denormalized
}

// PersonTrust represents a person with trust settings.
type PersonTrust struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	TrustLevel   int32    `json:"trust_level"`
	TrustDomains []string `json:"trust_domains,omitempty"`
}

// PersonSeniority represents a person with seniority settings.
type PersonSeniority struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	SeniorityTier int32  `json:"seniority_tier"`
	Title         string `json:"title,omitempty"`
}

// BriefingAssertion represents a prioritized assertion for briefing.
type BriefingAssertion struct {
	ID              int64     `json:"id"`
	Type            string    `json:"type"`
	Description     string    `json:"description"`
	Severity        string    `json:"severity"`
	LifecycleEvent  string    `json:"lifecycle_event"`
	OwnerName       string    `json:"owner_name"`
	SeniorityTier   int32     `json:"seniority_tier"`
	TrustLevel      int32     `json:"trust_level"`
	IsWatched       bool      `json:"is_watched"`
	PriorityTier    int32     `json:"priority_tier"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SeniorityEscalation represents a detected seniority escalation.
type SeniorityEscalation struct {
	AssertionRootID        int64  `json:"assertion_root_id"`
	PreviousMaxSeniority   int32  `json:"previous_max_seniority"`
	CurrentMaxSeniority    int32  `json:"current_max_seniority"`
	AssertionDescription   string `json:"assertion_description"`
}
