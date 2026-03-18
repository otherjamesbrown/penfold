//go:build quality

package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// getNewsletterExtraction queries the newsletter JSON from content_enrichment.
func getNewsletterExtraction(env *QualityEnv, sourceID int64) (*ActualNewsletterExtraction, error) {
	var rawJSON []byte
	err := env.DB.QueryRow(context.Background(),
		`SELECT extracted_data->'newsletter'
         FROM content_enrichment
         WHERE source_id = $1 AND tenant_id = $2`,
		sourceID, env.TenantID).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("query newsletter extraction: %w", err)
	}
	if rawJSON == nil {
		return nil, fmt.Errorf("no newsletter extraction data for source %d", sourceID)
	}
	var result ActualNewsletterExtraction
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		return nil, fmt.Errorf("unmarshal newsletter extraction: %w", err)
	}
	return &result, nil
}

// MatchNewsletterExtract validates L2 newsletter extraction quality.
// Returns []MatchDetail AND calls t.Error on failures.
func MatchNewsletterExtract(t *testing.T, env *QualityEnv, sourceID int64, expected *NewsletterExtractExpectation) []MatchDetail {
	t.Helper()
	if expected == nil {
		return nil
	}
	var details []MatchDetail

	actual, err := getNewsletterExtraction(env, sourceID)
	if err != nil {
		t.Errorf("newsletter_extract: %v", err)
		details = append(details, MatchDetail{Check: "newsletter_extract.exists", Pass: false, Message: err.Error()})
		return details
	}

	// Log actual output for debugging
	t.Logf("  newsletter_extract output:")
	t.Logf("    newsletter_name: %s", actual.NewsletterName)
	t.Logf("    summary: %s", truncate(actual.Summary, 100))
	t.Logf("    sections: %d", len(actual.Sections))
	t.Logf("    action_items: %d", len(actual.ActionItems))
	t.Logf("    key_announcements: %d", len(actual.KeyAnnouncements))
	t.Logf("    risks: %d", len(actual.Risks))
	t.Logf("    people_mentioned: %d", len(actual.PeopleMentioned))
	t.Logf("    quality_gate_triggered: %v", actual.QualityGateTriggered)

	// Check summary
	if expected.Summary != nil {
		details = append(details, matchStringField(t, "newsletter.summary", actual.Summary, expected.Summary)...)
	}

	// Check sections count
	if expected.Sections != nil {
		details = append(details, matchCount(t, "newsletter.sections", len(actual.Sections), expected.Sections)...)
	}

	// Check action_items
	if expected.ActionItems != nil {
		items := make([]string, len(actual.ActionItems))
		for i, a := range actual.ActionItems {
			items[i] = a.Action
		}
		details = append(details, matchItemList(t, "newsletter.action_items", items, expected.ActionItems)...)
	}

	// Check key_announcements
	if expected.KeyAnnouncements != nil {
		details = append(details, matchItemList(t, "newsletter.key_announcements", actual.KeyAnnouncements, expected.KeyAnnouncements)...)
	}

	// Check risks
	if expected.Risks != nil {
		details = append(details, matchItemList(t, "newsletter.risks", actual.Risks, expected.Risks)...)
	}

	// Check people_mentioned count
	if expected.PeopleMentioned != nil {
		details = append(details, matchCount(t, "newsletter.people_mentioned", len(actual.PeopleMentioned), expected.PeopleMentioned)...)
	}

	// Check quality_gate_triggered
	if expected.QualityGateTriggered != nil {
		pass := actual.QualityGateTriggered == *expected.QualityGateTriggered
		if !pass {
			t.Errorf("newsletter.quality_gate_triggered: expected %v, got %v", *expected.QualityGateTriggered, actual.QualityGateTriggered)
		}
		details = append(details, MatchDetail{
			Check: "newsletter.quality_gate_triggered", Pass: pass,
			Expected: fmt.Sprintf("%v", *expected.QualityGateTriggered),
			Actual:   fmt.Sprintf("%v", actual.QualityGateTriggered),
		})
	}

	return details
}

// --- Sub-matchers ---

// matchStringField checks min_length and must_mention on a string field.
func matchStringField(t *testing.T, field, actual string, expected *StringFieldExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.MinLength != nil {
		pass := len(actual) >= *expected.MinLength
		if !pass {
			t.Errorf("%s.min_length: expected >= %d, got %d", field, *expected.MinLength, len(actual))
		}
		details = append(details, MatchDetail{
			Check: field + ".min_length", Pass: pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinLength),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	for _, mention := range expected.MustMention {
		pass := containsCI(actual, mention)
		if !pass {
			t.Errorf("%s.must_mention: %q not found in %q", field, mention, truncate(actual, 100))
		}
		details = append(details, MatchDetail{
			Check: fmt.Sprintf("%s.must_mention.%s", field, mention), Pass: pass,
			Expected: fmt.Sprintf("contains %q", mention),
			Actual:   truncate(actual, 100),
		})
	}

	return details
}

// matchCount checks min_count/max_count on an array length.
func matchCount(t *testing.T, field string, actualCount int, expected *CountExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.MinCount != nil {
		pass := actualCount >= *expected.MinCount
		if !pass {
			t.Errorf("%s.min_count: expected >= %d, got %d", field, *expected.MinCount, actualCount)
		}
		details = append(details, MatchDetail{
			Check: field + ".min_count", Pass: pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinCount),
			Actual:   fmt.Sprintf("%d", actualCount),
		})
	}

	if expected.MaxCount != nil {
		pass := actualCount <= *expected.MaxCount
		if !pass {
			t.Errorf("%s.max_count: expected <= %d, got %d", field, *expected.MaxCount, actualCount)
		}
		details = append(details, MatchDetail{
			Check: field + ".max_count", Pass: pass,
			Expected: fmt.Sprintf("<= %d", *expected.MaxCount),
			Actual:   fmt.Sprintf("%d", actualCount),
		})
	}

	return details
}

// matchItemList checks count bounds and must_find/must_not_find on a string list.
func matchItemList(t *testing.T, field string, actual []string, expected *ItemListExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.MinCount != nil {
		pass := len(actual) >= *expected.MinCount
		if !pass {
			t.Errorf("%s.min_count: expected >= %d, got %d", field, *expected.MinCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check: field + ".min_count", Pass: pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	if expected.MaxCount != nil {
		pass := len(actual) <= *expected.MaxCount
		if !pass {
			t.Errorf("%s.max_count: expected <= %d, got %d", field, *expected.MaxCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check: field + ".max_count", Pass: pass,
			Expected: fmt.Sprintf("<= %d", *expected.MaxCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	for _, mf := range expected.MustFind {
		found := false
		for _, item := range actual {
			if containsCI(item, mf.DescriptionContains) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s.must_find: no item containing %q", field, mf.DescriptionContains)
		}
		details = append(details, MatchDetail{
			Check: fmt.Sprintf("%s.must_find.%s", field, mf.DescriptionContains), Pass: found,
			Expected: fmt.Sprintf("item containing %q", mf.DescriptionContains),
		})
	}

	for _, mnf := range expected.MustNotFind {
		found := false
		for _, item := range actual {
			if containsCI(item, mnf.DescriptionContains) {
				found = true
				break
			}
		}
		if found {
			t.Errorf("%s.must_not_find: found item containing %q (false positive)", field, mnf.DescriptionContains)
		}
		details = append(details, MatchDetail{
			Check: fmt.Sprintf("%s.must_not_find.%s", field, mnf.DescriptionContains), Pass: !found,
			Expected: fmt.Sprintf("no item containing %q", mnf.DescriptionContains),
		})
	}

	return details
}
