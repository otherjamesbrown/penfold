// Package conflict provides conflict detection and resolution for discovered relationships.
package conflict

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/relationship/extractor"
)

// Detector defines the interface for conflict detectors.
type Detector interface {
	// Name returns the detector name.
	Name() string

	// DetectType returns the conflict type this detector finds.
	DetectType() ConflictType

	// Detect finds conflicts in the given relationships.
	Detect(relationships []*extractor.Relationship) []*Conflict
}

// DetectorRegistry holds all registered conflict detectors.
type DetectorRegistry struct {
	detectors []Detector
	config    *DetectorConfig
	logger    logging.Logger
}

// NewDetectorRegistry creates a new DetectorRegistry with the given config.
func NewDetectorRegistry(config *DetectorConfig, logger logging.Logger) *DetectorRegistry {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}

	return &DetectorRegistry{
		detectors: make([]Detector, 0),
		config:    config,
		logger:    logger.With(logging.F("component", "detector_registry")),
	}
}

// Register adds a detector to the registry.
func (r *DetectorRegistry) Register(detector Detector) {
	r.detectors = append(r.detectors, detector)
	r.logger.Debug("Registered detector",
		logging.F("name", detector.Name()),
		logging.F("type", detector.DetectType()),
	)
}

// RegisterDefaults registers all default detectors.
func (r *DetectorRegistry) RegisterDefaults() {
	r.Register(NewDuplicateDetector(r.config, r.logger))
	r.Register(NewContradictionDetector(r.config, r.logger))
	r.Register(NewCycleDetector(r.config, r.logger))
	r.Register(NewInconsistencyDetector(r.config, r.logger))
}

// DetectAll runs all registered detectors and returns combined results.
func (r *DetectorRegistry) DetectAll(relationships []*extractor.Relationship) []*Conflict {
	var allConflicts []*Conflict

	for _, detector := range r.detectors {
		conflicts := detector.Detect(relationships)
		allConflicts = append(allConflicts, conflicts...)

		r.logger.Debug("Detector completed",
			logging.F("detector", detector.Name()),
			logging.F("conflicts_found", len(conflicts)),
		)
	}

	// Sort by severity (highest first), then by detection time
	sort.Slice(allConflicts, func(i, j int) bool {
		if allConflicts[i].Severity.SeverityScore() != allConflicts[j].Severity.SeverityScore() {
			return allConflicts[i].Severity.SeverityScore() > allConflicts[j].Severity.SeverityScore()
		}
		return allConflicts[i].DetectedAt.Before(allConflicts[j].DetectedAt)
	})

	return allConflicts
}

// DuplicateDetector identifies relationships between the same entities that may be duplicates.
type DuplicateDetector struct {
	config *DetectorConfig
	logger logging.Logger
}

// NewDuplicateDetector creates a new DuplicateDetector.
func NewDuplicateDetector(config *DetectorConfig, logger logging.Logger) *DuplicateDetector {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &DuplicateDetector{
		config: config,
		logger: logger.With(logging.F("detector", "duplicate")),
	}
}

// Name returns the detector name.
func (d *DuplicateDetector) Name() string {
	return "duplicate_detector"
}

// DetectType returns the conflict type.
func (d *DuplicateDetector) DetectType() ConflictType {
	return ConflictTypeDuplicate
}

// Detect finds duplicate relationships.
func (d *DuplicateDetector) Detect(relationships []*extractor.Relationship) []*Conflict {
	var conflicts []*Conflict

	// Group relationships by entity pair
	groups := make(map[string][]*extractor.Relationship)
	for _, rel := range relationships {
		if rel == nil || rel.Source == nil || rel.Target == nil {
			continue
		}
		key := d.entityPairKey(rel.Source.ID, rel.Target.ID)
		groups[key] = append(groups[key], rel)
	}

	// Check each group for duplicates
	now := time.Now()
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Check for same-type duplicates (strong duplicates)
		typeGroups := make(map[extractor.RelationshipType][]*extractor.Relationship)
		for _, rel := range group {
			typeGroups[rel.Type] = append(typeGroups[rel.Type], rel)
		}

		for relType, sameTypeRels := range typeGroups {
			if len(sameTypeRels) < 2 {
				continue
			}

			// These are definite duplicates - same entities, same type
			conflict := d.createConflict(sameTypeRels, now)
			conflict.Severity = SeverityMedium
			conflict.Context.Description = fmt.Sprintf(
				"Found %d relationships of type '%s' between the same entities",
				len(sameTypeRels), relType,
			)
			conflicts = append(conflicts, conflict)
		}

		// Check for similar-type duplicates (potential duplicates)
		if len(group) > 1 {
			// Check if there are similar relationships that might be duplicates
			// For example, "works_with" and "collaborates_with" between same entities
			for i := 0; i < len(group); i++ {
				for j := i + 1; j < len(group); j++ {
					if d.areSimilarRelationships(group[i], group[j]) {
						conflict := d.createConflict([]*extractor.Relationship{group[i], group[j]}, now)
						conflict.Severity = SeverityLow
						conflict.Context.Description = fmt.Sprintf(
							"Potentially duplicate relationships '%s' and '%s' between the same entities",
							group[i].Type, group[j].Type,
						)
						conflicts = append(conflicts, conflict)
					}
				}
			}
		}
	}

	return conflicts
}

// entityPairKey creates a consistent key for an entity pair (order-independent).
func (d *DuplicateDetector) entityPairKey(sourceID, targetID string) string {
	if sourceID < targetID {
		return sourceID + ":" + targetID
	}
	return targetID + ":" + sourceID
}

// areSimilarRelationships checks if two relationships are semantically similar.
func (d *DuplicateDetector) areSimilarRelationships(r1, r2 *extractor.Relationship) bool {
	// Similar relationship type mappings
	similarTypes := map[extractor.RelationshipType][]extractor.RelationshipType{
		extractor.RelationshipTypeWorksWith:        {extractor.RelationshipTypeCollaboratesWith},
		extractor.RelationshipTypeCollaboratesWith: {extractor.RelationshipTypeWorksWith},
		extractor.RelationshipTypeKnowsAbout:       {extractor.RelationshipTypeDiscussed},
		extractor.RelationshipTypeDiscussed:        {extractor.RelationshipTypeKnowsAbout},
	}

	// Check if types are similar
	if similar, ok := similarTypes[r1.Type]; ok {
		for _, t := range similar {
			if r2.Type == t {
				return true
			}
		}
	}

	// Check context similarity using simple string comparison
	if r1.Context != "" && r2.Context != "" {
		similarity := d.textSimilarity(r1.Context, r2.Context)
		return similarity >= d.config.DuplicateThreshold
	}

	return false
}

// textSimilarity calculates Jaccard similarity between two texts.
func (d *DuplicateDetector) textSimilarity(text1, text2 string) float64 {
	words1 := strings.Fields(strings.ToLower(text1))
	words2 := strings.Fields(strings.ToLower(text2))

	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	set2 := make(map[string]bool)
	for _, w := range words2 {
		set2[w] = true
	}

	// Calculate intersection and union
	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	union := len(set1)
	for w := range set2 {
		if !set1[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// createConflict creates a Conflict for the given relationships.
func (d *DuplicateDetector) createConflict(rels []*extractor.Relationship, now time.Time) *Conflict {
	relIDs := make([]string, len(rels))
	relTypes := make([]extractor.RelationshipType, 0)
	minConf := float32(1.0)
	maxConf := float32(0.0)
	minDate := now
	maxDate := time.Time{}

	for i, rel := range rels {
		relIDs[i] = rel.ID
		relTypes = append(relTypes, rel.Type)
		if rel.Confidence < minConf {
			minConf = rel.Confidence
		}
		if rel.Confidence > maxConf {
			maxConf = rel.Confidence
		}
		if rel.CreatedAt.Before(minDate) {
			minDate = rel.CreatedAt
		}
		if rel.CreatedAt.After(maxDate) {
			maxDate = rel.CreatedAt
		}
	}

	// Deduplicate relationship types
	typeSet := make(map[extractor.RelationshipType]bool)
	for _, t := range relTypes {
		typeSet[t] = true
	}
	uniqueTypes := make([]extractor.RelationshipType, 0, len(typeSet))
	for t := range typeSet {
		uniqueTypes = append(uniqueTypes, t)
	}

	return &Conflict{
		ID:              generateConflictID(relIDs),
		TenantID:        rels[0].TenantID,
		Type:            ConflictTypeDuplicate,
		Relationships:   rels,
		RelationshipIDs: relIDs,
		DetectedAt:      now,
		Context: &ConflictContext{
			SourceEntityID:    rels[0].Source.ID,
			TargetEntityID:    rels[0].Target.ID,
			SourceEntityName:  rels[0].Source.Name,
			TargetEntityName:  rels[0].Target.Name,
			RelationshipTypes: uniqueTypes,
			ConfidenceRange:   [2]float32{minConf, maxConf},
			DateRange:         [2]time.Time{minDate, maxDate},
		},
		Metadata: make(map[string]string),
	}
}

// ContradictionDetector identifies relationships that contradict each other.
type ContradictionDetector struct {
	config *DetectorConfig
	logger logging.Logger
}

// NewContradictionDetector creates a new ContradictionDetector.
func NewContradictionDetector(config *DetectorConfig, logger logging.Logger) *ContradictionDetector {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ContradictionDetector{
		config: config,
		logger: logger.With(logging.F("detector", "contradiction")),
	}
}

// Name returns the detector name.
func (d *ContradictionDetector) Name() string {
	return "contradiction_detector"
}

// DetectType returns the conflict type.
func (d *ContradictionDetector) DetectType() ConflictType {
	return ConflictTypeContradiction
}

// Detect finds contradictory relationships.
func (d *ContradictionDetector) Detect(relationships []*extractor.Relationship) []*Conflict {
	var conflicts []*Conflict

	// Index relationships by directed entity pair
	pairIndex := make(map[string][]*extractor.Relationship)
	for _, rel := range relationships {
		if rel == nil || rel.Source == nil || rel.Target == nil {
			continue
		}
		key := rel.Source.ID + "->" + rel.Target.ID
		pairIndex[key] = append(pairIndex[key], rel)
	}

	now := time.Now()
	checked := make(map[string]bool)

	for _, rel := range relationships {
		if rel == nil || rel.Source == nil || rel.Target == nil {
			continue
		}

		// Check for contradictions with reverse relationships
		reverseKey := rel.Target.ID + "->" + rel.Source.ID
		if reverseRels, ok := pairIndex[reverseKey]; ok {
			for _, reverseRel := range reverseRels {
				// Check if this pair is a known contradiction
				if d.areContradictory(rel.Type, reverseRel.Type) {
					conflictKey := d.conflictKey(rel.ID, reverseRel.ID)
					if checked[conflictKey] {
						continue
					}
					checked[conflictKey] = true

					conflict := d.createConflict([]*extractor.Relationship{rel, reverseRel}, now)
					conflicts = append(conflicts, conflict)
				}
			}
		}

		// Check for contradictions in same direction
		forwardKey := rel.Source.ID + "->" + rel.Target.ID
		if forwardRels, ok := pairIndex[forwardKey]; ok {
			for _, forwardRel := range forwardRels {
				if forwardRel.ID == rel.ID {
					continue
				}
				// Check if same-direction relationships contradict
				if d.areContradictory(rel.Type, forwardRel.Type) {
					conflictKey := d.conflictKey(rel.ID, forwardRel.ID)
					if checked[conflictKey] {
						continue
					}
					checked[conflictKey] = true

					conflict := d.createConflict([]*extractor.Relationship{rel, forwardRel}, now)
					conflicts = append(conflicts, conflict)
				}
			}
		}
	}

	return conflicts
}

// areContradictory checks if two relationship types are contradictory.
func (d *ContradictionDetector) areContradictory(type1, type2 extractor.RelationshipType) bool {
	if contradictions, ok := d.config.ContradictionPairs[type1]; ok {
		for _, t := range contradictions {
			if t == type2 {
				return true
			}
		}
	}
	return false
}

// conflictKey creates a consistent key for a conflict pair.
func (d *ContradictionDetector) conflictKey(id1, id2 string) string {
	if id1 < id2 {
		return id1 + ":" + id2
	}
	return id2 + ":" + id1
}

// createConflict creates a Conflict for contradictory relationships.
func (d *ContradictionDetector) createConflict(rels []*extractor.Relationship, now time.Time) *Conflict {
	relIDs := make([]string, len(rels))
	relTypes := make([]extractor.RelationshipType, len(rels))

	for i, rel := range rels {
		relIDs[i] = rel.ID
		relTypes[i] = rel.Type
	}

	severity := SeverityHigh
	// If confidence scores are both high, this is more critical
	if len(rels) >= 2 && rels[0].Confidence > 0.7 && rels[1].Confidence > 0.7 {
		severity = SeverityCritical
	}

	return &Conflict{
		ID:              generateConflictID(relIDs),
		TenantID:        rels[0].TenantID,
		Type:            ConflictTypeContradiction,
		Severity:        severity,
		Relationships:   rels,
		RelationshipIDs: relIDs,
		DetectedAt:      now,
		Context: &ConflictContext{
			Description: fmt.Sprintf(
				"Contradictory relationships '%s' and '%s' detected between entities",
				rels[0].Type, rels[1].Type,
			),
			SourceEntityID:    rels[0].Source.ID,
			TargetEntityID:    rels[0].Target.ID,
			SourceEntityName:  rels[0].Source.Name,
			TargetEntityName:  rels[0].Target.Name,
			RelationshipTypes: relTypes,
		},
		Metadata: make(map[string]string),
	}
}

// CycleDetector identifies circular dependencies in hierarchical relationships.
type CycleDetector struct {
	config *DetectorConfig
	logger logging.Logger
}

// NewCycleDetector creates a new CycleDetector.
func NewCycleDetector(config *DetectorConfig, logger logging.Logger) *CycleDetector {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &CycleDetector{
		config: config,
		logger: logger.With(logging.F("detector", "cycle")),
	}
}

// Name returns the detector name.
func (d *CycleDetector) Name() string {
	return "cycle_detector"
}

// DetectType returns the conflict type.
func (d *CycleDetector) DetectType() ConflictType {
	return ConflictTypeCycle
}

// Detect finds cycles in hierarchical relationships.
func (d *CycleDetector) Detect(relationships []*extractor.Relationship) []*Conflict {
	var conflicts []*Conflict

	// Filter to only hierarchical relationship types
	hierarchicalTypes := map[extractor.RelationshipType]bool{
		extractor.RelationshipTypeReportsTo: true,
		extractor.RelationshipTypeManages:   true,
		extractor.RelationshipTypePartOf:    true,
		extractor.RelationshipTypeMemberOf:  true,
	}

	// Build adjacency list for hierarchical relationships
	adjacency := make(map[string][]string)
	relMap := make(map[string]*extractor.Relationship)

	for _, rel := range relationships {
		if rel == nil || rel.Source == nil || rel.Target == nil {
			continue
		}
		if !hierarchicalTypes[rel.Type] {
			continue
		}

		adjacency[rel.Source.ID] = append(adjacency[rel.Source.ID], rel.Target.ID)
		relMap[rel.Source.ID+"->"+rel.Target.ID] = rel
	}

	// Find cycles using DFS
	now := time.Now()
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for node := range adjacency {
		if !visited[node] {
			if cycle := d.findCycle(node, adjacency, visited, recStack, nil, d.config.MaxCycleLength); cycle != nil {
				// Collect relationships in the cycle
				cycleRels := make([]*extractor.Relationship, 0)
				for i := 0; i < len(cycle)-1; i++ {
					key := cycle[i] + "->" + cycle[i+1]
					if rel, ok := relMap[key]; ok {
						cycleRels = append(cycleRels, rel)
					}
				}

				if len(cycleRels) > 0 {
					conflict := d.createConflict(cycleRels, cycle, now)
					conflicts = append(conflicts, conflict)
				}
			}
		}
	}

	return conflicts
}

// findCycle performs DFS to find cycles.
func (d *CycleDetector) findCycle(
	node string,
	adjacency map[string][]string,
	visited, recStack map[string]bool,
	path []string,
	maxLength int,
) []string {
	if len(path) > maxLength {
		return nil
	}

	visited[node] = true
	recStack[node] = true
	path = append(path, node)

	for _, neighbor := range adjacency[node] {
		if !visited[neighbor] {
			if cycle := d.findCycle(neighbor, adjacency, visited, recStack, path, maxLength); cycle != nil {
				return cycle
			}
		} else if recStack[neighbor] {
			// Found a cycle - extract it
			cycleStart := -1
			for i, n := range path {
				if n == neighbor {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := append(path[cycleStart:], neighbor)
				return cycle
			}
		}
	}

	recStack[node] = false
	return nil
}

// createConflict creates a Conflict for a cycle.
func (d *CycleDetector) createConflict(rels []*extractor.Relationship, cycle []string, now time.Time) *Conflict {
	relIDs := make([]string, len(rels))
	relTypes := make([]extractor.RelationshipType, len(rels))

	for i, rel := range rels {
		relIDs[i] = rel.ID
		relTypes[i] = rel.Type
	}

	// Longer cycles are more severe (harder to fix)
	severity := SeverityMedium
	if len(cycle) > 5 {
		severity = SeverityHigh
	}

	return &Conflict{
		ID:              generateConflictID(relIDs),
		TenantID:        rels[0].TenantID,
		Type:            ConflictTypeCycle,
		Severity:        severity,
		Relationships:   rels,
		RelationshipIDs: relIDs,
		DetectedAt:      now,
		Context: &ConflictContext{
			Description: fmt.Sprintf(
				"Circular dependency detected with %d entities: %s",
				len(cycle)-1, strings.Join(cycle, " -> "),
			),
			CyclePath:         cycle,
			RelationshipTypes: relTypes,
		},
		Metadata: make(map[string]string),
	}
}

// InconsistencyDetector identifies attribute mismatches between related relationships.
type InconsistencyDetector struct {
	config *DetectorConfig
	logger logging.Logger
}

// NewInconsistencyDetector creates a new InconsistencyDetector.
func NewInconsistencyDetector(config *DetectorConfig, logger logging.Logger) *InconsistencyDetector {
	if config == nil {
		config = DefaultDetectorConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}
	return &InconsistencyDetector{
		config: config,
		logger: logger.With(logging.F("detector", "inconsistency")),
	}
}

// Name returns the detector name.
func (d *InconsistencyDetector) Name() string {
	return "inconsistency_detector"
}

// DetectType returns the conflict type.
func (d *InconsistencyDetector) DetectType() ConflictType {
	return ConflictTypeInconsistency
}

// Detect finds inconsistencies in relationships.
func (d *InconsistencyDetector) Detect(relationships []*extractor.Relationship) []*Conflict {
	var conflicts []*Conflict

	// Group by entity pair and type to find relationships that should be consistent
	groups := make(map[string][]*extractor.Relationship)
	for _, rel := range relationships {
		if rel == nil || rel.Source == nil || rel.Target == nil {
			continue
		}
		// Key includes type since we're looking for inconsistencies within same relationship type
		key := fmt.Sprintf("%s->%s:%s", rel.Source.ID, rel.Target.ID, rel.Type)
		groups[key] = append(groups[key], rel)
	}

	now := time.Now()
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Check for confidence inconsistencies
		minConf := float32(1.0)
		maxConf := float32(0.0)
		for _, rel := range group {
			if rel.Confidence < minConf {
				minConf = rel.Confidence
			}
			if rel.Confidence > maxConf {
				maxConf = rel.Confidence
			}
		}

		if maxConf-minConf >= d.config.MinConfidenceDiff {
			conflict := d.createConfidenceConflict(group, minConf, maxConf, now)
			conflicts = append(conflicts, conflict)
		}

		// Check for strength inconsistencies
		minStrength := float32(1.0)
		maxStrength := float32(0.0)
		for _, rel := range group {
			if rel.Strength < minStrength {
				minStrength = rel.Strength
			}
			if rel.Strength > maxStrength {
				maxStrength = rel.Strength
			}
		}

		if maxStrength-minStrength >= d.config.MinConfidenceDiff {
			conflict := d.createStrengthConflict(group, minStrength, maxStrength, now)
			conflicts = append(conflicts, conflict)
		}
	}

	return conflicts
}

// createConfidenceConflict creates a Conflict for confidence inconsistencies.
func (d *InconsistencyDetector) createConfidenceConflict(
	rels []*extractor.Relationship,
	minConf, maxConf float32,
	now time.Time,
) *Conflict {
	relIDs := make([]string, len(rels))
	for i, rel := range rels {
		relIDs[i] = rel.ID
	}

	// Map confidence values to relationship IDs
	confMap := make(map[string]string)
	for _, rel := range rels {
		confMap[fmt.Sprintf("%.2f", rel.Confidence)] = rel.ID
	}

	return &Conflict{
		ID:              generateConflictID(relIDs),
		TenantID:        rels[0].TenantID,
		Type:            ConflictTypeInconsistency,
		Severity:        SeverityLow,
		Relationships:   rels,
		RelationshipIDs: relIDs,
		DetectedAt:      now,
		Context: &ConflictContext{
			Description: fmt.Sprintf(
				"Confidence score inconsistency: values range from %.2f to %.2f (diff: %.2f)",
				minConf, maxConf, maxConf-minConf,
			),
			SourceEntityID:   rels[0].Source.ID,
			TargetEntityID:   rels[0].Target.ID,
			SourceEntityName: rels[0].Source.Name,
			TargetEntityName: rels[0].Target.Name,
			ConfidenceRange:  [2]float32{minConf, maxConf},
			AttributeMismatches: []AttributeMismatch{
				{
					Attribute:       "confidence",
					Values:          []string{fmt.Sprintf("%.2f", minConf), fmt.Sprintf("%.2f", maxConf)},
					RelationshipIDs: confMap,
				},
			},
		},
		Metadata: make(map[string]string),
	}
}

// createStrengthConflict creates a Conflict for strength inconsistencies.
func (d *InconsistencyDetector) createStrengthConflict(
	rels []*extractor.Relationship,
	minStrength, maxStrength float32,
	now time.Time,
) *Conflict {
	relIDs := make([]string, len(rels))
	for i, rel := range rels {
		relIDs[i] = rel.ID
	}

	// Map strength values to relationship IDs
	strengthMap := make(map[string]string)
	for _, rel := range rels {
		strengthMap[fmt.Sprintf("%.2f", rel.Strength)] = rel.ID
	}

	return &Conflict{
		ID:              generateConflictID(relIDs),
		TenantID:        rels[0].TenantID,
		Type:            ConflictTypeInconsistency,
		Severity:        SeverityLow,
		Relationships:   rels,
		RelationshipIDs: relIDs,
		DetectedAt:      now,
		Context: &ConflictContext{
			Description: fmt.Sprintf(
				"Strength score inconsistency: values range from %.2f to %.2f (diff: %.2f)",
				minStrength, maxStrength, maxStrength-minStrength,
			),
			SourceEntityID:   rels[0].Source.ID,
			TargetEntityID:   rels[0].Target.ID,
			SourceEntityName: rels[0].Source.Name,
			TargetEntityName: rels[0].Target.Name,
			AttributeMismatches: []AttributeMismatch{
				{
					Attribute:       "strength",
					Values:          []string{fmt.Sprintf("%.2f", minStrength), fmt.Sprintf("%.2f", maxStrength)},
					RelationshipIDs: strengthMap,
				},
			},
		},
		Metadata: make(map[string]string),
	}
}

// generateConflictID creates a deterministic ID for a conflict.
func generateConflictID(relIDs []string) string {
	// Sort IDs for consistency
	sorted := make([]string, len(relIDs))
	copy(sorted, relIDs)
	sort.Strings(sorted)

	h := sha256.New()
	h.Write([]byte(strings.Join(sorted, ":")))
	return "conflict-" + hex.EncodeToString(h.Sum(nil))[:12]
}

// CalculateSeverity calculates an appropriate severity for a conflict.
func CalculateSeverity(conflict *Conflict) Severity {
	if conflict == nil {
		return SeverityLow
	}

	// Base severity by type
	baseSeverity := map[ConflictType]Severity{
		ConflictTypeDuplicate:     SeverityMedium,
		ConflictTypeContradiction: SeverityHigh,
		ConflictTypeCycle:         SeverityMedium,
		ConflictTypeInconsistency: SeverityLow,
	}

	severity := baseSeverity[conflict.Type]
	if severity == "" {
		severity = SeverityMedium
	}

	// Adjust based on confidence
	if len(conflict.Relationships) > 0 {
		avgConfidence := float32(0)
		for _, rel := range conflict.Relationships {
			avgConfidence += rel.Confidence
		}
		avgConfidence /= float32(len(conflict.Relationships))

		// High-confidence conflicts are more severe
		if avgConfidence > 0.8 {
			switch severity {
			case SeverityLow:
				severity = SeverityMedium
			case SeverityMedium:
				severity = SeverityHigh
			case SeverityHigh:
				severity = SeverityCritical
			}
		}
	}

	// Adjust based on number of relationships
	if len(conflict.Relationships) > 5 {
		switch severity {
		case SeverityLow:
			severity = SeverityMedium
		case SeverityMedium:
			severity = SeverityHigh
		}
	}

	return severity
}

// noopLogger is a no-op logger implementation.
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, fields ...logging.Field) {}
func (l *noopLogger) Info(msg string, fields ...logging.Field)  {}
func (l *noopLogger) Warn(msg string, fields ...logging.Field)  {}
func (l *noopLogger) Error(msg string, fields ...logging.Field) {}
func (l *noopLogger) With(fields ...logging.Field) logging.Logger {
	return l
}
func (l *noopLogger) WithContext(_ context.Context) logging.Logger {
	return l
}

// Utility functions

// abs returns the absolute value of a float32.
func abs32(x float32) float32 {
	return float32(math.Abs(float64(x)))
}
