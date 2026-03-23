package temporal

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// captureLogger captures log messages for test assertions.
type captureLogger struct {
	warns []string
	infos []string
}

func (l *captureLogger) Debug(msg string, fields ...logging.Field) {}
func (l *captureLogger) Info(msg string, fields ...logging.Field)  { l.infos = append(l.infos, msg) }
func (l *captureLogger) Warn(msg string, fields ...logging.Field)  { l.warns = append(l.warns, msg) }
func (l *captureLogger) Error(msg string, fields ...logging.Field) {}
func (l *captureLogger) With(fields ...logging.Field) logging.Logger {
	return l
}
func (l *captureLogger) WithContext(ctx context.Context) logging.Logger { return l }
func (l *captureLogger) Zerolog() zerolog.Logger                       { return zerolog.Nop() }

func TestStageActivityMap_AllActivitiesExist(t *testing.T) {
	mainActivities := make(map[string]bool)
	for _, a := range AllMainQueueActivities() {
		mainActivities[a] = true
	}

	for stage, activities := range StageActivityMap {
		for _, act := range activities {
			if !mainActivities[act] {
				t.Errorf("StageActivityMap[%q] references activity %q not in AllMainQueueActivities()", stage, act)
			}
		}
	}
}

func TestValidateStageRegistry_AllMatch(t *testing.T) {
	logger := &captureLogger{}

	// All mapped stages are defined and all activities registered
	var defined []string
	for stage := range StageActivityMap {
		defined = append(defined, stage)
	}

	var registered []string
	for _, acts := range StageActivityMap {
		registered = append(registered, acts...)
	}

	ValidateStageRegistry(logger, defined, registered)

	if len(logger.warns) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(logger.warns), logger.warns)
	}
}

func TestValidateStageRegistry_UnknownStage(t *testing.T) {
	logger := &captureLogger{}

	ValidateStageRegistry(logger, []string{"fake_stage"}, AllMainQueueActivities())

	found := false
	for _, w := range logger.warns {
		if w == "pipeline definition references unknown stage (no activity mapping)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about unknown stage, got: %v", logger.warns)
	}
}

func TestValidateStageRegistry_UnregisteredActivity(t *testing.T) {
	logger := &captureLogger{}

	// Define a stage but don't register its activity
	ValidateStageRegistry(logger, []string{"parse"}, []string{})

	found := false
	for _, w := range logger.warns {
		if w == "pipeline stage references unregistered activity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about unregistered activity, got: %v", logger.warns)
	}
}

func TestValidateStageRegistry_OrphanActivity(t *testing.T) {
	logger := &captureLogger{}

	// Register parse activity but don't define the parse stage
	ValidateStageRegistry(logger, []string{}, []string{ActivityParseEmail})

	found := false
	for _, w := range logger.warns {
		if w == "registered activity not referenced by any pipeline definition" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about orphan activity, got: %v", logger.warns)
	}
}
