// Package relationshipservice implements the RelationshipService gRPC server.
package relationshipservice

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/relationships"
)

// RelationshipRepository defines the interface for relationship data access.
type RelationshipRepository interface {
	ListRelationships(ctx context.Context, filter relationships.ListRelationshipsFilter) ([]relationships.Relationship, int64, error)
	GetRelationship(ctx context.Context, tenantID, relationshipID string) (*relationships.Relationship, error)
	SearchRelationships(ctx context.Context, query relationships.SearchRelationshipsQuery) ([]relationships.Relationship, error)
	ListEntities(ctx context.Context, filter relationships.ListEntitiesFilter) ([]relationships.Entity, int64, error)
	GetEntity(ctx context.Context, tenantID, entityID string) (*relationships.Entity, error)
	InsertRelationship(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error)
	MergeEntities(ctx context.Context, req relationships.MergeEntitiesRequest) (*relationships.MergeEntitiesResult, error)
	GetNetworkStats(ctx context.Context, tenantID string) (*relationships.NetworkStats, error)
	GetCentralEntities(ctx context.Context, tenantID string, limit int) ([]relationships.Entity, error)
	GetClusters(ctx context.Context, tenantID string) ([]relationships.NetworkCluster, error)
	ListConflicts(ctx context.Context, filter relationships.ListConflictsFilter) ([]relationships.RelationshipConflict, int64, error)
	GetConflict(ctx context.Context, tenantID, conflictID string) (*relationships.RelationshipConflict, error)
	ResolveConflict(ctx context.Context, req relationships.ResolveConflictRequest) (*relationships.ResolveConflictResult, error)
	FindDuplicateEntities(ctx context.Context, tenantID string, minSimilarity float64) ([]relationships.DuplicatePair, error)
	GetMergePreview(ctx context.Context, tenantID string, entityID1, entityID2 string) (*relationships.MergePreview, error)
	AutoMergeDuplicates(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error)
}

// Service implements the RelationshipService gRPC server.
type Service struct {
	relationshipv1.UnimplementedRelationshipServiceServer
	repo   RelationshipRepository
	logger logging.Logger
}

// NewService creates a new relationship service.
func NewService(repo RelationshipRepository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListRelationships returns a paginated list of relationships with optional filters.
func (s *Service) ListRelationships(ctx context.Context, req *relationshipv1.ListRelationshipsRequest) (*relationshipv1.ListRelationshipsResponse, error) {
	s.logger.Debug("ListRelationships called",
		logging.F("tenant_id", req.TenantId),
		logging.F("page_size", req.PageSize),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	filter := relationships.ListRelationshipsFilter{
		TenantID: req.TenantId,
		Limit:    int(req.PageSize),
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	if req.EntityId != nil {
		filter.EntityID = *req.EntityId
	}

	if req.MinConfidence != nil {
		filter.MinConfidence = float64(*req.MinConfidence)
	}

	if req.RelationshipType != nil {
		filter.RelationshipType = protoRelTypeToInternal(*req.RelationshipType)
	}

	if req.SourceEntityType != nil {
		filter.SourceEntityType = protoEntityTypeToInternal(*req.SourceEntityType)
	}

	if req.TargetEntityType != nil {
		filter.TargetEntityType = protoEntityTypeToInternal(*req.TargetEntityType)
	}

	rels, totalCount, err := s.repo.ListRelationships(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing relationships", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list relationships: %v", err)
	}

	protoRels := make([]*relationshipv1.Relationship, len(rels))
	for i, r := range rels {
		protoRels[i] = relationshipToProto(&r)
	}

	return &relationshipv1.ListRelationshipsResponse{
		Relationships: protoRels,
		TotalCount:    &totalCount,
	}, nil
}

// GetRelationship retrieves a single relationship by ID.
func (s *Service) GetRelationship(ctx context.Context, req *relationshipv1.GetRelationshipRequest) (*relationshipv1.Relationship, error) {
	s.logger.Debug("GetRelationship called",
		logging.F("tenant_id", req.TenantId),
		logging.F("relationship_id", req.RelationshipId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.RelationshipId == "" {
		return nil, status.Error(codes.InvalidArgument, "relationship_id is required")
	}

	rel, err := s.repo.GetRelationship(ctx, req.TenantId, req.RelationshipId)
	if err != nil {
		s.logger.Error("Error getting relationship", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get relationship: %v", err)
	}

	if rel == nil {
		return nil, status.Errorf(codes.NotFound, "relationship not found: %s", req.RelationshipId)
	}

	return relationshipToProto(rel), nil
}

// SearchRelationships searches relationships by entity name.
func (s *Service) SearchRelationships(ctx context.Context, req *relationshipv1.SearchRelationshipsRequest) (*relationshipv1.SearchRelationshipsResponse, error) {
	s.logger.Debug("SearchRelationships called",
		logging.F("tenant_id", req.TenantId),
		logging.F("query", req.Query),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	query := relationships.SearchRelationshipsQuery{
		TenantID: req.TenantId,
		Query:    req.Query,
		Limit:    int(req.Limit),
	}

	if query.Limit == 0 {
		query.Limit = 20
	}

	if req.MinConfidence != nil {
		query.MinConfidence = float64(*req.MinConfidence)
	}

	rels, err := s.repo.SearchRelationships(ctx, query)
	if err != nil {
		s.logger.Error("Error searching relationships", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to search relationships: %v", err)
	}

	protoRels := make([]*relationshipv1.Relationship, len(rels))
	for i, r := range rels {
		protoRels[i] = relationshipToProto(&r)
	}

	return &relationshipv1.SearchRelationshipsResponse{
		Relationships: protoRels,
		TotalCount:    int64(len(rels)),
	}, nil
}

// ListEntities returns a paginated list of entities with optional filters.
func (s *Service) ListEntities(ctx context.Context, req *relationshipv1.ListEntitiesRequest) (*relationshipv1.ListEntitiesResponse, error) {
	s.logger.Debug("ListEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("page_size", req.PageSize),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	filter := relationships.ListEntitiesFilter{
		TenantID: req.TenantId,
		Limit:    int(req.PageSize),
		Offset:   int(req.Offset),
	}

	if filter.Limit == 0 {
		filter.Limit = 100
	}

	if req.EntityType != nil {
		filter.EntityType = protoEntityTypeToInternal(*req.EntityType)
	}

	if req.Search != nil {
		filter.Search = *req.Search
	}

	if req.MinConfidence != nil {
		filter.MinConfidence = float64(*req.MinConfidence)
	}

	entities, totalCount, err := s.repo.ListEntities(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing entities", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list entities: %v", err)
	}

	protoEntities := make([]*relationshipv1.Entity, len(entities))
	for i, e := range entities {
		protoEntities[i] = entityToProto(&e)
	}

	return &relationshipv1.ListEntitiesResponse{
		Entities:   protoEntities,
		TotalCount: totalCount,
	}, nil
}

// GetEntity retrieves a single entity by ID.
func (s *Service) GetEntity(ctx context.Context, req *relationshipv1.GetEntityRequest) (*relationshipv1.Entity, error) {
	s.logger.Debug("GetEntity called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id", req.EntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	entity, err := s.repo.GetEntity(ctx, req.TenantId, req.EntityId)
	if err != nil {
		s.logger.Error("Error getting entity", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get entity: %v", err)
	}

	if entity == nil {
		return nil, status.Errorf(codes.NotFound, "entity not found: %s", req.EntityId)
	}

	return entityToProto(entity), nil
}

// MergeEntities merges two entities into one.
func (s *Service) MergeEntities(ctx context.Context, req *relationshipv1.MergeEntitiesRequest) (*relationshipv1.MergeEntitiesResponse, error) {
	s.logger.Debug("MergeEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("primary_entity_id", req.PrimaryEntityId),
		logging.F("merged_entity_id", req.MergedEntityId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.PrimaryEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "primary_entity_id is required")
	}
	if req.MergedEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "merged_entity_id is required")
	}

	result, err := s.repo.MergeEntities(ctx, relationships.MergeEntitiesRequest{
		TenantID:        req.TenantId,
		PrimaryEntityID: req.PrimaryEntityId,
		MergedEntityID:  req.MergedEntityId,
	})
	if err != nil {
		s.logger.Error("Error merging entities", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to merge entities: %v", err)
	}

	return &relationshipv1.MergeEntitiesResponse{
		PrimaryEntity:           entityToProto(result.PrimaryEntity),
		RelationshipsTransferred: int32(result.RelationshipsTransferred),
		Success:                  true,
		Message:                  "Entities merged successfully",
	}, nil
}

// GetNetworkStats retrieves statistics about the relationship network.
func (s *Service) GetNetworkStats(ctx context.Context, req *relationshipv1.GetNetworkStatsRequest) (*relationshipv1.NetworkStats, error) {
	s.logger.Debug("GetNetworkStats called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	stats, err := s.repo.GetNetworkStats(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Error getting network stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get network stats: %v", err)
	}

	entityTypeCounts := make(map[string]int32)
	for k, v := range stats.EntityTypeCounts {
		entityTypeCounts[string(k)] = int32(v)
	}

	relTypeCounts := make(map[string]int32)
	for k, v := range stats.RelTypeCounts {
		relTypeCounts[string(k)] = int32(v)
	}

	return &relationshipv1.NetworkStats{
		TotalNodes:             int32(stats.TotalNodes),
		TotalEdges:             int32(stats.TotalEdges),
		Density:                float32(stats.Density),
		AvgConnections:         float32(stats.AvgConnections),
		ClusterCount:           int32(stats.ClusterCount),
		EntityTypeCounts:       entityTypeCounts,
		RelationshipTypeCounts: relTypeCounts,
	}, nil
}

// GetCentralEntities retrieves the most connected entities in the network.
func (s *Service) GetCentralEntities(ctx context.Context, req *relationshipv1.GetCentralEntitiesRequest) (*relationshipv1.GetCentralEntitiesResponse, error) {
	s.logger.Debug("GetCentralEntities called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 10
	}

	entities, err := s.repo.GetCentralEntities(ctx, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Error getting central entities", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get central entities: %v", err)
	}

	protoEntities := make([]*relationshipv1.Entity, len(entities))
	for i, e := range entities {
		protoEntities[i] = entityToProto(&e)
	}

	return &relationshipv1.GetCentralEntitiesResponse{
		Entities: protoEntities,
	}, nil
}

// GetClusters retrieves clusters (communities) in the relationship network.
func (s *Service) GetClusters(ctx context.Context, req *relationshipv1.GetClustersRequest) (*relationshipv1.GetClustersResponse, error) {
	s.logger.Debug("GetClusters called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	clusters, err := s.repo.GetClusters(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Error getting clusters", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get clusters: %v", err)
	}

	protoClusters := make([]*relationshipv1.NetworkCluster, len(clusters))
	for i, c := range clusters {
		topEntities := make([]*relationshipv1.Entity, len(c.TopEntities))
		for j, e := range c.TopEntities {
			topEntities[j] = entityToProto(&e)
		}

		protoClusters[i] = &relationshipv1.NetworkCluster{
			Id:          c.ID,
			Name:        c.Name,
			EntityCount: int32(c.EntityCount),
			TopEntities: topEntities,
			Density:     float32(c.Density),
		}
	}

	return &relationshipv1.GetClustersResponse{
		Clusters: protoClusters,
	}, nil
}

// GetNetworkGraph retrieves the relationship network graph for visualization.
func (s *Service) GetNetworkGraph(ctx context.Context, req *relationshipv1.GetNetworkGraphRequest) (*relationshipv1.NetworkGraph, error) {
	s.logger.Debug("GetNetworkGraph called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Get stats for metadata
	stats, err := s.repo.GetNetworkStats(ctx, req.TenantId)
	if err != nil {
		s.logger.Error("Error getting network stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get network stats: %v", err)
	}

	// Get entities for nodes
	entities, _, err := s.repo.ListEntities(ctx, relationships.ListEntitiesFilter{
		TenantID: req.TenantId,
		Limit:    int(req.MaxNodes),
	})
	if err != nil {
		s.logger.Error("Error listing entities", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list entities: %v", err)
	}

	nodes := make([]*relationshipv1.GraphNode, len(entities))
	for i, e := range entities {
		nodes[i] = &relationshipv1.GraphNode{
			Id:         e.ID,
			Label:      e.Name,
			Type:       internalEntityTypeToProto(e.Type),
			Degree:     int32(e.RelationCount),
			Properties: e.Metadata,
		}
	}

	// Get relationships for edges
	rels, _, err := s.repo.ListRelationships(ctx, relationships.ListRelationshipsFilter{
		TenantID: req.TenantId,
		Limit:    1000,
	})
	if err != nil {
		s.logger.Error("Error listing relationships", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list relationships: %v", err)
	}

	edges := make([]*relationshipv1.GraphEdge, len(rels))
	for i, r := range rels {
		edges[i] = &relationshipv1.GraphEdge{
			Id:               r.ID,
			Source:           r.SourceID,
			Target:           r.TargetID,
			RelationshipType: internalRelTypeToProto(r.Type),
			Weight:           float32(r.Weight),
			Label:            string(r.Type),
		}
	}

	metadata := &relationshipv1.GraphMetadata{
		TotalNodes: int32(stats.TotalNodes),
		TotalEdges: int32(stats.TotalEdges),
		Truncated:  len(nodes) < stats.TotalNodes,
		Depth:      req.Depth,
	}
	if req.CenterEntityId != nil {
		metadata.CenterEntityId = req.CenterEntityId
	}

	return &relationshipv1.NetworkGraph{
		Nodes:    nodes,
		Edges:    edges,
		Metadata: metadata,
	}, nil
}

// ListConflicts returns a list of detected relationship conflicts.
func (s *Service) ListConflicts(ctx context.Context, req *relationshipv1.ListConflictsRequest) (*relationshipv1.ListConflictsResponse, error) {
	s.logger.Debug("ListConflicts called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	filter := relationships.ListConflictsFilter{
		TenantID: req.TenantId,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}

	if req.Status != nil {
		filter.Status = protoConflictStatusToInternal(*req.Status)
	}

	conflicts, totalCount, err := s.repo.ListConflicts(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing conflicts", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list conflicts: %v", err)
	}

	protoConflicts := make([]*relationshipv1.Conflict, len(conflicts))
	for i, c := range conflicts {
		protoConflicts[i] = conflictToProto(&c)
	}

	return &relationshipv1.ListConflictsResponse{
		Conflicts:  protoConflicts,
		TotalCount: totalCount,
	}, nil
}

// GetConflict retrieves a single conflict by ID.
func (s *Service) GetConflict(ctx context.Context, req *relationshipv1.GetConflictRequest) (*relationshipv1.Conflict, error) {
	s.logger.Debug("GetConflict called",
		logging.F("tenant_id", req.TenantId),
		logging.F("conflict_id", req.ConflictId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.ConflictId == "" {
		return nil, status.Error(codes.InvalidArgument, "conflict_id is required")
	}

	conflict, err := s.repo.GetConflict(ctx, req.TenantId, req.ConflictId)
	if err != nil {
		s.logger.Error("Error getting conflict", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get conflict: %v", err)
	}

	if conflict == nil {
		return nil, status.Errorf(codes.NotFound, "conflict not found: %s", req.ConflictId)
	}

	return conflictToProto(conflict), nil
}

// ResolveConflict resolves a relationship conflict.
func (s *Service) ResolveConflict(ctx context.Context, req *relationshipv1.ResolveConflictRequest) (*relationshipv1.ResolveConflictResponse, error) {
	s.logger.Debug("ResolveConflict called",
		logging.F("tenant_id", req.TenantId),
		logging.F("conflict_id", req.ConflictId),
		logging.F("strategy", req.Strategy),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.ConflictId == "" {
		return nil, status.Error(codes.InvalidArgument, "conflict_id is required")
	}

	resolveReq := relationships.ResolveConflictRequest{
		TenantID:   req.TenantId,
		ConflictID: req.ConflictId,
		Strategy:   protoConflictStrategyToInternal(req.Strategy),
	}

	if req.ResolvedBy != nil {
		resolveReq.ResolvedBy = *req.ResolvedBy
	}

	if req.Notes != nil {
		resolveReq.Notes = *req.Notes
	}

	result, err := s.repo.ResolveConflict(ctx, resolveReq)
	if err != nil {
		s.logger.Error("Error resolving conflict", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve conflict: %v", err)
	}

	return &relationshipv1.ResolveConflictResponse{
		Conflict:             conflictToProto(result.Conflict),
		RelationshipsUpdated: int32(result.RelationshipsUpdated),
		Success:              true,
		Message:              "Conflict resolved successfully",
	}, nil
}

// DiscoverRelationships is not implemented yet.
func (s *Service) DiscoverRelationships(ctx context.Context, req *relationshipv1.DiscoverRelationshipsRequest) (*relationshipv1.DiscoverRelationshipsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DiscoverRelationships not implemented")
}

// ValidateRelationship is not implemented yet.
func (s *Service) ValidateRelationship(ctx context.Context, req *relationshipv1.ValidateRelationshipRequest) (*relationshipv1.ValidateRelationshipResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ValidateRelationship not implemented")
}

// CreateRelationship manually creates a new relationship between two entities.
func (s *Service) CreateRelationship(ctx context.Context, req *relationshipv1.CreateRelationshipRequest) (*relationshipv1.CreateRelationshipResponse, error) {
	s.logger.Debug("CreateRelationship called",
		logging.F("tenant_id", req.TenantId),
		logging.F("from_entity_id", req.FromEntityId),
		logging.F("to_entity_id", req.ToEntityId),
		logging.F("type", req.Type),
	)

	// Validate required fields
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.FromEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "from_entity_id is required")
	}
	if req.ToEntityId == "" {
		return nil, status.Error(codes.InvalidArgument, "to_entity_id is required")
	}
	if req.Type == relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type is required and cannot be UNSPECIFIED")
	}

	// Validate that both entities exist
	fromEntity, err := s.repo.GetEntity(ctx, req.TenantId, req.FromEntityId)
	if err != nil {
		s.logger.Error("Error checking from_entity existence", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to check from_entity: %v", err)
	}
	if fromEntity == nil {
		return nil, status.Errorf(codes.NotFound, "from_entity not found: %s", req.FromEntityId)
	}

	toEntity, err := s.repo.GetEntity(ctx, req.TenantId, req.ToEntityId)
	if err != nil {
		s.logger.Error("Error checking to_entity existence", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to check to_entity: %v", err)
	}
	if toEntity == nil {
		return nil, status.Errorf(codes.NotFound, "to_entity not found: %s", req.ToEntityId)
	}

	// Build the insert request
	insertReq := relationships.InsertRelationshipRequest{
		TenantID:       req.TenantId,
		FromEntityID:   req.FromEntityId,
		FromEntityType: fromEntity.Type,
		ToEntityID:     req.ToEntityId,
		ToEntityType:   toEntity.Type,
		Type:           protoRelTypeToInternal(req.Type),
		Confidence:     1.0,
		UserConfirmed:  true,
	}

	if req.Subtype != nil {
		insertReq.Subtype = *req.Subtype
	}

	// Insert the relationship
	rel, err := s.repo.InsertRelationship(ctx, insertReq)
	if err != nil {
		// Check for duplicate key error
		errMsg := err.Error()
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate key") || strings.Contains(errMsg, "unique constraint") {
			return nil, status.Error(codes.AlreadyExists, "relationship already exists")
		}
		s.logger.Error("Error inserting relationship", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create relationship: %v", err)
	}

	// Populate entity names for the response
	rel.SourceName = fromEntity.Name
	rel.TargetName = toEntity.Name

	s.logger.Info("Relationship created",
		logging.F("relationship_id", rel.ID),
		logging.F("from_entity_id", req.FromEntityId),
		logging.F("to_entity_id", req.ToEntityId),
		logging.F("type", req.Type),
	)

	return &relationshipv1.CreateRelationshipResponse{
		Relationship: relationshipToProto(rel),
	}, nil
}

// FindDuplicates finds pairs of entities that are likely duplicates.
func (s *Service) FindDuplicates(ctx context.Context, req *relationshipv1.FindDuplicatesRequest) (*relationshipv1.FindDuplicatesResponse, error) {
	s.logger.Debug("FindDuplicates called",
		logging.F("tenant_id", req.TenantId),
		logging.F("min_similarity", req.GetMinSimilarity()),
	)

	// Validate tenant_id
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Default min_similarity to 0.85 if not provided, convert from float32 to float64
	minSimilarity := 0.85
	if req.MinSimilarity != nil {
		minSimilarity = float64(*req.MinSimilarity)
	}

	// Validate min_similarity bounds (0.5 to 1.0)
	if minSimilarity < 0.5 || minSimilarity > 1.0 {
		return nil, status.Errorf(codes.InvalidArgument, "min_similarity must be between 0.5 and 1.0, got: %.2f", minSimilarity)
	}

	// Call repository
	pairs, err := s.repo.FindDuplicateEntities(ctx, req.TenantId, minSimilarity)
	if err != nil {
		s.logger.Error("Error finding duplicate entities", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to find duplicate entities: %v", err)
	}

	// Convert to proto
	protoPairs := make([]*relationshipv1.DuplicatePair, len(pairs))
	for i, pair := range pairs {
		protoPairs[i] = duplicatePairToProto(&pair)
	}

	return &relationshipv1.FindDuplicatesResponse{
		DuplicatePairs: protoPairs,
		TotalCount:     int32(len(pairs)),
	}, nil
}

// MergePreview shows what would happen if two entities were merged.
func (s *Service) MergePreview(ctx context.Context, req *relationshipv1.MergePreviewRequest) (*relationshipv1.MergePreviewResponse, error) {
	s.logger.Debug("MergePreview called",
		logging.F("tenant_id", req.TenantId),
		logging.F("entity_id1", req.EntityId1),
		logging.F("entity_id2", req.EntityId2),
	)

	// Validate required fields
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.EntityId1 == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id_1 is required")
	}
	if req.EntityId2 == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id_2 is required")
	}

	// Call repository
	preview, err := s.repo.GetMergePreview(ctx, req.TenantId, req.EntityId1, req.EntityId2)
	if err != nil {
		s.logger.Error("Error getting merge preview", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get merge preview: %v", err)
	}

	// Convert to proto
	return mergePreviewToProto(preview), nil
}

// AutoMergeDuplicates automatically merges high-confidence duplicate entities.
func (s *Service) AutoMergeDuplicates(ctx context.Context, req *relationshipv1.AutoMergeDuplicatesRequest) (*relationshipv1.AutoMergeDuplicatesResponse, error) {
	s.logger.Debug("AutoMergeDuplicates called",
		logging.F("tenant_id", req.TenantId),
		logging.F("min_similarity", req.GetMinSimilarity()),
		logging.F("dry_run", req.DryRun),
	)

	// Validate tenant_id
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Default min_similarity to 0.95 if not provided, convert from float32 to float64
	minSimilarity := 0.95
	if req.MinSimilarity != nil {
		minSimilarity = float64(*req.MinSimilarity)
	}

	// Enforce minimum threshold of 0.90 for auto-merge (server-side enforcement)
	// Use a small epsilon to account for float32->float64 conversion precision loss
	const minThreshold = 0.899
	if minSimilarity < minThreshold {
		return nil, status.Errorf(codes.InvalidArgument, "min_similarity for auto-merge must be at least 0.90, got: %.2f", minSimilarity)
	}

	// Validate max bound
	if minSimilarity > 1.0 {
		return nil, status.Errorf(codes.InvalidArgument, "min_similarity must be between 0.90 and 1.0, got: %.2f", minSimilarity)
	}

	// Call repository
	result, err := s.repo.AutoMergeDuplicates(ctx, req.TenantId, minSimilarity, req.DryRun)
	if err != nil {
		s.logger.Error("Error auto-merging duplicates", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to auto-merge duplicates: %v", err)
	}

	// Convert to proto
	return autoMergeResultToProto(result, req.DryRun), nil
}

// Conversion helpers

func entityToProto(e *relationships.Entity) *relationshipv1.Entity {
	if e == nil {
		return nil
	}

	protoEntity := &relationshipv1.Entity{
		Id:            e.ID,
		Name:          e.Name,
		Type:          internalEntityTypeToProto(e.Type),
		Metadata:      e.Metadata,
		CreatedAt:     timestamppb.New(e.CreatedAt),
		UpdatedAt:     timestamppb.New(e.UpdatedAt),
		Aliases:       e.Aliases,
		Confidence:    float32(e.Confidence),
		SourceCount:   int32(e.SourceCount),
		RelationCount: int32(e.RelationCount),
		SentCount:     int32(e.SentCount),
		ReceivedCount: int32(e.ReceivedCount),
		FirstSeen:     timestamppb.New(e.FirstSeen),
		LastSeen:      timestamppb.New(e.LastSeen),
	}

	if e.CanonicalName != "" {
		protoEntity.CanonicalName = &e.CanonicalName
	}

	if e.CommunicationPatterns != "" {
		protoEntity.CommunicationPatterns = e.CommunicationPatterns
	}
	if len(e.ExpertiseAreas) > 0 {
		protoEntity.ExpertiseAreas = e.ExpertiseAreas
	}
	if e.OrgPosition != "" {
		protoEntity.OrgPosition = e.OrgPosition
	}

	return protoEntity
}

func relationshipToProto(r *relationships.Relationship) *relationshipv1.Relationship {
	if r == nil {
		return nil
	}

	sourceEntity := &relationshipv1.Entity{
		Id:   r.SourceID,
		Name: r.SourceName,
		Type: internalEntityTypeToProto(r.SourceType),
	}

	targetEntity := &relationshipv1.Entity{
		Id:   r.TargetID,
		Name: r.TargetName,
		Type: internalEntityTypeToProto(r.TargetType),
	}

	protoEvidence := make([]*relationshipv1.Evidence, len(r.Evidence))
	for i, ev := range r.Evidence {
		protoEvidence[i] = &relationshipv1.Evidence{
			SourceId:               ev.SourceID,
			SourceType:             ev.SourceType,
			Excerpt:                ev.Excerpt,
			DiscoveredAt:           timestamppb.New(ev.DiscoveredAt),
			ConfidenceContribution: float32(ev.ConfidenceContribution),
		}
	}

	return &relationshipv1.Relationship{
		Id:               r.ID,
		SourceEntity:     sourceEntity,
		TargetEntity:     targetEntity,
		RelationshipType: internalRelTypeToProto(r.Type),
		Confidence:       float32(r.Confidence),
		Evidence:         protoEvidence,
		Status:           internalRelStatusToProto(r.Status),
		TenantId:         r.TenantID,
		CreatedAt:        timestamppb.New(r.CreatedAt),
		UpdatedAt:        timestamppb.New(r.UpdatedAt),
	}
}

func conflictToProto(c *relationships.RelationshipConflict) *relationshipv1.Conflict {
	if c == nil {
		return nil
	}

	protoConflict := &relationshipv1.Conflict{
		Id:              c.ID,
		TenantId:        c.TenantID,
		Type:            c.Type,
		Description:     c.Description,
		SuggestedAction: c.SuggestedAction,
		Status:          internalConflictStatusToProto(c.Status),
		CreatedAt:       timestamppb.New(c.CreatedAt),
	}

	if c.Resolution != "" {
		protoConflict.Resolution = &c.Resolution
	}

	if c.ResolvedBy != "" {
		protoConflict.ResolvedBy = &c.ResolvedBy
	}

	if c.ResolvedAt != nil {
		protoConflict.ResolvedAt = timestamppb.New(*c.ResolvedAt)
	}

	protoRels := make([]*relationshipv1.Relationship, len(c.Relationships))
	for i, r := range c.Relationships {
		protoRels[i] = relationshipToProto(&r)
	}
	protoConflict.Relationships = protoRels

	return protoConflict
}

func internalEntityTypeToProto(t relationships.EntityType) relationshipv1.EntityType {
	switch t {
	case relationships.EntityTypePerson:
		return relationshipv1.EntityType_ENTITY_TYPE_PERSON
	case relationships.EntityTypeOrganization:
		return relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION
	case relationships.EntityTypeTopic:
		return relationshipv1.EntityType_ENTITY_TYPE_TOPIC
	case relationships.EntityTypeProject:
		return relationshipv1.EntityType_ENTITY_TYPE_PROJECT
	case relationships.EntityTypeLocation:
		return relationshipv1.EntityType_ENTITY_TYPE_LOCATION
	case relationships.EntityTypeProduct:
		return relationshipv1.EntityType_ENTITY_TYPE_PRODUCT
	default:
		return relationshipv1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

func protoEntityTypeToInternal(t relationshipv1.EntityType) relationships.EntityType {
	switch t {
	case relationshipv1.EntityType_ENTITY_TYPE_PERSON:
		return relationships.EntityTypePerson
	case relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION:
		return relationships.EntityTypeOrganization
	case relationshipv1.EntityType_ENTITY_TYPE_TOPIC:
		return relationships.EntityTypeTopic
	case relationshipv1.EntityType_ENTITY_TYPE_PROJECT:
		return relationships.EntityTypeProject
	case relationshipv1.EntityType_ENTITY_TYPE_LOCATION:
		return relationships.EntityTypeLocation
	case relationshipv1.EntityType_ENTITY_TYPE_PRODUCT:
		return relationships.EntityTypeProduct
	default:
		return ""
	}
}

func internalRelTypeToProto(t relationships.RelationshipType) relationshipv1.RelationshipType {
	switch t {
	case relationships.RelationshipTypeColleague:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS
	case relationships.RelationshipTypeReportsTo:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO
	case relationships.RelationshipTypeMemberOf:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF
	case relationships.RelationshipTypeWorksOn:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT
	case relationships.RelationshipTypeDiscusses:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED
	case relationships.RelationshipTypeMentions:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH
	case relationships.RelationshipTypeLocatedAt:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT
	case relationships.RelationshipTypeRelatedTo:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO
	case relationships.RelationshipTypeCollaborates:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH
	case relationships.RelationshipTypeKnows:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS
	default:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED
	}
}

func protoRelTypeToInternal(t relationshipv1.RelationshipType) relationships.RelationshipType {
	switch t {
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS:
		return relationships.RelationshipTypeColleague
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO:
		return relationships.RelationshipTypeReportsTo
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF:
		return relationships.RelationshipTypeMemberOf
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT:
		return relationships.RelationshipTypeWorksOn
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED:
		return relationships.RelationshipTypeDiscusses
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH:
		return relationships.RelationshipTypeMentions
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT:
		return relationships.RelationshipTypeLocatedAt
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO:
		return relationships.RelationshipTypeRelatedTo
	case relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH:
		return relationships.RelationshipTypeCollaborates
	default:
		return ""
	}
}

func internalRelStatusToProto(s relationships.RelationshipStatus) relationshipv1.RelationshipStatus {
	switch s {
	case relationships.RelationshipStatusDiscovered:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED
	case relationships.RelationshipStatusConfirmed:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
	case relationships.RelationshipStatusRejected:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_REJECTED
	case relationships.RelationshipStatusArchived:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED
	default:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_UNSPECIFIED
	}
}

func internalConflictStatusToProto(s relationships.ConflictStatus) relationshipv1.ConflictStatus {
	switch s {
	case relationships.ConflictStatusPending:
		return relationshipv1.ConflictStatus_CONFLICT_STATUS_PENDING
	case relationships.ConflictStatusResolved:
		return relationshipv1.ConflictStatus_CONFLICT_STATUS_RESOLVED
	default:
		return relationshipv1.ConflictStatus_CONFLICT_STATUS_UNSPECIFIED
	}
}

func protoConflictStatusToInternal(s relationshipv1.ConflictStatus) relationships.ConflictStatus {
	switch s {
	case relationshipv1.ConflictStatus_CONFLICT_STATUS_PENDING:
		return relationships.ConflictStatusPending
	case relationshipv1.ConflictStatus_CONFLICT_STATUS_RESOLVED:
		return relationships.ConflictStatusResolved
	default:
		return ""
	}
}

func protoConflictStrategyToInternal(s relationshipv1.ConflictResolutionStrategy) relationships.ConflictResolutionStrategy {
	switch s {
	case relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_KEEP_LATEST:
		return relationships.ConflictStrategyKeepLatest
	case relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_KEEP_FIRST:
		return relationships.ConflictStrategyKeepFirst
	case relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_MERGE:
		return relationships.ConflictStrategyMerge
	case relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_MANUAL:
		return relationships.ConflictStrategyManual
	default:
		return relationships.ConflictStrategyKeepLatest
	}
}

// Deduplication conversion helpers

func duplicatePairToProto(pair *relationships.DuplicatePair) *relationshipv1.DuplicatePair {
	if pair == nil {
		return nil
	}

	return &relationshipv1.DuplicatePair{
		EntityId1:   pair.EntityID1,
		EntityId2:   pair.EntityID2,
		EntityName1: pair.EntityName1,
		EntityName2: pair.EntityName2,
		Similarity:  float32(pair.Similarity),
		Signals:     pair.Signals,
	}
}

func mergePreviewToProto(preview *relationships.MergePreview) *relationshipv1.MergePreviewResponse {
	if preview == nil {
		return nil
	}

	// Convert relationship objects to just their IDs for the proto response
	relIDs := make([]string, len(preview.TransferringRelationships))
	for i, rel := range preview.TransferringRelationships {
		relIDs[i] = rel.ID
	}

	return &relationshipv1.MergePreviewResponse{
		MergedEntity:              entityToProto(preview.MergedEntity),
		TransferringAliases:       preview.TransferringAliases,
		TransferringRelationships: relIDs,
		ConflictFields:            preview.ConflictFields,
	}
}

func autoMergeResultToProto(result *relationships.AutoMergeResult, wasDryRun bool) *relationshipv1.AutoMergeDuplicatesResponse {
	if result == nil {
		return nil
	}

	// Convert merged pairs
	mergedPairs := make([]*relationshipv1.DuplicatePair, len(result.MergedPairs))
	for i, pair := range result.MergedPairs {
		mergedPairs[i] = duplicatePairToProto(&pair)
	}

	// Convert skipped pairs
	skippedPairs := make([]*relationshipv1.SkippedPair, len(result.SkippedPairs))
	for i, pair := range result.SkippedPairs {
		skippedPairs[i] = &relationshipv1.SkippedPair{
			Pair:   duplicatePairToProto(&pair),
			Reason: "merge_failed", // Generic reason, actual reason would come from the pair
		}
	}

	return &relationshipv1.AutoMergeDuplicatesResponse{
		MergedCount:  int32(result.MergedCount),
		MergedPairs:  mergedPairs,
		SkippedPairs: skippedPairs,
		WasDryRun:    wasDryRun,
	}
}
