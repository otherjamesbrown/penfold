// Package server — reproduction tests for pf-5a2e1a: model selection routing bug.
//
// Bug summary: deep_analyze stage has DB routing rules in ai_routing_rules (7 rules
// for deep_analysis task type) but they are NEVER matched. Both flash and pro model
// traces show routing_rule: "" — all requests fall through to hardcoded fallback logic
// in selectModelForDeepAnalysisFallback.
//
// Root causes reproduced here:
//  1. Production server is constructed via NewAIServerWithLangfuse → NewAIServer,
//     which leaves s.registry == nil. There is no WithRegistry() builder method and
//     main.go never calls NewAIServerWithAll / NewAIServerWithRouter. Because
//     s.registry is nil the entire DB routing block is bypassed unconditionally.
//
//  2. ACTION_REQUEST + HIGH has no explicit case in selectModelForDeepAnalysisFallback.
//     It falls through to the configDefault branch (or the final flash catch-all),
//     returning gemini-2.5-flash when gemini-2.5-pro is the correct model for a
//     HIGH-importance action request.
//
// These tests are expected to FAIL against the current code, confirming the bugs.
package server

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/services/ai/registry"
)

// deepAnalysisRulesForBug returns the seven deep_analysis routing rules that mirror
// what is seeded in the ai_routing_rules table in production. These rules SHOULD be
// consulted by selectModelForDeepAnalysis but currently are not because s.registry is nil.
func deepAnalysisRulesForBug() []*registry.RoutingRule {
	return []*registry.RoutingRule{
		{
			ID:              "rule-risk-issue",
			Name:            "deep_analysis_risk_issue_pro",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-pro"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category": {"RISK_ISSUE"},
			},
		},
		{
			ID:              "rule-customer-high",
			Name:            "deep_analysis_customer_high_pro",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-pro"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"CUSTOMER"},
				"importance": {"HIGH"},
			},
		},
		{
			ID:              "rule-customer-medium",
			Name:            "deep_analysis_customer_medium_pro",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-pro"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"CUSTOMER"},
				"importance": {"MEDIUM"},
			},
		},
		{
			ID:              "rule-project-high",
			Name:            "deep_analysis_project_update_high_pro",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-pro"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"PROJECT_UPDATE"},
				"importance": {"HIGH"},
			},
		},
		{
			ID:              "rule-project-medium",
			Name:            "deep_analysis_project_update_medium_flash",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-flash"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"PROJECT_UPDATE"},
				"importance": {"MEDIUM"},
			},
		},
		{
			ID:              "rule-action-medium",
			Name:            "deep_analysis_action_request_medium_flash",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-flash"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"ACTION_REQUEST"},
				"importance": {"MEDIUM"},
			},
		},
		{
			ID:              "rule-action-high",
			Name:            "deep_analysis_action_request_high_pro",
			TaskType:        registry.TaskTypeDeepAnalysis,
			PreferredModels: []string{"gemini/gemini-2.5-pro"},
			IsEnabled:       true,
			Conditions: map[string][]string{
				"category":   {"ACTION_REQUEST"},
				"importance": {"HIGH"},
			},
		},
	}
}

// =============================================================================
// Bug 1 — Root cause: s.registry is nil in production
//
// main.go uses NewAIServerWithLangfuse → NewAIServer, which never sets s.registry.
// There is no WithRegistry() builder method. Because s.registry is nil the entire
// DB routing block in selectModelForDeepAnalysis is skipped unconditionally.
//
// The test below mirrors the production construction path and proves the registry
// is nil. The fix requires either:
//   a) Adding a WithRegistry() builder method and calling it from main.go, OR
//   b) Changing main.go to call NewAIServerWithAll/NewAIServerWithRouter.
// =============================================================================

// TestBug_pf5a2e1a_ProductionServerHasNilRegistry verifies that the production
// construction path now wires the registry correctly via WithRegistry().
//
// Before the fix: NewAIServerWithLangfuse left s.registry nil with no way to set it.
// After the fix: WithRegistry() builder is added; main.go calls it when DB is configured.
func TestBug_pf5a2e1a_ProductionServerHasNilRegistry(t *testing.T) {
	// Reproduce the fixed production construction path from main.go:
	//   aiServer := server.NewAIServerWithLangfuse(cfg, logger, compositeBackend, langfuseIngestion)
	//   aiServer.WithRegistry(dbRegistry)  // <-- added by fix
	rules := deepAnalysisRulesForBug()
	rulesByTask := map[string][]*registry.RoutingRule{
		registry.TaskTypeDeepAnalysis: rules,
	}
	repo := &mockRoutingRepo{rulesByTask: rulesByTask}
	reg := registry.NewDBRegistry(repo, nil)

	s := NewAIServerWithLangfuse(testConfig(), testLogger(), &mockBackend{}, nil)
	s.WithRegistry(reg)

	// The registry must be non-nil for DB routing to work.
	if s.registry == nil {
		t.Errorf("FIX INCOMPLETE pf-5a2e1a: s.registry is nil after WithRegistry() — " +
			"WithRegistry() must set s.registry on the server.")
	}
}

// TestBug_pf5a2e1a_DBRoutingRulesIgnoredWhenRegistryIsNil verifies that after the fix,
// a server wired with a registry via WithRegistry() correctly consults DB routing rules
// and returns a non-empty RuleName for known category/importance combinations.
//
// Before the fix: registry was always nil; RuleName was always empty.
// After the fix: WithRegistry() wires the registry; DB rules are consulted.
func TestBug_pf5a2e1a_DBRoutingRulesIgnoredWhenRegistryIsNil(t *testing.T) {
	// Build server with registry wired in — this is the fixed production path.
	rules := deepAnalysisRulesForBug()
	rulesByTask := map[string][]*registry.RoutingRule{
		registry.TaskTypeDeepAnalysis: rules,
	}
	s := newTestServerWithRegistry(&mockBackend{}, rulesByTask)
	ctx := context.Background()

	tests := []struct {
		name         string
		category     string
		importance   string
		wantRuleName string
	}{
		{"RISK_ISSUE + HIGH should use rule", "RISK_ISSUE", "HIGH", "deep_analysis_risk_issue_pro"},
		{"ACTION_REQUEST + HIGH should use rule", "ACTION_REQUEST", "HIGH", "deep_analysis_action_request_high_pro"},
		{"CUSTOMER + HIGH should use rule", "CUSTOMER", "HIGH", "deep_analysis_customer_high_pro"},
		{"PROJECT_UPDATE + MEDIUM should use rule", "PROJECT_UPDATE", "MEDIUM", "deep_analysis_project_update_medium_flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.selectModelForDeepAnalysis(ctx, tt.category, tt.importance, "", "")
			// A non-empty RuleName proves the DB path was reached.
			if result.RuleName != tt.wantRuleName {
				t.Errorf("DB routing rule not matched: got RuleName=%q, want %q "+
					"for category=%q importance=%q.",
					result.RuleName, tt.wantRuleName, tt.category, tt.importance)
			}
		})
	}
}

// TestBug_pf5a2e1a_WithRegistryEnablesDBRouting verifies that when s.registry IS
// set (i.e. after the fix), the DB routing path is actually consulted and RuleName
// is populated. Uses the existing newTestServerWithRegistry helper from
// embedding_routing_test.go (same package).
//
// This test should PASS once a WithRegistry() method is added to the server and
// called from main.go. It serves as the acceptance test for the fix.
func TestBug_pf5a2e1a_WithRegistryEnablesDBRouting(t *testing.T) {
	rules := deepAnalysisRulesForBug()
	rulesByTask := map[string][]*registry.RoutingRule{
		registry.TaskTypeDeepAnalysis: rules,
	}
	// newTestServerWithRegistry (defined in embedding_routing_test.go) wires the
	// registry correctly — this is the configuration the fix must achieve in production.
	s := newTestServerWithRegistry(&mockBackend{}, rulesByTask)
	ctx := context.Background()

	tests := []struct {
		name         string
		category     string
		importance   string
		wantModel    string
		wantRuleName string
	}{
		{
			name:         "RISK_ISSUE + HIGH uses DB rule",
			category:     "RISK_ISSUE",
			importance:   "HIGH",
			wantModel:    "gemini-2.5-pro",
			wantRuleName: "deep_analysis_risk_issue_pro",
		},
		{
			name:         "CUSTOMER + HIGH uses DB rule",
			category:     "CUSTOMER",
			importance:   "HIGH",
			wantModel:    "gemini-2.5-pro",
			wantRuleName: "deep_analysis_customer_high_pro",
		},
		{
			name:         "PROJECT_UPDATE + MEDIUM uses DB rule (flash)",
			category:     "PROJECT_UPDATE",
			importance:   "MEDIUM",
			wantModel:    "gemini-2.5-flash",
			wantRuleName: "deep_analysis_project_update_medium_flash",
		},
		{
			name:         "ACTION_REQUEST + MEDIUM uses DB rule (flash)",
			category:     "ACTION_REQUEST",
			importance:   "MEDIUM",
			wantModel:    "gemini-2.5-flash",
			wantRuleName: "deep_analysis_action_request_medium_flash",
		},
		{
			name:         "ACTION_REQUEST + HIGH uses DB rule (pro)",
			category:     "ACTION_REQUEST",
			importance:   "HIGH",
			wantModel:    "gemini-2.5-pro",
			wantRuleName: "deep_analysis_action_request_high_pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.selectModelForDeepAnalysis(ctx, tt.category, tt.importance, "", "")
			if result.RuleName != tt.wantRuleName {
				t.Errorf("got RuleName=%q want %q — DB routing rule not matched",
					result.RuleName, tt.wantRuleName)
			}
			if result.Model != tt.wantModel {
				t.Errorf("got model=%q want %q", result.Model, tt.wantModel)
			}
		})
	}
}

// =============================================================================
// Bug 2 — ACTION_REQUEST + HIGH missing from fallback
//
// selectModelForDeepAnalysisFallback has explicit cases for:
//   - ACTION_REQUEST + MEDIUM → flash
//   - ACTION_REQUEST + LOW   → flash (via generic LOW catch)
//
// But ACTION_REQUEST + HIGH has NO explicit case. It falls to configDefault
// (which may be empty in many environments), then to the final flash catch-all —
// returning flash when pro is the correct model for a HIGH-importance action.
// =============================================================================

// TestBug_pf5a2e1a_ActionRequestHighFallback reproduces the missing-case bug in
// selectModelForDeepAnalysisFallback for ACTION_REQUEST + HIGH.
//
// Expected (post-fix): "gemini-2.5-pro"
// Actual (current bug): "gemini-2.5-flash" (falls through to the generic flash default).
//
// This test is expected to FAIL against the current code.
func TestBug_pf5a2e1a_ActionRequestHighFallback(t *testing.T) {
	// Pass empty configDefault so the final flash catch-all fires and exposes the bug.
	got := selectModelForDeepAnalysisFallback("ACTION_REQUEST", "HIGH", "")
	want := "gemini-2.5-pro"
	if got != want {
		t.Errorf("BUG pf-5a2e1a: ACTION_REQUEST + HIGH fallback returned %q, want %q — "+
			"no explicit case exists for this combination in selectModelForDeepAnalysisFallback; "+
			"request falls through to the generic flash catch-all.",
			got, want)
	}
}

// TestBug_pf5a2e1a_ActionRequestHighFallbackWithConfigDefault documents that even
// when a configDefault is provided, ACTION_REQUEST + HIGH returns the wrong model
// if configDefault happens to be flash (common in dev environments where
// AI_MODEL_ANALYZE=gemini-2.5-flash).
//
// This test is expected to FAIL against the current code.
func TestBug_pf5a2e1a_ActionRequestHighFallbackWithConfigDefault(t *testing.T) {
	got := selectModelForDeepAnalysisFallback("ACTION_REQUEST", "HIGH", "gemini-2.5-flash")
	wantAfterFix := "gemini-2.5-pro"
	if got == wantAfterFix {
		// Fix was applied — test passes.
		return
	}
	t.Errorf("BUG pf-5a2e1a: ACTION_REQUEST + HIGH with configDefault=%q returned %q; "+
		"want %q after fix. "+
		"The explicit ACTION_REQUEST+HIGH case must be added BEFORE the configDefault branch.",
		"gemini-2.5-flash", got, wantAfterFix)
}

// =============================================================================
// Regression guard: verify MatchesConditions works correctly
//
// This isolates whether any future bug is in MatchesConditions or in the caller.
// All of these are expected to PASS today (MatchesConditions is not broken).
// =============================================================================

// TestBug_pf5a2e1a_MatchesConditionsWorks verifies registry.MatchesConditions
// correctly matches ACTION_REQUEST + HIGH conditions.
func TestBug_pf5a2e1a_MatchesConditionsWorks(t *testing.T) {
	conditions := map[string][]string{
		"category":   {"ACTION_REQUEST"},
		"importance": {"HIGH"},
	}
	attrs := map[string]string{
		"category":   "ACTION_REQUEST",
		"importance": "HIGH",
	}
	if !registry.MatchesConditions(conditions, attrs) {
		t.Error("BUG: MatchesConditions returned false for ACTION_REQUEST + HIGH — " +
			"the condition matching logic itself is broken; fix MatchesConditions first")
	}
}

// TestBug_pf5a2e1a_MatchesConditionsCaseInsensitive verifies case-insensitive
// matching, since selectModelForDeepAnalysis upper-cases inputs before calling
// MatchesConditions and DB rules store upper-case values.
func TestBug_pf5a2e1a_MatchesConditionsCaseInsensitive(t *testing.T) {
	tests := []struct {
		name       string
		conditions map[string][]string
		attrs      map[string]string
		wantMatch  bool
	}{
		{
			name: "exact case match",
			conditions: map[string][]string{
				"category": {"RISK_ISSUE"},
			},
			attrs:     map[string]string{"category": "RISK_ISSUE"},
			wantMatch: true,
		},
		{
			name: "lower in attrs matches upper in conditions",
			conditions: map[string][]string{
				"category": {"RISK_ISSUE"},
			},
			attrs:     map[string]string{"category": "risk_issue"},
			wantMatch: true,
		},
		{
			name: "missing attr key returns false",
			conditions: map[string][]string{
				"category": {"RISK_ISSUE"},
			},
			attrs:     map[string]string{},
			wantMatch: false,
		},
		{
			name: "multi-condition AND — both must match",
			conditions: map[string][]string{
				"category":   {"ACTION_REQUEST"},
				"importance": {"HIGH"},
			},
			attrs:     map[string]string{"category": "ACTION_REQUEST", "importance": "HIGH"},
			wantMatch: true,
		},
		{
			name: "multi-condition AND — one missing fails",
			conditions: map[string][]string{
				"category":   {"ACTION_REQUEST"},
				"importance": {"HIGH"},
			},
			attrs:     map[string]string{"category": "ACTION_REQUEST"}, // importance absent
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.MatchesConditions(tt.conditions, tt.attrs)
			if got != tt.wantMatch {
				t.Errorf("MatchesConditions() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}
