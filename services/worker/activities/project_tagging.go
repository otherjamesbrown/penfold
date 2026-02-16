// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"regexp"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ProjectTaggingInput is the input for the TagProjects activity.
type ProjectTaggingInput struct {
	TenantID  string `json:"tenant_id"`
	ContentID int64  `json:"content_id"`
}

// ProjectTaggingOutput is the output from the TagProjects activity.
type ProjectTaggingOutput struct {
	ProjectsMatched  int `json:"projects_matched"`
	MentionsCreated  int `json:"mentions_created"`
}

// ProjectTaggingRepository defines the interface for project keyword lookups and mention creation.
type ProjectTaggingRepository interface {
	// GetProjectsWithKeywords retrieves all projects that have keywords defined.
	GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*entities.Project, error)

	// GetContentText retrieves the content text for a given content item.
	GetContentText(ctx context.Context, tenantID string, contentID int64) (string, error)

	// CreateContentMention creates a content_mention row linking content to an entity.
	CreateContentMention(ctx context.Context, tenantID string, contentID int64, entityType string, mentionedText string, resolvedEntityID int64) error
}

// ProjectTaggingActivities holds dependencies for project keyword tagging activities.
type ProjectTaggingActivities struct {
	logger logging.Logger
	repo   ProjectTaggingRepository
}

// NewProjectTaggingActivities creates a new ProjectTaggingActivities instance.
func NewProjectTaggingActivities(logger logging.Logger, repo ProjectTaggingRepository) *ProjectTaggingActivities {
	if logger == nil {
		panic("NewProjectTaggingActivities: logger is required")
	}
	if repo == nil {
		panic("NewProjectTaggingActivities: repo is required")
	}
	return &ProjectTaggingActivities{
		logger: logger.With(logging.F("component", "project_tagging_activities")),
		repo:   repo,
	}
}

// TagProjects matches project keywords against content text and creates content_mentions.
// This activity performs case-insensitive keyword matching with whole-word boundaries.
// Each project gets at most one mention per content item (deduplicated).
func (a *ProjectTaggingActivities) TagProjects(ctx context.Context, input ProjectTaggingInput) (*ProjectTaggingOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "TagProjects"),
		logging.F("tenant_id", input.TenantID),
		logging.F("content_id", input.ContentID),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting project tagging")

	logger.Info("Starting project keyword tagging")

	// Step 1: Fetch projects with keywords
	recordHeartbeat(ctx, "fetching projects with keywords")
	projects, err := a.repo.GetProjectsWithKeywords(ctx, input.TenantID)
	if err != nil {
		pe := perrors.ClassifyError(err, "tag_projects")
		logger.Error("Failed to fetch projects with keywords", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	if len(projects) == 0 {
		logger.Info("No projects with keywords found, skipping tagging")
		return &ProjectTaggingOutput{
			ProjectsMatched:  0,
			MentionsCreated:  0,
		}, nil
	}

	logger.Info("Fetched projects with keywords", logging.F("project_count", len(projects)))

	// Step 2: Fetch content text
	recordHeartbeat(ctx, "fetching content text")
	contentText, err := a.repo.GetContentText(ctx, input.TenantID, input.ContentID)
	if err != nil {
		pe := perrors.ClassifyError(err, "tag_projects")
		logger.Error("Failed to fetch content text", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	if contentText == "" {
		logger.Info("Content text is empty, skipping tagging")
		return &ProjectTaggingOutput{
			ProjectsMatched:  0,
			MentionsCreated:  0,
		}, nil
	}

	logger.Info("Fetched content text", logging.F("content_length", len(contentText)))

	// Step 3: Match keywords and create mentions
	recordHeartbeat(ctx, "matching keywords")
	projectsMatched := 0
	mentionsCreated := 0

	for _, project := range projects {
		// Check each keyword for this project
		var matchedKeyword string
		for _, keyword := range project.Keywords {
			if matchesKeyword(contentText, keyword) {
				matchedKeyword = keyword
				break // First match wins, deduplicate per project
			}
		}

		if matchedKeyword != "" {
			projectsMatched++

			// Create content mention
			recordHeartbeat(ctx, fmt.Sprintf("creating mention for project %d", project.ID))
			err := a.repo.CreateContentMention(ctx, input.TenantID, input.ContentID, "project", matchedKeyword, project.ID)
			if err != nil {
				pe := perrors.ClassifyError(err, "tag_projects")
				logger.Warn("Failed to create content mention",
					logging.Err(pe),
					logging.F("project_id", project.ID),
					logging.F("keyword", matchedKeyword))
				// Continue with other projects even if one fails
				continue
			}

			mentionsCreated++
			logger.Debug("Created content mention",
				logging.F("project_id", project.ID),
				logging.F("project_name", project.Name),
				logging.F("keyword", matchedKeyword))
		}
	}

	logger.Info("Project keyword tagging complete",
		logging.F("projects_matched", projectsMatched),
		logging.F("mentions_created", mentionsCreated))

	return &ProjectTaggingOutput{
		ProjectsMatched:  projectsMatched,
		MentionsCreated:  mentionsCreated,
	}, nil
}

// matchesKeyword performs case-insensitive whole-word matching of a keyword in text.
// Returns true if the keyword appears as a complete word (not as a substring of another word).
func matchesKeyword(text, keyword string) bool {
	if text == "" || keyword == "" {
		return false
	}

	// Build regex pattern for whole-word matching
	// \b matches word boundaries, so "MTC" matches "MTC project" but not "MTCX"
	// (?i) makes it case-insensitive
	pattern := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(keyword))
	matched, _ := regexp.MatchString(pattern, text)
	return matched
}

// Ensure ProjectTaggingActivities implements the expected interface.
var _ interface {
	TagProjects(ctx context.Context, input ProjectTaggingInput) (*ProjectTaggingOutput, error)
} = (*ProjectTaggingActivities)(nil)
