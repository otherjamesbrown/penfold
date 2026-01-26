// Package reviewservice implements the ReviewService gRPC server.
package reviewservice

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
)

// Service implements the ReviewService gRPC server.
type Service struct {
	reviewv1.UnimplementedReviewServiceServer
	repo   *reviewqueue.Repository
	logger logging.Logger
}

// NewService creates a new review service.
func NewService(repo *reviewqueue.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// =============================================================================
// Session Management RPCs
// =============================================================================

// StartSession starts a new review session.
func (s *Service) StartSession(ctx context.Context, req *reviewv1.StartSessionRequest) (*reviewv1.StartSessionResponse, error) {
	s.logger.Debug("StartSession called")

	session, previousEnded, err := s.repo.StartSession(ctx)
	if err != nil {
		s.logger.Error("Error starting session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to start session: %v", err)
	}

	return &reviewv1.StartSessionResponse{
		Session:              sessionToProto(session),
		PreviousSessionEnded: previousEnded,
	}, nil
}

// PauseSession pauses the current active session.
func (s *Service) PauseSession(ctx context.Context, req *reviewv1.PauseSessionRequest) (*reviewv1.PauseSessionResponse, error) {
	s.logger.Debug("PauseSession called",
		logging.F("session_id", req.SessionId),
	)

	var sessionID int64
	if req.SessionId != "" {
		var err error
		sessionID, err = strconv.ParseInt(req.SessionId, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid session_id: %v", err)
		}
	}

	session, err := s.repo.PauseSession(ctx, sessionID)
	if err != nil {
		s.logger.Error("Error pausing session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to pause session: %v", err)
	}

	return &reviewv1.PauseSessionResponse{
		Session: sessionToProto(session),
	}, nil
}

// ResumeSession resumes a paused session.
func (s *Service) ResumeSession(ctx context.Context, req *reviewv1.ResumeSessionRequest) (*reviewv1.ResumeSessionResponse, error) {
	s.logger.Debug("ResumeSession called",
		logging.F("session_id", req.SessionId),
	)

	var sessionID int64
	if req.SessionId != "" {
		var err error
		sessionID, err = strconv.ParseInt(req.SessionId, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid session_id: %v", err)
		}
	}

	session, err := s.repo.ResumeSession(ctx, sessionID)
	if err != nil {
		s.logger.Error("Error resuming session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resume session: %v", err)
	}

	return &reviewv1.ResumeSessionResponse{
		Session: sessionToProto(session),
	}, nil
}

// EndSession ends the current session.
func (s *Service) EndSession(ctx context.Context, req *reviewv1.EndSessionRequest) (*reviewv1.EndSessionResponse, error) {
	s.logger.Debug("EndSession called",
		logging.F("session_id", req.SessionId),
	)

	var sessionID int64
	if req.SessionId != "" {
		var err error
		sessionID, err = strconv.ParseInt(req.SessionId, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid session_id: %v", err)
		}
	}

	session, err := s.repo.EndSession(ctx, sessionID)
	if err != nil {
		s.logger.Error("Error ending session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to end session: %v", err)
	}

	summary := fmt.Sprintf("Session ended. Reviewed %d items: %d approved, %d rejected, %d deferred.",
		session.TotalReviewed, session.ApprovedCount, session.RejectedCount, session.DeferredCount)

	return &reviewv1.EndSessionResponse{
		Session: sessionToProto(session),
		Summary: summary,
	}, nil
}

// GetCurrentSession retrieves the current active or paused session.
func (s *Service) GetCurrentSession(ctx context.Context, req *reviewv1.GetCurrentSessionRequest) (*reviewv1.GetCurrentSessionResponse, error) {
	s.logger.Debug("GetCurrentSession called")

	session, err := s.repo.GetCurrentSession(ctx)
	if err != nil {
		s.logger.Error("Error getting current session", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get current session: %v", err)
	}

	return &reviewv1.GetCurrentSessionResponse{
		Session:    sessionToProto(session),
		HasSession: session != nil,
	}, nil
}

// GetSessionHistory retrieves past review sessions.
func (s *Service) GetSessionHistory(ctx context.Context, req *reviewv1.GetSessionHistoryRequest) (*reviewv1.GetSessionHistoryResponse, error) {
	s.logger.Debug("GetSessionHistory called",
		logging.F("limit", req.Limit),
		logging.F("offset", req.Offset),
	)

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	sessions, totalCount, err := s.repo.ListSessions(ctx, limit, int(req.Offset))
	if err != nil {
		s.logger.Error("Error listing sessions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list sessions: %v", err)
	}

	protoSessions := make([]*reviewv1.ReviewSession, len(sessions))
	for i, sess := range sessions {
		protoSessions[i] = sessionToProto(sess)
	}

	return &reviewv1.GetSessionHistoryResponse{
		Sessions:   protoSessions,
		TotalCount: totalCount,
	}, nil
}

// =============================================================================
// Review Item RPCs (stubs for now, to be connected to review_queue)
// =============================================================================

// GetDailyReview retrieves today's review items grouped by category.
func (s *Service) GetDailyReview(ctx context.Context, req *reviewv1.GetDailyReviewRequest) (*reviewv1.GetDailyReviewResponse, error) {
	s.logger.Debug("GetDailyReview called")

	// Get pending items from review queue
	filter := reviewqueue.ReviewFilter{
		Status: reviewqueue.StatusPending,
		Limit:  100,
	}

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing review items", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list review items: %v", err)
	}

	// Group items by question type as category
	categoryMap := make(map[string][]*reviewv1.ReviewItem)
	highPriorityCount := 0

	for _, item := range items {
		protoItem := reviewItemToProto(item)
		category := string(item.QuestionType)
		categoryMap[category] = append(categoryMap[category], protoItem)
		if item.Priority == reviewqueue.PriorityHigh {
			highPriorityCount++
		}
	}

	// Convert to categories
	var categories []*reviewv1.ReviewCategory
	for name, categoryItems := range categoryMap {
		categories = append(categories, &reviewv1.ReviewCategory{
			Name:        name,
			DisplayName: categoryDisplayName(name),
			Items:       categoryItems,
			ItemCount:   int32(len(categoryItems)),
		})
	}

	return &reviewv1.GetDailyReviewResponse{
		Review: &reviewv1.DailyReview{
			Categories:        categories,
			TotalPending:      int32(len(items)),
			HighPriorityCount: int32(highPriorityCount),
		},
	}, nil
}

// GetReviewItem retrieves a single review item by ID.
func (s *Service) GetReviewItem(ctx context.Context, req *reviewv1.GetReviewItemRequest) (*reviewv1.GetReviewItemResponse, error) {
	s.logger.Debug("GetReviewItem called", logging.F("id", req.Id))

	id, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid item id: %v", err)
	}

	item, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.Error("Error getting review item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get review item: %v", err)
	}
	if item == nil {
		return nil, status.Errorf(codes.NotFound, "review item with id %s not found", req.Id)
	}

	return &reviewv1.GetReviewItemResponse{
		Item: reviewItemToProto(item),
	}, nil
}

// ListReviewItems returns a filtered list of review items.
func (s *Service) ListReviewItems(ctx context.Context, req *reviewv1.ListReviewItemsRequest) (*reviewv1.ListReviewItemsResponse, error) {
	s.logger.Debug("ListReviewItems called",
		logging.F("page_size", req.PageSize),
	)

	filter := reviewqueue.ReviewFilter{
		Limit: int(req.PageSize),
	}

	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	// Map statuses
	if len(req.Statuses) > 0 {
		// Use the first status for now (simplified)
		filter.Status = protoStatusToInternal(req.Statuses[0])
	}

	// Map priorities
	if len(req.Priorities) > 0 {
		// Use the first priority for now (simplified)
		filter.Priority = protoPriorityToInternal(req.Priorities[0])
	}

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing review items", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list review items: %v", err)
	}

	protoItems := make([]*reviewv1.ReviewItem, len(items))
	for i, item := range items {
		protoItems[i] = reviewItemToProto(item)
	}

	totalCount := int64(len(items))
	return &reviewv1.ListReviewItemsResponse{
		Items:      protoItems,
		TotalCount: &totalCount,
	}, nil
}

// ApproveItem marks a review item as approved.
func (s *Service) ApproveItem(ctx context.Context, req *reviewv1.ApproveItemRequest) (*reviewv1.ApproveItemResponse, error) {
	s.logger.Debug("ApproveItem called", logging.F("id", req.Id))

	id, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid item id: %v", err)
	}

	// Resolve the item (approve = resolve with positive note)
	note := req.Note
	if note == "" {
		note = "Approved"
	}
	err = s.repo.Resolve(ctx, id, note, "grpc-user")
	if err != nil {
		s.logger.Error("Error approving item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to approve item: %v", err)
	}

	// Update session stats if there's an active session
	currentSession, _ := s.repo.GetCurrentSession(ctx)
	if currentSession != nil && currentSession.Status == reviewqueue.SessionStatusActive {
		_ = s.repo.IncrementSessionStats(ctx, currentSession.ID, 1, 0, 0)
	}

	// Get updated item
	item, _ := s.repo.Get(ctx, id)

	return &reviewv1.ApproveItemResponse{
		Item: reviewItemToProto(item),
		Action: &reviewv1.ReviewAction{
			Id:         fmt.Sprintf("action-%d", id),
			ItemId:     req.Id,
			ActionType: reviewv1.ActionType_ACTION_TYPE_APPROVE,
			NewStatus:  reviewv1.ReviewStatus_REVIEW_STATUS_APPROVED,
			Undoable:   false,
		},
	}, nil
}

// RejectItem marks a review item as rejected.
func (s *Service) RejectItem(ctx context.Context, req *reviewv1.RejectItemRequest) (*reviewv1.RejectItemResponse, error) {
	s.logger.Debug("RejectItem called", logging.F("id", req.Id))

	id, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid item id: %v", err)
	}

	// Dismiss the item (reject = dismiss)
	err = s.repo.Dismiss(ctx, id, req.Reason, "grpc-user")
	if err != nil {
		s.logger.Error("Error rejecting item", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to reject item: %v", err)
	}

	// Update session stats if there's an active session
	currentSession, _ := s.repo.GetCurrentSession(ctx)
	if currentSession != nil && currentSession.Status == reviewqueue.SessionStatusActive {
		_ = s.repo.IncrementSessionStats(ctx, currentSession.ID, 0, 1, 0)
	}

	// Get updated item
	item, _ := s.repo.Get(ctx, id)

	return &reviewv1.RejectItemResponse{
		Item: reviewItemToProto(item),
		Action: &reviewv1.ReviewAction{
			Id:         fmt.Sprintf("action-%d", id),
			ItemId:     req.Id,
			ActionType: reviewv1.ActionType_ACTION_TYPE_REJECT,
			NewStatus:  reviewv1.ReviewStatus_REVIEW_STATUS_REJECTED,
			Reason:     req.Reason,
			Undoable:   false,
		},
	}, nil
}

// UndoAction reverses the last action on a review item.
func (s *Service) UndoAction(ctx context.Context, req *reviewv1.UndoActionRequest) (*reviewv1.UndoActionResponse, error) {
	s.logger.Debug("UndoAction called", logging.F("id", req.Id))

	// Note: Full undo implementation would require an action history table
	// For now, return unimplemented
	return nil, status.Error(codes.Unimplemented, "undo is not yet implemented")
}

// GetReviewStats retrieves review statistics.
func (s *Service) GetReviewStats(ctx context.Context, req *reviewv1.GetReviewStatsRequest) (*reviewv1.GetReviewStatsResponse, error) {
	s.logger.Debug("GetReviewStats called")

	stats, err := s.repo.GetStats(ctx)
	if err != nil {
		s.logger.Error("Error getting stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	byPriority := make(map[string]int64)
	for k, v := range stats.ByPriority {
		byPriority[k] = int64(v)
	}

	byType := make(map[string]int64)
	for k, v := range stats.ByType {
		byType[k] = int64(v)
	}

	return &reviewv1.GetReviewStatsResponse{
		PendingCount:  int64(stats.TotalPending),
		ByPriority:    byPriority,
		ByContentType: byType,
	}, nil
}

// =============================================================================
// Conversion helpers
// =============================================================================

func sessionToProto(s *reviewqueue.Session) *reviewv1.ReviewSession {
	if s == nil {
		return nil
	}

	session := &reviewv1.ReviewSession{
		Id:                    fmt.Sprintf("%d", s.ID),
		Status:                sessionStatusToProto(s.Status),
		StartedAt:             timestamppb.New(s.StartedAt),
		TotalReviewed:         int32(s.TotalReviewed),
		ApprovedCount:         int32(s.ApprovedCount),
		RejectedCount:         int32(s.RejectedCount),
		DeferredCount:         int32(s.DeferredCount),
		ActiveDurationSeconds: s.ActiveDurationSeconds,
	}

	if s.PausedAt != nil {
		session.PausedAt = timestamppb.New(*s.PausedAt)
	}
	if s.EndedAt != nil {
		session.EndedAt = timestamppb.New(*s.EndedAt)
	}

	return session
}

func sessionStatusToProto(s reviewqueue.SessionStatus) reviewv1.SessionStatus {
	switch s {
	case reviewqueue.SessionStatusActive:
		return reviewv1.SessionStatus_SESSION_STATUS_ACTIVE
	case reviewqueue.SessionStatusPaused:
		return reviewv1.SessionStatus_SESSION_STATUS_PAUSED
	case reviewqueue.SessionStatusEnded:
		return reviewv1.SessionStatus_SESSION_STATUS_ENDED
	default:
		return reviewv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

func reviewItemToProto(item *reviewqueue.ReviewItem) *reviewv1.ReviewItem {
	if item == nil {
		return nil
	}

	protoItem := &reviewv1.ReviewItem{
		Id:             fmt.Sprintf("%d", item.ID),
		ContentType:    string(item.QuestionType),
		ContentSummary: item.Question,
		Priority:       internalPriorityToProto(item.Priority),
		Status:         internalStatusToProto(item.Status),
		CreatedAt:      timestamppb.New(item.CreatedAt),
		UpdatedAt:      timestamppb.New(item.UpdatedAt),
		Category:       string(item.QuestionType),
	}

	if item.SourceType != nil {
		protoItem.Source = *item.SourceType
	}

	return protoItem
}

func internalPriorityToProto(p reviewqueue.Priority) reviewv1.Priority {
	switch p {
	case reviewqueue.PriorityHigh:
		return reviewv1.Priority_PRIORITY_HIGH
	case reviewqueue.PriorityMedium:
		return reviewv1.Priority_PRIORITY_MEDIUM
	case reviewqueue.PriorityLow:
		return reviewv1.Priority_PRIORITY_LOW
	default:
		return reviewv1.Priority_PRIORITY_UNSPECIFIED
	}
}

func protoPriorityToInternal(p reviewv1.Priority) reviewqueue.Priority {
	switch p {
	case reviewv1.Priority_PRIORITY_HIGH, reviewv1.Priority_PRIORITY_URGENT:
		return reviewqueue.PriorityHigh
	case reviewv1.Priority_PRIORITY_MEDIUM:
		return reviewqueue.PriorityMedium
	case reviewv1.Priority_PRIORITY_LOW:
		return reviewqueue.PriorityLow
	default:
		return ""
	}
}

func internalStatusToProto(s reviewqueue.Status) reviewv1.ReviewStatus {
	switch s {
	case reviewqueue.StatusPending:
		return reviewv1.ReviewStatus_REVIEW_STATUS_PENDING
	case reviewqueue.StatusResolved:
		return reviewv1.ReviewStatus_REVIEW_STATUS_APPROVED
	case reviewqueue.StatusDismissed:
		return reviewv1.ReviewStatus_REVIEW_STATUS_REJECTED
	case reviewqueue.StatusDeferred:
		return reviewv1.ReviewStatus_REVIEW_STATUS_DEFERRED
	default:
		return reviewv1.ReviewStatus_REVIEW_STATUS_UNSPECIFIED
	}
}

func protoStatusToInternal(s reviewv1.ReviewStatus) reviewqueue.Status {
	switch s {
	case reviewv1.ReviewStatus_REVIEW_STATUS_PENDING:
		return reviewqueue.StatusPending
	case reviewv1.ReviewStatus_REVIEW_STATUS_APPROVED:
		return reviewqueue.StatusResolved
	case reviewv1.ReviewStatus_REVIEW_STATUS_REJECTED:
		return reviewqueue.StatusDismissed
	case reviewv1.ReviewStatus_REVIEW_STATUS_DEFERRED:
		return reviewqueue.StatusDeferred
	default:
		return ""
	}
}

func categoryDisplayName(name string) string {
	switch name {
	case "acronym":
		return "Acronyms"
	case "person":
		return "Person References"
	case "entity":
		return "Entities"
	case "duplicate":
		return "Duplicates"
	case "other":
		return "Other"
	default:
		return name
	}
}
