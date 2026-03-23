package temporal

import (
	"fmt"
	"sort"
	"strings"

	"context"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ExecutePipeline executes pipeline stages via the generic dispatch loop.
// stages must be pre-sorted by stage_order (use OrderedPipelineStages to sort).
// Returns the accumulated outputs keyed by stage name.
//
// The context must carry an embedded workflow.Context via WithWorkflowContext for
// executors (e.g. EmbeddingExecutor) that call workflow.ExecuteActivity internally.
func ExecutePipeline(
	ctx context.Context,
	registry *ExecutorRegistry,
	stages []PipelineStageConfig,
	input StageInput,
	logger logging.Logger,
) (map[string]StageOutput, error) {
	outputs := make(map[string]StageOutput)

	for _, stage := range stages {
		// (a) Disabled stage: skip entirely, don't record in outputs.
		if !stage.Enabled {
			if logger != nil {
				logger.Debug("dispatch loop: skipping disabled stage",
					logging.F("stage", stage.Stage),
					logging.F("stage_kind", stage.StageKind),
				)
			}
			continue
		}

		// (b) SkipWhenLow gate: when input signals shallow processing, skip low-value stages.
		if input.SkipDeep && stage.SkipWhenLow {
			out := StageOutput{
				Success:    true,
				Skipped:    true,
				SkipReason: "skip_when_low",
				StageName:  stage.Stage,
				StageKind:  stage.StageKind,
			}
			outputs[stage.Stage] = out
			continue
		}

		// (c) Dependency check: all depends_on stages must have succeeded.
		ready, unmetDeps := ResolveDependencies(stage, outputs)
		if !ready {
			reason := fmt.Sprintf("unmet_dependencies: [%s]", strings.Join(unmetDeps, ", "))
			if logger != nil {
				logger.Warn("dispatch loop: skipping stage with unmet dependencies",
					logging.F("stage", stage.Stage),
					logging.F("unmet_deps", strings.Join(unmetDeps, ", ")),
				)
			}
			out := StageOutput{
				Success:    false,
				Skipped:    true,
				SkipReason: reason,
				StageName:  stage.Stage,
				StageKind:  stage.StageKind,
			}
			outputs[stage.Stage] = out
			continue
		}

		// (d) Executor lookup: unknown kinds are skipped with a warning (don't fail pipeline).
		executor, err := registry.Get(stage.StageKind)
		if err != nil {
			if logger != nil {
				logger.Warn("dispatch loop: no executor registered for stage_kind, skipping",
					logging.F("stage", stage.Stage),
					logging.F("stage_kind", stage.StageKind),
				)
			}
			continue
		}

		// (e) Build per-stage input: copy input and set accumulated previous outputs.
		stageInput := input
		stageInput.PreviousOutputs = outputs

		// (f) Execute.
		output, execErr := executor.Execute(ctx, stage, stageInput)

		// (g) Store output.
		outputs[stage.Stage] = output

		// (h) Failure handling.
		if execErr != nil || (!output.Success && !stage.Optional) {
			// Non-optional failure: stop the pipeline.
			if execErr != nil {
				return outputs, execErr
			}
			return outputs, fmt.Errorf("stage %q failed: %s", stage.Stage, output.Error)
		}

		// (i) Optional failure: log and continue.
		if !output.Success && stage.Optional {
			if logger != nil {
				errMsg := output.Error
				if execErr != nil {
					errMsg = execErr.Error()
				}
				logger.Warn("dispatch loop: optional stage failed, continuing",
					logging.F("stage", stage.Stage),
					logging.F("stage_kind", stage.StageKind),
					logging.F("error", errMsg),
				)
			}
			continue
		}
	}

	return outputs, nil
}

// OrderedPipelineStages returns a copy of stages sorted by StageOrder ascending.
func OrderedPipelineStages(stages []PipelineStageConfig) []PipelineStageConfig {
	sorted := make([]PipelineStageConfig, len(stages))
	copy(sorted, stages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StageOrder < sorted[j].StageOrder
	})
	return sorted
}
