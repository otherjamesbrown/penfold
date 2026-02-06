package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

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

	// Process verified actions
	for _, action := range input.Analysis.VerifiedActions {
		if err := r.persistAction(ctx, tx, input, &action, output); err != nil {
			return nil, fmt.Errorf("failed to persist action: %w", err)
		}
	}

	// Process verified decisions
	for _, decision := range input.Analysis.VerifiedDecisions {
		if err := r.persistDecision(ctx, tx, input, &decision, output); err != nil {
			return nil, fmt.Errorf("failed to persist decision: %w", err)
		}
	}

	// Process risk references
	for _, risk := range input.Analysis.RiskReferences {
		if err := r.persistRisk(ctx, tx, input, &risk, output); err != nil {
			return nil, fmt.Errorf("failed to persist risk: %w", err)
		}
	}

	// Process implicit actions
	for _, action := range input.Analysis.ImplicitActions {
		if err := r.persistImplicitAction(ctx, tx, input, &action, output); err != nil {
			return nil, fmt.Errorf("failed to persist implicit action: %w", err)
		}
	}

	// Update entity project affinity
	if err := r.updateAffinity(ctx, tx, input, output); err != nil {
		return nil, fmt.Errorf("failed to update affinity: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Best-effort: detect acronyms and create review queue items (after commit, non-transactional)
	r.detectAndCreateAcronymQuestions(ctx, input, output)

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
		if !validReferenceTypes[risk.Significance] {
			return fmt.Errorf("risk reference [%d] has invalid reference_type: %s", i, risk.Significance)
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

// computeIdempotencyKey computes SHA256 hash of (source_id, assertion_type, description).
func (r *PersistRepo) computeIdempotencyKey(sourceID int64, assertionType, description string) string {
	data := fmt.Sprintf("%d:%s:%s", sourceID, assertionType, description)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// checkExistingAssertion checks if an assertion with the same source_id, type, and description exists.
// Returns the assertion ID if found, nil otherwise.
func (r *PersistRepo) checkExistingAssertion(ctx context.Context, tx pgx.Tx, sourceID int64, assertionType, description string) (*int64, error) {
	query := `
		SELECT id FROM assertions
		WHERE source_id = $1 AND assertion_type = $2 AND description = $3
		LIMIT 1
	`
	var id int64
	err := tx.QueryRow(ctx, query, sourceID, assertionType, description).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to check existing assertion: %w", err)
	}
	return &id, nil
}

// persistAction persists a verified action assertion.
func (r *PersistRepo) persistAction(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, action *VerifiedActionOutput, output *PersistFindingsOutput) error {
	assertionType := "action"

	// Check for existing assertion
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, action.Description)
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
func (r *PersistRepo) persistDecision(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, decision *VerifiedDecisionOutput, output *PersistFindingsOutput) error {
	assertionType := "decision"

	// Check for existing assertion
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, decision.Description)
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
func (r *PersistRepo) persistRisk(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, risk *RiskReferenceOutput, output *PersistFindingsOutput) error {
	assertionType := "risk"
	if !risk.IsNew && risk.RootID != nil {
		assertionType = "issue"
	}

	// Check for existing assertion
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, risk.Description)
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
func (r *PersistRepo) persistImplicitAction(ctx context.Context, tx pgx.Tx, input *PersistFindingsInput, action *ImplicitActionOutput, output *PersistFindingsOutput) error {
	assertionType := "action"

	// Check for existing assertion
	existingID, err := r.checkExistingAssertion(ctx, tx, input.SourceID, assertionType, action.Description)
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
}

// acronymPattern matches uppercase tokens 2-5 chars long.
var acronymPattern = regexp.MustCompile(`\b([A-Z]{2,5})\b`)

// detectAndCreateAcronymQuestions scans analysis text for uppercase acronyms and
// creates review queue items for unknown ones. Best-effort: errors are logged, not returned.
func (r *PersistRepo) detectAndCreateAcronymQuestions(ctx context.Context, input *PersistFindingsInput, output *PersistFindingsOutput) {
	if r.reviewQueue == nil {
		return
	}

	// Collect text from analysis findings
	texts := r.collectAnalysisTexts(input.Analysis)
	if len(texts) == 0 {
		return
	}

	// Extract candidate acronyms
	candidates := make(map[string]string) // term -> first context seen
	for _, text := range texts {
		for _, match := range acronymPattern.FindAllString(text, -1) {
			if commonAcronymExclusions[match] {
				continue
			}
			// Check all letters are uppercase (redundant with regex but safe)
			allUpper := true
			for _, ch := range match {
				if !unicode.IsUpper(ch) {
					allUpper = false
					break
				}
			}
			if !allUpper {
				continue
			}
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
		// Skip if already in glossary
		if r.glossaryRepo != nil {
			glossaryTerm, err := r.glossaryRepo.GetByTerm(ctx, term)
			if err != nil {
				logger.Warn("Failed to check glossary for acronym",
					logging.F("term", term), logging.Err(err))
				continue
			}
			if glossaryTerm != nil {
				continue // Already known
			}
		}

		// Create review queue item (CreateIfNotExists handles dedup)
		aq := reviewqueue.AcronymQuestion{
			Term:       term,
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
func (r *PersistRepo) collectAnalysisTexts(analysis *DeepAnalyzeOutput) []string {
	if analysis == nil {
		return nil
	}
	var texts []string

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

	return texts
}
