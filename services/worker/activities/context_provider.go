package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ContextProvider is the interface for pluggable context providers.
// Each provider produces a markdown section for inclusion in a prompt's background context.
type ContextProvider interface {
	Name() string
	Build(ctx context.Context, input ContextProviderInput) (string, error)
}

// ContextProviderInput carries all fields a provider might need.
// It mirrors BuildContextInput and adds optional resolved entity outputs
// for providers that consume entity resolution results.
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

	// ResolvedPeople and ResolvedProjects are populated by BuildContextPackage
	// after entity resolution. Providers that need them (e.g. entity_mentions)
	// receive them here rather than re-resolving themselves.
	ResolvedPeople   []workflows.ResolvedPerson
	ResolvedProjects []workflows.ResolvedProject
}

// providerRegistry maps provider names to their implementations.
// Providers are registered here as they are implemented.
var providerRegistry = map[string]ContextProvider{}

// LookupProvider returns the provider registered under the given name.
func LookupProvider(name string) (ContextProvider, bool) {
	p, ok := providerRegistry[name]
	return p, ok
}
