// Package network provides graph-based network analysis capabilities for relationship discovery.
package network

import (
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// RelationshipProvider is an interface for fetching relationships from storage.
type RelationshipProvider interface {
	// ListRelationships returns relationships for a tenant with optional filters.
	ListRelationships(ctx context.Context, tenantID string, filter *AnalysisFilter) ([]*relationshipv1.Relationship, error)
}

// NetworkAnalyzer provides graph analysis capabilities for relationship networks.
type NetworkAnalyzer struct {
	provider RelationshipProvider
	logger   logging.Logger
	cache    *analysisCache
}

// analysisCache provides caching for expensive analysis results.
type analysisCache struct {
	mu         sync.RWMutex
	graphs     map[string]*cachedGraph
	centrality map[string]*cachedCentrality
	clusters   map[string]*cachedClusters
	ttl        time.Duration
	maxSize    int
}

type cachedGraph struct {
	graph     *Graph
	filter    *AnalysisFilter
	createdAt time.Time
	version   string
}

type cachedCentrality struct {
	scores    *CentralityScores
	createdAt time.Time
	graphHash string
}

type cachedClusters struct {
	clusters  []Cluster
	createdAt time.Time
	graphHash string
}

// AnalyzerConfig configures the network analyzer.
type AnalyzerConfig struct {
	// CacheTTL is the time-to-live for cached analysis results.
	CacheTTL time.Duration
	// MaxCacheSize is the maximum number of cached items per type.
	MaxCacheSize int
}

// DefaultAnalyzerConfig returns default analyzer configuration.
func DefaultAnalyzerConfig() *AnalyzerConfig {
	return &AnalyzerConfig{
		CacheTTL:     15 * time.Minute,
		MaxCacheSize: 100,
	}
}

// NewNetworkAnalyzer creates a new NetworkAnalyzer.
func NewNetworkAnalyzer(provider RelationshipProvider, logger logging.Logger, config *AnalyzerConfig) *NetworkAnalyzer {
	if config == nil {
		config = DefaultAnalyzerConfig()
	}

	return &NetworkAnalyzer{
		provider: provider,
		logger:   logger.With(logging.F("component", "network_analyzer")),
		cache: &analysisCache{
			graphs:     make(map[string]*cachedGraph),
			centrality: make(map[string]*cachedCentrality),
			clusters:   make(map[string]*cachedClusters),
			ttl:        config.CacheTTL,
			maxSize:    config.MaxCacheSize,
		},
	}
}

// BuildGraph constructs a graph from relationships for the given tenant.
func (a *NetworkAnalyzer) BuildGraph(ctx context.Context, tenantID string, filter *AnalysisFilter) (*Graph, error) {
	// Check cache first
	cacheKey := a.buildCacheKey(tenantID, filter)
	if cached := a.getGraphFromCache(cacheKey); cached != nil {
		a.logger.Debug("Using cached graph",
			logging.F("tenant_id", tenantID),
			logging.F("cache_key", cacheKey),
		)
		return cached, nil
	}

	a.logger.Info("Building graph",
		logging.F("tenant_id", tenantID),
	)

	// Fetch relationships from provider
	relationships, err := a.provider.ListRelationships(ctx, tenantID, filter)
	if err != nil {
		return nil, fmt.Errorf("fetching relationships: %w", err)
	}

	graph := NewGraph(tenantID)

	// Build graph from relationships
	for _, rel := range relationships {
		// Apply filter if provided
		if filter != nil && !a.matchesFilter(rel, filter) {
			continue
		}

		// Add source entity as node
		if rel.SourceEntity != nil {
			sourceNode := &Node{
				ID:       rel.SourceEntity.Id,
				Entity:   rel.SourceEntity,
				Metadata: make(map[string]string),
			}
			if rel.SourceEntity.Metadata != nil {
				for k, v := range rel.SourceEntity.Metadata {
					sourceNode.Metadata[k] = v
				}
			}
			graph.AddNode(sourceNode)
		}

		// Add target entity as node
		if rel.TargetEntity != nil {
			targetNode := &Node{
				ID:       rel.TargetEntity.Id,
				Entity:   rel.TargetEntity,
				Metadata: make(map[string]string),
			}
			if rel.TargetEntity.Metadata != nil {
				for k, v := range rel.TargetEntity.Metadata {
					targetNode.Metadata[k] = v
				}
			}
			graph.AddNode(targetNode)
		}

		// Add edge
		if rel.SourceEntity != nil && rel.TargetEntity != nil {
			weight := float64(rel.Confidence)
			if len(rel.Evidence) > 0 {
				// Boost weight based on evidence count
				weight *= (1.0 + float64(len(rel.Evidence))*0.1)
			}

			edge := &Edge{
				ID:           rel.Id,
				SourceID:     rel.SourceEntity.Id,
				TargetID:     rel.TargetEntity.Id,
				Relationship: rel,
				Weight:       weight,
			}
			graph.AddEdge(edge)
		}
	}

	// Cache the result
	a.cacheGraph(cacheKey, graph, filter)

	a.logger.Info("Graph built successfully",
		logging.F("tenant_id", tenantID),
		logging.F("nodes", graph.NodeCount()),
		logging.F("edges", graph.EdgeCount()),
	)

	return graph, nil
}

// FindClusters identifies clusters of closely connected nodes using label propagation.
func (a *NetworkAnalyzer) FindClusters(graph *Graph) ([]Cluster, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	// Check cache
	graphHash := a.hashGraph(graph)
	cacheKey := fmt.Sprintf("%s:clusters", graph.TenantID)
	if cached := a.getClustersFromCache(cacheKey, graphHash); cached != nil {
		return cached, nil
	}

	nodes := graph.GetNodes()
	if len(nodes) == 0 {
		return []Cluster{}, nil
	}

	// Initialize: each node is its own cluster
	labels := make(map[string]int)
	for i, node := range nodes {
		labels[node.ID] = i
	}

	// Label propagation algorithm
	maxIterations := 100
	changed := true

	for iteration := 0; iteration < maxIterations && changed; iteration++ {
		changed = false

		// Process nodes in random order (we'll use deterministic order for reproducibility)
		for _, node := range nodes {
			neighbors := graph.GetAllNeighbors(node.ID)
			if len(neighbors) == 0 {
				continue
			}

			// Count label frequencies among neighbors
			labelCounts := make(map[int]int)
			for _, neighborID := range neighbors {
				label := labels[neighborID]
				labelCounts[label]++
			}

			// Find the most frequent label
			maxCount := 0
			bestLabel := labels[node.ID]
			for label, count := range labelCounts {
				if count > maxCount || (count == maxCount && label < bestLabel) {
					maxCount = count
					bestLabel = label
				}
			}

			// Update label if changed
			if labels[node.ID] != bestLabel {
				labels[node.ID] = bestLabel
				changed = true
			}
		}
	}

	// Build clusters from labels
	clusterNodes := make(map[int][]string)
	for nodeID, label := range labels {
		clusterNodes[label] = append(clusterNodes[label], nodeID)
	}

	// Convert to Cluster structs and assign sequential IDs
	clusters := make([]Cluster, 0, len(clusterNodes))
	clusterID := 0
	for _, nodeIDs := range clusterNodes {
		cluster := Cluster{
			ID:      clusterID,
			NodeIDs: nodeIDs,
			Size:    len(nodeIDs),
		}

		// Calculate cluster density
		cluster.Density = a.calculateClusterDensity(graph, nodeIDs)

		// Find centroid (node with highest degree within cluster)
		cluster.Centroid = a.findClusterCentroid(graph, nodeIDs)

		// Update node cluster assignments
		for _, nodeID := range nodeIDs {
			if node := graph.GetNode(nodeID); node != nil {
				node.ClusterID = clusterID
			}
		}

		clusters = append(clusters, cluster)
		clusterID++
	}

	// Sort clusters by size (descending)
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Size > clusters[j].Size
	})

	// Reassign IDs after sorting
	for i := range clusters {
		clusters[i].ID = i
		for _, nodeID := range clusters[i].NodeIDs {
			if node := graph.GetNode(nodeID); node != nil {
				node.ClusterID = i
			}
		}
	}

	// Cache results
	a.cacheClusters(cacheKey, clusters, graphHash)

	return clusters, nil
}

// CalculateCentrality computes centrality scores for all nodes in the graph.
func (a *NetworkAnalyzer) CalculateCentrality(graph *Graph) (*CentralityScores, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	// Check cache
	graphHash := a.hashGraph(graph)
	cacheKey := fmt.Sprintf("%s:centrality", graph.TenantID)
	if cached := a.getCentralityFromCache(cacheKey, graphHash); cached != nil {
		return cached, nil
	}

	scores := &CentralityScores{
		Degree:      make(map[string]float64),
		Betweenness: make(map[string]float64),
		Closeness:   make(map[string]float64),
	}

	nodes := graph.GetNodes()
	if len(nodes) == 0 {
		return scores, nil
	}

	// Calculate degree centrality
	maxDegree := 0
	for _, node := range nodes {
		if node.Degree > maxDegree {
			maxDegree = node.Degree
		}
	}
	for _, node := range nodes {
		if maxDegree > 0 {
			scores.Degree[node.ID] = float64(node.Degree) / float64(maxDegree)
		} else {
			scores.Degree[node.ID] = 0
		}
	}

	// Calculate betweenness centrality using Brandes' algorithm
	a.calculateBetweenness(graph, scores)

	// Calculate closeness centrality
	a.calculateCloseness(graph, scores)

	// Update nodes with their centrality scores (using degree centrality as primary)
	for _, node := range nodes {
		node.Centrality = scores.Degree[node.ID]
	}

	// Cache results
	a.cacheCentrality(cacheKey, scores, graphHash)

	return scores, nil
}

// FindShortestPath finds the shortest path between two nodes using Dijkstra's algorithm.
func (a *NetworkAnalyzer) FindShortestPath(graph *Graph, fromID, toID string) (*Path, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	if !graph.HasNode(fromID) {
		return nil, fmt.Errorf("source node %s not found", fromID)
	}
	if !graph.HasNode(toID) {
		return nil, fmt.Errorf("target node %s not found", toID)
	}

	if fromID == toID {
		return &Path{
			Nodes:       []string{fromID},
			Edges:       []string{},
			TotalWeight: 0,
			Length:      0,
		}, nil
	}

	// Dijkstra's algorithm
	dist := make(map[string]float64)
	prev := make(map[string]string)
	prevEdge := make(map[string]string)

	for _, node := range graph.GetNodes() {
		dist[node.ID] = math.Inf(1)
	}
	dist[fromID] = 0

	// Priority queue
	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &pqItem{nodeID: fromID, priority: 0})

	visited := make(map[string]bool)

	for pq.Len() > 0 {
		item := heap.Pop(pq).(*pqItem)
		nodeID := item.nodeID

		if visited[nodeID] {
			continue
		}
		visited[nodeID] = true

		if nodeID == toID {
			break
		}

		// Check outgoing edges
		for _, neighborID := range graph.GetNeighbors(nodeID) {
			if visited[neighborID] {
				continue
			}

			edge := graph.GetEdgeBetween(nodeID, neighborID)
			if edge == nil {
				continue
			}

			// Use inverse of weight for shortest path (higher weight = closer relationship)
			edgeCost := 1.0 / (edge.Weight + 0.01) // Add small value to avoid division by zero

			newDist := dist[nodeID] + edgeCost
			if newDist < dist[neighborID] {
				dist[neighborID] = newDist
				prev[neighborID] = nodeID
				prevEdge[neighborID] = edge.ID
				heap.Push(pq, &pqItem{nodeID: neighborID, priority: newDist})
			}
		}

		// Also check incoming edges (for undirected path finding)
		for _, neighborID := range graph.GetIncomingNeighbors(nodeID) {
			if visited[neighborID] {
				continue
			}

			edge := graph.GetEdgeBetween(neighborID, nodeID)
			if edge == nil {
				continue
			}

			edgeCost := 1.0 / (edge.Weight + 0.01)

			newDist := dist[nodeID] + edgeCost
			if newDist < dist[neighborID] {
				dist[neighborID] = newDist
				prev[neighborID] = nodeID
				prevEdge[neighborID] = edge.ID
				heap.Push(pq, &pqItem{nodeID: neighborID, priority: newDist})
			}
		}
	}

	// Check if path exists
	if _, ok := prev[toID]; !ok && fromID != toID {
		return nil, fmt.Errorf("no path found between %s and %s", fromID, toID)
	}

	// Reconstruct path
	path := &Path{
		Nodes:       make([]string, 0),
		Edges:       make([]string, 0),
		TotalWeight: dist[toID],
	}

	// Build path in reverse
	current := toID
	for current != "" {
		path.Nodes = append([]string{current}, path.Nodes...)
		if edgeID, ok := prevEdge[current]; ok {
			path.Edges = append([]string{edgeID}, path.Edges...)
		}
		current = prev[current]
	}

	path.Length = len(path.Nodes) - 1

	return path, nil
}

// ExportVisualization exports the graph in a format suitable for visualization tools.
func (a *NetworkAnalyzer) ExportVisualization(graph *Graph, clusters []Cluster, centrality *CentralityScores) (*VisualizationData, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	// Color map for entity types
	typeColors := map[relationshipv1.EntityType]string{
		relationshipv1.EntityType_ENTITY_TYPE_PERSON:       "#4285F4", // Blue
		relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION: "#34A853", // Green
		relationshipv1.EntityType_ENTITY_TYPE_TOPIC:        "#FBBC05", // Yellow
		relationshipv1.EntityType_ENTITY_TYPE_PROJECT:      "#EA4335", // Red
		relationshipv1.EntityType_ENTITY_TYPE_LOCATION:     "#9C27B0", // Purple
		relationshipv1.EntityType_ENTITY_TYPE_EVENT:        "#00BCD4", // Cyan
		relationshipv1.EntityType_ENTITY_TYPE_PRODUCT:      "#FF5722", // Orange
		relationshipv1.EntityType_ENTITY_TYPE_DOCUMENT:     "#607D8B", // Gray
	}

	// Cluster colors
	clusterColors := []string{
		"#1f77b4", "#ff7f0e", "#2ca02c", "#d62728", "#9467bd",
		"#8c564b", "#e377c2", "#7f7f7f", "#bcbd22", "#17becf",
	}

	data := &VisualizationData{
		Nodes:    make([]VisualizationNode, 0, graph.NodeCount()),
		Edges:    make([]VisualizationEdge, 0, graph.EdgeCount()),
		Clusters: make([]VisualizationCluster, 0),
	}

	// Export nodes
	for _, node := range graph.GetNodes() {
		vizNode := VisualizationNode{
			ID:         node.ID,
			Label:      node.Entity.GetName(),
			Type:       node.Entity.GetType().String(),
			Size:       math.Log2(float64(node.Degree+1)) + 1, // Log scale for size
			ClusterID:  node.ClusterID,
			Properties: node.Metadata,
		}

		// Set color based on entity type
		if color, ok := typeColors[node.Entity.GetType()]; ok {
			vizNode.Color = color
		} else {
			vizNode.Color = "#999999"
		}

		// Set centrality score
		if centrality != nil {
			if score, ok := centrality.Degree[node.ID]; ok {
				vizNode.Centrality = score
			}
		}

		data.Nodes = append(data.Nodes, vizNode)
	}

	// Export edges
	for _, edge := range graph.GetEdges() {
		vizEdge := VisualizationEdge{
			ID:     edge.ID,
			Source: edge.SourceID,
			Target: edge.TargetID,
			Weight: edge.Weight,
			Type:   edge.Relationship.GetRelationshipType().String(),
		}

		// Create label from relationship type
		vizEdge.Label = a.formatRelationshipType(edge.Relationship.GetRelationshipType())

		data.Edges = append(data.Edges, vizEdge)
	}

	// Export clusters
	if len(clusters) > 0 {
		for _, cluster := range clusters {
			vizCluster := VisualizationCluster{
				ID:      cluster.ID,
				NodeIDs: cluster.NodeIDs,
				Label:   fmt.Sprintf("Cluster %d", cluster.ID),
				Color:   clusterColors[cluster.ID%len(clusterColors)],
			}
			data.Clusters = append(data.Clusters, vizCluster)
		}
	}

	// Calculate statistics
	data.Statistics = VisualizationStatistics{
		TotalNodes:    graph.NodeCount(),
		TotalEdges:    graph.EdgeCount(),
		AverageDegree: graph.AverageDegree(),
		Density:       graph.Density(),
		ClusterCount:  len(clusters),
	}

	if len(clusters) > 0 {
		data.Statistics.LargestCluster = clusters[0].Size // Already sorted by size
	}

	return data, nil
}

// InvalidateCache invalidates all cached data for a tenant.
func (a *NetworkAnalyzer) InvalidateCache(tenantID string) {
	a.cache.mu.Lock()
	defer a.cache.mu.Unlock()

	// Remove all cached items for this tenant
	for key := range a.cache.graphs {
		if len(key) >= len(tenantID) && key[:len(tenantID)] == tenantID {
			delete(a.cache.graphs, key)
		}
	}

	for key := range a.cache.centrality {
		if len(key) >= len(tenantID) && key[:len(tenantID)] == tenantID {
			delete(a.cache.centrality, key)
		}
	}

	for key := range a.cache.clusters {
		if len(key) >= len(tenantID) && key[:len(tenantID)] == tenantID {
			delete(a.cache.clusters, key)
		}
	}

	a.logger.Info("Cache invalidated", logging.F("tenant_id", tenantID))
}

// InvalidateCacheForRelationship invalidates cache when a specific relationship changes.
func (a *NetworkAnalyzer) InvalidateCacheForRelationship(tenantID, _ string) {
	// For simplicity, invalidate all cache for the tenant
	// A more sophisticated implementation could track dependencies
	a.InvalidateCache(tenantID)
}

// --- Helper methods ---

// matchesFilter checks if a relationship matches the given filter criteria.
func (a *NetworkAnalyzer) matchesFilter(rel *relationshipv1.Relationship, filter *AnalysisFilter) bool {
	if filter == nil {
		return true
	}

	// Check relationship types
	if len(filter.RelationshipTypes) > 0 {
		found := false
		for _, rt := range filter.RelationshipTypes {
			if rel.RelationshipType == rt {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check entity types
	if len(filter.EntityTypes) > 0 {
		sourceMatch := false
		targetMatch := false
		for _, et := range filter.EntityTypes {
			if rel.SourceEntity != nil && rel.SourceEntity.Type == et {
				sourceMatch = true
			}
			if rel.TargetEntity != nil && rel.TargetEntity.Type == et {
				targetMatch = true
			}
		}
		if !sourceMatch && !targetMatch {
			return false
		}
	}

	// Check minimum confidence
	if filter.MinConfidence > 0 && float64(rel.Confidence) < filter.MinConfidence {
		return false
	}

	// Check confirmed only
	if filter.ConfirmedOnly && rel.Status != relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED {
		return false
	}

	// Check time range
	if filter.TimeRangeStart != nil && rel.CreatedAt != nil {
		if rel.CreatedAt.AsTime().Before(*filter.TimeRangeStart) {
			return false
		}
	}
	if filter.TimeRangeEnd != nil && rel.CreatedAt != nil {
		if rel.CreatedAt.AsTime().After(*filter.TimeRangeEnd) {
			return false
		}
	}

	return true
}

// calculateBetweenness computes betweenness centrality using Brandes' algorithm.
func (a *NetworkAnalyzer) calculateBetweenness(graph *Graph, scores *CentralityScores) {
	nodes := graph.GetNodes()

	// Initialize betweenness scores
	for _, node := range nodes {
		scores.Betweenness[node.ID] = 0
	}

	// For each node as source
	for _, source := range nodes {
		// BFS to find shortest paths
		stack := make([]string, 0)
		pred := make(map[string][]string)
		sigma := make(map[string]float64)
		dist := make(map[string]int)

		for _, node := range nodes {
			pred[node.ID] = make([]string, 0)
			sigma[node.ID] = 0
			dist[node.ID] = -1
		}

		sigma[source.ID] = 1
		dist[source.ID] = 0

		queue := []string{source.ID}

		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)

			for _, w := range graph.GetAllNeighbors(v) {
				// First visit to w?
				if dist[w] < 0 {
					queue = append(queue, w)
					dist[w] = dist[v] + 1
				}

				// Shortest path via v?
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		// Backpropagate dependencies
		delta := make(map[string]float64)
		for _, node := range nodes {
			delta[node.ID] = 0
		}

		for len(stack) > 0 {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			for _, v := range pred[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}

			if w != source.ID {
				scores.Betweenness[w] += delta[w]
			}
		}
	}

	// Normalize betweenness scores
	n := len(nodes)
	if n > 2 {
		normFactor := 2.0 / float64((n-1)*(n-2))
		for nodeID := range scores.Betweenness {
			scores.Betweenness[nodeID] *= normFactor
		}
	}
}

// calculateCloseness computes closeness centrality.
func (a *NetworkAnalyzer) calculateCloseness(graph *Graph, scores *CentralityScores) {
	nodes := graph.GetNodes()
	n := len(nodes)

	for _, source := range nodes {
		// BFS to find distances
		dist := make(map[string]int)
		for _, node := range nodes {
			dist[node.ID] = -1
		}
		dist[source.ID] = 0

		queue := []string{source.ID}
		totalDist := 0
		reachable := 0

		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]

			for _, w := range graph.GetAllNeighbors(v) {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					totalDist += dist[w]
					reachable++
					queue = append(queue, w)
				}
			}
		}

		// Calculate closeness
		if reachable > 0 && totalDist > 0 {
			// Normalized closeness: (reachable / (n-1)) * (reachable / totalDist)
			scores.Closeness[source.ID] = float64(reachable) / float64(n-1) * float64(reachable) / float64(totalDist)
		} else {
			scores.Closeness[source.ID] = 0
		}
	}
}

// calculateClusterDensity calculates the edge density within a cluster.
func (a *NetworkAnalyzer) calculateClusterDensity(graph *Graph, nodeIDs []string) float64 {
	if len(nodeIDs) < 2 {
		return 1.0
	}

	nodeSet := make(map[string]bool)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	internalEdges := 0
	for _, nodeID := range nodeIDs {
		for _, neighborID := range graph.GetAllNeighbors(nodeID) {
			if nodeSet[neighborID] {
				internalEdges++
			}
		}
	}

	// Each edge is counted twice (once from each endpoint)
	internalEdges /= 2

	// Maximum possible edges in the cluster
	maxEdges := len(nodeIDs) * (len(nodeIDs) - 1) / 2

	return float64(internalEdges) / float64(maxEdges)
}

// findClusterCentroid finds the node with the highest degree within a cluster.
func (a *NetworkAnalyzer) findClusterCentroid(graph *Graph, nodeIDs []string) string {
	if len(nodeIDs) == 0 {
		return ""
	}

	maxDegree := -1
	centroid := nodeIDs[0]

	for _, nodeID := range nodeIDs {
		if node := graph.GetNode(nodeID); node != nil {
			if node.Degree > maxDegree {
				maxDegree = node.Degree
				centroid = nodeID
			}
		}
	}

	return centroid
}

// formatRelationshipType converts a relationship type to a human-readable label.
func (a *NetworkAnalyzer) formatRelationshipType(rt relationshipv1.RelationshipType) string {
	switch rt {
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT:
		return "works at"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS:
		return "knows"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH:
		return "mentioned with"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED:
		return "discussed"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MANAGES:
		return "manages"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO:
		return "reports to"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH:
		return "collaborates with"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_ATTENDED:
		return "attended"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_OWNS:
		return "owns"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF:
		return "member of"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_CREATED:
		return "created"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT:
		return "located at"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_PART_OF:
		return "part of"
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO:
		return "related to"
	default:
		return "related"
	}
}

// --- Cache methods ---

func (a *NetworkAnalyzer) buildCacheKey(tenantID string, filter *AnalysisFilter) string {
	key := tenantID
	if filter != nil {
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%v", filter)))
		key += ":" + hex.EncodeToString(h.Sum(nil))[:8]
	}
	return key
}

func (a *NetworkAnalyzer) hashGraph(graph *Graph) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d:%d:%s", graph.NodeCount(), graph.EdgeCount(), graph.UpdatedAt.String())))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (a *NetworkAnalyzer) getGraphFromCache(key string) *Graph {
	a.cache.mu.RLock()
	defer a.cache.mu.RUnlock()

	if cached, ok := a.cache.graphs[key]; ok {
		if time.Since(cached.createdAt) < a.cache.ttl {
			return cached.graph.Clone()
		}
	}
	return nil
}

func (a *NetworkAnalyzer) cacheGraph(key string, graph *Graph, filter *AnalysisFilter) {
	a.cache.mu.Lock()
	defer a.cache.mu.Unlock()

	// Enforce max size
	if len(a.cache.graphs) >= a.cache.maxSize {
		a.evictOldestGraph()
	}

	a.cache.graphs[key] = &cachedGraph{
		graph:     graph.Clone(),
		filter:    filter,
		createdAt: time.Now(),
		version:   a.hashGraph(graph),
	}
}

func (a *NetworkAnalyzer) getCentralityFromCache(key, graphHash string) *CentralityScores {
	a.cache.mu.RLock()
	defer a.cache.mu.RUnlock()

	if cached, ok := a.cache.centrality[key]; ok {
		if time.Since(cached.createdAt) < a.cache.ttl && cached.graphHash == graphHash {
			// Return a copy
			scores := &CentralityScores{
				Degree:      make(map[string]float64),
				Betweenness: make(map[string]float64),
				Closeness:   make(map[string]float64),
			}
			for k, v := range cached.scores.Degree {
				scores.Degree[k] = v
			}
			for k, v := range cached.scores.Betweenness {
				scores.Betweenness[k] = v
			}
			for k, v := range cached.scores.Closeness {
				scores.Closeness[k] = v
			}
			return scores
		}
	}
	return nil
}

func (a *NetworkAnalyzer) cacheCentrality(key string, scores *CentralityScores, graphHash string) {
	a.cache.mu.Lock()
	defer a.cache.mu.Unlock()

	if len(a.cache.centrality) >= a.cache.maxSize {
		a.evictOldestCentrality()
	}

	a.cache.centrality[key] = &cachedCentrality{
		scores:    scores,
		createdAt: time.Now(),
		graphHash: graphHash,
	}
}

func (a *NetworkAnalyzer) getClustersFromCache(key, graphHash string) []Cluster {
	a.cache.mu.RLock()
	defer a.cache.mu.RUnlock()

	if cached, ok := a.cache.clusters[key]; ok {
		if time.Since(cached.createdAt) < a.cache.ttl && cached.graphHash == graphHash {
			// Return a copy
			clusters := make([]Cluster, len(cached.clusters))
			for i, c := range cached.clusters {
				clusters[i] = Cluster{
					ID:       c.ID,
					NodeIDs:  make([]string, len(c.NodeIDs)),
					Size:     c.Size,
					Density:  c.Density,
					Centroid: c.Centroid,
				}
				copy(clusters[i].NodeIDs, c.NodeIDs)
			}
			return clusters
		}
	}
	return nil
}

func (a *NetworkAnalyzer) cacheClusters(key string, clusters []Cluster, graphHash string) {
	a.cache.mu.Lock()
	defer a.cache.mu.Unlock()

	if len(a.cache.clusters) >= a.cache.maxSize {
		a.evictOldestClusters()
	}

	a.cache.clusters[key] = &cachedClusters{
		clusters:  clusters,
		createdAt: time.Now(),
		graphHash: graphHash,
	}
}

func (a *NetworkAnalyzer) evictOldestGraph() {
	var oldestKey string
	var oldestTime time.Time

	for key, cached := range a.cache.graphs {
		if oldestKey == "" || cached.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = cached.createdAt
		}
	}

	if oldestKey != "" {
		delete(a.cache.graphs, oldestKey)
	}
}

func (a *NetworkAnalyzer) evictOldestCentrality() {
	var oldestKey string
	var oldestTime time.Time

	for key, cached := range a.cache.centrality {
		if oldestKey == "" || cached.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = cached.createdAt
		}
	}

	if oldestKey != "" {
		delete(a.cache.centrality, oldestKey)
	}
}

func (a *NetworkAnalyzer) evictOldestClusters() {
	var oldestKey string
	var oldestTime time.Time

	for key, cached := range a.cache.clusters {
		if oldestKey == "" || cached.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = cached.createdAt
		}
	}

	if oldestKey != "" {
		delete(a.cache.clusters, oldestKey)
	}
}

// --- Priority queue implementation for Dijkstra's algorithm ---

type pqItem struct {
	nodeID   string
	priority float64
	index    int
}

type priorityQueue []*pqItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*pqItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}
