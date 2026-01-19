// Package network provides graph-based network analysis capabilities for relationship discovery.
// It includes data structures and algorithms for analyzing relationship networks,
// finding clusters, calculating centrality metrics, and supporting visualization.
package network

import (
	"sync"
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
)

// Graph represents a network of entities and their relationships.
// It is the primary data structure for network analysis operations.
type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges map[string]*Edge

	// Adjacency list for efficient neighbor lookups
	adjacency map[string][]string // nodeID -> list of neighbor nodeIDs
	// Reverse adjacency for incoming edges
	reverseAdjacency map[string][]string // nodeID -> list of nodes pointing to this node

	// Metadata
	TenantID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Node represents an entity in the network graph.
type Node struct {
	ID         string
	Entity     *relationshipv1.Entity
	Degree     int // Total connections (in + out)
	InDegree   int // Incoming connections
	OutDegree  int // Outgoing connections
	Metadata   map[string]string
	ClusterID  int     // Assigned cluster after clustering
	Centrality float64 // Calculated centrality score
}

// Edge represents a relationship between two entities in the graph.
type Edge struct {
	ID           string
	SourceID     string
	TargetID     string
	Relationship *relationshipv1.Relationship
	Weight       float64 // Derived from confidence and evidence
}

// Cluster represents a group of closely connected nodes.
type Cluster struct {
	ID       int
	NodeIDs  []string
	Size     int
	Density  float64 // Internal edge density
	Centroid string  // Most central node in the cluster
}

// Path represents a sequence of nodes connecting two entities.
type Path struct {
	Nodes       []string
	Edges       []string
	TotalWeight float64
	Length      int
}

// CentralityScores holds centrality metrics for all nodes in the graph.
type CentralityScores struct {
	Degree      map[string]float64
	Betweenness map[string]float64
	Closeness   map[string]float64
}

// AnalysisFilter specifies criteria for filtering graph analysis.
type AnalysisFilter struct {
	RelationshipTypes []relationshipv1.RelationshipType
	EntityTypes       []relationshipv1.EntityType
	MinConfidence     float64
	TimeRangeStart    *time.Time
	TimeRangeEnd      *time.Time
	ConfirmedOnly     bool
}

// VisualizationData contains graph data formatted for visualization tools.
type VisualizationData struct {
	Nodes      []VisualizationNode     `json:"nodes"`
	Edges      []VisualizationEdge     `json:"edges"`
	Clusters   []VisualizationCluster  `json:"clusters,omitempty"`
	Statistics VisualizationStatistics `json:"statistics"`
}

// VisualizationNode is a node formatted for visualization.
type VisualizationNode struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Type       string            `json:"type"`
	Size       float64           `json:"size"`  // Based on degree
	Color      string            `json:"color"` // Based on type or cluster
	ClusterID  int               `json:"cluster_id,omitempty"`
	Centrality float64           `json:"centrality"`
	Properties map[string]string `json:"properties,omitempty"`
}

// VisualizationEdge is an edge formatted for visualization.
type VisualizationEdge struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
	Type   string  `json:"type"`
}

// VisualizationCluster is a cluster formatted for visualization.
type VisualizationCluster struct {
	ID      int      `json:"id"`
	NodeIDs []string `json:"node_ids"`
	Label   string   `json:"label"`
	Color   string   `json:"color"`
}

// VisualizationStatistics contains aggregate graph statistics.
type VisualizationStatistics struct {
	TotalNodes     int     `json:"total_nodes"`
	TotalEdges     int     `json:"total_edges"`
	AverageDegree  float64 `json:"average_degree"`
	Density        float64 `json:"density"`
	ClusterCount   int     `json:"cluster_count"`
	LargestCluster int     `json:"largest_cluster"`
}

// NewGraph creates a new empty Graph.
func NewGraph(tenantID string) *Graph {
	now := time.Now()
	return &Graph{
		nodes:            make(map[string]*Node),
		edges:            make(map[string]*Edge),
		adjacency:        make(map[string][]string),
		reverseAdjacency: make(map[string][]string),
		TenantID:         tenantID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// AddNode adds a node to the graph. Returns true if the node was added,
// false if it already existed.
func (g *Graph) AddNode(node *Node) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[node.ID]; exists {
		return false
	}

	g.nodes[node.ID] = node
	g.adjacency[node.ID] = make([]string, 0)
	g.reverseAdjacency[node.ID] = make([]string, 0)
	g.UpdatedAt = time.Now()
	return true
}

// AddEdge adds an edge to the graph. Returns true if the edge was added,
// false if it already existed or the source/target nodes don't exist.
func (g *Graph) AddEdge(edge *Edge) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check if edge already exists
	if _, exists := g.edges[edge.ID]; exists {
		return false
	}

	// Check if both nodes exist
	sourceNode, sourceExists := g.nodes[edge.SourceID]
	targetNode, targetExists := g.nodes[edge.TargetID]
	if !sourceExists || !targetExists {
		return false
	}

	// Add the edge
	g.edges[edge.ID] = edge

	// Update adjacency lists
	g.adjacency[edge.SourceID] = append(g.adjacency[edge.SourceID], edge.TargetID)
	g.reverseAdjacency[edge.TargetID] = append(g.reverseAdjacency[edge.TargetID], edge.SourceID)

	// Update node degrees
	sourceNode.OutDegree++
	sourceNode.Degree++
	targetNode.InDegree++
	targetNode.Degree++

	g.UpdatedAt = time.Now()
	return true
}

// GetNode returns a node by ID, or nil if not found.
func (g *Graph) GetNode(id string) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[id]
}

// GetEdge returns an edge by ID, or nil if not found.
func (g *Graph) GetEdge(id string) *Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edges[id]
}

// GetNeighbors returns all neighbor node IDs for a given node (outgoing edges).
func (g *Graph) GetNeighbors(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if neighbors, exists := g.adjacency[nodeID]; exists {
		result := make([]string, len(neighbors))
		copy(result, neighbors)
		return result
	}
	return nil
}

// GetIncomingNeighbors returns all node IDs that have edges pointing to this node.
func (g *Graph) GetIncomingNeighbors(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if neighbors, exists := g.reverseAdjacency[nodeID]; exists {
		result := make([]string, len(neighbors))
		copy(result, neighbors)
		return result
	}
	return nil
}

// GetAllNeighbors returns all connected node IDs (both incoming and outgoing).
func (g *Graph) GetAllNeighbors(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	seen := make(map[string]bool)
	var result []string

	if outgoing, exists := g.adjacency[nodeID]; exists {
		for _, id := range outgoing {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}

	if incoming, exists := g.reverseAdjacency[nodeID]; exists {
		for _, id := range incoming {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}

	return result
}

// GetNodes returns all nodes in the graph.
func (g *Graph) GetNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetEdges returns all edges in the graph.
func (g *Graph) GetEdges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := make([]*Edge, 0, len(g.edges))
	for _, edge := range g.edges {
		edges = append(edges, edge)
	}
	return edges
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount returns the number of edges in the graph.
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

// HasNode checks if a node with the given ID exists.
func (g *Graph) HasNode(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.nodes[id]
	return exists
}

// HasEdge checks if an edge with the given ID exists.
func (g *Graph) HasEdge(id string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, exists := g.edges[id]
	return exists
}

// GetEdgeBetween returns the edge between two nodes, or nil if none exists.
func (g *Graph) GetEdgeBetween(sourceID, targetID string) *Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, edge := range g.edges {
		if edge.SourceID == sourceID && edge.TargetID == targetID {
			return edge
		}
	}
	return nil
}

// RemoveNode removes a node and all its edges from the graph.
func (g *Graph) RemoveNode(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[id]; !exists {
		return false
	}

	// Remove all edges connected to this node
	edgesToRemove := make([]string, 0)
	for edgeID, edge := range g.edges {
		if edge.SourceID == id || edge.TargetID == id {
			edgesToRemove = append(edgesToRemove, edgeID)
		}
	}

	for _, edgeID := range edgesToRemove {
		edge := g.edges[edgeID]

		// Update degrees of connected nodes
		if sourceNode, exists := g.nodes[edge.SourceID]; exists && edge.SourceID != id {
			sourceNode.OutDegree--
			sourceNode.Degree--
		}
		if targetNode, exists := g.nodes[edge.TargetID]; exists && edge.TargetID != id {
			targetNode.InDegree--
			targetNode.Degree--
		}

		delete(g.edges, edgeID)
	}

	// Update adjacency lists of other nodes
	for nodeID := range g.adjacency {
		if nodeID != id {
			g.adjacency[nodeID] = removeFromSlice(g.adjacency[nodeID], id)
		}
	}
	for nodeID := range g.reverseAdjacency {
		if nodeID != id {
			g.reverseAdjacency[nodeID] = removeFromSlice(g.reverseAdjacency[nodeID], id)
		}
	}

	// Remove the node itself
	delete(g.nodes, id)
	delete(g.adjacency, id)
	delete(g.reverseAdjacency, id)

	g.UpdatedAt = time.Now()
	return true
}

// RemoveEdge removes an edge from the graph.
func (g *Graph) RemoveEdge(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	edge, exists := g.edges[id]
	if !exists {
		return false
	}

	// Update node degrees
	if sourceNode, exists := g.nodes[edge.SourceID]; exists {
		sourceNode.OutDegree--
		sourceNode.Degree--
	}
	if targetNode, exists := g.nodes[edge.TargetID]; exists {
		targetNode.InDegree--
		targetNode.Degree--
	}

	// Update adjacency lists
	g.adjacency[edge.SourceID] = removeFromSlice(g.adjacency[edge.SourceID], edge.TargetID)
	g.reverseAdjacency[edge.TargetID] = removeFromSlice(g.reverseAdjacency[edge.TargetID], edge.SourceID)

	delete(g.edges, id)
	g.UpdatedAt = time.Now()
	return true
}

// Density calculates the density of the graph (ratio of actual edges to possible edges).
func (g *Graph) Density() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	n := len(g.nodes)
	if n < 2 {
		return 0
	}

	// For a directed graph: density = edges / (n * (n - 1))
	maxEdges := float64(n * (n - 1))
	return float64(len(g.edges)) / maxEdges
}

// AverageDegree calculates the average degree of nodes in the graph.
func (g *Graph) AverageDegree() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return 0
	}

	totalDegree := 0
	for _, node := range g.nodes {
		totalDegree += node.Degree
	}
	return float64(totalDegree) / float64(len(g.nodes))
}

// Clone creates a deep copy of the graph.
func (g *Graph) Clone() *Graph {
	g.mu.RLock()
	defer g.mu.RUnlock()

	clone := NewGraph(g.TenantID)
	clone.CreatedAt = g.CreatedAt
	clone.UpdatedAt = g.UpdatedAt

	// Copy nodes
	for id, node := range g.nodes {
		clonedNode := &Node{
			ID:         node.ID,
			Entity:     node.Entity, // Shallow copy of proto entity
			Degree:     node.Degree,
			InDegree:   node.InDegree,
			OutDegree:  node.OutDegree,
			ClusterID:  node.ClusterID,
			Centrality: node.Centrality,
		}
		if node.Metadata != nil {
			clonedNode.Metadata = make(map[string]string)
			for k, v := range node.Metadata {
				clonedNode.Metadata[k] = v
			}
		}
		clone.nodes[id] = clonedNode
		clone.adjacency[id] = make([]string, len(g.adjacency[id]))
		copy(clone.adjacency[id], g.adjacency[id])
		clone.reverseAdjacency[id] = make([]string, len(g.reverseAdjacency[id]))
		copy(clone.reverseAdjacency[id], g.reverseAdjacency[id])
	}

	// Copy edges
	for id, edge := range g.edges {
		clone.edges[id] = &Edge{
			ID:           edge.ID,
			SourceID:     edge.SourceID,
			TargetID:     edge.TargetID,
			Relationship: edge.Relationship, // Shallow copy of proto relationship
			Weight:       edge.Weight,
		}
	}

	return clone
}

// removeFromSlice removes the first occurrence of a value from a slice.
func removeFromSlice(slice []string, value string) []string {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
