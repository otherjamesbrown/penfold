// Package activities contains a reproduction test for bug pf-90b749.
//
// Bug: extract_assertions receives ONLY the reply body text — no thread context,
// no parent email (QuotedContent), and no glossary/topic BackgroundContext.
// When an email says "I escalated #2 to Juniper yesterday", the model has no way
// to resolve what "#2" refers to.
//
// Three missing inputs:
// 1. ExtractAssertionsInput has no BackgroundContext field (unlike SLMPipelineExtractEntitiesInput)
// 2. ExtractAssertionsInput has no QuotedContent field (parent email body)
// 3. The activity does not include either in the AssertionRequest sent to AI
//
// This test file MUST FAIL on the current code because:
//   - ExtractAssertionsInput has no BackgroundContext field
//   - ExtractAssertionsInput has no QuotedContent field
//   - The ExtractAssertions activity does not pass context to AssertionRequest
package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestExtractAssertionsInput_HasBackgroundContextField reproduces pf-90b749 (part 1).
//
// ExtractAssertionsInput should have a BackgroundContext field (like
// SLMPipelineExtractEntitiesInput does) so that glossary terms and topic
// descriptions can be injected into the assertion extraction prompt.
//
// This test FAILS because ExtractAssertionsInput.BackgroundContext does not exist.
func TestExtractAssertionsInput_HasBackgroundContextField(t *testing.T) {
	// This compile-time check will fail if BackgroundContext is not on the struct.
	// The test documents the EXPECTED shape of ExtractAssertionsInput after the fix.
	input := workflows.ExtractAssertionsInput{
		TenantID:          "tenant-abc",
		SourceID:          123,
		JobID:             "job-1",
		Content:           "I escalated #2 to Juniper yesterday.",
		Subject:           "Re: GPU requirements",
		ContentType:       "email",
		SenderEmail:       "alice@example.com",
		BackgroundContext: "Glossary: Juniper = Network vendor. Topic: GPU procurement.", // field must exist
	}
	// If BackgroundContext exists on the struct we can round-trip it.
	require.Equal(t, "Glossary: Juniper = Network vendor. Topic: GPU procurement.", input.BackgroundContext)
}

// TestExtractAssertionsInput_HasQuotedContentField reproduces pf-90b749 (part 2).
//
// ExtractAssertionsInput should carry QuotedContent — the parent email body that
// was stripped by ParseEmail. Without it, self-referential phrases like
// "per my email below" or "I escalated #2" cannot be resolved.
//
// This test FAILS because ExtractAssertionsInput.QuotedContent does not exist.
func TestExtractAssertionsInput_HasQuotedContentField(t *testing.T) {
	input := workflows.ExtractAssertionsInput{
		TenantID:      "tenant-abc",
		SourceID:      123,
		JobID:         "job-1",
		Content:       "I escalated #2 to Juniper yesterday.",
		Subject:       "Re: GPU requirements",
		ContentType:   "email",
		SenderEmail:   "alice@example.com",
		QuotedContent: "On Jan 6 Bob wrote:\n> We need 8× H100s. Option #1 costs $4M, option #2 $2M.", // field must exist
	}
	require.Equal(t, "On Jan 6 Bob wrote:\n> We need 8× H100s. Option #1 costs $4M, option #2 $2M.", input.QuotedContent)
}

// TestExtractAssertions_BackgroundContextPassedToAI reproduces pf-90b749 (part 3a).
//
// When BackgroundContext is set on ExtractAssertionsInput, the ExtractAssertions
// activity must include it in the content delivered to the AI so that the LLM can
// ground assertions in tenant-specific glossary terms and topics.
//
// This test FAILS because:
//   - ExtractAssertionsInput has no BackgroundContext field (compile error)
//   - Even if the field existed, the activity does not pass it to AssertionRequest
func TestExtractAssertions_BackgroundContextPassedToAI(t *testing.T) {
	logger := logging.NewNopLogger()

	var capturedContent string
	mockClient := &mockAIClient{
		extractAssertionsFn: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
			capturedContent = req.Content
			return &aiv1.AssertionResponse{
				Assertions: []*aiv1.Assertion{},
				ModelUsed:  "test-model",
			}, nil
		},
	}

	acts := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	backgroundCtx := "Background Context:\nGlossary: Juniper = our primary network hardware vendor.\nTopic: GPU procurement — ongoing evaluation of H100 options."

	// BackgroundContext field does not exist yet — this will fail to compile.
	input := workflows.ExtractAssertionsInput{
		TenantID:          "tenant-abc",
		SourceID:          456,
		JobID:             "job-bg-001",
		Content:           "I escalated #2 to Juniper yesterday.",
		Subject:           "Re: GPU requirements",
		ContentType:       "email",
		SenderEmail:       "alice@example.com",
		BackgroundContext: backgroundCtx,
	}

	_, err := acts.ExtractAssertions(context.Background(), input)
	require.NoError(t, err)

	// The AI must receive the background context so it can resolve "Juniper" and "#2".
	require.Contains(t, capturedContent, "Background Context",
		"AssertionRequest.Content must include the BackgroundContext section; got: %q", capturedContent)
	require.Contains(t, capturedContent, "Juniper = our primary network hardware vendor",
		"BackgroundContext glossary entry must be present in AI content")
}

// TestExtractAssertions_QuotedContentPassedToAI reproduces pf-90b749 (part 3b).
//
// When QuotedContent is set on ExtractAssertionsInput, the ExtractAssertions
// activity must include the parent email in the content delivered to the AI so
// the LLM can resolve forward-references like "#2" or "the option below".
//
// This test FAILS because:
//   - ExtractAssertionsInput has no QuotedContent field (compile error)
//   - Even if the field existed, the activity does not append it to AssertionRequest content
func TestExtractAssertions_QuotedContentPassedToAI(t *testing.T) {
	logger := logging.NewNopLogger()

	var capturedContent string
	mockClient := &mockAIClient{
		extractAssertionsFn: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
			capturedContent = req.Content
			return &aiv1.AssertionResponse{
				Assertions: []*aiv1.Assertion{},
				ModelUsed:  "test-model",
			}, nil
		},
	}

	acts := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	quotedParent := "On Jan 6 Bob wrote:\n> We need 8× H100s. Option #1 costs $4M, option #2 costs $2M."

	// QuotedContent field does not exist yet — this will fail to compile.
	input := workflows.ExtractAssertionsInput{
		TenantID:      "tenant-abc",
		SourceID:      789,
		JobID:         "job-qc-001",
		Content:       "I escalated #2 to Juniper yesterday.",
		Subject:       "Re: GPU requirements",
		ContentType:   "email",
		SenderEmail:   "alice@example.com",
		QuotedContent: quotedParent,
	}

	_, err := acts.ExtractAssertions(context.Background(), input)
	require.NoError(t, err)

	// The AI must receive the quoted parent so "#2" resolves to "option #2 costs $2M".
	require.Contains(t, capturedContent, "Option #1 costs $4M",
		"AssertionRequest.Content must include the parent email body; got: %q", capturedContent)
	require.Contains(t, capturedContent, "option #2 costs $2M",
		"QuotedContent must be present in AI content so forward-references resolve")

	// New reply body must also be present.
	require.Contains(t, capturedContent, "I escalated #2 to Juniper",
		"Primary content (new reply body) must still be present")
}

// TestExtractAssertions_OrderingWithContext reproduces pf-90b749 ordering requirement.
//
// When both BackgroundContext and QuotedContent are present, the AI content must
// be structured as:
//   [BackgroundContext]
//   [Subject line]
//   [New email body]
//   [Quoted / parent email]
//
// This ordering lets the LLM use context to ground assertions in the reply body
// before seeing the parent thread.
//
// This test FAILS because both fields are missing from ExtractAssertionsInput.
func TestExtractAssertions_OrderingWithContext(t *testing.T) {
	logger := logging.NewNopLogger()

	var capturedContent string
	mockClient := &mockAIClient{
		extractAssertionsFn: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
			capturedContent = req.Content
			return &aiv1.AssertionResponse{
				Assertions: []*aiv1.Assertion{},
				ModelUsed:  "test-model",
			}, nil
		},
	}

	acts := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	// Both fields missing — compile failure is the first failure mode.
	input := workflows.ExtractAssertionsInput{
		TenantID:          "tenant-abc",
		SourceID:          999,
		JobID:             "job-order-001",
		Content:           "I escalated #2 to Juniper yesterday.",
		Subject:           "Re: GPU requirements",
		ContentType:       "email",
		SenderEmail:       "alice@example.com",
		BackgroundContext: "Background Context:\nGlossary: Juniper = network vendor.",
		QuotedContent:     "On Jan 6 Bob wrote:\n> Option #2 costs $2M.",
	}

	_, err := acts.ExtractAssertions(context.Background(), input)
	require.NoError(t, err)

	// Verify all three sections are present.
	require.Contains(t, capturedContent, "Background Context", "background section must be present")
	require.Contains(t, capturedContent, "Subject: Re: GPU requirements", "subject must be present")
	require.Contains(t, capturedContent, "I escalated #2 to Juniper", "new email body must be present")
	require.Contains(t, capturedContent, "Option #2 costs $2M", "quoted parent must be present")

	// BackgroundContext must precede the reply body.
	bgIdx := strings.Index(capturedContent, "Background Context")
	bodyIdx := strings.Index(capturedContent, "I escalated #2")
	require.Less(t, bgIdx, bodyIdx,
		"BackgroundContext must appear before the reply body so the LLM reads it first")

	// Subject must precede the reply body (already enforced by pf-e219c1, preserved here).
	subjectIdx := strings.Index(capturedContent, "Subject: Re: GPU requirements")
	require.Less(t, subjectIdx, bodyIdx,
		"Subject must appear before the reply body")

	// Reply body must precede the quoted parent (new content before thread history).
	quotedIdx := strings.Index(capturedContent, "Option #2 costs $2M")
	require.Less(t, bodyIdx, quotedIdx,
		"New reply body must appear before the quoted parent email")
}
