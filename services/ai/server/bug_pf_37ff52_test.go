// Package server contains reproduction tests for bug pf-37ff52.
//
// Bug: quality_gate_triggered fires based on email thread subject keywords
// rather than actual body content. Two compounding problems:
//
//  1. stripEmailAddressesKeepSubject() prepends the Subject line to the body
//     without any labeling that distinguishes it from body content. The model
//     receives "Subject: Immediate CLIC Action Required\n---\nCan we get a call?"
//     and treats the subject as "content that contains risks".
//
//  2. There is no safety net: when quality_gate_triggered=true but the quality
//     gate pass itself finds zero detailed_risks and semResp.Risks is also empty,
//     quality_gate_triggered should be forced back to false. No such guard exists.
//
// Evidence: Trace 56b4e25c — body is just "Can we get a call?" (scheduling),
// risks=EMPTY, action_items=EMPTY, but quality_gate_triggered=true because
// subject contains "Immediate...Action Required...Issues Impacting MTC Revenue".
//
// These tests reproduce both failure modes. They are expected to FAIL against
// the current implementation.
package server

import (
	"context"
	"strings"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
)

// =============================================================================
// Test 1: stripEmailAddressesKeepSubject must label the subject distinctly
// =============================================================================

// TestStripEmailAddressesKeepSubject_SubjectIsLabeled_BugReproduction verifies
// that when an urgent subject keyword appears in the thread subject (not the body),
// the stripped output labels the subject with a clear marker so the LLM can
// distinguish it from body content.
//
// Currently FAILS: stripEmailAddressesKeepSubject() outputs:
//
//	"Subject: Immediate CLIC Action Required — Issues Impacting MTC Revenue\n---\nCan we get a call?"
//
// The LLM sees the subject and body as one undifferentiated "content" block.
// The fix should produce output that contains a distinct label such as
// "[THREAD SUBJECT]" or "## Email Subject" before the subject text, and a
// separate label such as "[BODY]" or "## Email Body" before the body text.
func TestStripEmailAddressesKeepSubject_SubjectIsLabeled_BugReproduction(t *testing.T) {
	// This is the content format produced by the ingestion pipeline for an email
	// thread where the subject contains urgent-sounding keywords but the body
	// is a trivial scheduling message — exactly the pattern in trace 56b4e25c.
	content := "EMAIL METADATA:\n" +
		"From: alice@example.com\n" +
		"To: bob@example.com\n" +
		"CC: carol@example.com\n" +
		"Subject: Immediate CLIC Action Required — Issues Impacting MTC Revenue\n" +
		"---\n" +
		"BODY:\n" +
		"Can we get a call?"

	result := stripEmailAddressesKeepSubject(content)

	// The subject text should still be present (we don't want to drop it).
	if !strings.Contains(result, "Immediate CLIC Action Required") {
		t.Errorf("BUG pf-37ff52 (pre): subject text was dropped entirely; want it preserved but labeled.\nGot: %q", result)
	}

	// The body text should still be present.
	if !strings.Contains(result, "Can we get a call?") {
		t.Errorf("BUG pf-37ff52 (pre): body text was dropped; want it preserved.\nGot: %q", result)
	}

	// -------------------------------------------------------------------
	// CORE ASSERTION: subject must be labeled so the LLM can distinguish
	// it from body content.
	//
	// Currently FAILS: the output is just:
	//   "Subject: Immediate CLIC Action Required — ...\n---\nCan we get a call?"
	//
	// The bare "Subject: ..." line with no wrapper makes the LLM treat the
	// subject keywords as body "content that contains risks".
	//
	// Fix: wrap the subject in a distinctive marker, e.g.:
	//   "[THREAD SUBJECT]: Immediate CLIC Action Required — ...\n[BODY]:\nCan we get a call?"
	// or similar that clearly demarcates subject from body for the LLM.
	// -------------------------------------------------------------------
	hasSubjectLabel := strings.Contains(result, "[THREAD SUBJECT]") ||
		strings.Contains(result, "[EMAIL SUBJECT]") ||
		strings.Contains(result, "## Subject") ||
		strings.Contains(result, "## Email Subject") ||
		strings.Contains(result, "THREAD SUBJECT:")

	if !hasSubjectLabel {
		t.Errorf("BUG pf-37ff52: stripEmailAddressesKeepSubject() does not label the subject distinctly.\n"+
			"The model cannot distinguish subject keywords from body content.\n"+
			"Got output: %q\n"+
			"Want: output containing a distinct subject label like '[THREAD SUBJECT]', '## Subject', etc.\n"+
			"This causes quality_gate_triggered=true for trivial bodies with alarming subject lines.",
			result)
	}

	hasBodyLabel := strings.Contains(result, "[BODY]") ||
		strings.Contains(result, "## Body") ||
		strings.Contains(result, "## Email Body") ||
		strings.Contains(result, "BODY:")

	if !hasBodyLabel {
		t.Errorf("BUG pf-37ff52: stripEmailAddressesKeepSubject() does not label the body section distinctly.\n"+
			"Got output: %q\n"+
			"Want: output containing a distinct body label like '[BODY]', '## Body', etc.",
			result)
	}
}

// TestStripEmailAddressesKeepSubject_UrgentSubjectTrivialBody_SubjectSeparatedFromBody
// verifies the subject and body are structurally separated so they cannot be
// confused by the LLM. Uses the exact subject pattern from trace 56b4e25c.
func TestStripEmailAddressesKeepSubject_UrgentSubjectTrivialBody_SubjectSeparatedFromBody(t *testing.T) {
	content := "EMAIL METADATA:\n" +
		"From: sender@corp.com\n" +
		"To: recipient@corp.com\n" +
		"Subject: Re: Immediate Action Required — Revenue Issues Impacting MTC\n" +
		"---\n" +
		"BODY:\n" +
		"Hey, can we schedule a quick call this week?"

	result := stripEmailAddressesKeepSubject(content)

	// Find where the subject text appears in the result.
	subjectIdx := strings.Index(result, "Immediate Action Required")
	bodyIdx := strings.Index(result, "can we schedule a quick call")

	if subjectIdx == -1 {
		t.Fatalf("subject text not found in result: %q", result)
	}
	if bodyIdx == -1 {
		t.Fatalf("body text not found in result: %q", result)
	}

	// There must be some structural separator between subject and body.
	// We check the region between the end of the subject phrase and the
	// start of the body phrase for a labeling marker.
	subjectEnd := subjectIdx + len("Immediate Action Required")
	interstitial := result[subjectEnd:bodyIdx]

	hasStructure := strings.Contains(interstitial, "[") ||
		strings.Contains(interstitial, "#") ||
		strings.Contains(interstitial, "BODY") ||
		strings.Contains(interstitial, "body")

	if !hasStructure {
		t.Errorf("BUG pf-37ff52: no structural labeling between subject and body in stripped output.\n"+
			"Region between subject and body: %q\n"+
			"Full output: %q\n"+
			"Want: a label or marker (e.g. '[BODY]', '## Body') to demarcate subject from body for the LLM.",
			interstitial, result)
	}
}

// =============================================================================
// Test 2: Quality gate safety net — empty risks must force gate to false
// =============================================================================

// TestExtractEntities_QualityGateForcedFalseWhenRisksEmpty_BugReproduction
// reproduces the core of trace 56b4e25c: triage_category=RISK_ISSUE, semantic
// risks=[], action_items=[], but quality_gate_triggered=true in the response.
//
// The quality gate exists to re-examine content when RISK_ISSUE is set but
// risks are empty. However, if BOTH passes (semantic + quality gate) return
// empty risks AND empty action_items, the content is clearly not risky. The
// response should set quality_gate_triggered=false in that case.
//
// Currently FAILS: ExtractEntities sets qualityGateTriggered=true as soon as
// the quality gate runs (line ~491 in extract.go), regardless of what the
// quality gate pass finds. No post-hoc safety net corrects it.
func TestExtractEntities_QualityGateForcedFalseWhenRisksEmpty_BugReproduction(t *testing.T) {
	// Simulate a RISK_ISSUE email where both the semantic pass and the quality
	// gate pass find zero risks — matching trace 56b4e25c.
	callCount := 0
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, _ backend.CompletionOptions) (*backend.CompletionResult, error) {
			callCount++
			switch callCount {
			case 1:
				// NER pass — finds nothing of interest
				return &backend.CompletionResult{
					Content:      `{"people":[],"dates":[],"projects":[],"organisations":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			case 2:
				// Semantic pass — empty risks, empty action_items (trivial body)
				return &backend.CompletionResult{
					Content:      `{"action_items":[],"decisions":[],"risks":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			case 3:
				// Quality gate pass — also finds zero detailed risks
				return &backend.CompletionResult{
					Content:      `{"risks":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			default:
				t.Errorf("unexpected call %d to ChatCompletion", callCount)
				return &backend.CompletionResult{Content: `{}`}, nil
			}
		},
	}

	srv := NewAIServer(testConfig(), logging.NewNopLogger(), be)

	// Use the exact pattern from trace 56b4e25c: RISK_ISSUE triage with a trivial body.
	triageRiskIssue := "RISK_ISSUE"
	tenantID := "tenant-test"
	req := &aiv1.ExtractEntitiesRequest{
		Content:        "EMAIL METADATA:\nFrom: sender@corp.com\nTo: recipient@corp.com\nSubject: Immediate CLIC Action Required — Issues Impacting MTC Revenue\n---\nBODY:\nCan we get a call?",
		TriageCategory: &triageRiskIssue,
		TenantId:       &tenantID,
	}

	resp, err := srv.ExtractEntities(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractEntities returned unexpected error: %v", err)
	}

	// -------------------------------------------------------------------
	// CORE ASSERTION: quality_gate_triggered must be false when both
	// semantic and quality gate passes return zero risks.
	//
	// Currently FAILS: resp.QualityGateTriggered=true because the flag is
	// set when the gate *runs*, not only when it finds something.
	//
	// Fix: after the quality gate pass, if detailed_risks is empty AND
	// semResp.Risks is empty, set qualityGateTriggered=false (safety net).
	// -------------------------------------------------------------------
	if resp.QualityGateTriggered {
		t.Errorf("BUG pf-37ff52: quality_gate_triggered=true but both semantic and quality gate passes found zero risks.\n"+
			"Semantic risks: %v\n"+
			"Detailed risks: %v\n"+
			"QualityGateTriggered: %v\n"+
			"This is a false positive. When no risks are found by either pass, the gate must be forced to false.\n"+
			"Evidence: trace 56b4e25c — body='Can we get a call?' triggers gate due to alarming subject line.",
			resp.Risks,
			resp.DetailedRisks,
			resp.QualityGateTriggered)
	}

	// Verify the response has no risks (sanity check on mock setup).
	if len(resp.Risks) > 0 {
		t.Errorf("expected zero semantic risks, got %v", resp.Risks)
	}
	if len(resp.DetailedRisks) > 0 {
		t.Errorf("expected zero detailed risks from quality gate, got %v", resp.DetailedRisks)
	}
}

// TestExtractEntities_QualityGateTriggeredTrueWhenRisksFound verifies that
// quality_gate_triggered=true IS correct when the quality gate pass finds
// actual detailed risks. This is the non-bug case — the gate should fire
// when real risks are present.
//
// This test should PASS (it documents correct behavior and guards against
// over-correction in the fix).
func TestExtractEntities_QualityGateTriggeredTrueWhenRisksFound(t *testing.T) {
	callCount := 0
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, _ backend.CompletionOptions) (*backend.CompletionResult, error) {
			callCount++
			switch callCount {
			case 1:
				// NER pass
				return &backend.CompletionResult{
					Content:      `{"people":[],"dates":[],"projects":[],"organisations":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			case 2:
				// Semantic pass — empty risks (triggers the gate)
				return &backend.CompletionResult{
					Content:      `{"action_items":[],"decisions":[],"risks":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			case 3:
				// Quality gate pass — finds a genuine risk in the body
				return &backend.CompletionResult{
					Content:      `{"risks":[{"description":"Production database at risk of data loss","severity_hint":"critical","owner_hint":"DevOps"}]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			default:
				t.Errorf("unexpected call %d to ChatCompletion", callCount)
				return &backend.CompletionResult{Content: `{}`}, nil
			}
		},
	}

	srv := NewAIServer(testConfig(), logging.NewNopLogger(), be)

	triageRiskIssue2 := "RISK_ISSUE"
	tenantID2 := "tenant-test"
	req := &aiv1.ExtractEntitiesRequest{
		Content:        "EMAIL METADATA:\nFrom: ops@corp.com\nTo: team@corp.com\nSubject: P0 Incident\n---\nBODY:\nThe production database is at risk of data loss. Immediate action required.",
		TriageCategory: &triageRiskIssue2,
		TenantId:       &tenantID2,
	}

	resp, err := srv.ExtractEntities(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractEntities returned unexpected error: %v", err)
	}

	// When real risks ARE found by the quality gate, the flag SHOULD be true.
	if !resp.QualityGateTriggered {
		t.Errorf("expected quality_gate_triggered=true when quality gate finds genuine risks, got false")
	}

	if len(resp.DetailedRisks) == 0 {
		t.Errorf("expected detailed risks to be populated when quality gate finds risks, got empty")
	}
}

// TestExtractEntities_QualityGateNotTriggeredForNonRiskIssue verifies that
// non-RISK_ISSUE triage categories never trigger the quality gate, even if the
// subject line contains alarming keywords. This documents the correct guard.
//
// This test should PASS (existing behavior is correct for this case).
func TestExtractEntities_QualityGateNotTriggeredForNonRiskIssue(t *testing.T) {
	callCount := 0
	be := &mockBackend{
		chatCompletionFunc: func(_ context.Context, _ []backend.Message, _ backend.CompletionOptions) (*backend.CompletionResult, error) {
			callCount++
			switch callCount {
			case 1:
				return &backend.CompletionResult{
					Content:      `{"people":[],"dates":[],"projects":[],"organisations":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			case 2:
				return &backend.CompletionResult{
					Content:      `{"action_items":[],"decisions":[],"risks":[]}`,
					Model:        "test-model",
					InputTokens:  10,
					OutputTokens: 5,
				}, nil
			default:
				t.Errorf("unexpected call %d — quality gate should not run for non-RISK_ISSUE", callCount)
				return &backend.CompletionResult{Content: `{}`}, nil
			}
		},
	}

	srv := NewAIServer(testConfig(), logging.NewNopLogger(), be)

	triageFYI := "FYI"
	tenantID3 := "tenant-test"
	req := &aiv1.ExtractEntitiesRequest{
		Content:        "EMAIL METADATA:\nFrom: alice@corp.com\nTo: bob@corp.com\nSubject: Immediate Action Required\n---\nBODY:\nPlease review the attached document.",
		TriageCategory: &triageFYI, // Not RISK_ISSUE — gate should not fire
		TenantId:       &tenantID3,
	}

	resp, err := srv.ExtractEntities(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractEntities returned unexpected error: %v", err)
	}

	if resp.QualityGateTriggered {
		t.Errorf("quality_gate_triggered=true for FYI triage category; gate should only run for RISK_ISSUE")
	}

	// Only NER + semantic calls should have been made (not quality gate).
	if callCount != 2 {
		t.Errorf("expected 2 ChatCompletion calls (NER + semantic), got %d", callCount)
	}
}
