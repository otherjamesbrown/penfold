package langfuse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event is an envelope for a single ingestion event sent to the Langfuse batch API.
type Event struct {
	ID        string `json:"id"`        // unique envelope UUID for deduplication
	Type      string `json:"type"`      // "trace-create", "span-create", "generation-create", "span-update"
	Timestamp string `json:"timestamp"` // ISO8601
	Body      any    `json:"body"`      // event-specific body struct
}

// Ingestion buffers events and flushes them in a single batch POST to Langfuse.
type Ingestion struct {
	client *Client
	mu     sync.Mutex
	batch  []Event
}

// NewIngestion creates a new Ingestion buffer backed by the given Client.
func NewIngestion(client *Client) *Ingestion {
	return &Ingestion{
		client: client,
		batch:  make([]Event, 0),
	}
}

// newEnvelopeID generates a unique UUID for the event envelope.
func newEnvelopeID() string {
	return uuid.New().String()
}

// isoNow returns the current time formatted as ISO8601/RFC3339.
func isoNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// traceCreateBody is the JSON body for a trace-create event.
type traceCreateBody struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Tags        []string       `json:"tags,omitempty"`
	Timestamp   string         `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Environment string         `json:"environment,omitempty"`
}

// spanCreateBody is the JSON body for a span-create event.
type spanCreateBody struct {
	ID                  string         `json:"id"`
	TraceID             string         `json:"traceId"`
	ParentObservationID string         `json:"parentObservationId,omitempty"`
	Name                string         `json:"name"`
	StartTime           string         `json:"startTime"`
	EndTime             string         `json:"endTime,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// generationUsage holds token counts for a generation event.
type generationUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
}

// generationCreateBody is the JSON body for a generation-create event.
type generationCreateBody struct {
	ID                  string         `json:"id"`
	TraceID             string         `json:"traceId"`
	ParentObservationID string         `json:"parentObservationId,omitempty"`
	Name                string         `json:"name"`
	Model               string         `json:"model,omitempty"`
	Input               any            `json:"input,omitempty"`
	Output              any            `json:"output,omitempty"`
	Usage               generationUsage `json:"usage,omitempty"`
	StartTime           string         `json:"startTime"`
	EndTime             string         `json:"endTime,omitempty"`
	Level               string         `json:"level,omitempty"`
	StatusMessage       string         `json:"statusMessage,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// spanUpdateBody is the JSON body for a span-update event.
type spanUpdateBody struct {
	ID      string `json:"id"`
	EndTime string `json:"endTime"`
}

// CreateTrace buffers a trace-create event.
func (i *Ingestion) CreateTrace(e TraceEvent) {
	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	body := traceCreateBody{
		ID:          e.ID,
		Name:        e.Name,
		Tags:        e.Tags,
		Timestamp:   ts.UTC().Format(time.RFC3339Nano),
		Metadata:    e.Metadata,
		Environment: e.Environment,
	}

	evt := Event{
		ID:        newEnvelopeID(),
		Type:      "trace-create",
		Timestamp: isoNow(),
		Body:      body,
	}

	i.mu.Lock()
	i.batch = append(i.batch, evt)
	i.mu.Unlock()
}

// CreateSpan buffers a span-create event.
func (i *Ingestion) CreateSpan(e SpanEvent) {
	body := spanCreateBody{
		ID:                  e.ID,
		TraceID:             e.TraceID,
		ParentObservationID: e.ParentID,
		Name:                e.Name,
		StartTime:           e.StartTime.UTC().Format(time.RFC3339Nano),
		Metadata:            e.Metadata,
	}
	if !e.EndTime.IsZero() {
		body.EndTime = e.EndTime.UTC().Format(time.RFC3339Nano)
	}

	evt := Event{
		ID:        newEnvelopeID(),
		Type:      "span-create",
		Timestamp: isoNow(),
		Body:      body,
	}

	i.mu.Lock()
	i.batch = append(i.batch, evt)
	i.mu.Unlock()
}

// CreateGeneration buffers a generation-create event.
func (i *Ingestion) CreateGeneration(e GenerationEvent) {
	body := generationCreateBody{
		ID:                  e.ID,
		TraceID:             e.TraceID,
		ParentObservationID: e.ParentID,
		Name:                e.Name,
		Model:               e.Model,
		Input:               e.Input,
		Output:              e.Output,
		Usage: generationUsage{
			PromptTokens:     e.PromptTokens,
			CompletionTokens: e.CompletionTokens,
		},
		StartTime:     e.StartTime.UTC().Format(time.RFC3339Nano),
		Level:         e.Level,
		StatusMessage: e.StatusMessage,
		Metadata:      e.Metadata,
	}
	if !e.EndTime.IsZero() {
		body.EndTime = e.EndTime.UTC().Format(time.RFC3339Nano)
	}

	evt := Event{
		ID:        newEnvelopeID(),
		Type:      "generation-create",
		Timestamp: isoNow(),
		Body:      body,
	}

	i.mu.Lock()
	i.batch = append(i.batch, evt)
	i.mu.Unlock()
}

// UpdateTrace buffers a trace-create event with only the specified fields.
// The Langfuse API treats trace-create with an existing ID as an upsert,
// so only the provided fields are updated.
func (i *Ingestion) UpdateTrace(id string, tags []string) {
	body := struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags,omitempty"`
	}{ID: id, Tags: tags}

	evt := Event{
		ID:        newEnvelopeID(),
		Type:      "trace-create",
		Timestamp: isoNow(),
		Body:      body,
	}

	i.mu.Lock()
	i.batch = append(i.batch, evt)
	i.mu.Unlock()
}

// UpdateSpan buffers a span-update event that sets the end time of an existing span.
func (i *Ingestion) UpdateSpan(id string, endTime time.Time) {
	body := spanUpdateBody{
		ID:      id,
		EndTime: endTime.UTC().Format(time.RFC3339Nano),
	}

	evt := Event{
		ID:        newEnvelopeID(),
		Type:      "span-update",
		Timestamp: isoNow(),
		Body:      body,
	}

	i.mu.Lock()
	i.batch = append(i.batch, evt)
	i.mu.Unlock()
}

// PendingEvents returns a copy of the currently buffered events. Used for test inspection.
func (i *Ingestion) PendingEvents() []Event {
	i.mu.Lock()
	defer i.mu.Unlock()

	result := make([]Event, len(i.batch))
	copy(result, i.batch)
	return result
}

// batchPayload is the request body for POST /api/public/ingestion.
type batchPayload struct {
	Batch []Event `json:"batch"`
}

// Flush POSTs all buffered events to the Langfuse ingestion API in a single batch.
// If the buffer is empty, Flush returns nil without making an HTTP call.
// On success (2xx), the buffer is cleared. On error, the buffer is preserved for retry.
func (i *Ingestion) Flush(ctx context.Context) error {
	i.mu.Lock()
	if len(i.batch) == 0 {
		i.mu.Unlock()
		return nil
	}
	// Snapshot the current batch to send.
	toSend := make([]Event, len(i.batch))
	copy(toSend, i.batch)
	i.mu.Unlock()

	payload := batchPayload{Batch: toSend}

	resp, err := i.client.doRequest(ctx, http.MethodPost, "/api/public/ingestion", payload)
	if err != nil {
		return fmt.Errorf("ingestion flush: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body) // drain body

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ingestion flush: API error (status %d)", resp.StatusCode)
	}

	// Success — clear the buffer.
	i.mu.Lock()
	i.batch = i.batch[:0]
	i.mu.Unlock()

	return nil
}
