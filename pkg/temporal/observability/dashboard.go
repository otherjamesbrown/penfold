package observability

import (
	"encoding/json"
	"fmt"
	"time"
)

// GrafanaDashboard represents a Grafana dashboard configuration.
type GrafanaDashboard struct {
	UID           string          `json:"uid"`
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Refresh       string          `json:"refresh,omitempty"`
	SchemaVersion int             `json:"schemaVersion"`
	Panels        []GrafanaPanel  `json:"panels"`
	Templating    GrafanaTemplate `json:"templating,omitempty"`
	Time          GrafanaTime     `json:"time"`
	Annotations   GrafanaAnnotations `json:"annotations,omitempty"`
}

// GrafanaPanel represents a panel in a Grafana dashboard.
type GrafanaPanel struct {
	ID          int                `json:"id"`
	Type        string             `json:"type"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	GridPos     GrafanaGridPos     `json:"gridPos"`
	Targets     []GrafanaTarget    `json:"targets,omitempty"`
	FieldConfig *GrafanaFieldConfig `json:"fieldConfig,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// GrafanaGridPos represents panel position in the grid.
type GrafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// GrafanaTarget represents a query target.
type GrafanaTarget struct {
	RefID      string `json:"refId"`
	Expr       string `json:"expr"`
	LegendFormat string `json:"legendFormat,omitempty"`
	Interval   string `json:"interval,omitempty"`
}

// GrafanaFieldConfig represents field configuration for panels.
type GrafanaFieldConfig struct {
	Defaults  GrafanaFieldDefaults `json:"defaults"`
	Overrides []interface{}        `json:"overrides,omitempty"`
}

// GrafanaFieldDefaults represents default field configuration.
type GrafanaFieldDefaults struct {
	Unit       string            `json:"unit,omitempty"`
	Decimals   int               `json:"decimals,omitempty"`
	Min        *float64          `json:"min,omitempty"`
	Max        *float64          `json:"max,omitempty"`
	Thresholds *GrafanaThresholds `json:"thresholds,omitempty"`
	Color      map[string]string `json:"color,omitempty"`
}

// GrafanaThresholds represents threshold configuration.
type GrafanaThresholds struct {
	Mode  string             `json:"mode"`
	Steps []GrafanaThreshold `json:"steps"`
}

// GrafanaThreshold represents a single threshold step.
type GrafanaThreshold struct {
	Value *float64 `json:"value"`
	Color string   `json:"color"`
}

// GrafanaTemplate represents dashboard variables/templating.
type GrafanaTemplate struct {
	List []GrafanaVariable `json:"list"`
}

// GrafanaVariable represents a dashboard variable.
type GrafanaVariable struct {
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Type    string `json:"type"`
	Query   string `json:"query,omitempty"`
	Current GrafanaVariableCurrent `json:"current,omitempty"`
	Options []GrafanaVariableOption `json:"options,omitempty"`
}

// GrafanaVariableCurrent represents the current value of a variable.
type GrafanaVariableCurrent struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

// GrafanaVariableOption represents a variable option.
type GrafanaVariableOption struct {
	Text     string `json:"text"`
	Value    string `json:"value"`
	Selected bool   `json:"selected,omitempty"`
}

// GrafanaTime represents the time range for a dashboard.
type GrafanaTime struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GrafanaAnnotations represents dashboard annotations.
type GrafanaAnnotations struct {
	List []GrafanaAnnotation `json:"list"`
}

// GrafanaAnnotation represents a single annotation configuration.
type GrafanaAnnotation struct {
	Name       string `json:"name"`
	Enable     bool   `json:"enable"`
	Datasource string `json:"datasource"`
	Expr       string `json:"expr,omitempty"`
}

// DashboardBuilder provides a fluent interface for building Grafana dashboards.
type DashboardBuilder struct {
	dashboard *GrafanaDashboard
	panelID   int
}

// NewDashboardBuilder creates a new dashboard builder.
func NewDashboardBuilder(uid, title string) *DashboardBuilder {
	return &DashboardBuilder{
		dashboard: &GrafanaDashboard{
			UID:           uid,
			Title:         title,
			SchemaVersion: 38,
			Refresh:       "30s",
			Tags:          []string{"temporal", "observability"},
			Time: GrafanaTime{
				From: "now-1h",
				To:   "now",
			},
		},
		panelID: 1,
	}
}

// WithDescription sets the dashboard description.
func (b *DashboardBuilder) WithDescription(desc string) *DashboardBuilder {
	b.dashboard.Description = desc
	return b
}

// WithTags sets the dashboard tags.
func (b *DashboardBuilder) WithTags(tags ...string) *DashboardBuilder {
	b.dashboard.Tags = tags
	return b
}

// WithTimeRange sets the default time range.
func (b *DashboardBuilder) WithTimeRange(from, to string) *DashboardBuilder {
	b.dashboard.Time = GrafanaTime{From: from, To: to}
	return b
}

// WithRefresh sets the auto-refresh interval.
func (b *DashboardBuilder) WithRefresh(refresh string) *DashboardBuilder {
	b.dashboard.Refresh = refresh
	return b
}

// AddPanel adds a panel to the dashboard.
func (b *DashboardBuilder) AddPanel(panel GrafanaPanel) *DashboardBuilder {
	panel.ID = b.panelID
	b.panelID++
	b.dashboard.Panels = append(b.dashboard.Panels, panel)
	return b
}

// Build returns the completed dashboard.
func (b *DashboardBuilder) Build() *GrafanaDashboard {
	return b.dashboard
}

// JSON returns the dashboard as JSON bytes.
func (b *DashboardBuilder) JSON() ([]byte, error) {
	return json.MarshalIndent(b.dashboard, "", "  ")
}

// NewTemporalDashboard creates a pre-configured Temporal observability dashboard.
func NewTemporalDashboard(namespace, serviceName string) *GrafanaDashboard {
	builder := NewDashboardBuilder(
		fmt.Sprintf("temporal-%s-%s", namespace, serviceName),
		fmt.Sprintf("Temporal Observability - %s", serviceName),
	).
		WithDescription(fmt.Sprintf("Temporal workflow and activity observability for %s", serviceName)).
		WithTags("temporal", "workflows", "activities", namespace, serviceName)

	// Add workflow metrics row
	builder.AddPanel(GrafanaPanel{
		Type:  "row",
		Title: "Workflow Metrics",
		GridPos: GrafanaGridPos{H: 1, W: 24, X: 0, Y: 0},
	})

	// Workflow started/completed rate
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Workflow Rate",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 0, Y: 1},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_workflow_started_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Started {{workflow_type}}",
			},
			{
				RefID:        "B",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Completed {{workflow_type}}",
			},
			{
				RefID:        "C",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_workflow_failed_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Failed {{workflow_type}}",
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit: "ops",
			},
		},
	})

	// Workflow duration histogram
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Workflow Duration (p99)",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 12, Y: 1},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_workflow_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p99 {{workflow_type}}",
			},
			{
				RefID:        "B",
				Expr:         fmt.Sprintf(`histogram_quantile(0.95, rate(penfold_temporal_workflow_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p95 {{workflow_type}}",
			},
			{
				RefID:        "C",
				Expr:         fmt.Sprintf(`histogram_quantile(0.50, rate(penfold_temporal_workflow_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p50 {{workflow_type}}",
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit: "s",
			},
		},
	})

	// Workflow success rate gauge
	builder.AddPanel(GrafanaPanel{
		Type:  "gauge",
		Title: "Workflow Success Rate",
		GridPos: GrafanaGridPos{H: 4, W: 6, X: 0, Y: 9},
		Targets: []GrafanaTarget{
			{
				RefID: "A",
				Expr:  fmt.Sprintf(`sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m]))) * 100`, serviceName, serviceName, serviceName),
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit:     "percent",
				Decimals: 1,
				Thresholds: &GrafanaThresholds{
					Mode: "absolute",
					Steps: []GrafanaThreshold{
						{Value: nil, Color: "red"},
						{Value: floatPtr(90), Color: "yellow"},
						{Value: floatPtr(99), Color: "green"},
					},
				},
			},
		},
		Options: map[string]interface{}{
			"showThresholdLabels": false,
			"showThresholdMarkers": true,
		},
	})

	// Add activity metrics row
	builder.AddPanel(GrafanaPanel{
		Type:  "row",
		Title: "Activity Metrics",
		GridPos: GrafanaGridPos{H: 1, W: 24, X: 0, Y: 13},
	})

	// Activity started/completed rate
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Activity Rate",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 0, Y: 14},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_activity_started_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Started {{activity_type}}",
			},
			{
				RefID:        "B",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_activity_completed_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Completed {{activity_type}}",
			},
			{
				RefID:        "C",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_activity_failed_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "Failed {{activity_type}}",
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit: "ops",
			},
		},
	})

	// Activity duration histogram
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Activity Duration (p99)",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 12, Y: 14},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_activity_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p99 {{activity_type}}",
			},
			{
				RefID:        "B",
				Expr:         fmt.Sprintf(`histogram_quantile(0.95, rate(penfold_temporal_activity_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p95 {{activity_type}}",
			},
			{
				RefID:        "C",
				Expr:         fmt.Sprintf(`histogram_quantile(0.50, rate(penfold_temporal_activity_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
				LegendFormat: "p50 {{activity_type}}",
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit: "s",
			},
		},
	})

	// Task queue metrics row
	builder.AddPanel(GrafanaPanel{
		Type:  "row",
		Title: "Task Queue Metrics",
		GridPos: GrafanaGridPos{H: 1, W: 24, X: 0, Y: 22},
	})

	// Task queue depth
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Task Queue Depth",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 0, Y: 23},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`penfold_temporal_task_queue_depth{service="%s"}`, serviceName),
				LegendFormat: "{{task_queue}} {{task_type}}",
			},
		},
	})

	// Task queue poll rate
	builder.AddPanel(GrafanaPanel{
		Type:  "timeseries",
		Title: "Task Queue Poll Rate",
		GridPos: GrafanaGridPos{H: 8, W: 12, X: 12, Y: 23},
		Targets: []GrafanaTarget{
			{
				RefID:        "A",
				Expr:         fmt.Sprintf(`rate(penfold_temporal_task_queue_polls_total{service="%s"}[5m])`, serviceName),
				LegendFormat: "{{task_queue}} {{task_type}}",
			},
		},
		FieldConfig: &GrafanaFieldConfig{
			Defaults: GrafanaFieldDefaults{
				Unit: "ops",
			},
		},
	})

	return builder.Build()
}

// floatPtr returns a pointer to a float64.
func floatPtr(v float64) *float64 {
	return &v
}

// AlertRule represents a Prometheus alerting rule.
type AlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// AlertRuleGroup represents a group of Prometheus alerting rules.
type AlertRuleGroup struct {
	Name  string      `yaml:"name"`
	Rules []AlertRule `yaml:"rules"`
}

// NewTemporalAlertRules creates default alerting rules for Temporal observability.
func NewTemporalAlertRules(serviceName string) *AlertRuleGroup {
	return &AlertRuleGroup{
		Name: fmt.Sprintf("temporal-%s", serviceName),
		Rules: []AlertRule{
			{
				Alert: "TemporalWorkflowHighFailureRate",
				Expr:  fmt.Sprintf(`sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m]))) > 0.05`, serviceName, serviceName, serviceName),
				For:   "5m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "High Temporal workflow failure rate",
					"description": fmt.Sprintf("Workflow failure rate for %s is above 5%% for the last 5 minutes", serviceName),
				},
			},
			{
				Alert: "TemporalWorkflowCriticalFailureRate",
				Expr:  fmt.Sprintf(`sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m]))) > 0.10`, serviceName, serviceName, serviceName),
				For:   "5m",
				Labels: map[string]string{
					"severity": "critical",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "Critical Temporal workflow failure rate",
					"description": fmt.Sprintf("Workflow failure rate for %s is above 10%% for the last 5 minutes", serviceName),
				},
			},
			{
				Alert: "TemporalActivityHighFailureRate",
				Expr:  fmt.Sprintf(`sum(rate(penfold_temporal_activity_failed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_activity_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_activity_failed_total{service="%s"}[5m]))) > 0.05`, serviceName, serviceName, serviceName),
				For:   "5m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "High Temporal activity failure rate",
					"description": fmt.Sprintf("Activity failure rate for %s is above 5%% for the last 5 minutes", serviceName),
				},
			},
			{
				Alert: "TemporalWorkflowSlowDuration",
				Expr:  fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_workflow_duration_seconds_bucket{service="%s"}[5m])) > 300`, serviceName),
				For:   "10m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "Slow Temporal workflow execution",
					"description": fmt.Sprintf("Workflow p99 duration for %s is above 5 minutes", serviceName),
				},
			},
			{
				Alert: "TemporalActivitySlowDuration",
				Expr:  fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_activity_duration_seconds_bucket{service="%s"}[5m])) > 60`, serviceName),
				For:   "10m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "Slow Temporal activity execution",
					"description": fmt.Sprintf("Activity p99 duration for %s is above 1 minute", serviceName),
				},
			},
			{
				Alert: "TemporalTaskQueueBacklog",
				Expr:  fmt.Sprintf(`penfold_temporal_task_queue_depth{service="%s"} > 1000`, serviceName),
				For:   "5m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "Temporal task queue backlog",
					"description": fmt.Sprintf("Task queue depth for %s is above 1000", serviceName),
				},
			},
			{
				Alert: "TemporalNoWorkflowsProcessed",
				Expr:  fmt.Sprintf(`sum(rate(penfold_temporal_workflow_started_total{service="%s"}[5m])) == 0`, serviceName),
				For:   "15m",
				Labels: map[string]string{
					"severity": "warning",
					"service":  serviceName,
				},
				Annotations: map[string]string{
					"summary":     "No Temporal workflows processed",
					"description": fmt.Sprintf("No workflows have been started for %s in the last 15 minutes", serviceName),
				},
			},
		},
	}
}

// SLODefinition represents a Service Level Objective.
type SLODefinition struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Target      float64       `json:"target"`
	Window      time.Duration `json:"window"`
	Metric      string        `json:"metric"`
	Indicator   string        `json:"indicator"`
}

// NewTemporalSLOs creates default SLO definitions for Temporal workflows.
func NewTemporalSLOs(serviceName string) []SLODefinition {
	return []SLODefinition{
		{
			Name:        "Workflow Success Rate",
			Description: "Percentage of workflows that complete successfully",
			Target:      99.0,
			Window:      30 * 24 * time.Hour, // 30 days
			Metric:      fmt.Sprintf(`sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_workflow_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_workflow_failed_total{service="%s"}[5m]))) * 100`, serviceName, serviceName, serviceName),
			Indicator:   "availability",
		},
		{
			Name:        "Workflow Duration p99",
			Description: "99th percentile workflow execution duration",
			Target:      300.0, // 5 minutes
			Window:      7 * 24 * time.Hour, // 7 days
			Metric:      fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_workflow_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
			Indicator:   "latency",
		},
		{
			Name:        "Activity Success Rate",
			Description: "Percentage of activities that complete successfully",
			Target:      99.5,
			Window:      30 * 24 * time.Hour, // 30 days
			Metric:      fmt.Sprintf(`sum(rate(penfold_temporal_activity_completed_total{service="%s"}[5m])) / (sum(rate(penfold_temporal_activity_completed_total{service="%s"}[5m])) + sum(rate(penfold_temporal_activity_failed_total{service="%s"}[5m]))) * 100`, serviceName, serviceName, serviceName),
			Indicator:   "availability",
		},
		{
			Name:        "Activity Duration p99",
			Description: "99th percentile activity execution duration",
			Target:      30.0, // 30 seconds
			Window:      7 * 24 * time.Hour, // 7 days
			Metric:      fmt.Sprintf(`histogram_quantile(0.99, rate(penfold_temporal_activity_duration_seconds_bucket{service="%s"}[5m]))`, serviceName),
			Indicator:   "latency",
		},
	}
}

// DashboardJSON returns the JSON representation of a Grafana dashboard.
func DashboardJSON(dashboard *GrafanaDashboard) (string, error) {
	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal dashboard: %w", err)
	}
	return string(data), nil
}
