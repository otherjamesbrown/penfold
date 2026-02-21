// Package pipeline provides types and repository for pipeline status and job tracking.
package pipeline

import (
	"encoding/json"
	"time"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProcessingStatusResult contains the results of building a processing status
// from a set of pipeline runs. Used to share logic between contentservice and
// conversationservice.
type ProcessingStatusResult struct {
	// Stages is the list of per-stage results (latest run per stage).
	Stages []*contentv1.StageResult
	// ContentContribution is the triage-determined contribution level (e.g. "LOW", "MEDIUM", "HIGH").
	ContentContribution string
	// ContributionReason is the triage explanation for the contribution level.
	ContributionReason string
	// TriageCategory is the triage-determined category.
	TriageCategory string
	// TriageImportance is the triage-determined importance.
	TriageImportance string
	// StartedAt is the earliest pipeline run timestamp.
	StartedAt *time.Time
	// FinishedAt is the latest pipeline run finish timestamp.
	FinishedAt *time.Time
	// TotalDurationMS is the sum of all run durations in milliseconds.
	TotalDurationMS int64
	// LangfuseTraceID is the Langfuse trace ID (from the most recent non-nil run).
	LangfuseTraceID *string
	// TotalInputTokens is the sum of input tokens across all runs.
	TotalInputTokens int32
	// TotalOutputTokens is the sum of output tokens across all runs.
	TotalOutputTokens int32
}

// stageNameToEnum maps pipeline stage names to proto ProcessingStage enum values.
var stageNameToEnum = map[string]contentv1.ProcessingStage{
	"parse":            contentv1.ProcessingStage_PROCESSING_STAGE_PARSE,
	"segment":          contentv1.ProcessingStage_PROCESSING_STAGE_SEGMENT,
	"triage":           contentv1.ProcessingStage_PROCESSING_STAGE_TRIAGE,
	"extract_ner":      contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT_NER,
	"extract_semantic": contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT_SEMANTIC,
	"resolve":          contentv1.ProcessingStage_PROCESSING_STAGE_RESOLVE,
	"analyze":          contentv1.ProcessingStage_PROCESSING_STAGE_ANALYZE,
	"persist":          contentv1.ProcessingStage_PROCESSING_STAGE_PERSIST,
	"embed":            contentv1.ProcessingStage_PROCESSING_STAGE_EMBED,
}

// DBStatusToProtoState maps a sources.processing_status DB string to a
// contentv1.ProcessingState proto enum. This is the authoritative mapping for
// the sources table status field and is shared by contentservice and
// conversationservice.
func DBStatusToProtoState(s string) contentv1.ProcessingState {
	switch s {
	case "pending":
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	case "processing":
		return contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS
	case "completed":
		return contentv1.ProcessingState_PROCESSING_STATE_COMPLETED
	case "failed":
		return contentv1.ProcessingState_PROCESSING_STATE_FAILED
	case "cancelled":
		return contentv1.ProcessingState_PROCESSING_STATE_CANCELLED
	case "rejected":
		return contentv1.ProcessingState_PROCESSING_STATE_REJECTED
	case "skipped":
		return contentv1.ProcessingState_PROCESSING_STATE_SKIPPED
	default:
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	}
}

// DBRunStatusToStageStatus converts a pipeline_run status string to proto StageStatus.
func DBRunStatusToStageStatus(s string) contentv1.StageStatus {
	switch s {
	case "pending":
		return contentv1.StageStatus_STAGE_STATUS_PENDING
	case "running":
		return contentv1.StageStatus_STAGE_STATUS_RUNNING
	case "completed":
		return contentv1.StageStatus_STAGE_STATUS_COMPLETED
	case "failed":
		return contentv1.StageStatus_STAGE_STATUS_FAILED
	case "skipped":
		return contentv1.StageStatus_STAGE_STATUS_SKIPPED
	default:
		return contentv1.StageStatus_STAGE_STATUS_UNSPECIFIED
	}
}

// BuildProcessingStatus builds a ProcessingStatusResult from a slice of pipeline runs.
// Runs should be ordered by created_at DESC (newest first) so that the first
// occurrence of each stage is the latest run for that stage.
//
// This is the shared logic used by both contentservice.GetProcessingStatus
// and conversationservice.GetConversationProcessingStatus.
func BuildProcessingStatus(runs []PipelineRun) *ProcessingStatusResult {
	result := &ProcessingStatusResult{}

	seenStages := make(map[string]bool)
	var triageParsedData json.RawMessage

	for i := range runs {
		run := &runs[i]

		// Accumulate token counts across all runs (not just latest per stage).
		if run.InputTokens != nil {
			result.TotalInputTokens += int32(*run.InputTokens)
		}
		if run.OutputTokens != nil {
			result.TotalOutputTokens += int32(*run.OutputTokens)
		}
		if run.DurationMS != nil {
			result.TotalDurationMS += int64(*run.DurationMS)
		}

		// Track Langfuse trace ID (prefer the most recent non-nil value).
		if run.LangfuseTraceID != nil && result.LangfuseTraceID == nil {
			result.LangfuseTraceID = run.LangfuseTraceID
		}

		// Track time bounds.
		if result.StartedAt == nil || run.CreatedAt.Before(*result.StartedAt) {
			t := run.CreatedAt
			result.StartedAt = &t
		}
		if run.DurationMS != nil {
			finishedAt := run.CreatedAt.Add(time.Duration(*run.DurationMS) * time.Millisecond)
			if result.FinishedAt == nil || finishedAt.After(*result.FinishedAt) {
				result.FinishedAt = &finishedAt
			}
		}

		// Build stage result only for the latest run per stage.
		if seenStages[run.Stage] {
			continue
		}
		seenStages[run.Stage] = true

		// Capture triage parsed_data for contribution info extraction.
		if run.Stage == "triage" && len(run.ParsedData) > 0 {
			triageParsedData = run.ParsedData
		}

		protoStage, known := stageNameToEnum[run.Stage]
		if !known {
			// Unknown stage — skip to avoid polluting the result.
			continue
		}

		sr := &contentv1.StageResult{
			Stage:  protoStage,
			Status: DBRunStatusToStageStatus(run.Status),
		}
		if run.DurationMS != nil {
			ms := int64(*run.DurationMS)
			sr.DurationMs = &ms
		}
		if run.ModelID != nil {
			sr.ModelId = run.ModelID
		}
		if run.SkipReason != nil {
			sr.SkipReason = run.SkipReason
		}
		if run.InputTokens != nil {
			v := int32(*run.InputTokens)
			sr.InputTokens = &v
		}
		if run.OutputTokens != nil {
			v := int32(*run.OutputTokens)
			sr.OutputTokens = &v
		}

		result.Stages = append(result.Stages, sr)
	}

	// Extract contribution info from triage parsed_data.
	if len(triageParsedData) > 0 {
		var triageData map[string]interface{}
		if err := json.Unmarshal(triageParsedData, &triageData); err == nil {
			if v, ok := triageData["content_contribution"].(string); ok {
				result.ContentContribution = v
			}
			if v, ok := triageData["contribution_reason"].(string); ok {
				result.ContributionReason = v
			}
			if v, ok := triageData["category"].(string); ok {
				result.TriageCategory = v
			}
			if v, ok := triageData["importance"].(string); ok {
				result.TriageImportance = v
			}
		}
	}

	return result
}

// DeriveProcessingState derives an overall contentv1.ProcessingState from a
// set of pipeline runs. Used when no authoritative processing_status field is
// available (e.g. when aggregating conversation item status without a content
// record lookup).
//
// Rules (in order of precedence):
//  1. No runs → PENDING
//  2. Any run with status "failed" → FAILED
//  3. Any run with status "running" → IN_PROGRESS
//  4. All runs have status "completed" or "skipped" → COMPLETED
//  5. Otherwise → IN_PROGRESS
func DeriveProcessingState(runs []PipelineRun) contentv1.ProcessingState {
	if len(runs) == 0 {
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	}

	hasFailed := false
	hasRunning := false
	allDone := true

	for i := range runs {
		switch runs[i].Status {
		case "failed":
			hasFailed = true
		case "running":
			hasRunning = true
			allDone = false
		case "completed", "skipped":
			// these are terminal states for a stage
		default:
			// pending or unknown — not done
			allDone = false
		}
	}

	switch {
	case hasFailed:
		return contentv1.ProcessingState_PROCESSING_STATE_FAILED
	case hasRunning:
		return contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS
	case allDone:
		return contentv1.ProcessingState_PROCESSING_STATE_COMPLETED
	default:
		return contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS
	}
}

// ApplyToProtoProcessingStatus applies the ProcessingStatusResult fields to
// an existing contentv1.ProcessingStatus proto message. Call this after
// BuildProcessingStatus to populate the common fields.
func ApplyToProtoProcessingStatus(ps *ProcessingStatusResult, proto *contentv1.ProcessingStatus) {
	proto.Stages = ps.Stages
	proto.ContentContribution = ps.ContentContribution
	proto.ContributionReason = ps.ContributionReason
	proto.TriageCategory = ps.TriageCategory
	proto.TriageImportance = ps.TriageImportance

	if ps.StartedAt != nil {
		proto.StartedAt = timestamppb.New(*ps.StartedAt)
	}
	if ps.FinishedAt != nil {
		proto.FinishedAt = timestamppb.New(*ps.FinishedAt)
	}
	if ps.TotalDurationMS > 0 {
		proto.TotalDurationMs = &ps.TotalDurationMS
	}
	if ps.LangfuseTraceID != nil {
		proto.LangfuseTraceId = ps.LangfuseTraceID
	}
	if ps.TotalInputTokens > 0 {
		proto.TotalInputTokens = &ps.TotalInputTokens
	}
	if ps.TotalOutputTokens > 0 {
		proto.TotalOutputTokens = &ps.TotalOutputTokens
	}
}
