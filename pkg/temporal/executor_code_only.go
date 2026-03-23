package temporal

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// CodeOnlyExecutor handles pipeline stages with stage_kind="code_only".
// These stages dispatch a single Temporal activity with no LLM calls:
// parse, parse_transcript, resolve, enrich_entities, persist.
//
// The executor looks up the activity name in StageActivityMap, wraps the
// call with a timeout from StageConfig.TimeoutSeconds (falling back to
// FastActivityOptions), and captures the activity output in StageOutput.RawData.
//
// Optional stages capture activity errors in StageOutput.Error and return
// Success=true so the pipeline continues unblocked.
type CodeOnlyExecutor struct{}

// Execute implements StageExecutor for code_only stages.
//
// Behaviour:
//   - Returns Skipped=true when Config.SkipWhenLow is set and input.SkipDeep is true.
//   - Returns an error if the stage has no activity mapping in StageActivityMap.
//   - Returns an error if the stage maps to more than one activity (multi-activity
//     stages belong to a different executor, e.g. StructuredExtractExecutor).
//   - Applies Config.TimeoutSeconds as StartToCloseTimeout when > 0, with
//     half-duration HeartbeatTimeout; otherwise uses FastActivityOptions defaults.
//   - Captures the activity output as JSON in StageOutput.RawData.
//   - When Config.Optional is true, activity errors are captured in StageOutput.Error
//     and Success=true is returned so the pipeline continues.
func (e *CodeOnlyExecutor) Execute(ctx workflow.Context, input StageInput) (StageOutput, error) {
	start := workflow.Now(ctx)

	// Skip when triage returned SkipDeep and this stage is marked skip_when_low.
	if input.Config.SkipWhenLow && input.SkipDeep {
		return StageOutput{
			Success:    true,
			Skipped:    true,
			SkipReason: "skip_deep",
		}, nil
	}

	// Resolve activity name from the shared StageActivityMap.
	activities, ok := StageActivityMap[input.Config.Stage]
	if !ok {
		return StageOutput{}, fmt.Errorf("code_only stage %q: no activity mapping in StageActivityMap", input.Config.Stage)
	}
	if len(activities) != 1 {
		return StageOutput{}, fmt.Errorf("code_only stage %q: expected 1 activity, got %d (use a multi-activity executor)", input.Config.Stage, len(activities))
	}
	activityName := activities[0]

	// Build activity options: start from fast defaults, apply per-stage timeout if configured.
	opts := FastActivityOptions()
	opts = WithNonRetryableErrors(opts, NonRetryableErrors()...)
	if input.Config.TimeoutSeconds > 0 {
		d := time.Duration(input.Config.TimeoutSeconds) * time.Second
		opts.StartToCloseTimeout = d
		if input.Config.TimeoutSeconds > 1 {
			opts.HeartbeatTimeout = time.Duration(input.Config.TimeoutSeconds/2) * time.Second
		}
	}

	// Execute the activity and capture its output as raw JSON.
	var rawResult json.RawMessage
	err := workflow.ExecuteActivity(
		workflow.WithActivityOptions(ctx, opts),
		activityName,
		input.Payload,
	).Get(ctx, &rawResult)

	durationMs := workflow.Now(ctx).Sub(start).Milliseconds()

	if err != nil {
		if input.Config.Optional {
			// Non-blocking: capture the error but return success so the pipeline continues.
			return StageOutput{
				Success:    true,
				Error:      err.Error(),
				DurationMs: durationMs,
			}, nil
		}
		return StageOutput{
			Success:    false,
			Error:      err.Error(),
			DurationMs: durationMs,
		}, err
	}

	return StageOutput{
		Success:    true,
		RawData:    rawResult,
		DurationMs: durationMs,
	}, nil
}
