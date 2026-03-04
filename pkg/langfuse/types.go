package langfuse

import "time"

// Langfuse observation level constants.
const (
	LevelError = "ERROR"
)

// TraceEvent holds the data for creating a trace in Langfuse.
type TraceEvent struct {
	ID          string
	Name        string
	Tags        []string
	Metadata    map[string]any
	Environment string
	Timestamp   time.Time
}

// SpanEvent holds the data for creating a span (observation) in Langfuse.
type SpanEvent struct {
	ID            string
	TraceID       string
	ParentID      string // parent observation ID; empty means root of trace
	Name          string
	Input         any    // optional input data displayed in the Langfuse UI
	StartTime     time.Time
	EndTime       time.Time
	Metadata      map[string]any
	Level         string // DEFAULT, ERROR; empty means success (omitted from JSON)
	StatusMessage string // human-readable error message for failed spans
}

// GenerationEvent holds the data for creating a generation (LLM call) in Langfuse.
type GenerationEvent struct {
	ID               string
	TraceID          string
	ParentID         string // phase span ID
	Name             string
	Model            string
	Input            any
	Output           any
	PromptTokens     int
	CompletionTokens int
	StartTime        time.Time
	EndTime          time.Time
	Level            string // DEFAULT, ERROR
	StatusMessage    string
	Metadata         map[string]any
}
