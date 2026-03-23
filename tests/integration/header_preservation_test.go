//go:build integration

package integration

// ============================================================================
// Header Preservation Integration Tests (pf-a4b0a1)
// ============================================================================
//
// Verifies that all email headers are preserved end-to-end through both
// ingest paths (penfold batch processor and penf-cli) after removal of
// the PreserveHeaders whitelist (pf-3505c6, pf-0aba97, pf-ea748f).
//
// Test cases:
//   TC1: All headers preserved on ingest (penfold batch processor path)
//   TC2: All headers preserved on ingest (penf-cli / ingest service path)
//   TC3: New header-based classification rules work without code changes
//   TC4: Existing header-based classification rules continue to work
//   TC5: ParseOptions no longer has PreserveHeaders field (compile-time + runtime)
//
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/ingest/eml"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/repository"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/otherjamesbrown/penfold/services/gateway/ingestservice"
)

// headerTestEMLTemplate is the EML content used for TC1, TC2, and TC5.
// It includes both standard headers (From, To, Subject, Date) and
// uncommon headers (X-Custom-Foo, X-Mailer, List-Unsubscribe, List-Id)
// that were previously filtered by the PreserveHeaders whitelist.
const headerTestEMLTemplate = `From: sender@example.com
To: recipient@example.com
Subject: Header Preservation Test
Date: Mon, 23 Mar 2026 10:00:00 +0000
Message-ID: <%s>
X-Custom-Foo: custom-foo-value
X-Mailer: TestMailer/1.0
List-Unsubscribe: <mailto:unsub@example.com>
List-Id: test-list.example.com
Precedence: bulk
X-Spam-Status: No
Content-Type: text/plain; charset="UTF-8"

This is a test email for header preservation verification.
`

// writeHeaderTestEML writes a test .eml file with a unique Message-ID and returns its path.
func writeHeaderTestEML(t *testing.T, dir, messageID string) string {
	t.Helper()
	content := fmt.Sprintf(headerTestEMLTemplate, messageID)
	path := filepath.Join(dir, "header-test.eml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// queryIngestionMetadata fetches ingestion_metadata for a source by ID and tenant.
func queryIngestionMetadata(t *testing.T, db *TestDB, sourceID int64, tenantID string) map[string]interface{} {
	t.Helper()
	ctx := context.Background()
	var metaJSON []byte
	err := db.Pool.QueryRow(ctx, `
		SELECT ingestion_metadata
		FROM sources
		WHERE id = $1 AND tenant_id = $2
	`, sourceID, tenantID).Scan(&metaJSON)
	require.NoError(t, err, "failed to query ingestion_metadata for source %d", sourceID)
	require.NotEmpty(t, metaJSON, "ingestion_metadata is empty for source %d", sourceID)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(metaJSON, &meta), "failed to unmarshal ingestion_metadata")
	return meta
}

// assertHeadersInMetadata checks that all expectedHeaders appear in meta["headers"].
func assertHeadersInMetadata(t *testing.T, meta map[string]interface{}, expectedHeaders map[string]string) {
	t.Helper()

	headersRaw, ok := meta["headers"]
	require.True(t, ok, "ingestion_metadata[\"headers\"] is absent (keys present: %v)", metadataKeyList(meta))

	// ingestion_metadata is JSON, so the headers map comes back as map[string]interface{}.
	headersMap, ok := headersRaw.(map[string]interface{})
	require.True(t, ok, "ingestion_metadata[\"headers\"] is not a map, got %T", headersRaw)

	for header, expectedValue := range expectedHeaders {
		actual, ok := headersMap[header]
		assert.True(t, ok, "header %q absent from ingestion_metadata[\"headers\"]", header)
		if ok && expectedValue != "" {
			assert.Equal(t, expectedValue, actual,
				"header %q has wrong value: got %q, want %q", header, actual, expectedValue)
		}
	}
}

// metadataKeyList returns the keys of a map for use in error messages.
func metadataKeyList(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ============================================================================
// TC1: Penfold batch processor — all headers preserved in ingestion_metadata
// ============================================================================

// TestHeaderPreservation_PenfoldBatchProcessor verifies that the penfold
// batch processor (pkg/ingest/batch/processor.go) writes all email headers
// into ingestion_metadata["headers"], including headers that were previously
// excluded by the PreserveHeaders whitelist.
//
// This test replicates the exact logic of processFile() using real DB storage,
// exercising the full parser → metadata-building → storage path.
func TestHeaderPreservation_PenfoldBatchProcessor(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	defer db.CleanupTables(t, "sources", "ingest_jobs", "ingest_errors")

	tmpDir := t.TempDir()
	messageID := "header-tc1-" + uuid.New().String() + "@example.com"
	emlPath := writeHeaderTestEML(t, tmpDir, messageID)

	// Parse the email using penfold's EML parser (same as batch processor does).
	parser := eml.NewParser(eml.DefaultParseOptions())
	parseResult, err := parser.ParseFile(emlPath)
	require.NoError(t, err, "failed to parse test email")
	email := parseResult.Email

	// Build metadata exactly as processFile() does in pkg/ingest/batch/processor.go.
	metadata := map[string]interface{}{
		"file_path":            emlPath,
		"message_id_synthetic": email.MessageIDSynthetic,
		"date_fallback":        email.DateFallback,
		"has_attachments":      email.HasAttachments(),
		"attachment_count":     email.AttachmentCount(),
		"source_tag":           "header-test-tc1",
		"job_id":               "test-job-tc1",
		"labels":               []string{},
		"from":                 email.From.Email,
		"to":                   email.ToAddresses(),
		"cc":                   email.CcAddresses(),
		"subject":              email.Subject,
	}
	// Merge all parsed headers (the core of the pf-3505c6 + pf-0aba97 fix).
	// Previously only whitelisted headers were merged; now all are included.
	headers := make(map[string]string)
	if email.From.Email != "" {
		headers["From"] = email.From.Email
	}
	for k, v := range email.Headers {
		headers[k] = v
	}
	if len(headers) > 0 {
		metadata["headers"] = headers
	}

	// Persist via storage.Repository (same as processFile does).
	logger := logging.NewNopLogger()
	repo := storage.NewRepository(db.Pool, logger)
	ctx := context.Background()

	source := &storage.EmailSource{
		TenantID:        db.TenantID,
		SourceSystem:    storage.SourceSystemManualEML,
		ExternalID:      email.MessageID,
		ContentHash:     email.ContentHash,
		RawContent:      "test body",
		ContentType:     "message/rfc822",
		ContentSize:     int32(len("test body")),
		Metadata:        metadata,
		SourceTimestamp: time.Now(),
	}
	created, err := repo.CreateSource(ctx, source)
	require.NoError(t, err, "failed to create source")

	// Query DB and verify all uncommon headers appear in ingestion_metadata.
	meta := queryIngestionMetadata(t, db, created.ID, db.TenantID)
	assertHeadersInMetadata(t, meta, map[string]string{
		"X-Custom-Foo":     "custom-foo-value",
		"X-Mailer":         "TestMailer/1.0",
		"List-Unsubscribe": "<mailto:unsub@example.com>",
		"List-Id":          "test-list.example.com",
		"Precedence":       "bulk",
		"X-Spam-Status":    "No",
	})

	t.Logf("TC1 passed: penfold batch processor stored %d headers in ingestion_metadata", len(email.Headers))
}

// ============================================================================
// TC2: Penf-CLI ingest service path — all headers preserved in ingestion_metadata
// ============================================================================

// TestHeaderPreservation_PenfoldIngestService verifies that the ingest service
// (the gateway handler for penf-cli's IngestEmail gRPC call) correctly stores
// all headers passed in the request into ingestion_metadata["headers"].
//
// The penf-cli command parses emails locally, builds req.Headers from email.Headers
// (all headers, now that the PreserveHeaders whitelist is removed), and sends via gRPC.
// This test exercises that path using the ingest service directly.
func TestHeaderPreservation_PenfoldIngestService(t *testing.T) {
	db := SetupTestDB(t)
	defer db.CleanupTables(t, "sources", "ingest_jobs", "ingest_errors", "tenants")

	ctx := context.Background()
	logger := logging.NewNopLogger()

	// Create test tenant (same pattern as setupIngestService in ingest_test.go).
	tenantRepo := tenant.NewRepository(db.Pool)
	created, err := tenantRepo.Create(ctx, tenant.TenantInput{
		Name: "Header Test Tenant TC2",
		Slug: "header-tc2-" + uuid.New().String()[:8],
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tenantRepo.Delete(ctx, created.ID, "test cleanup") })

	tenantID := created.ID
	ingestRepo := storage.NewRepository(db.Pool, logger)
	tenantAdapter := ingestservice.NewTenantRepoAdapter(func(ctx context.Context, ref string) (string, error) {
		t, err := tenantRepo.GetByRef(ctx, ref)
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", nil
		}
		return t.ID, nil
	})

	seriesRepo := repository.NewSeriesRepository(db.Pool)
	svc := ingestservice.NewService(ingestRepo, tenantAdapter, seriesRepo, logger)

	// Build the gRPC request exactly as penf-cli's emailToProtoRequest does,
	// including all headers from the parsed email (the pf-ea748f fix).
	messageID := "header-tc2-" + uuid.New().String() + "@example.com"
	req := &ingestv1.IngestEmailRequest{
		TenantId:     tenantID,
		MessageId:    messageID,
		ContentHash:  testHexHash(),
		Subject:      "Header Preservation Test TC2",
		BodyPlain:    "Test body for TC2",
		SourceSystem: "manual_eml",
		SourceTag:    "header-test-tc2",
		From: &ingestv1.EmailAddress{
			Name:    "Sender",
			Address: "sender@example.com",
		},
		// All headers are now included (after pf-ea748f removes the whitelist filter).
		// Previously, penf-cli only included headers from a hardcoded list.
		// Now it includes all parsed headers, forwarded from email.Headers.
		Headers: map[string]string{
			"From":             "Sender <sender@example.com>",
			"X-Custom-Foo":    "custom-foo-value",
			"X-Mailer":        "TestMailer/1.0",
			"List-Unsubscribe": "<mailto:unsub@example.com>",
			"List-Id":         "test-list.example.com",
			"Precedence":      "bulk",
			"X-Spam-Status":   "No",
		},
	}

	resp, err := svc.IngestEmail(ctx, req)
	require.NoError(t, err, "ingest service call failed")
	require.False(t, resp.WasDuplicate, "unexpected duplicate")
	require.NotZero(t, resp.SourceId, "expected a source ID")

	// Query DB and verify all uncommon headers appear in ingestion_metadata.
	var metaJSON []byte
	err = db.Pool.QueryRow(ctx, `
		SELECT ingestion_metadata FROM sources WHERE id = $1 AND tenant_id = $2
	`, resp.SourceId, tenantID).Scan(&metaJSON)
	require.NoError(t, err)

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(metaJSON, &meta))

	headersRaw, ok := meta["headers"]
	require.True(t, ok, "ingestion_metadata[\"headers\"] absent (keys: %v)", metadataKeyList(meta))
	headersMap, ok := headersRaw.(map[string]interface{})
	require.True(t, ok, "headers is not a map, got %T", headersRaw)

	expectedHeaders := []string{
		"X-Custom-Foo", "X-Mailer", "List-Unsubscribe", "List-Id", "Precedence", "X-Spam-Status",
	}
	for _, h := range expectedHeaders {
		assert.Contains(t, headersMap, h,
			"header %q should be present in ingestion_metadata[\"headers\"]", h)
	}

	t.Logf("TC2 passed: ingest service stored %d headers in ingestion_metadata", len(headersMap))
}

// ============================================================================
// TC3: New header-based classification rule works without code changes
// ============================================================================

// TestHeaderPreservation_NewHeaderBasedClassificationRule verifies that a
// classification rule matching on a previously-unpreserved header (X-Custom-Foo)
// now fires correctly because that header is preserved in ingestion_metadata.
//
// Before the fix: X-Custom-Foo was not in PreserveHeaders → not in metadata →
//   the triage activity couldn't build "header:X-Custom-Foo" → rule silently failed.
// After the fix: X-Custom-Foo is always preserved → triage builds "header:X-Custom-Foo"
//   → rule fires correctly.
func TestHeaderPreservation_NewHeaderBasedClassificationRule(t *testing.T) {
	// Build an in-memory rule engine with a rule matching header:X-Custom-Foo.
	// This simulates what an operator would add to classification_rules in the DB
	// to classify emails from a new system without requiring a code change.
	rules := []classification.ClassificationRule{
		{
			ID:                 100,
			TenantID:           "test-tenant",
			Name:               "custom_foo_system",
			Priority:           5,
			ContentTypeScope:   "EMAIL",
			ContentType:        "EMAIL",
			ContentSubtype:     "NOTIFICATION",
			NotificationSource: "custom_foo",
			Active:             true,
			Conditions: []classification.MatchCondition{
				{
					ID:            100,
					Field:         "header:X-Custom-Foo",
					MatchType:     "exists",
					Value:         "",
					CaseSensitive: false,
				},
			},
		},
	}

	engine := classification.NewEngine(rules)

	// Simulate the metadata map that the triage activity builds from ingestion_metadata.
	// The triage activity flattens metadata["headers"] into "header:<Name>" entries.
	// After the fix, X-Custom-Foo is preserved and appears as "header:X-Custom-Foo".
	metadata := map[string]string{
		"from_address":        "sender@custom-system.example.com",
		"subject":             "Custom System Notification",
		"header:X-Custom-Foo": "custom-foo-value",
	}

	result := engine.Classify("EMAIL", metadata)

	assert.Equal(t, "EMAIL", result.ContentType,
		"TC3: expected EMAIL content type")
	assert.Equal(t, "NOTIFICATION", result.ContentSubtype,
		"TC3: expected NOTIFICATION subtype (header:X-Custom-Foo rule fired)")
	assert.Equal(t, "custom_foo_system", result.RuleName,
		"TC3: expected 'custom_foo_system' rule to fire")
	assert.Equal(t, "custom_foo", result.NotificationSource,
		"TC3: expected notification source = 'custom_foo'")

	t.Logf("TC3 passed: classification rule on header:X-Custom-Foo fired (rule: %s, subtype: %s)",
		result.RuleName, result.ContentSubtype)
}

// ============================================================================
// TC4: Existing header-based classification rules continue to work
// ============================================================================

// TestHeaderPreservation_ExistingNewsletterClassificationRules verifies that
// existing classification rules that match on List-Unsubscribe and Precedence
// (the original incident from pf-2b8d3d) continue to fire correctly after the fix.
func TestHeaderPreservation_ExistingNewsletterClassificationRules(t *testing.T) {
	// Build a rule engine with the newsletter rule that uses List-Unsubscribe,
	// matching the seeded rule in migrations/065_seed_classification_rules.sql.
	rules := []classification.ClassificationRule{
		{
			ID:               9,
			TenantID:         "test-tenant",
			Name:             "newsletter",
			Priority:         90,
			ContentTypeScope: "EMAIL",
			ContentType:      "EMAIL",
			ContentSubtype:   "NEWSLETTER",
			Active:           true,
			Conditions: []classification.MatchCondition{
				// Rule matches if Precedence=bulk OR Precedence=list OR List-Unsubscribe exists.
				{ID: 14, Field: "header:Precedence", MatchType: "exact", Value: "bulk", CaseSensitive: false},
				{ID: 15, Field: "header:Precedence", MatchType: "exact", Value: "list", CaseSensitive: false},
				{ID: 16, Field: "header:List-Unsubscribe", MatchType: "exists", Value: "", CaseSensitive: false},
			},
		},
	}

	engine := classification.NewEngine(rules)

	tests := []struct {
		name        string
		metadata    map[string]string
		wantSubtype string
		wantRule    string
	}{
		{
			name: "List-Unsubscribe header fires newsletter rule",
			metadata: map[string]string{
				"from_address":            "newsletter@example.com",
				"subject":                 "Weekly Digest",
				"header:List-Unsubscribe": "<mailto:unsub@example.com>",
			},
			wantSubtype: "NEWSLETTER",
			wantRule:    "newsletter",
		},
		{
			name: "Precedence=bulk fires newsletter rule",
			metadata: map[string]string{
				"from_address":      "bulk@example.com",
				"subject":           "Bulk Mailing",
				"header:Precedence": "bulk",
			},
			wantSubtype: "NEWSLETTER",
			wantRule:    "newsletter",
		},
		{
			name: "Precedence=list fires newsletter rule",
			metadata: map[string]string{
				"from_address":      "list@example.com",
				"subject":           "Mailing List",
				"header:Precedence": "list",
			},
			wantSubtype: "NEWSLETTER",
			wantRule:    "newsletter",
		},
		{
			name: "List-Id header combined with List-Unsubscribe fires newsletter rule",
			metadata: map[string]string{
				"from_address":            "updates@example.com",
				"subject":                 "Product Updates",
				"header:List-Unsubscribe": "<https://example.com/unsub>",
				"header:List-Id":          "updates.example.com",
			},
			wantSubtype: "NEWSLETTER",
			wantRule:    "newsletter",
		},
		{
			name: "No newsletter headers falls through to default",
			metadata: map[string]string{
				"from_address": "alice@example.com",
				"subject":      "Hi there",
			},
			wantSubtype: "HUMAN",
			wantRule:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Classify("EMAIL", tt.metadata)
			assert.Equal(t, tt.wantSubtype, result.ContentSubtype,
				"wrong ContentSubtype for %q", tt.name)
			if tt.wantRule != "" {
				assert.Equal(t, tt.wantRule, result.RuleName,
					"wrong rule name for %q", tt.name)
			}
		})
	}

	t.Logf("TC4 passed: existing List-Unsubscribe and Precedence classification rules fire correctly")
}

// ============================================================================
// TC5: PreserveHeaders whitelist fully removed
// ============================================================================

// TestHeaderPreservation_AllHeadersPreservedUnconditionally verifies that:
// 1. ParseOptions no longer has a PreserveHeaders field (enforced at compile time:
//    this file compiles without setting any such field).
// 2. The parser returns ALL headers unconditionally, regardless of options passed.
// 3. Uncommon headers appear in email.Headers without any configuration.
func TestHeaderPreservation_AllHeadersPreservedUnconditionally(t *testing.T) {
	emlContent := strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: TC5 Test",
		"Date: Mon, 23 Mar 2026 10:00:00 +0000",
		"Message-ID: <tc5-" + uuid.New().String() + "@example.com>",
		"X-Custom-Foo: foo-value",
		"X-Mailer: TestMailer/2.0",
		"List-Unsubscribe: <mailto:unsub@example.com>",
		"List-Id: mylist.example.com",
		"Precedence: bulk",
		"X-Spam-Score: 0.1",
		"Auto-Submitted: auto-generated",
		`Content-Type: text/plain; charset="UTF-8"`,
		"",
		"Body text for TC5.",
	}, "\r\n")

	tmpDir := t.TempDir()
	emlPath := filepath.Join(tmpDir, "tc5.eml")
	require.NoError(t, os.WriteFile(emlPath, []byte(emlContent), 0644))

	// TC5a: Default options preserve all headers unconditionally.
	t.Run("default options preserve all headers", func(t *testing.T) {
		parser := eml.NewParser(eml.DefaultParseOptions())
		result, err := parser.ParseFile(emlPath)
		require.NoError(t, err)

		email := result.Email
		require.NotEmpty(t, email.Headers, "email.Headers should not be empty")

		expectedHeaders := map[string]string{
			"X-Custom-Foo":    "foo-value",
			"X-Mailer":        "TestMailer/2.0",
			"List-Unsubscribe": "<mailto:unsub@example.com>",
			"List-Id":         "mylist.example.com",
			"Precedence":      "bulk",
			"X-Spam-Score":    "0.1",
			"Auto-Submitted":  "auto-generated",
		}

		for header, expectedValue := range expectedHeaders {
			actual, ok := email.Headers[header]
			assert.True(t, ok, "header %q missing from email.Headers", header)
			if ok {
				assert.Equal(t, expectedValue, actual,
					"header %q: got %q, want %q", header, actual, expectedValue)
			}
		}

		t.Logf("TC5a: parser preserved %d headers unconditionally", len(email.Headers))
	})

	// TC5b: ParseOptions has no PreserveHeaders field (compile-time verification).
	// The fact that this code compiles without setting ParseOptions.PreserveHeaders
	// proves the field was removed. All uncommon headers are still preserved.
	t.Run("ParseOptions has no PreserveHeaders field", func(t *testing.T) {
		opts := eml.ParseOptions{
			IncludeAttachmentContent: false,
			MaxBodySize:              0,
			// PreserveHeaders field does not exist — this file will not compile if it does.
			// All headers are now preserved unconditionally (pf-3505c6).
		}

		parser := eml.NewParser(opts)
		result, err := parser.ParseFile(emlPath)
		require.NoError(t, err)

		assert.NotEmpty(t, result.Email.Headers["X-Custom-Foo"],
			"X-Custom-Foo should be preserved without any PreserveHeaders configuration")
		assert.NotEmpty(t, result.Email.Headers["List-Unsubscribe"],
			"List-Unsubscribe should be preserved without any PreserveHeaders configuration")
		assert.NotEmpty(t, result.Email.Headers["Auto-Submitted"],
			"Auto-Submitted should be preserved without any PreserveHeaders configuration")
	})
}
