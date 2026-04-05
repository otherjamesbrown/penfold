// Package validation provides validation reporting functionality.
package validation

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"sync"
	"time"
)

// ValidationReport contains the complete validation results and summary.
type ValidationReport struct {
	mu sync.RWMutex

	// Metadata.
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version"`
	Environment string    `json:"environment,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`

	// Results organized by category.
	Results map[string][]ValidationResult `json:"results"`

	// Summary statistics.
	Summary ReportSummary `json:"summary"`
}

// ReportSummary contains aggregated statistics about the validation run.
type ReportSummary struct {
	TotalChecks    int                      `json:"total_checks"`
	PassedChecks   int                      `json:"passed_checks"`
	FailedChecks   int                      `json:"failed_checks"`
	WarningChecks  int                      `json:"warning_checks"`
	SkippedChecks  int                      `json:"skipped_checks"`
	CriticalFailed int                      `json:"critical_failed"`
	OverallStatus  CheckStatus              `json:"overall_status"`
	Duration       time.Duration            `json:"duration_ms"`
	ByCategory     map[string]CategoryStats `json:"by_category"`
}

// CategoryStats contains statistics for a single category.
type CategoryStats struct {
	Total   int         `json:"total"`
	Passed  int         `json:"passed"`
	Failed  int         `json:"failed"`
	Warning int         `json:"warning"`
	Skipped int         `json:"skipped"`
	Status  CheckStatus `json:"status"`
}

// NewValidationReport creates a new validation report.
func NewValidationReport() *ValidationReport {
	return &ValidationReport{
		Title:   "Penfold Go Migration Validation Report",
		Version: "1.0.0",
		Results: make(map[string][]ValidationResult),
		Summary: ReportSummary{
			ByCategory: make(map[string]CategoryStats),
		},
	}
}

// AddResult adds a validation result to the report.
func (r *ValidationReport) AddResult(category string, result ValidationResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Results[category] == nil {
		r.Results[category] = make([]ValidationResult, 0)
	}
	r.Results[category] = append(r.Results[category], result)
}

// calculateSummary computes the summary statistics from results.
func (r *ValidationReport) calculateSummary() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Summary = ReportSummary{
		ByCategory: make(map[string]CategoryStats),
	}

	for category, results := range r.Results {
		stats := CategoryStats{}

		for _, result := range results {
			stats.Total++
			r.Summary.TotalChecks++

			switch result.Status {
			case StatusPass:
				stats.Passed++
				r.Summary.PassedChecks++
			case StatusFail:
				stats.Failed++
				r.Summary.FailedChecks++
			case StatusWarn:
				stats.Warning++
				r.Summary.WarningChecks++
			case StatusSkip:
				stats.Skipped++
				r.Summary.SkippedChecks++
			}
		}

		// Determine category status.
		if stats.Failed > 0 {
			stats.Status = StatusFail
		} else if stats.Warning > 0 {
			stats.Status = StatusWarn
		} else if stats.Passed > 0 {
			stats.Status = StatusPass
		} else {
			stats.Status = StatusSkip
		}

		r.Summary.ByCategory[category] = stats
	}

	// Calculate overall status.
	if r.Summary.FailedChecks > 0 {
		r.Summary.OverallStatus = StatusFail
	} else if r.Summary.WarningChecks > 0 {
		r.Summary.OverallStatus = StatusWarn
	} else if r.Summary.PassedChecks > 0 {
		r.Summary.OverallStatus = StatusPass
	} else {
		r.Summary.OverallStatus = StatusSkip
	}

	r.Summary.Duration = r.EndTime.Sub(r.StartTime)
}

// GetResults returns all results for a category.
func (r *ValidationReport) GetResults(category string) []ValidationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if results, ok := r.Results[category]; ok {
		// Return a copy.
		copied := make([]ValidationResult, len(results))
		copy(copied, results)
		return copied
	}
	return nil
}

// GetAllResults returns all results across all categories.
func (r *ValidationReport) GetAllResults() map[string][]ValidationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string][]ValidationResult)
	for k, v := range r.Results {
		copied := make([]ValidationResult, len(v))
		copy(copied, v)
		results[k] = copied
	}
	return results
}

// GetFailedResults returns only failed results.
func (r *ValidationReport) GetFailedResults() []ValidationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var failed []ValidationResult
	for _, results := range r.Results {
		for _, result := range results {
			if result.Status == StatusFail {
				failed = append(failed, result)
			}
		}
	}
	return failed
}

// GenerateReport creates a report with the given options.
func (r *ValidationReport) GenerateReport(w io.Writer, format ReportFormat) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch format {
	case FormatJSON:
		return r.generateJSON(w)
	case FormatHTML:
		return r.generateHTML(w)
	case FormatText:
		return r.generateText(w)
	default:
		return fmt.Errorf("unsupported report format: %s", format)
	}
}

// ReportFormat represents the output format for reports.
type ReportFormat string

const (
	// FormatJSON outputs the report as JSON.
	FormatJSON ReportFormat = "json"
	// FormatHTML outputs the report as HTML.
	FormatHTML ReportFormat = "html"
	// FormatText outputs the report as plain text.
	FormatText ReportFormat = "text"
)

// generateJSON writes the report as JSON.
func (r *ValidationReport) generateJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// generateText writes the report as plain text.
func (r *ValidationReport) generateText(w io.Writer) error {
	// Header.
	_, _ = fmt.Fprintf(w, "================================================================================\n")
	_, _ = fmt.Fprintf(w, "%s\n", r.Title)
	_, _ = fmt.Fprintf(w, "================================================================================\n\n")

	_, _ = fmt.Fprintf(w, "Version:     %s\n", r.Version)
	if r.Environment != "" {
		_, _ = fmt.Fprintf(w, "Environment: %s\n", r.Environment)
	}
	_, _ = fmt.Fprintf(w, "Start Time:  %s\n", r.StartTime.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "End Time:    %s\n", r.EndTime.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Duration:    %s\n\n", r.Summary.Duration)

	// Overall Status.
	_, _ = fmt.Fprintf(w, "OVERALL STATUS: %s\n", r.Summary.OverallStatus)
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n\n")

	// Summary.
	_, _ = fmt.Fprintf(w, "SUMMARY\n")
	_, _ = fmt.Fprintf(w, "  Total Checks:   %d\n", r.Summary.TotalChecks)
	_, _ = fmt.Fprintf(w, "  Passed:         %d\n", r.Summary.PassedChecks)
	_, _ = fmt.Fprintf(w, "  Failed:         %d\n", r.Summary.FailedChecks)
	_, _ = fmt.Fprintf(w, "  Warnings:       %d\n", r.Summary.WarningChecks)
	_, _ = fmt.Fprintf(w, "  Skipped:        %d\n\n", r.Summary.SkippedChecks)

	// Results by category.
	_, _ = fmt.Fprintf(w, "RESULTS BY CATEGORY\n")
	_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")

	// Sort categories for consistent output.
	categories := make([]string, 0, len(r.Results))
	for cat := range r.Results {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, category := range categories {
		results := r.Results[category]
		stats := r.Summary.ByCategory[category]

		_, _ = fmt.Fprintf(w, "\n[%s] Status: %s (%d/%d passed)\n", category, stats.Status, stats.Passed, stats.Total)
		_, _ = fmt.Fprintf(w, "  %-40s %-8s %-12s %s\n", "Check", "Status", "Duration", "Message")
		_, _ = fmt.Fprintf(w, "  %-40s %-8s %-12s %s\n", "-----", "------", "--------", "-------")

		for _, result := range results {
			durationStr := fmt.Sprintf("%.1fms", float64(result.Duration.Microseconds())/1000)
			message := result.Message
			if result.Error != "" {
				message = result.Error
			}
			if len(message) > 50 {
				message = message[:47] + "..."
			}
			_, _ = fmt.Fprintf(w, "  %-40s %-8s %-12s %s\n",
				truncateString(result.Name, 40),
				result.Status,
				durationStr,
				message,
			)
		}
	}

	// Failed checks detail.
	failed := r.GetFailedResults()
	if len(failed) > 0 {
		_, _ = fmt.Fprintf(w, "\n\nFAILED CHECKS DETAIL\n")
		_, _ = fmt.Fprintf(w, "--------------------------------------------------------------------------------\n")
		for _, result := range failed {
			_, _ = fmt.Fprintf(w, "\n  %s:\n", result.Name)
			if result.Error != "" {
				_, _ = fmt.Fprintf(w, "    Error: %s\n", result.Error)
			}
			if result.Details != nil {
				_, _ = fmt.Fprintf(w, "    Details: %v\n", result.Details)
			}
		}
	}

	_, _ = fmt.Fprintf(w, "\n================================================================================\n")
	_, _ = fmt.Fprintf(w, "Report generated at %s\n", time.Now().Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "================================================================================\n")

	return nil
}

// truncateString truncates a string to the specified length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// generateHTML writes the report as HTML.
func (r *ValidationReport) generateHTML(w io.Writer) error {
	const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        :root {
            --pass-color: #28a745;
            --fail-color: #dc3545;
            --warn-color: #ffc107;
            --skip-color: #6c757d;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f8f9fa;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 20px;
        }
        .header h1 {
            margin: 0 0 10px 0;
        }
        .meta {
            opacity: 0.9;
            font-size: 14px;
        }
        .summary-card {
            background: white;
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .status-badge {
            display: inline-block;
            padding: 5px 15px;
            border-radius: 20px;
            font-weight: bold;
            color: white;
        }
        .status-PASS { background-color: var(--pass-color); }
        .status-FAIL { background-color: var(--fail-color); }
        .status-WARN { background-color: var(--warn-color); color: #333; }
        .status-SKIP { background-color: var(--skip-color); }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
            gap: 15px;
            margin-top: 20px;
        }
        .stat-box {
            text-align: center;
            padding: 15px;
            border-radius: 8px;
            background: #f8f9fa;
        }
        .stat-box .number {
            font-size: 32px;
            font-weight: bold;
        }
        .stat-box .label {
            font-size: 14px;
            color: #666;
        }
        .category-section {
            background: white;
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .category-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 2px solid #eee;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #eee;
        }
        th {
            background-color: #f8f9fa;
            font-weight: 600;
        }
        tr:hover {
            background-color: #f8f9fa;
        }
        .error-text {
            color: var(--fail-color);
            font-size: 13px;
        }
        .footer {
            text-align: center;
            padding: 20px;
            color: #666;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Title}}</h1>
        <div class="meta">
            Version: {{.Version}} |
            Started: {{.StartTime.Format "2006-01-02 15:04:05"}} |
            Duration: {{.Summary.Duration}}
        </div>
    </div>

    <div class="summary-card">
        <h2>Overall Status: <span class="status-badge status-{{.Summary.OverallStatus}}">{{.Summary.OverallStatus}}</span></h2>
        <div class="stats-grid">
            <div class="stat-box">
                <div class="number">{{.Summary.TotalChecks}}</div>
                <div class="label">Total</div>
            </div>
            <div class="stat-box" style="color: var(--pass-color);">
                <div class="number">{{.Summary.PassedChecks}}</div>
                <div class="label">Passed</div>
            </div>
            <div class="stat-box" style="color: var(--fail-color);">
                <div class="number">{{.Summary.FailedChecks}}</div>
                <div class="label">Failed</div>
            </div>
            <div class="stat-box" style="color: var(--warn-color);">
                <div class="number">{{.Summary.WarningChecks}}</div>
                <div class="label">Warnings</div>
            </div>
            <div class="stat-box" style="color: var(--skip-color);">
                <div class="number">{{.Summary.SkippedChecks}}</div>
                <div class="label">Skipped</div>
            </div>
        </div>
    </div>

    {{range $category, $results := .Results}}
    <div class="category-section">
        <div class="category-header">
            <h3>{{$category}}</h3>
            {{with index $.Summary.ByCategory $category}}
            <span class="status-badge status-{{.Status}}">{{.Passed}}/{{.Total}} Passed</span>
            {{end}}
        </div>
        <table>
            <thead>
                <tr>
                    <th>Check</th>
                    <th>Status</th>
                    <th>Duration</th>
                    <th>Message</th>
                </tr>
            </thead>
            <tbody>
                {{range $results}}
                <tr>
                    <td>{{.Name}}</td>
                    <td><span class="status-badge status-{{.Status}}">{{.Status}}</span></td>
                    <td>{{printf "%.1f" (divFloat .Duration.Microseconds 1000)}}ms</td>
                    <td>
                        {{if .Error}}<span class="error-text">{{.Error}}</span>{{else}}{{.Message}}{{end}}
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    <div class="footer">
        Report generated at {{now.Format "2006-01-02 15:04:05 MST"}}
    </div>
</body>
</html>`

	funcMap := template.FuncMap{
		"now": time.Now,
		"divFloat": func(a int64, b int64) float64 {
			return float64(a) / float64(b)
		},
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	return tmpl.Execute(w, r)
}

// ExportJSON exports the report to JSON format.
func (r *ValidationReport) ExportJSON(w io.Writer) error {
	return r.GenerateReport(w, FormatJSON)
}

// ExportHTML exports the report to HTML format.
func (r *ValidationReport) ExportHTML(w io.Writer) error {
	return r.GenerateReport(w, FormatHTML)
}

// ExportText exports the report to text format.
func (r *ValidationReport) ExportText(w io.Writer) error {
	return r.GenerateReport(w, FormatText)
}

// PassRate returns the percentage of passed checks.
func (r *ValidationReport) PassRate() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Summary.TotalChecks == 0 {
		return 0
	}
	return float64(r.Summary.PassedChecks) / float64(r.Summary.TotalChecks) * 100
}

// IsPassing returns true if no critical checks failed.
func (r *ValidationReport) IsPassing() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Summary.OverallStatus != StatusFail
}

// MigrationStatus represents the overall migration status.
type MigrationStatus struct {
	Phase            string    `json:"phase"`
	Progress         float64   `json:"progress"`
	ServicesReady    int       `json:"services_ready"`
	TotalServices    int       `json:"total_services"`
	ValidationPassed bool      `json:"validation_passed"`
	LastValidation   time.Time `json:"last_validation"`
	Issues           []string  `json:"issues,omitempty"`
}

// GetMigrationStatus returns the current migration status based on the report.
func (r *ValidationReport) GetMigrationStatus() *MigrationStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := &MigrationStatus{
		LastValidation: r.EndTime,
		Issues:         make([]string, 0),
	}

	// Count services.
	if serviceResults, ok := r.Results["services"]; ok {
		status.TotalServices = len(serviceResults)
		for _, result := range serviceResults {
			if result.Status == StatusPass {
				status.ServicesReady++
			} else if result.Status == StatusFail {
				status.Issues = append(status.Issues, fmt.Sprintf("Service %s: %s", result.Name, result.Error))
			}
		}
	}

	// Determine phase.
	servicePercent := float64(status.ServicesReady) / float64(status.TotalServices) * 100
	switch {
	case servicePercent == 0:
		status.Phase = "not_started"
		status.Progress = 0
	case servicePercent < 50:
		status.Phase = "early_migration"
		status.Progress = servicePercent * 0.3
	case servicePercent < 100:
		status.Phase = "migration_in_progress"
		status.Progress = 30 + (servicePercent-50)*0.4
	case r.Summary.OverallStatus == StatusPass:
		status.Phase = "migration_complete"
		status.Progress = 100
		status.ValidationPassed = true
	default:
		status.Phase = "validation_pending"
		status.Progress = 90
	}

	return status
}
