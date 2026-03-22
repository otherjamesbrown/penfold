package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// newsletterUserContext is the v1 user-context JSON shape stored in
// pipeline_operational_config under key "newsletter.user_context".
type newsletterUserContext struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	Priorities    string `json:"priorities"`
	InterestAreas string `json:"interest_areas"`
}

// BuildNewsletterContext composes a four-section background context string for
// newsletter_extract pipeline stages.  It never hard-errors on missing sections —
// absent data is silently omitted so extraction continues with whatever is available.
//
// Sections produced:
//  1. User Context  — from pipeline_operational_config key "newsletter.user_context"
//  2. Glossary      — acronyms and terms scanned from content (reuses BuildContext path)
//  3. Active Projects — from the projects table
//  4. Tracked Products — from the products table (status = 'active')
func (a *ContextBuilderActivities) BuildNewsletterContext(ctx context.Context, input workflows.BuildNewsletterContextInput) (*workflows.BuildNewsletterContextOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "BuildNewsletterContext"),
		logging.F("tenant_id", input.TenantID),
	)

	output := &workflows.BuildNewsletterContextOutput{}

	// Guard: nothing to do without a tenant.
	if input.TenantID == "" {
		logger.Warn("BuildNewsletterContext called with empty tenant_id; returning empty context")
		return output, nil
	}

	var sections []string

	// ── Section 1: User Context ──────────────────────────────────────────────
	if a.newsletterContextRepo != nil {
		userCtxJSON, err := a.newsletterContextRepo.GetUserContextJSON(ctx, input.TenantID)
		if err != nil {
			logger.Warn("failed to load newsletter user context (continuing without it)",
				logging.F("error", err.Error()),
			)
		} else if userCtxJSON != "" {
			var uc newsletterUserContext
			if jsonErr := json.Unmarshal([]byte(userCtxJSON), &uc); jsonErr != nil {
				logger.Warn("failed to parse newsletter user context JSON (continuing without it)",
					logging.F("error", jsonErr.Error()),
				)
			} else {
				sections = append(sections, formatUserContextSection(uc))
				output.UserContextFound = true
			}
		}
	}

	// ── Section 2: Glossary ──────────────────────────────────────────────────
	// Reuse the existing BuildContext path (extract_ner stage) which scans for
	// acronyms and looks them up in the glossary table.
	cp, err := a.BuildContext(ctx, "extract_ner", workflows.BuildContextInput{
		TenantID: input.TenantID,
		Subject:  input.Subject,
		Content:  input.Content,
	}, nil, nil)
	if err != nil {
		logger.Warn("failed to build glossary context (continuing without it)",
			logging.F("error", err.Error()),
		)
	} else if len(cp.GlossaryTerms) > 0 {
		var lines []string
		for _, t := range cp.GlossaryTerms {
			lines = append(lines, fmt.Sprintf("- **%s**: %s", t.Term, t.Definition))
		}
		sections = append(sections, "### Glossary\n"+strings.Join(lines, "\n"))
		output.GlossaryCount = len(cp.GlossaryTerms)
	}

	// ── Sections 3 & 4: Projects / Products ─────────────────────────────────
	if a.newsletterContextRepo != nil {
		// Active Projects
		projects, err := a.newsletterContextRepo.ListActiveProjects(ctx, input.TenantID, 20)
		if err != nil {
			logger.Warn("failed to load active projects (continuing without them)",
				logging.F("error", err.Error()),
			)
		} else if len(projects) > 0 {
			sections = append(sections, formatNameDescriptionSection("### Active Projects", projects))
			output.ProjectCount = len(projects)
		}

		// Tracked Products
		products, err := a.newsletterContextRepo.ListActiveProducts(ctx, input.TenantID, 20)
		if err != nil {
			logger.Warn("failed to load tracked products (continuing without them)",
				logging.F("error", err.Error()),
			)
		} else if len(products) > 0 {
			sections = append(sections, formatNameDescriptionSection("### Tracked Products", products))
			output.ProductCount = len(products)
		}
	}

	output.BackgroundContext = strings.Join(sections, "\n\n")

	logger.Info("built newsletter context",
		logging.F("user_context_found", output.UserContextFound),
		logging.F("glossary_count", output.GlossaryCount),
		logging.F("project_count", output.ProjectCount),
		logging.F("product_count", output.ProductCount),
		logging.F("context_length", len(output.BackgroundContext)),
	)

	return output, nil
}

// formatUserContextSection renders the User Context section from a parsed JSON blob.
func formatUserContextSection(uc newsletterUserContext) string {
	var parts []string
	identity := strings.TrimSpace(uc.Name + ", " + uc.Role)
	// Strip trailing comma if role is empty.
	identity = strings.TrimSuffix(strings.TrimSpace(identity), ",")
	if identity != "" {
		parts = append(parts, identity+".")
	}
	if uc.Priorities != "" {
		parts = append(parts, "Priorities: "+uc.Priorities+".")
	}
	if uc.InterestAreas != "" {
		parts = append(parts, "Interest areas: "+uc.InterestAreas+".")
	}
	return "### User Context\n" + strings.Join(parts, " ")
}

// formatNameDescriptionSection renders a markdown section for a list of name/description pairs.
func formatNameDescriptionSection(header string, items []NewsletterNameDescription) string {
	var lines []string
	for _, item := range items {
		if item.Description != "" {
			lines = append(lines, fmt.Sprintf("- **%s**: %s", item.Name, item.Description))
		} else {
			lines = append(lines, fmt.Sprintf("- **%s**", item.Name))
		}
	}
	return header + "\n" + strings.Join(lines, "\n")
}
