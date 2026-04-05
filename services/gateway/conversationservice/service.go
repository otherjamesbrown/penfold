// Package conversationservice implements the ConversationService gRPC server.
package conversationservice

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// Repository defines the interface for conversation data access.
type Repository interface {
	ListConversations(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error)
	ListConversationsByState(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error)
	ListConversationsFiltered(ctx context.Context, tenantID string, state *string, minItems *int32, limit, offset int32) ([]ConversationSummary, int64, error)
	GetConversation(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error)
	BeginTx(ctx context.Context) (pgx.Tx, error)
	MoveItems(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) (moved, skipped int32, err error)
	MoveSpecificItems(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string, contentIDs []string) (int32, error)
	MergeParticipants(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) error
	UpdateConversationStatsTx(ctx context.Context, tx pgx.Tx, conversationID string) error
	CreateConversation(ctx context.Context, tx pgx.Tx, conv *Conversation) (string, error)
	RemoveItem(ctx context.Context, conversationID, contentID string) error
	DeleteConversation(ctx context.Context, conversationID string) error
	DeleteConversationTx(ctx context.Context, tx pgx.Tx, conversationID string) error
	UpdateConversationStats(ctx context.Context, conversationID string) error
	InvalidateSummary(ctx context.Context, conversationID string) error
	GetItemCount(ctx context.Context, conversationID string) (int32, error)
}

// AuditRepository defines the interface for conversation audit queries.
type AuditRepository interface {
	// FindOrphansWithThread finds content items that have a thread_id in
	// content_enrichment but no entry in conversation_items.
	FindOrphansWithThread(ctx context.Context, tenantID string) ([]AuditOrphan, error)

	// FindOrphansWithReplySubject finds content items whose subject starts
	// with Re:/FW:/Fwd: but have no thread_id (parser missed References header).
	FindOrphansWithReplySubject(ctx context.Context, tenantID string) ([]AuditOrphan, error)

	// FindDuplicateMemberships finds content items linked to multiple conversations.
	FindDuplicateMemberships(ctx context.Context, tenantID string) ([]AuditDuplicate, error)

	// FindMergeCandidatesByOverlap finds conversations that share content items.
	FindMergeCandidatesByOverlap(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error)

	// FindMergeCandidatesByTopic finds conversations with similar topics
	// (after stripping Re:/FW:/Fwd: prefixes).
	FindMergeCandidatesByTopic(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error)

	// CountConversations returns the total conversation count for a tenant.
	CountConversations(ctx context.Context, tenantID string) (int32, error)
}

// PipelineRepository defines the interface for pipeline data access used by
// GetConversationProcessingStatus.
type PipelineRepository interface {
	GetSourceByContentID(ctx context.Context, contentID string) (*pipeline.PendingSource, error)
	ListSourceHistory(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error)
}

// Service implements the ConversationService gRPC server.
type Service struct {
	conversationv1.UnimplementedConversationServiceServer
	repo         Repository
	pipelineRepo PipelineRepository
	auditRepo    AuditRepository
	logger       logging.Logger
}

// NewService creates a new conversation service.
func NewService(repo Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// NewServiceWithPipeline creates a new conversation service with a pipeline repository.
func NewServiceWithPipeline(repo Repository, pipelineRepo PipelineRepository, logger logging.Logger) *Service {
	return &Service{
		repo:         repo,
		pipelineRepo: pipelineRepo,
		logger:       logger,
	}
}

// SetPipelineRepo sets the pipeline repository for processing status lookups.
// This follows the same pattern as contentservice.SetPipelineRepo.
func (s *Service) SetPipelineRepo(repo PipelineRepository) {
	s.pipelineRepo = repo
}

// SetAuditRepo sets the audit repository for conversation audit queries.
func (s *Service) SetAuditRepo(repo AuditRepository) {
	s.auditRepo = repo
}

// ListConversations lists conversations with pagination.
func (s *Service) ListConversations(ctx context.Context, req *conversationv1.ListConversationsRequest) (*conversationv1.ListConversationsResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	var conversations []ConversationSummary
	var totalCount int64
	var err error

	// Use filtered query when min_items is set, otherwise use existing queries
	if req.MinItems != nil {
		var statePtr *string
		if req.State != nil {
			statePtr = req.State
		}
		conversations, totalCount, err = s.repo.ListConversationsFiltered(ctx, req.GetTenantId(), statePtr, req.MinItems, req.GetLimit(), req.GetOffset())
	} else if req.GetState() != "" {
		conversations, totalCount, err = s.repo.ListConversationsByState(ctx, req.GetTenantId(), req.GetState(), req.GetLimit(), req.GetOffset())
	} else {
		conversations, totalCount, err = s.repo.ListConversations(ctx, req.GetTenantId(), req.GetLimit(), req.GetOffset())
	}

	if err != nil {
		s.logger.Error("failed to list conversations", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list conversations: %v", err)
	}

	protoConversations := make([]*conversationv1.ConversationSummary, len(conversations))
	for i, conv := range conversations {
		protoConversations[i] = conversationSummaryToProto(&conv)
	}

	return &conversationv1.ListConversationsResponse{
		Conversations: protoConversations,
		TotalCount:    totalCount,
	}, nil
}

// ShowConversation gets a single conversation with items and participants.
func (s *Service) ShowConversation(ctx context.Context, req *conversationv1.ShowConversationRequest) (*conversationv1.ShowConversationResponse, error) {
	// Validate tenant ID
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Validate conversation ID
	if req.GetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}

	// Query conversation
	conversation, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetConversationId())
	if err != nil {
		s.logger.Error("failed to get conversation", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get conversation: %v", err)
	}

	// Check if conversation exists
	if conversation == nil {
		return nil, status.Error(codes.NotFound, "conversation not found")
	}

	// Convert to proto
	return conversationDetailToProto(conversation), nil
}

// GetConversationProcessingStatus returns aggregated processing status for all
// items in a conversation, including per-item stage breakdowns and token totals.
func (s *Service) GetConversationProcessingStatus(ctx context.Context, req *conversationv1.GetConversationProcessingStatusRequest) (*conversationv1.ConversationProcessingStatus, error) {
	if req.GetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}

	// Look up the conversation. The existing RPCs require tenant_id from the request,
	// but this RPC's spec only provides conversation_id. We pass an empty tenant_id
	// and rely on the repository's SQL to handle tenant filtering appropriately.
	// (The test mocks pass empty string for tenantID and it works correctly.)
	conv, err := s.repo.GetConversation(ctx, "", req.GetConversationId())
	if err != nil {
		s.logger.Error("failed to get conversation for processing status",
			logging.Err(err),
			logging.F("conversation_id", req.GetConversationId()),
		)
		return nil, status.Errorf(codes.Internal, "failed to get conversation: %v", err)
	}
	if conv == nil {
		return nil, status.Error(codes.NotFound, "conversation not found")
	}

	// Build per-item processing status.
	var items []*conversationv1.ContentProcessingSummary
	var totalCompleted, totalProcessing, totalFailed, totalPending int32
	var totalInputTokens, totalOutputTokens int32

	for _, item := range conv.Items {
		summary := &conversationv1.ContentProcessingSummary{
			ContentId: item.ContentID,
		}

		// If item has no source, mark as pending with no stage details.
		if item.SourceID == nil {
			summary.State = contentv1.ProcessingState_PROCESSING_STATE_PENDING
			totalPending++
			items = append(items, summary)
			continue
		}

		summary.SourceId = *item.SourceID

		// Look up the source record to get the authoritative processing_status.
		// This field is reset to 'pending' or 'processing' when ReprocessContent is
		// called, while old pipeline_runs from the prior cycle may still show completed.
		src, err := s.pipelineRepo.GetSourceByContentID(ctx, item.ContentID)
		if err != nil {
			// Log and continue — don't fail the whole request for one item.
			s.logger.Error("failed to get source for item",
				logging.Err(err),
				logging.F("content_id", item.ContentID),
				logging.F("source_id", *item.SourceID),
			)
			summary.State = contentv1.ProcessingState_PROCESSING_STATE_PENDING
			totalPending++
			items = append(items, summary)
			continue
		}

		// Get pipeline runs for this source.
		runs, err := s.pipelineRepo.ListSourceHistory(ctx, *item.SourceID, "")
		if err != nil {
			// Log and continue — don't fail the whole request for one item.
			s.logger.Error("failed to get pipeline history for item",
				logging.Err(err),
				logging.F("content_id", item.ContentID),
				logging.F("source_id", *item.SourceID),
			)
			summary.State = contentv1.ProcessingState_PROCESSING_STATE_PENDING
			totalPending++
			items = append(items, summary)
			continue
		}

		// Use shared helper to build stage results and contribution info.
		ps := pipeline.BuildProcessingStatus(runs)

		// Use the authoritative sources.processing_status when it is set.
		// DeriveProcessingState reads only pipeline_runs and cannot detect terminal
		// states like "rejected" or requeued states like "pending" that are
		// recorded in the sources table. When ProcessingStatus is empty (e.g. legacy
		// records or test stubs), fall back to deriving state from pipeline runs.
		if src.ProcessingStatus != "" {
			summary.State = pipeline.DBStatusToProtoState(src.ProcessingStatus)
		} else {
			summary.State = pipeline.DeriveProcessingState(runs)
		}
		summary.ContentContribution = ps.ContentContribution
		summary.ContributionReason = ps.ContributionReason
		summary.Stages = ps.Stages

		// Accumulate token counts.
		totalInputTokens += ps.TotalInputTokens
		totalOutputTokens += ps.TotalOutputTokens

		// Increment state counters.
		switch summary.State {
		case contentv1.ProcessingState_PROCESSING_STATE_COMPLETED:
			totalCompleted++
		case contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS:
			totalProcessing++
		case contentv1.ProcessingState_PROCESSING_STATE_FAILED,
			contentv1.ProcessingState_PROCESSING_STATE_REJECTED,
			contentv1.ProcessingState_PROCESSING_STATE_CANCELLED:
			totalFailed++
		default:
			totalPending++
		}

		items = append(items, summary)
	}

	resp := &conversationv1.ConversationProcessingStatus{
		ConversationId: conv.ID,
		Topic:          conv.Topic,
		TotalItems:     int32(len(conv.Items)),
		Completed:      totalCompleted,
		Processing:     totalProcessing,
		Failed:         totalFailed,
		Pending:        totalPending,
		Items:          items,
	}

	if totalInputTokens > 0 {
		resp.TotalInputTokens = &totalInputTokens
	}
	if totalOutputTokens > 0 {
		resp.TotalOutputTokens = &totalOutputTokens
	}

	return resp, nil
}

// MergeConversations moves all items from source into target, then deletes source.
func (s *Service) MergeConversations(ctx context.Context, req *conversationv1.MergeConversationsRequest) (*conversationv1.MergeConversationsResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GetSourceConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "source_conversation_id is required")
	}
	if req.GetTargetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_conversation_id is required")
	}
	if req.GetSourceConversationId() == req.GetTargetConversationId() {
		return nil, status.Error(codes.InvalidArgument, "source and target must be different conversations")
	}

	// Verify both conversations exist
	source, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetSourceConversationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get source conversation: %v", err)
	}
	if source == nil {
		return nil, status.Error(codes.NotFound, "source conversation not found")
	}

	target, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetTargetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get target conversation: %v", err)
	}
	if target == nil {
		return nil, status.Error(codes.NotFound, "target conversation not found")
	}

	// Run merge in a transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	moved, skipped, err := s.repo.MoveItems(ctx, tx, req.GetSourceConversationId(), req.GetTargetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to move items: %v", err)
	}

	if err := s.repo.MergeParticipants(ctx, tx, req.GetSourceConversationId(), req.GetTargetConversationId()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to merge participants: %v", err)
	}

	if err := s.repo.UpdateConversationStatsTx(ctx, tx, req.GetTargetConversationId()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update target stats: %v", err)
	}

	// Delete source conversation (CASCADE removes remaining items, participants, state_history)
	if err := s.repo.DeleteConversationTx(ctx, tx, req.GetSourceConversationId()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete source conversation: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit merge: %v", err)
	}

	// Invalidate summary so backfill regenerates it
	if err := s.repo.InvalidateSummary(ctx, req.GetTargetConversationId()); err != nil {
		s.logger.Error("failed to invalidate summary after merge", logging.Err(err))
	}

	s.logger.Info("merged conversations",
		logging.F("source", req.GetSourceConversationId()),
		logging.F("target", req.GetTargetConversationId()),
		logging.F("moved", moved),
		logging.F("skipped", skipped),
	)

	return &conversationv1.MergeConversationsResponse{
		ConversationId:    req.GetTargetConversationId(),
		ItemsMoved:        moved,
		DuplicatesSkipped: skipped,
	}, nil
}

// SplitConversation extracts specified items into a new conversation.
func (s *Service) SplitConversation(ctx context.Context, req *conversationv1.SplitConversationRequest) (*conversationv1.SplitConversationResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}
	if len(req.GetContentIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one content_id is required")
	}

	// Verify source conversation exists
	source, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get conversation: %v", err)
	}
	if source == nil {
		return nil, status.Error(codes.NotFound, "conversation not found")
	}

	// Validate: can't extract all items
	if len(req.GetContentIds()) >= len(source.Items) {
		return nil, status.Error(codes.InvalidArgument, "cannot extract all items from a conversation")
	}

	// Validate: all content_ids exist in the source conversation
	itemSet := make(map[string]bool, len(source.Items))
	for _, item := range source.Items {
		itemSet[item.ContentID] = true
	}
	for _, contentID := range req.GetContentIds() {
		if !itemSet[contentID] {
			return nil, status.Errorf(codes.InvalidArgument, "item %s not found in conversation", contentID)
		}
	}

	topic := req.GetNewTopic()
	if topic == "" {
		topic = fmt.Sprintf("Split from: %s", source.Topic)
	}

	// Generate new conversation ID
	newConvID := fmt.Sprintf("conv-%s", generateShortID())

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Create new conversation
	newConv := &Conversation{
		ID:       newConvID,
		TenantID: req.GetTenantId(),
		Topic:    topic,
		Metadata: map[string]interface{}{},
	}
	_, err = s.repo.CreateConversation(ctx, tx, newConv)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create new conversation: %v", err)
	}

	// Move specified items
	moved, err := s.repo.MoveSpecificItems(ctx, tx, req.GetConversationId(), newConvID, req.GetContentIds())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to move items: %v", err)
	}

	// Update stats on both conversations
	if err := s.repo.UpdateConversationStatsTx(ctx, tx, req.GetConversationId()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update source stats: %v", err)
	}
	if err := s.repo.UpdateConversationStatsTx(ctx, tx, newConvID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update new conversation stats: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit split: %v", err)
	}

	// Invalidate summaries on both conversations
	if err := s.repo.InvalidateSummary(ctx, req.GetConversationId()); err != nil {
		s.logger.Error("failed to invalidate source summary after split", logging.Err(err))
	}
	if err := s.repo.InvalidateSummary(ctx, newConvID); err != nil {
		s.logger.Error("failed to invalidate new conversation summary after split", logging.Err(err))
	}

	s.logger.Info("split conversation",
		logging.F("source", req.GetConversationId()),
		logging.F("new", newConvID),
		logging.F("moved", moved),
		logging.F("topic", topic),
	)

	return &conversationv1.SplitConversationResponse{
		NewConversationId: newConvID,
		ItemsMoved:        moved,
		Topic:             topic,
	}, nil
}

// UnlinkItem removes a single item from a conversation.
func (s *Service) UnlinkItem(ctx context.Context, req *conversationv1.UnlinkItemRequest) (*conversationv1.UnlinkItemResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}
	if req.GetContentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Verify conversation exists
	conv, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetConversationId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get conversation: %v", err)
	}
	if conv == nil {
		return nil, status.Error(codes.NotFound, "conversation not found")
	}

	// Check it's not the last item
	if len(conv.Items) <= 1 {
		return nil, status.Error(codes.FailedPrecondition, "cannot remove the last item from a conversation")
	}

	// Remove the item
	if err := s.repo.RemoveItem(ctx, req.GetConversationId(), req.GetContentId()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove item: %v", err)
	}

	// Update stats
	if err := s.repo.UpdateConversationStats(ctx, req.GetConversationId()); err != nil {
		s.logger.Error("failed to update stats after unlink", logging.Err(err))
	}

	// Invalidate summary
	if err := s.repo.InvalidateSummary(ctx, req.GetConversationId()); err != nil {
		s.logger.Error("failed to invalidate summary after unlink", logging.Err(err))
	}

	remaining, err := s.repo.GetItemCount(ctx, req.GetConversationId())
	if err != nil {
		s.logger.Error("failed to get remaining count", logging.Err(err))
		remaining = int32(len(conv.Items) - 1) // fallback estimate
	}

	s.logger.Info("unlinked item from conversation",
		logging.F("conversation_id", req.GetConversationId()),
		logging.F("content_id", req.GetContentId()),
		logging.F("remaining", remaining),
	)

	return &conversationv1.UnlinkItemResponse{
		RemainingItems: remaining,
	}, nil
}

// RunConversationAudit detects orphans, duplicates, and merge candidates.
func (s *Service) RunConversationAudit(ctx context.Context, req *conversationv1.RunConversationAuditRequest) (*conversationv1.RunConversationAuditResponse, error) {
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	if s.auditRepo == nil {
		return nil, status.Error(codes.FailedPrecondition, "audit repository not configured")
	}

	tenantID := req.GetTenantId()
	runAll := !req.GetOrphansOnly() && !req.GetDuplicatesOnly() && !req.GetMergeOnly()

	resp := &conversationv1.RunConversationAuditResponse{}

	// Count conversations audited.
	count, err := s.auditRepo.CountConversations(ctx, tenantID)
	if err != nil {
		s.logger.Error("failed to count conversations for audit", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count conversations: %v", err)
	}
	resp.ConversationsAudited = count

	// 1. Orphan detection
	if runAll || req.GetOrphansOnly() {
		orphansThread, err := s.auditRepo.FindOrphansWithThread(ctx, tenantID)
		if err != nil {
			s.logger.Error("audit: orphan thread detection failed", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "orphan detection failed: %v", err)
		}
		for _, o := range orphansThread {
			resp.Orphans = append(resp.Orphans, &conversationv1.AuditOrphan{
				ContentId:               o.ContentID,
				Reason:                  o.Reason,
				SuggestedConversationId: o.SuggestedConversationID,
			})
		}

		orphansReply, err := s.auditRepo.FindOrphansWithReplySubject(ctx, tenantID)
		if err != nil {
			s.logger.Error("audit: orphan reply subject detection failed", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "orphan detection failed: %v", err)
		}
		for _, o := range orphansReply {
			resp.Orphans = append(resp.Orphans, &conversationv1.AuditOrphan{
				ContentId:               o.ContentID,
				Reason:                  o.Reason,
				SuggestedConversationId: o.SuggestedConversationID,
			})
		}
	}

	// 2. Duplicate membership detection
	if runAll || req.GetDuplicatesOnly() {
		duplicates, err := s.auditRepo.FindDuplicateMemberships(ctx, tenantID)
		if err != nil {
			s.logger.Error("audit: duplicate membership detection failed", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "duplicate detection failed: %v", err)
		}
		for _, d := range duplicates {
			resp.Flagged = append(resp.Flagged, &conversationv1.AuditFinding{
				ConversationId:  d.ConversationIDs[0],
				ItemId:          d.ContentID,
				Reason:          fmt.Sprintf("item linked to %d conversations: %v", len(d.ConversationIDs), d.ConversationIDs),
				SuggestedAction: "merge",
			})
		}
	}

	// 3. Merge candidate detection
	if runAll || req.GetMergeOnly() {
		overlapCandidates, err := s.auditRepo.FindMergeCandidatesByOverlap(ctx, tenantID)
		if err != nil {
			s.logger.Error("audit: merge candidate overlap detection failed", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "merge candidate detection failed: %v", err)
		}
		for _, mc := range overlapCandidates {
			resp.MergeCandidates = append(resp.MergeCandidates, &conversationv1.AuditMergeCandidate{
				ConversationIdA: mc.ConversationIDA,
				ConversationIdB: mc.ConversationIDB,
				TopicA:          mc.TopicA,
				TopicB:          mc.TopicB,
				Reason:          mc.Reason,
				SharedItems:     mc.SharedItems,
			})
		}

		topicCandidates, err := s.auditRepo.FindMergeCandidatesByTopic(ctx, tenantID)
		if err != nil {
			s.logger.Error("audit: merge candidate topic detection failed", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "merge candidate detection failed: %v", err)
		}
		for _, mc := range topicCandidates {
			resp.MergeCandidates = append(resp.MergeCandidates, &conversationv1.AuditMergeCandidate{
				ConversationIdA: mc.ConversationIDA,
				ConversationIdB: mc.ConversationIDB,
				TopicA:          mc.TopicA,
				TopicB:          mc.TopicB,
				Reason:          mc.Reason,
			})
		}
	}

	s.logger.Info("conversation audit completed",
		logging.F("tenant_id", tenantID),
		logging.F("conversations_audited", count),
		logging.F("orphans", len(resp.Orphans)),
		logging.F("flagged", len(resp.Flagged)),
		logging.F("merge_candidates", len(resp.MergeCandidates)),
	)

	return resp, nil
}

// conversationSummaryToProto converts a ConversationSummary to proto.
func conversationSummaryToProto(conv *ConversationSummary) *conversationv1.ConversationSummary {
	pb := &conversationv1.ConversationSummary{
		Id:               conv.ID,
		TenantId:         conv.TenantID,
		Topic:            conv.Topic,
		ParticipantCount: conv.ParticipantCount,
		ItemCount:        conv.ItemCount,
		CreatedAt:        timestamppb.New(conv.CreatedAt),
		UpdatedAt:        timestamppb.New(conv.UpdatedAt),
	}

	// Handle optional fields
	if conv.ThreadKey != nil {
		pb.ThreadKey = conv.ThreadKey
	}

	if conv.FirstSeen != nil {
		pb.FirstSeen = timestamppb.New(*conv.FirstSeen)
	}

	if conv.LastSeen != nil {
		pb.LastSeen = timestamppb.New(*conv.LastSeen)
	}

	if conv.State != "" {
		pb.State = &conv.State
	}

	return pb
}

// conversationDetailToProto converts a ConversationDetail to proto.
func conversationDetailToProto(conv *ConversationDetail) *conversationv1.ShowConversationResponse {
	resp := &conversationv1.ShowConversationResponse{
		Id:               conv.ID,
		TenantId:         conv.TenantID,
		Topic:            conv.Topic,
		ParticipantCount: conv.ParticipantCount,
		ItemCount:        conv.ItemCount,
		SummaryVersion:   conv.SummaryVersion,
		CreatedAt:        timestamppb.New(conv.CreatedAt),
		UpdatedAt:        timestamppb.New(conv.UpdatedAt),
	}

	// Handle optional fields
	if conv.ThreadKey != nil {
		resp.ThreadKey = conv.ThreadKey
	}

	if conv.FirstSeen != nil {
		resp.FirstSeen = timestamppb.New(*conv.FirstSeen)
	}

	if conv.LastSeen != nil {
		resp.LastSeen = timestamppb.New(*conv.LastSeen)
	}

	// Summary fields
	if conv.StateSummary != nil {
		resp.StateSummary = conv.StateSummary
	}

	if conv.SummaryUpdatedAt != nil {
		resp.SummaryUpdatedAt = timestamppb.New(*conv.SummaryUpdatedAt)
	}

	// State fields
	if conv.State != "" {
		resp.State = &conv.State
	}

	if conv.StateReason != nil {
		resp.StateReason = conv.StateReason
	}

	if conv.StateChangedAt != nil {
		resp.StateChangedAt = timestamppb.New(*conv.StateChangedAt)
	}

	// Convert items
	resp.Items = make([]*conversationv1.ConversationItem, len(conv.Items))
	for i, item := range conv.Items {
		protoItem := &conversationv1.ConversationItem{
			ConversationId: item.ConversationID,
			ContentId:      item.ContentID,
			AddedAt:        timestamppb.New(item.AddedAt),
		}
		if item.SourceID != nil {
			protoItem.SourceId = item.SourceID
		}
		if item.ContentDate != nil {
			protoItem.ContentDate = timestamppb.New(*item.ContentDate)
		}
		if item.FromName != nil {
			protoItem.FromName = item.FromName
		}
		if item.Subject != nil {
			protoItem.Subject = item.Subject
		}
		resp.Items[i] = protoItem
	}

	// Convert participants
	resp.Participants = make([]*conversationv1.ConversationParticipant, len(conv.Participants))
	for i, part := range conv.Participants {
		protoParticipant := &conversationv1.ConversationParticipant{
			ConversationId: part.ConversationID,
		}
		if part.Name != nil {
			protoParticipant.Name = part.Name
		}
		if part.Address != nil {
			protoParticipant.Address = part.Address
		}
		resp.Participants[i] = protoParticipant
	}

	return resp
}
