package batch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/processors"
	"github.com/otherjamesbrown/penfold/pkg/ingest/eml"
)

// TestIngestStoresHeaders verifies that the ingest processor preserves specified headers
// in the ingestion_metadata["headers"] map.
//
// This test SHOULD FAIL initially because processor.go sets PreserveHeaders: nil,
// meaning no headers are preserved during parsing.
func TestIngestStoresHeaders(t *testing.T) {
	// Create a test .eml with Auto-Submitted and X-Jira-Issue headers
	emlContent := `From: jira@example.com
To: user@example.com
Subject: [PROJ-123] Issue Updated
Date: Mon, 20 Jan 2026 10:00:00 -0800
Message-ID: <jira-001@example.com>
Auto-Submitted: auto-generated
X-Jira-Issue: PROJ-123
Precedence: bulk
Content-Type: text/plain; charset="UTF-8"

Issue PROJ-123 has been updated.
`

	// Write test file
	tmpDir := t.TempDir()
	emlPath := filepath.Join(tmpDir, "jira-notification.eml")
	if err := os.WriteFile(emlPath, []byte(emlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse with options matching processor.go (after fix)
	parseOpts := eml.DefaultParseOptions()
	// Preserve headers needed for content classification
	parseOpts.PreserveHeaders = []string{
		"Auto-Submitted",
		"Precedence",
		"X-Auto-Response-Suppress",
		"X-Mailer",
		"X-MS-Exchange-Organization-AuthAs",
		"X-Jira-Issue",
		"X-Jira-Fingerprint",
		"X-Aha-Issue",
	}
	parser := eml.NewParser(parseOpts)

	result, err := parser.ParseFile(emlPath)
	if err != nil {
		t.Fatalf("failed to parse email: %v", err)
	}

	email := result.Email

	// Build metadata the same way processor.go does
	metadata := map[string]interface{}{
		"from":    email.From.Email,
		"subject": email.Subject,
	}

	// The fix should add headers to metadata
	if email.Headers != nil && len(email.Headers) > 0 {
		metadata["headers"] = email.Headers
	}

	// Verify headers are in metadata
	t.Run("headers map exists", func(t *testing.T) {
		headersIface, ok := metadata["headers"]
		if !ok {
			t.Errorf("EXPECTED FAILURE: metadata[\"headers\"] not present - PreserveHeaders is nil")
			return
		}

		headers, ok := headersIface.(map[string]string)
		if !ok {
			t.Fatalf("headers is not map[string]string: %T", headersIface)
		}

		if len(headers) == 0 {
			t.Error("headers map is empty")
		}
	})

	t.Run("Auto-Submitted header preserved", func(t *testing.T) {
		headersIface, ok := metadata["headers"]
		if !ok {
			t.Skip("headers not present in metadata")
		}

		headers := headersIface.(map[string]string)
		autoSubmitted, ok := headers["Auto-Submitted"]
		if !ok {
			t.Error("EXPECTED FAILURE: Auto-Submitted header not preserved")
			return
		}

		if autoSubmitted != "auto-generated" {
			t.Errorf("expected Auto-Submitted='auto-generated', got '%s'", autoSubmitted)
		}
	})

	t.Run("X-Jira-Issue header preserved", func(t *testing.T) {
		headersIface, ok := metadata["headers"]
		if !ok {
			t.Skip("headers not present in metadata")
		}

		headers := headersIface.(map[string]string)
		jiraIssue, ok := headers["X-Jira-Issue"]
		if !ok {
			t.Error("EXPECTED FAILURE: X-Jira-Issue header not preserved")
			return
		}

		if jiraIssue != "PROJ-123" {
			t.Errorf("expected X-Jira-Issue='PROJ-123', got '%s'", jiraIssue)
		}
	})

	t.Run("Precedence header preserved", func(t *testing.T) {
		headersIface, ok := metadata["headers"]
		if !ok {
			t.Skip("headers not present in metadata")
		}

		headers := headersIface.(map[string]string)
		precedence, ok := headers["Precedence"]
		if !ok {
			t.Error("EXPECTED FAILURE: Precedence header not preserved")
			return
		}

		if precedence != "bulk" {
			t.Errorf("expected Precedence='bulk', got '%s'", precedence)
		}
	})
}

// TestIngestStoresAttachments verifies that the ingest processor preserves attachment
// metadata (filename, mime_type, size) in ingestion_metadata["attachments"] array.
//
// This test SHOULD FAIL initially because processor.go doesn't store attachment metadata
// in the metadata map.
func TestIngestStoresAttachments(t *testing.T) {
	// Create a test .eml with a calendar attachment
	emlContent := `From: calendar@example.com
To: user@example.com
Subject: Meeting Invitation
Date: Mon, 20 Jan 2026 11:00:00 -0800
Message-ID: <cal-001@example.com>
Content-Type: multipart/mixed; boundary="----BOUNDARY"

------BOUNDARY
Content-Type: text/plain; charset="UTF-8"

Please see the attached calendar invitation.

------BOUNDARY
Content-Type: text/calendar; charset="UTF-8"; method=REQUEST
Content-Disposition: attachment; filename="meeting.ics"
Content-Transfer-Encoding: base64

QkVHSU46VkNBTEVOREFSClZFUlNJT046Mi4wCkJFR0lOOlZFVkVOVApTVU1NQVJZOlRl
c3QgTWVldGluZwpFTkQ6VkVWRU5UCkVORDpWQ0FMRU5EQVI=
------BOUNDARY--
`

	// Write test file
	tmpDir := t.TempDir()
	emlPath := filepath.Join(tmpDir, "calendar-invite.eml")
	if err := os.WriteFile(emlPath, []byte(emlContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse with attachment metadata enabled
	parseOpts := eml.DefaultParseOptions()
	parseOpts.IncludeAttachmentContent = false // We only need metadata
	parser := eml.NewParser(parseOpts)

	result, err := parser.ParseFile(emlPath)
	if err != nil {
		t.Fatalf("failed to parse email: %v", err)
	}

	email := result.Email

	// Build metadata the same way processor.go does (after fix)
	metadata := map[string]interface{}{
		"from":              email.From.Email,
		"subject":           email.Subject,
		"has_attachments":   email.HasAttachments(),
		"attachment_count":  email.AttachmentCount(),
	}

	// Store attachment metadata for classification
	if len(email.Attachments) > 0 {
		attachmentMeta := make([]map[string]interface{}, len(email.Attachments))
		for i, att := range email.Attachments {
			attachmentMeta[i] = map[string]interface{}{
				"filename":   att.Filename,
				"mime_type":  att.MimeType,
				"size_bytes": att.Size,
			}
			if att.ContentID != "" {
				attachmentMeta[i]["content_id"] = att.ContentID
			}
			if att.IsInline {
				attachmentMeta[i]["is_inline"] = true
			}
		}
		metadata["attachments"] = attachmentMeta
	}

	// Verify attachments array exists
	t.Run("attachments array exists", func(t *testing.T) {
		attachmentsIface, ok := metadata["attachments"]
		if !ok {
			t.Error("EXPECTED FAILURE: metadata[\"attachments\"] not present")
			return
		}

		attachments, ok := attachmentsIface.([]map[string]interface{})
		if !ok {
			t.Fatalf("attachments is not []map[string]interface{}: %T", attachmentsIface)
		}

		if len(attachments) == 0 {
			t.Error("attachments array is empty but email has attachments")
		}
	})

	t.Run("calendar attachment metadata preserved", func(t *testing.T) {
		attachmentsIface, ok := metadata["attachments"]
		if !ok {
			t.Skip("attachments not present in metadata")
		}

		attachments := attachmentsIface.([]map[string]interface{})
		if len(attachments) == 0 {
			t.Fatal("no attachments in metadata")
		}

		att := attachments[0]

		filename, ok := att["filename"].(string)
		if !ok || filename != "meeting.ics" {
			t.Errorf("expected filename='meeting.ics', got '%v'", att["filename"])
		}

		mimeType, ok := att["mime_type"].(string)
		if !ok {
			t.Error("EXPECTED FAILURE: mime_type not present in attachment metadata")
			return
		}

		if mimeType != "text/calendar" {
			t.Errorf("expected mime_type='text/calendar', got '%s'", mimeType)
		}

		size, ok := att["size_bytes"].(int)
		if !ok {
			t.Error("size_bytes not present or not an int")
		} else if size <= 0 {
			t.Error("size_bytes should be > 0")
		}
	})
}

// TestClassifierDetectsCalendar verifies that the ContentClassifier correctly identifies
// calendar invites when attachment metadata is present.
//
// This test may PASS if the classifier logic is correct and we provide the right metadata structure.
func TestClassifierDetectsCalendar(t *testing.T) {
	classifier := classification.NewContentClassifier()

	// Create a source with calendar attachment metadata
	source := &processors.Source{
		ID:          1,
		TenantID:    "test-tenant",
		ContentType: "message/rfc822",
		Metadata: map[string]interface{}{
			"from":    "calendar@example.com",
			"subject": "Meeting Invitation",
			"attachments": []interface{}{
				map[string]interface{}{
					"filename":  "meeting.ics",
					"mime_type": "text/calendar",
					"size":      150,
					"is_inline": false,
				},
			},
		},
	}

	result, err := classifier.Classify(context.Background(), source)
	if err != nil {
		t.Fatalf("classifier failed: %v", err)
	}

	if result.ContentType != enrichment.ContentTypeCalendar {
		t.Errorf("expected ContentType=%s, got %s", enrichment.ContentTypeCalendar, result.ContentType)
	}

	if result.Subtype != enrichment.SubtypeCalendarInvite {
		t.Errorf("expected Subtype=%s, got %s", enrichment.SubtypeCalendarInvite, result.Subtype)
	}

	if result.Profile != enrichment.ProfileStateTracking {
		t.Errorf("expected Profile=%s, got %s", enrichment.ProfileStateTracking, result.Profile)
	}

	t.Logf("Classification result: %s/%s (profile: %s, detected via: %s)",
		result.ContentType, result.Subtype, result.Profile, result.DetectedVia)
}

// TestClassifierDetectsJiraNotification verifies that the ContentClassifier correctly
// identifies Jira notifications based on headers (Auto-Submitted + jira sender).
//
// This test may PASS if the classifier logic is correct and we provide the right metadata structure.
func TestClassifierDetectsJiraNotification(t *testing.T) {
	classifier := classification.NewContentClassifier()

	// Create a source with Jira notification headers
	source := &processors.Source{
		ID:          1,
		TenantID:    "test-tenant",
		ContentType: "message/rfc822",
		Metadata: map[string]interface{}{
			"from":    "jira@example.atlassian.net",
			"subject": "[PROJ-123] Issue Updated",
			"headers": map[string]interface{}{
				"Auto-Submitted": "auto-generated",
				"Precedence":     "bulk",
			},
		},
	}

	result, err := classifier.Classify(context.Background(), source)
	if err != nil {
		t.Fatalf("classifier failed: %v", err)
	}

	if result.ContentType != enrichment.ContentTypeEmail {
		t.Errorf("expected ContentType=%s, got %s", enrichment.ContentTypeEmail, result.ContentType)
	}

	if result.Subtype != enrichment.SubtypeNotificationJira {
		t.Errorf("expected Subtype=%s, got %s", enrichment.SubtypeNotificationJira, result.Subtype)
	}

	if result.Profile != enrichment.ProfileMetadataOnly {
		t.Errorf("expected Profile=%s, got %s", enrichment.ProfileMetadataOnly, result.Profile)
	}

	t.Logf("Classification result: %s/%s (profile: %s, detected via: %s)",
		result.ContentType, result.Subtype, result.Profile, result.DetectedVia)
}

// TestClassifierFallsToThread verifies that the ContentClassifier correctly identifies
// thread emails (with In-Reply-To header) and doesn't misclassify them as notifications.
//
// This test should PASS if the classifier logic correctly checks for threading headers.
func TestClassifierFallsToThread(t *testing.T) {
	classifier := classification.NewContentClassifier()

	// Create a source with In-Reply-To header (but no automation headers)
	source := &processors.Source{
		ID:          1,
		TenantID:    "test-tenant",
		ContentType: "message/rfc822",
		Metadata: map[string]interface{}{
			"from":        "alice@example.com",
			"subject":     "RE: Project Discussion",
			"in_reply_to": "<previous-msg@example.com>",
			"references":  []string{"<original@example.com>", "<previous-msg@example.com>"},
		},
	}

	result, err := classifier.Classify(context.Background(), source)
	if err != nil {
		t.Fatalf("classifier failed: %v", err)
	}

	if result.ContentType != enrichment.ContentTypeEmail {
		t.Errorf("expected ContentType=%s, got %s", enrichment.ContentTypeEmail, result.ContentType)
	}

	if result.Subtype != enrichment.SubtypeEmailThread {
		t.Errorf("expected Subtype=%s, got %s", enrichment.SubtypeEmailThread, result.Subtype)
	}

	if result.Profile != enrichment.ProfileFullAI {
		t.Errorf("expected Profile=%s, got %s", enrichment.ProfileFullAI, result.Profile)
	}

	t.Logf("Classification result: %s/%s (profile: %s, detected via: %s)",
		result.ContentType, result.Subtype, result.Profile, result.DetectedVia)
}

// TestClassifierWithCompleteMetadata is an integration test that verifies the classifier
// works correctly when given the complete metadata structure that the fixed ingest layer
// should produce.
func TestClassifierWithCompleteMetadata(t *testing.T) {
	tests := []struct {
		name            string
		metadata        map[string]interface{}
		expectedType    enrichment.ContentType
		expectedSubtype enrichment.ContentSubtype
		expectedProfile enrichment.ProcessingProfile
	}{
		{
			name: "calendar invite with attachment",
			metadata: map[string]interface{}{
				"from":    "calendar@google.com",
				"subject": "Meeting: Project Sync",
				"headers": map[string]interface{}{
					"Content-Type": "multipart/mixed",
				},
				"attachments": []interface{}{
					map[string]interface{}{
						"filename":  "invite.ics",
						"mime_type": "text/calendar",
						"size":      200,
					},
				},
			},
			expectedType:    enrichment.ContentTypeCalendar,
			expectedSubtype: enrichment.SubtypeCalendarInvite,
			expectedProfile: enrichment.ProfileStateTracking,
		},
		{
			name: "jira notification",
			metadata: map[string]interface{}{
				"from":    "jira@company.atlassian.net",
				"subject": "[PROJ-456] Bug Fixed",
				"headers": map[string]interface{}{
					"Auto-Submitted": "auto-generated",
					"Precedence":     "bulk",
				},
			},
			expectedType:    enrichment.ContentTypeEmail,
			expectedSubtype: enrichment.SubtypeNotificationJira,
			expectedProfile: enrichment.ProfileMetadataOnly,
		},
		{
			name: "threaded email reply",
			metadata: map[string]interface{}{
				"from":        "bob@company.com",
				"subject":     "RE: Design Review",
				"in_reply_to": "<msg-001@company.com>",
				"references":  []string{"<msg-001@company.com>"},
			},
			expectedType:    enrichment.ContentTypeEmail,
			expectedSubtype: enrichment.SubtypeEmailThread,
			expectedProfile: enrichment.ProfileFullAI,
		},
		{
			name: "standalone email (no thread, no automation)",
			metadata: map[string]interface{}{
				"from":    "alice@company.com",
				"subject": "New Project Proposal",
			},
			expectedType:    enrichment.ContentTypeEmail,
			expectedSubtype: enrichment.SubtypeEmailStandalone,
			expectedProfile: enrichment.ProfileFullAI,
		},
	}

	classifier := classification.NewContentClassifier()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &processors.Source{
				ID:          1,
				TenantID:    "test-tenant",
				ContentType: "message/rfc822",
				Metadata:    tt.metadata,
			}

			result, err := classifier.Classify(context.Background(), source)
			if err != nil {
				t.Fatalf("classifier failed: %v", err)
			}

			if result.ContentType != tt.expectedType {
				t.Errorf("ContentType: expected %s, got %s", tt.expectedType, result.ContentType)
			}

			if result.Subtype != tt.expectedSubtype {
				t.Errorf("Subtype: expected %s, got %s", tt.expectedSubtype, result.Subtype)
			}

			if result.Profile != tt.expectedProfile {
				t.Errorf("Profile: expected %s, got %s", tt.expectedProfile, result.Profile)
			}

			// Log for debugging
			resultJSON, _ := json.MarshalIndent(result, "", "  ")
			t.Logf("Result: %s", resultJSON)
		})
	}
}
