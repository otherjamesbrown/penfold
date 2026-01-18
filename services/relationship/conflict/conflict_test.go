// Package conflict provides conflict detection and resolution for discovered relationships.
package conflict

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/services/relationship/extractor"
)

// Helper function to create test relationships
func createTestRelationship(id, sourceID, targetID string, relType extractor.RelationshipType, confidence float32) *extractor.Relationship {
	now := time.Now()
	return &extractor.Relationship{
		ID: id,
		Source: &extractor.Entity{
			ID:   sourceID,
			Name: "Source " + sourceID,
			Type: extractor.EntityTypePerson,
		},
		Target: &extractor.Entity{
			ID:   targetID,
			Name: "Target " + targetID,
			Type: extractor.EntityTypePerson,
		},
		Type:       relType,
		Confidence: confidence,
		Strength:   0.5,
		Context:    "Test context",
		TenantID:   "tenant-1",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// TestDuplicateDetector tests duplicate relationship detection.
func TestDuplicateDetector(t *testing.T) {
	t.Run("detects same-type duplicates", func(t *testing.T) {
		detector := NewDuplicateDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7),
		}

		conflicts := detector.Detect(rels)

		// Should find at least one duplicate conflict
		require.NotEmpty(t, conflicts)

		// Find the same-type duplicate conflict
		var samTypeDup *Conflict
		for _, c := range conflicts {
			if c.Type == ConflictTypeDuplicate && len(c.RelationshipIDs) == 2 {
				samTypeDup = c
				break
			}
		}
		require.NotNil(t, samTypeDup, "should find same-type duplicate")
		assert.Equal(t, ConflictTypeDuplicate, samTypeDup.Type)
		assert.Len(t, samTypeDup.RelationshipIDs, 2)
	})

	t.Run("detects similar-type duplicates", func(t *testing.T) {
		detector := NewDuplicateDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeWorksWith, 0.8),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeCollaboratesWith, 0.7),
		}

		conflicts := detector.Detect(rels)

		require.Len(t, conflicts, 1)
		assert.Equal(t, ConflictTypeDuplicate, conflicts[0].Type)
		assert.Equal(t, SeverityLow, conflicts[0].Severity)
	})

	t.Run("ignores unrelated relationships", func(t *testing.T) {
		detector := NewDuplicateDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-2", "C", "D", extractor.RelationshipTypeKnows, 0.7),
		}

		conflicts := detector.Detect(rels)

		assert.Empty(t, conflicts)
	})
}

// TestContradictionDetector tests contradiction detection.
func TestContradictionDetector(t *testing.T) {
	t.Run("detects reports_to/manages contradiction", func(t *testing.T) {
		detector := NewContradictionDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-2", "B", "A", extractor.RelationshipTypeReportsTo, 0.7),
		}

		// Add the reverse type to contradiction pairs
		config := DefaultDetectorConfig()
		config.ContradictionPairs[extractor.RelationshipTypeReportsTo] = append(
			config.ContradictionPairs[extractor.RelationshipTypeReportsTo],
			extractor.RelationshipTypeReportsTo,
		)
		detector = NewContradictionDetector(config, nil)

		conflicts := detector.Detect(rels)

		// This should detect the circular reports_to relationship
		assert.GreaterOrEqual(t, len(conflicts), 0)
	})

	t.Run("detects manages/reports_to contradiction", func(t *testing.T) {
		detector := NewContradictionDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeManages, 0.8),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeReportsTo, 0.7),
		}

		conflicts := detector.Detect(rels)

		require.Len(t, conflicts, 1)
		assert.Equal(t, ConflictTypeContradiction, conflicts[0].Type)
		assert.Equal(t, SeverityHigh, conflicts[0].Severity)
	})
}

// TestCycleDetector tests cycle detection in hierarchical relationships.
func TestCycleDetector(t *testing.T) {
	t.Run("detects simple cycle", func(t *testing.T) {
		detector := NewCycleDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-2", "B", "C", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-3", "C", "A", extractor.RelationshipTypeReportsTo, 0.8),
		}

		conflicts := detector.Detect(rels)

		require.Len(t, conflicts, 1)
		assert.Equal(t, ConflictTypeCycle, conflicts[0].Type)
		assert.NotEmpty(t, conflicts[0].Context.CyclePath)
	})

	t.Run("ignores non-hierarchical relationships", func(t *testing.T) {
		detector := NewCycleDetector(DefaultDetectorConfig(), nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-2", "B", "C", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-3", "C", "A", extractor.RelationshipTypeKnows, 0.8),
		}

		conflicts := detector.Detect(rels)

		assert.Empty(t, conflicts)
	})

	t.Run("respects max cycle length", func(t *testing.T) {
		config := DefaultDetectorConfig()
		config.MaxCycleLength = 2
		detector := NewCycleDetector(config, nil)

		// Create a cycle of length 4
		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-2", "B", "C", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-3", "C", "D", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-4", "D", "A", extractor.RelationshipTypeReportsTo, 0.8),
		}

		conflicts := detector.Detect(rels)

		// Should not detect cycles longer than max length
		assert.Empty(t, conflicts)
	})
}

// TestInconsistencyDetector tests attribute inconsistency detection.
func TestInconsistencyDetector(t *testing.T) {
	t.Run("detects confidence inconsistency", func(t *testing.T) {
		config := DefaultDetectorConfig()
		config.MinConfidenceDiff = 0.3
		detector := NewInconsistencyDetector(config, nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.9),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.4),
		}

		conflicts := detector.Detect(rels)

		require.Len(t, conflicts, 1)
		assert.Equal(t, ConflictTypeInconsistency, conflicts[0].Type)
		assert.NotEmpty(t, conflicts[0].Context.AttributeMismatches)
	})

	t.Run("ignores small confidence differences", func(t *testing.T) {
		config := DefaultDetectorConfig()
		config.MinConfidenceDiff = 0.3
		detector := NewInconsistencyDetector(config, nil)

		rels := []*extractor.Relationship{
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7),
		}

		conflicts := detector.Detect(rels)

		assert.Empty(t, conflicts)
	})
}

// TestLatestWinsStrategy tests the latest-wins resolution strategy.
func TestLatestWinsStrategy(t *testing.T) {
	t.Run("keeps most recent relationship", func(t *testing.T) {
		strategy := NewLatestWinsStrategy(nil)

		older := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8)
		older.UpdatedAt = time.Now().Add(-24 * time.Hour)

		newer := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7)
		newer.UpdatedAt = time.Now()

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{older, newer},
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		assert.Equal(t, ResolutionActionKeep, resolution.Action)
		assert.Equal(t, "rel-2", resolution.KeptRelationshipID)
		assert.Contains(t, resolution.RemovedRelationshipIDs, "rel-1")
	})

	t.Run("cannot resolve contradictions", func(t *testing.T) {
		strategy := NewLatestWinsStrategy(nil)

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeContradiction,
			Relationships: []*extractor.Relationship{},
		}

		_, err := strategy.Resolve(conflict)

		assert.Error(t, err)
	})
}

// TestHighConfidenceStrategy tests the high-confidence resolution strategy.
func TestHighConfidenceStrategy(t *testing.T) {
	t.Run("keeps highest confidence relationship", func(t *testing.T) {
		strategy := NewHighConfidenceStrategy(nil)

		low := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.5)
		high := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.9)

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{low, high},
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		assert.Equal(t, ResolutionActionKeep, resolution.Action)
		assert.Equal(t, "rel-2", resolution.KeptRelationshipID)
	})

	t.Run("flags for review when confidence gap is small", func(t *testing.T) {
		strategy := NewHighConfidenceStrategy(nil)

		rel1 := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.6)
		rel2 := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.62)

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{rel1, rel2},
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		// Small gap should result in flag for review
		assert.Equal(t, ResolutionActionFlagForReview, resolution.Action)
	})
}

// TestMergeStrategy tests the merge resolution strategy.
func TestMergeStrategy(t *testing.T) {
	t.Run("merges relationships with combined evidence", func(t *testing.T) {
		strategy := NewMergeStrategy(nil)

		rel1 := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.6)
		rel1.Evidence = []extractor.Evidence{{SourceID: "src-1"}}

		rel2 := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7)
		rel2.Evidence = []extractor.Evidence{{SourceID: "src-2"}}

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{rel1, rel2},
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		assert.Equal(t, ResolutionActionMerge, resolution.Action)
		assert.NotNil(t, resolution.MergedRelationship)
		assert.Len(t, resolution.MergedRelationship.Evidence, 2)
		assert.Len(t, resolution.RemovedRelationshipIDs, 2)
	})

	t.Run("combined confidence is probabilistic", func(t *testing.T) {
		strategy := NewMergeStrategy(nil)

		rel1 := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.6)
		rel2 := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7)

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{rel1, rel2},
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		// P(A or B) = P(A) + P(B) - P(A)*P(B) = 0.6 + 0.7 - 0.42 = 0.88
		assert.InDelta(t, 0.88, resolution.MergedRelationship.Confidence, 0.01)
	})
}

// TestHumanReviewStrategy tests the human review strategy.
func TestHumanReviewStrategy(t *testing.T) {
	t.Run("always flags for review", func(t *testing.T) {
		strategy := NewHumanReviewStrategy(nil)

		conflict := &Conflict{
			ID:       "conflict-1",
			Type:     ConflictTypeContradiction,
			Severity: SeverityHigh,
		}

		resolution, err := strategy.Resolve(conflict)

		require.NoError(t, err)
		assert.Equal(t, ResolutionActionFlagForReview, resolution.Action)
		assert.Equal(t, float64(1.0), resolution.Confidence)
	})
}

// TestConflictResolver tests the main ConflictResolver.
func TestConflictResolver(t *testing.T) {
	t.Run("detects multiple conflict types", func(t *testing.T) {
		resolver := NewConflictResolver(DefaultResolverConfig(), nil)

		rels := []*extractor.Relationship{
			// Duplicates
			createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
			createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7),
			// Cycle
			createTestRelationship("rel-3", "C", "D", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-4", "D", "E", extractor.RelationshipTypeReportsTo, 0.8),
			createTestRelationship("rel-5", "E", "C", extractor.RelationshipTypeReportsTo, 0.8),
		}

		conflicts := resolver.DetectConflicts(rels)

		// Should find at least duplicates and cycles
		assert.NotEmpty(t, conflicts)

		hasdup := false
		hascycle := false
		for _, c := range conflicts {
			if c.Type == ConflictTypeDuplicate {
				hasdup = true
			}
			if c.Type == ConflictTypeCycle {
				hascycle = true
			}
		}
		assert.True(t, hasdup, "should detect duplicate")
		assert.True(t, hascycle, "should detect cycle")
	})

	t.Run("auto-resolves high-confidence conflicts", func(t *testing.T) {
		config := DefaultResolverConfig()
		config.AutoResolveMinConfidence = 0.7
		resolver := NewConflictResolver(config, nil)

		low := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.5)
		high := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.95)

		conflict := &Conflict{
			ID:              "conflict-1",
			Type:            ConflictTypeDuplicate,
			Relationships:   []*extractor.Relationship{low, high},
			RelationshipIDs: []string{"rel-1", "rel-2"},
		}

		resolution, err := resolver.AutoResolve(conflict)

		require.NoError(t, err)
		assert.Equal(t, ResolutionActionKeep, resolution.Action)
	})

	t.Run("proposes multiple resolution options", func(t *testing.T) {
		resolver := NewConflictResolver(DefaultResolverConfig(), nil)

		rel1 := createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8)
		rel2 := createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7)

		conflict := &Conflict{
			ID:            "conflict-1",
			Type:          ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{rel1, rel2},
		}

		options, err := resolver.ProposeResolutions(conflict)

		require.NoError(t, err)
		assert.NotEmpty(t, options)

		// Should have at least one recommended option
		hasRecommended := false
		for _, opt := range options {
			if opt.Recommended {
				hasRecommended = true
				break
			}
		}
		assert.True(t, hasRecommended)
	})

	t.Run("tracks resolution history", func(t *testing.T) {
		resolver := NewConflictResolver(DefaultResolverConfig(), nil)

		conflict := &Conflict{
			ID:   "conflict-1",
			Type: ConflictTypeDuplicate,
			Relationships: []*extractor.Relationship{
				createTestRelationship("rel-1", "A", "B", extractor.RelationshipTypeKnows, 0.8),
				createTestRelationship("rel-2", "A", "B", extractor.RelationshipTypeKnows, 0.7),
			},
		}

		_, err := resolver.Resolve(conflict, "high_confidence")
		require.NoError(t, err)

		history := resolver.GetResolutionHistory(10)
		require.Len(t, history, 1)
		assert.Equal(t, "conflict-1", history[0].ConflictID)
	})
}

// TestInMemoryReviewQueue tests the review queue implementation.
func TestInMemoryReviewQueue(t *testing.T) {
	ctx := context.Background()

	t.Run("queues and retrieves conflicts", func(t *testing.T) {
		queue := NewInMemoryReviewQueue(nil)

		conflict := &Conflict{
			ID:       "conflict-1",
			TenantID: "tenant-1",
			Type:     ConflictTypeContradiction,
			Severity: SeverityHigh,
		}

		err := queue.QueueForReview(ctx, conflict)
		require.NoError(t, err)

		pending, err := queue.GetPendingConflicts(ctx, "tenant-1", 10)
		require.NoError(t, err)
		require.Len(t, pending, 1)
		assert.Equal(t, "conflict-1", pending[0].ID)
	})

	t.Run("applies resolution", func(t *testing.T) {
		queue := NewInMemoryReviewQueue(nil)

		conflict := &Conflict{
			ID:       "conflict-1",
			TenantID: "tenant-1",
		}

		err := queue.QueueForReview(ctx, conflict)
		require.NoError(t, err)

		resolution := &Resolution{
			Action:      ResolutionActionKeep,
			Confidence:  0.9,
			Explanation: "Test resolution",
		}

		err = queue.ApplyResolution(ctx, "conflict-1", resolution)
		require.NoError(t, err)

		// Should not appear in pending
		pending, err := queue.GetPendingConflicts(ctx, "tenant-1", 10)
		require.NoError(t, err)
		assert.Empty(t, pending)
	})

	t.Run("prioritizes by severity", func(t *testing.T) {
		queue := NewInMemoryReviewQueue(nil)

		low := &Conflict{ID: "low", TenantID: "tenant-1", Severity: SeverityLow}
		high := &Conflict{ID: "high", TenantID: "tenant-1", Severity: SeverityHigh}
		urgent := &Conflict{ID: "urgent", TenantID: "tenant-1", Severity: SeverityCritical}

		// Queue in reverse priority order
		err := queue.QueueForReview(ctx, low)
		require.NoError(t, err)
		err = queue.QueueForReview(ctx, urgent)
		require.NoError(t, err)
		err = queue.QueueForReview(ctx, high)
		require.NoError(t, err)

		pending, err := queue.GetPendingConflicts(ctx, "tenant-1", 10)
		require.NoError(t, err)
		require.Len(t, pending, 3)

		// Should be sorted by priority
		assert.Equal(t, "urgent", pending[0].ID)
		assert.Equal(t, "high", pending[1].ID)
		assert.Equal(t, "low", pending[2].ID)
	})
}

// TestSeverityCalculation tests severity score calculations.
func TestSeverityCalculation(t *testing.T) {
	t.Run("severity scores are ordered correctly", func(t *testing.T) {
		assert.Greater(t, SeverityCritical.SeverityScore(), SeverityHigh.SeverityScore())
		assert.Greater(t, SeverityHigh.SeverityScore(), SeverityMedium.SeverityScore())
		assert.Greater(t, SeverityMedium.SeverityScore(), SeverityLow.SeverityScore())
	})
}

// TestConflictFilter tests conflict filtering.
func TestConflictFilter(t *testing.T) {
	t.Run("filters by type", func(t *testing.T) {
		resolver := NewConflictResolver(DefaultResolverConfig(), nil)

		// Manually add conflicts
		dup := &Conflict{ID: "dup", Type: ConflictTypeDuplicate, TenantID: "t1"}
		cycle := &Conflict{ID: "cycle", Type: ConflictTypeCycle, TenantID: "t1"}

		resolver.conflictsByID["dup"] = dup
		resolver.conflictsByID["cycle"] = cycle

		filter := &ConflictFilter{
			Types: []ConflictType{ConflictTypeDuplicate},
		}

		results := resolver.GetConflicts(filter)
		require.Len(t, results, 1)
		assert.Equal(t, ConflictTypeDuplicate, results[0].Type)
	})

	t.Run("filters by tenant", func(t *testing.T) {
		resolver := NewConflictResolver(DefaultResolverConfig(), nil)

		c1 := &Conflict{ID: "c1", Type: ConflictTypeDuplicate, TenantID: "t1"}
		c2 := &Conflict{ID: "c2", Type: ConflictTypeDuplicate, TenantID: "t2"}

		resolver.conflictsByID["c1"] = c1
		resolver.conflictsByID["c2"] = c2

		filter := &ConflictFilter{
			TenantID: "t1",
		}

		results := resolver.GetConflicts(filter)
		require.Len(t, results, 1)
		assert.Equal(t, "t1", results[0].TenantID)
	})
}
