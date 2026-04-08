//go:build integration

package activities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/topics"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paritySource represents a synthetic input for parity testing.
// Using synthetic inputs (rather than real DB source rows) ensures the test is
// deterministic and not dependent on production data changing over time.
type paritySource struct {
	name        string
	pipeline    string // "standard" or "newsletter"
	stage       string // "extract_ner" or "newsletter_extract"
	subject     string
	content     string
	senderEmail string
	senderName  string
}

// parityTestSources contains 10 sources: 5 standard (extract_ner) and 5 newsletter.
// Content is chosen to exercise acronym scanning and context lookup while avoiding
// false positives from topic keyword matches that would diverge between old/new paths.
var parityTestSources = []paritySource{
	// ── Standard sources (5) ─────────────────────────────────────────────────
	{
		name:        "standard-acronym-heavy",
		pipeline:    "standard",
		stage:       "extract_ner",
		subject:     "NLB configuration update for MTC",
		content:     "We have completed the NLB setup for the MTC environment. The DNS and SSL certificates are in place. Please review the MTC runbook before the launch.",
		senderEmail: "alice@example.com",
		senderName:  "Alice Smith",
	},
	{
		name:        "standard-no-acronyms",
		pipeline:    "standard",
		stage:       "extract_ner",
		subject:     "Weekly status update",
		content:     "The team has made good progress this week. Stakeholder demos are scheduled for Thursday. Please prepare your slides.",
		senderEmail: "bob@example.com",
		senderName:  "Bob Jones",
	},
	{
		name:        "standard-mixed-acronyms",
		pipeline:    "standard",
		stage:       "extract_ner",
		subject:     "SLA review for Q3",
		content:     "The SLA targets for Q3 were met. The KPI dashboard has been updated. Please confirm the OKR alignment with your team.",
		senderEmail: "carol@example.com",
		senderName:  "Carol White",
	},
	{
		name:        "standard-long-content",
		pipeline:    "standard",
		stage:       "extract_ner",
		subject:     "Architecture decision: switching from REST to gRPC",
		content:     "After evaluating our API needs, we have decided to migrate from REST to gRPC. The gRPC framework provides better performance for our use case. The POC has been completed and the migration plan is attached. ETA for the first service is end of Q4.",
		senderEmail: "dave@example.com",
		senderName:  "Dave Brown",
	},
	{
		name:        "standard-minimal",
		pipeline:    "standard",
		stage:       "extract_ner",
		subject:     "Quick update",
		content:     "Just a quick note to confirm the meeting is confirmed for tomorrow at 3pm.",
		senderEmail: "eve@example.com",
		senderName:  "Eve Green",
	},
	// ── Newsletter sources (5) ────────────────────────────────────────────────
	{
		name:        "newsletter-acronym-rich",
		pipeline:    "newsletter",
		stage:       "newsletter_extract",
		subject:     "AI Weekly: LLM advances and RAG systems",
		content:     "This week in AI: LLM benchmarks show continued improvement. RAG pipelines are becoming standard for enterprise use. The NLP community is excited about the new RLHF techniques.",
		senderEmail: "newsletter@aiweekly.com",
		senderName:  "AI Weekly",
	},
	{
		name:        "newsletter-product-news",
		pipeline:    "newsletter",
		stage:       "newsletter_extract",
		subject:     "Product digest: AWS and GCP updates",
		content:     "AWS announced new EC2 instance types optimised for ML workloads. GCP updated their BigQuery pricing model. Azure has released new SDK updates for Python developers.",
		senderEmail: "digest@cloud-news.com",
		senderName:  "Cloud Digest",
	},
	{
		name:        "newsletter-no-acronyms",
		pipeline:    "newsletter",
		stage:       "newsletter_extract",
		subject:     "Industry perspectives: enterprise knowledge management",
		content:     "Enterprise knowledge management is evolving rapidly. Organizations are investing heavily in knowledge graphs and semantic search. The key challenge remains getting employees to document their expertise.",
		senderEmail: "news@industry.com",
		senderName:  "Industry Digest",
	},
	{
		name:        "newsletter-tech-brief",
		pipeline:    "newsletter",
		stage:       "newsletter_extract",
		subject:     "Dev brief: API and SDK roundup",
		content:     "New API releases this week include improvements to the REST and GraphQL ecosystem. SDK updates for Go, Python, and TypeScript are available. The CLI tooling has also been refreshed.",
		senderEmail: "brief@devnews.com",
		senderName:  "Dev Brief",
	},
	{
		name:        "newsletter-minimal",
		pipeline:    "newsletter",
		stage:       "newsletter_extract",
		subject:     "Short update from the team",
		content:     "A brief note from the editorial team. We will be on a short break next week and will return with the full edition on Monday.",
		senderEmail: "hello@newsletter.com",
		senderName:  "The Team",
	},
}

// parityResult records the outcome of one source comparison.
type parityResult struct {
	sourceName   string
	oldOutput    string
	newOutput    string
	match        bool
	diffSections []sectionDiff
}

// sectionDiff records a per-section mismatch between old and new outputs.
type sectionDiff struct {
	header  string
	oldBody string
	newBody string
}

// TestContextBuilderParity runs both old and new context builder paths on 10 synthetic
// sources and asserts semantic equivalence (order-independent section comparison).
//
// This test MUST run before Task pf-5ccf06 removes the old BuildContext and
// BuildNewsletterContext functions. It serves as the regression gate for parity.
func TestContextBuilderParity(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDBForBuildStageContext(t)
	defer pool.Close()

	tenantID := testRunTenantID

	// Check migration 149 is applied (context_providers column must exist).
	pipelineRepo := NewPipelineRepository(pool)
	providers, err := pipelineRepo.GetContextProviders(ctx, tenantID, "newsletter", "newsletter_extract")
	require.NoError(t, err, "GetContextProviders must work — migration 149 must be applied")
	if len(providers) == 0 {
		t.Skip("newsletter_extract context_providers not seeded — migration 149 may not have run")
	}

	// The test tenant may not have standard/extract_ner seeded (it's a minimal test tenant).
	// Seed it for the duration of this test and clean up after.
	cleanup := seedStandardPipelineForTenant(t, ctx, pool, tenantID)
	defer cleanup()

	logger := logging.MustGlobal()

	// Build repositories.
	contextRepo := NewContextPackageRepo(pool, logger)
	newsletterRepo := NewPostgresNewsletterContextRepository(pool)
	topicsRepo := topics.NewRepository(pool, logger)
	topicAdapter := NewTopicLookupAdapter(topicsRepo)

	// Register providers with real repos so they produce content from the DB.
	RegisterContextProviders(logger, contextRepo, newsletterRepo, topicAdapter, nil)

	// Build activities with real repos (mock entity resolver — not needed for context assembly parity).
	a := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, contextRepo, topicAdapter, pipelineRepo, nil).
		WithNewsletterContextRepo(newsletterRepo)

	var results []parityResult

	for _, src := range parityTestSources {
		t.Run(src.name, func(t *testing.T) {
			var oldOutput, newOutput string

			switch src.pipeline {
			case "standard":
				// Old path: BuildContext with extract_ner stage.
				input := workflows.BuildContextInput{
					TenantID:    tenantID,
					Subject:     src.subject,
					Content:     src.content,
					SenderEmail: src.senderEmail,
					SenderName:  src.senderName,
				}
				cp, err := a.BuildContext(ctx, src.stage, input, nil, nil)
				require.NoError(t, err, "BuildContext must not error for %s", src.name)
				oldOutput = cp.FormatForPrompt()

			case "newsletter":
				// Old path: BuildNewsletterContext.
				nlInput := workflows.BuildNewsletterContextInput{
					TenantID: tenantID,
					Subject:  src.subject,
					Content:  src.content,
				}
				nlOut, err := a.BuildNewsletterContext(ctx, nlInput)
				require.NoError(t, err, "BuildNewsletterContext must not error for %s", src.name)
				oldOutput = nlOut.BackgroundContext

			default:
				t.Fatalf("unknown pipeline: %s", src.pipeline)
			}

			// New path: BuildStageContext (config-driven, reads context_providers from DB).
			newOutput, err = a.BuildStageContext(ctx, BuildStageContextInput{
				TenantID: tenantID,
				Pipeline: src.pipeline,
				Stage:    src.stage,
				ProviderInput: ContextProviderInput{
					TenantID:    tenantID,
					Subject:     src.subject,
					Content:     src.content,
					SenderEmail: src.senderEmail,
					SenderName:  src.senderName,
				},
			})
			require.NoError(t, err, "BuildStageContext must not error for %s", src.name)

			// Normalize and compare: parse into sections, sort by header, compare.
			oldSections := parseSections(oldOutput)
			newSections := parseSections(newOutput)

			diffs := diffSections(oldSections, newSections)
			match := len(diffs) == 0

			results = append(results, parityResult{
				sourceName:   src.name,
				oldOutput:    oldOutput,
				newOutput:    newOutput,
				match:        match,
				diffSections: diffs,
			})

			if !match {
				t.Logf("MISMATCH for %s:", src.name)
				t.Logf("  Old output (%d chars):\n%s", len(oldOutput), indent(oldOutput))
				t.Logf("  New output (%d chars):\n%s", len(newOutput), indent(newOutput))
				for _, d := range diffs {
					t.Logf("  Section %q differs:", d.header)
					if d.oldBody == "" && d.newBody != "" {
						t.Logf("    OLD: (absent)")
						t.Logf("    NEW: %s", d.newBody)
					} else if d.oldBody != "" && d.newBody == "" {
						t.Logf("    OLD: %s", d.oldBody)
						t.Logf("    NEW: (absent)")
					} else {
						t.Logf("    OLD: %s", d.oldBody)
						t.Logf("    NEW: %s", d.newBody)
					}
				}
			} else {
				t.Logf("MATCH for %s (%d chars)", src.name, len(oldOutput))
			}

			assert.True(t, match, "parity failed for %s: %d section(s) differ — see log for diff", src.name, len(diffs))
		})
	}

	// Print summary table.
	t.Logf("\n=== Parity Validation Summary ===")
	t.Logf("%-40s %-12s %-8s", "Source", "Pipeline", "Result")
	t.Logf("%s", strings.Repeat("-", 64))
	for _, r := range results {
		result := "MATCH"
		if !r.match {
			result = fmt.Sprintf("MISMATCH (%d section(s))", len(r.diffSections))
		}
		t.Logf("%-40s %-12s %-8s", r.sourceName, pipelineOf(r.sourceName), result)
	}

	allMatch := true
	for _, r := range results {
		if !r.match {
			allMatch = false
			break
		}
	}
	assert.True(t, allMatch, "parity validation: one or more sources produced different context between old and new paths")
}

// parseSections splits a BackgroundContext markdown string into a map of
// section header → section body. Sections are identified by "### Header" lines.
// Returns a map for order-independent comparison.
func parseSections(context string) map[string]string {
	sections := make(map[string]string)
	if context == "" {
		return sections
	}

	// Split on double newline (section separator in FormatForPrompt and provider outputs).
	blocks := strings.Split(context, "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		// Extract header (first line starting with ###).
		lines := strings.SplitN(block, "\n", 2)
		if len(lines) == 0 {
			continue
		}
		header := strings.TrimSpace(lines[0])
		if !strings.HasPrefix(header, "### ") {
			// Not a section header — treat as standalone content under empty key.
			// This handles the User Context section which doesn't have ### prefix sometimes.
			// But BuildNewsletterContext and providers both use ### prefix, so this shouldn't occur.
			continue
		}
		body := ""
		if len(lines) > 1 {
			body = strings.TrimSpace(lines[1])
		}
		sections[header] = body
	}
	return sections
}

// diffSections compares two section maps and returns the set of differing sections.
// A diff is reported if: a section appears in one map but not the other,
// or if the same section has different body content (after normalizing whitespace).
func diffSections(old, new map[string]string) []sectionDiff {
	var diffs []sectionDiff

	// Collect all headers from both maps.
	allHeaders := make(map[string]bool)
	for h := range old {
		allHeaders[h] = true
	}
	for h := range new {
		allHeaders[h] = true
	}

	// Sort headers for deterministic diff output.
	headers := make([]string, 0, len(allHeaders))
	for h := range allHeaders {
		headers = append(headers, h)
	}
	sort.Strings(headers)

	for _, h := range headers {
		oldBody, inOld := old[h]
		newBody, inNew := new[h]

		switch {
		case inOld && inNew:
			// Both present: compare body content (normalize whitespace).
			if normalizeWhitespace(oldBody) != normalizeWhitespace(newBody) {
				diffs = append(diffs, sectionDiff{header: h, oldBody: oldBody, newBody: newBody})
			}
		case inOld && !inNew:
			// Only in old: this is a mismatch only if old body is non-empty.
			// (Empty sections are not rendered by either path, so absence == empty.)
			if strings.TrimSpace(oldBody) != "" {
				diffs = append(diffs, sectionDiff{header: h, oldBody: oldBody, newBody: ""})
			}
		case !inOld && inNew:
			// Only in new: mismatch if new body is non-empty.
			if strings.TrimSpace(newBody) != "" {
				diffs = append(diffs, sectionDiff{header: h, oldBody: "", newBody: newBody})
			}
		}
	}
	return diffs
}

// normalizeWhitespace collapses internal whitespace for comparison.
// This prevents false positives from trivial spacing differences between providers.
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}

// indent prepends two spaces to each line for test log readability.
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

// pipelineOf returns "standard" or "newsletter" based on the source name prefix.
func pipelineOf(name string) string {
	if strings.HasPrefix(name, "newsletter-") {
		return "newsletter"
	}
	return "standard"
}

// seedStandardPipelineForTenant inserts standard/extract_ner pipeline_definitions
// for the given tenant so BuildStageContext can resolve its context_providers.
// Returns a cleanup function that removes the inserted rows.
func seedStandardPipelineForTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) func() {
	t.Helper()

	const stage = "extract_ner"
	const pipeline = "standard"
	providers := []string{"glossary", "topics"}

	// Check if row already exists (idempotency: don't delete rows we didn't create).
	var existing []string
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(context_providers, '{}') FROM pipeline_definitions WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3`,
		tenantID, pipeline, stage,
	).Scan(&existing)

	rowExistedBefore := err == nil

	if pgx.ErrNoRows == err {
		// Row doesn't exist — insert it with stage_order=30 (matches extract_ner in other tenants).
		_, insertErr := pool.Exec(ctx,
			`INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, context_providers)
			 VALUES ($1, $2, $3, 30, $4)`,
			tenantID, pipeline, stage, providers,
		)
		require.NoError(t, insertErr, "seed: insert standard/extract_ner pipeline_definition")
	} else if len(existing) == 0 {
		// Row exists but has empty providers — update it.
		_, updateErr := pool.Exec(ctx,
			`UPDATE pipeline_definitions SET context_providers = $1 WHERE tenant_id = $2 AND pipeline = $3 AND stage = $4`,
			providers, tenantID, pipeline, stage,
		)
		require.NoError(t, updateErr, "seed: update standard/extract_ner context_providers")
	}
	// If row exists and already has providers, leave it as-is.

	return func() {
		if !rowExistedBefore {
			// We inserted the row — delete it.
			_, _ = pool.Exec(ctx,
				`DELETE FROM pipeline_definitions WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3`,
				tenantID, pipeline, stage,
			)
		} else if len(existing) == 0 {
			// We updated the row — reset to empty.
			_, _ = pool.Exec(ctx,
				`UPDATE pipeline_definitions SET context_providers = '{}' WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3`,
				tenantID, pipeline, stage,
			)
		}
	}
}
