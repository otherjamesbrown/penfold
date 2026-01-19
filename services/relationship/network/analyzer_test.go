package network

import (
	"context"
	"testing"
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockRelationshipProvider implements RelationshipProvider for testing.
type mockRelationshipProvider struct {
	relationships []*relationshipv1.Relationship
}

func (m *mockRelationshipProvider) ListRelationships(_ context.Context, _ string, _ *AnalysisFilter) ([]*relationshipv1.Relationship, error) {
	return m.relationships, nil
}

// createTestLogger creates a logger for testing.
func createTestLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// createTestRelationships creates a set of test relationships forming a network.
func createTestRelationships() []*relationshipv1.Relationship {
	now := timestamppb.Now()

	return []*relationshipv1.Relationship{
		{
			Id: "rel-1",
			SourceEntity: &relationshipv1.Entity{
				Id:   "alice",
				Name: "Alice",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "bob",
				Name: "Bob",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS,
			Confidence:       0.9,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED,
			CreatedAt:        now,
		},
		{
			Id: "rel-2",
			SourceEntity: &relationshipv1.Entity{
				Id:   "alice",
				Name: "Alice",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "charlie",
				Name: "Charlie",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS,
			Confidence:       0.8,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED,
			CreatedAt:        now,
		},
		{
			Id: "rel-3",
			SourceEntity: &relationshipv1.Entity{
				Id:   "bob",
				Name: "Bob",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "charlie",
				Name: "Charlie",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH,
			Confidence:       0.85,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED,
			CreatedAt:        now,
		},
		{
			Id: "rel-4",
			SourceEntity: &relationshipv1.Entity{
				Id:   "alice",
				Name: "Alice",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "acme",
				Name: "Acme Corp",
				Type: relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT,
			Confidence:       0.95,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED,
			CreatedAt:        now,
		},
		{
			Id: "rel-5",
			SourceEntity: &relationshipv1.Entity{
				Id:   "bob",
				Name: "Bob",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "acme",
				Name: "Acme Corp",
				Type: relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT,
			Confidence:       0.95,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED,
			CreatedAt:        now,
		},
		{
			Id: "rel-6",
			SourceEntity: &relationshipv1.Entity{
				Id:   "david",
				Name: "David",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			TargetEntity: &relationshipv1.Entity{
				Id:   "eve",
				Name: "Eve",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
			RelationshipType: relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS,
			Confidence:       0.7,
			Status:           relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED,
			CreatedAt:        now,
		},
	}
}

// TestGraph tests the Graph data structure.
func TestGraph(t *testing.T) {
	t.Run("NewGraph", func(t *testing.T) {
		g := NewGraph("tenant-1")
		if g == nil {
			t.Fatal("NewGraph returned nil")
		}
		if g.TenantID != "tenant-1" {
			t.Errorf("Expected TenantID 'tenant-1', got '%s'", g.TenantID)
		}
		if g.NodeCount() != 0 {
			t.Errorf("Expected 0 nodes, got %d", g.NodeCount())
		}
		if g.EdgeCount() != 0 {
			t.Errorf("Expected 0 edges, got %d", g.EdgeCount())
		}
	})

	t.Run("AddNode", func(t *testing.T) {
		g := NewGraph("tenant-1")
		node := &Node{
			ID: "node-1",
			Entity: &relationshipv1.Entity{
				Id:   "node-1",
				Name: "Test Node",
				Type: relationshipv1.EntityType_ENTITY_TYPE_PERSON,
			},
		}

		if !g.AddNode(node) {
			t.Error("AddNode returned false for new node")
		}
		if g.AddNode(node) {
			t.Error("AddNode returned true for duplicate node")
		}
		if g.NodeCount() != 1 {
			t.Errorf("Expected 1 node, got %d", g.NodeCount())
		}
	})

	t.Run("AddEdge", func(t *testing.T) {
		g := NewGraph("tenant-1")
		node1 := &Node{ID: "node-1", Entity: &relationshipv1.Entity{Id: "node-1", Name: "Node 1"}}
		node2 := &Node{ID: "node-2", Entity: &relationshipv1.Entity{Id: "node-2", Name: "Node 2"}}
		g.AddNode(node1)
		g.AddNode(node2)

		edge := &Edge{
			ID:       "edge-1",
			SourceID: "node-1",
			TargetID: "node-2",
			Weight:   1.0,
		}

		if !g.AddEdge(edge) {
			t.Error("AddEdge returned false for valid edge")
		}
		if g.AddEdge(edge) {
			t.Error("AddEdge returned true for duplicate edge")
		}
		if g.EdgeCount() != 1 {
			t.Errorf("Expected 1 edge, got %d", g.EdgeCount())
		}

		// Check degrees were updated
		if node1.OutDegree != 1 {
			t.Errorf("Expected source OutDegree 1, got %d", node1.OutDegree)
		}
		if node2.InDegree != 1 {
			t.Errorf("Expected target InDegree 1, got %d", node2.InDegree)
		}
	})

	t.Run("AddEdge_InvalidNodes", func(t *testing.T) {
		g := NewGraph("tenant-1")
		edge := &Edge{
			ID:       "edge-1",
			SourceID: "nonexistent-1",
			TargetID: "nonexistent-2",
			Weight:   1.0,
		}

		if g.AddEdge(edge) {
			t.Error("AddEdge returned true for edge with nonexistent nodes")
		}
	})

	t.Run("GetNeighbors", func(t *testing.T) {
		g := NewGraph("tenant-1")
		node1 := &Node{ID: "node-1", Entity: &relationshipv1.Entity{Id: "node-1"}}
		node2 := &Node{ID: "node-2", Entity: &relationshipv1.Entity{Id: "node-2"}}
		node3 := &Node{ID: "node-3", Entity: &relationshipv1.Entity{Id: "node-3"}}
		g.AddNode(node1)
		g.AddNode(node2)
		g.AddNode(node3)
		g.AddEdge(&Edge{ID: "e1", SourceID: "node-1", TargetID: "node-2", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e2", SourceID: "node-1", TargetID: "node-3", Weight: 1.0})

		neighbors := g.GetNeighbors("node-1")
		if len(neighbors) != 2 {
			t.Errorf("Expected 2 neighbors, got %d", len(neighbors))
		}

		incoming := g.GetIncomingNeighbors("node-2")
		if len(incoming) != 1 {
			t.Errorf("Expected 1 incoming neighbor, got %d", len(incoming))
		}
	})

	t.Run("RemoveNode", func(t *testing.T) {
		g := NewGraph("tenant-1")
		node1 := &Node{ID: "node-1", Entity: &relationshipv1.Entity{Id: "node-1"}}
		node2 := &Node{ID: "node-2", Entity: &relationshipv1.Entity{Id: "node-2"}}
		g.AddNode(node1)
		g.AddNode(node2)
		g.AddEdge(&Edge{ID: "e1", SourceID: "node-1", TargetID: "node-2", Weight: 1.0})

		if !g.RemoveNode("node-1") {
			t.Error("RemoveNode returned false for existing node")
		}
		if g.HasNode("node-1") {
			t.Error("Node still exists after removal")
		}
		if g.HasEdge("e1") {
			t.Error("Edge still exists after node removal")
		}
		// Check degree was updated on remaining node
		if node2.InDegree != 0 {
			t.Errorf("Expected InDegree 0 after source removal, got %d", node2.InDegree)
		}
	})

	t.Run("RemoveEdge", func(t *testing.T) {
		g := NewGraph("tenant-1")
		node1 := &Node{ID: "node-1", Entity: &relationshipv1.Entity{Id: "node-1"}}
		node2 := &Node{ID: "node-2", Entity: &relationshipv1.Entity{Id: "node-2"}}
		g.AddNode(node1)
		g.AddNode(node2)
		g.AddEdge(&Edge{ID: "e1", SourceID: "node-1", TargetID: "node-2", Weight: 1.0})

		if !g.RemoveEdge("e1") {
			t.Error("RemoveEdge returned false for existing edge")
		}
		if g.HasEdge("e1") {
			t.Error("Edge still exists after removal")
		}
		if node1.OutDegree != 0 {
			t.Errorf("Expected OutDegree 0 after edge removal, got %d", node1.OutDegree)
		}
	})

	t.Run("Density", func(t *testing.T) {
		g := NewGraph("tenant-1")
		// Empty graph
		if g.Density() != 0 {
			t.Errorf("Expected density 0 for empty graph, got %f", g.Density())
		}

		// Single node
		g.AddNode(&Node{ID: "n1", Entity: &relationshipv1.Entity{Id: "n1"}})
		if g.Density() != 0 {
			t.Errorf("Expected density 0 for single node, got %f", g.Density())
		}

		// Complete graph with 3 nodes (6 directed edges possible)
		g.AddNode(&Node{ID: "n2", Entity: &relationshipv1.Entity{Id: "n2"}})
		g.AddNode(&Node{ID: "n3", Entity: &relationshipv1.Entity{Id: "n3"}})
		g.AddEdge(&Edge{ID: "e1", SourceID: "n1", TargetID: "n2", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e2", SourceID: "n2", TargetID: "n1", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e3", SourceID: "n1", TargetID: "n3", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e4", SourceID: "n3", TargetID: "n1", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e5", SourceID: "n2", TargetID: "n3", Weight: 1.0})
		g.AddEdge(&Edge{ID: "e6", SourceID: "n3", TargetID: "n2", Weight: 1.0})

		density := g.Density()
		if density != 1.0 {
			t.Errorf("Expected density 1.0 for complete graph, got %f", density)
		}
	})

	t.Run("Clone", func(t *testing.T) {
		g := NewGraph("tenant-1")
		g.AddNode(&Node{ID: "n1", Entity: &relationshipv1.Entity{Id: "n1"}, Metadata: map[string]string{"key": "value"}})
		g.AddNode(&Node{ID: "n2", Entity: &relationshipv1.Entity{Id: "n2"}})
		g.AddEdge(&Edge{ID: "e1", SourceID: "n1", TargetID: "n2", Weight: 1.5})

		clone := g.Clone()

		if clone.TenantID != g.TenantID {
			t.Error("Clone has different TenantID")
		}
		if clone.NodeCount() != g.NodeCount() {
			t.Error("Clone has different node count")
		}
		if clone.EdgeCount() != g.EdgeCount() {
			t.Error("Clone has different edge count")
		}

		// Verify it's a deep copy
		g.RemoveNode("n1")
		if !clone.HasNode("n1") {
			t.Error("Clone was affected by original modification")
		}
	})
}

// TestNetworkAnalyzer tests the NetworkAnalyzer.
func TestNetworkAnalyzer(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()

	t.Run("NewNetworkAnalyzer", func(t *testing.T) {
		analyzer := NewNetworkAnalyzer(provider, logger, nil)
		if analyzer == nil {
			t.Fatal("NewNetworkAnalyzer returned nil")
		}
	})

	t.Run("BuildGraph", func(t *testing.T) {
		analyzer := NewNetworkAnalyzer(provider, logger, nil)
		ctx := context.Background()

		graph, err := analyzer.BuildGraph(ctx, "tenant-1", nil)
		if err != nil {
			t.Fatalf("BuildGraph failed: %v", err)
		}

		// 6 relationships should create nodes for: alice, bob, charlie, acme, david, eve
		if graph.NodeCount() != 6 {
			t.Errorf("Expected 6 nodes, got %d", graph.NodeCount())
		}
		if graph.EdgeCount() != 6 {
			t.Errorf("Expected 6 edges, got %d", graph.EdgeCount())
		}
	})

	t.Run("BuildGraph_WithFilter", func(t *testing.T) {
		analyzer := NewNetworkAnalyzer(provider, logger, nil)
		ctx := context.Background()

		filter := &AnalysisFilter{
			RelationshipTypes: []relationshipv1.RelationshipType{
				relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS,
			},
		}

		graph, err := analyzer.BuildGraph(ctx, "tenant-1", filter)
		if err != nil {
			t.Fatalf("BuildGraph with filter failed: %v", err)
		}

		// Only KNOWS relationships: alice->bob, alice->charlie, david->eve
		if graph.EdgeCount() != 3 {
			t.Errorf("Expected 3 edges after filtering, got %d", graph.EdgeCount())
		}
	})

	t.Run("BuildGraph_ConfirmedOnly", func(t *testing.T) {
		analyzer := NewNetworkAnalyzer(provider, logger, nil)
		ctx := context.Background()

		filter := &AnalysisFilter{
			ConfirmedOnly: true,
		}

		graph, err := analyzer.BuildGraph(ctx, "tenant-1", filter)
		if err != nil {
			t.Fatalf("BuildGraph with confirmed filter failed: %v", err)
		}

		// david->eve is DISCOVERED, not CONFIRMED
		if graph.EdgeCount() != 5 {
			t.Errorf("Expected 5 confirmed edges, got %d", graph.EdgeCount())
		}
	})

	t.Run("BuildGraph_Caching", func(t *testing.T) {
		analyzer := NewNetworkAnalyzer(provider, logger, &AnalyzerConfig{
			CacheTTL:     5 * time.Minute,
			MaxCacheSize: 10,
		})
		ctx := context.Background()

		// First call should build the graph
		graph1, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Second call should return cached result
		graph2, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Both should have same structure
		if graph1.NodeCount() != graph2.NodeCount() {
			t.Error("Cached graph has different node count")
		}

		// But should be different instances
		graph1.AddNode(&Node{ID: "new-node", Entity: &relationshipv1.Entity{Id: "new-node"}})
		if graph2.HasNode("new-node") {
			t.Error("Cache should return cloned graphs")
		}
	})
}

// TestFindClusters tests the cluster detection algorithm.
func TestFindClusters(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, nil)
	ctx := context.Background()

	t.Run("FindClusters_Basic", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		clusters, err := analyzer.FindClusters(graph)
		if err != nil {
			t.Fatalf("FindClusters failed: %v", err)
		}

		if len(clusters) == 0 {
			t.Error("Expected at least 1 cluster")
		}

		// Verify all nodes are assigned to clusters
		nodeCount := 0
		for _, cluster := range clusters {
			nodeCount += cluster.Size
		}
		if nodeCount != graph.NodeCount() {
			t.Errorf("Cluster node count (%d) doesn't match graph node count (%d)", nodeCount, graph.NodeCount())
		}

		// Verify clusters are sorted by size
		for i := 1; i < len(clusters); i++ {
			if clusters[i].Size > clusters[i-1].Size {
				t.Error("Clusters are not sorted by size descending")
			}
		}
	})

	t.Run("FindClusters_EmptyGraph", func(t *testing.T) {
		emptyGraph := NewGraph("tenant-1")
		clusters, err := analyzer.FindClusters(emptyGraph)
		if err != nil {
			t.Fatalf("FindClusters failed on empty graph: %v", err)
		}
		if len(clusters) != 0 {
			t.Errorf("Expected 0 clusters for empty graph, got %d", len(clusters))
		}
	})

	t.Run("FindClusters_NilGraph", func(t *testing.T) {
		_, err := analyzer.FindClusters(nil)
		if err == nil {
			t.Error("Expected error for nil graph")
		}
	})

	t.Run("FindClusters_Caching", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		clusters1, _ := analyzer.FindClusters(graph)
		clusters2, _ := analyzer.FindClusters(graph)

		if len(clusters1) != len(clusters2) {
			t.Error("Cached clusters have different count")
		}
	})
}

// TestCalculateCentrality tests the centrality calculation algorithms.
func TestCalculateCentrality(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, nil)
	ctx := context.Background()

	t.Run("CalculateCentrality_Basic", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		scores, err := analyzer.CalculateCentrality(graph)
		if err != nil {
			t.Fatalf("CalculateCentrality failed: %v", err)
		}

		if len(scores.Degree) != graph.NodeCount() {
			t.Errorf("Expected %d degree scores, got %d", graph.NodeCount(), len(scores.Degree))
		}
		if len(scores.Betweenness) != graph.NodeCount() {
			t.Errorf("Expected %d betweenness scores, got %d", graph.NodeCount(), len(scores.Betweenness))
		}
		if len(scores.Closeness) != graph.NodeCount() {
			t.Errorf("Expected %d closeness scores, got %d", graph.NodeCount(), len(scores.Closeness))
		}

		// Alice should have highest degree (connected to bob, charlie, acme)
		aliceDegree := scores.Degree["alice"]
		if aliceDegree <= 0 {
			t.Error("Alice should have positive degree centrality")
		}

		// All degree scores should be between 0 and 1
		for nodeID, score := range scores.Degree {
			if score < 0 || score > 1 {
				t.Errorf("Degree score for %s out of range: %f", nodeID, score)
			}
		}
	})

	t.Run("CalculateCentrality_EmptyGraph", func(t *testing.T) {
		emptyGraph := NewGraph("tenant-1")
		scores, err := analyzer.CalculateCentrality(emptyGraph)
		if err != nil {
			t.Fatalf("CalculateCentrality failed on empty graph: %v", err)
		}
		if len(scores.Degree) != 0 {
			t.Errorf("Expected 0 scores for empty graph, got %d", len(scores.Degree))
		}
	})

	t.Run("CalculateCentrality_NilGraph", func(t *testing.T) {
		_, err := analyzer.CalculateCentrality(nil)
		if err == nil {
			t.Error("Expected error for nil graph")
		}
	})

	t.Run("CalculateCentrality_Caching", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		scores1, _ := analyzer.CalculateCentrality(graph)
		scores2, _ := analyzer.CalculateCentrality(graph)

		if len(scores1.Degree) != len(scores2.Degree) {
			t.Error("Cached centrality scores differ")
		}
	})
}

// TestFindShortestPath tests the shortest path algorithm.
func TestFindShortestPath(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, nil)
	ctx := context.Background()

	t.Run("FindShortestPath_Direct", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Alice directly connects to Bob
		path, err := analyzer.FindShortestPath(graph, "alice", "bob")
		if err != nil {
			t.Fatalf("FindShortestPath failed: %v", err)
		}

		if len(path.Nodes) != 2 {
			t.Errorf("Expected path of length 2, got %d", len(path.Nodes))
		}
		if path.Nodes[0] != "alice" || path.Nodes[1] != "bob" {
			t.Error("Path nodes are incorrect")
		}
		if path.Length != 1 {
			t.Errorf("Expected path length 1, got %d", path.Length)
		}
	})

	t.Run("FindShortestPath_Indirect", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Bob connects to Acme through shared employer relationship
		path, err := analyzer.FindShortestPath(graph, "bob", "acme")
		if err != nil {
			t.Fatalf("FindShortestPath failed: %v", err)
		}

		if path.Length < 1 {
			t.Errorf("Expected path length >= 1, got %d", path.Length)
		}
	})

	t.Run("FindShortestPath_SameNode", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		path, err := analyzer.FindShortestPath(graph, "alice", "alice")
		if err != nil {
			t.Fatalf("FindShortestPath to same node failed: %v", err)
		}

		if path.Length != 0 {
			t.Errorf("Expected path length 0 for same node, got %d", path.Length)
		}
		if len(path.Nodes) != 1 {
			t.Errorf("Expected 1 node in path, got %d", len(path.Nodes))
		}
	})

	t.Run("FindShortestPath_NoPath", func(t *testing.T) {
		// Create a disconnected graph
		disconnectedProvider := &mockRelationshipProvider{
			relationships: []*relationshipv1.Relationship{
				{
					Id:           "r1",
					SourceEntity: &relationshipv1.Entity{Id: "a", Name: "A"},
					TargetEntity: &relationshipv1.Entity{Id: "b", Name: "B"},
					Confidence:   0.9,
				},
				{
					Id:           "r2",
					SourceEntity: &relationshipv1.Entity{Id: "c", Name: "C"},
					TargetEntity: &relationshipv1.Entity{Id: "d", Name: "D"},
					Confidence:   0.9,
				},
			},
		}
		disconnectedAnalyzer := NewNetworkAnalyzer(disconnectedProvider, logger, nil)
		graph, _ := disconnectedAnalyzer.BuildGraph(ctx, "tenant-1", nil)

		_, err := disconnectedAnalyzer.FindShortestPath(graph, "a", "c")
		if err == nil {
			t.Error("Expected error for disconnected nodes")
		}
	})

	t.Run("FindShortestPath_NonexistentNode", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

		_, err := analyzer.FindShortestPath(graph, "nonexistent", "alice")
		if err == nil {
			t.Error("Expected error for nonexistent source node")
		}

		_, err = analyzer.FindShortestPath(graph, "alice", "nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent target node")
		}
	})

	t.Run("FindShortestPath_NilGraph", func(t *testing.T) {
		_, err := analyzer.FindShortestPath(nil, "alice", "bob")
		if err == nil {
			t.Error("Expected error for nil graph")
		}
	})
}

// TestExportVisualization tests the visualization export.
func TestExportVisualization(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, nil)
	ctx := context.Background()

	t.Run("ExportVisualization_Basic", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		clusters, _ := analyzer.FindClusters(graph)
		centrality, _ := analyzer.CalculateCentrality(graph)

		vizData, err := analyzer.ExportVisualization(graph, clusters, centrality)
		if err != nil {
			t.Fatalf("ExportVisualization failed: %v", err)
		}

		if len(vizData.Nodes) != graph.NodeCount() {
			t.Errorf("Expected %d viz nodes, got %d", graph.NodeCount(), len(vizData.Nodes))
		}
		if len(vizData.Edges) != graph.EdgeCount() {
			t.Errorf("Expected %d viz edges, got %d", graph.EdgeCount(), len(vizData.Edges))
		}

		// Check statistics
		if vizData.Statistics.TotalNodes != graph.NodeCount() {
			t.Error("Statistics node count mismatch")
		}
		if vizData.Statistics.TotalEdges != graph.EdgeCount() {
			t.Error("Statistics edge count mismatch")
		}

		// Check node properties
		for _, node := range vizData.Nodes {
			if node.ID == "" {
				t.Error("Node missing ID")
			}
			if node.Color == "" {
				t.Error("Node missing color")
			}
			if node.Size <= 0 {
				t.Error("Node has invalid size")
			}
		}

		// Check edge properties
		for _, edge := range vizData.Edges {
			if edge.ID == "" {
				t.Error("Edge missing ID")
			}
			if edge.Source == "" || edge.Target == "" {
				t.Error("Edge missing source or target")
			}
		}
	})

	t.Run("ExportVisualization_WithClusters", func(t *testing.T) {
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		clusters, _ := analyzer.FindClusters(graph)

		vizData, err := analyzer.ExportVisualization(graph, clusters, nil)
		if err != nil {
			t.Fatalf("ExportVisualization failed: %v", err)
		}

		if len(vizData.Clusters) != len(clusters) {
			t.Errorf("Expected %d clusters, got %d", len(clusters), len(vizData.Clusters))
		}

		for _, cluster := range vizData.Clusters {
			if cluster.Color == "" {
				t.Error("Cluster missing color")
			}
		}
	})

	t.Run("ExportVisualization_NilGraph", func(t *testing.T) {
		_, err := analyzer.ExportVisualization(nil, nil, nil)
		if err == nil {
			t.Error("Expected error for nil graph")
		}
	})
}

// TestCacheInvalidation tests cache invalidation.
func TestCacheInvalidation(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, &AnalyzerConfig{
		CacheTTL:     5 * time.Minute,
		MaxCacheSize: 10,
	})
	ctx := context.Background()

	t.Run("InvalidateCache", func(t *testing.T) {
		// Build and cache a graph
		_, _ = analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Invalidate cache
		analyzer.InvalidateCache("tenant-1")

		// This should not panic and should rebuild the graph
		graph, err := analyzer.BuildGraph(ctx, "tenant-1", nil)
		if err != nil {
			t.Fatalf("BuildGraph after invalidation failed: %v", err)
		}
		if graph.NodeCount() != 6 {
			t.Error("Graph not rebuilt correctly after invalidation")
		}
	})

	t.Run("InvalidateCacheForRelationship", func(t *testing.T) {
		// Build and cache a graph
		_, _ = analyzer.BuildGraph(ctx, "tenant-1", nil)

		// Invalidate cache for a specific relationship
		analyzer.InvalidateCacheForRelationship("tenant-1", "rel-1")

		// Should rebuild correctly
		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)
		if graph.NodeCount() != 6 {
			t.Error("Graph not rebuilt correctly after relationship invalidation")
		}
	})
}

// TestAnalysisFilter tests the filter matching logic.
func TestAnalysisFilter(t *testing.T) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, nil)
	ctx := context.Background()

	t.Run("Filter_MinConfidence", func(t *testing.T) {
		filter := &AnalysisFilter{
			MinConfidence: 0.85,
		}

		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", filter)

		// alice->bob (0.9), bob->charlie (0.85), alice->acme (0.95), bob->acme (0.95) meet threshold >= 0.85
		if graph.EdgeCount() != 4 {
			t.Errorf("Expected 4 edges with confidence >= 0.85, got %d", graph.EdgeCount())
		}
	})

	t.Run("Filter_EntityTypes", func(t *testing.T) {
		filter := &AnalysisFilter{
			EntityTypes: []relationshipv1.EntityType{
				relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION,
			},
		}

		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", filter)

		// Only relationships involving organizations: alice->acme, bob->acme
		if graph.EdgeCount() != 2 {
			t.Errorf("Expected 2 edges with organizations, got %d", graph.EdgeCount())
		}
	})

	t.Run("Filter_TimeRange", func(t *testing.T) {
		now := time.Now()
		past := now.Add(-1 * time.Hour)
		future := now.Add(1 * time.Hour)

		filter := &AnalysisFilter{
			TimeRangeStart: &past,
			TimeRangeEnd:   &future,
		}

		graph, _ := analyzer.BuildGraph(ctx, "tenant-1", filter)

		// All relationships should be within range
		if graph.EdgeCount() != 6 {
			t.Errorf("Expected 6 edges within time range, got %d", graph.EdgeCount())
		}
	})
}

// BenchmarkBuildGraph benchmarks graph building.
func BenchmarkBuildGraph(b *testing.B) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, &AnalyzerConfig{
		CacheTTL:     0, // Disable caching for benchmark
		MaxCacheSize: 0,
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.InvalidateCache("tenant-1")
		_, _ = analyzer.BuildGraph(ctx, "tenant-1", nil)
	}
}

// BenchmarkFindClusters benchmarks cluster detection.
func BenchmarkFindClusters(b *testing.B) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, &AnalyzerConfig{
		CacheTTL:     0,
		MaxCacheSize: 0,
	})
	ctx := context.Background()
	graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.InvalidateCache("tenant-1")
		_, _ = analyzer.FindClusters(graph)
	}
}

// BenchmarkCalculateCentrality benchmarks centrality calculation.
func BenchmarkCalculateCentrality(b *testing.B) {
	provider := &mockRelationshipProvider{
		relationships: createTestRelationships(),
	}
	logger := createTestLogger()
	analyzer := NewNetworkAnalyzer(provider, logger, &AnalyzerConfig{
		CacheTTL:     0,
		MaxCacheSize: 0,
	})
	ctx := context.Background()
	graph, _ := analyzer.BuildGraph(ctx, "tenant-1", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.InvalidateCache("tenant-1")
		_, _ = analyzer.CalculateCentrality(graph)
	}
}
