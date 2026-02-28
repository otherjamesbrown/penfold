package classification

import (
	"testing"
)

// buildSeededRules returns the in-memory equivalent of the seeded classification rules
// from migrations/065_seed_classification_rules.sql, replicating the exact rules from
// source_system.go for backwards compatibility testing.
func buildSeededRules() []ClassificationRule {
	return []ClassificationRule{
		{
			ID:                 1,
			TenantID:           "tenant-a",
			Name:               "jira",
			Priority:           10,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "jira",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 1, Field: "from_address", MatchType: "contains", Value: "jira", CaseSensitive: false},
				{ID: 2, Field: "subject", MatchType: "prefix", Value: "[TRACK-JIRA]", CaseSensitive: false},
				{ID: 3, Field: "message_id", MatchType: "contains", Value: "@Atlassian.JIRA", CaseSensitive: false},
			},
		},
		{
			ID:                 2,
			TenantID:           "tenant-a",
			Name:               "aha",
			Priority:           20,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "aha",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 4, Field: "from_address", MatchType: "glob", Value: "*@*.mailer.aha.io", CaseSensitive: false},
				{ID: 5, Field: "subject", MatchType: "prefix", Value: "[AHA]", CaseSensitive: false},
				{ID: 19, Field: "from_address", MatchType: "exact", Value: "support@aha.io", CaseSensitive: false},
			},
		},
		{
			ID:                 3,
			TenantID:           "tenant-a",
			Name:               "google_docs",
			Priority:           30,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "google_docs",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 6, Field: "from_address", MatchType: "glob", Value: "*-noreply@docs.google.com", CaseSensitive: false},
				{ID: 7, Field: "from_address", MatchType: "exact", Value: "drive-shares-dm-noreply@google.com", CaseSensitive: false},
			},
		},
		{
			ID:                 4,
			TenantID:           "tenant-a",
			Name:               "webex",
			Priority:           40,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "webex",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 8, Field: "from_address", MatchType: "exact", Value: "messenger@webex.com", CaseSensitive: false},
			},
		},
		{
			ID:                 5,
			TenantID:           "tenant-a",
			Name:               "smartsheet",
			Priority:           50,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "smartsheet",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 9, Field: "from_address", MatchType: "glob", Value: "*@*.smartsheet.com", CaseSensitive: false},
			},
		},
		{
			ID:               6,
			TenantID:         "tenant-a",
			Name:             "auto_reply",
			Priority:         60,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "AUTO_REPLY",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 10, Field: "subject", MatchType: "prefix", Value: "Automatic reply:", CaseSensitive: false},
			},
		},
		{
			ID:               7,
			TenantID:         "tenant-a",
			Name:             "calendar_cancel",
			Priority:         70,
			ContentTypeScope: "",
			ContentType:      "CALENDAR",
			ContentSubtype:   "CANCELLATION",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 11, Field: "subject", MatchType: "prefix", Value: "Canceled:", CaseSensitive: false},
				{ID: 12, Field: "subject", MatchType: "prefix", Value: "Cancelled:", CaseSensitive: false},
			},
		},
		{
			ID:               8,
			TenantID:         "tenant-a",
			Name:             "calendar_invite",
			Priority:         80,
			ContentTypeScope: "",
			ContentType:      "CALENDAR",
			ContentSubtype:   "INVITE",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 13, Field: "header:Content-Type", MatchType: "contains", Value: "text/calendar", CaseSensitive: false},
			},
		},
		{
			ID:               9,
			TenantID:         "tenant-a",
			Name:             "newsletter",
			Priority:         90,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "NEWSLETTER",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 14, Field: "header:Precedence", MatchType: "exact", Value: "bulk", CaseSensitive: false},
				{ID: 15, Field: "header:Precedence", MatchType: "exact", Value: "list", CaseSensitive: false},
				{ID: 16, Field: "header:List-Unsubscribe", MatchType: "exists", Value: "", CaseSensitive: false},
				{ID: 17, Field: "subject", MatchType: "contains", Value: "Newsletter", CaseSensitive: false},
				{ID: 18, Field: "subject", MatchType: "contains", Value: "Weekly Digest", CaseSensitive: false},
			},
		},
		// --- Wave 3 rules (migration 092) ---
		{
			ID:               10,
			TenantID:         "tenant-a",
			Name:             "calendar_accept",
			Priority:         7,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "CALENDAR_RESPONSE",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 20, Field: "subject", MatchType: "prefix", Value: "Accepted:", CaseSensitive: false},
			},
		},
		{
			ID:               11,
			TenantID:         "tenant-a",
			Name:             "calendar_decline",
			Priority:         7,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "CALENDAR_RESPONSE",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 21, Field: "subject", MatchType: "prefix", Value: "Declined:", CaseSensitive: false},
			},
		},
		{
			ID:               12,
			TenantID:         "tenant-a",
			Name:             "calendar_tentative",
			Priority:         7,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "CALENDAR_RESPONSE",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 22, Field: "subject", MatchType: "prefix", Value: "Tentative:", CaseSensitive: false},
			},
		},
		{
			ID:               13,
			TenantID:         "tenant-a",
			Name:             "calendar_update",
			Priority:         7,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "CALENDAR_UPDATE",
			Active:           true,
			Conditions: []MatchCondition{
				{ID: 23, Field: "subject", MatchType: "prefix", Value: "Updated invitation:", CaseSensitive: false},
			},
		},
		{
			ID:                 14,
			TenantID:           "tenant-a",
			Name:               "google_security",
			Priority:           5,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "google",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 24, Field: "from_address", MatchType: "exact", Value: "no-reply@accounts.google.com", CaseSensitive: false},
			},
		},
		{
			ID:                 15,
			TenantID:           "tenant-a",
			Name:               "it_notification",
			Priority:           5,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "akamai",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 25, Field: "from_address", MatchType: "exact", Value: "thesolutioncenter@akamai.com", CaseSensitive: false},
			},
		},
	}
}

// TestRuleEngine_JiraNotification verifies that from_address containing "jira" → EMAIL/NOTIFICATION/jira
func TestRuleEngine_JiraNotification(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "jira@company.atlassian.net",
		"subject":      "Issue Updated",
	})

	if result.ContentType != "EMAIL" {
		t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
	}
	if result.ContentSubtype != "NOTIFICATION" {
		t.Errorf("expected ContentSubtype=NOTIFICATION, got %q", result.ContentSubtype)
	}
	if result.NotificationSource != "jira" {
		t.Errorf("expected NotificationSource=jira, got %q", result.NotificationSource)
	}
	if result.RuleName != "jira" {
		t.Errorf("expected RuleName=jira, got %q", result.RuleName)
	}
}

// TestRuleEngine_AutoReply verifies that subject "Automatic reply:..." → EMAIL/AUTO_REPLY
func TestRuleEngine_AutoReply(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "user@company.com",
		"subject":      "Automatic reply: Out of office",
	})

	if result.ContentType != "EMAIL" {
		t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
	}
	if result.ContentSubtype != "AUTO_REPLY" {
		t.Errorf("expected ContentSubtype=AUTO_REPLY, got %q", result.ContentSubtype)
	}
	if result.RuleName != "auto_reply" {
		t.Errorf("expected RuleName=auto_reply, got %q", result.RuleName)
	}
}

// TestRuleEngine_CalendarInvite verifies that header:Content-Type contains "text/calendar" → CALENDAR/INVITE
func TestRuleEngine_CalendarInvite(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("CALENDAR", map[string]string{
		"from_address":        "user@company.com",
		"subject":             "Team Meeting",
		"header:Content-Type": "text/calendar; charset=utf-8",
	})

	if result.ContentType != "CALENDAR" {
		t.Errorf("expected ContentType=CALENDAR, got %q", result.ContentType)
	}
	if result.ContentSubtype != "INVITE" {
		t.Errorf("expected ContentSubtype=INVITE, got %q", result.ContentSubtype)
	}
	if result.RuleName != "calendar_invite" {
		t.Errorf("expected RuleName=calendar_invite, got %q", result.RuleName)
	}
}

// TestRuleEngine_CalendarCancellation verifies that subject "Canceled:..." → CALENDAR/CANCELLATION
func TestRuleEngine_CalendarCancellation(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	tests := []struct {
		name    string
		subject string
	}{
		{"Canceled prefix", "Canceled: Team Meeting"},
		{"Cancelled prefix", "Cancelled: Project Review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Classify("CALENDAR", map[string]string{
				"from_address": "user@company.com",
				"subject":      tt.subject,
			})

			if result.ContentType != "CALENDAR" {
				t.Errorf("expected ContentType=CALENDAR, got %q", result.ContentType)
			}
			if result.ContentSubtype != "CANCELLATION" {
				t.Errorf("expected ContentSubtype=CANCELLATION, got %q", result.ContentSubtype)
			}
			if result.RuleName != "calendar_cancel" {
				t.Errorf("expected RuleName=calendar_cancel, got %q", result.RuleName)
			}
		})
	}
}

// TestRuleEngine_ContentTypeScope verifies that auto_reply rule (scope=EMAIL) doesn't fire for CALENDAR items.
func TestRuleEngine_ContentTypeScope(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	// An email with "Automatic reply:" subject classified as CALENDAR content type
	// should NOT match the auto_reply rule (scope=EMAIL).
	// It also should not match calendar_cancel or calendar_invite.
	// So it should fall through to default EMAIL/HUMAN.
	result := engine.Classify("CALENDAR", map[string]string{
		"from_address": "user@company.com",
		"subject":      "Automatic reply: OOO",
	})

	// auto_reply has scope=EMAIL, so it must NOT fire for a CALENDAR item
	if result.RuleName == "auto_reply" {
		t.Errorf("auto_reply rule (scope=EMAIL) must not fire for CALENDAR content type, but it did")
	}
	// With no other matching rule, default applies
	if result.ContentType != "EMAIL" || result.ContentSubtype != "HUMAN" {
		t.Errorf("expected default EMAIL/HUMAN for unmatched CALENDAR item, got %q/%q", result.ContentType, result.ContentSubtype)
	}
}

// TestRuleEngine_NullScope verifies that a rule with empty scope evaluates for all content types.
func TestRuleEngine_NullScope(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	// calendar_cancel has empty scope so it applies to any content type
	// Pass it as EMAIL content type — it should still match
	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "user@company.com",
		"subject":      "Canceled: Team Meeting",
	})

	// The jira, aha, etc. rules (scope=EMAIL) are checked first (lower priority numbers).
	// None of them match. Then calendar_cancel (priority=70, scope="") should match.
	if result.RuleName != "calendar_cancel" {
		t.Errorf("expected calendar_cancel to match (null scope = all types), got rule=%q", result.RuleName)
	}
	if result.ContentType != "CALENDAR" {
		t.Errorf("expected ContentType=CALENDAR, got %q", result.ContentType)
	}
}

// TestRuleEngine_PriorityOrder verifies that higher priority (lower number) wins when multiple rules match.
func TestRuleEngine_PriorityOrder(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	// from_address matches jira (priority 10) AND subject matches calendar_cancel (priority 70).
	// JIRA must win.
	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "jira@company.atlassian.net",
		"subject":      "Canceled: Sprint Planning",
	})

	if result.RuleName != "jira" {
		t.Errorf("expected jira rule (priority 10) to win over calendar_cancel (priority 70), got %q", result.RuleName)
	}
	if result.ContentType != "EMAIL" {
		t.Errorf("expected ContentType=EMAIL (from jira rule), got %q", result.ContentType)
	}
}

// TestRuleEngine_DefaultHumanEmail verifies that no rules matching → EMAIL/HUMAN.
func TestRuleEngine_DefaultHumanEmail(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "colleague@company.com",
		"subject":      "Project update",
		"message_id":   "<abc123@company.com>",
	})

	if result.ContentType != "EMAIL" {
		t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
	}
	if result.ContentSubtype != "HUMAN" {
		t.Errorf("expected ContentSubtype=HUMAN, got %q", result.ContentSubtype)
	}
	if result.RuleName != "" {
		t.Errorf("expected no rule to match, got RuleName=%q", result.RuleName)
	}
}

// TestRuleEngine_NewSystemNoCodeChange verifies the data-driven nature: insert a new rule in memory,
// confirm it matches without any code change.
func TestRuleEngine_NewSystemNoCodeChange(t *testing.T) {
	// Start with seeded rules and add a brand-new rule for "github" notifications
	rules := buildSeededRules()
	rules = append(rules, ClassificationRule{
		ID:                 99,
		TenantID:           "tenant-a",
		Name:               "github",
		Priority:           25,
		ContentTypeScope:   "EMAIL",
		ContentType:        "EMAIL",
		ContentSubtype:     "NOTIFICATION",
		NotificationSource: "github",
		Active:             true,
		Conditions: []MatchCondition{
			{ID: 99, Field: "from_address", MatchType: "suffix", Value: "@github.com", CaseSensitive: false},
		},
	})

	engine := NewEngine(rules)

	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "noreply@github.com",
		"subject":      "New pull request",
	})

	if result.RuleName != "github" {
		t.Errorf("expected new github rule to match, got %q", result.RuleName)
	}
	if result.NotificationSource != "github" {
		t.Errorf("expected NotificationSource=github, got %q", result.NotificationSource)
	}
}

// TestRuleEngine_PerTenant verifies that different tenants with different rules produce different results.
func TestRuleEngine_PerTenant(t *testing.T) {
	// Tenant A: has jira rule
	rulesA := []ClassificationRule{
		{
			ID:                 1,
			TenantID:           "tenant-a",
			Name:               "jira",
			Priority:           10,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "jira",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 1, Field: "from_address", MatchType: "contains", Value: "jira", CaseSensitive: false},
			},
		},
	}

	// Tenant B: has NO jira rule, only a custom rule
	rulesB := []ClassificationRule{
		{
			ID:                 2,
			TenantID:           "tenant-b",
			Name:               "internal",
			Priority:           10,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "INTERNAL",
			NotificationSource: "internal",
			Active:             true,
			Conditions: []MatchCondition{
				{ID: 2, Field: "from_address", MatchType: "suffix", Value: "@acme.internal", CaseSensitive: false},
			},
		},
	}

	engineA := NewEngine(rulesA)
	engineB := NewEngine(rulesB)

	metadata := map[string]string{
		"from_address": "jira@company.atlassian.net",
		"subject":      "Issue Updated",
	}

	resultA := engineA.Classify("EMAIL", metadata)
	resultB := engineB.Classify("EMAIL", metadata)

	if resultA.RuleName != "jira" {
		t.Errorf("tenant-a: expected jira rule, got %q", resultA.RuleName)
	}
	// Tenant B has no jira rule, so it should fall through to default
	if resultB.RuleName != "" {
		t.Errorf("tenant-b: expected no match (default), got rule=%q", resultB.RuleName)
	}
	if resultB.ContentSubtype != "HUMAN" {
		t.Errorf("tenant-b: expected default HUMAN subtype, got %q", resultB.ContentSubtype)
	}
}

// TestRuleEngine_BackwardsCompatible ports ALL test cases from source_system_test.go and
// verifies the rule engine produces matching classification outcomes.
// This is critical — the engine must be a drop-in replacement for ClassifySourceSystem.
//
// Mapping from SourceSystem to engine output:
//   SourceSystemJira            → EMAIL/NOTIFICATION/jira
//   SourceSystemAha             → EMAIL/NOTIFICATION/aha
//   SourceSystemGoogleDocs      → EMAIL/NOTIFICATION/google_docs
//   SourceSystemWebex           → EMAIL/NOTIFICATION/webex
//   SourceSystemSmartsheet      → EMAIL/NOTIFICATION/smartsheet
//   SourceSystemAutoReply       → EMAIL/AUTO_REPLY
//   SourceSystemOutlookCalendar → CALENDAR/CANCELLATION or CALENDAR/INVITE (depends on rule)
//   SourceSystemHumanEmail      → EMAIL/HUMAN (default)
func TestRuleEngine_BackwardsCompatible(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	type want struct {
		contentType    string
		contentSubtype string
		// notificationSource is checked only when non-empty
		notificationSource string
	}

	tests := []struct {
		name        string
		fromAddress string
		subject     string
		messageID   string
		headers     map[string]string
		// contentType passed to engine; "EMAIL" for all non-calendar cases
		inputContentType string
		want             want
	}{
		// --- TestClassifySourceSystem cases ---
		// AC1: JIRA detection - from_address containing "jira@"
		{
			name:             "jira from address with jira@",
			fromAddress:      "jira@company.atlassian.net",
			subject:          "Issue Updated",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		// AC1: JIRA detection - message_id containing "@Atlassian.JIRA"
		{
			name:             "jira message_id with @Atlassian.JIRA",
			fromAddress:      "notifications@company.com",
			subject:          "Issue Updated",
			messageID:        "<12345@Atlassian.JIRA>",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		// AC1: JIRA detection - subject prefix "[TRACK-JIRA]"
		{
			name:             "jira subject prefix [TRACK-JIRA]",
			fromAddress:      "notifications@company.com",
			subject:          "[TRACK-JIRA] New issue assigned",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		// AC2: AHA detection - from_address matching *@*.mailer.aha.io
		{
			name:             "aha from address *@*.mailer.aha.io",
			fromAddress:      "noreply@product.mailer.aha.io",
			subject:          "Feature request updated",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "aha"},
		},
		// AC2: AHA detection - subject prefix "[AHA]"
		{
			name:             "aha subject prefix [AHA]",
			fromAddress:      "product@company.com",
			subject:          "[AHA] New feature request",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "aha"},
		},
		// AC3: Google Docs detection - from matches *-noreply@docs.google.com
		{
			name:             "google docs from *-noreply@docs.google.com",
			fromAddress:      "comments-noreply@docs.google.com",
			subject:          "New comment on document",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "google_docs"},
		},
		// AC3: Google Docs detection - from = drive-shares-dm-noreply@google.com
		{
			name:             "google docs from drive-shares-dm-noreply@google.com",
			fromAddress:      "drive-shares-dm-noreply@google.com",
			subject:          "Document shared with you",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "google_docs"},
		},
		// AC4: Webex detection - from = messenger@webex.com
		{
			name:             "webex from messenger@webex.com",
			fromAddress:      "messenger@webex.com",
			subject:          "New message from team",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "webex"},
		},
		// AC5: Smartsheet detection - from matching *@*.smartsheet.com
		{
			name:             "smartsheet from *@*.smartsheet.com",
			fromAddress:      "notifications@app.smartsheet.com",
			subject:          "Sheet updated",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "smartsheet"},
		},
		// AC6: Auto-reply detection
		{
			name:             "auto-reply subject Automatic reply:",
			fromAddress:      "user@company.com",
			subject:          "Automatic reply: Out of office",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "AUTO_REPLY", ""},
		},
		// AC7: Calendar cancellation - "Canceled:"
		{
			name:             "calendar cancelled with Canceled:",
			fromAddress:      "user@company.com",
			subject:          "Canceled: Team Meeting",
			inputContentType: "CALENDAR",
			want:             want{"CALENDAR", "CANCELLATION", ""},
		},
		// AC7: Calendar cancellation - "Cancelled:"
		{
			name:             "calendar cancelled with Cancelled:",
			fromAddress:      "user@company.com",
			subject:          "Cancelled: Project Review",
			inputContentType: "CALENDAR",
			want:             want{"CALENDAR", "CANCELLATION", ""},
		},
		// AC7: Calendar invite with text/calendar content-type
		{
			name:             "calendar invite with text/calendar content-type",
			fromAddress:      "user@company.com",
			subject:          "Team Meeting",
			headers:          map[string]string{"Content-Type": "text/calendar; charset=utf-8"},
			inputContentType: "CALENDAR",
			want:             want{"CALENDAR", "INVITE", ""},
		},
		// AC8: Human email fallback
		{
			name:             "human email no patterns match",
			fromAddress:      "colleague@company.com",
			subject:          "Project update",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "HUMAN", ""},
		},
		// AC9: OR logic - any single condition is sufficient
		{
			name:             "jira OR logic - only from_address matches",
			fromAddress:      "jira@company.com",
			subject:          "Normal subject",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "jira OR logic - only message_id matches",
			fromAddress:      "normal@company.com",
			subject:          "Normal subject",
			messageID:        "<abc@Atlassian.JIRA>",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "jira OR logic - only subject matches",
			fromAddress:      "normal@company.com",
			subject:          "[TRACK-JIRA] Issue created",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		// AC10: Priority ordering - JIRA wins over calendar
		{
			name:             "priority ordering jira wins over calendar",
			fromAddress:      "jira@company.atlassian.net",
			subject:          "Canceled: Sprint Planning",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		// AC11: JIRA independent of thread/reply indicators
		{
			name:             "jira independent of content_subtype",
			fromAddress:      "jira@company.atlassian.net",
			subject:          "Re: [PROJ-123] Issue Updated",
			messageID:        "<msg@Atlassian.JIRA>",
			headers:          map[string]string{"In-Reply-To": "<previous@company.com>"},
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},

		// --- TestClassifySourceSystem_CaseInsensitivity cases ---
		{
			name:             "JIRA uppercase from",
			fromAddress:      "JIRA@COMPANY.COM",
			subject:          "Issue",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "jira lowercase from",
			fromAddress:      "jira@company.com",
			subject:          "Issue",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "automatic reply mixed case",
			fromAddress:      "user@company.com",
			subject:          "AUTOMATIC REPLY: OOO",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "AUTO_REPLY", ""},
		},
		{
			name:             "canceled mixed case",
			fromAddress:      "user@company.com",
			subject:          "CANCELED: Meeting",
			inputContentType: "CALENDAR",
			want:             want{"CALENDAR", "CANCELLATION", ""},
		},

		// --- TestClassifySourceSystem_EdgeCases cases ---
		{
			name:             "empty inputs",
			fromAddress:      "",
			subject:          "",
			messageID:        "",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "HUMAN", ""},
		},
		{
			name:             "whitespace subject prefix",
			fromAddress:      "user@company.com",
			subject:          "  Automatic reply: OOO",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "AUTO_REPLY", ""},
		},
		{
			name:             "partial jira match in from",
			fromAddress:      "jirasupport@company.com",
			subject:          "Test",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "jira in middle of message_id",
			fromAddress:      "user@company.com",
			subject:          "Test",
			messageID:        "<prefix-abc@Atlassian.JIRA-suffix>",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "jira"},
		},
		{
			name:             "multiple domain parts smartsheet",
			fromAddress:      "noreply@us.app.smartsheet.com",
			subject:          "Update",
			inputContentType: "EMAIL",
			want:             want{"EMAIL", "NOTIFICATION", "smartsheet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"from_address": tt.fromAddress,
				"subject":      tt.subject,
				"message_id":   tt.messageID,
			}
			// Add headers as "header:<Key>" entries
			for k, v := range tt.headers {
				metadata["header:"+k] = v
			}

			// Apply whitespace trimming to from_address and subject, as source_system.go does
			metadata["from_address"] = trimWhitespace(metadata["from_address"])
			metadata["subject"] = trimWhitespace(metadata["subject"])
			metadata["message_id"] = trimWhitespace(metadata["message_id"])

			result := engine.Classify(tt.inputContentType, metadata)

			if result.ContentType != tt.want.contentType {
				t.Errorf("ContentType: got %q, want %q", result.ContentType, tt.want.contentType)
			}
			if result.ContentSubtype != tt.want.contentSubtype {
				t.Errorf("ContentSubtype: got %q, want %q", result.ContentSubtype, tt.want.contentSubtype)
			}
			if tt.want.notificationSource != "" && result.NotificationSource != tt.want.notificationSource {
				t.Errorf("NotificationSource: got %q, want %q", result.NotificationSource, tt.want.notificationSource)
			}
		})
	}
}

// trimWhitespace trims leading and trailing whitespace from a string.
// This mirrors the strings.TrimSpace call in source_system.go.
func trimWhitespace(s string) string {
	result := s
	for len(result) > 0 && (result[0] == ' ' || result[0] == '\t' || result[0] == '\n' || result[0] == '\r') {
		result = result[1:]
	}
	for len(result) > 0 {
		last := result[len(result)-1]
		if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
			result = result[:len(result)-1]
		} else {
			break
		}
	}
	return result
}

// TestRuleEngine_InactiveRulesIgnored verifies that inactive rules are not evaluated.
func TestRuleEngine_InactiveRulesIgnored(t *testing.T) {
	rules := []ClassificationRule{
		{
			ID:               1,
			TenantID:         "tenant-a",
			Name:             "disabled_rule",
			Priority:         5,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "NOTIFICATION",
			Active:           false, // inactive
			Conditions: []MatchCondition{
				{ID: 1, Field: "from_address", MatchType: "contains", Value: "jira", CaseSensitive: false},
			},
		},
	}

	engine := NewEngine(rules)
	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "jira@company.com",
	})

	// Inactive rule must not fire
	if result.RuleName == "disabled_rule" {
		t.Errorf("inactive rule must not fire, but it did")
	}
	if result.ContentSubtype != "HUMAN" {
		t.Errorf("expected default HUMAN result, got %q", result.ContentSubtype)
	}
}

// TestRuleEngine_EmptyRules verifies that an engine with no rules always returns the default.
func TestRuleEngine_EmptyRules(t *testing.T) {
	engine := NewEngine([]ClassificationRule{})

	result := engine.Classify("EMAIL", map[string]string{
		"from_address": "jira@company.com",
	})

	if result.ContentType != "EMAIL" || result.ContentSubtype != "HUMAN" {
		t.Errorf("expected EMAIL/HUMAN default, got %q/%q", result.ContentType, result.ContentSubtype)
	}
}

// TestMatchCondition_AllMatchTypes tests each match type individually.
func TestMatchCondition_AllMatchTypes(t *testing.T) {
	tests := []struct {
		name      string
		cond      MatchCondition
		input     string
		wantMatch bool
	}{
		{
			name:      "contains - match",
			cond:      MatchCondition{MatchType: "contains", Value: "jira", CaseSensitive: false},
			input:     "jira@company.com",
			wantMatch: true,
		},
		{
			name:      "contains - no match",
			cond:      MatchCondition{MatchType: "contains", Value: "jira", CaseSensitive: false},
			input:     "user@company.com",
			wantMatch: false,
		},
		{
			name:      "prefix - match",
			cond:      MatchCondition{MatchType: "prefix", Value: "[TRACK-JIRA]", CaseSensitive: false},
			input:     "[TRACK-JIRA] Issue created",
			wantMatch: true,
		},
		{
			name:      "prefix - no match",
			cond:      MatchCondition{MatchType: "prefix", Value: "[TRACK-JIRA]", CaseSensitive: false},
			input:     "Normal subject",
			wantMatch: false,
		},
		{
			name:      "suffix - match",
			cond:      MatchCondition{MatchType: "suffix", Value: "@github.com", CaseSensitive: false},
			input:     "noreply@github.com",
			wantMatch: true,
		},
		{
			name:      "suffix - no match",
			cond:      MatchCondition{MatchType: "suffix", Value: "@github.com", CaseSensitive: false},
			input:     "noreply@gitlab.com",
			wantMatch: false,
		},
		{
			name:      "exact - match",
			cond:      MatchCondition{MatchType: "exact", Value: "messenger@webex.com", CaseSensitive: false},
			input:     "messenger@webex.com",
			wantMatch: true,
		},
		{
			name:      "exact - no match",
			cond:      MatchCondition{MatchType: "exact", Value: "messenger@webex.com", CaseSensitive: false},
			input:     "xmessenger@webex.com",
			wantMatch: false,
		},
		{
			name:      "glob - match *@*.mailer.aha.io",
			cond:      MatchCondition{MatchType: "glob", Value: "*@*.mailer.aha.io", CaseSensitive: false},
			input:     "noreply@product.mailer.aha.io",
			wantMatch: true,
		},
		{
			name:      "glob - no match *@*.mailer.aha.io",
			cond:      MatchCondition{MatchType: "glob", Value: "*@*.mailer.aha.io", CaseSensitive: false},
			input:     "noreply@other.com",
			wantMatch: false,
		},
		{
			name:      "glob - match *@*.smartsheet.com with multiple subdomains",
			cond:      MatchCondition{MatchType: "glob", Value: "*@*.smartsheet.com", CaseSensitive: false},
			input:     "noreply@us.app.smartsheet.com",
			wantMatch: true,
		},
		{
			name:      "case sensitive - match",
			cond:      MatchCondition{MatchType: "contains", Value: "Jira", CaseSensitive: true},
			input:     "Jira@company.com",
			wantMatch: true,
		},
		{
			name:      "case sensitive - no match on wrong case",
			cond:      MatchCondition{MatchType: "contains", Value: "Jira", CaseSensitive: true},
			input:     "jira@company.com",
			wantMatch: false,
		},
		{
			name:      "exists - match (non-empty value)",
			cond:      MatchCondition{MatchType: "exists", Value: "", CaseSensitive: false},
			input:     "<mailto:unsub@example.com>",
			wantMatch: true,
		},
		{
			name:      "exists - no match (empty value)",
			cond:      MatchCondition{MatchType: "exists", Value: "", CaseSensitive: false},
			input:     "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchCondition(tt.cond, tt.input)
			if got != tt.wantMatch {
				t.Errorf("matchCondition(%v, %q) = %v, want %v", tt.cond, tt.input, got, tt.wantMatch)
			}
		})
	}
}

// TestRuleEngine_Newsletter verifies that emails with bulk Precedence header → EMAIL/NEWSLETTER.
func TestRuleEngine_Newsletter(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address":      "news@company.com",
		"subject":           "Company Updates",
		"header:Precedence": "bulk",
	})

	if result.ContentType != "EMAIL" {
		t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
	}
	if result.ContentSubtype != "NEWSLETTER" {
		t.Errorf("expected ContentSubtype=NEWSLETTER, got %q", result.ContentSubtype)
	}
	if result.RuleName != "newsletter" {
		t.Errorf("expected RuleName=newsletter, got %q", result.RuleName)
	}
}

// TestRuleEngine_NewsletterListUnsubscribe verifies that List-Unsubscribe header triggers newsletter.
func TestRuleEngine_NewsletterListUnsubscribe(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address":            "marketing@example.com",
		"subject":                 "Weekly Updates",
		"header:List-Unsubscribe": "<mailto:unsub@example.com>",
	})

	if result.ContentSubtype != "NEWSLETTER" {
		t.Errorf("expected ContentSubtype=NEWSLETTER, got %q", result.ContentSubtype)
	}
}

// TestRuleEngine_NewsletterSubjectContains verifies subject-based newsletter detection.
func TestRuleEngine_NewsletterSubjectContains(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	tests := []struct {
		name    string
		subject string
	}{
		{"Newsletter in subject", "Monthly Newsletter - January"},
		{"Weekly Digest in subject", "Weekly Digest: Top Stories"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Classify("EMAIL", map[string]string{
				"from_address": "sender@example.com",
				"subject":      tt.subject,
			})

			if result.ContentSubtype != "NEWSLETTER" {
				t.Errorf("expected ContentSubtype=NEWSLETTER for subject %q, got %q", tt.subject, result.ContentSubtype)
			}
		})
	}
}

// TestRuleEngine_Wave3CalendarResponse verifies wave 3 calendar response classification rules.
func TestRuleEngine_Wave3CalendarResponse(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	tests := []struct {
		name           string
		subject        string
		wantSubtype    string
		wantRuleName   string
	}{
		{"Accepted subject", "Accepted: Weekly Team Standup", "CALENDAR_RESPONSE", "calendar_accept"},
		{"Declined subject", "Declined: James / Tom 1:1", "CALENDAR_RESPONSE", "calendar_decline"},
		{"Tentative subject", "Tentative: Project Review Meeting", "CALENDAR_RESPONSE", "calendar_tentative"},
		{"Updated invitation", "Updated invitation: Standup", "CALENDAR_UPDATE", "calendar_update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Classify("EMAIL", map[string]string{
				"from_address": "colleague@example.com",
				"subject":      tt.subject,
			})

			if result.ContentType != "EMAIL" {
				t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
			}
			if result.ContentSubtype != tt.wantSubtype {
				t.Errorf("expected ContentSubtype=%q, got %q", tt.wantSubtype, result.ContentSubtype)
			}
			if result.RuleName != tt.wantRuleName {
				t.Errorf("expected RuleName=%q, got %q", tt.wantRuleName, result.RuleName)
			}
		})
	}
}

// TestRuleEngine_Wave3Notifications verifies wave 3 notification classification rules.
func TestRuleEngine_Wave3Notifications(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	tests := []struct {
		name               string
		fromAddress        string
		subject            string
		wantSubtype        string
		wantNotifSource    string
		wantRuleName       string
	}{
		{
			name:            "Google security alert",
			fromAddress:     "no-reply@accounts.google.com",
			subject:         "Security alert: New sign-in from Chrome on Mac",
			wantSubtype:     "NOTIFICATION",
			wantNotifSource: "google",
			wantRuleName:    "google_security",
		},
		{
			name:            "IT notification from Akamai",
			fromAddress:     "thesolutioncenter@akamai.com",
			subject:         "IT Notification: System maintenance",
			wantSubtype:     "NOTIFICATION",
			wantNotifSource: "akamai",
			wantRuleName:    "it_notification",
		},
		{
			name:            "Aha from support@aha.io (wave 3 fix)",
			fromAddress:     "support@aha.io",
			subject:         "Your feature request was updated",
			wantSubtype:     "NOTIFICATION",
			wantNotifSource: "aha",
			wantRuleName:    "aha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Classify("EMAIL", map[string]string{
				"from_address": tt.fromAddress,
				"subject":      tt.subject,
			})

			if result.ContentType != "EMAIL" {
				t.Errorf("expected ContentType=EMAIL, got %q", result.ContentType)
			}
			if result.ContentSubtype != tt.wantSubtype {
				t.Errorf("expected ContentSubtype=%q, got %q", tt.wantSubtype, result.ContentSubtype)
			}
			if result.NotificationSource != tt.wantNotifSource {
				t.Errorf("expected NotificationSource=%q, got %q", tt.wantNotifSource, result.NotificationSource)
			}
			if result.RuleName != tt.wantRuleName {
				t.Errorf("expected RuleName=%q, got %q", tt.wantRuleName, result.RuleName)
			}
		})
	}
}

// TestRuleEngine_Wave3PriorityOrdering verifies that wave 3 rules (priority 5, 7) win
// over lower-priority rules when multiple rules could match.
func TestRuleEngine_Wave3PriorityOrdering(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	// Google security alert (priority 5) should not match newsletter (priority 90)
	// even if headers suggest newsletter
	result := engine.Classify("EMAIL", map[string]string{
		"from_address":      "no-reply@accounts.google.com",
		"subject":           "Security Newsletter: Account Activity",
		"header:Precedence": "bulk",
	})

	if result.RuleName != "google_security" {
		t.Errorf("expected google_security (priority 5) to win, got %q", result.RuleName)
	}
}

// TestRuleEngine_NewsletterNotOverrideNotification verifies that notification rules
// (lower priority number = higher priority) win over newsletter rules.
// A Jira notification with List-Unsubscribe header should stay as NOTIFICATION.
func TestRuleEngine_NewsletterNotOverrideNotification(t *testing.T) {
	engine := NewEngine(buildSeededRules())

	result := engine.Classify("EMAIL", map[string]string{
		"from_address":            "jira@company.atlassian.net",
		"subject":                 "Issue Updated",
		"header:List-Unsubscribe": "<mailto:unsub@atlassian.com>",
	})

	if result.ContentSubtype != "NOTIFICATION" {
		t.Errorf("expected ContentSubtype=NOTIFICATION (jira wins over newsletter), got %q", result.ContentSubtype)
	}
	if result.NotificationSource != "jira" {
		t.Errorf("expected NotificationSource=jira, got %q", result.NotificationSource)
	}
	if result.RuleName != "jira" {
		t.Errorf("expected RuleName=jira (priority 10 beats newsletter priority 90), got %q", result.RuleName)
	}
}
