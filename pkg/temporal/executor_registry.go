package temporal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LangfusePhaseReport carries the data captured by LangfuseWrappedExecutor for
// each stage execution. The production reporter maps this to ReportLangfusePhaseInput
// and calls the ReportLangfusePhase Temporal activity.
type LangfusePhaseReport struct {
	PhaseID       string
	TraceID       string
	PhaseName     string    // Stage name (stage.Stage)
	StageKind     string    // Stage kind (stage.StageKind)
	StartTime     time.Time
	EndTime       time.Time
	Success       bool
	StatusMessage string // Non-empty on failure
}

// LangfuseReporter is called by LangfuseWrappedExecutor after each stage execution.
// In production the implementation calls the ReportLangfusePhase Temporal activity.
// In tests a mock implementation is injected via NewExecutorRegistry.
type LangfuseReporter interface {
	ReportPhase(ctx context.Context, report LangfusePhaseReport) error
}

// LangfuseWrappedExecutor wraps a StageExecutor and automatically reports a
// Langfuse phase span after every Execute call. The PhaseID is written into
// StageOutput.LangfusePhaseID so downstream stages and the dispatch loop can
// reference it.
type LangfuseWrappedExecutor struct {
	inner    StageExecutor
	reporter LangfuseReporter
}

// Execute records timing, delegates to the inner executor, then reports to Langfuse.
// The reporter is called best-effort — its error is not propagated to the caller.
func (w *LangfuseWrappedExecutor) Execute(ctx context.Context, stage PipelineStageConfig, input StageInput) (StageOutput, error) {
	phaseID := uuid.New().String()
	start := time.Now()

	output, err := w.inner.Execute(ctx, stage, input)

	end := time.Now()

	// Always populate the phase ID and duration, even on error, so the caller
	// has a reference for Langfuse lookups.
	output.LangfusePhaseID = phaseID
	output.Duration = end.Sub(start)

	report := LangfusePhaseReport{
		PhaseID:   phaseID,
		TraceID:   input.LangfuseTraceID,
		PhaseName: stage.Stage,
		StageKind: stage.StageKind,
		StartTime: start,
		EndTime:   end,
		Success:   err == nil && output.Success,
	}
	if err != nil {
		report.StatusMessage = err.Error()
	} else if !output.Success {
		report.StatusMessage = output.Error
	}

	_ = w.reporter.ReportPhase(ctx, report)

	return output, err
}

// ExecutorRegistry maps stage_kind strings to StageExecutor implementations.
// All registered executors are automatically wrapped with Langfuse tracing.
// Safe for concurrent use after initial registration.
type ExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[string]StageExecutor
	reporter  LangfuseReporter
}

// NewExecutorRegistry creates a registry with the given Langfuse reporter.
// The reporter is injected into every LangfuseWrappedExecutor created by Register.
func NewExecutorRegistry(reporter LangfuseReporter) *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[string]StageExecutor),
		reporter:  reporter,
	}
}

// Register wraps executor with Langfuse tracing and stores it under kind.
// Overwrites any existing registration for the same stage_kind.
func (r *ExecutorRegistry) Register(kind string, executor StageExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[kind] = &LangfuseWrappedExecutor{inner: executor, reporter: r.reporter}
}

// Get returns the wrapped executor for stage_kind.
// Returns a descriptive error if the kind has not been registered.
func (r *ExecutorRegistry) Get(kind string) (StageExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[kind]
	if !ok {
		return nil, fmt.Errorf("executor registry: unknown stage_kind %q", kind)
	}
	return executor, nil
}
