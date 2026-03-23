package temporal

import (
	"fmt"
	"strings"
)

// ValidatePipelineDependencies checks that the depends_on declarations in a set of
// pipeline stages form a valid DAG — no cycles, and every named dependency exists as
// a stage in the pipeline. Returns a descriptive error on the first violation found.
//
// This is called at pipeline load time (inside FetchPipelineDefinition) so circular
// or missing dependencies are caught before any stage executes.
func ValidatePipelineDependencies(stages []PipelineStageConfig) error {
	// Build a set of known stage names for existence checks.
	known := make(map[string]struct{}, len(stages))
	for _, s := range stages {
		known[s.Stage] = struct{}{}
	}

	// Check every dependency name resolves to a known stage.
	for _, s := range stages {
		for _, dep := range s.DependsOn {
			if _, ok := known[dep]; !ok {
				return fmt.Errorf("stage %q depends on unknown stage %q", s.Stage, dep)
			}
		}
	}

	// Kahn's algorithm: detect cycles via topological sort.
	// inDegree counts how many stages each stage depends on.
	inDegree := make(map[string]int, len(stages))
	// dependents maps each stage to the stages that directly depend on it.
	dependents := make(map[string][]string, len(stages))
	for _, s := range stages {
		if _, ok := inDegree[s.Stage]; !ok {
			inDegree[s.Stage] = 0
		}
		for _, dep := range s.DependsOn {
			inDegree[s.Stage]++
			dependents[dep] = append(dependents[dep], s.Stage)
		}
	}

	// Seed the queue with stages that have no dependencies.
	queue := make([]string, 0, len(stages))
	for _, s := range stages {
		if inDegree[s.Stage] == 0 {
			queue = append(queue, s.Stage)
		}
	}

	processed := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		processed++
		for _, dependent := range dependents[cur] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if processed != len(stages) {
		// Some stages were never processed — they are in a cycle.
		var cycleStages []string
		for stage, deg := range inDegree {
			if deg > 0 {
				cycleStages = append(cycleStages, stage)
			}
		}
		return fmt.Errorf("circular dependency detected among stages: %s", strings.Join(cycleStages, ", "))
	}

	return nil
}

// ResolveDependencies checks whether all depends_on stages for a given stage have
// produced successful outputs. Returns ready=true when all dependencies are met.
// When ready=false, unmetDeps lists the stage names that have not yet succeeded.
//
// A dependency is considered met when the stage has an entry in outputs AND its
// Success field is true. A failed or missing dependency is unmet.
func ResolveDependencies(stage PipelineStageConfig, outputs map[string]StageOutput) (ready bool, unmetDeps []string) {
	if len(stage.DependsOn) == 0 {
		return true, nil
	}

	for _, dep := range stage.DependsOn {
		out, exists := outputs[dep]
		if !exists || !out.Success {
			unmetDeps = append(unmetDeps, dep)
		}
	}

	return len(unmetDeps) == 0, unmetDeps
}
