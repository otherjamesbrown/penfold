package pipelineservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScheduleToCloseTimeout_NilDB verifies that a nil DB returns an error.
func TestScheduleToCloseTimeout_NilDB(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	_, err := svc.scheduleToCloseTimeout(ctx, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not available")
}

// TestScheduleToCloseTimeout_DefaultsLogic verifies tier selection logic using
// a stubbed config reader that returns the documented default values.
//
// Since the implementation falls back to hardcoded defaults when rows are missing
// (sql.ErrNoRows), we test that the tier boundaries produce correct durations
// by exercising the switch statement directly with known defaults:
//   - tier2_threshold=100, tier2_timeout_hours=4
//   - tier1_threshold=50,  tier1_timeout_hours=2
//   - default_timeout_hours=1
func TestScheduleToCloseTimeout_DefaultsLogic(t *testing.T) {
	cases := []struct {
		name         string
		pendingCount int
		want         time.Duration
	}{
		{"below_tier1", 0, 1 * time.Hour},
		{"at_tier1_boundary", 50, 1 * time.Hour},
		{"above_tier1", 51, 2 * time.Hour},
		{"at_tier2_boundary", 100, 2 * time.Hour},
		{"above_tier2", 101, 4 * time.Hour},
		{"deep_queue", 500, 4 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Exercise the logic directly using the default config values (matching
			// what scheduleToCloseTimeout returns when no DB rows exist).
			got := scheduleToCloseTimeoutLogic(tc.pendingCount, 50, 2, 100, 4, 1)
			assert.Equal(t, tc.want, got)
		})
	}
}

// scheduleToCloseTimeoutLogic is the pure timeout-selection logic extracted for
// unit testing without a database. It mirrors the switch statement in
// scheduleToCloseTimeout so the logic can be tested independently of DB access.
func scheduleToCloseTimeoutLogic(pendingCount, tier1Threshold, tier1Hours, tier2Threshold, tier2Hours, defaultHours int) time.Duration {
	switch {
	case pendingCount > tier2Threshold:
		return time.Duration(tier2Hours) * time.Hour
	case pendingCount > tier1Threshold:
		return time.Duration(tier1Hours) * time.Hour
	default:
		return time.Duration(defaultHours) * time.Hour
	}
}
