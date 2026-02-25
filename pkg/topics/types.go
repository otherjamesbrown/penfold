// Package topics provides types and repository for topic management.
// Topics represent contextual knowledge entities — richer than glossary terms
// but without the ownership, actions, or risks of projects/products.
package topics

import "time"

// Topic represents a contextual knowledge entity with keywords for auto-tagging.
type Topic struct {
	ID          int64     `json:"id,omitempty"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Keywords    []string  `json:"keywords,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TopicFilter contains filtering options for topic queries.
type TopicFilter struct {
	TenantID   string
	NameSearch string
	Keyword    string
	Limit      int
	Offset     int
}
