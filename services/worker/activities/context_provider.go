package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/logging"
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

// providerRegistry maps provider name to ContextProvider implementation.
// Populated by RegisterContextProviders; the generic context builder uses this map
// to resolve names declared in pipeline_definitions.context_providers.
var providerRegistry = map[string]ContextProvider{}

// LookupProvider returns the ContextProvider registered under name, and whether it was found.
func LookupProvider(name string) (ContextProvider, bool) {
	p, ok := providerRegistry[name]
	return p, ok
}

// RegisterContextProviders initializes and registers all built-in context providers.
// Called from NewContextBuilderActivities and updated when optional repos are wired in.
// Providers with a nil repo return "" gracefully — they are always registered.
func RegisterContextProviders(
	logger logging.Logger,
	contextRepo ContextPackageRepository,
	newsletterRepo NewsletterContextRepository,
	topicRepo TopicLookupInterface,
	tenantContextRepo TenantContextRepository,
	operConfig OperationalConfigReader,
) {
	providerRegistry["user_context"] = NewUserContextProvider(newsletterRepo, logger)
	providerRegistry["glossary"] = NewGlossaryProvider(contextRepo, logger)
	providerRegistry["active_projects"] = NewActiveProjectsProvider(newsletterRepo, logger)
	providerRegistry["topics"] = NewTopicsProvider(topicRepo, logger)
	providerRegistry["tracked_products"] = NewTrackedProductsProvider(newsletterRepo, logger)
	providerRegistry["tenant_context"] = NewTenantContextProvider(tenantContextRepo, operConfig, logger)
}
