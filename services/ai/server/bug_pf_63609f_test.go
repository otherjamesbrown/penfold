// Package server contains reproduction tests for bug pf-63609f.
//
// Bug: DECISION category has no routing rule in ai_routing_rules and no case
// in selectModelForDeepAnalysisFallback. ACTION_REQUEST has only a MEDIUM rule
// in ai_routing_rules — HIGH falls through to default gemini-2.5-flash. Both
// bugs cause HIGH-importance traces to incorrectly use flash instead of pro.
//
// Root cause (selectModelForDeepAnalysisFallback):
//   - DECISION category is entirely absent from the fallback switch. Any DECISION
//     trace (regardless of importance) falls through to the configDefault or the
//     final hardcoded "gemini-2.5-flash". A HIGH-importance decision email should
//     use gemini-2.5-pro.
//   - ACTION_REQUEST + HIGH: the fallback switch DOES have a case for this, so
//     the fallback itself is correct. The actual bug lives in ai_routing_rules
//     (no DB rule for ACTION_REQUEST/HIGH means the DB primary path misses it
//     and falls through to the fallback). This test documents the DB-rule gap by
//     confirming the fallback is the only safety net.
//
// TestSelectModelForDeepAnalysis_DecisionCategory is expected to FAIL against
// current code because DECISION is not handled in selectModelForDeepAnalysisFallback.
//
// TestSelectModelForDeepAnalysis_ActionRequestHigh is expected to PASS against
// current code (the fallback handles ACTION_REQUEST/HIGH correctly), but documents
// that the DB routing rule is missing — the fallback is the only defence.
package server

import (
	"strings"
	"testing"
)

// TestSelectModelForDeepAnalysis_DecisionCategory verifies that when category is
// "DECISION" and importance is "HIGH", the fallback function returns gemini-2.5-pro.
//
// Currently FAILS: DECISION is not handled in selectModelForDeepAnalysisFallback.
// It falls through to the default branch and returns gemini-2.5-flash (or
// configDefault if set). This means HIGH-importance DECISION emails are analysed
// with the balanced model rather than the quality model, losing analytical depth
// on content that affects outcomes.
//
// Fix required: add a DECISION case to selectModelForDeepAnalysisFallback that
// returns gemini-2.5-pro for HIGH (and optionally MEDIUM) importance.
func TestSelectModelForDeepAnalysis_DecisionCategory(t *testing.T) {
	tests := []struct {
		name       string
		category   string
		importance string
		wantModel  string
	}{
		{
			// PRIMARY REPRODUCTION: DECISION + HIGH must use gemini-2.5-pro.
			// Currently FAILS: falls through to default and returns gemini-2.5-flash.
			name:       "DECISION + HIGH must use gemini-2.5-pro",
			category:   "DECISION",
			importance: "HIGH",
			wantModel:  "gemini-2.5-pro",
		},
		{
			// DECISION + MEDIUM should also use pro (same rationale as CUSTOMER/MEDIUM).
			// Currently FAILS: falls through to default and returns gemini-2.5-flash.
			name:       "DECISION + MEDIUM must use gemini-2.5-pro",
			category:   "DECISION",
			importance: "MEDIUM",
			wantModel:  "gemini-2.5-pro",
		},
		{
			// DECISION + LOW is covered by the existing importance == "LOW" case.
			// Currently PASSES: returns gemini-2.5-flash via the LOW branch.
			name:      "DECISION + LOW uses gemini-2.5-flash via LOW branch",
			category:  "DECISION",
			importance: "LOW",
			wantModel: "gemini-2.5-flash",
		},
		{
			// Case-insensitive normalisation: caller uppercases inputs before invoking fallback.
			// Simulate that behaviour here to confirm the fix handles normalised input.
			// Currently FAILS for same reason as DECISION + HIGH.
			name:       "case normalised: DECISION + HIGH must use gemini-2.5-pro",
			category:   strings.ToUpper("decision"),
			importance: strings.ToUpper("high"),
			wantModel:  "gemini-2.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inputs are already uppercased (mirroring what selectModelForDeepAnalysis
			// does before invoking the fallback). Pass empty configDefault so the test
			// isolates the category/importance logic and is not influenced by any
			// external configuration.
			got := selectModelForDeepAnalysisFallback(tt.category, tt.importance, "")

			if got != tt.wantModel {
				t.Errorf(
					"BUG pf-63609f: selectModelForDeepAnalysisFallback(%q, %q, \"\") = %q, want %q\n"+
						"DECISION category has no case in selectModelForDeepAnalysisFallback.\n"+
						"HIGH-importance DECISION traces incorrectly use flash instead of pro.\n"+
						"Fix: add a DECISION case returning gemini-2.5-pro for HIGH/MEDIUM importance.",
					tt.category, tt.importance, got, tt.wantModel,
				)
			}
		})
	}
}

// TestSelectModelForDeepAnalysis_ActionRequestHigh verifies that when category is
// "ACTION_REQUEST" and importance is "HIGH", the fallback returns gemini-2.5-pro.
//
// The fallback function already contains an ACTION_REQUEST/HIGH case, so the
// DECISION+HIGH and DECISION+MEDIUM sub-cases here are EXPECTED TO PASS against
// current code. The purpose of this test is to:
//
//  1. Document that the fallback is the only safety net for ACTION_REQUEST/HIGH
//     (no DB rule exists in ai_routing_rules for this combination).
//  2. Act as a regression guard — the fix for DECISION must not inadvertently
//     break the ACTION_REQUEST/HIGH path.
//  3. Provide explicit test coverage that was absent from analyze_test.go
//     (TestSelectModelForDeepAnalysis does not include ACTION_REQUEST/HIGH).
//
// The companion migration fix must add a DB routing rule for ACTION_REQUEST/HIGH
// so the primary DB path also covers this case, not just this fallback.
func TestSelectModelForDeepAnalysis_ActionRequestHigh(t *testing.T) {
	tests := []struct {
		name       string
		category   string
		importance string
		wantModel  string
	}{
		{
			// ACTION_REQUEST + HIGH: fallback handles this correctly already.
			// PASSES with current code. Absence of test coverage was the oversight.
			name:       "ACTION_REQUEST + HIGH must use gemini-2.5-pro",
			category:   "ACTION_REQUEST",
			importance: "HIGH",
			wantModel:  "gemini-2.5-pro",
		},
		{
			// ACTION_REQUEST + MEDIUM: uses flash per design.
			// PASSES with current code.
			name:       "ACTION_REQUEST + MEDIUM uses gemini-2.5-flash",
			category:   "ACTION_REQUEST",
			importance: "MEDIUM",
			wantModel:  "gemini-2.5-flash",
		},
		{
			// ACTION_REQUEST + LOW: covered by the importance == "LOW" branch.
			// PASSES with current code.
			name:       "ACTION_REQUEST + LOW uses gemini-2.5-flash via LOW branch",
			category:   "ACTION_REQUEST",
			importance: "LOW",
			wantModel:  "gemini-2.5-flash",
		},
		{
			// Case-normalised variant to mirror real call-site behaviour.
			// PASSES with current code.
			name:       "case normalised: ACTION_REQUEST + HIGH must use gemini-2.5-pro",
			category:   strings.ToUpper("action_request"),
			importance: strings.ToUpper("high"),
			wantModel:  "gemini-2.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inputs are pre-uppercased. Empty configDefault isolates the logic.
			got := selectModelForDeepAnalysisFallback(tt.category, tt.importance, "")

			if got != tt.wantModel {
				t.Errorf(
					"REGRESSION pf-63609f: selectModelForDeepAnalysisFallback(%q, %q, \"\") = %q, want %q\n"+
						"ACTION_REQUEST/HIGH fallback case must return gemini-2.5-pro.\n"+
						"Note: this fallback is the only current safety net — no DB routing rule exists\n"+
						"for ACTION_REQUEST/HIGH deep_analysis. The migration fix must add that rule.",
					tt.category, tt.importance, got, tt.wantModel,
				)
			}
		})
	}
}
