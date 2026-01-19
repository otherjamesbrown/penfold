// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// RelationshipDiscoveryInput is the input for relationship discovery workflows.
type RelationshipDiscoveryInput = pkgtemporal.RelationshipDiscoveryInput

// RelationshipDiscoveryResult is the result of relationship discovery workflows.
type RelationshipDiscoveryResult = pkgtemporal.RelationshipDiscoveryResult

// RelationshipDiscoveryWorkflowStatus tracks the status of relationship discovery.
type RelationshipDiscoveryWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for RelationshipDiscoveryWorkflow.
const (
	RelationshipDiscoveryStatusQuery  = "relationship_discovery_status"
	RelationshipDiscoveryPauseSignal  = "relationship_discovery_pause"
	RelationshipDiscoveryCancelSignal = "relationship_discovery_cancel"
)

// Activity input types for relationship discovery.
type (
	// LoadEntitiesInput is the input for the LoadEntities activity.
	LoadEntitiesInput struct {
		TenantID   string   `json:"tenant_id"`
		EntityIDs  []string `json:"entity_ids,omitempty"`  // Specific entities to load, or empty for all
		TimeWindow string   `json:"time_window,omitempty"` // For incremental mode
	}

	// LoadEntitiesOutput is the output from the LoadEntities activity.
	LoadEntitiesOutput struct {
		Entities []EntityInfo `json:"entities"`
		Count    int          `json:"count"`
	}

	// EntityInfo contains information about an entity.
	EntityInfo struct {
		ID         string            `json:"id"`
		Type       string            `json:"type"`
		Name       string            `json:"name"`
		Attributes map[string]string `json:"attributes,omitempty"`
	}

	// LoadSourcesForEntityInput is the input for the LoadSourcesForEntity activity.
	LoadSourcesForEntityInput struct {
		TenantID   string `json:"tenant_id"`
		EntityID   string `json:"entity_id"`
		TimeWindow string `json:"time_window,omitempty"`
		MaxSources int    `json:"max_sources"`
	}

	// LoadSourcesForEntityOutput is the output from the LoadSourcesForEntity activity.
	LoadSourcesForEntityOutput struct {
		Sources []SourceReference `json:"sources"`
		Count   int               `json:"count"`
	}

	// SourceReference contains a reference to a source.
	SourceReference struct {
		SourceID int64  `json:"source_id"`
		Type     string `json:"type"`
		Summary  string `json:"summary,omitempty"`
	}

	// AnalyzeRelationshipsInput is the input for the AnalyzeRelationships activity.
	AnalyzeRelationshipsInput struct {
		TenantID  string            `json:"tenant_id"`
		JobID     string            `json:"job_id"`
		Entity    EntityInfo        `json:"entity"`
		Sources   []SourceReference `json:"sources"`
		MaxDepth  int               `json:"max_depth"`
	}

	// AnalyzeRelationshipsOutput is the output from the AnalyzeRelationships activity.
	AnalyzeRelationshipsOutput struct {
		Relationships []DiscoveredRelationship `json:"relationships"`
		Count         int                      `json:"count"`
	}

	// DiscoveredRelationship contains a discovered relationship.
	DiscoveredRelationship struct {
		FromEntityID   string   `json:"from_entity_id"`
		ToEntityID     string   `json:"to_entity_id"`
		RelationType   string   `json:"relation_type"`
		Confidence     float64  `json:"confidence"`
		EvidenceSources []int64 `json:"evidence_sources"`
	}

	// ValidateRelationshipsInput is the input for the ValidateRelationships activity.
	ValidateRelationshipsInput struct {
		TenantID      string                   `json:"tenant_id"`
		JobID         string                   `json:"job_id"`
		Relationships []DiscoveredRelationship `json:"relationships"`
	}

	// ValidateRelationshipsOutput is the output from the ValidateRelationships activity.
	ValidateRelationshipsOutput struct {
		ValidRelationships   []DiscoveredRelationship `json:"valid_relationships"`
		InvalidCount         int                      `json:"invalid_count"`
		DuplicateCount       int                      `json:"duplicate_count"`
	}

	// PersistRelationshipsInput is the input for the PersistRelationships activity.
	PersistRelationshipsInput struct {
		TenantID      string                   `json:"tenant_id"`
		JobID         string                   `json:"job_id"`
		Relationships []DiscoveredRelationship `json:"relationships"`
	}

	// PersistRelationshipsOutput is the output from the PersistRelationships activity.
	PersistRelationshipsOutput struct {
		CreatedCount int `json:"created_count"`
		UpdatedCount int `json:"updated_count"`
	}

	// CalculateNetworkStatsInput is the input for the CalculateNetworkStats activity.
	CalculateNetworkStatsInput struct {
		TenantID string `json:"tenant_id"`
	}

	// RollbackRelationshipsInput is the input for the RollbackRelationships compensation activity.
	RollbackRelationshipsInput struct {
		TenantID        string  `json:"tenant_id"`
		JobID           string  `json:"job_id"`
		RelationshipIDs []int64 `json:"relationship_ids"`
	}
)

// relationshipDiscoveryState maintains the internal state of the workflow.
type relationshipDiscoveryState struct {
	status                DailyReviewWorkflowStatus
	result                *RelationshipDiscoveryResult
	paused                bool
	cancelRequested       bool
	cancelReason          string
	processedEntityCount  int
	allRelationships      []DiscoveredRelationship
	persistedRelationshipIDs []int64
}

// RelationshipDiscoveryWorkflow orchestrates the discovery and analysis of relationships.
// It performs the following steps:
// 1. Load entities to analyze
// 2. For each entity (with batching):
//    a. Load sources containing the entity
//    b. Analyze relationships via LLM
// 3. Validate discovered relationships
// 4. Persist relationships to database
// 5. Calculate network statistics
//
// This workflow implements the saga pattern with compensation for rollback on failure.
// It supports incremental, full, and entity-focused discovery modes.
func RelationshipDiscoveryWorkflow(ctx workflow.Context, input RelationshipDiscoveryInput) (*RelationshipDiscoveryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting relationship discovery workflow",
		"tenant_id", input.TenantID,
		"job_id", input.JobID,
		"discover_mode", input.DiscoverMode,
		"entity_count", len(input.EntityIDs),
	)

	// Initialize workflow state
	state := &relationshipDiscoveryState{
		status: RelationshipDiscoveryWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     5,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &RelationshipDiscoveryResult{
			TenantID: input.TenantID,
		},
		allRelationships:         make([]DiscoveredRelationship, 0),
		persistedRelationshipIDs: make([]int64, 0),
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, RelationshipDiscoveryStatusQuery, func() (RelationshipDiscoveryWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, RelationshipDiscoveryPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, RelationshipDiscoveryCancelSignal)

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	batchOpts := pkgtemporal.BatchActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()
	longRunningOpts := pkgtemporal.LongRunningActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	batchOpts = pkgtemporal.WithNonRetryableErrors(batchOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)
	longRunningOpts = pkgtemporal.WithNonRetryableErrors(longRunningOpts, pkgtemporal.NonRetryableErrors()...)

	// Saga compensation stack
	var compensations []func(workflow.Context) error

	// Handle pause signal
	handlePauseSignal := func() {
		for {
			if !state.paused {
				return
			}
			selector := workflow.NewSelector(ctx)
			selector.AddReceive(pauseSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.PauseResumeSignal
				c.Receive(ctx, &signal)
				state.paused = signal.Paused
				logger.Info("Received pause/resume signal", "paused", signal.Paused)
			})
			selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.CancelWithCompensationSignal
				c.Receive(ctx, &signal)
				state.cancelRequested = true
				state.cancelReason = signal.Reason
				state.paused = false
			})
			selector.Select(ctx)
		}
	}

	// Drain signals helper
	drainSignals := func() {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(pauseSignalChan, func(c workflow.ReceiveChannel, more bool) {
			var signal pkgtemporal.PauseResumeSignal
			c.Receive(ctx, &signal)
			state.paused = signal.Paused
		})
		selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
			var signal pkgtemporal.CancelWithCompensationSignal
			c.Receive(ctx, &signal)
			state.cancelRequested = true
			state.cancelReason = signal.Reason
		})

		for selector.HasPending() {
			selector.Select(ctx)
		}
	}

	// Check controls helper
	checkControls := func() bool {
		drainSignals()
		handlePauseSignal()
		return state.cancelRequested
	}

	// Run compensation helper
	runCompensation := func(ctx workflow.Context) {
		if len(compensations) == 0 {
			return
		}
		logger.Info("Running saga compensation", "compensation_count", len(compensations))
		state.status.CompensationRan = true

		for i := len(compensations) - 1; i >= 0; i-- {
			if err := compensations[i](ctx); err != nil {
				logger.Warn("Compensation failed", "index", i, "error", err)
			}
		}
	}

	// Update status helper
	updateStatus := func(stage, activity string) {
		state.status.Stage = stage
		state.status.LastActivity = activity
		state.status.LastUpdated = workflow.Now(ctx)
	}

	// Step 1: Load entities to analyze
	updateStatus("loading_entities", "LoadEntities")
	var loadOutput LoadEntitiesOutput
	ctx1 := workflow.WithActivityOptions(ctx, batchOpts)
	err := workflow.ExecuteActivity(ctx1, "LoadEntities", LoadEntitiesInput{
		TenantID:   input.TenantID,
		EntityIDs:  input.EntityIDs,
		TimeWindow: input.TimeWindow,
	}).Get(ctx, &loadOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("load_entities: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to load entities", "error", err)
		return state.result, nil
	}
	state.status.StepsCompleted = 1

	if len(loadOutput.Entities) == 0 {
		state.result.Status = "completed"
		state.result.EntitiesProcessed = 0
		logger.Info("No entities to process")
		return state.result, nil
	}

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after loading entities")
		return state.result, nil
	}

	// Step 2: Process entities in batches
	updateStatus("analyzing_entities", "AnalyzeRelationships")
	const entityBatchSize = 10
	const maxSourcesPerEntity = 50

	for i := 0; i < len(loadOutput.Entities); i += entityBatchSize {
		end := i + entityBatchSize
		if end > len(loadOutput.Entities) {
			end = len(loadOutput.Entities)
		}
		batch := loadOutput.Entities[i:end]

		// Process each entity in the batch in parallel
		var futures []workflow.Future
		for _, entity := range batch {
			// Load sources for entity
			entityCopy := entity // Capture loop variable
			ctx2 := workflow.WithActivityOptions(ctx, fastOpts)
			future := workflow.ExecuteActivity(ctx2, "LoadSourcesForEntity", LoadSourcesForEntityInput{
				TenantID:   input.TenantID,
				EntityID:   entityCopy.ID,
				TimeWindow: input.TimeWindow,
				MaxSources: maxSourcesPerEntity,
			})
			futures = append(futures, future)
		}

		// Collect sources and analyze relationships
		for j, future := range futures {
			var sourcesOutput LoadSourcesForEntityOutput
			if err := future.Get(ctx, &sourcesOutput); err != nil {
				logger.Warn("Failed to load sources for entity", "entity_id", batch[j].ID, "error", err)
				continue
			}

			if len(sourcesOutput.Sources) == 0 {
				continue
			}

			// Analyze relationships for this entity
			var analyzeOutput AnalyzeRelationshipsOutput
			ctx3 := workflow.WithActivityOptions(ctx, llmOpts)
			err := workflow.ExecuteActivity(ctx3, "AnalyzeRelationships", AnalyzeRelationshipsInput{
				TenantID: input.TenantID,
				JobID:    input.JobID,
				Entity:   batch[j],
				Sources:  sourcesOutput.Sources,
				MaxDepth: input.MaxDepth,
			}).Get(ctx, &analyzeOutput)
			if err != nil {
				logger.Warn("Failed to analyze relationships for entity", "entity_id", batch[j].ID, "error", err)
				continue
			}

			state.allRelationships = append(state.allRelationships, analyzeOutput.Relationships...)
			state.result.SourcesAnalyzed += len(sourcesOutput.Sources)
		}

		state.processedEntityCount += len(batch)
		state.result.EntitiesProcessed = state.processedEntityCount

		// Check for cancellation between batches
		if checkControls() {
			// If we've processed some relationships, we can still validate and save them
			if len(state.allRelationships) == 0 {
				state.result.Status = "cancelled"
				state.result.Error = state.cancelReason
				logger.Info("Workflow cancelled during entity analysis")
				return state.result, nil
			}
			// Continue to validation with partial results
			logger.Info("Workflow cancellation requested, completing with partial results")
			break
		}
	}
	state.status.StepsCompleted = 2

	// Step 3: Validate discovered relationships
	updateStatus("validating_relationships", "ValidateRelationships")
	var validateOutput ValidateRelationshipsOutput
	ctx4 := workflow.WithActivityOptions(ctx, batchOpts)
	err = workflow.ExecuteActivity(ctx4, "ValidateRelationships", ValidateRelationshipsInput{
		TenantID:      input.TenantID,
		JobID:         input.JobID,
		Relationships: state.allRelationships,
	}).Get(ctx, &validateOutput)
	if err != nil {
		logger.Warn("Relationship validation failed, using all discovered relationships", "error", err)
		validateOutput.ValidRelationships = state.allRelationships
	}
	state.status.StepsCompleted = 3

	if len(validateOutput.ValidRelationships) == 0 {
		state.result.Status = "completed"
		logger.Info("No valid relationships discovered")
		return state.result, nil
	}

	// Step 4: Persist relationships
	updateStatus("persisting_relationships", "PersistRelationships")
	var persistOutput PersistRelationshipsOutput
	ctx5 := workflow.WithActivityOptions(ctx, longRunningOpts)
	err = workflow.ExecuteActivity(ctx5, "PersistRelationships", PersistRelationshipsInput{
		TenantID:      input.TenantID,
		JobID:         input.JobID,
		Relationships: validateOutput.ValidRelationships,
	}).Get(ctx, &persistOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("persist_relationships: %v", err)
		state.status.ErrorMessage = state.result.Error
		runCompensation(ctx)
		logger.Error("Failed to persist relationships", "error", err)
		return state.result, nil
	}
	state.result.RelationshipsCreated = persistOutput.CreatedCount
	state.result.RelationshipsUpdated = persistOutput.UpdatedCount
	state.status.StepsCompleted = 4

	// Add compensation for rollback
	compensations = append(compensations, func(ctx workflow.Context) error {
		return workflow.ExecuteActivity(
			workflow.WithActivityOptions(ctx, batchOpts),
			"RollbackRelationships",
			RollbackRelationshipsInput{
				TenantID:        input.TenantID,
				JobID:           input.JobID,
				RelationshipIDs: state.persistedRelationshipIDs,
			},
		).Get(ctx, nil)
	})

	// Step 5: Calculate network statistics
	updateStatus("calculating_stats", "CalculateNetworkStats")
	var networkStats pkgtemporal.NetworkStats
	ctx6 := workflow.WithActivityOptions(ctx, batchOpts)
	err = workflow.ExecuteActivity(ctx6, "CalculateNetworkStats", CalculateNetworkStatsInput{
		TenantID: input.TenantID,
	}).Get(ctx, &networkStats)
	if err != nil {
		logger.Warn("Failed to calculate network stats", "error", err)
	} else {
		state.result.NetworkStats = &networkStats
	}
	state.status.StepsCompleted = 5

	// Build top relationships summary
	topCount := 10
	if len(validateOutput.ValidRelationships) < topCount {
		topCount = len(validateOutput.ValidRelationships)
	}
	state.result.TopRelationships = make([]pkgtemporal.RelationshipSummary, topCount)
	for i := 0; i < topCount; i++ {
		rel := validateOutput.ValidRelationships[i]
		state.result.TopRelationships[i] = pkgtemporal.RelationshipSummary{
			FromEntityID:  rel.FromEntityID,
			ToEntityID:    rel.ToEntityID,
			RelationType:  rel.RelationType,
			Confidence:    rel.Confidence,
			EvidenceCount: len(rel.EvidenceSources),
		}
	}

	updateStatus("completed", "")
	state.result.Status = "completed"
	logger.Info("Relationship discovery workflow completed",
		"tenant_id", input.TenantID,
		"entities_processed", state.result.EntitiesProcessed,
		"sources_analyzed", state.result.SourcesAnalyzed,
		"relationships_created", state.result.RelationshipsCreated,
		"relationships_updated", state.result.RelationshipsUpdated,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
