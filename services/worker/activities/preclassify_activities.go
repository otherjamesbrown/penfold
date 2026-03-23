// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/routing"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// PreClassifyActivities holds dependencies for pre-triage classification (pf-b375ad).
// Runs the DB-backed rule engine against sender metadata before triage, in shadow
// mode alongside looksLikeNotificationSender(). No AI client needed.
type PreClassifyActivities struct {
	logger             logging.Logger
	classificationRepo ClassificationRepository
	routingRepo        RoutingRepository
}

// NewPreClassifyActivities creates a new PreClassifyActivities instance.
func NewPreClassifyActivities(
	logger logging.Logger,
	classificationRepo ClassificationRepository,
) *PreClassifyActivities {
	return &PreClassifyActivities{
		logger:             logger.With(logging.F("component", "preclassify_activities")),
		classificationRepo: classificationRepo,
	}
}

// WithRoutingRepo sets the routing repository for pipeline route lookup.
func (a *PreClassifyActivities) WithRoutingRepo(repo RoutingRepository) *PreClassifyActivities {
	a.routingRepo = repo
	return a
}

// PreClassify runs the classification rule engine against sender metadata
// and returns the classification result. Used in shadow mode (pf-b375ad) to
// compare against looksLikeNotificationSender() before switching over.
func (a *PreClassifyActivities) PreClassify(ctx context.Context, input workflows.PreClassifyInput) (*workflows.PreClassifyOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "PreClassify"),
		logging.F("tenant_id", input.TenantID),
		logging.F("sender_email", input.SenderEmail),
	)

	rules, err := a.classificationRepo.LoadRules(ctx, input.TenantID)
	if err != nil {
		logger.Warn("Failed to load classification rules for pre-classify",
			logging.Err(err),
		)
		return &workflows.PreClassifyOutput{
			Error: err.Error(),
		}, nil
	}

	engine := classification.NewEngine(rules)

	// Build metadata map (same pattern as triage_activities.go).
	metadata := map[string]string{
		"from_address": input.SenderEmail,
		"subject":      input.Subject,
	}
	for k, v := range input.Headers {
		metadata["header:"+k] = v
	}

	result := engine.Classify(input.ContentType, metadata)

	out := &workflows.PreClassifyOutput{
		Matched:        result.RuleName != "",
		ContentSubtype: result.ContentSubtype,
		RuleName:       result.RuleName,
	}

	// Resolve pipeline name from routing table if available.
	if a.routingRepo != nil && result.ContentType != "" {
		routes, err := a.routingRepo.LoadRoutes(ctx, input.TenantID)
		if err != nil {
			logger.Warn("Failed to load routing rules for pre-classify",
				logging.Err(err),
			)
		} else {
			router := routing.NewRouter(routes)
			pipelines := router.Resolve(result.ContentType, result.ContentSubtype)
			if len(pipelines) > 0 {
				out.PipelineName = pipelines[0]
			}
		}
	}

	logger.Info("Pre-classify rule engine result",
		logging.F("matched", out.Matched),
		logging.F("content_subtype", out.ContentSubtype),
		logging.F("rule_name", out.RuleName),
		logging.F("pipeline_name", out.PipelineName),
	)

	return out, nil
}
