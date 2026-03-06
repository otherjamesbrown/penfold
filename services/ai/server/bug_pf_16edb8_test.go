// Package server contains reproduction tests for bug pf-16edb8.
//
// Bug: Two compounding issues with triage of forwarded emails.
//
// Issue 1 — Truncation eats forwarding headers before substantive content.
//
// Forwarded emails have a standard structure:
//
//	---------- Forwarded message ---------
//	From: Original Sender <sender@example.com>
//	Date: Mon, 2 Jan 2025 10:00:00 -0500
//	Subject: Urgent: $8M CIS revenue risk
//	To: recipient@example.com
//
//	<substantive content starts here — often at char 400+>
//
// The current 500-char truncation in buildTriagePrompt (triage.go lines 71–75)
// cuts the content at 500 bytes. With ~400 chars of forwarding headers, only
// ~100 chars of substantive content survive. For longer headers the body may
// be entirely absent from what the LLM sees.
//
// Issue 2 — Triage prompt has no instruction to evaluate forwarded content.
//
// The triagePromptTemplate (triage.go lines 39–62) contains no mention of
// forwarded emails. The LLM therefore has no guidance to look past the
// forwarding headers for the original message body.
package server

import (
	"context"
	"strings"
	"testing"
)

// buildForwardedEmailContent returns a realistic forwarded email content string
// where the first ~400 chars are forwarding headers and the substantive
// financial risk content begins after char 400.
//
// This mirrors what arrives in triage.go after Gmail API body decoding.
func buildForwardedEmailContent() string {
	// Forwarding headers: build up to ~400 chars so substantive content starts
	// well past the 500-char truncation boundary.
	headers := "---------- Forwarded message ---------\n" +
		"From: Alice Lim <alice.lim@bigcorp.com>\n" +
		"Date: Mon, 3 Mar 2025 09:14:22 -0500 (EST)\n" +
		"Subject: FW: Q1 Revenue Risk -- CIS Portfolio Review\n" +
		"To: Bob Chen <bob.chen@bigcorp.com>, Carol Reed <carol.reed@bigcorp.com>\n" +
		"Cc: David Park <david.park@bigcorp.com>, Eve Torres <eve.torres@bigcorp.com>\n" +
		"\n"

	// Pad headers to exactly 420 chars, ensuring the body starts at ~421 which
	// is within the 500-char window, but the key phrase appears further in the
	// body (beyond char 500). We add a realistic preamble line to push the
	// phrase past the 500-char boundary.
	padLen := 420 - len(headers)
	if padLen > 0 {
		headers += strings.Repeat(" ", padLen) + "\n"
	}

	// Add a preamble line that consumes the remaining chars up to ~500, so the
	// "$8M CIS revenue risk" phrase lands beyond char 500.
	headers += "Please review the forwarded message below and respond before EOD.\n"

	// Substantive body — this is what the LLM needs to classify correctly.
	body := "URGENT: The Q1 CIS portfolio is tracking a $8M CIS revenue risk due to delayed contract renewals. " +
		"Three enterprise accounts (Acme, Globex, Initech) are at risk of churn. " +
		"Immediate escalation to VP Sales and Legal required before end of quarter."

	return headers + body
}

// TestTriageTruncation_ForwardedEmail reproduces bug pf-16edb8 Issue 1.
//
// Verifies that after the 500-char truncation in buildTriagePrompt, the substantive
// financial risk content ("$8M CIS revenue risk") is present in the user prompt
// sent to the LLM.
//
// Currently FAILS because the current 500-char limit cuts off all content after
// the forwarding headers, so the substantive phrase never reaches the model.
//
// Regression: once fixed, the phrase must be present and this test must PASS.
func TestTriageTruncation_ForwardedEmail(t *testing.T) {
	content := buildForwardedEmailContent()

	// Precondition: verify the test data is set up correctly.
	// The substantive phrase must exist in the full content string.
	if !strings.Contains(content, "$8M CIS revenue risk") {
		t.Fatal("test setup error: substantive phrase not found in full content")
	}

	// Precondition: the substantive phrase must start after char 500, i.e. within
	// the region that the current truncation discards.
	phrasePos := strings.Index(content, "$8M CIS revenue risk")
	if phrasePos < 500 {
		t.Fatalf("test setup error: substantive phrase starts at char %d, expected > 500 for bug to reproduce", phrasePos)
	}

	// Build the triage prompt using the real buildTriagePrompt function.
	// Use an AIServer with no promptStore so it falls back to the hardcoded template.
	s := &AIServer{}
	_, userPrompt, _, _ := s.buildTriagePrompt(
		context.Background(),
		"FW: Q1 Revenue Risk -- CIS Portfolio Review",
		"alice.lim@bigcorp.com",
		content,
		0,
	)

	// ASSERTION (currently FAILS due to 500-char truncation):
	// The substantive financial risk content must survive the truncation so the
	// LLM can classify this email as RISK_ISSUE/HIGH rather than FYI/LOW.
	if !strings.Contains(userPrompt, "$8M CIS revenue risk") {
		t.Errorf(
			"BUG pf-16edb8 (truncation): user prompt does NOT contain substantive content "+
				"\"$8M CIS revenue risk\" — the 500-char truncation cut the content at char 500 "+
				"but the substantive body begins at char %d.\n"+
				"Fix: raise the truncation limit (e.g. to 2000 chars) or strip forwarding headers "+
				"before truncating.\n"+
				"User prompt (first 600 chars):\n%.600s",
			phrasePos, userPrompt,
		)
	}
}

// TestTriageTruncation_ForwardedEmail_TruncationIsAt1500 is a diagnostic assertion
// that confirms the truncation boundary is exactly 1500 chars after the fix.
//
// Updated from the original 500-char assertion (which documented the buggy behaviour).
// The limit was raised to 1500 to allow forwarded email bodies to survive truncation.
func TestTriageTruncation_ForwardedEmail_TruncationIsAt1500(t *testing.T) {
	longContent := strings.Repeat("x", 1600)

	s := &AIServer{}
	_, userPrompt, _, _ := s.buildTriagePrompt(context.Background(), "", "", longContent, 0)

	if !strings.Contains(userPrompt, strings.Repeat("x", 1500)) {
		t.Errorf("expected 1500 x's in user prompt after truncation")
	}
	if strings.Contains(userPrompt, strings.Repeat("x", 1501)) {
		t.Errorf("truncation is NOT at 1500 chars — more than 1500 x's found in user prompt")
	}
}

// TestTriagePrompt_MissingForwardedEmailInstruction reproduces bug pf-16edb8 Issue 2.
//
// Verifies that the triage system prompt instructs the LLM to evaluate forwarded
// email content (looking past "---------- Forwarded message ---------" headers).
//
// Currently FAILS because triagePromptTemplate contains no reference to
// forwarding or forwarded content.
//
// Regression: once fixed, the template must contain forwarding guidance and this
// test must PASS.
func TestTriagePrompt_MissingForwardedEmailInstruction(t *testing.T) {
	// ASSERTION (currently FAILS):
	// The system prompt must contain guidance about forwarded emails so the LLM
	// knows to look past the forwarding headers for the original message body.
	forwardKeywords := []string{"forward", "Forward", "FORWARD", "forwarded", "Forwarded"}
	found := false
	for _, kw := range forwardKeywords {
		if strings.Contains(triagePromptTemplate, kw) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf(
			"BUG pf-16edb8 (prompt): triagePromptTemplate contains no instruction about forwarded "+
				"emails. The LLM receives forwarded content but has no guidance to look past the "+
				"forwarding headers (\"---------- Forwarded message ---------\") for the original "+
				"body.\n"+
				"Fix: add a note to the system prompt instructing the model to evaluate the content "+
				"of forwarded messages, not just the forwarding metadata.\n"+
				"Current prompt (first 200 chars):\n%.200s",
			triagePromptTemplate,
		)
	}
}
