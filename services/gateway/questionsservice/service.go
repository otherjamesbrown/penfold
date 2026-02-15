// Package questionsservice implements the QuestionsService gRPC server.
package questionsservice

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	questionsv1 "github.com/otherjamesbrown/penfold/api/proto/questions/v1"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	"github.com/otherjamesbrown/penfold/pkg/sources"
)

// Service implements the QuestionsService gRPC server.
type Service struct {
	questionsv1.UnimplementedQuestionsServiceServer
	repo         *reviewqueue.Repository
	glossaryRepo *glossary.Repository
	sourcesRepo  *sources.Repository
	logger       logging.Logger
}

// NewService creates a new questions service.
func NewService(repo *reviewqueue.Repository, glossaryRepo *glossary.Repository, sourcesRepo *sources.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:         repo,
		glossaryRepo: glossaryRepo,
		sourcesRepo:  sourcesRepo,
		logger:       logger,
	}
}

// ListQuestions returns pending questions filtered by type and priority.
func (s *Service) ListQuestions(ctx context.Context, req *questionsv1.ListQuestionsRequest) (*questionsv1.ListQuestionsResponse, error) {
	s.logger.Debug("ListQuestions called",
		logging.F("status", req.Status),
		logging.F("question_type", req.QuestionType),
		logging.F("limit", req.Limit),
		logging.F("tenant_id", req.GetTenantId()),
	)

	filter := reviewqueue.ReviewFilter{
		TenantID: req.GetTenantId(),
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	// Map proto status to internal status
	if req.Status != questionsv1.QuestionStatus_QUESTION_STATUS_UNSPECIFIED {
		filter.Status = protoStatusToInternal(req.Status)
	}

	// Map proto question type to internal type
	if req.QuestionType != questionsv1.QuestionType_QUESTION_TYPE_UNSPECIFIED {
		filter.QuestionType = protoTypeToInternal(req.QuestionType)
	}

	// Map proto priority to internal priority
	if req.Priority != questionsv1.QuestionPriority_QUESTION_PRIORITY_UNSPECIFIED {
		filter.Priority = protoPriorityToInternal(req.Priority)
	}

	if req.SourceType != "" {
		filter.SourceType = req.SourceType
	}

	if filter.Limit == 0 {
		filter.Limit = 50
	}

	items, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing questions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list questions: %v", err)
	}

	protoQuestions := make([]*questionsv1.Question, len(items))
	for i, item := range items {
		protoQuestions[i] = reviewItemToProto(item)
	}

	return &questionsv1.ListQuestionsResponse{
		Questions:  protoQuestions,
		TotalCount: int64(len(items)),
	}, nil
}

// GetQuestion retrieves a single question by ID.
func (s *Service) GetQuestion(ctx context.Context, req *questionsv1.GetQuestionRequest) (*questionsv1.GetQuestionResponse, error) {
	s.logger.Debug("GetQuestion called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	item, err := s.repo.Get(ctx, req.Id)
	if err != nil {
		s.logger.Error("Error getting question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get question: %v", err)
	}
	if item == nil {
		return nil, status.Errorf(codes.NotFound, "question with id %d not found", req.Id)
	}

	return &questionsv1.GetQuestionResponse{
		Question: reviewItemToProto(item),
	}, nil
}

// GetNextQuestion retrieves the next highest-priority question.
func (s *Service) GetNextQuestion(ctx context.Context, req *questionsv1.GetNextQuestionRequest) (*questionsv1.GetNextQuestionResponse, error) {
	s.logger.Debug("GetNextQuestion called",
		logging.F("question_type", req.QuestionType),
		logging.F("tenant_id", req.GetTenantId()),
	)

	var questionType reviewqueue.QuestionType
	if req.QuestionType != questionsv1.QuestionType_QUESTION_TYPE_UNSPECIFIED {
		questionType = protoTypeToInternal(req.QuestionType)
	}

	item, err := s.repo.GetNext(ctx, questionType, req.GetTenantId())
	if err != nil {
		s.logger.Error("Error getting next question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get next question: %v", err)
	}

	return &questionsv1.GetNextQuestionResponse{
		Question: reviewItemToProto(item),
	}, nil
}

// ResolveQuestion marks a question as resolved with an answer.
func (s *Service) ResolveQuestion(ctx context.Context, req *questionsv1.ResolveQuestionRequest) (*questionsv1.ResolveQuestionResponse, error) {
	s.logger.Debug("ResolveQuestion called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.Answer == "" {
		return nil, status.Error(codes.InvalidArgument, "answer is required")
	}

	// Get the question first to check type
	item, err := s.repo.Get(ctx, req.Id)
	if err != nil {
		s.logger.Error("Error getting question for resolve", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get question: %v", err)
	}
	if item == nil {
		return nil, status.Errorf(codes.NotFound, "question with id %d not found", req.Id)
	}

	// Resolve the question
	err = s.repo.Resolve(ctx, req.Id, req.Answer, "grpc-user")
	if err != nil {
		s.logger.Error("Error resolving question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to resolve question: %v", err)
	}

	// If this is an acronym question, add to glossary
	addedToGlossary := false
	if item.QuestionType == reviewqueue.QuestionTypeAcronym && item.SuggestedTerm != nil && *item.SuggestedTerm != "" {
		expandInSearch := true
		input := glossary.TermInput{
			Term:           *item.SuggestedTerm,
			Expansion:      req.Answer,
			ExpandInSearch: &expandInSearch,
			Source:         "review_queue",
		}

		_, err := s.glossaryRepo.Create(ctx, input)
		if err != nil {
			s.logger.Warn("Failed to add term to glossary (may already exist)",
				logging.F("term", *item.SuggestedTerm),
				logging.Err(err),
			)
		} else {
			addedToGlossary = true
			s.logger.Info("Added term to glossary",
				logging.F("term", *item.SuggestedTerm),
				logging.F("expansion", req.Answer),
			)
		}
	}

	// Get the updated question
	updatedItem, _ := s.repo.Get(ctx, req.Id)

	return &questionsv1.ResolveQuestionResponse{
		Resolved:        true,
		AddedToGlossary: addedToGlossary,
		Question:        reviewItemToProto(updatedItem),
	}, nil
}

// DismissQuestion marks a question as dismissed (not needed).
func (s *Service) DismissQuestion(ctx context.Context, req *questionsv1.DismissQuestionRequest) (*questionsv1.DismissQuestionResponse, error) {
	s.logger.Debug("DismissQuestion called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	err := s.repo.Dismiss(ctx, req.Id, req.Reason, "grpc-user")
	if err != nil {
		if err.Error() == "item not found or already processed" {
			return nil, status.Errorf(codes.NotFound, "question with id %d not found or already processed", req.Id)
		}
		s.logger.Error("Error dismissing question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to dismiss question: %v", err)
	}

	return &questionsv1.DismissQuestionResponse{
		Dismissed: true,
	}, nil
}

// DeferQuestion marks a question as deferred for later.
func (s *Service) DeferQuestion(ctx context.Context, req *questionsv1.DeferQuestionRequest) (*questionsv1.DeferQuestionResponse, error) {
	s.logger.Debug("DeferQuestion called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	err := s.repo.Defer(ctx, req.Id)
	if err != nil {
		if err.Error() == "item not found or already processed" {
			return nil, status.Errorf(codes.NotFound, "question with id %d not found or already processed", req.Id)
		}
		s.logger.Error("Error deferring question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to defer question: %v", err)
	}

	return &questionsv1.DeferQuestionResponse{
		Deferred: true,
	}, nil
}

// GetQueueStats retrieves statistics about the questions queue.
func (s *Service) GetQueueStats(ctx context.Context, req *questionsv1.GetQueueStatsRequest) (*questionsv1.GetQueueStatsResponse, error) {
	s.logger.Debug("GetQueueStats called",
		logging.F("tenant_id", req.GetTenantId()),
	)

	stats, err := s.repo.GetStats(ctx, req.GetTenantId())
	if err != nil {
		s.logger.Error("Error getting queue stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get queue stats: %v", err)
	}

	// Convert map[string]int to map[string]int64
	byPriority := make(map[string]int64)
	for k, v := range stats.ByPriority {
		byPriority[k] = int64(v)
	}

	byType := make(map[string]int64)
	for k, v := range stats.ByType {
		byType[k] = int64(v)
	}

	protoStats := &questionsv1.QueueStats{
		TotalPending:  int64(stats.TotalPending),
		ByPriority:    byPriority,
		ByType:        byType,
		ResolvedToday: int64(stats.ResolvedToday),
	}

	if stats.OldestPending != nil {
		protoStats.OldestPending = timestamppb.New(*stats.OldestPending)
	}

	return &questionsv1.GetQueueStatsResponse{
		Stats: protoStats,
	}, nil
}

// Conversion functions

func protoStatusToInternal(s questionsv1.QuestionStatus) reviewqueue.Status {
	switch s {
	case questionsv1.QuestionStatus_QUESTION_STATUS_PENDING:
		return reviewqueue.StatusPending
	case questionsv1.QuestionStatus_QUESTION_STATUS_RESOLVED:
		return reviewqueue.StatusResolved
	case questionsv1.QuestionStatus_QUESTION_STATUS_DISMISSED:
		return reviewqueue.StatusDismissed
	case questionsv1.QuestionStatus_QUESTION_STATUS_DEFERRED:
		return reviewqueue.StatusDeferred
	default:
		return reviewqueue.StatusPending
	}
}

func internalStatusToProto(s reviewqueue.Status) questionsv1.QuestionStatus {
	switch s {
	case reviewqueue.StatusPending:
		return questionsv1.QuestionStatus_QUESTION_STATUS_PENDING
	case reviewqueue.StatusResolved:
		return questionsv1.QuestionStatus_QUESTION_STATUS_RESOLVED
	case reviewqueue.StatusDismissed:
		return questionsv1.QuestionStatus_QUESTION_STATUS_DISMISSED
	case reviewqueue.StatusDeferred:
		return questionsv1.QuestionStatus_QUESTION_STATUS_DEFERRED
	default:
		return questionsv1.QuestionStatus_QUESTION_STATUS_UNSPECIFIED
	}
}

func protoTypeToInternal(t questionsv1.QuestionType) reviewqueue.QuestionType {
	switch t {
	case questionsv1.QuestionType_QUESTION_TYPE_ACRONYM:
		return reviewqueue.QuestionTypeAcronym
	case questionsv1.QuestionType_QUESTION_TYPE_PERSON:
		return reviewqueue.QuestionTypePerson
	case questionsv1.QuestionType_QUESTION_TYPE_ENTITY:
		return reviewqueue.QuestionTypeEntity
	case questionsv1.QuestionType_QUESTION_TYPE_DUPLICATE:
		return reviewqueue.QuestionTypeDuplicate
	case questionsv1.QuestionType_QUESTION_TYPE_OTHER:
		return reviewqueue.QuestionTypeOther
	default:
		return ""
	}
}

func internalTypeToProto(t reviewqueue.QuestionType) questionsv1.QuestionType {
	switch t {
	case reviewqueue.QuestionTypeAcronym:
		return questionsv1.QuestionType_QUESTION_TYPE_ACRONYM
	case reviewqueue.QuestionTypePerson:
		return questionsv1.QuestionType_QUESTION_TYPE_PERSON
	case reviewqueue.QuestionTypeEntity:
		return questionsv1.QuestionType_QUESTION_TYPE_ENTITY
	case reviewqueue.QuestionTypeDuplicate:
		return questionsv1.QuestionType_QUESTION_TYPE_DUPLICATE
	case reviewqueue.QuestionTypeOther:
		return questionsv1.QuestionType_QUESTION_TYPE_OTHER
	default:
		return questionsv1.QuestionType_QUESTION_TYPE_UNSPECIFIED
	}
}

func protoPriorityToInternal(p questionsv1.QuestionPriority) reviewqueue.Priority {
	switch p {
	case questionsv1.QuestionPriority_QUESTION_PRIORITY_LOW:
		return reviewqueue.PriorityLow
	case questionsv1.QuestionPriority_QUESTION_PRIORITY_MEDIUM:
		return reviewqueue.PriorityMedium
	case questionsv1.QuestionPriority_QUESTION_PRIORITY_HIGH:
		return reviewqueue.PriorityHigh
	default:
		return ""
	}
}

func internalPriorityToProto(p reviewqueue.Priority) questionsv1.QuestionPriority {
	switch p {
	case reviewqueue.PriorityLow:
		return questionsv1.QuestionPriority_QUESTION_PRIORITY_LOW
	case reviewqueue.PriorityMedium:
		return questionsv1.QuestionPriority_QUESTION_PRIORITY_MEDIUM
	case reviewqueue.PriorityHigh:
		return questionsv1.QuestionPriority_QUESTION_PRIORITY_HIGH
	default:
		return questionsv1.QuestionPriority_QUESTION_PRIORITY_UNSPECIFIED
	}
}

// GetQuestionSource retrieves the source content for a question.
func (s *Service) GetQuestionSource(ctx context.Context, req *questionsv1.GetQuestionSourceRequest) (*questionsv1.GetQuestionSourceResponse, error) {
	s.logger.Debug("GetQuestionSource called",
		logging.F("question_id", req.QuestionId),
		logging.F("context_chars", req.ContextChars),
	)

	if req.QuestionId == 0 {
		return nil, status.Error(codes.InvalidArgument, "question_id is required")
	}

	// Get the question first
	item, err := s.repo.Get(ctx, req.QuestionId)
	if err != nil {
		s.logger.Error("Error getting question", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get question: %v", err)
	}
	if item == nil {
		return nil, status.Errorf(codes.NotFound, "question with id %d not found", req.QuestionId)
	}

	// Check if we have a source ID
	if item.SourceID == nil || *item.SourceID == 0 {
		return nil, status.Error(codes.NotFound, "question has no source reference")
	}

	// Check if sources repo is available
	if s.sourcesRepo == nil {
		return nil, status.Error(codes.Unavailable, "source content retrieval not configured")
	}

	// Determine how to look up the source based on source_type
	// When source_type is 'meeting', source_id refers to meetings.id, not sources.id
	var source *sources.Source
	var meeting *sources.Meeting
	sourceType := ""
	if item.SourceType != nil {
		sourceType = *item.SourceType
	}

	if sourceType == "meeting" {
		// source_id is a meeting ID
		var err error
		meeting, err = s.sourcesRepo.GetMeetingByID(ctx, *item.SourceID)
		if err != nil {
			s.logger.Error("Error getting meeting", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to get meeting: %v", err)
		}
		if meeting == nil {
			return nil, status.Errorf(codes.NotFound, "meeting with id %d not found", *item.SourceID)
		}

		// Get the source content via meeting ID
		source, err = s.sourcesRepo.GetSourceByMeetingID(ctx, meeting.ID)
		if err != nil {
			s.logger.Error("Error getting source by meeting ID", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to get source: %v", err)
		}
		if source == nil {
			return nil, status.Errorf(codes.NotFound, "source for meeting %d not found", meeting.ID)
		}
	} else {
		// source_id is a sources.id
		var err error
		source, err = s.sourcesRepo.GetByID(ctx, *item.SourceID)
		if err != nil {
			s.logger.Error("Error getting source", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to get source: %v", err)
		}
		if source == nil {
			return nil, status.Errorf(codes.NotFound, "source with id %d not found", *item.SourceID)
		}
	}

	// Build the response
	sourceContent := &questionsv1.SourceContent{
		SourceId:    source.ID,
		SourceType:  source.SourceSystem,
		TotalLength: int32(len(source.RawContent)),
	}

	// Get title from meeting if available
	if meeting != nil {
		sourceContent.Title = meeting.Title
		if meeting.MeetingDate != nil {
			sourceContent.SourceTimestamp = timestamppb.New(*meeting.MeetingDate)
		}
		// Add meeting metadata
		sourceContent.Metadata = map[string]string{
			"platform":   meeting.Platform,
			"source_tag": meeting.SourceTag,
		}
	} else if source.MeetingID != nil {
		// Try to get meeting from source.MeetingID
		m, err := s.sourcesRepo.GetMeetingByID(ctx, *source.MeetingID)
		if err == nil && m != nil {
			sourceContent.Title = m.Title
			if m.MeetingDate != nil {
				sourceContent.SourceTimestamp = timestamppb.New(*m.MeetingDate)
			}
			sourceContent.Metadata = map[string]string{
				"platform":   m.Platform,
				"source_tag": m.SourceTag,
			}
		}
	} else if item.SourceReference != nil {
		sourceContent.Title = *item.SourceReference
	}

	// Handle content based on context_chars
	contextChars := int(req.ContextChars)
	if contextChars == 0 {
		contextChars = 500 // Default context
	}

	// Find the snippet in the content
	content := source.RawContent
	snippetContext := ""
	if item.Context != nil {
		snippetContext = *item.Context
	}

	// Try to find the context snippet in the content
	if snippetContext != "" {
		idx := strings.Index(content, snippetContext)
		if idx >= 0 {
			sourceContent.SnippetOffset = int32(idx)
			// Get surrounding context
			start := idx - contextChars
			if start < 0 {
				start = 0
			}
			end := idx + len(snippetContext) + contextChars
			if end > len(content) {
				end = len(content)
			}
			sourceContent.Content = content[start:end]
			sourceContent.Snippet = snippetContext
		} else {
			// Snippet not found, return beginning of content
			if len(content) > contextChars*2 {
				sourceContent.Content = content[:contextChars*2]
			} else {
				sourceContent.Content = content
			}
		}
	} else {
		// No snippet, return beginning of content
		if len(content) > contextChars*2 {
			sourceContent.Content = content[:contextChars*2]
		} else {
			sourceContent.Content = content
		}
	}

	// If context_chars is -1, return full content
	if req.ContextChars < 0 {
		sourceContent.Content = content
	}

	return &questionsv1.GetQuestionSourceResponse{
		Source:   sourceContent,
		Question: reviewItemToProto(item),
	}, nil
}

func reviewItemToProto(item *reviewqueue.ReviewItem) *questionsv1.Question {
	if item == nil {
		return nil
	}

	q := &questionsv1.Question{
		Id:           item.ID,
		QuestionType: internalTypeToProto(item.QuestionType),
		Priority:     internalPriorityToProto(item.Priority),
		Question:     item.Question,
		Status:       internalStatusToProto(item.Status),
		Confidence:   item.Confidence,
		CreatedAt:    timestamppb.New(item.CreatedAt),
	}

	// Handle optional fields
	if item.Context != nil {
		q.Context = *item.Context
	}
	if item.SourceType != nil {
		q.SourceType = *item.SourceType
	}
	if item.SourceID != nil {
		q.SourceId = *item.SourceID
	}
	if item.SourceReference != nil {
		q.SourceReference = *item.SourceReference
	}
	if item.SuggestedTerm != nil {
		q.SuggestedTerm = *item.SuggestedTerm
	}
	if item.SuggestedExpansion != nil {
		q.SuggestedExpansion = *item.SuggestedExpansion
	}
	if item.MatchedText != nil {
		q.MatchedText = *item.MatchedText
	}
	if item.Resolution != nil {
		q.Resolution = *item.Resolution
	}
	if item.ResolvedAt != nil {
		q.ResolvedAt = timestamppb.New(*item.ResolvedAt)
	}
	if item.ResolvedBy != nil {
		q.ResolvedBy = *item.ResolvedBy
	}
	if len(item.CandidatePersonIDs) > 0 {
		q.CandidatePersonIds = item.CandidatePersonIDs
	}

	return q
}
