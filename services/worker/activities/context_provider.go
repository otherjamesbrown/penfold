package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ContextProvider is the interface for config-driven context providers.
// Each provider contributes one markdown section to the assembled BackgroundContext.
// Build returns an empty string (not an error) when no data is available.
// Errors are non-blocking: the provider logs a warning and returns ("", nil).
type ContextProvider interface {
	// Name returns the provider identifier used in pipeline_definitions.context_providers.
	Name() string

	// Build assembles the markdown section for this provider and returns it.
	// An empty return value means the section is omitted from the final context.
	Build(ctx context.Context, input ContextProviderInput) (string, error)
}

// ContextProviderInput carries all data that context providers may need.
// Not all fields are used by every provider.
type ContextProviderInput struct {
	TenantID          string
	SourceID          int64
	ContentID         string
	JobID             string
	ContentType       string
	ContentSubtype    string
	Content           string
	Subject           string
	SenderEmail       string
	SenderName        string
	ThreadID          string
	ParticipantEmails []workflows.Participant
	ConversationID    string
	Extraction        *workflows.SLMPipelineExtractEntitiesOutput
	// Optional: populated after entity resolution, for providers that need resolved data.
	ResolvedPeople   []workflows.ResolvedPerson
	ResolvedProjects []workflows.ResolvedProject
}

// providerRegistry maps provider name to ContextProvider implementation.
// Register new providers here. The generic context builder uses this map
// to resolve names declared in pipeline_definitions.context_providers.
var providerRegistry = map[string]ContextProvider{
	"tracked_products": &TrackedProductsProvider{},
}
