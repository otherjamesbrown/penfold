package digest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TimeWindow represents a time range for content gathering.
type TimeWindow struct {
	From time.Time
	To   time.Time
}

// CalculateWindow computes a TimeWindow from a window expression.
// Supported expressions:
//   - "7d", "30d"  — relative days from refDate
//   - "1m"         — relative months from refDate
//   - "since_last_run" — from lastRunAt to refDate (errors if lastRunAt is nil)
//   - JSON object: {"from":"2025-01-01","to":"2025-01-07"} — absolute range
func CalculateWindow(expr string, refDate time.Time, lastRunAt *time.Time) (TimeWindow, error) {
	expr = strings.TrimSpace(expr)

	// Try JSON object first
	if strings.HasPrefix(expr, "{") {
		var raw struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal([]byte(expr), &raw); err != nil {
			return TimeWindow{}, fmt.Errorf("parse window JSON: %w", err)
		}
		from, err := time.Parse("2006-01-02", raw.From)
		if err != nil {
			return TimeWindow{}, fmt.Errorf("parse window from date: %w", err)
		}
		to, err := time.Parse("2006-01-02", raw.To)
		if err != nil {
			return TimeWindow{}, fmt.Errorf("parse window to date: %w", err)
		}
		// To is end-of-day
		to = to.Add(24*time.Hour - time.Nanosecond)
		return TimeWindow{From: from, To: to}, nil
	}

	// since_last_run
	if expr == "since_last_run" {
		if lastRunAt == nil {
			return TimeWindow{}, fmt.Errorf("since_last_run requires lastRunAt")
		}
		return TimeWindow{From: *lastRunAt, To: refDate}, nil
	}

	// Relative: Nd or Nm
	if strings.HasSuffix(expr, "d") {
		daysStr := strings.TrimSuffix(expr, "d")
		var days int
		if _, err := fmt.Sscanf(daysStr, "%d", &days); err != nil {
			return TimeWindow{}, fmt.Errorf("parse days: %w", err)
		}
		from := refDate.AddDate(0, 0, -days)
		return TimeWindow{From: from, To: refDate}, nil
	}

	if strings.HasSuffix(expr, "m") {
		monthsStr := strings.TrimSuffix(expr, "m")
		var months int
		if _, err := fmt.Sscanf(monthsStr, "%d", &months); err != nil {
			return TimeWindow{}, fmt.Errorf("parse months: %w", err)
		}
		from := refDate.AddDate(0, -months, 0)
		return TimeWindow{From: from, To: refDate}, nil
	}

	return TimeWindow{}, fmt.Errorf("unsupported window expression: %q", expr)
}
