package activities

import (
	"fmt"
	"strings"
)

// newsletterUserContext is the v1 user-context JSON shape stored in
// pipeline_operational_config under key "newsletter.user_context".
type newsletterUserContext struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	Priorities    string `json:"priorities"`
	InterestAreas string `json:"interest_areas"`
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
