// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"regexp"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	sourcemappings "github.com/otherjamesbrown/penfold/pkg/source_mappings"
)

// AttributionInput is the input for the AttributeProject activity.
type AttributionInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	Subject  string `json:"subject,omitempty"`
	BodyText string `json:"body_text,omitempty"`
}

// AttributionOutput is the output from the AttributeProject activity.
type AttributionOutput struct {
	AssertionsAttributed int     `json:"assertions_attributed"`
	ProjectsMatched      int     `json:"projects_matched"`
	AttributionSource    string  `json:"attribution_source"`
	AttributedProjectIDs []int64 `json:"attributed_project_ids"`
}

// AttributionRepository defines the interface for the attribution activity's data access.
type AttributionRepository interface {
	FindMappingForSource(ctx context.Context, tenantID, sourceType, sourceIdentifier string) (*sourcemappings.SourceMapping, error)
	GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error)
	GetSourceMetadata(ctx context.Context, sourceID int64) (sourceSystem string, sourceTag string, err error)
	GetAssertionsForSource(ctx context.Context, tenantID string, sourceID int64) ([]AssertionRef, error)
	UpdateAssertionAttribution(ctx context.Context, assertionID int64, projectID int64, source string, confidence float64) error
	UpdateSourceAttributedProjects(ctx context.Context, sourceID int64, projectIDs []int64) error
	GetMinConfidence(ctx context.Context, tenantID string) (float64, error)
	CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error
}

// AssertionRef holds minimal assertion data for attribution.
type AssertionRef struct {
	ID          int64
	Description string
	SourceQuote string
}

// projectMatch pairs a project with its best keyword match and confidence.
type projectMatch struct {
	project    *entities.Project
	keyword    string
	confidence float64
}

// AttributionActivities holds dependencies for the attribute_project activity.
type AttributionActivities struct {
	logger logging.Logger
	repo   AttributionRepository
}

// NewAttributionActivities creates a new AttributionActivities instance.
func NewAttributionActivities(logger logging.Logger, repo AttributionRepository) *AttributionActivities {
	if logger == nil {
		panic("NewAttributionActivities: logger is required")
	}
	if repo == nil {
		panic("NewAttributionActivities: repo is required")
	}
	return &AttributionActivities{
		logger: logger.With(logging.F("component", "attribution_activities")),
		repo:   repo,
	}
}

// AttributeProject attributes assertions to projects using a confidence-ordered signal stack:
// 1. Channel mapping (0.95) — from project_source_mappings
// 2. Keyword matching (0.75–0.85) — from projects.keywords
func (a *AttributionActivities) AttributeProject(ctx context.Context, input AttributionInput) (*AttributionOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "AttributeProject"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	recordHeartbeat(ctx, "starting project attribution")
	logger.Info("Starting project attribution")

	minConfidence, err := a.repo.GetMinConfidence(ctx, input.TenantID)
	if err != nil {
		logger.Warn("Failed to read min confidence, using default 0.7", logging.Err(err))
		minConfidence = 0.7
	}

	assertions, err := a.repo.GetAssertionsForSource(ctx, input.TenantID, input.SourceID)
	if err != nil {
		pe := perrors.ClassifyError(err, "attribute_project")
		logger.Error("Failed to get assertions", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	if len(assertions) == 0 {
		logger.Info("No assertions found for source, skipping attribution")
		return &AttributionOutput{}, nil
	}

	logger.Info("Found assertions for attribution", logging.F("assertion_count", len(assertions)))

	attributedProjectSet := make(map[int64]bool)
	totalAttributed := 0
	attributionSource := ""

	// === Signal 1: Channel mapping ===
	recordHeartbeat(ctx, "checking channel mapping")
	sourceSystem, sourceTag, err := a.repo.GetSourceMetadata(ctx, input.SourceID)
	if err != nil {
		logger.Warn("Failed to get source metadata", logging.Err(err))
	}

	if sourceTag != "" {
		sourceType := "channel"
		if sourceSystem != "" {
			sourceType = sourceSystem
		}

		mapping, err := a.repo.FindMappingForSource(ctx, input.TenantID, sourceType, sourceTag)
		if err != nil {
			logger.Warn("Failed to check channel mapping", logging.Err(err))
		}

		if mapping != nil && float64(mapping.Confidence) >= minConfidence {
			logger.Info("Channel mapping match found",
				logging.F("project_id", mapping.ProjectID),
				logging.F("confidence", mapping.Confidence),
			)

			for _, assertion := range assertions {
				if err := a.repo.UpdateAssertionAttribution(ctx, assertion.ID, mapping.ProjectID, "channel_mapping", float64(mapping.Confidence)); err != nil {
					logger.Warn("Failed to update assertion attribution",
						logging.F("assertion_id", assertion.ID),
						logging.Err(err))
					continue
				}
				totalAttributed++
			}
			attributedProjectSet[mapping.ProjectID] = true
			attributionSource = "channel_mapping"

			_ = a.repo.CreateContentMention(ctx, input.TenantID, input.SourceID, "project", sourceTag, mapping.ProjectID)
		}
	}

	// === Signal 2: Keyword matching (if channel mapping didn't cover everything) ===
	if totalAttributed == 0 {
		recordHeartbeat(ctx, "checking keyword matches")

		projects, err := a.repo.GetProjectsWithKeywords(ctx, input.TenantID)
		if err != nil {
			pe := perrors.ClassifyError(err, "attribute_project")
			logger.Error("Failed to get projects with keywords", logging.Err(pe))
			return nil, WrapForTemporal(pe)
		}

		if len(projects) > 0 {
			contentText := input.Subject + " " + input.BodyText

			var matchedProjects []projectMatch
			for _, project := range projects {
				for _, keyword := range project.Keywords {
					if matchesKeywordInText(contentText, keyword) {
						conf := 0.75
						if matchesKeywordInText(input.Subject, keyword) {
							conf = 0.85
						}
						if conf >= minConfidence {
							matchedProjects = append(matchedProjects, projectMatch{
								project:    project,
								keyword:    keyword,
								confidence: conf,
							})
							break
						}
					}
				}
			}

			if len(matchedProjects) > 0 {
				logger.Info("Keyword matches found", logging.F("matched_projects", len(matchedProjects)))

				if len(matchedProjects) == 1 {
					pm := matchedProjects[0]
					for _, assertion := range assertions {
						if err := a.repo.UpdateAssertionAttribution(ctx, assertion.ID, pm.project.ID, "keyword", pm.confidence); err != nil {
							logger.Warn("Failed to update assertion attribution",
								logging.F("assertion_id", assertion.ID),
								logging.Err(err))
							continue
						}
						totalAttributed++
					}
					attributedProjectSet[pm.project.ID] = true
					_ = a.repo.CreateContentMention(ctx, input.TenantID, input.SourceID, "project", pm.keyword, pm.project.ID)
				} else {
					totalAttributed += a.distributeAssertions(ctx, logger, assertions, matchedProjects)
					for _, pm := range matchedProjects {
						attributedProjectSet[pm.project.ID] = true
						_ = a.repo.CreateContentMention(ctx, input.TenantID, input.SourceID, "project", pm.keyword, pm.project.ID)
					}
				}
				attributionSource = "keyword"
			}
		}
	}

	// Update source-level attributed_project_ids
	if len(attributedProjectSet) > 0 {
		projectIDs := make([]int64, 0, len(attributedProjectSet))
		for id := range attributedProjectSet {
			projectIDs = append(projectIDs, id)
		}

		if err := a.repo.UpdateSourceAttributedProjects(ctx, input.SourceID, projectIDs); err != nil {
			logger.Warn("Failed to update source attributed_project_ids", logging.Err(err))
		}

		logger.Info("Project attribution complete",
			logging.F("assertions_attributed", totalAttributed),
			logging.F("projects_matched", len(attributedProjectSet)),
			logging.F("attribution_source", attributionSource),
			logging.F("project_ids", projectIDs),
		)

		return &AttributionOutput{
			AssertionsAttributed: totalAttributed,
			ProjectsMatched:      len(attributedProjectSet),
			AttributionSource:    attributionSource,
			AttributedProjectIDs: projectIDs,
		}, nil
	}

	logger.Info("No project attributions found")
	return &AttributionOutput{}, nil
}

// distributeAssertions assigns assertions to multiple projects based on per-assertion keyword matching.
// For assertions matching multiple or no projects, uses round-robin to ensure distribution.
func (a *AttributionActivities) distributeAssertions(
	ctx context.Context,
	logger logging.Logger,
	assertions []AssertionRef,
	matchedProjects []projectMatch,
) int {
	totalAttributed := 0

	for i, assertion := range assertions {
		assertionText := assertion.Description + " " + assertion.SourceQuote
		var bestMatch *projectMatch
		matchCount := 0

		for j := range matchedProjects {
			pm := &matchedProjects[j]
			if matchesKeywordInText(assertionText, pm.keyword) {
				matchCount++
				if bestMatch == nil {
					bestMatch = pm
				}
			}
		}

		if matchCount == 0 || matchCount > 1 {
			idx := i % len(matchedProjects)
			bestMatch = &matchedProjects[idx]
		}

		if bestMatch != nil {
			if err := a.repo.UpdateAssertionAttribution(ctx, assertion.ID, bestMatch.project.ID, "keyword", bestMatch.confidence); err != nil {
				logger.Warn("Failed to update assertion attribution",
					logging.F("assertion_id", assertion.ID),
					logging.Err(err))
				continue
			}
			totalAttributed++
		}
	}

	return totalAttributed
}

// matchesKeywordInText performs case-insensitive whole-word matching.
func matchesKeywordInText(text, keyword string) bool {
	if text == "" || keyword == "" {
		return false
	}
	pattern := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(keyword))
	matched, _ := regexp.MatchString(pattern, text)
	return matched
}

// Ensure AttributionActivities implements the expected interface.
var _ interface {
	AttributeProject(ctx context.Context, input AttributionInput) (*AttributionOutput, error)
} = (*AttributionActivities)(nil)
