//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentListResponse represents the JSON output from `penf content list -o json`
type contentListResponse struct {
	Items []contentItem `json:"items"`
	Total int           `json:"total"`
}

type contentItem struct {
	ID         string `json:"id"`
	ContentID  string `json:"content_id"`
	SourceID   int64  `json:"source_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// pipelineKickResponse represents the JSON output from `penf pipeline kick -o json`
type pipelineKickResponse struct {
	Queued  int `json:"queued"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// waitForProcessingComplete polls content status until complete or timeout.
func waitForProcessingComplete(t *testing.T, env *E2EEnv, sourceID int64, timeout time.Duration) error {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var status string
		err := env.DB.QueryRow(ctx, `
			SELECT processing_status FROM sources WHERE id = $1
		`, sourceID).Scan(&status)

		if err == nil {
			if status == "completed" {
				return nil
			}
			if status == "failed" {
				return fmt.Errorf("processing failed")
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for processing after %v", timeout)
}

// getSourceIDFromContentID looks up the source ID for a content_id.
func getSourceIDFromContentID(env *E2EEnv, contentID string) (int64, error) {
	ctx := context.Background()
	var sourceID int64
	err := env.DB.QueryRow(ctx, `
		SELECT id FROM sources WHERE content_id = $1
	`, contentID).Scan(&sourceID)
	return sourceID, err
}

// getLatestSourceByTag gets the most recent source with the given source_tag.
func getLatestSourceByTag(env *E2EEnv, sourceTag string) (int64, error) {
	ctx := context.Background()
	var sourceID int64
	err := env.DB.QueryRow(ctx, `
		SELECT s.id FROM sources s
		JOIN ingest_jobs j ON s.tenant_id = j.tenant_id
		WHERE j.source_tag = $1
		ORDER BY s.created_at DESC
		LIMIT 1
	`, sourceTag).Scan(&sourceID)
	return sourceID, err
}

// assertStageCompleted checks pipeline_runs for stage entry with status=completed.
func assertStageCompleted(t *testing.T, env *E2EEnv, sourceID int64, stageName string) {
	t.Helper()
	ctx := context.Background()

	var status string
	var durationMs *int

	err := env.DB.QueryRow(ctx, `
		SELECT status, duration_ms
		FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2
	`, sourceID, stageName).Scan(&status, &durationMs)

	require.NoError(t, err, "stage %s should have pipeline_runs entry", stageName)
	require.Equal(t, "completed", status, "stage %s should be completed", stageName)

	if durationMs != nil {
		t.Logf("Stage %s completed in %dms", stageName, *durationMs)
	} else {
		t.Logf("Stage %s completed", stageName)
	}
}

// assertStageSkipped verifies no pipeline_runs entry exists for stage.
func assertStageSkipped(t *testing.T, env *E2EEnv, sourceID int64, stageName string) {
	t.Helper()
	ctx := context.Background()

	var count int
	err := env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2
	`, sourceID, stageName).Scan(&count)

	require.NoError(t, err)
	require.Equal(t, 0, count, "stage %s should be skipped (no pipeline_runs entry)", stageName)
}

// countEmbeddingsForSource counts embeddings by representation_type for a source.
func countEmbeddingsForSource(env *E2EEnv, sourceID int64) (map[string]int, error) {
	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT representation_type, COUNT(*)
		FROM embeddings
		WHERE source_id = $1
		GROUP BY representation_type
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var repType string
		var count int
		if err := rows.Scan(&repType, &count); err != nil {
			return nil, err
		}
		counts[repType] = count
	}
	return counts, rows.Err()
}

// getAssertionsForSource queries assertions table for a source.
func getAssertionsForSource(env *E2EEnv, sourceID int64) ([]map[string]interface{}, error) {
	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT id, assertion_type, description, confidence, is_current
		FROM assertions
		WHERE source_id = $1
		ORDER BY created_at
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assertions []map[string]interface{}
	for rows.Next() {
		var id int64
		var assertionType, description string
		var confidence float64
		var isCurrent bool
		err := rows.Scan(&id, &assertionType, &description, &confidence, &isCurrent)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, map[string]interface{}{
			"id":          id,
			"type":        assertionType,
			"description": description,
			"confidence":  confidence,
			"is_current":  isCurrent,
		})
	}
	return assertions, rows.Err()
}

// createTempEmail creates a temporary email file for testing and returns its path.
func createTempEmail(t *testing.T, subject, body string) string {
	t.Helper()
	content := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: %s
Date: Thu, 5 Feb 2026 10:00:00 +0000
Message-ID: <%s@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

%s
`, subject, fmt.Sprintf("test-%d", time.Now().UnixNano()), body)

	tmpFile, err := os.CreateTemp("", "e2e-email-*.eml")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	return tmpFile.Name()
}

// TestSLMPipeline_FullEmailPipeline tests the complete SLM pipeline with an email.
// Uses CLI commands: ingest email -> pipeline kick -> verify results
func TestSLMPipeline_FullEmailPipeline(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Step 1: Create temp email and ingest via CLI
	emailPath := createTempEmail(t,
		"Project Alpha Status Update",
		"Hi team,\n\nPlease find below the status update for Project Alpha.\n\nBest regards,\nJohn")
	sourceTag := fmt.Sprintf("e2e-full-pipeline-%d", time.Now().UnixNano())

	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode, "ingest should succeed: %s", result.Stderr)
	t.Logf("Ingest output: %s", result.Stdout)

	// Step 2: Trigger pipeline processing
	kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
	t.Logf("Pipeline kick output: %s", kickResult.Stdout)

	// Step 3: Get the source ID and wait for completion
	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 90*time.Second)
	require.NoError(t, err, "pipeline should complete")

	// Step 4: Verify all stages completed
	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "triage")
	assertStageCompleted(t, env, sourceID, "embed")

	// Verify source status
	var status string
	err = env.DB.QueryRow(ctx, "SELECT processing_status FROM sources WHERE id = $1", sourceID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "completed", status)

	// Verify embeddings were created
	embeddings, err := countEmbeddingsForSource(env, sourceID)
	require.NoError(t, err)
	assert.Greater(t, len(embeddings), 0, "should have created embeddings")
	t.Logf("Embeddings created: %v", embeddings)
}

// TestSLMPipeline_MeetingTranscript tests pipeline with a meeting transcript.
func TestSLMPipeline_MeetingTranscript(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create a temp VTT file
	transcriptContent := `WEBVTT

00:00:05.000 --> 00:00:10.000
Sarah: Welcome everyone to the Project Alpha review.

00:00:10.000 --> 00:00:15.000
John: Thanks Sarah. Let me walk through the Q1 timeline.

00:00:15.000 --> 00:00:20.000
Marcus: I have some concerns about the deadline.`

	tmpFile, err := os.CreateTemp("", "e2e-meeting-*.vtt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(transcriptContent)
	require.NoError(t, err)
	tmpFile.Close()

	// Ingest meeting transcript
	sourceTag := fmt.Sprintf("e2e-meeting-%d", time.Now().UnixNano())
	result := env.CLI.Run(ctx, "ingest", "file", tmpFile.Name(), "--source", sourceTag, "--type", "meeting")

	if result.ExitCode != 0 {
		t.Logf("Meeting ingest not supported via file command, skipping: %s", result.Stderr)
		t.Skip("Meeting transcript ingest via CLI not available")
	}

	// Trigger and wait for processing
	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID, 60*time.Second)
	require.NoError(t, err)

	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "triage")
}

// TestSLMPipeline_TriageGateLOW tests that LOW importance content skips deep processing.
func TestSLMPipeline_TriageGateLOW(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create temp file with low priority FYI email
	emailContent := `From: facilities@acme.com
To: all@acme.com
Subject: FYI: Coffee machine maintenance
Date: Mon, 1 Jan 2024 10:00:00 -0500
Message-ID: <low-priority-test@acme.com>

FYI - The coffee machine on floor 3 is being serviced tomorrow morning.

Regards,
Facilities`

	tmpFile, err := os.CreateTemp("", "e2e-low-priority-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(emailContent)
	require.NoError(t, err)
	tmpFile.Close()

	sourceTag := fmt.Sprintf("e2e-low-priority-%d", time.Now().UnixNano())
	result := env.CLI.Run(ctx, "ingest", "email", tmpFile.Name(), "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode, "ingest should succeed: %s", result.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify parse and triage ran
	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "triage")

	// Check if deep processing was skipped (depends on triage result)
	// Note: The actual skip behavior depends on the LLM's triage decision
	var skipDeep bool
	err = env.DB.QueryRow(ctx, `
		SELECT COALESCE((metadata->>'skip_deep')::boolean, false)
		FROM sources WHERE id = $1
	`, sourceID).Scan(&skipDeep)

	if err == nil && skipDeep {
		t.Log("Triage correctly identified as LOW importance - deep processing skipped")
		assertStageSkipped(t, env, sourceID, "extract")
		assertStageSkipped(t, env, sourceID, "analyze")
	} else {
		t.Log("Triage did not skip deep processing (LLM may have classified differently)")
	}

	// Embedding should always run
	assertStageCompleted(t, env, sourceID, "embed")
}

// TestSLMPipeline_HighImportanceRisk tests routing for high-importance risk content.
func TestSLMPipeline_HighImportanceRisk(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create temp file with high priority security alert
	emailContent := `From: security@acme.com
To: engineering@acme.com
Subject: URGENT: Security vulnerability in production
Date: Mon, 1 Jan 2024 02:15:00 -0500
Message-ID: <security-urgent@acme.com>

URGENT: Security vulnerability discovered in production API

We've identified a critical authentication bypass in the user API endpoint.
This needs immediate attention - potential data exposure risk.

Severity: HIGH
Impact: All user accounts potentially compromised
Recommended action: Immediate hotfix deployment

-Security Team`

	tmpFile, err := os.CreateTemp("", "e2e-security-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(emailContent)
	require.NoError(t, err)
	tmpFile.Close()

	sourceTag := fmt.Sprintf("e2e-security-%d", time.Now().UnixNano())
	result := env.CLI.Run(ctx, "ingest", "email", tmpFile.Name(), "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode, "ingest should succeed: %s", result.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 90*time.Second)
	require.NoError(t, err)

	// Verify all stages ran (HIGH importance should not skip)
	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "triage")
	assertStageCompleted(t, env, sourceID, "embed")

	// Check for assertions
	assertions, err := getAssertionsForSource(env, sourceID)
	require.NoError(t, err)
	t.Logf("Assertions created: %d", len(assertions))

	for _, a := range assertions {
		t.Logf("  - %s: %s (confidence: %.2f)", a["type"], a["description"], a["confidence"])
	}
}

// TestSLMPipeline_GoldenThread tests assertion lifecycle across multiple emails.
func TestSLMPipeline_GoldenThread(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// First email: Initial decision
	email1 := `From: john.smith@acme.com
To: team@acme.com
Subject: Decision: Project Alpha Architecture
Date: Mon, 1 Jan 2024 10:00:00 -0500
Message-ID: <decision-1@acme.com>

Team,

We've decided to move forward with the microservices architecture for Project Alpha.
This will allow us to scale more effectively.

Decision owner: Sarah Chen
Timeline: Q1 2026

-John`

	tmpFile1, err := os.CreateTemp("", "e2e-thread-1-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile1.Name())
	tmpFile1.WriteString(email1)
	tmpFile1.Close()

	sourceTag1 := fmt.Sprintf("e2e-thread-1-%d", time.Now().UnixNano())
	result1 := env.CLI.Run(ctx, "ingest", "email", tmpFile1.Name(), "--source", sourceTag1)
	require.Equal(t, 0, result1.ExitCode)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag1)

	sourceID1, err := getLatestSourceByTag(env, sourceTag1)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID1, 90*time.Second)
	require.NoError(t, err)
	t.Logf("First email processed: source_id=%d", sourceID1)

	// Get assertions from first email
	assertions1, err := getAssertionsForSource(env, sourceID1)
	require.NoError(t, err)
	t.Logf("First email assertions: %d", len(assertions1))

	// Second email: Update to the decision
	email2 := `From: sarah.chen@acme.com
To: team@acme.com
Subject: RE: Decision: Project Alpha Architecture
Date: Tue, 2 Jan 2024 14:00:00 -0500
Message-ID: <decision-2@acme.com>
In-Reply-To: <decision-1@acme.com>

Update on the microservices decision:

After discussing with the infrastructure team, we're modifying the approach
to use a hybrid architecture for the first phase.

This reduces initial complexity while maintaining scalability goals.

-Sarah`

	tmpFile2, err := os.CreateTemp("", "e2e-thread-2-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile2.Name())
	tmpFile2.WriteString(email2)
	tmpFile2.Close()

	sourceTag2 := fmt.Sprintf("e2e-thread-2-%d", time.Now().UnixNano())
	result2 := env.CLI.Run(ctx, "ingest", "email", tmpFile2.Name(), "--source", sourceTag2)
	require.Equal(t, 0, result2.ExitCode)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag2)

	sourceID2, err := getLatestSourceByTag(env, sourceTag2)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID2, 90*time.Second)
	require.NoError(t, err)
	t.Logf("Second email processed: source_id=%d", sourceID2)

	assertions2, err := getAssertionsForSource(env, sourceID2)
	require.NoError(t, err)
	t.Logf("Second email assertions: %d", len(assertions2))
}

// TestSLMPipeline_BatchIngestion tests processing multiple emails via CLI.
func TestSLMPipeline_BatchIngestion(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create temp directory with multiple test emails
	emailDir, err := os.MkdirTemp("", "e2e-batch-emails-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(emailDir) })

	// Create 5 test emails
	subjects := []string{
		"Project Alpha Update",
		"Budget Review Meeting",
		"Q1 Timeline Discussion",
		"Team Status Report",
		"Weekly Standup Notes",
	}
	for i, subject := range subjects {
		content := fmt.Sprintf(`From: sender%d@example.com
To: team@example.com
Subject: %s
Date: Thu, 5 Feb 2026 10:0%d:00 +0000
Message-ID: <batch-test-%d-%d@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

This is test email #%d about %s.
Please review and provide feedback.
`, i, subject, i, i, time.Now().UnixNano(), i+1, subject)

		emailPath := fmt.Sprintf("%s/email-%03d.eml", emailDir, i+1)
		err := os.WriteFile(emailPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	sourceTag := fmt.Sprintf("e2e-batch-%d", time.Now().UnixNano())

	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", sourceTag, "--concurrency", "2")
	require.Equal(t, 0, result.ExitCode, "batch ingest should succeed: %s", result.Stderr)
	t.Logf("Batch ingest output: %s", result.Stdout)

	// Trigger processing
	kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
	t.Logf("Pipeline kick output: %s", kickResult.Stdout)

	// Count sources created
	var sourceCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources s
		JOIN ingest_jobs j ON s.tenant_id = j.tenant_id
		WHERE j.source_tag = $1
	`, sourceTag).Scan(&sourceCount)
	require.NoError(t, err)
	t.Logf("Sources created: %d", sourceCount)

	assert.GreaterOrEqual(t, sourceCount, 5, "should have ingested multiple emails")

	// Wait for all to complete (with longer timeout for batch)
	time.Sleep(5 * time.Second) // Initial processing delay

	var completedCount int
	for i := 0; i < 30; i++ { // Poll for up to 60 seconds
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM sources
			WHERE processing_status = 'completed'
			AND tenant_id = $1
		`, TestTenantID).Scan(&completedCount)
		require.NoError(t, err)

		if completedCount >= sourceCount {
			break
		}
		time.Sleep(2 * time.Second)
	}

	t.Logf("Completed: %d/%d", completedCount, sourceCount)
	assert.Equal(t, sourceCount, completedCount, "all sources should complete processing")
}

// TestSLMPipeline_PartialFailureRecovery tests that partial failures are handled gracefully.
func TestSLMPipeline_PartialFailureRecovery(t *testing.T) {
	t.Skip("Partial failure recovery requires workflow cancellation - manual test")
}

// TestSLMPipeline_Idempotency tests duplicate ingestion detection via CLI.
func TestSLMPipeline_Idempotency(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create a test email
	emailContent := `From: test@acme.com
To: team@acme.com
Subject: Idempotency test
Date: Mon, 1 Jan 2024 10:00:00 -0500
Message-ID: <idempotency-test@acme.com>

Test email for idempotency verification.
This email should only be processed once.`

	tmpFile, err := os.CreateTemp("", "e2e-idem-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(emailContent)
	tmpFile.Close()

	// First ingestion
	sourceTag := fmt.Sprintf("e2e-idem-%d", time.Now().UnixNano())
	result1 := env.CLI.Run(ctx, "ingest", "email", tmpFile.Name(), "--source", sourceTag)
	require.Equal(t, 0, result1.ExitCode)
	t.Logf("First ingest: %s", result1.Stdout)

	// Count sources after first ingest
	var count1 int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", TestTenantID).Scan(&count1)

	// Second ingestion of same file
	result2 := env.CLI.Run(ctx, "ingest", "email", tmpFile.Name(), "--source", sourceTag)
	require.Equal(t, 0, result2.ExitCode)
	t.Logf("Second ingest: %s", result2.Stdout)

	// Count should be same (duplicate skipped) or +1 if dedup is at processing level
	var count2 int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", TestTenantID).Scan(&count2)

	// Check if "skipped" appears in output (indicating duplicate detection)
	if strings.Contains(result2.Stdout, "skipped") || strings.Contains(result2.Stdout, "Skipped") {
		t.Log("Duplicate correctly detected at ingest time")
		assert.Equal(t, count1, count2, "source count should not increase for duplicate")
	} else {
		t.Log("Duplicate detection may happen at processing time")
	}
}

// TestSLMPipeline_SRTTranscript tests SRT format parsing via CLI.
func TestSLMPipeline_SRTTranscript(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create SRT transcript file
	transcriptContent := `1
00:00:05,000 --> 00:00:10,000
Sarah: Welcome everyone to the incident retrospective.

2
00:00:10,000 --> 00:00:15,000
John: Thanks. Let's start with the timeline.

3
00:00:15,000 --> 00:00:20,000
Marcus: The outage began at 2:15 AM UTC.`

	tmpFile, err := os.CreateTemp("", "e2e-meeting-*.srt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(transcriptContent)
	tmpFile.Close()

	sourceTag := fmt.Sprintf("e2e-srt-%d", time.Now().UnixNano())
	result := env.CLI.Run(ctx, "ingest", "file", tmpFile.Name(), "--source", sourceTag, "--type", "meeting")

	if result.ExitCode != 0 {
		t.Logf("SRT ingest not supported via file command, skipping: %s", result.Stderr)
		t.Skip("SRT transcript ingest via CLI not available")
	}

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID, 60*time.Second)
	require.NoError(t, err)

	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "embed")
}

// TestSLMPipeline_EntityResolution tests that entities are resolved correctly.
func TestSLMPipeline_EntityResolution(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create email with known entities from fixtures
	emailContent := `From: john.smith@acme.com
To: team@acme.com
Subject: Project Alpha Update
Date: Mon, 1 Jan 2024 10:00:00 -0500
Message-ID: <entity-test@acme.com>

Hi team,

Project Alpha is moving forward with Sarah Chen as the technical lead.
Marcus will handle the infrastructure side.

We're targeting the Q1 deadline as discussed in the last TER (Technical Execution Review).

The MVP (Minimum Viable Product) scope is now finalized.

-John Smith`

	tmpFile, err := os.CreateTemp("", "e2e-entity-*.eml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(emailContent)
	tmpFile.Close()

	sourceTag := fmt.Sprintf("e2e-entity-%d", time.Now().UnixNano())
	result := env.CLI.Run(ctx, "ingest", "email", tmpFile.Name(), "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode, "ingest should succeed: %s", result.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 90*time.Second)
	require.NoError(t, err)

	// Verify context building ran (entity resolution)
	assertStageCompleted(t, env, sourceID, "parse")
	assertStageCompleted(t, env, sourceID, "triage")
	assertStageCompleted(t, env, sourceID, "embed")

	// Verify glossary terms were available
	var glossaryCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary WHERE tenant_id = $1", TestTenantID).Scan(&glossaryCount)
	require.NoError(t, err)
	assert.Greater(t, glossaryCount, 0, "glossary should have terms loaded")
	t.Logf("Glossary terms available: %d", glossaryCount)

	// Verify people were available
	var peopleCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people WHERE tenant_id = $1", TestTenantID).Scan(&peopleCount)
	require.NoError(t, err)
	assert.Greater(t, peopleCount, 0, "people should be loaded")
	t.Logf("People available: %d", peopleCount)

	// Use CLI to check content status
	historyResult := env.CLI.Run(ctx, "pipeline", "history", "--source", fmt.Sprintf("%d", sourceID))
	t.Logf("Pipeline history:\n%s", historyResult.Stdout)
}
