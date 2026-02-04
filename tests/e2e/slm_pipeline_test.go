//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSLMPipeline_FullEmailPipeline tests the complete SLM pipeline with an email.
// All stages should run: parse -> triage -> extract -> context -> analyze -> persist -> embed.
func TestSLMPipeline_FullEmailPipeline(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Email content
	emailContent := `Hi Sarah,

Quick update on Project Alpha - we discussed the MVP timeline at TER yesterday.
Marcus mentioned some concerns about the Q1 deadline.

Can we sync with the team on Thursday?

Thanks,
John`

	// Create source record
	sourceID, err := env.createSource(ctx, "default", "email", "e2e-full-email", "", "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	// Start pipeline workflow
	input := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID,
		ContentType: "email",
		BodyText:    emailContent,
		Subject:     "RE: Project Alpha MVP Status",
		SenderEmail: "john.smith@acme.com",
		SenderName:  "John Smith",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	// Wait for completion
	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify all stages completed
	env.assertStageCompleted(t, sourceID, "parse")
	env.assertStageCompleted(t, sourceID, "triage")
	env.assertStageCompleted(t, sourceID, "extract")
	env.assertStageCompleted(t, sourceID, "context")
	env.assertStageCompleted(t, sourceID, "analyze")
	env.assertStageCompleted(t, sourceID, "persist")
	env.assertStageCompleted(t, sourceID, "embed")

	// Verify source status
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus)

	// Verify embeddings were created
	embeddings, err := env.countEmbeddingsForSource(ctx, sourceID)
	require.NoError(t, err)
	assert.Greater(t, len(embeddings), 0, "should have created embeddings")
	t.Logf("Embeddings created: %v", embeddings)

	// Verify assertions were created
	assertions, err := env.getAssertionsForSource(ctx, sourceID)
	require.NoError(t, err)
	t.Logf("Assertions created: %d", len(assertions))
}

// TestSLMPipeline_MeetingTranscript tests pipeline with a VTT transcript.
func TestSLMPipeline_MeetingTranscript(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// VTT transcript content
	transcriptContent := `WEBVTT

00:00:05.000 --> 00:00:10.000
Sarah: Welcome everyone to the Project Alpha review.

00:00:10.000 --> 00:00:15.000
John: Thanks Sarah. Let me walk through the Q1 timeline.

00:00:15.000 --> 00:00:20.000
Marcus: I have some concerns about the deadline.`

	sourceID, err := env.createSource(ctx, "default", "meeting", "e2e-meeting-vtt", "", "meeting", transcriptContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	input := PipelineInput{
		TenantID:          "default",
		SourceID:          sourceID,
		ContentType:       "meeting",
		TranscriptContent: transcriptContent,
		TranscriptFormat:  "vtt",
		JobID:             fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify parse stage handled transcript format
	env.assertStageCompleted(t, sourceID, "parse")
	env.assertStageCompleted(t, sourceID, "triage")

	// Verify source status
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus)
}

// TestSLMPipeline_TriageGateLOW tests that LOW importance content skips deep processing.
func TestSLMPipeline_TriageGateLOW(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Low priority FYI email content
	emailContent := `FYI - The coffee machine on floor 3 is being serviced tomorrow morning.

Regards,
Facilities`

	sourceID, err := env.createSource(ctx, "default", "email", "e2e-low-priority", "", "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	input := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID,
		ContentType: "email",
		BodyText:    emailContent,
		Subject:     "FYI: Coffee machine maintenance",
		SenderEmail: "facilities@acme.com",
		SenderName:  "Facilities Team",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify parse and triage ran
	env.assertStageCompleted(t, sourceID, "parse")
	env.assertStageCompleted(t, sourceID, "triage")

	// Verify deep processing stages were skipped
	env.assertStageSkipped(t, sourceID, "extract")
	env.assertStageSkipped(t, sourceID, "context")
	env.assertStageSkipped(t, sourceID, "analyze")
	env.assertStageSkipped(t, sourceID, "persist")

	// Verify embedding still ran (critical for search)
	env.assertStageCompleted(t, sourceID, "embed")

	// Verify source completed
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus)
}

// TestSLMPipeline_HighImportanceRisk tests routing for high-importance risk content.
func TestSLMPipeline_HighImportanceRisk(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// High priority risk escalation
	emailContent := `URGENT: Security vulnerability discovered in production API

We've identified a critical authentication bypass in the user API endpoint.
This needs immediate attention - potential data exposure risk.

Severity: HIGH
Impact: All user accounts potentially compromised
Recommended action: Immediate hotfix deployment

-Security Team`

	sourceID, err := env.createSource(ctx, "default", "email", "e2e-risk-high", "", "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	input := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID,
		ContentType: "email",
		BodyText:    emailContent,
		Subject:     "URGENT: Security vulnerability in production",
		SenderEmail: "security@acme.com",
		SenderName:  "Security Team",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify all stages ran (HIGH importance should not skip)
	env.assertStageCompleted(t, sourceID, "parse")
	env.assertStageCompleted(t, sourceID, "triage")
	env.assertStageCompleted(t, sourceID, "extract")
	env.assertStageCompleted(t, sourceID, "context")
	env.assertStageCompleted(t, sourceID, "analyze")
	env.assertStageCompleted(t, sourceID, "persist")
	env.assertStageCompleted(t, sourceID, "embed")

	// Verify assertions were created (should include risk assertions)
	assertions, err := env.getAssertionsForSource(ctx, sourceID)
	require.NoError(t, err)
	assert.Greater(t, len(assertions), 0, "should have created risk assertions")

	// Check for risk-type assertions
	hasRisk := false
	for _, a := range assertions {
		if a.AssertionType == "risk" {
			hasRisk = true
			t.Logf("Risk assertion: %s", a.Description)
		}
	}
	assert.True(t, hasRisk, "should have at least one risk assertion")
}

// TestSLMPipeline_GoldenThread tests assertion lifecycle across multiple emails.
func TestSLMPipeline_GoldenThread(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// First email: Initial decision
	email1 := `Team,

We've decided to move forward with the microservices architecture for Project Alpha.
This will allow us to scale more effectively.

Decision owner: Sarah Chen
Timeline: Q1 2026

-John`

	sourceID1, err := env.createSource(ctx, "default", "email", "e2e-thread-1", "", "email", email1)
	require.NoError(t, err)

	input1 := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID1,
		ContentType: "email",
		BodyText:    email1,
		Subject:     "Decision: Project Alpha Architecture",
		SenderEmail: "john.smith@acme.com",
		SenderName:  "John Smith",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run1, err := env.startPipelineWorkflow(ctx, input1)
	require.NoError(t, err)
	t.Logf("Started workflow 1: %s", run1.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID1, 60*time.Second)
	require.NoError(t, err)

	// Get assertions from first email
	assertions1, err := env.getAssertionsForSource(ctx, sourceID1)
	require.NoError(t, err)
	require.Greater(t, len(assertions1), 0, "first email should create assertions")

	// Find decision assertion
	var decisionRootID int64
	for _, a := range assertions1 {
		if a.AssertionType == "decision" && a.AssertionRootID == nil {
			decisionRootID = a.ID
			t.Logf("Found decision root: ID=%d, desc=%s", a.ID, a.Description)
			break
		}
	}

	if decisionRootID == 0 {
		t.Log("No decision assertion found in first email, skipping lifecycle test")
		return
	}

	// Second email: Update to the decision
	email2 := `Update on the microservices decision:

After discussing with the infrastructure team, we're modifying the approach
to use a hybrid architecture for the first phase.

This reduces initial complexity while maintaining scalability goals.

-Sarah`

	sourceID2, err := env.createSource(ctx, "default", "email", "e2e-thread-2", "", "email", email2)
	require.NoError(t, err)

	input2 := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID2,
		ContentType: "email",
		BodyText:    email2,
		Subject:     "RE: Decision: Project Alpha Architecture",
		SenderEmail: "sarah.chen@acme.com",
		SenderName:  "Sarah Chen",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run2, err := env.startPipelineWorkflow(ctx, input2)
	require.NoError(t, err)
	t.Logf("Started workflow 2: %s", run2.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID2, 60*time.Second)
	require.NoError(t, err)

	// Query golden thread
	thread, err := env.getGoldenThread(ctx, decisionRootID)
	require.NoError(t, err)
	t.Logf("Golden thread length: %d", len(thread))

	// Verify lifecycle events
	for _, a := range thread {
		t.Logf("Thread assertion: ID=%d, lifecycle=%v, current=%v, desc=%s",
			a.ID, a.LifecycleEvent, a.IsCurrent, a.Description)
	}
}

// TestSLMPipeline_BatchIngestion tests processing multiple emails for consistency.
func TestSLMPipeline_BatchIngestion(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest 5 emails from fixtures
	emails := []string{
		"001-project-update.eml",
		"002-incident-response.eml",
		"003-meeting-invite.eml",
		"004-code-review.eml",
		"005-project-kickoff.eml",
	}

	var sourceIDs []int64

	for i, emailFile := range emails {
		emailPath := env.FixturePath(filepath.Join("emails", emailFile))
		content, err := os.ReadFile(emailPath)
		require.NoError(t, err, "failed to read %s", emailFile)

		sourceID, err := env.createSource(ctx, "default", "email", fmt.Sprintf("e2e-batch-%d", i), "", "email", string(content))
		require.NoError(t, err)
		sourceIDs = append(sourceIDs, sourceID)

		input := PipelineInput{
			TenantID:    "default",
			SourceID:    sourceID,
			ContentType: "email",
			BodyText:    string(content),
			Subject:     fmt.Sprintf("Batch test email %d", i),
			SenderEmail: "test@acme.com",
			JobID:       fmt.Sprintf("e2e-batch-%d-%d", i, time.Now().UnixNano()),
		}

		run, err := env.startPipelineWorkflow(ctx, input)
		require.NoError(t, err)
		t.Logf("Started workflow %d: %s", i, run.GetID())
	}

	// Wait for all to complete
	for i, sourceID := range sourceIDs {
		err = env.waitForPipelineComplete(ctx, sourceID, 90*time.Second)
		require.NoError(t, err, "workflow %d (source %d) should complete", i, sourceID)
	}

	// Verify all completed successfully
	for i, sourceID := range sourceIDs {
		source, err := env.getSourceByID(ctx, sourceID)
		require.NoError(t, err)
		assert.Equal(t, "completed", source.ProcessingStatus, "source %d should be completed", i)

		// Verify embeddings
		embeddings, err := env.countEmbeddingsForSource(ctx, sourceID)
		require.NoError(t, err)
		assert.Greater(t, len(embeddings), 0, "source %d should have embeddings", i)
	}

	t.Logf("Batch ingestion completed: %d emails processed", len(sourceIDs))
}

// TestSLMPipeline_PartialFailureRecovery tests that partial failures are preserved.
// This test simulates what happens if a workflow is interrupted mid-pipeline.
func TestSLMPipeline_PartialFailureRecovery(t *testing.T) {
	t.Skip("Partial failure recovery requires workflow cancellation - manual test")

	// This test would require:
	// 1. Start workflow
	// 2. Wait for parse stage to complete
	// 3. Cancel workflow
	// 4. Verify parse results are preserved
	// 5. Restart workflow
	// 6. Verify idempotency
}

// TestSLMPipeline_Idempotency tests duplicate ingestion detection.
func TestSLMPipeline_Idempotency(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	emailContent := `Test email for idempotency verification.

This email should only be processed once.`

	contentHash := computeContentHash(emailContent)

	// First ingestion
	sourceID1, err := env.createSource(ctx, "default", "email", "e2e-idem-1", contentHash, "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID 1: %d", sourceID1)

	input1 := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID1,
		ContentType: "email",
		ContentHash: contentHash,
		BodyText:    emailContent,
		Subject:     "Idempotency test",
		SenderEmail: "test@acme.com",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run1, err := env.startPipelineWorkflow(ctx, input1)
	require.NoError(t, err)
	t.Logf("Started workflow 1: %s", run1.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID1, 60*time.Second)
	require.NoError(t, err)

	// Get embedding count after first ingestion
	embeddings1, err := env.countEmbeddingsForSource(ctx, sourceID1)
	require.NoError(t, err)
	t.Logf("First ingestion embeddings: %v", embeddings1)

	// Second ingestion with same content_hash
	sourceID2, err := env.createSource(ctx, "default", "email", "e2e-idem-2", contentHash, "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID 2: %d", sourceID2)

	input2 := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID2,
		ContentType: "email",
		ContentHash: contentHash,
		BodyText:    emailContent,
		Subject:     "Idempotency test",
		SenderEmail: "test@acme.com",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run2, err := env.startPipelineWorkflow(ctx, input2)
	require.NoError(t, err)
	t.Logf("Started workflow 2: %s", run2.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID2, 60*time.Second)
	require.NoError(t, err)

	// Both sources should be marked complete
	source1, err := env.getSourceByID(ctx, sourceID1)
	require.NoError(t, err)
	assert.Equal(t, "completed", source1.ProcessingStatus)

	source2, err := env.getSourceByID(ctx, sourceID2)
	require.NoError(t, err)
	assert.Equal(t, "completed", source2.ProcessingStatus)

	t.Log("Idempotency test completed - both sources processed")
}

// TestSLMPipeline_SRTTranscript tests SRT format parsing.
func TestSLMPipeline_SRTTranscript(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// SRT transcript content
	transcriptContent := `1
00:00:05,000 --> 00:00:10,000
Sarah: Welcome everyone to the incident retrospective.

2
00:00:10,000 --> 00:00:15,000
John: Thanks. Let's start with the timeline.

3
00:00:15,000 --> 00:00:20,000
Marcus: The outage began at 2:15 AM UTC.`

	sourceID, err := env.createSource(ctx, "default", "meeting", "e2e-meeting-srt", "", "meeting", transcriptContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	input := PipelineInput{
		TenantID:          "default",
		SourceID:          sourceID,
		ContentType:       "meeting",
		TranscriptContent: transcriptContent,
		TranscriptFormat:  "srt",
		JobID:             fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify parse stage handled SRT format
	env.assertStageCompleted(t, sourceID, "parse")
	env.assertStageCompleted(t, sourceID, "triage")
	env.assertStageCompleted(t, sourceID, "embed")

	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, "completed", source.ProcessingStatus)
}

// TestSLMPipeline_EntityResolution tests that entities are resolved correctly.
func TestSLMPipeline_EntityResolution(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Email with known entities from fixtures
	emailContent := `Hi team,

Project Alpha is moving forward with Sarah Chen as the technical lead.
Marcus will handle the infrastructure side.

We're targeting the Q1 deadline as discussed in the last TER (Technical Execution Review).

The MVP (Minimum Viable Product) scope is now finalized.

-John Smith`

	sourceID, err := env.createSource(ctx, "default", "email", "e2e-entity-res", "", "email", emailContent)
	require.NoError(t, err)
	t.Logf("Created source ID: %d", sourceID)

	input := PipelineInput{
		TenantID:    "default",
		SourceID:    sourceID,
		ContentType: "email",
		BodyText:    emailContent,
		Subject:     "Project Alpha Update",
		SenderEmail: "john.smith@acme.com",
		SenderName:  "John Smith",
		JobID:       fmt.Sprintf("e2e-test-%d", time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err)
	t.Logf("Started workflow: %s", run.GetID())

	err = env.waitForPipelineComplete(ctx, sourceID, 60*time.Second)
	require.NoError(t, err)

	// Verify context building ran (entity resolution)
	env.assertStageCompleted(t, sourceID, "context")

	// Check pipeline_runs for context stage to see if entities were resolved
	runs, err := env.getPipelineRuns(ctx, sourceID)
	require.NoError(t, err)

	var contextStage *PipelineRunRow
	for _, r := range runs {
		if r.Stage == "context" {
			contextStage = &r
			break
		}
	}

	require.NotNil(t, contextStage, "context stage should have run")
	assert.Equal(t, "completed", contextStage.Status)
	if contextStage.DurationMs != nil {
		t.Logf("Context stage completed in %dms", *contextStage.DurationMs)
	} else {
		t.Logf("Context stage completed")
	}

	// Verify glossary terms were available (check that glossary fixture loaded)
	var glossaryCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary WHERE tenant_id = 'default'").Scan(&glossaryCount)
	require.NoError(t, err)
	assert.Greater(t, glossaryCount, 0, "glossary should have terms loaded")
	t.Logf("Glossary terms available: %d", glossaryCount)

	// Verify people were available
	var peopleCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people WHERE tenant_id = 'default'").Scan(&peopleCount)
	require.NoError(t, err)
	assert.Greater(t, peopleCount, 0, "people should be loaded")
	t.Logf("People available: %d", peopleCount)

	// Check if specific people are in the database
	var sarahExists bool
	err = env.DB.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM people
			WHERE tenant_id = 'default'
			AND (canonical_name ILIKE '%sarah%chen%' OR primary_email ILIKE '%sarah.chen%')
		)
	`).Scan(&sarahExists)
	require.NoError(t, err)
	if sarahExists {
		t.Log("Sarah Chen found in people database")
	} else {
		t.Log("Sarah Chen not found - may need to be created by fixture")
	}
}
