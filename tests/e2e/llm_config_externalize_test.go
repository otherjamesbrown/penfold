//go:build e2e

// Acceptance test for SPEC items 2A (per-stage LLM parameters) + 2B (model routing rules).
//
// 2A -- Per-Stage LLM Parameters:
//   extract.go and analyze.go have hardcoded Temperature, MaxTokens, and retry counts
//   in CompletionOptions structs:
//
//   | Stage              | Temp | MaxTokens | Retries | File:Line                 |
//   |--------------------|------|-----------|---------|---------------------------|
//   | extract_ner        | 0.1  | 8192      | 2       | extract.go:293-296, 274   |
//   | extract_semantic   | 0.1  | 8192      | 2       | extract.go:369-374        |
//   | quality_gate_risk  | 0.1  | 8192      | 2       | extract.go:454-459        |
//   | analyze            | 0.2  | 8192      | 2       | analyze.go:470-475, 478   |
//
//   Fix: Add max_tokens, temperature, max_retries columns to pipeline_definitions.
//   Handlers read from DB row; fall back to hardcoded values when NULL.
//
//   Migration: 087_pipeline_definitions_llm_params.sql
//
// 2B -- Model Routing Rules:
//   selectModelForDeepAnalysis() in analyze.go:330-377 has hardcoded model selection
//   logic based on triage category + importance. The ai_routing_rules table already
//   exists (migration 022) but has no way to express conditional matching.
//
//   Fix: Add conditions JSONB column to ai_routing_rules. Expand the task_type
//   CHECK constraint to include 'deep_analysis'. Seed routing rules that encode
//   the existing selectModelForDeepAnalysis logic. Update the Go struct, Validate(),
//   and scanRoutingRules() to handle the new column.
//
//   Migration: 085_routing_rules_conditions.sql
//
// This test file defines "done" for 2A+2B. The source-grep subtests WILL FAIL until
// the refactor is complete. That is intentional.

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 2A: Per-Stage LLM Parameters
// ============================================================================

// expectedStageLLMParams maps pipeline stage names to their expected default
// LLM parameters. These are the hardcoded values in extract.go / analyze.go
// that migration 087 should seed into pipeline_definitions.
var expectedStageLLMParams = []struct {
	stage      string
	temp       float64
	maxTokens  int
	maxRetries int
}{
	{"extract_ner", 0.1, 8192, 2},
	{"extract_semantic", 0.1, 8192, 2},
	// quality_gate_risk is NOT a named pipeline stage -- handled separately
	{"analyze", 0.2, 8192, 2},
}

// TestLLMConfigExternalize_MigrationExists_087 verifies that the migration file
// for adding LLM parameter columns to pipeline_definitions exists.
func TestLLMConfigExternalize_MigrationExists_087(t *testing.T) {
	projectRoot := findProjectRoot(t)

	migrationPath := filepath.Join(projectRoot, "migrations", "087_pipeline_definitions_llm_params.sql")
	_, err := os.Stat(migrationPath)

	if os.IsNotExist(err) {
		t.Error("migration 087_pipeline_definitions_llm_params.sql does not exist; " +
			"this migration must add max_tokens, temperature, max_retries columns " +
			"to pipeline_definitions and seed default values")
	} else {
		require.NoError(t, err)
		t.Logf("Migration file exists: %s", migrationPath)
	}
}

// TestLLMConfigExternalize_PipelineDefinitionsColumns verifies that the
// pipeline_definitions table has the new LLM parameter columns after migration 087.
func TestLLMConfigExternalize_PipelineDefinitionsColumns(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expectedColumns := []string{"max_tokens", "temperature", "max_retries"}

	for _, col := range expectedColumns {
		t.Run("column_"+col, func(t *testing.T) {
			var exists bool
			err := env.DB.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_name = 'pipeline_definitions'
					  AND column_name = $1
				)
			`, col).Scan(&exists)

			require.NoError(t, err, "should query information_schema for column %s", col)
			assert.True(t, exists,
				"pipeline_definitions should have column %q after migration 087", col)

			if exists {
				t.Logf("Column %s exists on pipeline_definitions", col)
			}
		})
	}
}

// TestLLMConfigExternalize_SeedValuesMatch verifies that the migration seeds
// correct default LLM parameter values for each stage. The seed values must
// match the hardcoded values currently in extract.go and analyze.go.
func TestLLMConfigExternalize_SeedValuesMatch(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, expected := range expectedStageLLMParams {
		t.Run("seed_"+expected.stage, func(t *testing.T) {
			var temperature *float64
			var maxTokens *int
			var maxRetries *int

			err := env.DB.QueryRow(ctx, `
				SELECT temperature, max_tokens, max_retries
				FROM pipeline_definitions
				WHERE stage = $1
				  AND pipeline = 'standard'
				LIMIT 1
			`, expected.stage).Scan(&temperature, &maxTokens, &maxRetries)

			require.NoError(t, err,
				"pipeline_definitions should have a row for stage %q in standard pipeline",
				expected.stage)

			if temperature != nil {
				assert.InDelta(t, expected.temp, *temperature, 0.001,
					"temperature for %s should be %.1f; got %.4f",
					expected.stage, expected.temp, *temperature)
			} else {
				t.Errorf("temperature for %s is NULL; expected %.1f", expected.stage, expected.temp)
			}

			if maxTokens != nil {
				assert.Equal(t, expected.maxTokens, *maxTokens,
					"max_tokens for %s should be %d; got %d",
					expected.stage, expected.maxTokens, *maxTokens)
			} else {
				t.Errorf("max_tokens for %s is NULL; expected %d", expected.stage, expected.maxTokens)
			}

			if maxRetries != nil {
				assert.Equal(t, expected.maxRetries, *maxRetries,
					"max_retries for %s should be %d; got %d",
					expected.stage, expected.maxRetries, *maxRetries)
			} else {
				t.Errorf("max_retries for %s is NULL; expected %d", expected.stage, expected.maxRetries)
			}

			t.Logf("Stage %s: temperature=%.4f, max_tokens=%v, max_retries=%v",
				expected.stage,
				safeFloat(temperature),
				safeInt(maxTokens),
				safeInt(maxRetries))
		})
	}
}

// TestLLMConfigExternalize_DBOverrideWorks is the integration smoke test for 2A.
// It updates the pipeline_definitions temperature for the analyze stage to 0.3,
// then processes content through the full pipeline and verifies it completes.
// If the handler reads from DB, the modified value is used (proving DB override works).
// If the handler ignores DB, it still completes using the hardcoded value (the test
// just proves the pipeline doesn't break when DB values differ from hardcoded ones).
func TestLLMConfigExternalize_DBOverrideWorks(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean up and ensure test tenant exists
	err := env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed")

	err = env.EnsureTenantExists()
	require.NoError(t, err, "ensure tenant should succeed")

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err, "fixture loading should succeed")

	// Try to update the analyze stage temperature to 0.3.
	// This will fail if the columns don't exist yet (expected before migration 087).
	_, updateErr := env.DB.Exec(ctx, `
		UPDATE pipeline_definitions
		SET temperature = 0.3
		WHERE stage = 'analyze'
		  AND pipeline = 'standard'
		  AND tenant_id = $1
	`, env.TenantID)

	if updateErr != nil {
		t.Logf("Could not update analyze temperature (columns may not exist yet): %v", updateErr)
		t.Log("Skipping pipeline run -- migration 087 must be applied first")
		t.FailNow()
	}

	t.Log("Updated analyze stage temperature to 0.3")

	// Restore original value after test
	t.Cleanup(func() {
		_, _ = env.DB.Exec(ctx, `
			UPDATE pipeline_definitions
			SET temperature = 0.2
			WHERE stage = 'analyze'
			  AND pipeline = 'standard'
			  AND tenant_id = $1
		`, env.TenantID)
	})

	// Insert a high-importance email to exercise the analyze stage
	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<llm-config-test-%d@e2e.penfold>", time.Now().UnixNano()),
		Subject:     "URGENT: LLM Config Externalize E2E - Security Architecture Review",
		FromAddress: "john.smith@acme.com",
		FromName:    "John Smith",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `Team,

This is a high-priority risk issue requiring full deep analysis.

Key findings:
- Sarah Chen discovered a critical vulnerability in the authentication module
- Marcus Johnson identified three production servers with outdated TLS certificates
- Risk assessment: HIGH -- immediate remediation required

Decision: Emergency patching of all production services by end of week.
Owner: Sarah Chen
Impact: All customer-facing services will require rolling restarts.

Action items:
1. Patch authentication module vulnerability within 24 hours
2. Rotate all TLS certificates on affected servers
3. Enable enhanced audit logging across all production endpoints

This is the highest priority security item for the organisation.

-John Smith`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("llm-config-test-%d", time.Now().UnixNano()),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0), "source ID must be positive")
	t.Logf("Created source ID: %d", sourceID)

	// Process through the full pipeline
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify pipeline completed (proves DB-backed LLM params didn't break anything)
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err, "should fetch source")
	require.Equal(t, "completed", source.ProcessingStatus,
		"pipeline should complete with DB-overridden temperature; if failed, "+
			"the analyze handler may not be reading LLM params from pipeline_definitions")

	// Verify extraction and analysis stages completed
	env.assertStageCompleted(t, sourceID, "extract_ner")
	env.assertStageCompleted(t, sourceID, "extract_semantic")
	env.assertStageCompleted(t, sourceID, "analyze")

	t.Log("Pipeline completed successfully with DB-overridden LLM parameters")
}

// TestLLMConfigExternalize_SourceCodeUsesDBParams greps extract.go and analyze.go
// to verify they read LLM parameters from pipeline config instead of using raw
// hardcoded literals as the primary path.
//
// After the refactor:
//   - extract.go should read temperature/max_tokens/max_retries from pipeline config
//     (DBConfigResolver, pipeline_definitions, or similar) for NER, semantic, and
//     quality gate LLM calls. The 0.1/8192/2 literals should only be fallback defaults.
//   - analyze.go should read temperature/max_tokens/max_retries from pipeline config
//     for the deep analysis LLM call. The 0.2/8192/2 literals should only be fallbacks.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestLLMConfigExternalize_SourceCodeUsesDBParams(t *testing.T) {
	projectRoot := findProjectRoot(t)

	t.Run("extract_reads_params_from_config", func(t *testing.T) {
		extractFile := filepath.Join(projectRoot, "services", "ai", "server", "extract.go")
		content, err := os.ReadFile(extractFile)
		require.NoError(t, err, "should read extract.go")
		src := string(content)

		// After refactor, extract.go should reference pipeline config for LLM params.
		// Look for evidence of config-based resolution for temperature/max_tokens/retries.
		hasConfigLookup := strings.Contains(src, "pipeline_definitions") ||
			strings.Contains(src, "pipelineConfig") ||
			strings.Contains(src, "stageConfig") ||
			strings.Contains(src, "StageParams") ||
			strings.Contains(src, "LLMParams") ||
			strings.Contains(src, "configResolver") && (strings.Contains(src, "Temperature") || strings.Contains(src, "MaxTokens"))

		if !hasConfigLookup {
			// The hardcoded values are still the primary path.
			// Scan for raw 0.1 temperature in CompletionOptions (not in comments/strings).
			violations := findHardcodedLLMParamViolations(t, extractFile, "extract.go")
			if len(violations) > 0 {
				t.Errorf("extract.go still uses hardcoded LLM params as primary path:")
				for _, v := range violations {
					t.Errorf("  %s:%d: %s", v.file, v.line, strings.TrimSpace(v.text))
				}
				t.Error("After refactor, Temperature/MaxTokens/max retries must be read from pipeline config, " +
					"with hardcoded values as fallback only")
			}
		} else {
			t.Log("extract.go references pipeline config for LLM parameters (refactor complete)")
		}
	})

	t.Run("analyze_reads_params_from_config", func(t *testing.T) {
		analyzeFile := filepath.Join(projectRoot, "services", "ai", "server", "analyze.go")
		content, err := os.ReadFile(analyzeFile)
		require.NoError(t, err, "should read analyze.go")
		src := string(content)

		hasConfigLookup := strings.Contains(src, "pipeline_definitions") ||
			strings.Contains(src, "pipelineConfig") ||
			strings.Contains(src, "stageConfig") ||
			strings.Contains(src, "StageParams") ||
			strings.Contains(src, "LLMParams") ||
			strings.Contains(src, "configResolver") && (strings.Contains(src, "Temperature") || strings.Contains(src, "MaxTokens"))

		if !hasConfigLookup {
			violations := findHardcodedLLMParamViolations(t, analyzeFile, "analyze.go")
			if len(violations) > 0 {
				t.Errorf("analyze.go still uses hardcoded LLM params as primary path:")
				for _, v := range violations {
					t.Errorf("  %s:%d: %s", v.file, v.line, strings.TrimSpace(v.text))
				}
				t.Error("After refactor, Temperature/MaxTokens/max retries must be read from pipeline config, " +
					"with hardcoded values as fallback only")
			}
		} else {
			t.Log("analyze.go references pipeline config for LLM parameters (refactor complete)")
		}
	})
}

// ============================================================================
// 2B: Model Routing Rules
// ============================================================================

// TestLLMConfigExternalize_MigrationExists_085 verifies that the migration file
// for adding conditions JSONB column to ai_routing_rules exists.
func TestLLMConfigExternalize_MigrationExists_085(t *testing.T) {
	projectRoot := findProjectRoot(t)

	migrationPath := filepath.Join(projectRoot, "migrations", "085_routing_rules_conditions.sql")
	_, err := os.Stat(migrationPath)

	if os.IsNotExist(err) {
		t.Error("migration 085_routing_rules_conditions.sql does not exist; " +
			"this migration must add a conditions JSONB column to ai_routing_rules " +
			"and expand the task_type CHECK constraint to include 'deep_analysis'")
	} else {
		require.NoError(t, err)
		t.Logf("Migration file exists: %s", migrationPath)
	}
}

// TestLLMConfigExternalize_RoutingRulesConditionsColumn verifies that the
// ai_routing_rules table has the new conditions JSONB column after migration 085.
func TestLLMConfigExternalize_RoutingRulesConditionsColumn(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var exists bool
	err := env.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = 'ai_routing_rules'
			  AND column_name = 'conditions'
		)
	`, ).Scan(&exists)

	require.NoError(t, err, "should query information_schema for conditions column")
	assert.True(t, exists,
		"ai_routing_rules should have a 'conditions' JSONB column after migration 085")

	if exists {
		// Also verify the column type is JSONB
		var dataType string
		err = env.DB.QueryRow(ctx, `
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = 'ai_routing_rules'
			  AND column_name = 'conditions'
		`).Scan(&dataType)
		require.NoError(t, err)
		assert.Equal(t, "jsonb", dataType,
			"conditions column should be JSONB; got %s", dataType)
		t.Logf("conditions column exists with type: %s", dataType)
	}
}

// TestLLMConfigExternalize_TaskTypeConstraintIncludesDeepAnalysis verifies that
// the ai_routing_rules task_type CHECK constraint has been expanded to include
// 'deep_analysis'. The original constraint (migration 022) only allows:
// embedding, summarization, extraction, classification.
func TestLLMConfigExternalize_TaskTypeConstraintIncludesDeepAnalysis(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to insert a test row with task_type = 'deep_analysis'.
	// If the constraint has been expanded, this succeeds. If not, it fails.
	_, err := env.DB.Exec(ctx, `
		INSERT INTO ai_routing_rules (name, task_type, preferred_models, priority, is_enabled)
		VALUES ('__e2e_constraint_test', 'deep_analysis', ARRAY['gemini/gemini-2.0-flash'], 0, false)
	`)

	if err != nil {
		if strings.Contains(err.Error(), "ai_routing_rules_task_type_check") ||
			strings.Contains(err.Error(), "check constraint") ||
			strings.Contains(err.Error(), "violates check") {
			t.Error("task_type CHECK constraint on ai_routing_rules does not include 'deep_analysis'; " +
				"migration 085 must ALTER the constraint to add this value")
		} else {
			t.Errorf("unexpected error inserting deep_analysis routing rule: %v", err)
		}
	} else {
		t.Log("task_type CHECK constraint accepts 'deep_analysis'")
		// Clean up the test row
		_, _ = env.DB.Exec(ctx, `DELETE FROM ai_routing_rules WHERE name = '__e2e_constraint_test'`)
	}
}

// TestLLMConfigExternalize_DeepAnalysisRoutingRulesSeeded verifies that migration 085
// seeds routing rules for task_type = 'deep_analysis' that encode the logic currently
// hardcoded in selectModelForDeepAnalysis().
func TestLLMConfigExternalize_DeepAnalysisRoutingRulesSeeded(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Count deep_analysis rules
	var count int
	err := env.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM ai_routing_rules
		WHERE task_type = 'deep_analysis'
		  AND is_enabled = true
	`).Scan(&count)

	require.NoError(t, err, "should query ai_routing_rules for deep_analysis rules")
	assert.Greater(t, count, 0,
		"ai_routing_rules should have at least one enabled deep_analysis routing rule; "+
			"migration 085 must seed rules encoding selectModelForDeepAnalysis() logic")

	t.Logf("Found %d enabled deep_analysis routing rules", count)
}

// TestLLMConfigExternalize_RoutingConditionsMatchLogic verifies that seeded routing
// rules have conditions JSONB that matches the selectModelForDeepAnalysis() logic:
//   - RISK_ISSUE -> gemini-2.5-pro
//   - CUSTOMER + HIGH/MEDIUM -> gemini-2.5-pro
//   - PROJECT_UPDATE + HIGH -> gemini-2.5-pro
//   - default / LOW -> gemini-2.0-flash
func TestLLMConfigExternalize_RoutingConditionsMatchLogic(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("risk_issue_routes_to_pro", func(t *testing.T) {
		var ruleCount int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM ai_routing_rules
			WHERE task_type = 'deep_analysis'
			  AND is_enabled = true
			  AND conditions IS NOT NULL
			  AND conditions @> '{"category": ["RISK_ISSUE"]}'
		`).Scan(&ruleCount)

		if err != nil {
			// conditions column may not exist yet
			t.Logf("Query failed (conditions column may not exist): %v", err)
			t.Error("ai_routing_rules must have a conditions JSONB column with " +
				"seeded rules for RISK_ISSUE routing")
			return
		}

		assert.Greater(t, ruleCount, 0,
			"should have a routing rule with conditions matching RISK_ISSUE category")

		if ruleCount > 0 {
			// Verify the rule routes to a Pro model
			var preferredModels []string
			err = env.DB.QueryRow(ctx, `
				SELECT preferred_models
				FROM ai_routing_rules
				WHERE task_type = 'deep_analysis'
				  AND is_enabled = true
				  AND conditions @> '{"category": ["RISK_ISSUE"]}'
				ORDER BY priority DESC
				LIMIT 1
			`).Scan(&preferredModels)
			require.NoError(t, err)

			hasPro := false
			for _, m := range preferredModels {
				if strings.Contains(m, "pro") || strings.Contains(m, "2.5-pro") {
					hasPro = true
					break
				}
			}
			assert.True(t, hasPro,
				"RISK_ISSUE routing rule should prefer a Pro model; got preferred_models=%v",
				preferredModels)

			t.Logf("RISK_ISSUE rule preferred_models: %v", preferredModels)
		}
	})

	t.Run("default_routes_to_flash", func(t *testing.T) {
		// There should be a default/fallback rule (no conditions or empty conditions)
		// that routes to Flash
		var ruleCount int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM ai_routing_rules
			WHERE task_type = 'deep_analysis'
			  AND is_enabled = true
			  AND (conditions IS NULL OR conditions = '{}')
		`).Scan(&ruleCount)

		if err != nil {
			t.Logf("Query failed (conditions column may not exist): %v", err)
			t.Error("ai_routing_rules must have a default deep_analysis rule")
			return
		}

		assert.Greater(t, ruleCount, 0,
			"should have a default deep_analysis routing rule (no conditions)")

		if ruleCount > 0 {
			var preferredModels []string
			err = env.DB.QueryRow(ctx, `
				SELECT preferred_models
				FROM ai_routing_rules
				WHERE task_type = 'deep_analysis'
				  AND is_enabled = true
				  AND (conditions IS NULL OR conditions = '{}')
				ORDER BY priority ASC
				LIMIT 1
			`).Scan(&preferredModels)
			require.NoError(t, err)

			hasFlash := false
			for _, m := range preferredModels {
				if strings.Contains(m, "flash") || strings.Contains(m, "2.0-flash") {
					hasFlash = true
					break
				}
			}
			assert.True(t, hasFlash,
				"default deep_analysis rule should prefer Flash model; got preferred_models=%v",
				preferredModels)

			t.Logf("Default rule preferred_models: %v", preferredModels)
		}
	})
}

// TestLLMConfigExternalize_SelectModelUsesDBRouting greps analyze.go to verify
// that selectModelForDeepAnalysis() uses DB-backed routing rules instead of
// hardcoded model names as the primary path.
//
// After the refactor:
//   - selectModelForDeepAnalysis() must query ai_routing_rules for deep_analysis
//     task_type and match conditions against the triage metadata.
//   - The hardcoded "gemini-2.5-pro" / "gemini-2.0-flash" literals should only
//     appear as fallback defaults.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestLLMConfigExternalize_SelectModelUsesDBRouting(t *testing.T) {
	projectRoot := findProjectRoot(t)

	analyzeFile := filepath.Join(projectRoot, "services", "ai", "server", "analyze.go")
	content, err := os.ReadFile(analyzeFile)
	require.NoError(t, err, "should read analyze.go")
	src := string(content)

	t.Run("selectModel_uses_db_lookup", func(t *testing.T) {
		// After refactor, selectModelForDeepAnalysis should reference DB routing.
		hasDBLookup := strings.Contains(src, "ai_routing_rules") ||
			strings.Contains(src, "routingRules") ||
			strings.Contains(src, "RoutingRule") ||
			strings.Contains(src, "GetRoutingRules") ||
			strings.Contains(src, "routing.") && strings.Contains(src, "deep_analysis") ||
			strings.Contains(src, "routingResolver") ||
			strings.Contains(src, "matchRoutingRule")

		if !hasDBLookup {
			t.Error("selectModelForDeepAnalysis() does not reference DB-backed routing rules; " +
				"after refactor, model selection must go through ai_routing_rules table first")
		} else {
			t.Log("selectModelForDeepAnalysis() references DB-backed routing (refactor complete)")
		}
	})

	t.Run("hardcoded_models_are_fallback_only", func(t *testing.T) {
		violations := findHardcodedModelViolationsInSelectModel(t, analyzeFile)

		if len(violations) > 0 {
			t.Errorf("selectModelForDeepAnalysis() still uses hardcoded model names as primary path:")
			for _, v := range violations {
				t.Errorf("  %s:%d: %s", v.file, v.line, strings.TrimSpace(v.text))
			}
			t.Error("After refactor, hardcoded model names should only appear as fallback constants")
		} else {
			t.Log("selectModelForDeepAnalysis() no longer uses hardcoded models as primary path")
		}
	})
}

// TestLLMConfigExternalize_RoutingRuleStructHasConditions greps routing.go to verify
// that the RoutingRule Go struct has a Conditions field.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestLLMConfigExternalize_RoutingRuleStructHasConditions(t *testing.T) {
	projectRoot := findProjectRoot(t)

	routingFile := filepath.Join(projectRoot, "services", "ai", "registry", "routing.go")
	content, err := os.ReadFile(routingFile)
	require.NoError(t, err, "should read routing.go")
	src := string(content)

	t.Run("struct_has_conditions_field", func(t *testing.T) {
		hasField := strings.Contains(src, "Conditions") &&
			(strings.Contains(src, "json.RawMessage") ||
				strings.Contains(src, "map[string]") ||
				strings.Contains(src, "RoutingConditions") ||
				strings.Contains(src, "JSONB") ||
				strings.Contains(src, "jsonb") ||
				strings.Contains(src, "pgtype.JSONB") ||
				strings.Contains(src, "[]byte"))

		if !hasField {
			t.Error("RoutingRule struct in routing.go is missing a Conditions field; " +
				"this field must store the JSONB conditions from ai_routing_rules")
		} else {
			t.Log("RoutingRule struct has Conditions field")
		}
	})

	t.Run("validate_accepts_deep_analysis", func(t *testing.T) {
		hasDeepAnalysis := strings.Contains(src, "deep_analysis") ||
			strings.Contains(src, "TaskTypeDeepAnalysis")

		if !hasDeepAnalysis {
			t.Error("routing.go does not include 'deep_analysis' as a valid task type; " +
				"Validate() must accept deep_analysis in the task type switch")
		} else {
			t.Log("routing.go includes deep_analysis as valid task type")
		}
	})
}

// TestLLMConfigExternalize_ScanRoutingRulesReadsConditions greps repository.go
// to verify that scanRoutingRules() reads the conditions JSONB column.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestLLMConfigExternalize_ScanRoutingRulesReadsConditions(t *testing.T) {
	projectRoot := findProjectRoot(t)

	repoFile := filepath.Join(projectRoot, "services", "ai", "registry", "repository.go")
	content, err := os.ReadFile(repoFile)
	require.NoError(t, err, "should read repository.go")
	src := string(content)

	// Find the scanRoutingRules function and check if it scans a Conditions field
	hasScanConditions := false
	lines := strings.Split(src, "\n")
	inScanFunc := false
	for _, line := range lines {
		if strings.Contains(line, "func") && strings.Contains(line, "scanRoutingRules") {
			inScanFunc = true
			continue
		}
		if inScanFunc {
			// Look for Conditions being scanned
			if strings.Contains(line, "Conditions") || strings.Contains(line, "conditions") {
				hasScanConditions = true
				break
			}
			// Stop if we leave the function (two blank lines or next func)
			if strings.HasPrefix(strings.TrimSpace(line), "func ") {
				break
			}
		}
	}

	if !hasScanConditions {
		t.Error("scanRoutingRules() in repository.go does not scan the Conditions field; " +
			"after migration 085, scanRoutingRules must read the conditions JSONB column " +
			"from ai_routing_rules into RoutingRule.Conditions")
	} else {
		t.Log("scanRoutingRules() reads Conditions field")
	}
}

// ============================================================================
// Helpers
// ============================================================================

// llmParamViolation represents a line with hardcoded LLM parameters.
type llmParamViolation struct {
	file string
	line int
	text string
}

// findHardcodedLLMParamViolations scans a Go file for CompletionOptions blocks
// containing raw temperature/max_tokens literals that are not guarded by a fallback comment.
func findHardcodedLLMParamViolations(t *testing.T, filePath, displayName string) []llmParamViolation {
	t.Helper()

	f, err := os.Open(filePath)
	if err != nil {
		t.Logf("Warning: could not open %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	var violations []llmParamViolation
	scanner := bufio.NewScanner(f)
	lineNum := 0
	inCompletionOpts := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Track when we're inside a CompletionOptions block
		if strings.Contains(line, "CompletionOptions{") || strings.Contains(line, "CompletionOptions {") {
			inCompletionOpts = true
			continue
		}
		if inCompletionOpts && trimmed == "}" {
			inCompletionOpts = false
			continue
		}

		if !inCompletionOpts {
			continue
		}

		// Look for hardcoded Temperature or MaxTokens that are NOT fallback-guarded
		isTemperature := strings.Contains(line, "Temperature:") && (strings.Contains(line, "0.1") || strings.Contains(line, "0.2"))
		isMaxTokens := strings.Contains(line, "MaxTokens:") && strings.Contains(line, "8192")

		if isTemperature || isMaxTokens {
			// Check if line has a fallback annotation
			if strings.Contains(line, "fallback") || strings.Contains(line, "Fallback") || strings.Contains(line, "default") {
				continue
			}
			violations = append(violations, llmParamViolation{
				file: displayName,
				line: lineNum,
				text: line,
			})
		}
	}

	return violations
}

// findHardcodedModelViolationsInSelectModel scans analyze.go for hardcoded model
// names (gemini-2.5-pro, gemini-2.0-flash) inside selectModelForDeepAnalysis()
// that are NOT guarded by a fallback comment.
func findHardcodedModelViolationsInSelectModel(t *testing.T, filePath string) []llmParamViolation {
	t.Helper()

	f, err := os.Open(filePath)
	if err != nil {
		t.Logf("Warning: could not open %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	var violations []llmParamViolation
	scanner := bufio.NewScanner(f)
	lineNum := 0
	inSelectModel := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track when we're inside selectModelForDeepAnalysis
		if strings.Contains(line, "func selectModelForDeepAnalysis(") || strings.Contains(line, "func (s *AIServer) selectModelForDeepAnalysis(") {
			inSelectModel = true
			continue
		}
		// Exit when we hit a top-level closing brace (not indented)
		if inSelectModel && trimmed == "}" && !strings.Contains(line, "\t\t") {
			inSelectModel = false
			continue
		}

		if !inSelectModel {
			continue
		}

		// Skip comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Look for hardcoded return of model names
		if strings.Contains(line, `"gemini-2.5-pro"`) || strings.Contains(line, `"gemini-2.0-flash"`) {
			if strings.Contains(line, "fallback") || strings.Contains(line, "Fallback") || strings.Contains(line, "default") || strings.Contains(line, "Default") {
				continue
			}
			violations = append(violations, llmParamViolation{
				file: filePath,
				line: lineNum,
				text: line,
			})
		}
	}

	return violations
}

// safeFloat dereferences a *float64, returning 0 if nil.
func safeFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// safeInt dereferences an *int, returning 0 if nil.
func safeInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
