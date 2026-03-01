// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/routing"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TriageActivities holds dependencies for triage-related activities.
type TriageActivities struct {
	logger             logging.Logger
	aiClient           AIClient
	pipelineRepo       PipelineRepository
	enrichmentRepo     EnrichmentRepository
	classificationRepo ClassificationRepository // optional: rule engine for source system classification
	routingRepo        RoutingRepository        // optional: pipeline routing table lookup
}

// NewTriageActivities creates a new TriageActivities instance.
func NewTriageActivities(
	logger logging.Logger,
	aiClient AIClient,
	pipelineRepo PipelineRepository,
	enrichmentRepo EnrichmentRepository,
	classificationRepo ClassificationRepository, // optional: rule engine for source system classification
) *TriageActivities {
	if logger == nil {
		panic("NewTriageActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewTriageActivities: aiClient is required")
	}
	// pipelineRepo is optional (provenance recording)
	// enrichmentRepo is optional (for content subtype classification)
	// classificationRepo is optional (for rule-engine source system classification)
	return &TriageActivities{
		logger:             logger.With(logging.F("component", "triage_activities")),
		aiClient:           aiClient,
		pipelineRepo:       pipelineRepo,
		enrichmentRepo:     enrichmentRepo,
		classificationRepo: classificationRepo,
	}
}

// WithRoutingRepo sets the routing repository for pipeline route lookup.
func (a *TriageActivities) WithRoutingRepo(repo RoutingRepository) *TriageActivities {
	a.routingRepo = repo
	return a
}

// Triage performs Stage 1 content triage using an SLM.
// Classifies content into categories and importance levels.
func (a *TriageActivities) Triage(ctx context.Context, input workflows.TriageInput) (*workflows.TriageOutput, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "Triage"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
		logging.F("content_type", input.ContentType),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting triage")

	logger.Info("Starting content triage")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Classify content subtype BEFORE validating content (Stage 0.5 - before AI triage)
	// This allows us to handle metadata-only calendar invites with empty body
	// This runs for all content types but is most useful for emails
	var subtype enrichment.ContentSubtype
	if a.enrichmentRepo != nil && input.SourceID > 0 {
		subtype = enrichment.ClassifyContentSubtype(
			input.Headers,
			input.SenderEmail,
			input.Subject,
			nil, // TODO: Load tenant patterns from config if needed
		)

		logger.Info("Content subtype classified",
			logging.F("subtype", string(subtype)),
			logging.F("source_id", input.SourceID),
		)

		// Update the enrichment record with the classified subtype
		// This is a best-effort update; don't fail the activity if it fails
		if enrichmentRec, err := a.enrichmentRepo.GetBySourceID(ctx, input.SourceID); err == nil && enrichmentRec != nil {
			enrichmentRec.ContentSubtype = string(subtype)
			if updateErr := a.enrichmentRepo.Update(ctx, enrichmentRec); updateErr != nil {
				logger.Warn("Failed to update enrichment subtype",
					logging.Err(updateErr),
					logging.F("source_id", input.SourceID),
				)
			} else {
				logger.Info("Updated enrichment subtype",
					logging.F("source_id", input.SourceID),
					logging.F("subtype", string(subtype)),
				)
			}
		} else if err != nil {
			logger.Warn("Failed to fetch enrichment for subtype update",
				logging.Err(err),
				logging.F("source_id", input.SourceID),
			)
		}
	}

	// Classify source system using rule engine (replaces ClassifySourceSystem in pipeline.go)
	var sourceSystem string
	var routingContentType, routingSubtype string
	var matchedPipelines []string
	var ruleEngineSubtype string // pf-2d512a: capture rule engine subtype for override

	if a.classificationRepo != nil {
		rules, err := a.classificationRepo.LoadRules(ctx, input.TenantID)
		if err != nil {
			logger.Warn("Failed to load classification rules, falling back to default",
				logging.Err(err),
			)
			sourceSystem = string(enrichment.SourceSystemHumanEmail)
		} else {
			engine := classification.NewEngine(rules)
			// Build metadata map from available inputs
			metadata := map[string]string{
				"from_address": input.SenderEmail,
				"subject":      input.Subject,
			}
			// Add headers (header:Name format)
			for k, v := range input.Headers {
				metadata["header:"+k] = v
			}
			result := engine.Classify(input.ContentType, metadata)
			sourceSystem = mapToSourceSystem(result)
			routingContentType = result.ContentType
			routingSubtype = result.ContentSubtype
			ruleEngineSubtype = result.ContentSubtype // pf-2d512a: always use rule engine taxonomy
			logger.Info("Source system classified via rule engine",
				logging.F("source_system", sourceSystem),
				logging.F("rule_name", result.RuleName),
				logging.F("content_subtype", result.ContentSubtype),
			)
		}
	} else {
		sourceSystem = string(enrichment.SourceSystemHumanEmail)
	}

	// Deterministic newsletter detection: self-addressed blast (pf-199f31).
	// If To header address equals From address, it's a self-addressed newsletter blast
	// (e.g. "Akamai Wave <AkamaiWave@akamai.com>" sending to itself).
	// Only applies when rule engine defaulted to HUMAN (no specific rule matched).
	if routingSubtype == "HUMAN" && input.SenderEmail != "" && len(input.Headers) > 0 {
		if toAddr := extractEmailFromHeader(input.Headers["To"]); toAddr != "" {
			if strings.EqualFold(toAddr, input.SenderEmail) {
				routingContentType = "EMAIL"
				routingSubtype = "NEWSLETTER"
				ruleEngineSubtype = "NEWSLETTER"
				logger.Info("Newsletter detected: To == From (self-addressed blast)",
					logging.F("from", input.SenderEmail),
					logging.F("to", toAddr),
				)
			}
		}
	}

	// Override content_subtype from rule engine (pf-2d512a).
	// The rule engine taxonomy (HUMAN, NEWSLETTER, NOTIFICATION, etc.) is more authoritative
	// than the heuristic classifier (standalone, forward, etc.). When the rule engine runs,
	// always use its result — including the default HUMAN for unmatched items.
	if ruleEngineSubtype != "" {
		subtype = enrichment.ContentSubtype(ruleEngineSubtype)
		logger.Info("Content subtype overridden by rule engine",
			logging.F("subtype", string(subtype)),
		)

		// Update the enrichment record with the rule engine's subtype
		if a.enrichmentRepo != nil && input.SourceID > 0 {
			if enrichmentRec, err := a.enrichmentRepo.GetBySourceID(ctx, input.SourceID); err == nil && enrichmentRec != nil {
				enrichmentRec.ContentSubtype = string(subtype)
				if updateErr := a.enrichmentRepo.Update(ctx, enrichmentRec); updateErr != nil {
					logger.Warn("Failed to update enrichment subtype from rule engine",
						logging.Err(updateErr),
					)
				}
			}
		}
	}

	// Derive routing keys for non-email content where rule engine defaults to EMAIL/HUMAN.
	if routingContentType == "" {
		routingContentType = strings.ToUpper(input.ContentType)
	}
	if routingContentType == "EMAIL" && routingSubtype == "HUMAN" && !strings.EqualFold(input.ContentType, "email") {
		routingContentType = strings.ToUpper(input.ContentType)
		switch routingContentType {
		case "MEETING":
			routingSubtype = "TRANSCRIPT"
		}
	}
	if routingSubtype == "" {
		routingSubtype = "HUMAN"
	}

	// Pipeline routing: look up which pipelines this item enters.
	if a.routingRepo != nil {
		routes, err := a.routingRepo.LoadRoutes(ctx, input.TenantID)
		if err != nil {
			logger.Warn("Failed to load routing rules",
				logging.Err(err),
			)
		} else {
			router := routing.NewRouter(routes)
			matchedPipelines = router.Resolve(routingContentType, routingSubtype)
			logger.Info("Pipeline routing resolved",
				logging.F("routing_content_type", routingContentType),
				logging.F("routing_subtype", routingSubtype),
				logging.F("pipelines", matchedPipelines),
				logging.F("route_count", len(matchedPipelines)),
			)
		}
	}

	// Handle auto-reply emails (pf-64dcc4)
	// Auto-replies are system-generated messages with no meaningful content contribution.
	// Detect by subject prefix "automatic reply:" (case-insensitive), matching the same
	// heuristic used by ClassifySourceSystem in pkg/enrichment/classification/source_system.go.
	// Short-circuit before the AI call to avoid the LLM mis-classifying OOO body text.
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.Subject)), "automatic reply:") {
		logger.Info("Auto-reply email detected, skipping AI triage",
			logging.F("subject", input.Subject),
		)

		output := &workflows.TriageOutput{
			Category:            "INTERNAL_COMMS",
			Importance:          "LOW",
			Reason:              "Auto-reply email (subject prefix: 'Automatic reply:')",
			Confidence:          1.0,
			ModelUsed:           "subject-classifier",
			SkipDeep:            true,
			ContentSubtype:      string(subtype),
			ContentContribution: "NONE",
			ContributionReason:  "Auto-reply emails do not contribute meaningful content",
			SourceSystem:        sourceSystem,
			RoutingContentType:  routingContentType,
			RoutingSubtype:      routingSubtype,
			Pipelines:           matchedPipelines,
		}

		logger.Info("Triage completed (auto-reply short-circuit)",
			logging.F("category", output.Category),
			logging.F("importance", output.Importance),
			logging.F("content_contribution", output.ContentContribution),
			logging.F("skip_deep", output.SkipDeep),
			logging.F("pipelines", matchedPipelines),
		)

		// Record pipeline run for provenance tracking (pf-91b00d).
		// Without this record, BuildProcessingStatus reads the previous run's parsed_data
		// (which may show MEDIUM from a pre-fix ingestion) rather than the current NONE result.
		if a.pipelineRepo != nil {
			parsedJSON, _ := json.Marshal(output)
			inputJSON, _ := json.Marshal(map[string]interface{}{
				"content_length": len(input.Content),
				"content_type":   input.ContentType,
				"has_subject":    input.Subject != "",
				"has_sender":     input.SenderEmail != "",
				"tenant_id":      input.TenantID,
			})
			outputJSON, _ := json.Marshal(map[string]interface{}{
				"category":   output.Category,
				"importance": output.Importance,
				"model_used": output.ModelUsed,
			})
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:   input.SourceID,
				Stage:      "triage",
				ModelID:    output.ModelUsed,
				Status:     "completed",
				DurationMS: 0, // No AI call; no measurable duration
				InputData:  inputJSON,
				OutputData: outputJSON,
				ParsedData: parsedJSON,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for auto-reply short-circuit", logging.Err(runErr))
				// Don't fail the activity if run recording fails
			}
		}

		return output, nil
	}

	// Handle meeting transcripts (pf-025e10)
	// Meetings are inherently high-value — someone scheduled, recorded, and transcribed them.
	// The 500-char triage truncation captures only opening greetings/small-talk, causing the
	// LLM to misclassify as PERSONAL/LOW. Short-circuit with PROJECT_UPDATE/HIGH so all
	// extraction stages run. The extraction stages will determine actual content and entities.
	if strings.EqualFold(input.ContentType, "meeting") {
		logger.Info("Meeting transcript detected, skipping AI triage",
			logging.F("content_type", input.ContentType),
			logging.F("content_length", len(input.Content)),
		)

		output := &workflows.TriageOutput{
			Category:            "PROJECT_UPDATE",
			Importance:          "HIGH",
			Reason:              "Meeting transcript (auto-classified: meetings are inherently high-value)",
			Confidence:          1.0,
			ModelUsed:           "content-type-classifier",
			SkipDeep:            false,
			ContentSubtype:      string(subtype),
			ContentContribution: "HIGH",
			ContributionReason:  "Meeting transcripts contain substantive discussion requiring full extraction",
			SourceSystem:        sourceSystem,
			RoutingContentType:  routingContentType,
			RoutingSubtype:      routingSubtype,
			Pipelines:           matchedPipelines,
		}

		logger.Info("Triage completed (meeting short-circuit)",
			logging.F("category", output.Category),
			logging.F("importance", output.Importance),
			logging.F("content_contribution", output.ContentContribution),
			logging.F("skip_deep", output.SkipDeep),
			logging.F("pipelines", matchedPipelines),
		)

		// Record pipeline run for provenance tracking
		if a.pipelineRepo != nil {
			parsedJSON, _ := json.Marshal(output)
			inputJSON, _ := json.Marshal(map[string]interface{}{
				"content_length": len(input.Content),
				"content_type":   input.ContentType,
				"has_subject":    input.Subject != "",
				"has_sender":     input.SenderEmail != "",
				"tenant_id":      input.TenantID,
			})
			outputJSON, _ := json.Marshal(map[string]interface{}{
				"category":   output.Category,
				"importance": output.Importance,
				"model_used": output.ModelUsed,
			})
			runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
				SourceID:   input.SourceID,
				Stage:      "triage",
				ModelID:    output.ModelUsed,
				Status:     "completed",
				DurationMS: 0,
				InputData:  inputJSON,
				OutputData: outputJSON,
				ParsedData: parsedJSON,
			})
			if runErr != nil {
				logger.Warn("Failed to record pipeline run for meeting short-circuit", logging.Err(runErr))
			}
		}

		return output, nil
	}

	// Handle calendar invites with empty body (pf-479452)
	// Calendar metadata (organizer, attendees, title, date) is extracted later in Stage 3,
	// so we can skip AI triage and return a metadata-only result
	if input.Content == "" && subtype.IsCalendar() {
		logger.Info("Calendar invite with empty body detected, skipping AI triage",
			logging.F("subtype", string(subtype)),
		)

		// Return metadata-only result without calling AI
		output := &workflows.TriageOutput{
			Category:           "MEETING",
			Importance:         "MEDIUM",
			Reason:             "Calendar invite with no body text (metadata-only)",
			Confidence:         1.0, // High confidence - deterministic classification
			ModelUsed:          "metadata-classifier",
			SkipDeep:           true, // Skip deep processing (Stages 2-4)
			ContentSubtype:     string(subtype),
			SourceSystem:       sourceSystem,
			RoutingContentType: routingContentType,
			RoutingSubtype:     routingSubtype,
			Pipelines:          matchedPipelines,
		}

		logger.Info("Triage completed (metadata-only)",
			logging.F("category", output.Category),
			logging.F("importance", output.Importance),
			logging.F("skip_deep", output.SkipDeep),
			logging.F("pipelines", matchedPipelines),
		)

		return output, nil
	}

	// Validate input - only fail if content is empty and it's NOT a calendar
	if input.Content == "" {
		return nil, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn("AI client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	startTime := time.Now()

	// Record heartbeat before calling AI service
	recordHeartbeat(ctx, "calling AI service for triage")

	// Inject pipeline OTel span context so stage spans nest under the pipeline trace.
	if input.PipelineSpanID != "" && input.LangfuseTraceID != "" {
		otelTraceID := strings.ReplaceAll(input.LangfuseTraceID, "-", "")
		ctx = tracing.ContextWithPipelineSpan(ctx, otelTraceID, input.PipelineSpanID)
	}

	// Create stage span wrapping the gRPC call
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.triage", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Build TriageContentRequest.
	// Prepend To/CC headers to content so the LLM has recipient context (pf-199f31).
	triageContent := input.Content
	if len(input.Headers) > 0 {
		var headerLines []string
		if to := input.Headers["To"]; to != "" {
			headerLines = append(headerLines, "To: "+to)
		}
		if cc := input.Headers["Cc"]; cc != "" {
			headerLines = append(headerLines, "CC: "+cc)
		}
		if len(headerLines) > 0 {
			triageContent = strings.Join(headerLines, "\n") + "\n\n" + triageContent
		}
	}
	req := &aiv1.TriageContentRequest{
		Content: triageContent,
	}
	if input.Subject != "" {
		req.Subject = &input.Subject
	}
	if input.SenderEmail != "" {
		req.Sender = &input.SenderEmail
	}
	if input.TenantID != "" {
		req.TenantId = &input.TenantID
	}
	if input.SourceID > 0 {
		req.SourceId = &input.SourceID
	}
	if input.ContentID != "" {
		req.ContentId = &input.ContentID
	}
	// Pipeline span context is injected above; gRPC OTel interceptors propagate traceparent automatically.

	// Attach Langfuse tracing metadata for AI coordinator to use when creating generation spans.
	callCtx := stageCtx
	if input.LangfuseTraceID != "" {
		md := metadata.Pairs(
			"x-langfuse-trace-id", input.LangfuseTraceID,
			"x-langfuse-phase-id", input.LangfusePhaseID,
		)
		callCtx = metadata.NewOutgoingContext(stageCtx, md)
	}

	// Call AI service with stage span context
	resp, err := a.aiClient.TriageContent(callCtx, req)
	if err != nil {
		pe := perrors.ClassifyError(err, "triage")
		logger.Error("Failed to perform triage", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	// Extract content_contribution from proto response (with default)
	contentContribution := ""
	if resp.ContentContribution != nil {
		contentContribution = *resp.ContentContribution
	}
	// Default to HIGH if not provided (never skip by accident)
	if contentContribution == "" {
		contentContribution = "HIGH"
	}

	contributionReason := ""
	if resp.ContributionReason != nil {
		contributionReason = *resp.ContributionReason
	}

	// Determine if we should skip deep processing (Stage 2-4)
	skipDeep := shouldSkipDeep(resp.Category, resp.Importance)

	// Convert proto response to domain output
	output := &workflows.TriageOutput{
		Category:            resp.Category,
		Importance:          resp.Importance,
		Reason:              resp.Reason,
		Confidence:          0.85, // Default confidence - can be enhanced later
		ModelUsed:           resp.ModelUsed,
		SkipDeep:            skipDeep,
		ContentSubtype:      string(subtype),
		ContentContribution: contentContribution,
		ContributionReason:  contributionReason,
		SourceSystem:        sourceSystem,
		RoutingContentType:  routingContentType,
		RoutingSubtype:      routingSubtype,
		Pipelines:           matchedPipelines,
	}

	// Cap ContentContribution for notification subtypes (pf-bcb565).
	// Notifications (Jira, Google Docs, Aha, Slack, etc.) don't need deep_analyze —
	// extraction captures entity names, but deep analysis wastes expensive model capacity.
	// Cap at LOW so extraction runs but deep_analyze is skipped.
	if subtype.IsNotification() && contributionAbove(output.ContentContribution, "LOW") {
		logger.Info("Capping contribution for notification content",
			logging.F("original_contribution", output.ContentContribution),
			logging.F("capped_to", "LOW"),
			logging.F("content_subtype", string(subtype)),
		)
		output.ContentContribution = "LOW"
		output.ContributionReason = fmt.Sprintf("Notification content capped from %s to LOW (pf-bcb565)", contentContribution)
	}

	// Record heartbeat after processing
	recordHeartbeat(ctx, "triage complete")

	logger.Info("Triage completed successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("category", output.Category),
		logging.F("importance", output.Importance),
		logging.F("content_contribution", output.ContentContribution),
		logging.F("skip_deep", output.SkipDeep),
		logging.F("model", output.ModelUsed),
	)

	// Record pipeline run for provenance tracking
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		// Capture IO data
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length": len(input.Content),
			"content_type": input.ContentType,
			"has_subject": input.Subject != "",
			"has_sender": input.SenderEmail != "",
			"tenant_id": input.TenantID,
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"category": resp.Category,
			"importance": resp.Importance,
			"model_used": resp.ModelUsed,
		})
		parsedJSON, _ := json.Marshal(output)

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:        input.SourceID,
			Stage:           "triage",
			ModelID:         output.ModelUsed,
			PromptVersion:   int(resp.GetPromptVersion()),
			Status:          "completed",
			DurationMS:      durationMS,
			InputData:       inputJSON,
			OutputData:      outputJSON,
			ParsedData:      parsedJSON,
			InputTokens:     int(resp.GetInputTokens()),
			OutputTokens:    int(resp.GetOutputTokens()),
			LangfuseTraceID: input.LangfuseTraceID,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
			// Don't fail the activity if run recording fails
		}
	}

	return output, nil
}

// shouldSkipDeep determines if deep processing (stages 2-4) should be skipped
// based on the triage results.
// Skip when:
// - Category is PERSONAL (any importance)
// - Category is INTERNAL_COMMS and Importance is LOW
func shouldSkipDeep(category, importance string) bool {
	if category == "PERSONAL" {
		return true
	}
	if category == "INTERNAL_COMMS" && importance == "LOW" {
		return true
	}
	return false
}

// contributionAbove returns true if contribution is strictly above the threshold.
// Ordering: NONE < LOW < MEDIUM < HIGH.
func contributionAbove(contribution, threshold string) bool {
	order := map[string]int{"NONE": 0, "LOW": 1, "MEDIUM": 2, "HIGH": 3}
	return order[contribution] > order[threshold]
}

// mapToSourceSystem converts a ClassificationResult to the legacy SourceSystem string.
// This bridges the rule engine output to the enrichment.SourceSystem constants.
func mapToSourceSystem(result classification.ClassificationResult) string {
	switch result.ContentSubtype {
	case "NOTIFICATION":
		if result.NotificationSource != "" {
			return result.NotificationSource // "jira", "aha", etc.
		}
		return string(enrichment.SourceSystemHumanEmail)
	case "AUTO_REPLY":
		return string(enrichment.SourceSystemAutoReply)
	case "CANCELLATION", "INVITE":
		return string(enrichment.SourceSystemOutlookCalendar)
	default:
		return string(enrichment.SourceSystemHumanEmail)
	}
}

// extractEmailFromHeader extracts a bare email address from a header value.
// Handles formats like "Name <user@example.com>" and "user@example.com".
func extractEmailFromHeader(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	// Check for angle bracket format: "Name <user@example.com>"
	if start := strings.LastIndex(header, "<"); start != -1 {
		if end := strings.Index(header[start:], ">"); end != -1 {
			return strings.TrimSpace(header[start+1 : start+end])
		}
	}
	// Bare email: "user@example.com"
	if strings.Contains(header, "@") {
		// Take first token if comma-separated
		if idx := strings.Index(header, ","); idx != -1 {
			header = header[:idx]
		}
		return strings.TrimSpace(header)
	}
	return ""
}

// Ensure TriageActivities implements required interfaces at compile time.
var _ interface {
	Triage(ctx context.Context, input workflows.TriageInput) (*workflows.TriageOutput, error)
} = (*TriageActivities)(nil)
