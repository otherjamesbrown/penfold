// Package auditservice implements the AuditService gRPC server.
package auditservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditv1 "github.com/otherjamesbrown/penfold/api/proto/audit/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions/audit"
)

// Service implements the AuditService gRPC server.
type Service struct {
	auditv1.UnimplementedAuditServiceServer
	repo   *audit.PostgresRepository
	logger logging.Logger
}

// NewService creates a new audit service.
func NewService(repo *audit.PostgresRepository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListTraces lists resolution traces with optional filtering.
func (s *Service) ListTraces(ctx context.Context, req *auditv1.ListTracesRequest) (*auditv1.ListTracesResponse, error) {
	s.logger.Debug("ListTraces called",
		logging.F("tenant_id", req.TenantId),
		logging.F("content_id", req.ContentId),
		logging.F("content_type", req.ContentType),
	)

	filter := audit.TraceFilter{
		TenantID:       req.TenantId,
		ContentType:    req.ContentType,
		HadCorrections: req.HadCorrections,
		Limit:          int(req.Limit),
		Offset:         int(req.Offset),
	}

	if req.ContentId > 0 {
		filter.ContentID = &req.ContentId
	}

	if req.Since != nil {
		t := req.Since.AsTime()
		filter.Since = &t
	}

	traces, err := s.repo.ListTraces(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing traces", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list traces: %v", err)
	}

	protoTraces := make([]*auditv1.TraceSummary, len(traces))
	for i, t := range traces {
		protoTraces[i] = traceSummaryToProto(t)
	}

	return &auditv1.ListTracesResponse{
		Traces:     protoTraces,
		TotalCount: int64(len(traces)),
	}, nil
}

// GetTrace retrieves detailed information about a specific trace.
func (s *Service) GetTrace(ctx context.Context, req *auditv1.GetTraceRequest) (*auditv1.GetTraceResponse, error) {
	s.logger.Debug("GetTrace called",
		logging.F("trace_id", req.TraceId),
		logging.F("include_llm_calls", req.IncludeLlmCalls),
	)

	if req.TraceId == "" {
		return nil, status.Error(codes.InvalidArgument, "trace_id is required")
	}

	detail, err := s.repo.GetTraceDetail(ctx, req.TraceId, req.IncludeLlmCalls)
	if err != nil {
		s.logger.Error("Error getting trace detail", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "trace not found: %s", req.TraceId)
	}

	return &auditv1.GetTraceResponse{
		Trace: traceDetailToProto(detail),
	}, nil
}

// ListCorrections lists decisions that were corrected by humans.
func (s *Service) ListCorrections(ctx context.Context, req *auditv1.ListCorrectionsRequest) (*auditv1.ListCorrectionsResponse, error) {
	s.logger.Debug("ListCorrections called",
		logging.F("tenant_id", req.TenantId),
	)

	filter := audit.TraceFilter{
		TenantID: req.TenantId,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if req.Since != nil {
		t := req.Since.AsTime()
		filter.Since = &t
	}

	corrections, err := s.repo.GetCorrections(ctx, filter)
	if err != nil {
		s.logger.Error("Error getting corrections", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get corrections: %v", err)
	}

	protoCorrections := make([]*auditv1.Decision, len(corrections))
	for i, c := range corrections {
		protoCorrections[i] = decisionToProto(c)
	}

	return &auditv1.ListCorrectionsResponse{
		Corrections: protoCorrections,
		TotalCount:  int64(len(corrections)),
	}, nil
}

// ListComparisons lists model comparison runs.
func (s *Service) ListComparisons(ctx context.Context, req *auditv1.ListComparisonsRequest) (*auditv1.ListComparisonsResponse, error) {
	s.logger.Debug("ListComparisons called",
		logging.F("tenant_id", req.TenantId),
	)

	filter := audit.ComparisonFilter{
		TenantID: req.TenantId,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if req.Since != nil {
		t := req.Since.AsTime()
		filter.Since = &t
	}

	comparisons, err := s.repo.ListComparisons(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing comparisons", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list comparisons: %v", err)
	}

	protoComparisons := make([]*auditv1.ComparisonSummary, len(comparisons))
	for i, c := range comparisons {
		protoComparisons[i] = comparisonSummaryToProto(c)
	}

	return &auditv1.ListComparisonsResponse{
		Comparisons: protoComparisons,
		TotalCount:  int64(len(comparisons)),
	}, nil
}

// GetComparison retrieves detailed information about a model comparison.
func (s *Service) GetComparison(ctx context.Context, req *auditv1.GetComparisonRequest) (*auditv1.GetComparisonResponse, error) {
	s.logger.Debug("GetComparison called",
		logging.F("comparison_id", req.ComparisonId),
	)

	if req.ComparisonId == "" {
		return nil, status.Error(codes.InvalidArgument, "comparison_id is required")
	}

	detail, err := s.repo.GetComparisonDetail(ctx, req.ComparisonId)
	if err != nil {
		s.logger.Error("Error getting comparison detail", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "comparison not found: %s", req.ComparisonId)
	}

	return &auditv1.GetComparisonResponse{
		Comparison: comparisonDetailToProto(detail),
	}, nil
}

// GetModelStats retrieves aggregate statistics for each model.
func (s *Service) GetModelStats(ctx context.Context, req *auditv1.GetModelStatsRequest) (*auditv1.GetModelStatsResponse, error) {
	s.logger.Debug("GetModelStats called",
		logging.F("tenant_id", req.TenantId),
		logging.F("days_since", req.DaysSince),
	)

	daysSince := int(req.DaysSince)
	if daysSince <= 0 {
		daysSince = 30
	}

	stats, err := s.repo.GetModelStats(ctx, req.TenantId, daysSince)
	if err != nil {
		s.logger.Error("Error getting model stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get model stats: %v", err)
	}

	protoStats := make([]*auditv1.ModelStats, len(stats))
	for i, s := range stats {
		protoStats[i] = modelStatsToProto(s)
	}

	return &auditv1.GetModelStatsResponse{
		Stats: protoStats,
	}, nil
}

// Helper functions for converting between domain and proto types.

func traceSummaryToProto(t audit.TraceSummary) *auditv1.TraceSummary {
	return &auditv1.TraceSummary{
		Id:              t.ID,
		ContentId:       t.ContentID,
		ContentType:     t.ContentType,
		ContentSummary:  t.ContentSummary,
		Status:          t.Status,
		MentionsFound:   int32(t.MentionsFound),
		AutoResolved:    int32(t.AutoResolved),
		QueuedForReview: int32(t.QueuedForReview),
		ModelUsed:       t.ModelUsed,
		DurationMs:      int32(t.DurationMs),
		StartedAt:       timestamppb.New(t.StartedAt),
	}
}

func stageToProto(s audit.Stage) *auditv1.Stage {
	stage := &auditv1.Stage{
		Id:            s.ID,
		TraceId:       s.TraceID,
		StageNumber:   int32(s.StageNumber),
		StageName:     string(s.StageName),
		StartedAt:     timestamppb.New(s.StartedAt),
		DurationMs:    int32(s.DurationMs),
		InputSummary:  s.InputSummary,
		OutputSummary: s.OutputSummary,
		Status:        string(s.Status),
		Skipped:       s.Skipped,
		SkipReason:    s.SkipReason,
		ErrorMessage:  s.ErrorMessage,
	}

	if s.CompletedAt != nil {
		stage.CompletedAt = timestamppb.New(*s.CompletedAt)
	}

	return stage
}

func decisionToProto(d audit.Decision) *auditv1.Decision {
	decision := &auditv1.Decision{
		Id:              d.ID,
		TraceId:         d.TraceID,
		DecisionType:    string(d.DecisionType),
		MentionedText:   d.MentionedText,
		ChosenOption:    d.ChosenOption,
		Confidence:      d.Confidence,
		Reasoning:       d.Reasoning,
		CorrectionNotes: d.CorrectionNotes,
		CreatedAt:       timestamppb.New(d.CreatedAt),
	}

	if d.StageID != nil {
		decision.StageId = *d.StageID
	}

	if d.MentionID != nil {
		decision.MentionId = *d.MentionID
	}

	if d.WasCorrect != nil {
		decision.WasCorrect = *d.WasCorrect
	}

	return decision
}

func llmCallToProto(c audit.LLMCall) *auditv1.LLMCall {
	call := &auditv1.LLMCall{
		Id:             c.ID,
		TraceId:        c.TraceID,
		Model:          c.Model,
		PromptTemplate: c.PromptTemplate,
		PromptText:     c.PromptText,
		PromptTokens:   int32(c.PromptTokens),
		ResponseText:   c.ResponseText,
		ResponseTokens: int32(c.ResponseTokens),
		LatencyMs:      int32(c.LatencyMs),
		AttemptNumber:  int32(c.AttemptNumber),
		IsFallback:     c.IsFallback,
		FallbackReason: c.FallbackReason,
	}

	if c.StageID != nil {
		call.StageId = *c.StageID
	}

	return call
}

func traceDetailToProto(d *audit.TraceDetail) *auditv1.TraceDetail {
	detail := &auditv1.TraceDetail{
		Trace:                traceSummaryToProto(traceSummaryFromTrace(d.Trace)),
		NewEntitiesSuggested: int32(d.Trace.NewEntitiesSuggested),
	}

	detail.Stages = make([]*auditv1.Stage, len(d.Stages))
	for i, s := range d.Stages {
		detail.Stages[i] = stageToProto(s)
	}

	detail.Decisions = make([]*auditv1.Decision, len(d.Decisions))
	for i, dec := range d.Decisions {
		detail.Decisions[i] = decisionToProto(dec)
	}

	detail.LlmCalls = make([]*auditv1.LLMCall, len(d.LLMCalls))
	for i, c := range d.LLMCalls {
		detail.LlmCalls[i] = llmCallToProto(c)
	}

	return detail
}

func traceSummaryFromTrace(t audit.Trace) audit.TraceSummary {
	return audit.TraceSummary{
		ID:              t.ID,
		ContentID:       t.ContentID,
		ContentType:     t.ContentType,
		ContentSummary:  t.ContentSummary,
		Status:          string(t.Status),
		MentionsFound:   t.MentionsFound,
		AutoResolved:    t.AutoResolved,
		QueuedForReview: t.QueuedForReview,
		ModelUsed:       t.ModelUsed,
		DurationMs:      t.DurationMs,
		StartedAt:       t.StartedAt,
	}
}

func comparisonSummaryToProto(c audit.ComparisonSummary) *auditv1.ComparisonSummary {
	return &auditv1.ComparisonSummary{
		Id:                 c.ID,
		ContentId:          c.ContentID,
		ContentType:        c.ContentType,
		ContentSummary:     c.ContentSummary,
		Models:             c.Models,
		TotalDecisions:     int32(c.TotalDecisions),
		UnanimousDecisions: int32(c.UnanimousDecisions),
		DivergentDecisions: int32(c.DivergentDecisions),
		StartedAt:          timestamppb.New(c.StartedAt),
	}
}

func modelDecisionToProto(d audit.ModelDecision) *auditv1.ModelDecision {
	decision := &auditv1.ModelDecision{
		Model:          d.Model,
		EntityName:     d.EntityName,
		Confidence:     d.Confidence,
		Reasoning:      d.Reasoning,
		SuggestNew:     d.SuggestNew,
		NewEntityType:  d.NewEntityType,
	}

	if d.EntityID != nil {
		decision.EntityId = *d.EntityID
	}

	return decision
}

func comparisonDecisionToProto(d audit.ComparisonDecision) *auditv1.ComparisonDecision {
	decision := &auditv1.ComparisonDecision{
		Id:               d.ID,
		ComparisonId:     d.ComparisonID,
		MentionedText:    d.MentionedText,
		IsUnanimous:      d.IsUnanimous,
		DivergenceType:   d.DivergenceType,
		ConfidenceSpread: d.ConfidenceSpread,
		ModelsCorrect:    d.ModelsCorrect,
	}

	if d.MentionIndex != nil {
		decision.MentionIndex = int32(*d.MentionIndex)
	}

	if d.GroundTruthEntityID != nil {
		decision.GroundTruthEntityId = *d.GroundTruthEntityID
	}

	decision.ModelDecisions = make([]*auditv1.ModelDecision, len(d.ModelDecisions))
	for i, md := range d.ModelDecisions {
		decision.ModelDecisions[i] = modelDecisionToProto(md)
	}

	return decision
}

func comparisonDetailToProto(d *audit.ComparisonDetail) *auditv1.ComparisonDetail {
	detail := &auditv1.ComparisonDetail{
		Comparison:  comparisonSummaryFromComparison(d.Comparison),
		Purpose:     d.Comparison.Purpose,
		InitiatedBy: d.Comparison.InitiatedBy,
		TraceIds:    d.Comparison.TraceIDs,
	}

	detail.Decisions = make([]*auditv1.ComparisonDecision, len(d.Decisions))
	for i, dec := range d.Decisions {
		detail.Decisions[i] = comparisonDecisionToProto(dec)
	}

	return detail
}

func comparisonSummaryFromComparison(c audit.Comparison) *auditv1.ComparisonSummary {
	return &auditv1.ComparisonSummary{
		Id:                 c.ID,
		ContentId:          c.ContentID,
		ContentType:        c.ContentType,
		ContentSummary:     c.ContentSummary,
		Models:             c.Models,
		TotalDecisions:     int32(c.TotalDecisions),
		UnanimousDecisions: int32(c.UnanimousDecisions),
		DivergentDecisions: int32(c.DivergentDecisions),
		StartedAt:          timestamppb.New(c.StartedAt),
	}
}

func modelStatsToProto(s audit.ModelStats) *auditv1.ModelStats {
	return &auditv1.ModelStats{
		Model:              s.Model,
		TotalComparisons:   int32(s.TotalComparisons),
		TotalDecisions:     int32(s.TotalDecisions),
		CorrectDecisions:   int32(s.CorrectDecisions),
		Accuracy:           s.Accuracy,
		AverageConfidence:  s.AverageConfidence,
		AverageLatencyMs:   int32(s.AverageLatencyMs),
		UnanimousAgreement: int32(s.UnanimousAgreement),
	}
}
