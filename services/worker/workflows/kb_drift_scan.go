package workflows

import (
	"fmt"
	"sort"

	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// KBDriftScanWorkflow re-verifies all KB article facts against the current system state,
// catching out-of-band changes that bypassed CoBuild (Layer 3 of KB maintenance defence).
//
// Each nightly run:
//  1. Lists all KB articles.
//  2. Processes them in batches of 10; runs Layer 1 factcheck on each.
//  3. Logs a gap entry for any article whose claims have drifted since write-time.
//  4. On a rotating basis, picks 5 articles and runs a Layer 2 semantic judge to
//     catch slow interpretation drift that Layer 1 won't see.
//  5. Writes a brief drift-scan summary shard with pass/fail counts.
func KBDriftScanWorkflow(ctx workflow.Context, input pkgtemporal.KBDriftScanInput) (*pkgtemporal.KBDriftScanResult, error) {
	logger := workflow.GetLogger(ctx)

	runDate := workflow.Now(ctx).Format("2006-01-02")

	logger.Info("Starting KB drift scan",
		"tenant_id", input.TenantID,
		"date", runDate,
	)

	fastCtx := workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())
	llmCtx := workflow.WithActivityOptions(ctx, pkgtemporal.LLMActivityOptions())

	// 1. List all KB articles.
	var listOut pkgtemporal.KBListArticlesOutput
	listIn := pkgtemporal.KBListArticlesInput{TenantID: input.TenantID}
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBListArticles, listIn).Get(ctx, &listOut); err != nil {
		return nil, fmt.Errorf("list KB articles: %w", err)
	}

	articles := listOut.Articles
	if len(articles) == 0 {
		logger.Info("No KB articles found, drift scan complete")
		return &pkgtemporal.KBDriftScanResult{Status: "completed"}, nil
	}

	// 2. Determine the 5 articles for tonight's Layer 2 rolling check.
	// Use day-of-year offset to rotate through the full corpus over time.
	layer2IDs := rollingLayer2Selection(articles, workflow.Now(ctx).YearDay(), 5)
	layer2Set := make(map[string]bool, len(layer2IDs))
	for _, id := range layer2IDs {
		layer2Set[id] = true
	}

	// 3. Process articles in batches of 10 (Layer 1 factcheck).
	const batchSize = 10
	result := &pkgtemporal.KBDriftScanResult{Status: "completed"}

	for i := 0; i < len(articles); i += batchSize {
		end := i + batchSize
		if end > len(articles) {
			end = len(articles)
		}
		batch := articles[i:end]

		for _, article := range batch {
			// Fetch current article content.
			var fetchOut pkgtemporal.KBFetchArticleOutput
			fetchIn := pkgtemporal.KBFetchArticleInput{
				TenantID:  input.TenantID,
				ArticleID: article.ID,
			}
			if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBFetchArticle, fetchIn).Get(ctx, &fetchOut); err != nil {
				logger.Warn("Failed to fetch KB article, skipping", "article_id", article.ID, "error", err)
				continue
			}

			result.ArticlesChecked++

			// Layer 1: deterministic factcheck.
			var factOut pkgtemporal.KBFactCheckOutput
			factIn := pkgtemporal.KBFactCheckInput{
				TenantID:  input.TenantID,
				ArticleID: article.ID,
				Content:   fetchOut.Content,
			}
			if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBFactCheck, factIn).Get(ctx, &factOut); err != nil {
				logger.Warn("Factcheck activity failed, skipping", "article_id", article.ID, "error", err)
				continue
			}

			if !factOut.Passed {
				result.ArticlesFailed++
				// Log each broken claim as a drift gap.
				for _, claim := range factOut.BrokenClaims {
					gapIn := pkgtemporal.KBAppendGapInput{
						TenantID:    input.TenantID,
						ArticleID:   article.ID,
						Category:    "drift-detected",
						Description: claim,
						Date:        runDate,
					}
					if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBAppendGap, gapIn).Get(ctx, nil); err != nil {
						logger.Warn("Failed to log drift gap", "article_id", article.ID, "error", err)
					} else {
						result.GapsLogged++
					}
				}
			} else {
				result.ArticlesPassed++
			}

			// Layer 2: semantic judge on the rotating 5-article selection.
			if layer2Set[article.ID] {
				result.Layer2Checked++
				var judgeOut pkgtemporal.KBLayer2JudgeOutput
				judgeIn := pkgtemporal.KBLayer2JudgeInput{
					TenantID:    input.TenantID,
					ArticleID:   article.ID,
					Content:     fetchOut.Content,
					PrevContent: fetchOut.PrevContent,
				}
				if err := workflow.ExecuteActivity(llmCtx, pkgtemporal.ActivityKBLayer2Judge, judgeIn).Get(ctx, &judgeOut); err != nil {
					logger.Warn("Layer 2 judge failed, skipping", "article_id", article.ID, "error", err)
				} else if judgeOut.Verdict != "consistent" {
					for _, issue := range judgeOut.Issues {
						gapIn := pkgtemporal.KBAppendGapInput{
							TenantID:    input.TenantID,
							ArticleID:   article.ID,
							Category:    "layer2-drift",
							Description: fmt.Sprintf("verdict=%s | %s", judgeOut.Verdict, issue),
							Date:        runDate,
						}
						if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBAppendGap, gapIn).Get(ctx, nil); err != nil {
							logger.Warn("Failed to log Layer 2 gap", "article_id", article.ID, "error", err)
						} else {
							result.GapsLogged++
						}
					}
				}
			}
		}
	}

	// 4. Write drift scan summary shard.
	summaryIn := pkgtemporal.KBWriteSummaryInput{
		TenantID:        input.TenantID,
		Date:            runDate,
		ArticlesChecked: result.ArticlesChecked,
		ArticlesPassed:  result.ArticlesPassed,
		ArticlesFailed:  result.ArticlesFailed,
		Layer2Checked:   result.Layer2Checked,
		GapsLogged:      result.GapsLogged,
	}
	if err := workflow.ExecuteActivity(fastCtx, pkgtemporal.ActivityKBWriteSummary, summaryIn).Get(ctx, nil); err != nil {
		// Non-fatal: log but don't fail the workflow.
		logger.Warn("Failed to write drift scan summary", "error", err)
	}

	logger.Info("KB drift scan complete",
		"articles_checked", result.ArticlesChecked,
		"articles_failed", result.ArticlesFailed,
		"gaps_logged", result.GapsLogged,
		"layer2_checked", result.Layer2Checked,
	)

	return result, nil
}

// rollingLayer2Selection picks up to n articles for tonight's Layer 2 semantic check.
// Articles are sorted by ID for determinism, then offset by dayOfYear so the selection
// rotates through the full corpus over time.
func rollingLayer2Selection(articles []pkgtemporal.KBArticle, dayOfYear, n int) []string {
	if len(articles) == 0 || n <= 0 {
		return nil
	}

	sorted := make([]pkgtemporal.KBArticle, len(articles))
	copy(sorted, articles)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	offset := dayOfYear % len(sorted)
	selected := make([]string, 0, n)
	for i := 0; i < n && i < len(sorted); i++ {
		idx := (offset + i) % len(sorted)
		selected = append(selected, sorted[idx].ID)
	}
	return selected
}
