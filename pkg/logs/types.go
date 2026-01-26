// Package logs provides types and operations for service log management.
package logs

import (
	"time"
)

// Level represents log severity levels.
type Level string

const (
	// LevelDebug is for debug messages.
	LevelDebug Level = "debug"
	// LevelInfo is for informational messages.
	LevelInfo Level = "info"
	// LevelWarn is for warning messages.
	LevelWarn Level = "warn"
	// LevelError is for error messages.
	LevelError Level = "error"
)

// ValidLevels is the set of valid log levels.
var ValidLevels = map[Level]bool{
	LevelDebug: true,
	LevelInfo:  true,
	LevelWarn:  true,
	LevelError: true,
}

// IsValid returns true if the level is valid.
func (l Level) IsValid() bool {
	return ValidLevels[l]
}

// Priority returns the numeric priority of the level (higher = more severe).
func (l Level) Priority() int {
	priorities := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return priorities[l]
}

// Entry represents a single log entry.
type Entry struct {
	ID        int64             `json:"id"`
	TenantID  string            `json:"tenant_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Level     Level             `json:"level"`
	Service   string            `json:"service"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Caller    string            `json:"caller,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// EntryInput represents input for creating a log entry.
type EntryInput struct {
	TenantID  string            `json:"tenant_id,omitempty"`
	Timestamp time.Time         `json:"timestamp,omitempty"`
	Level     Level             `json:"level"`
	Service   string            `json:"service"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Caller    string            `json:"caller,omitempty"`
}

// Filter specifies criteria for querying logs.
type Filter struct {
	Service     string    `json:"service,omitempty"`      // Filter by service name
	Level       Level     `json:"level,omitempty"`        // Minimum log level
	Since       time.Time `json:"since,omitempty"`        // Logs after this time
	Until       time.Time `json:"until,omitempty"`        // Logs before this time
	Contains    string    `json:"contains,omitempty"`     // Full-text search in message
	TraceID     string    `json:"trace_id,omitempty"`     // Filter by trace ID
	TenantID    string    `json:"tenant_id,omitempty"`    // Filter by tenant
	Limit       int       `json:"limit,omitempty"`        // Max results (default 100)
	Offset      int       `json:"offset,omitempty"`       // Pagination offset
	OrderAsc    bool      `json:"order_asc,omitempty"`    // True for oldest first
}

// Stats contains log statistics.
type Stats struct {
	TotalCount   int64            `json:"total_count"`
	CountByLevel map[Level]int64  `json:"count_by_level"`
	CountByService map[string]int64 `json:"count_by_service"`
	OldestLog    time.Time        `json:"oldest_log,omitempty"`
	NewestLog    time.Time        `json:"newest_log,omitempty"`
}
