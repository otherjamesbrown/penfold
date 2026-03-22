package temporal

import (
	"testing"
)

func TestAllMainQueueActivities(t *testing.T) {
	activities := AllMainQueueActivities()

	// Count updated to reflect all activities including graph, digest (daily/weekly/journal), rollup, newsletter, structured extract, etc.
	expectedCount := 80
	if len(activities) != expectedCount {
		t.Errorf("Expected %d main queue activities, got %d", expectedCount, len(activities))
	}

	// Test expected activities are present
	expectedActivities := []string{
		"ValidateContent",
		"FetchContent",
		"UpdateContentStatus",
		"GenerateContentEmbedding",
		"DeleteEmbedding",
		"GenerateContentSummary",
		"DeleteSummary",
		"ExtractEntitiesActivity",
		"ExtractAssertions",
		"ExtractMentions",
		"TagProjects",
		"ParseEmail",
		"ParseTranscript",
		"PersistFindings",
		"Triage",
		"BuildContextPackage",
		"DeepAnalyze",
		"RecordOverrides",
		"EnrichPersonMetadata",
		"GroupEmailThread",
		"CreateEnrichmentRecord",
		"LinkConversation",
		"BackfillConversationSummaries",
		"RegenerateConversationSummary",
		"CheckStaleConversations",
		// Langfuse direct ingestion activities
		// Note: ReportLangfuseGeneration was removed in pf-37ebe8.
		// Generation reporting now happens in the AI coordinator via gRPC metadata.
		"CreateLangfuseTrace",
		"ReportLangfusePhase",
		"FinishLangfuseTrace",
		"PersistLangfuseTraceID",
		"UpdateLangfuseTraceTags",
		"UpdateLangfuseTraceMetadata",
		// Auto-drain activity
		"KickNextPending",
		// Skipped stage provenance recording
		"RecordSkippedStage",
		// Assertion cleanup for reprocessing
		"DeleteAssertions",
		// Pipeline definition lookup
		"FetchPipelineDefinition",
		// Pre-extraction context (glossary + topics) for semantic extraction
		"BuildExtractionContext",
		// Session ledger consolidation
		"ConsolidateEntries",
		// Newsletter enrichment activities
		"NewsletterExtract",
		"PersistExtractedData",
		// Notification enrichment activities
		"NotificationExtract",
		// Generic structured extraction (replaces bespoke newsletter/notification extract)
		"StructuredExtract",
		// Newsletter context builder (enriches extraction context for newsletter_extract stages)
		"BuildNewsletterContext",
	}

	activityMap := make(map[string]bool)
	for _, activity := range activities {
		activityMap[activity] = true
	}

	for _, expected := range expectedActivities {
		if !activityMap[expected] {
			t.Errorf("Expected activity %q not found in main queue activities", expected)
		}
	}

	// Test no duplicates
	if len(activityMap) != len(activities) {
		t.Errorf("Duplicate activities found in main queue activities")
	}
}

func TestAllAIQueueActivities(t *testing.T) {
	activities := AllAIQueueActivities()

	// Test count (7 activities: 4 unique + 3 shared with main)
	expectedCount := 7
	if len(activities) != expectedCount {
		t.Errorf("Expected %d AI queue activities, got %d", expectedCount, len(activities))
	}

	// Test expected activities are present
	expectedActivities := []string{
		"GenerateEmbedding",
		"GenerateEmbeddingBatch",
		"GenerateSummary",
		"GenerateSummaryWithOptions",
		"ExtractAssertions",
		"ExtractEntitiesActivity",
		"ExtractMentions",
	}

	activityMap := make(map[string]bool)
	for _, activity := range activities {
		activityMap[activity] = true
	}

	for _, expected := range expectedActivities {
		if !activityMap[expected] {
			t.Errorf("Expected activity %q not found in AI queue activities", expected)
		}
	}

	// Test no duplicates
	if len(activityMap) != len(activities) {
		t.Errorf("Duplicate activities found in AI queue activities")
	}
}

func TestAllEmailQueueActivities(t *testing.T) {
	activities := AllEmailQueueActivities()

	// Count updated to reflect all activities including graph API activities
	expectedCount := 17
	if len(activities) != expectedCount {
		t.Errorf("Expected %d email queue activities, got %d", expectedCount, len(activities))
	}

	// Test expected activities are present
	expectedActivities := []string{
		"FetchSource",
		"UpdateSourceStatus",
		"ParseEmail",
	}

	activityMap := make(map[string]bool)
	for _, activity := range activities {
		activityMap[activity] = true
	}

	for _, expected := range expectedActivities {
		if !activityMap[expected] {
			t.Errorf("Expected activity %q not found in email queue activities", expected)
		}
	}

	// Test no duplicates
	if len(activityMap) != len(activities) {
		t.Errorf("Duplicate activities found in email queue activities")
	}
}

func TestActivityConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"ValidateContent", ActivityValidateContent, "ValidateContent"},
		{"FetchContent", ActivityFetchContent, "FetchContent"},
		{"UpdateContentStatus", ActivityUpdateContentStatus, "UpdateContentStatus"},
		{"GenerateContentEmbedding", ActivityGenerateContentEmbedding, "GenerateContentEmbedding"},
		{"DeleteEmbedding", ActivityDeleteEmbedding, "DeleteEmbedding"},
		{"GenerateContentSummary", ActivityGenerateContentSummary, "GenerateContentSummary"},
		{"DeleteSummary", ActivityDeleteSummary, "DeleteSummary"},
		{"ExtractEntitiesActivity", ActivityExtractEntitiesActivity, "ExtractEntitiesActivity"},
		{"ExtractAssertions", ActivityExtractAssertions, "ExtractAssertions"},
		{"ExtractMentions", ActivityExtractMentions, "ExtractMentions"},
		{"ParseEmail", ActivityParseEmail, "ParseEmail"},
		{"ParseTranscript", ActivityParseTranscript, "ParseTranscript"},
		{"PersistFindings", ActivityPersistFindings, "PersistFindings"},
		{"Triage", ActivityTriage, "Triage"},
		{"BuildContextPackage", ActivityBuildContextPackage, "BuildContextPackage"},
		{"DeepAnalyze", ActivityDeepAnalyze, "DeepAnalyze"},
		{"GenerateEmbedding", ActivityGenerateEmbedding, "GenerateEmbedding"},
		{"GenerateEmbeddingBatch", ActivityGenerateEmbeddingBatch, "GenerateEmbeddingBatch"},
		{"GenerateSummary", ActivityGenerateSummary, "GenerateSummary"},
		{"GenerateSummaryWithOptions", ActivityGenerateSummaryWithOptions, "GenerateSummaryWithOptions"},
		{"FetchSource", ActivityFetchSource, "FetchSource"},
		{"UpdateSourceStatus", ActivityUpdateSourceStatus, "UpdateSourceStatus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}
