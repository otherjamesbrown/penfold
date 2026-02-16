// Package assertionsservice implements the AssertionsService gRPC server.
package assertionsservice

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	"github.com/otherjamesbrown/penfold/pkg/assertions"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the AssertionsService gRPC server.
type Service struct {
	assertionsv1.UnimplementedAssertionsServiceServer
	repo   *assertions.Repository
	logger logging.Logger
}

// NewService creates a new assertions service.
func NewService(repo *assertions.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListAssertions lists assertions with filters.
func (s *Service) ListAssertions(ctx context.Context, req *assertionsv1.ListAssertionsRequest) (*assertionsv1.ListAssertionsResponse, error) {
	s.logger.Debug("ListAssertions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("type", req.AssertionType),
		logging.F("attributed_to", req.AttributedTo))

	// Validate tenant ID
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Build filters
	filters := assertions.ListFilters{
		Limit:  req.Limit,
		Offset: req.Offset,
	}

	if req.AssertionType != nil {
		filters.Type = req.AssertionType
	}

	if req.Since != nil {
		since := req.Since.AsTime()
		filters.Since = &since
	}

	if req.Until != nil {
		until := req.Until.AsTime()
		filters.Until = &until
	}

	if req.AttributedTo != nil {
		filters.AttributedTo = req.AttributedTo
	}

	if req.ProjectId != nil {
		filters.ProjectID = req.ProjectId
	}

	// Query assertions
	results, totalCount, err := s.repo.ListAssertions(ctx, req.TenantId, filters)
	if err != nil {
		s.logger.Error("Error listing assertions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list assertions: %v", err)
	}

	// Convert to proto
	protoAssertions := make([]*assertionsv1.AssertionDetail, len(results))
	for i, result := range results {
		protoAssertions[i] = assertionResultToProto(result, req.ShowSource)
	}

	return &assertionsv1.ListAssertionsResponse{
		Assertions: protoAssertions,
		TotalCount: totalCount,
	}, nil
}

// SearchAssertions searches assertions by keyword.
func (s *Service) SearchAssertions(ctx context.Context, req *assertionsv1.SearchAssertionsRequest) (*assertionsv1.SearchAssertionsResponse, error) {
	s.logger.Debug("SearchAssertions called",
		logging.F("tenant_id", req.TenantId),
		logging.F("query", req.Query))

	// Validate required fields
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	// Build filters
	filters := assertions.ListFilters{
		Limit:  req.Limit,
		Offset: req.Offset,
	}

	if req.AssertionType != nil {
		filters.Type = req.AssertionType
	}

	if req.Since != nil {
		since := req.Since.AsTime()
		filters.Since = &since
	}

	if req.Until != nil {
		until := req.Until.AsTime()
		filters.Until = &until
	}

	// Search assertions
	results, totalCount, err := s.repo.SearchAssertions(ctx, req.TenantId, req.Query, filters)
	if err != nil {
		s.logger.Error("Error searching assertions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to search assertions: %v", err)
	}

	// Convert to proto
	protoAssertions := make([]*assertionsv1.AssertionDetail, len(results))
	for i, result := range results {
		protoAssertions[i] = assertionResultToProto(result, req.ShowSource)
	}

	return &assertionsv1.SearchAssertionsResponse{
		Assertions: protoAssertions,
		TotalCount: totalCount,
	}, nil
}

// GetAssertionSummary returns aggregate statistics for assertions.
func (s *Service) GetAssertionSummary(ctx context.Context, req *assertionsv1.GetAssertionSummaryRequest) (*assertionsv1.AssertionSummaryResponse, error) {
	s.logger.Debug("GetAssertionSummary called",
		logging.F("tenant_id", req.TenantId),
		logging.F("group_by", req.GroupBy))

	// Validate tenant ID
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Default group_by to "type"
	groupBy := req.GroupBy
	if groupBy == "" {
		groupBy = "type"
	}

	// Convert timestamps
	var since, until *time.Time
	if req.Since != nil {
		t := req.Since.AsTime()
		since = &t
	}

	if req.Until != nil {
		t := req.Until.AsTime()
		until = &t
	}

	// Get summary
	entries, totalAssertions, err := s.repo.GetAssertionSummary(ctx, req.TenantId, since, until, groupBy)
	if err != nil {
		s.logger.Error("Error getting assertion summary", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get assertion summary: %v", err)
	}

	// Convert to proto
	protoEntries := make([]*assertionsv1.SummaryEntry, len(entries))
	for i, entry := range entries {
		protoEntries[i] = &assertionsv1.SummaryEntry{
			Key:      entry.Key,
			Count:    entry.Count,
			ThisWeek: entry.ThisWeek,
			LastWeek: entry.LastWeek,
		}
	}

	return &assertionsv1.AssertionSummaryResponse{
		Entries:          protoEntries,
		TotalAssertions:  totalAssertions,
	}, nil
}

// assertionResultToProto converts an AssertionResult to proto.
func assertionResultToProto(result *assertions.AssertionResult, includeSource bool) *assertionsv1.AssertionDetail {
	var confidence float32
	if result.Confidence != nil {
		confidence = *result.Confidence
	}
	detail := &assertionsv1.AssertionDetail{
		Id:            result.ID,
		AssertionType: result.AssertionType,
		Description:   result.Description,
		SourceQuote:   result.SourceQuote,
		Confidence:    confidence,
		CreatedAt:     timestamppb.New(result.CreatedAt),
	}

	// Add attributions
	if len(result.Attributions) > 0 {
		detail.AttributedTo = make([]*assertionsv1.Attribution, len(result.Attributions))
		for i, attr := range result.Attributions {
			detail.AttributedTo[i] = &assertionsv1.Attribution{
				EntityId: attr.EntityID,
				Name:     attr.Name,
				Role:     attr.Role,
			}
		}
	}

	// Add source context if requested
	if includeSource && result.ContentID != nil {
		sourceType := ""
		if result.SourceType != nil {
			sourceType = *result.SourceType
		}
		detail.Source = &assertionsv1.SourceContext{
			ContentId:  *result.ContentID,
			SourceType: sourceType,
			Subject:    result.Subject,
		}
		if result.SourceDate != nil {
			detail.Source.Date = timestamppb.New(*result.SourceDate)
		}
		if result.FromName != nil {
			detail.Source.From = result.FromName
		}
	}

	return detail
}
