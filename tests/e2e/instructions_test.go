//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstructions_Phase1_CRUDAndEvaluation is the acceptance test for
// Epic 5 Phase 1 — Watch Instructions.
//
// It verifies:
//   - Schema: watch_instructions and instruction_matches tables exist with expected columns
//   - Schema: instruction_evaluate pipeline stage registered, operational config present
//   - CRUD: Full lifecycle via CLI (add, list, show, edit, enable/disable, filter)
//   - Pipeline: instruction evaluation matches content and records results
//   - Batching: multiple instructions evaluated together, only matching ones recorded
//
// Expected: FAIL until Phase 1 is implemented. The tables, pipeline stage,
// and CLI commands do not exist yet. This test defines the acceptance gate.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestInstructions_Phase1_CRUDAndEvaluation -v
func TestInstructions_Phase1_CRUDAndEvaluation(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Cleanup watch_instructions and instruction_matches for this tenant
	// after the test completes, regardless of pass/fail.
	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM instruction_matches WHERE tenant_id = $1`, env.TenantID)
		env.DB.Exec(ctx, `DELETE FROM watch_instructions WHERE tenant_id = $1`, env.TenantID)
	})

	// Create a project for project-scoped instructions
	projectResult := env.CLI.Run(ctx, "project", "add", "InstructionsTestProject",
		"--description", "Project for watch instructions e2e test",
		"--keywords", "instrtest9x7",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	var projectName string
	err = env.DB.QueryRow(ctx, `
		SELECT id, name FROM projects
		WHERE tenant_id = $1 AND name = 'InstructionsTestProject'
	`, env.TenantID).Scan(&projectID, &projectName)
	require.NoError(t, err, "test project should exist")
	t.Logf("Test project: id=%d name=%s", projectID, projectName)

	// ================================================================
	// Part 1: Schema verification
	// ================================================================
	t.Run("schema_verification", func(t *testing.T) {
		// ----- watch_instructions table -----
		t.Run("watch_instructions_table", func(t *testing.T) {
			expectedColumns := []string{
				"id", "tenant_id", "project_id", "name", "instruction",
				"priority", "enabled", "model_hint", "created_by",
				"version", "last_matched_at", "match_count",
				"created_at", "updated_at",
			}

			for _, col := range expectedColumns {
				var exists bool
				err := env.DB.QueryRow(ctx, `
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_schema = 'public'
						  AND table_name = 'watch_instructions'
						  AND column_name = $1
					)
				`, col).Scan(&exists)
				require.NoError(t, err, "querying column %s", col)
				assert.True(t, exists,
					"watch_instructions table should have column %q", col)
			}
		})

		// ----- instruction_matches table -----
		t.Run("instruction_matches_table", func(t *testing.T) {
			expectedColumns := []string{
				"id", "tenant_id", "instruction_id", "content_id",
				"source_id", "confidence", "explanation", "matched_at",
			}

			for _, col := range expectedColumns {
				var exists bool
				err := env.DB.QueryRow(ctx, `
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_schema = 'public'
						  AND table_name = 'instruction_matches'
						  AND column_name = $1
					)
				`, col).Scan(&exists)
				require.NoError(t, err, "querying column %s", col)
				assert.True(t, exists,
					"instruction_matches table should have column %q", col)
			}
		})

		// ----- instruction_evaluate pipeline stage -----
		t.Run("pipeline_stage_registered", func(t *testing.T) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM pipeline_stages
				WHERE stage = 'instruction_evaluate'
			`).Scan(&count)
			require.NoError(t, err, "querying pipeline_stages")
			assert.Greater(t, count, 0,
				"instruction_evaluate stage must be registered in pipeline_stages")
		})

		// ----- operational config -----
		t.Run("operational_config_max_instructions", func(t *testing.T) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM pipeline_operational_config
				WHERE key = 'max_instructions_per_evaluation'
			`).Scan(&count)
			require.NoError(t, err, "querying pipeline_operational_config")
			assert.Greater(t, count, 0,
				"pipeline_operational_config should have key 'max_instructions_per_evaluation'")
		})
	})

	// ================================================================
	// Part 2: CRUD via CLI
	// ================================================================
	t.Run("crud_via_cli", func(t *testing.T) {
		// Track the instruction ID across subtests
		var instructionID int64

		// ----- Add instruction -----
		t.Run("add_instruction", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "add",
				"--name", "Coordination Issues",
				"--instruction", "Alert me when emails discuss poor coordination between teams",
				"--project", projectName,
				"--priority", "high",
			)
			require.Equal(t, 0, result.ExitCode,
				"instruction add should succeed: %s", result.Stderr)
			t.Logf("instruction add output: %s", result.Stdout)

			// Verify in database
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM watch_instructions
				WHERE tenant_id = $1 AND name = 'Coordination Issues'
			`, env.TenantID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count,
				"watch_instructions should have one row for 'Coordination Issues'")

			// Capture the instruction ID for subsequent tests
			err = env.DB.QueryRow(ctx, `
				SELECT id FROM watch_instructions
				WHERE tenant_id = $1 AND name = 'Coordination Issues'
			`, env.TenantID).Scan(&instructionID)
			require.NoError(t, err)
			require.NotZero(t, instructionID, "instruction ID should not be zero")
			t.Logf("Created instruction ID: %d", instructionID)
		})

		if instructionID == 0 {
			t.Fatal("add_instruction must pass before running remaining CRUD subtests")
		}
		idStr := fmt.Sprintf("%d", instructionID)

		// ----- List instructions -----
		t.Run("list_instructions", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "list", "-o", "json")
			require.Equal(t, 0, result.ExitCode,
				"instruction list should succeed: %s", result.Stderr)

			var instructions []map[string]interface{}
			err := json.Unmarshal([]byte(result.Stdout), &instructions)
			require.NoError(t, err, "instruction list output should be valid JSON: %s", result.Stdout)

			// Find our instruction
			var found bool
			for _, inst := range instructions {
				if name, ok := inst["name"].(string); ok && name == "Coordination Issues" {
					found = true
					break
				}
			}
			assert.True(t, found,
				"instruction list should include 'Coordination Issues'")
		})

		// ----- Show instruction -----
		t.Run("show_instruction", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "show", idStr, "-o", "json")
			require.Equal(t, 0, result.ExitCode,
				"instruction show should succeed: %s", result.Stderr)

			var detail map[string]interface{}
			err := json.Unmarshal([]byte(result.Stdout), &detail)
			require.NoError(t, err, "instruction show output should be valid JSON: %s", result.Stdout)

			// Verify all expected fields are present
			assert.Equal(t, "Coordination Issues", detail["name"],
				"instruction name should match")
			assert.Equal(t, "Alert me when emails discuss poor coordination between teams",
				detail["instruction"],
				"instruction text should match")
			assert.Equal(t, "high", detail["priority"],
				"priority should be 'high'")
			assert.Equal(t, true, detail["enabled"],
				"instruction should be enabled by default")

			// Verify project association
			if projName, ok := detail["project_name"].(string); ok {
				assert.Equal(t, projectName, projName,
					"instruction should be associated with test project")
			} else if projID, ok := detail["project_id"]; ok {
				assert.NotNil(t, projID,
					"instruction should have a project_id")
			}

			t.Logf("instruction show detail: %v", detail)
		})

		// ----- Edit instruction -----
		t.Run("edit_instruction_priority", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "edit", idStr,
				"--priority", "critical",
			)
			require.Equal(t, 0, result.ExitCode,
				"instruction edit should succeed: %s", result.Stderr)

			// Verify priority was updated in DB
			var priority string
			err := env.DB.QueryRow(ctx, `
				SELECT priority FROM watch_instructions
				WHERE tenant_id = $1 AND id = $2
			`, env.TenantID, instructionID).Scan(&priority)
			require.NoError(t, err)
			assert.Equal(t, "critical", priority,
				"priority should be updated to 'critical'")
		})

		// ----- Disable instruction -----
		t.Run("disable_instruction", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "disable", idStr)
			require.Equal(t, 0, result.ExitCode,
				"instruction disable should succeed: %s", result.Stderr)

			var enabled bool
			err := env.DB.QueryRow(ctx, `
				SELECT enabled FROM watch_instructions
				WHERE tenant_id = $1 AND id = $2
			`, env.TenantID, instructionID).Scan(&enabled)
			require.NoError(t, err)
			assert.False(t, enabled,
				"instruction should be disabled after 'instruction disable'")
		})

		// ----- Enable instruction -----
		t.Run("enable_instruction", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "enable", idStr)
			require.Equal(t, 0, result.ExitCode,
				"instruction enable should succeed: %s", result.Stderr)

			var enabled bool
			err := env.DB.QueryRow(ctx, `
				SELECT enabled FROM watch_instructions
				WHERE tenant_id = $1 AND id = $2
			`, env.TenantID, instructionID).Scan(&enabled)
			require.NoError(t, err)
			assert.True(t, enabled,
				"instruction should be enabled after 'instruction enable'")
		})

		// ----- List with --enabled filter -----
		t.Run("list_enabled_filter", func(t *testing.T) {
			// First, add a second instruction and disable it
			addResult := env.CLI.Run(ctx, "instruction", "add",
				"--name", "Disabled Instruction",
				"--instruction", "This instruction is disabled for testing",
				"--priority", "low",
			)
			require.Equal(t, 0, addResult.ExitCode,
				"instruction add (disabled): %s", addResult.Stderr)

			var disabledID int64
			err := env.DB.QueryRow(ctx, `
				SELECT id FROM watch_instructions
				WHERE tenant_id = $1 AND name = 'Disabled Instruction'
			`, env.TenantID).Scan(&disabledID)
			require.NoError(t, err)

			disableResult := env.CLI.Run(ctx, "instruction", "disable", fmt.Sprintf("%d", disabledID))
			require.Equal(t, 0, disableResult.ExitCode,
				"instruction disable: %s", disableResult.Stderr)

			// List with --enabled filter
			result := env.CLI.Run(ctx, "instruction", "list", "--enabled", "-o", "json")
			require.Equal(t, 0, result.ExitCode,
				"instruction list --enabled should succeed: %s", result.Stderr)

			var instructions []map[string]interface{}
			err = json.Unmarshal([]byte(result.Stdout), &instructions)
			require.NoError(t, err, "output should be valid JSON: %s", result.Stdout)

			// All returned instructions should be enabled
			for _, inst := range instructions {
				if enabled, ok := inst["enabled"].(bool); ok {
					assert.True(t, enabled,
						"list --enabled should only return enabled instructions, got: %v", inst["name"])
				}
			}

			// "Coordination Issues" should be present (it was re-enabled)
			var foundCoordination bool
			for _, inst := range instructions {
				if name, ok := inst["name"].(string); ok && name == "Coordination Issues" {
					foundCoordination = true
				}
			}
			assert.True(t, foundCoordination,
				"enabled list should include 'Coordination Issues'")

			// "Disabled Instruction" should NOT be present
			var foundDisabled bool
			for _, inst := range instructions {
				if name, ok := inst["name"].(string); ok && name == "Disabled Instruction" {
					foundDisabled = true
				}
			}
			assert.False(t, foundDisabled,
				"enabled list should NOT include 'Disabled Instruction'")
		})

		// ----- List with --project filter -----
		t.Run("list_project_filter", func(t *testing.T) {
			result := env.CLI.Run(ctx, "instruction", "list",
				"--project", projectName, "-o", "json")
			require.Equal(t, 0, result.ExitCode,
				"instruction list --project should succeed: %s", result.Stderr)

			var instructions []map[string]interface{}
			err := json.Unmarshal([]byte(result.Stdout), &instructions)
			require.NoError(t, err, "output should be valid JSON: %s", result.Stdout)

			// "Coordination Issues" (scoped to project) should be present
			var foundCoordination bool
			for _, inst := range instructions {
				if name, ok := inst["name"].(string); ok && name == "Coordination Issues" {
					foundCoordination = true
				}
			}
			assert.True(t, foundCoordination,
				"project-filtered list should include 'Coordination Issues' (scoped to %s)", projectName)

			// "Disabled Instruction" (no project) should NOT be present
			var foundDisabled bool
			for _, inst := range instructions {
				if name, ok := inst["name"].(string); ok && name == "Disabled Instruction" {
					foundDisabled = true
				}
			}
			assert.False(t, foundDisabled,
				"project-filtered list should NOT include 'Disabled Instruction' (no project scope)")
		})
	})

	// ================================================================
	// Part 3: Pipeline evaluation
	// ================================================================
	t.Run("pipeline_evaluation", func(t *testing.T) {
		// Create a fresh instruction for pipeline evaluation
		addResult := env.CLI.Run(ctx, "instruction", "add",
			"--name", "Deployment Failures",
			"--instruction", "Flag emails that discuss deployment failures or outages",
			"--project", projectName,
			"--priority", "high",
		)
		require.Equal(t, 0, addResult.ExitCode,
			"instruction add (Deployment Failures): %s", addResult.Stderr)

		var evalInstructionID int64
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM watch_instructions
			WHERE tenant_id = $1 AND name = 'Deployment Failures'
		`, env.TenantID).Scan(&evalInstructionID)
		require.NoError(t, err, "Deployment Failures instruction should exist")
		t.Logf("Evaluation instruction ID: %d", evalInstructionID)

		// Capture initial state
		var initialMatchCount int
		err = env.DB.QueryRow(ctx, `
			SELECT match_count FROM watch_instructions
			WHERE tenant_id = $1 AND id = $2
		`, env.TenantID, evalInstructionID).Scan(&initialMatchCount)
		require.NoError(t, err)
		t.Logf("Initial match_count: %d", initialMatchCount)

		// Ingest an email that should match the instruction
		sourceTag := fmt.Sprintf("e2e-instr-eval-%d", time.Now().UnixNano())
		emailPath := createTempEmail(t,
			"Production Outage - API Gateway Down",
			`Team,

URGENT: We experienced a critical production outage this morning.

The API gateway deployment at 03:00 UTC failed and caused a cascading failure
across all downstream services. The deployment failure was caused by a
misconfigured environment variable in the new release.

Impact:
- API gateway was unresponsive for 45 minutes
- 12,000 requests failed during the outage window
- Customer-facing dashboard showed errors

Root cause: Deployment pipeline did not validate environment config before rollout.
The deployment failure could have been prevented by the pre-flight checks we discussed
last week.

ACTION: Add pre-deployment validation to the CI/CD pipeline by Friday.
DECISION: Roll back to previous stable release until fix is verified.

-- SRE Team`)

		ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
		require.Equal(t, 0, ingestResult.ExitCode,
			"ingest: %s", ingestResult.Stderr)

		// Trigger pipeline and wait for completion
		env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

		sourceID, err := getLatestSourceByTag(env, sourceTag)
		require.NoError(t, err, "should find source with tag %s", sourceTag)
		t.Logf("Source ID: %d", sourceID)

		err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
		require.NoError(t, err, "pipeline should complete within timeout")

		// Verify instruction match_count increased
		var updatedMatchCount int
		err = env.DB.QueryRow(ctx, `
			SELECT match_count FROM watch_instructions
			WHERE tenant_id = $1 AND id = $2
		`, env.TenantID, evalInstructionID).Scan(&updatedMatchCount)
		require.NoError(t, err)
		t.Logf("Updated match_count: %d (was %d)", updatedMatchCount, initialMatchCount)

		assert.Greater(t, updatedMatchCount, initialMatchCount,
			"instruction match_count should increase after matching content. "+
				"The instruction_evaluate pipeline stage should increment match_count "+
				"when content matches the instruction.")

		// Verify last_matched_at is set
		var lastMatchedAt *time.Time
		err = env.DB.QueryRow(ctx, `
			SELECT last_matched_at FROM watch_instructions
			WHERE tenant_id = $1 AND id = $2
		`, env.TenantID, evalInstructionID).Scan(&lastMatchedAt)
		require.NoError(t, err)
		assert.NotNil(t, lastMatchedAt,
			"last_matched_at should be set after a match")
		if lastMatchedAt != nil {
			t.Logf("last_matched_at: %s", lastMatchedAt.Format(time.RFC3339))
		}

		// Verify instruction_matches has a row linking instruction to content
		var matchCount int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM instruction_matches
			WHERE tenant_id = $1
			  AND instruction_id = $2
			  AND source_id = $3
		`, env.TenantID, evalInstructionID, sourceID).Scan(&matchCount)
		require.NoError(t, err)
		assert.Greater(t, matchCount, 0,
			"instruction_matches should have at least one row linking "+
				"instruction %d to source %d", evalInstructionID, sourceID)

		// Verify match has confidence and explanation
		if matchCount > 0 {
			var confidence float64
			var explanation string
			err = env.DB.QueryRow(ctx, `
				SELECT confidence, explanation FROM instruction_matches
				WHERE tenant_id = $1
				  AND instruction_id = $2
				  AND source_id = $3
				ORDER BY matched_at DESC
				LIMIT 1
			`, env.TenantID, evalInstructionID, sourceID).Scan(&confidence, &explanation)
			require.NoError(t, err)

			assert.Greater(t, confidence, 0.0,
				"match confidence should be > 0")
			assert.NotEmpty(t, explanation,
				"match explanation should not be empty")
			t.Logf("Match confidence=%.2f explanation=%q", confidence, explanation)
		}

		// Verify via CLI: penf instruction history <id>
		historyResult := env.CLI.Run(ctx, "instruction", "history", fmt.Sprintf("%d", evalInstructionID), "-o", "json")
		require.Equal(t, 0, historyResult.ExitCode,
			"instruction history should succeed: %s", historyResult.Stderr)

		var history []map[string]interface{}
		err = json.Unmarshal([]byte(historyResult.Stdout), &history)
		require.NoError(t, err, "instruction history output should be valid JSON: %s", historyResult.Stdout)

		assert.Greater(t, len(history), 0,
			"instruction history should show at least one match")
		if len(history) > 0 {
			t.Logf("instruction history entry: %v", history[0])
		}
	})

	// ================================================================
	// Part 4: Batching verification
	// ================================================================
	t.Run("batching_verification", func(t *testing.T) {
		// Create 3 instructions — only one should match the content we ingest
		matchingResult := env.CLI.Run(ctx, "instruction", "add",
			"--name", "Database Migration Issues",
			"--instruction", "Flag emails discussing database migration failures or schema rollback issues",
			"--priority", "high",
		)
		require.Equal(t, 0, matchingResult.ExitCode,
			"instruction add (Database Migration): %s", matchingResult.Stderr)

		nonMatch1Result := env.CLI.Run(ctx, "instruction", "add",
			"--name", "Marketing Campaign Updates",
			"--instruction", "Alert me when emails discuss marketing campaign performance metrics or ROI analysis",
			"--priority", "normal",
		)
		require.Equal(t, 0, nonMatch1Result.ExitCode,
			"instruction add (Marketing Campaign): %s", nonMatch1Result.Stderr)

		nonMatch2Result := env.CLI.Run(ctx, "instruction", "add",
			"--name", "Office Relocation",
			"--instruction", "Flag emails about office space relocation plans or new building lease negotiations",
			"--priority", "low",
		)
		require.Equal(t, 0, nonMatch2Result.ExitCode,
			"instruction add (Office Relocation): %s", nonMatch2Result.Stderr)

		// Get instruction IDs
		var matchingID, nonMatch1ID, nonMatch2ID int64
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM watch_instructions
			WHERE tenant_id = $1 AND name = 'Database Migration Issues'
		`, env.TenantID).Scan(&matchingID)
		require.NoError(t, err)

		err = env.DB.QueryRow(ctx, `
			SELECT id FROM watch_instructions
			WHERE tenant_id = $1 AND name = 'Marketing Campaign Updates'
		`, env.TenantID).Scan(&nonMatch1ID)
		require.NoError(t, err)

		err = env.DB.QueryRow(ctx, `
			SELECT id FROM watch_instructions
			WHERE tenant_id = $1 AND name = 'Office Relocation'
		`, env.TenantID).Scan(&nonMatch2ID)
		require.NoError(t, err)

		t.Logf("Batching test instructions: matching=%d nonMatch1=%d nonMatch2=%d",
			matchingID, nonMatch1ID, nonMatch2ID)

		// Ingest an email that should match ONLY the database migration instruction
		sourceTag := fmt.Sprintf("e2e-instr-batch-%d", time.Now().UnixNano())
		emailPath := createTempEmail(t,
			"Database Migration Failure - Urgent Rollback Required",
			`Team,

The database migration for the users table failed during tonight's release window.
The schema migration script timed out after 30 minutes, leaving the database in an
inconsistent state.

We had to perform an emergency schema rollback to restore service. The rollback
completed successfully but we lost the new indexes that were part of this migration.

Root cause: The migration attempted to add a NOT NULL column to a table with 50M rows
without a default value, causing a full table rewrite that exceeded our maintenance window.

ACTION: Rewrite the migration to use a phased approach (add nullable, backfill, set NOT NULL).
DECISION: All future schema migrations must include rollback scripts and time estimates.
RISK: The delayed migration blocks the v2 API launch scheduled for next week.

-- Database Team`)

		ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
		require.Equal(t, 0, ingestResult.ExitCode,
			"ingest: %s", ingestResult.Stderr)

		env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

		sourceID, err := getLatestSourceByTag(env, sourceTag)
		require.NoError(t, err, "should find source with tag %s", sourceTag)
		t.Logf("Batch test source ID: %d", sourceID)

		err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
		require.NoError(t, err, "pipeline should complete within timeout")

		// Verify: matching instruction has a match
		var matchingHits int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM instruction_matches
			WHERE tenant_id = $1
			  AND instruction_id = $2
			  AND source_id = $3
		`, env.TenantID, matchingID, sourceID).Scan(&matchingHits)
		require.NoError(t, err)
		assert.Greater(t, matchingHits, 0,
			"'Database Migration Issues' instruction should match the database migration email. "+
				"The instruction_evaluate stage should detect the match and record it in instruction_matches.")

		// Verify: non-matching instructions should NOT have matches for this content
		var nonMatch1Hits int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM instruction_matches
			WHERE tenant_id = $1
			  AND instruction_id = $2
			  AND source_id = $3
		`, env.TenantID, nonMatch1ID, sourceID).Scan(&nonMatch1Hits)
		require.NoError(t, err)
		assert.Equal(t, 0, nonMatch1Hits,
			"'Marketing Campaign Updates' instruction should NOT match a database migration email. "+
				"The pipeline should evaluate all instructions but only record actual matches.")

		var nonMatch2Hits int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM instruction_matches
			WHERE tenant_id = $1
			  AND instruction_id = $2
			  AND source_id = $3
		`, env.TenantID, nonMatch2ID, sourceID).Scan(&nonMatch2Hits)
		require.NoError(t, err)
		assert.Equal(t, 0, nonMatch2Hits,
			"'Office Relocation' instruction should NOT match a database migration email. "+
				"The pipeline should evaluate all instructions but only record actual matches.")

		t.Logf("Batching results: matching=%d nonMatch1=%d nonMatch2=%d",
			matchingHits, nonMatch1Hits, nonMatch2Hits)
	})
}
