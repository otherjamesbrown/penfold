package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
)

// PersistRepo implements PersistRepository for Stage 4.5 persistence operations.
type PersistRepo struct {
	pool         *pgxpool.Pool
	logger       logging.Logger
	reviewQueue  *reviewqueue.Repository
	glossaryRepo *glossary.Repository
}

// NewPersistRepo creates a new persist repository.
func NewPersistRepo(pool *pgxpool.Pool, logger logging.Logger, opts ...PersistRepoOption) *PersistRepo {
	r := &PersistRepo{
		pool:   pool,
		logger: logger.With(logging.F("component", "persist_repo")),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// PersistRepoOption configures a PersistRepo.
type PersistRepoOption func(*PersistRepo)

// WithReviewQueue sets the review queue repository for acronym detection.
func WithReviewQueue(rq *reviewqueue.Repository) PersistRepoOption {
	return func(r *PersistRepo) { r.reviewQueue = rq }
}

// WithGlossary sets the glossary repository for acronym dedup.
func WithGlossary(g *glossary.Repository) PersistRepoOption {
	return func(r *PersistRepo) { r.glossaryRepo = g }
}

// Validation constants
var (
	validLifecycleEvents = map[string]bool{
		"new":           true,
		"identified":    true,
		"confirmed":     true,
		"raised":        true,
		"updated":       true,
		"escalated":     true,
		"de_escalated":  true,
		"assigned":      true,
		"decided":       true,
		"deferred":      true,
		"resolved":      true,
		"reopened":      true,
	}

	validReferenceTypes = map[string]bool{
		"origination": true,
		"escalation":  true,
		"decision":    true,
		"discussion":  true,
		"resolution":  true,
		"mention":     true,
	}

	validSignificanceLevels = map[string]bool{
		"primary":   true,
		"secondary": true,
		"passing":   true,
	}

	validAssertionTypes = map[string]bool{
		"risk":       true,
		"issue":      true,
		"decision":   true,
		"action":     true,
		"commitment": true,
	}
)

// PersistFindings validates and writes all findings from Stage 4 output in a single transaction.
func (r *PersistRepo) PersistFindings(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
	if input == nil || input.Analysis == nil {
		return nil, fmt.Errorf("input and analysis are required")
	}

	// Validate all inputs before starting transaction
	if err := r.validateInput(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Start transaction
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	output := &PersistFindingsOutput{
		CreatedAssertionIDs: []int64{},
		CreatedReferenceIDs: []int64{},
		CreatedReviewIDs:    []int64{},
	}

	// Verify entity IDs exist
	if err := r.verifyEntityIDs(ctx, tx, input); err != nil {
		return nil, fmt.Errorf("entity verification failed: %w", err)
	}

	// Clear existing assertions for this source before inserting new ones.
	// This ensures reprocessing always reflects the latest extraction output.
	// ON DELETE CASCADE on assertion_references handles reference cleanup.
	// Self-referencing FKs (assertion_root_id, superseded_by) use ON DELETE SET NULL.
	deleteTag, err := tx.Exec(ctx, `DELETE FROM assertions WHERE source_id = $1`, input.SourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear existing assertions for source %d: %w", input.SourceID, err)
	}
	if n := deleteTag.RowsAffected(); n > 0 {
		r.logger.Info("Cleared stale assertions before reprocess",
			logging.F("source_id", input.SourceID),
			logging.F("deleted_count", n),
		)
	}

	// Resolve thread context for cross-thread dedup.
	// If the pipeline didn't pass a thread_id, look it up from thread_messages.
	tc := r.resolveThreadContext(ctx, tx, input.SourceID)
	if tc != nil && input.ThreadID == nil {
		input.ThreadID = &tc.threadID
	}

	// Process verified actions
	for _, action := range input.Analysis.VerifiedActions {
		if err := r.persistAction(ctx, tx, input, tc, &action, output); err != nil {
			return nil, fmt.Errorf("failed to persist action: %w", err)
		}
	}

	// Process verified decisions
	for _, decision := range input.Analysis.VerifiedDecisions {
		if err := r.persistDecision(ctx, tx, input, tc, &decision, output); err != nil {
			return nil, fmt.Errorf("failed to persist decision: %w", err)
		}
	}

	// Process risk references
	for _, risk := range input.Analysis.RiskReferences {
		if err := r.persistRisk(ctx, tx, input, tc, &risk, output); err != nil {
			return nil, fmt.Errorf("failed to persist risk: %w", err)
		}
	}

	// Process implicit actions
	for _, action := range input.Analysis.ImplicitActions {
		if err := r.persistImplicitAction(ctx, tx, input, tc, &action, output); err != nil {
			return nil, fmt.Errorf("failed to persist implicit action: %w", err)
		}
	}

	// Update entity project affinity
	if err := r.updateAffinity(ctx, tx, input, output); err != nil {
		return nil, fmt.Errorf("failed to update affinity: %w", err)
	}

	if output.AssertionsDeduplicated > 0 {
		r.logger.Info("Cross-thread dedup skipped duplicate assertions from quoted text",
			logging.F("source_id", input.SourceID),
			logging.F("deduplicated_count", output.AssertionsDeduplicated),
		)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Best-effort: detect acronyms and create review queue items (after commit, non-transactional)
	r.detectAndCreateAcronymQuestions(ctx, input, output)

	// Best-effort: create entity review items for auto-created people (after commit, non-transactional)
	r.createEntityReviewItems(ctx, input, output)

	return output, nil
}

// validateInput validates all input data before any DB operations.
func (r *PersistRepo) validateInput(input *PersistFindingsInput) error {
	// Validate verified actions
	for i, action := range input.Analysis.VerifiedActions {
		if action.ContextExcerpt == "" {
			return fmt.Errorf("verified action [%d] missing context_excerpt", i)
		}
	}

	// Validate verified decisions
	for i, decision := range input.Analysis.VerifiedDecisions {
		if decision.ContextExcerpt == "" {
			return fmt.Errorf("verified decision [%d] missing context_excerpt", i)
		}
	}

	// Validate risk references
	for i, risk := range input.Analysis.RiskReferences {
		if risk.ContextExcerpt == "" {
			return fmt.Errorf("risk reference [%d] missing context_excerpt", i)
		}
		if risk.LifecycleChange != nil {
			if !validLifecycleEvents[*risk.LifecycleChange] {
				return fmt.Errorf("risk reference [%d] has invalid lifecycle_event: %s", i, *risk.LifecycleChange)
			}
		}
		if !validSignificanceLevels[risk.Significance] {
			return fmt.Errorf("risk reference [%d] has invalid significance: %s", i, risk.Significance)
		}
	}

	// Validate implicit actions
	for i, action := range input.Analysis.ImplicitActions {
		if action.ContextExcerpt == "" {
			return fmt.Errorf("implicit action [%d] missing context_excerpt", i)
		}
	}

	return nil
}

// verifyEntityIDs verifies that referenced person and project IDs exist in the database.
func (r *PersistRepo) verifyEntityIDs(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput) error {
	// Collect all person IDs to verify
	personIDs := make(map[int64]bool)
	for _, personID := range input.ResolvedPeople {
		personIDs[personID] = true
	}

	// Verify person IDs if any
	if len(personIDs) > 0 {
		ids := make([]int64, 0, len(personIDs))
		for id := range personIDs {
			ids = append(ids, id)
		}

		query := `SELECT COUNT(*) FROM people WHERE id = ANY($1)`
		var count int
		if err := tx.QueryRow(ctx, query, ids).Scan(&count); err != nil {
			return fmt.Errorf("failed to verify person IDs: %w", err)
		}
		if count != len(ids) {
			return fmt.Errorf("not all person IDs exist in database: expected %d, found %d", len(ids), count)
		}
	}

	// Verify project ID if provided
	if input.ProjectID != nil {
		query := `SELECT COUNT(*) FROM projects WHERE id = $1`
		var count int
		if err := tx.QueryRow(ctx, query, *input.ProjectID).Scan(&count); err != nil {
			return fmt.Errorf("failed to verify project ID: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("project ID %d does not exist", *input.ProjectID)
		}
	}

	return nil
}

// computeIdempotencyKey is deprecated - deduplication now uses database unique index
// on (source_id, assertion_type, source_quote) instead of computing hash keys.
// Kept for backward compatibility with existing tests, but not used in production code.
func (r *PersistRepo) computeIdempotencyKey(sourceID int64, assertionType, description string) string {
	data := fmt.Sprintf("%d:%s:%s", sourceID, assertionType, description)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// checkExistingAssertion checks if an assertion with the same source_id, type, and source_quote exists.
// Returns the assertion ID if found, nil otherwise.
// Note: Changed from description to source_quote for correct deduplication (pf-35906f).
func (r *PersistRepo) checkExistingAssertion(ctx context.Context, tx pgx.Tx, sourceID int64, assertionType, sourceQuote string) (*int64, error) {
	query := `
		SELECT id FROM assertions
		WHERE source_id = $1 AND assertion_type = $2 AND source_quote = $3
		LIMIT 1
	`
	var id int64
	err := tx.QueryRow(ctx, query, sourceID, assertionType, sourceQuote).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check existing assertion: %w", err)
	}
	return &id, nil
}

// threadContext holds thread membership data for the current source, used for cross-thread dedup.
type threadContext struct {
	threadID    int64
	messageDate time.Time
}

// resolveThreadContext looks up the email thread membership for a source from the thread_messages table.
// Returns nil if the source is not part of any thread.
func (r *PersistRepo) resolveThreadContext(ctx context.Context, tx pgx.Tx, sourceID int64) *threadContext {
	query := `
		SELECT tm.thread_id, tm.message_date
		FROM thread_messages tm
		WHERE tm.source_id = $1
		LIMIT 1
	`
	var tc threadContext
	err := tx.QueryRow(ctx, query, sourceID).Scan(&tc.threadID, &tc.messageDate)
	if err != nil {
		// Not in a thread, or query failed — cross-thread dedup won't apply
		return nil
	}
	return &tc
}

// isQuotedTextDuplicate checks if the same source_quote already exists as an assertion
// in an earlier message within the same email thread. This catches duplicates from
// quoted reply chains where the LLM extracts the same assertion from quoted text.
//
// Only checks messages with an earlier message_date to ensure the original assertion
// is preserved and only the later quoted copies are deduplicated.
func (r *PersistRepo) isQuotedTextDuplicate(ctx context.Context, tx pgx.Tx, sourceID int64, tc *threadContext, sourceQuote string) bool {
	if tc == nil || sourceQuote == "" {
		return false
	}

	query := `
		SELECT 1 FROM assertions a
		JOIN thread_messages tm ON a.source_id = tm.source_id AND tm.thread_id = $1
		WHERE a.source_quote = $2
		AND a.source_id != $3
		AND tm.message_date < $4
		LIMIT 1
	`
	var dummy int
	err := tx.QueryRow(ctx, query, tc.threadID, sourceQuote, sourceID, tc.messageDate).Scan(&dummy)
	return err == nil // found match = is duplicate from quoted text
}

// persistAction persists a verified action assertion.
func (r *PersistRepo) persistAction(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, tc *threadContext, action *VerifiedActionOutput, output *PersistFindingsOutput) error {
	assertionType := "action"

	// Cross-thread dedup: skip if identical quote exists in an earlier message in same thread.
	// This catches assertions extracted from quoted reply text that duplicate the original.
	if r.isQuotedTextDuplicate(ctx, tx, input.SourceID, tc, action.ContextExcerpt) {
		output.AssertionsDeduplicated++
		return nil
	}

	// Check for existing assertion (using source_quote, not description)
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, action.ContextExcerpt)
	if err != nil {
		return err
	}

	// Resolve assignee person ID
	var assigneePersonID *int64
	if action.Assignee != "" {
		if personID, ok := input.ResolvedPeople[action.Assignee]; ok {
			assigneePersonID = &personID
		}
	}

	// Map lifecycle event
	lifecycleEvent := "raised"
	if action.Status == "refined" {
		lifecycleEvent = "updated"
	}

	var assertionID int64
	if existingID != nil {
		// Update existing assertion
		assertionID = *existingID
		output.AssertionsSuperseded++
	} else {
		// Insert new assertion
		query := `
			INSERT INTO assertions (
				tenant_id, source_id, thread_id, project_id,
				assertion_type, description, source_quote,
				assignee_person_id, status, lifecycle_event,
				assertion_root_id, is_current
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			) RETURNING id
		`
		err := tx.QueryRow(ctx, query,
			input.TenantID, input.SourceID, input.ThreadID, input.ProjectID,
			assertionType, action.Description, action.ContextExcerpt,
			assigneePersonID, "open", lifecycleEvent,
			nil, // assertion_root_id will be updated below
			true,
		).Scan(&assertionID)
		if err != nil {
			return fmt.Errorf("failed to insert action assertion: %w", err)
		}

		// Set assertion_root_id to own ID
		updateQuery := `UPDATE assertions SET assertion_root_id = $1 WHERE id = $1`
		if _, err := tx.Exec(ctx, updateQuery, assertionID); err != nil {
			return fmt.Errorf("failed to set assertion_root_id: %w", err)
		}

		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, assertionID)
	}

	// Create assertion reference
	refID, err := r.createAssertionReference(ctx, tx, assertionID, assertionID, input.SourceID, "origination", "primary", action.ContextExcerpt)
	if err != nil {
		return err
	}
	output.ReferencesCreated++
	output.CreatedReferenceIDs = append(output.CreatedReferenceIDs, refID)

	return nil
}

// persistDecision persists a verified decision assertion.
func (r *PersistRepo) persistDecision(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, tc *threadContext, decision *VerifiedDecisionOutput, output *PersistFindingsOutput) error {
	assertionType := "decision"

	// Cross-thread dedup: skip if identical quote exists in an earlier message in same thread.
	if r.isQuotedTextDuplicate(ctx, tx, input.SourceID, tc, decision.ContextExcerpt) {
		output.AssertionsDeduplicated++
		return nil
	}

	// Check for existing assertion (using source_quote, not description)
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, decision.ContextExcerpt)
	if err != nil {
		return err
	}

	// Map lifecycle event
	lifecycleEvent := "decided"
	if decision.Status == "refined" {
		lifecycleEvent = "updated"
	}

	var assertionID int64
	if existingID != nil {
		// Update existing assertion
		assertionID = *existingID
		output.AssertionsSuperseded++
	} else {
		// Insert new assertion
		query := `
			INSERT INTO assertions (
				tenant_id, source_id, thread_id, project_id,
				assertion_type, description, source_quote,
				lifecycle_event, assertion_root_id, is_current
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
			) RETURNING id
		`
		err := tx.QueryRow(ctx, query,
			input.TenantID, input.SourceID, input.ThreadID, input.ProjectID,
			assertionType, decision.Description, decision.ContextExcerpt,
			lifecycleEvent,
			nil, // assertion_root_id will be updated below
			true,
		).Scan(&assertionID)
		if err != nil {
			return fmt.Errorf("failed to insert decision assertion: %w", err)
		}

		// Set assertion_root_id to own ID
		updateQuery := `UPDATE assertions SET assertion_root_id = $1 WHERE id = $1`
		if _, err := tx.Exec(ctx, updateQuery, assertionID); err != nil {
			return fmt.Errorf("failed to set assertion_root_id: %w", err)
		}

		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, assertionID)
	}

	// Create assertion reference
	refID, err := r.createAssertionReference(ctx, tx, assertionID, assertionID, input.SourceID, "decision", "primary", decision.ContextExcerpt)
	if err != nil {
		return err
	}
	output.ReferencesCreated++
	output.CreatedReferenceIDs = append(output.CreatedReferenceIDs, refID)

	return nil
}

// persistRisk persists a risk or issue assertion.
func (r *PersistRepo) persistRisk(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, tc *threadContext, risk *RiskReferenceOutput, output *PersistFindingsOutput) error {
	assertionType := "risk"
	if !risk.IsNew && risk.RootID != nil {
		assertionType = "issue"
	}

	// Cross-thread dedup: skip if identical quote exists in an earlier message in same thread.
	// Exception: explicit RootID updates (lifecycle changes) should not be deduped.
	if risk.RootID == nil && r.isQuotedTextDuplicate(ctx, tx, input.SourceID, tc, risk.ContextExcerpt) {
		output.AssertionsDeduplicated++
		return nil
	}

	// Check for existing assertion (using source_quote, not description)
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, risk.ContextExcerpt)
	if err != nil {
		return err
	}

	// Resolve owner person ID
	var ownerPersonID *int64
	if risk.OwnerChange != nil {
		if personID, ok := input.ResolvedPeople[*risk.OwnerChange]; ok {
			ownerPersonID = &personID
		}
	}

	// Map lifecycle event
	lifecycleEvent := "raised"
	if risk.LifecycleChange != nil {
		lifecycleEvent = *risk.LifecycleChange
	}

	var assertionID int64
	var rootID int64

	if risk.RootID != nil {
		// This is an update to an existing risk
		rootID = *risk.RootID

		// Supersede the old assertion
		updateQuery := `
			UPDATE assertions
			SET is_current = false, superseded_by = $1
			WHERE assertion_root_id = $2 AND is_current = true
		`
		if _, err := tx.Exec(ctx, updateQuery, 0, rootID); err != nil {
			return fmt.Errorf("failed to supersede old assertion: %w", err)
		}

		// Insert new version
		query := `
			INSERT INTO assertions (
				tenant_id, source_id, thread_id, project_id,
				assertion_type, description, source_quote,
				owner_person_id, severity, lifecycle_event,
				assertion_root_id, is_current
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			) RETURNING id
		`
		err := tx.QueryRow(ctx, query,
			input.TenantID, input.SourceID, input.ThreadID, input.ProjectID,
			assertionType, risk.Description, risk.ContextExcerpt,
			ownerPersonID, risk.SeverityChange, lifecycleEvent,
			rootID, true,
		).Scan(&assertionID)
		if err != nil {
			return fmt.Errorf("failed to insert risk assertion version: %w", err)
		}

		// Update superseded_by
		updateSupersededQuery := `UPDATE assertions SET superseded_by = $1 WHERE assertion_root_id = $2 AND id != $1`
		if _, err := tx.Exec(ctx, updateSupersededQuery, assertionID, rootID); err != nil {
			return fmt.Errorf("failed to update superseded_by: %w", err)
		}

		output.AssertionsSuperseded++
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, assertionID)
	} else if existingID != nil {
		// Idempotent: assertion already exists
		assertionID = *existingID
		rootID = assertionID
	} else {
		// Insert new assertion
		query := `
			INSERT INTO assertions (
				tenant_id, source_id, thread_id, project_id,
				assertion_type, description, source_quote,
				owner_person_id, lifecycle_event,
				assertion_root_id, is_current
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			) RETURNING id
		`
		err := tx.QueryRow(ctx, query,
			input.TenantID, input.SourceID, input.ThreadID, input.ProjectID,
			assertionType, risk.Description, risk.ContextExcerpt,
			ownerPersonID, lifecycleEvent,
			nil, // assertion_root_id will be updated below
			true,
		).Scan(&assertionID)
		if err != nil {
			return fmt.Errorf("failed to insert risk assertion: %w", err)
		}

		// Set assertion_root_id to own ID
		updateQuery := `UPDATE assertions SET assertion_root_id = $1 WHERE id = $1`
		if _, err := tx.Exec(ctx, updateQuery, assertionID); err != nil {
			return fmt.Errorf("failed to set assertion_root_id: %w", err)
		}

		rootID = assertionID
		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, assertionID)
	}

	// Determine reference type
	referenceType := "origination"
	if risk.LifecycleChange != nil {
		switch *risk.LifecycleChange {
		case "escalated":
			referenceType = "escalation"
		case "decided", "deferred":
			referenceType = "decision"
		case "resolved":
			referenceType = "resolution"
		default:
			referenceType = "discussion"
		}
	}

	// Create assertion reference
	refID, err := r.createAssertionReference(ctx, tx, assertionID, rootID, input.SourceID, referenceType, risk.Significance, risk.ContextExcerpt)
	if err != nil {
		return err
	}
	output.ReferencesCreated++
	output.CreatedReferenceIDs = append(output.CreatedReferenceIDs, refID)

	return nil
}

// persistImplicitAction persists an implicit action assertion.
func (r *PersistRepo) persistImplicitAction(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, tc *threadContext, action *ImplicitActionOutput, output *PersistFindingsOutput) error {
	assertionType := "action"

	// Cross-thread dedup: skip if identical quote exists in an earlier message in same thread.
	if r.isQuotedTextDuplicate(ctx, tx, input.SourceID, tc, action.ContextExcerpt) {
		output.AssertionsDeduplicated++
		return nil
	}

	// Check for existing assertion (using source_quote, not description)
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, action.ContextExcerpt)
	if err != nil {
		return err
	}

	lifecycleEvent := "raised"

	var assertionID int64
	if existingID != nil {
		// Update existing assertion
		assertionID = *existingID
		output.AssertionsSuperseded++
	} else {
		// Insert new assertion
		query := `
			INSERT INTO assertions (
				tenant_id, source_id, thread_id, project_id,
				assertion_type, description, source_quote,
				status, lifecycle_event, assertion_root_id, is_current
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			) RETURNING id
		`
		err := tx.QueryRow(ctx, query,
			input.TenantID, input.SourceID, input.ThreadID, input.ProjectID,
			assertionType, action.Description, action.ContextExcerpt,
			"open", lifecycleEvent,
			nil, // assertion_root_id will be updated below
			true,
		).Scan(&assertionID)
		if err != nil {
			return fmt.Errorf("failed to insert implicit action assertion: %w", err)
		}

		// Set assertion_root_id to own ID
		updateQuery := `UPDATE assertions SET assertion_root_id = $1 WHERE id = $1`
		if _, err := tx.Exec(ctx, updateQuery, assertionID); err != nil {
			return fmt.Errorf("failed to set assertion_root_id: %w", err)
		}

		output.AssertionsCreated++
		output.CreatedAssertionIDs = append(output.CreatedAssertionIDs, assertionID)
	}

	// Create assertion reference
	refID, err := r.createAssertionReference(ctx, tx, assertionID, assertionID, input.SourceID, "origination", "secondary", action.ContextExcerpt)
	if err != nil {
		return err
	}
	output.ReferencesCreated++
	output.CreatedReferenceIDs = append(output.CreatedReferenceIDs, refID)

	return nil
}

// createAssertionReference creates an assertion_references row.
func (r *PersistRepo) createAssertionReference(ctx context.Context, tx pgx.Tx, assertionID, rootID, sourceID int64, refType, significance, contextExcerpt string) (int64, error) {
	query := `
		INSERT INTO assertion_references (
			assertion_id, assertion_root_id, source_id,
			reference_type, significance, context_excerpt, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`
	var id int64
	err := tx.QueryRow(ctx, query, assertionID, rootID, sourceID, refType, significance, contextExcerpt, time.Now()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create assertion reference: %w", err)
	}
	return id, nil
}

// updateAffinity updates entity_project_affinity for people mentioned in the analysis.
func (r *PersistRepo) updateAffinity(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, output *PersistFindingsOutput) error {
	if input.ProjectID == nil {
		return nil
	}

	for name, personID := range input.ResolvedPeople {
		// UPSERT entity_project_affinity
		query := `
			INSERT INTO entity_project_affinity (
				tenant_id, entity_type, entity_id, project_id,
				mention_count, last_mentioned_at, affinity_score
			) VALUES ($1, $2, $3, $4, 1, $5, 0.1)
			ON CONFLICT (tenant_id, entity_type, entity_id, project_id)
			DO UPDATE SET
				mention_count = entity_project_affinity.mention_count + 1,
				last_mentioned_at = $5,
				affinity_score = LEAST(1.0, (entity_project_affinity.mention_count + 1) * 0.1)
		`
		_, err := tx.Exec(ctx, query, input.TenantID, "person", personID, *input.ProjectID, time.Now())
		if err != nil {
			return fmt.Errorf("failed to update affinity for person %s: %w", name, err)
		}
		output.AffinityUpdates++
	}

	return nil
}

// commonAcronymExclusions contains uppercase tokens that look like acronyms but aren't.
var commonAcronymExclusions = map[string]bool{
	"I": true, "A": true, "IT": true, "AM": true, "PM": true,
	"IS": true, "AT": true, "IN": true, "ON": true, "TO": true,
	"OR": true, "AN": true, "AS": true, "IF": true, "BY": true,
	"DO": true, "GO": true, "NO": true, "SO": true, "UP": true,
	"WE": true, "OK": true, "BE": true, "HE": true, "ME": true,
	"MY": true, "OF": true, "US": true, "VS": true, "RE": true,
	"THE": true, "AND": true, "FOR": true, "NOT": true, "BUT": true,
	"ALL": true, "CAN": true, "HER": true, "WAS": true, "ONE": true,
	"OUR": true, "OUT": true, "ARE": true, "HAS": true, "HIS": true,
	"HOW": true, "ITS": true, "MAY": true, "NEW": true, "NOW": true,
	"OLD": true, "SEE": true, "WAY": true, "WHO": true, "DID": true,
	"GET": true, "HIM": true, "LET": true, "SAY": true, "SHE": true,
	"TOO": true, "USE": true, "HAD": true,
	"TBD": true, "TBA": true, "FYI": true, "ASAP": true, "NOTE": true,
	"TODO": true, "DONE": true, "NONE": true, "NULL": true,
	"FW": true, "CC": true, "BCC": true,
	// Geography/regions
	"USA": true, "CA": true, "NY": true, "UK": true, "EU": true,
	// Time zones
	"PST": true, "GMT": true, "EST": true, "CST": true, "MST": true,
	"UTC": true, "CET": true, "BST": true,
	// Business terms
	"LLC": true, "OOO": true, "HR": true, "PR": true, "QA": true,
	"VP": true, "CEO": true, "CFO": true, "CTO": true, "COO": true,
	// Common tech
	"VM": true, "OS": true, "UI": true, "UX": true, "DB": true,
	"AI": true, "ML": true,
	// Other
	"PDF": true, "FAQ": true, "ETA": true, "AKA": true, "RFP": true,
	"RFI": true, "NDA": true, "EOD": true, "WFH": true,
}

// properNounExclusions contains CamelCase words that are proper nouns, not acronyms.
var properNounExclusions = map[string]bool{
	"TIKTOK": true, "LINKEDIN": true, "GITHUB": true, "YOUTUBE": true,
	"MACBOOK": true, "IPHONE": true, "IPAD": true, "AIRPODS": true,
	"FACEBOOK": true, "INSTAGRAM": true, "TWITTER": true, "SNAPCHAT": true,
	"GOOGLE": true, "MICROSOFT": true, "AMAZON": true, "APPLE": true,
	"NETFLIX": true, "SPOTIFY": true, "AIRBNB": true, "UBER": true,
	"SALESFORCE": true, "SLACK": true, "ZOOM": true, "ASANA": true,
	"JIRA": true, "CONFLUENCE": true, "BITBUCKET": true, "GITLAB": true,
	"POSTGRES": true, "POSTGRESQL": true, "MONGODB": true, "MYSQL": true,
	"REDIS": true, "ELASTICSEARCH": true, "KUBERNETES": true, "DOCKER": true,
}

const (
	// maxAcronymLength is the maximum length for a valid acronym (matching pkg/ingest/meeting/acronyms.go)
	maxAcronymLength = 10
	// minUppercaseCount is the minimum number of uppercase letters required
	minUppercaseCount = 2
)

// acronymPattern matches potential acronyms including:
// - Pure uppercase: API, JWT, SLA, CLIC, TRR
// - Mixed-case: IaaS, SteerCo, PostgreSQL
// - With digits: FY26, S3
// Pattern: 2+ uppercase letters with optional lowercase/digits
var acronymPattern = regexp.MustCompile(`\b([A-Z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*|[A-Z]{2,}[a-z]*)\b`)

// normalizeAcronym normalizes plural acronyms to singular form.
// Strips trailing 's' from acronyms like "SLIs" -> "SLI", "APIs" -> "API".
// This prevents duplicate review items for singular/plural variants.
func normalizeAcronym(term string) string {
	if len(term) < 2 {
		return term
	}

	// Check if ends with lowercase 's' and rest is uppercase (e.g., "SLIs")
	if term[len(term)-1] == 's' {
		rest := term[:len(term)-1]
		// Only strip 's' if the rest is all uppercase (acronym pattern)
		allUpper := true
		for _, r := range rest {
			if r >= 'a' && r <= 'z' {
				allUpper = false
				break
			}
		}
		if allUpper && len(rest) >= 2 {
			return rest
		}
	}

	return term
}

// isValidAcronym checks if a matched token is actually a valid acronym.
// Filters out:
// - Tokens longer than maxAcronymLength
// - Tokens with fewer than minUppercaseCount uppercase letters
// - Proper nouns (via blocklist)
func isValidAcronym(token string) bool {
	// Length cap
	if len(token) > maxAcronymLength {
		return false
	}

	// Count uppercase letters
	upperCount := 0
	for _, r := range token {
		if r >= 'A' && r <= 'Z' {
			upperCount++
		}
	}
	if upperCount < minUppercaseCount {
		return false
	}

	// Check common exclusions (words that aren't acronyms)
	upperToken := strings.ToUpper(token)
	if commonAcronymExclusions[upperToken] {
		return false
	}

	// Check proper noun blocklist (case-insensitive)
	if properNounExclusions[upperToken] {
		return false
	}

	return true
}

// detectAndCreateAcronymQuestions scans analysis text for uppercase acronyms and
// creates review queue items for unknown ones. Best-effort: errors are logged, not returned.
func (r *PersistRepo) detectAndCreateAcronymQuestions(ctx context.Context, input *PersistFindingsInput, output *PersistFindingsOutput) {
	if r.reviewQueue == nil {
		return
	}

	// Collect text from original email content, Stage 4 (Analysis), and Stage 2 (SLM assertions from DB)
	texts := r.collectAnalysisTexts(ctx, input.SourceID, input.Analysis, input.BodyText, input.Subject)
	if len(texts) == 0 {
		return
	}

	// Extract candidate acronyms
	candidates := make(map[string]string) // term -> first context seen
	for _, text := range texts {
		for _, match := range acronymPattern.FindAllString(text, -1) {
			// Validate acronym: check length, uppercase count, common words, proper nouns
			if !isValidAcronym(match) {
				continue
			}
			// Pattern now accepts mixed-case (IaaS, SteerCo) and digits (FY26)
			if _, seen := candidates[match]; !seen {
				// Store a snippet of context around the acronym
				idx := strings.Index(text, match)
				start := idx - 40
				if start < 0 {
					start = 0
				}
				end := idx + len(match) + 40
				if end > len(text) {
					end = len(text)
				}
				candidates[match] = text[start:end]
			}
		}
	}

	if len(candidates) == 0 {
		return
	}

	logger := r.logger.With(
		logging.F("source_id", input.SourceID),
		logging.F("candidate_count", len(candidates)),
	)
	logger.Debug("Detected acronym candidates")

	for term, ctxSnippet := range candidates {
		// Normalize plural forms before dedup check (SLIs -> SLI, APIs -> API)
		normalizedTerm := normalizeAcronym(term)

		// Skip if already in glossary
		if r.glossaryRepo != nil {
			glossaryTerm, err := r.glossaryRepo.GetByTerm(ctx, input.TenantID, normalizedTerm)
			if err != nil {
				logger.Warn("Failed to check glossary for acronym",
					logging.F("term", normalizedTerm), logging.Err(err))
				continue
			}
			if glossaryTerm != nil {
				continue // Already known
			}
		}

		// Create review queue item (CreateIfNotExists handles dedup)
		aq := reviewqueue.AcronymQuestion{
			Term:       normalizedTerm,
			Context:    ctxSnippet,
			SourceType: "source",
			SourceID:   input.SourceID,
			Confidence: 0.7,
		}
		_, created, err := r.reviewQueue.CreateIfNotExists(ctx, aq.ToInput())
		if err != nil {
			logger.Warn("Failed to create acronym review item",
				logging.F("term", term), logging.Err(err))
			continue
		}
		if created {
			output.ReviewItemsCreated++
		}
	}
}

// collectAnalysisTexts gathers text strings from analysis findings for acronym scanning.
// It collects from original email content, Stage 4 (Analysis), and Stage 2 (SLM assertions from DB).
// This ensures acronyms are detected even when Stage 4 times out or is nil.
func (r *PersistRepo) collectAnalysisTexts(ctx context.Context, sourceID int64, analysis *DeepAnalyzeOutput, bodyText, subject string) []string {
	var texts []string

	// FIRST: Collect from original email content (if available)
	// This ensures acronyms in the original email are detected even if LLM doesn't mention them
	if subject != "" {
		texts = append(texts, subject)
	}
	if bodyText != "" {
		texts = append(texts, bodyText)
	}

	// SECOND: Collect from Stage 4 (Analysis) if available
	if analysis != nil {
		for _, a := range analysis.VerifiedActions {
			texts = append(texts, a.Description, a.ContextExcerpt)
		}
		for _, d := range analysis.VerifiedDecisions {
			texts = append(texts, d.Description, d.ContextExcerpt)
		}
		for _, risk := range analysis.RiskReferences {
			texts = append(texts, risk.Description, risk.ContextExcerpt)
		}
		for _, a := range analysis.ImplicitActions {
			texts = append(texts, a.Description, a.ContextExcerpt)
		}
		if analysis.Summary != "" {
			texts = append(texts, analysis.Summary)
		}
		texts = append(texts, analysis.Insights...)
	}

	// THIRD: Collect from Stage 2 assertions (SLM extraction output from DB)
	// This handles the case where Stage 4 times out but Stage 2 completed successfully.
	stage2Texts := r.collectStage2Texts(ctx, sourceID)
	texts = append(texts, stage2Texts...)

	if len(texts) == 0 {
		return nil
	}

	return texts
}

// collectStage2Texts queries the database for Stage 2 SLM assertions and extracts text for acronym scanning.
// This is a fallback when Stage 4 (DeepAnalyze) times out or fails.
func (r *PersistRepo) collectStage2Texts(ctx context.Context, sourceID int64) []string {
	// Skip if pool is not available (e.g., in unit tests without DB)
	if r.pool == nil {
		return nil
	}

	// Query for Stage 2 assertions (those with extraction_model set, indicating SLM output)
	// We look for assertions with extraction_model IS NOT NULL, which indicates Stage 2 SLM output
	// (as opposed to Stage 4 assertions created by PersistFindings which don't set extraction_model).
	query := `
		SELECT description, source_quote
		FROM assertions
		WHERE source_id = $1
		  AND extraction_model IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 100
	`

	rows, err := r.pool.Query(ctx, query, sourceID)
	if err != nil {
		r.logger.Warn("Failed to query Stage 2 assertions for acronym detection",
			logging.F("source_id", sourceID), logging.Err(err))
		return nil
	}
	defer rows.Close()

	var texts []string
	for rows.Next() {
		var description, sourceQuote string
		if err := rows.Scan(&description, &sourceQuote); err != nil {
			r.logger.Warn("Failed to scan Stage 2 assertion",
				logging.F("source_id", sourceID), logging.Err(err))
			continue
		}
		texts = append(texts, description)
		if sourceQuote != "" {
			texts = append(texts, sourceQuote)
		}
	}

	if err := rows.Err(); err != nil {
		r.logger.Warn("Error iterating Stage 2 assertions",
			logging.F("source_id", sourceID), logging.Err(err))
	}

	r.logger.Debug("Collected Stage 2 assertion texts for acronym scanning",
		logging.F("source_id", sourceID),
		logging.F("text_count", len(texts)))

	return texts
}

// createEntityReviewItems creates review queue items for auto-created entities that need review.
// Best-effort: errors are logged, not returned.
func (r *PersistRepo) createEntityReviewItems(ctx context.Context, input *PersistFindingsInput, output *PersistFindingsOutput) {
	if r.reviewQueue == nil {
		return
	}

	// Skip if no resolved people
	if len(input.ResolvedPeople) == 0 {
		return
	}

	// Collect person IDs to query
	personIDs := make([]int64, 0, len(input.ResolvedPeople))
	nameToID := make(map[int64]string) // person_id -> name for logging
	for name, personID := range input.ResolvedPeople {
		personIDs = append(personIDs, personID)
		nameToID[personID] = name
	}

	// Query database for entity metadata (needs_review, confidence, auto_created)
	query := `
		SELECT id, preferred_name, needs_review, confidence, auto_created
		FROM people
		WHERE id = ANY($1)
		  AND needs_review = true
	`
	rows, err := r.pool.Query(ctx, query, personIDs)
	if err != nil {
		r.logger.Warn("Failed to query entities for review",
			logging.F("source_id", input.SourceID), logging.Err(err))
		return
	}
	defer rows.Close()

	logger := r.logger.With(
		logging.F("source_id", input.SourceID),
	)

	createdCount := 0
	for rows.Next() {
		var personID int64
		var preferredName string
		var needsReview bool
		var confidence float32
		var autoCreated bool

		if err := rows.Scan(&personID, &preferredName, &needsReview, &confidence, &autoCreated); err != nil {
			logger.Warn("Failed to scan entity row",
				logging.F("person_id", personID), logging.Err(err))
			continue
		}

		// Create review queue item for this entity
		pq := reviewqueue.PersonQuestion{
			MatchedText:    preferredName,
			Context:        fmt.Sprintf("Auto-created entity (confidence: %.2f)", confidence),
			CandidateIDs:   []int64{personID},
			CandidateNames: []string{preferredName},
			SourceType:     "source",
			SourceID:       input.SourceID,
			Confidence:     float64(confidence),
		}

		_, created, err := r.reviewQueue.CreateIfNotExists(ctx, pq.ToInput())
		if err != nil {
			logger.Warn("Failed to create entity review item",
				logging.F("person_id", personID),
				logging.F("name", preferredName),
				logging.Err(err))
			continue
		}

		if created {
			createdCount++
			output.ReviewItemsCreated++
		}
	}

	if err := rows.Err(); err != nil {
		logger.Warn("Error iterating entity rows",
			logging.Err(err))
	}

	if createdCount > 0 {
		logger.Info("Created entity review items",
			logging.F("count", createdCount))
	}
}
