// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// createRelationshipTestDeps creates test dependencies for relationship commands.
func createRelationshipTestDeps(cfg *config.CLIConfig) *RelationshipCommandDeps {
	return &RelationshipCommandDeps{
		Config:       cfg,
		OutputFormat: cfg.OutputFormat,
		LoadConfig: func() (*config.CLIConfig, error) {
			return cfg, nil
		},
		InitClient: func(c *config.CLIConfig) (*client.GRPCClient, error) {
			return nil, nil
		},
	}
}

func TestNewRelationshipCommand(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	if cmd == nil {
		t.Fatal("NewRelationshipCommand returned nil")
	}

	if cmd.Use != "relationship" {
		t.Errorf("expected Use to be 'relationship', got %q", cmd.Use)
	}

	// Check aliases.
	expectedAliases := []string{"rel", "relations"}
	for _, expected := range expectedAliases {
		found := false
		for _, alias := range cmd.Aliases {
			if alias == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected alias %q not found", expected)
		}
	}

	// Check subcommands exist.
	subcommands := cmd.Commands()
	expectedSubcmds := []string{"list", "show", "search", "entity", "network", "conflict"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestNewRelationshipCommand_WithNilDeps(t *testing.T) {
	cmd := NewRelationshipCommand(nil)

	if cmd == nil {
		t.Fatal("NewRelationshipCommand with nil deps returned nil")
	}
}

func TestRelationshipEntitySubcommands(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	// Find the entity subcommand.
	entityCmd, _, err := cmd.Find([]string{"entity"})
	if err != nil {
		t.Fatalf("failed to find entity command: %v", err)
	}

	// Check entity subcommands.
	subcommands := entityCmd.Commands()
	expectedSubcmds := []string{"list", "show", "merge"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected entity subcommand %q not found", expected)
		}
	}
}

func TestRelationshipNetworkSubcommands(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	// Find the network subcommand.
	networkCmd, _, err := cmd.Find([]string{"network"})
	if err != nil {
		t.Fatalf("failed to find network command: %v", err)
	}

	// Check network subcommands.
	subcommands := networkCmd.Commands()
	expectedSubcmds := []string{"graph", "central", "clusters"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected network subcommand %q not found", expected)
		}
	}
}

func TestRelationshipConflictSubcommands(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	// Find the conflict subcommand.
	conflictCmd, _, err := cmd.Find([]string{"conflict"})
	if err != nil {
		t.Fatalf("failed to find conflict command: %v", err)
	}

	// Check conflict subcommands.
	subcommands := conflictCmd.Commands()
	expectedSubcmds := []string{"list", "show", "resolve"}

	for _, expected := range expectedSubcmds {
		found := false
		for _, sub := range subcommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected conflict subcommand %q not found", expected)
		}
	}
}

func TestGetMockRelationships(t *testing.T) {
	relationships := getMockRelationships(100, 0, "")

	if len(relationships) == 0 {
		t.Fatal("expected non-empty relationship list")
	}

	// Check that relationships have required fields.
	for _, r := range relationships {
		if r.ID == "" {
			t.Error("relationship ID should not be empty")
		}
		if r.SourceID == "" || r.TargetID == "" {
			t.Error("relationship source/target IDs should not be empty")
		}
		if r.SourceName == "" || r.TargetName == "" {
			t.Error("relationship source/target names should not be empty")
		}
		if r.Type == "" {
			t.Error("relationship type should not be empty")
		}
	}
}

func TestGetMockRelationships_WithConfidenceFilter(t *testing.T) {
	// Get all relationships.
	allRelationships := getMockRelationships(100, 0, "")

	// Get high-confidence relationships.
	highConfidence := getMockRelationships(100, 0.9, "")

	if len(highConfidence) >= len(allRelationships) {
		t.Error("high confidence filter should return fewer or equal results")
	}

	// Verify all returned relationships meet the threshold.
	for _, r := range highConfidence {
		if r.Confidence < 0.9 {
			t.Errorf("relationship %s has confidence %.2f, expected >= 0.9", r.ID, r.Confidence)
		}
	}
}

func TestGetMockRelationships_WithTypeFilter(t *testing.T) {
	// Get colleague relationships.
	colleagues := getMockRelationships(100, 0, "colleague")

	// Verify all returned relationships are of the right type.
	for _, r := range colleagues {
		if r.Type != RelationshipTypeColleague {
			t.Errorf("relationship %s has type %s, expected colleague", r.ID, r.Type)
		}
	}
}

func TestGetMockRelationships_WithLimit(t *testing.T) {
	limit := 2
	relationships := getMockRelationships(limit, 0, "")

	if len(relationships) > limit {
		t.Errorf("expected at most %d relationships, got %d", limit, len(relationships))
	}
}

func TestGetMockRelationshipByID(t *testing.T) {
	rel := getMockRelationshipByID("rel-001")

	if rel == nil {
		t.Fatal("expected relationship, got nil")
	}

	if rel.ID != "rel-001" {
		t.Errorf("expected ID 'rel-001', got %q", rel.ID)
	}
}

func TestGetMockRelationshipByID_Unknown(t *testing.T) {
	rel := getMockRelationshipByID("rel-unknown")

	// Should return a fallback relationship.
	if rel == nil {
		t.Fatal("expected fallback relationship, got nil")
	}

	if rel.ID != "rel-unknown" {
		t.Errorf("expected ID 'rel-unknown', got %q", rel.ID)
	}
}

func TestSearchMockRelationships(t *testing.T) {
	results := searchMockRelationships("Alice", 100, 0)

	if len(results) == 0 {
		t.Fatal("expected search results for 'Alice'")
	}

	// Verify all results contain Alice.
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.SourceName), "alice") &&
			!strings.Contains(strings.ToLower(r.TargetName), "alice") {
			t.Errorf("result %s doesn't match query 'Alice'", r.ID)
		}
	}
}

func TestSearchMockRelationships_NoResults(t *testing.T) {
	results := searchMockRelationships("nonexistent_xyz_123", 100, 0)

	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestGetMockEntities(t *testing.T) {
	entities := getMockEntities(100, 0, "")

	if len(entities) == 0 {
		t.Fatal("expected non-empty entity list")
	}

	// Check that entities have required fields.
	for _, e := range entities {
		if e.ID == "" {
			t.Error("entity ID should not be empty")
		}
		if e.Name == "" {
			t.Error("entity name should not be empty")
		}
		if e.Type == "" {
			t.Error("entity type should not be empty")
		}
	}
}

func TestGetMockEntities_WithTypeFilter(t *testing.T) {
	// Get person entities.
	persons := getMockEntities(100, 0, "person")

	// Verify all returned entities are persons.
	for _, e := range persons {
		if e.Type != EntityTypePerson {
			t.Errorf("entity %s has type %s, expected person", e.ID, e.Type)
		}
	}
}

func TestGetMockEntities_SortedByRelationCount(t *testing.T) {
	entities := getMockEntities(100, 0, "")

	// Verify entities are sorted by relation count (descending).
	for i := 1; i < len(entities); i++ {
		if entities[i].RelationCount > entities[i-1].RelationCount {
			t.Errorf("entities not sorted: %s has %d relations, but comes after %s with %d",
				entities[i].Name, entities[i].RelationCount,
				entities[i-1].Name, entities[i-1].RelationCount)
		}
	}
}

func TestGetMockEntityByID(t *testing.T) {
	entity := getMockEntityByID("ent-alice")

	if entity == nil {
		t.Fatal("expected entity, got nil")
	}

	if entity.ID != "ent-alice" {
		t.Errorf("expected ID 'ent-alice', got %q", entity.ID)
	}
}

func TestGetMockEntityByID_NotFound(t *testing.T) {
	entity := getMockEntityByID("ent-nonexistent")

	if entity != nil {
		t.Errorf("expected nil for nonexistent entity, got %+v", entity)
	}
}

func TestGetMockCentralEntities(t *testing.T) {
	entities := getMockCentralEntities(5)

	if len(entities) == 0 {
		t.Fatal("expected non-empty central entities list")
	}

	if len(entities) > 5 {
		t.Errorf("expected at most 5 entities, got %d", len(entities))
	}
}

func TestGetMockClusters(t *testing.T) {
	clusters := getMockClusters()

	if len(clusters) == 0 {
		t.Fatal("expected non-empty cluster list")
	}

	// Check cluster fields.
	for _, c := range clusters {
		if c.ID == "" {
			t.Error("cluster ID should not be empty")
		}
		if c.Name == "" {
			t.Error("cluster name should not be empty")
		}
		if c.EntityCount <= 0 {
			t.Error("cluster entity count should be positive")
		}
	}
}

func TestGetMockConflicts(t *testing.T) {
	conflicts := getMockConflicts(100)

	if len(conflicts) == 0 {
		t.Fatal("expected non-empty conflict list")
	}

	// Check conflict fields.
	for _, c := range conflicts {
		if c.ID == "" {
			t.Error("conflict ID should not be empty")
		}
		if c.Type == "" {
			t.Error("conflict type should not be empty")
		}
		if c.Description == "" {
			t.Error("conflict description should not be empty")
		}
		if c.Status == "" {
			t.Error("conflict status should not be empty")
		}
	}
}

func TestGetMockConflictByID(t *testing.T) {
	conflict := getMockConflictByID("conf-001")

	if conflict == nil {
		t.Fatal("expected conflict, got nil")
	}

	if conflict.ID != "conf-001" {
		t.Errorf("expected ID 'conf-001', got %q", conflict.ID)
	}
}

func TestGetMockConflictByID_NotFound(t *testing.T) {
	conflict := getMockConflictByID("conf-nonexistent")

	if conflict != nil {
		t.Errorf("expected nil for nonexistent conflict, got %+v", conflict)
	}
}

func TestEntity_JSONSerialization(t *testing.T) {
	entity := Entity{
		ID:            "ent-test",
		Name:          "Test Entity",
		Type:          EntityTypePerson,
		Aliases:       []string{"TE", "Test"},
		Confidence:    0.95,
		SourceCount:   10,
		FirstSeen:     time.Now().AddDate(0, -1, 0),
		LastSeen:      time.Now(),
		Metadata:      map[string]string{"key": "value"},
		RelationCount: 5,
	}

	data, err := json.Marshal(entity)
	if err != nil {
		t.Fatalf("failed to marshal Entity: %v", err)
	}

	var decoded Entity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Entity: %v", err)
	}

	if decoded.ID != entity.ID {
		t.Errorf("expected ID %q, got %q", entity.ID, decoded.ID)
	}
	if decoded.Name != entity.Name {
		t.Errorf("expected Name %q, got %q", entity.Name, decoded.Name)
	}
	if decoded.Type != entity.Type {
		t.Errorf("expected Type %q, got %q", entity.Type, decoded.Type)
	}
}

func TestEntity_YAMLSerialization(t *testing.T) {
	entity := Entity{
		ID:            "ent-test",
		Name:          "Test Entity",
		Type:          EntityTypePerson,
		Aliases:       []string{"TE"},
		Confidence:    0.95,
		RelationCount: 5,
	}

	data, err := yaml.Marshal(entity)
	if err != nil {
		t.Fatalf("failed to marshal Entity: %v", err)
	}

	var decoded Entity
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Entity: %v", err)
	}

	if decoded.ID != entity.ID {
		t.Errorf("expected ID %q, got %q", entity.ID, decoded.ID)
	}
}

func TestRelationship_JSONSerialization(t *testing.T) {
	rel := Relationship{
		ID:          "rel-test",
		SourceID:    "ent-1",
		SourceName:  "Entity 1",
		TargetID:    "ent-2",
		TargetName:  "Entity 2",
		Type:        RelationshipTypeColleague,
		Confidence:  0.9,
		Weight:      0.8,
		Evidence:    []string{"Evidence 1"},
		FirstSeen:   time.Now().AddDate(0, -1, 0),
		LastSeen:    time.Now(),
		SourceCount: 5,
	}

	data, err := json.Marshal(rel)
	if err != nil {
		t.Fatalf("failed to marshal Relationship: %v", err)
	}

	var decoded Relationship
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Relationship: %v", err)
	}

	if decoded.ID != rel.ID {
		t.Errorf("expected ID %q, got %q", rel.ID, decoded.ID)
	}
	if decoded.Type != rel.Type {
		t.Errorf("expected Type %q, got %q", rel.Type, decoded.Type)
	}
}

func TestRelationshipConflict_JSONSerialization(t *testing.T) {
	conflict := RelationshipConflict{
		ID:              "conf-test",
		Type:            "duplicate",
		Description:     "Test conflict",
		SuggestedAction: "Resolve it",
		CreatedAt:       time.Now(),
		Status:          "pending",
	}

	data, err := json.Marshal(conflict)
	if err != nil {
		t.Fatalf("failed to marshal RelationshipConflict: %v", err)
	}

	var decoded RelationshipConflict
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal RelationshipConflict: %v", err)
	}

	if decoded.ID != conflict.ID {
		t.Errorf("expected ID %q, got %q", conflict.ID, decoded.ID)
	}
}

func TestNetworkCluster_JSONSerialization(t *testing.T) {
	cluster := NetworkCluster{
		ID:          "cluster-test",
		Name:        "Test Cluster",
		EntityCount: 10,
		TopEntities: []Entity{{ID: "ent-1", Name: "Entity 1"}},
		Density:     0.5,
	}

	data, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("failed to marshal NetworkCluster: %v", err)
	}

	var decoded NetworkCluster
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal NetworkCluster: %v", err)
	}

	if decoded.ID != cluster.ID {
		t.Errorf("expected ID %q, got %q", cluster.ID, decoded.ID)
	}
}

func TestGetConfidenceColor(t *testing.T) {
	tests := []struct {
		confidence float64
		wantColor  string
	}{
		{0.9, "\033[32m"},  // Green for high confidence.
		{0.8, "\033[32m"},  // Green for high confidence.
		{0.7, "\033[33m"},  // Yellow for medium.
		{0.5, "\033[33m"},  // Yellow for medium.
		{0.3, "\033[31m"},  // Red for low.
		{0.0, "\033[31m"},  // Red for low.
	}

	for _, tt := range tests {
		got := getConfidenceColor(tt.confidence)
		if got != tt.wantColor {
			t.Errorf("getConfidenceColor(%.1f) = %q, want %q", tt.confidence, got, tt.wantColor)
		}
	}
}

func TestGetEntityTypeColor(t *testing.T) {
	tests := []struct {
		entityType EntityType
		wantColor  string
	}{
		{EntityTypePerson, "\033[36m"},       // Cyan.
		{EntityTypeOrganization, "\033[35m"}, // Magenta.
		{EntityTypeTopic, "\033[34m"},        // Blue.
		{EntityTypeProject, "\033[33m"},      // Yellow.
		{EntityTypeLocation, "\033[32m"},     // Green.
		{EntityType("unknown"), ""},          // No color for unknown.
	}

	for _, tt := range tests {
		got := getEntityTypeColor(tt.entityType)
		if got != tt.wantColor {
			t.Errorf("getEntityTypeColor(%q) = %q, want %q", tt.entityType, got, tt.wantColor)
		}
	}
}

func TestRunRelationshipList(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipTenant = ""
	relationshipFormat = ""
	relationshipLimit = 20
	relationshipConfidenceMin = 0
	relationshipType = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runRelationshipList(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runRelationshipList failed: %v", err)
	}
}

func TestRunRelationshipShow(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runRelationshipShow(context.Background(), deps, "rel-001")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runRelationshipShow failed: %v", err)
	}
}

func TestRunRelationshipSearch(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""
	relationshipLimit = 20
	relationshipConfidenceMin = 0

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runRelationshipSearch(context.Background(), deps, "Alice")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runRelationshipSearch failed: %v", err)
	}
}

func TestRunEntityList(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""
	relationshipLimit = 20
	relationshipConfidenceMin = 0
	relationshipEntityType = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runEntityList(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEntityList failed: %v", err)
	}
}

func TestRunEntityShow(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runEntityShow(context.Background(), deps, "ent-alice")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEntityShow failed: %v", err)
	}
}

func TestRunEntityShow_NotFound(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	err := runEntityShow(context.Background(), deps, "ent-nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent entity")
	}

	if !strings.Contains(err.Error(), "entity not found") {
		t.Errorf("expected 'entity not found' error, got: %v", err)
	}
}

func TestRunEntityMerge(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runEntityMerge(context.Background(), deps, "ent-1", "ent-2")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEntityMerge failed: %v", err)
	}
}

func TestRunNetworkGraph(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runNetworkGraph(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runNetworkGraph failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check that output contains expected sections.
	if !strings.Contains(output, "Nodes") {
		t.Error("output should contain 'Nodes'")
	}
	if !strings.Contains(output, "Edges") {
		t.Error("output should contain 'Edges'")
	}
}

func TestRunNetworkCentral(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""
	relationshipLimit = 10

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runNetworkCentral(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runNetworkCentral failed: %v", err)
	}
}

func TestRunNetworkClusters(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runNetworkClusters(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runNetworkClusters failed: %v", err)
	}
}

func TestRunConflictList(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""
	relationshipLimit = 20

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runConflictList(context.Background(), deps)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConflictList failed: %v", err)
	}
}

func TestRunConflictShow(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runConflictShow(context.Background(), deps, "conf-001")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConflictShow failed: %v", err)
	}
}

func TestRunConflictShow_NotFound(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Reset global flags.
	relationshipFormat = ""

	err := runConflictShow(context.Background(), deps, "conf-nonexistent")

	if err == nil {
		t.Error("expected error for nonexistent conflict")
	}

	if !strings.Contains(err.Error(), "conflict not found") {
		t.Errorf("expected 'conflict not found' error, got: %v", err)
	}
}

func TestRunConflictResolve(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	// Capture stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := runConflictResolve(context.Background(), deps, "conf-001", ConflictStrategyMerge)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runConflictResolve failed: %v", err)
	}
}

func TestRunConflictResolve_InvalidStrategy(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)

	err := runConflictResolve(context.Background(), deps, "conf-001", ConflictResolutionStrategy("invalid"))

	if err == nil {
		t.Error("expected error for invalid strategy")
	}

	if !strings.Contains(err.Error(), "invalid resolution strategy") {
		t.Errorf("expected 'invalid resolution strategy' error, got: %v", err)
	}
}

func TestOutputRelationships_JSON(t *testing.T) {
	relationships := getMockRelationships(2, 0, "")

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputRelationships(config.OutputFormatJSON, relationships)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputRelationships JSON failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid JSON.
	var decoded []Relationship
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if len(decoded) != len(relationships) {
		t.Errorf("expected %d relationships in JSON output, got %d", len(relationships), len(decoded))
	}
}

func TestOutputRelationships_YAML(t *testing.T) {
	relationships := getMockRelationships(2, 0, "")

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputRelationships(config.OutputFormatYAML, relationships)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputRelationships YAML failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid YAML.
	var decoded []Relationship
	if err := yaml.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
}

func TestOutputRelationships_Text(t *testing.T) {
	relationships := getMockRelationships(2, 0, "")

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputRelationships(config.OutputFormatText, relationships)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputRelationships text failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check that output contains expected content.
	if !strings.Contains(output, "Relationships") {
		t.Error("output should contain 'Relationships'")
	}
}

func TestOutputRelationships_Empty(t *testing.T) {
	var relationships []Relationship

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputRelationships(config.OutputFormatText, relationships)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputRelationships empty failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No relationships found") {
		t.Error("output should indicate no relationships found")
	}
}

func TestOutputEntities_JSON(t *testing.T) {
	entities := getMockEntities(2, 0, "")

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEntities(config.OutputFormatJSON, entities)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputEntities JSON failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify it's valid JSON.
	var decoded []Entity
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestOutputConflicts_Empty(t *testing.T) {
	var conflicts []RelationshipConflict

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputConflicts(config.OutputFormatText, conflicts)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("outputConflicts empty failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No conflicts detected") {
		t.Error("output should indicate no conflicts detected")
	}
}

func TestRelationshipCommand_ListAlias(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	// Find the list command.
	listCmd, _, err := cmd.Find([]string{"list"})
	if err != nil {
		t.Fatalf("failed to find list command: %v", err)
	}

	// Check for 'ls' alias.
	hasAlias := false
	for _, a := range listCmd.Aliases {
		if a == "ls" {
			hasAlias = true
			break
		}
	}
	if !hasAlias {
		t.Error("list command should have 'ls' alias")
	}
}

func TestRelationshipCommand_NetworkAliases(t *testing.T) {
	cfg := mockConfig()
	deps := createRelationshipTestDeps(cfg)
	cmd := NewRelationshipCommand(deps)

	// Find the network command.
	networkCmd, _, err := cmd.Find([]string{"network"})
	if err != nil {
		t.Fatalf("failed to find network command: %v", err)
	}

	// Check for 'net' alias.
	hasAlias := false
	for _, a := range networkCmd.Aliases {
		if a == "net" {
			hasAlias = true
			break
		}
	}
	if !hasAlias {
		t.Error("network command should have 'net' alias")
	}
}

func TestEntityTypeConstants(t *testing.T) {
	types := []EntityType{
		EntityTypePerson,
		EntityTypeOrganization,
		EntityTypeTopic,
		EntityTypeProject,
		EntityTypeLocation,
	}

	for _, et := range types {
		if string(et) == "" {
			t.Error("entity type constant should not be empty")
		}
	}
}

func TestRelationshipTypeConstants(t *testing.T) {
	types := []RelationshipType{
		RelationshipTypeColleague,
		RelationshipTypeReportsTo,
		RelationshipTypeMemberOf,
		RelationshipTypeWorksOn,
		RelationshipTypeDiscusses,
		RelationshipTypeMentions,
		RelationshipTypeLocatedAt,
		RelationshipTypeRelatedTo,
	}

	for _, rt := range types {
		if string(rt) == "" {
			t.Error("relationship type constant should not be empty")
		}
	}
}

func TestConflictStrategyConstants(t *testing.T) {
	strategies := []ConflictResolutionStrategy{
		ConflictStrategyKeepLatest,
		ConflictStrategyKeepFirst,
		ConflictStrategyMerge,
		ConflictStrategyManual,
	}

	for _, s := range strategies {
		if string(s) == "" {
			t.Error("conflict strategy constant should not be empty")
		}
	}
}
