package observability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewDashboardBuilder(t *testing.T) {
	t.Run("creates builder with defaults", func(t *testing.T) {
		builder := NewDashboardBuilder("test-uid", "Test Dashboard")
		if builder == nil {
			t.Fatal("NewDashboardBuilder returned nil")
		}
		if builder.dashboard == nil {
			t.Fatal("dashboard is nil")
		}
		if builder.dashboard.UID != "test-uid" {
			t.Errorf("UID = %q, want %q", builder.dashboard.UID, "test-uid")
		}
		if builder.dashboard.Title != "Test Dashboard" {
			t.Errorf("Title = %q, want %q", builder.dashboard.Title, "Test Dashboard")
		}
		if builder.dashboard.SchemaVersion != 38 {
			t.Errorf("SchemaVersion = %d, want %d", builder.dashboard.SchemaVersion, 38)
		}
		if builder.dashboard.Refresh != "30s" {
			t.Errorf("Refresh = %q, want %q", builder.dashboard.Refresh, "30s")
		}
	})
}

func TestDashboardBuilderMethods(t *testing.T) {
	t.Run("WithDescription", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title").
			WithDescription("Test description")

		if builder.dashboard.Description != "Test description" {
			t.Errorf("Description = %q, want %q", builder.dashboard.Description, "Test description")
		}
	})

	t.Run("WithTags", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title").
			WithTags("tag1", "tag2", "tag3")

		if len(builder.dashboard.Tags) != 3 {
			t.Errorf("len(Tags) = %d, want 3", len(builder.dashboard.Tags))
		}
		if builder.dashboard.Tags[0] != "tag1" {
			t.Errorf("Tags[0] = %q, want %q", builder.dashboard.Tags[0], "tag1")
		}
	})

	t.Run("WithTimeRange", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title").
			WithTimeRange("now-6h", "now")

		if builder.dashboard.Time.From != "now-6h" {
			t.Errorf("Time.From = %q, want %q", builder.dashboard.Time.From, "now-6h")
		}
		if builder.dashboard.Time.To != "now" {
			t.Errorf("Time.To = %q, want %q", builder.dashboard.Time.To, "now")
		}
	})

	t.Run("WithRefresh", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title").
			WithRefresh("1m")

		if builder.dashboard.Refresh != "1m" {
			t.Errorf("Refresh = %q, want %q", builder.dashboard.Refresh, "1m")
		}
	})

	t.Run("AddPanel", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title")

		panel1 := GrafanaPanel{
			Type:    "timeseries",
			Title:   "Panel 1",
			GridPos: GrafanaGridPos{H: 8, W: 12, X: 0, Y: 0},
		}
		panel2 := GrafanaPanel{
			Type:    "gauge",
			Title:   "Panel 2",
			GridPos: GrafanaGridPos{H: 4, W: 6, X: 12, Y: 0},
		}

		builder.AddPanel(panel1).AddPanel(panel2)

		if len(builder.dashboard.Panels) != 2 {
			t.Errorf("len(Panels) = %d, want 2", len(builder.dashboard.Panels))
		}

		// Panels should have auto-assigned IDs
		if builder.dashboard.Panels[0].ID != 1 {
			t.Errorf("Panels[0].ID = %d, want 1", builder.dashboard.Panels[0].ID)
		}
		if builder.dashboard.Panels[1].ID != 2 {
			t.Errorf("Panels[1].ID = %d, want 2", builder.dashboard.Panels[1].ID)
		}
	})

	t.Run("Build returns dashboard", func(t *testing.T) {
		builder := NewDashboardBuilder("uid", "Title")
		dashboard := builder.Build()

		if dashboard == nil {
			t.Fatal("Build returned nil")
		}
		if dashboard != builder.dashboard {
			t.Error("Build returned different dashboard instance")
		}
	})

	t.Run("JSON returns valid JSON", func(t *testing.T) {
		builder := NewDashboardBuilder("test-uid", "Test Dashboard").
			WithDescription("A test dashboard")

		jsonBytes, err := builder.JSON()
		if err != nil {
			t.Fatalf("JSON() error: %v", err)
		}

		// Verify it's valid JSON
		var parsed GrafanaDashboard
		if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}

		if parsed.UID != "test-uid" {
			t.Errorf("Parsed UID = %q, want %q", parsed.UID, "test-uid")
		}
	})
}

func TestNewTemporalDashboard(t *testing.T) {
	dashboard := NewTemporalDashboard("default", "worker")

	t.Run("has correct UID", func(t *testing.T) {
		expected := "temporal-default-worker"
		if dashboard.UID != expected {
			t.Errorf("UID = %q, want %q", dashboard.UID, expected)
		}
	})

	t.Run("has correct title", func(t *testing.T) {
		if !strings.Contains(dashboard.Title, "worker") {
			t.Errorf("Title should contain service name, got %q", dashboard.Title)
		}
	})

	t.Run("has panels", func(t *testing.T) {
		if len(dashboard.Panels) == 0 {
			t.Error("Dashboard has no panels")
		}
	})

	t.Run("has workflow metrics panels", func(t *testing.T) {
		foundWorkflowRow := false
		foundWorkflowRate := false
		for _, panel := range dashboard.Panels {
			if panel.Title == "Workflow Metrics" {
				foundWorkflowRow = true
			}
			if panel.Title == "Workflow Rate" {
				foundWorkflowRate = true
			}
		}
		if !foundWorkflowRow {
			t.Error("Missing 'Workflow Metrics' row panel")
		}
		if !foundWorkflowRate {
			t.Error("Missing 'Workflow Rate' panel")
		}
	})

	t.Run("has activity metrics panels", func(t *testing.T) {
		foundActivityRow := false
		foundActivityRate := false
		for _, panel := range dashboard.Panels {
			if panel.Title == "Activity Metrics" {
				foundActivityRow = true
			}
			if panel.Title == "Activity Rate" {
				foundActivityRate = true
			}
		}
		if !foundActivityRow {
			t.Error("Missing 'Activity Metrics' row panel")
		}
		if !foundActivityRate {
			t.Error("Missing 'Activity Rate' panel")
		}
	})

	t.Run("has task queue metrics panels", func(t *testing.T) {
		foundTaskQueueRow := false
		foundTaskQueueDepth := false
		for _, panel := range dashboard.Panels {
			if panel.Title == "Task Queue Metrics" {
				foundTaskQueueRow = true
			}
			if panel.Title == "Task Queue Depth" {
				foundTaskQueueDepth = true
			}
		}
		if !foundTaskQueueRow {
			t.Error("Missing 'Task Queue Metrics' row panel")
		}
		if !foundTaskQueueDepth {
			t.Error("Missing 'Task Queue Depth' panel")
		}
	})

	t.Run("panels have valid targets", func(t *testing.T) {
		for _, panel := range dashboard.Panels {
			if panel.Type == "row" {
				continue // Row panels don't have targets
			}
			if len(panel.Targets) == 0 && panel.Type != "gauge" {
				// Only check non-gauge panels without targets
				continue
			}
			for _, target := range panel.Targets {
				if target.Expr == "" {
					t.Errorf("Panel %q has target with empty Expr", panel.Title)
				}
				if !strings.Contains(target.Expr, "worker") {
					t.Errorf("Panel %q target should contain service name in query", panel.Title)
				}
			}
		}
	})
}

func TestNewTemporalAlertRules(t *testing.T) {
	rules := NewTemporalAlertRules("myservice")

	t.Run("has correct group name", func(t *testing.T) {
		if rules.Name != "temporal-myservice" {
			t.Errorf("Name = %q, want %q", rules.Name, "temporal-myservice")
		}
	})

	t.Run("has rules", func(t *testing.T) {
		if len(rules.Rules) == 0 {
			t.Error("No alert rules defined")
		}
	})

	t.Run("has workflow failure rate alerts", func(t *testing.T) {
		foundWarning := false
		foundCritical := false
		for _, rule := range rules.Rules {
			if rule.Alert == "TemporalWorkflowHighFailureRate" {
				foundWarning = true
				if rule.Labels["severity"] != "warning" {
					t.Error("HighFailureRate should have warning severity")
				}
			}
			if rule.Alert == "TemporalWorkflowCriticalFailureRate" {
				foundCritical = true
				if rule.Labels["severity"] != "critical" {
					t.Error("CriticalFailureRate should have critical severity")
				}
			}
		}
		if !foundWarning {
			t.Error("Missing TemporalWorkflowHighFailureRate alert")
		}
		if !foundCritical {
			t.Error("Missing TemporalWorkflowCriticalFailureRate alert")
		}
	})

	t.Run("has activity failure rate alert", func(t *testing.T) {
		found := false
		for _, rule := range rules.Rules {
			if rule.Alert == "TemporalActivityHighFailureRate" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Missing TemporalActivityHighFailureRate alert")
		}
	})

	t.Run("has slow duration alerts", func(t *testing.T) {
		foundWorkflow := false
		foundActivity := false
		for _, rule := range rules.Rules {
			if rule.Alert == "TemporalWorkflowSlowDuration" {
				foundWorkflow = true
			}
			if rule.Alert == "TemporalActivitySlowDuration" {
				foundActivity = true
			}
		}
		if !foundWorkflow {
			t.Error("Missing TemporalWorkflowSlowDuration alert")
		}
		if !foundActivity {
			t.Error("Missing TemporalActivitySlowDuration alert")
		}
	})

	t.Run("has task queue backlog alert", func(t *testing.T) {
		found := false
		for _, rule := range rules.Rules {
			if rule.Alert == "TemporalTaskQueueBacklog" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Missing TemporalTaskQueueBacklog alert")
		}
	})

	t.Run("alerts have service label", func(t *testing.T) {
		for _, rule := range rules.Rules {
			if rule.Labels["service"] != "myservice" {
				t.Errorf("Alert %s missing service label", rule.Alert)
			}
		}
	})

	t.Run("alerts have annotations", func(t *testing.T) {
		for _, rule := range rules.Rules {
			if rule.Annotations["summary"] == "" {
				t.Errorf("Alert %s missing summary annotation", rule.Alert)
			}
			if rule.Annotations["description"] == "" {
				t.Errorf("Alert %s missing description annotation", rule.Alert)
			}
		}
	})
}

func TestNewTemporalSLOs(t *testing.T) {
	slos := NewTemporalSLOs("myservice")

	t.Run("has SLOs defined", func(t *testing.T) {
		if len(slos) == 0 {
			t.Error("No SLOs defined")
		}
	})

	t.Run("has workflow success rate SLO", func(t *testing.T) {
		found := false
		for _, slo := range slos {
			if slo.Name == "Workflow Success Rate" {
				found = true
				if slo.Target < 99.0 {
					t.Error("Workflow success rate target should be >= 99%")
				}
				if slo.Indicator != "availability" {
					t.Error("Workflow success rate should be availability indicator")
				}
				break
			}
		}
		if !found {
			t.Error("Missing Workflow Success Rate SLO")
		}
	})

	t.Run("has workflow duration SLO", func(t *testing.T) {
		found := false
		for _, slo := range slos {
			if slo.Name == "Workflow Duration p99" {
				found = true
				if slo.Indicator != "latency" {
					t.Error("Workflow duration should be latency indicator")
				}
				break
			}
		}
		if !found {
			t.Error("Missing Workflow Duration p99 SLO")
		}
	})

	t.Run("has activity SLOs", func(t *testing.T) {
		foundSuccessRate := false
		foundDuration := false
		for _, slo := range slos {
			if slo.Name == "Activity Success Rate" {
				foundSuccessRate = true
			}
			if slo.Name == "Activity Duration p99" {
				foundDuration = true
			}
		}
		if !foundSuccessRate {
			t.Error("Missing Activity Success Rate SLO")
		}
		if !foundDuration {
			t.Error("Missing Activity Duration p99 SLO")
		}
	})

	t.Run("SLOs have valid windows", func(t *testing.T) {
		for _, slo := range slos {
			if slo.Window <= 0 {
				t.Errorf("SLO %s has invalid window: %v", slo.Name, slo.Window)
			}
			if slo.Window < 24*time.Hour {
				t.Errorf("SLO %s window should be at least 1 day", slo.Name)
			}
		}
	})

	t.Run("SLOs have metrics with service name", func(t *testing.T) {
		for _, slo := range slos {
			if !strings.Contains(slo.Metric, "myservice") {
				t.Errorf("SLO %s metric should contain service name", slo.Name)
			}
		}
	})
}

func TestDashboardJSON(t *testing.T) {
	t.Run("returns valid JSON", func(t *testing.T) {
		dashboard := NewTemporalDashboard("default", "worker")
		jsonStr, err := DashboardJSON(dashboard)
		if err != nil {
			t.Fatalf("DashboardJSON error: %v", err)
		}

		if jsonStr == "" {
			t.Error("DashboardJSON returned empty string")
		}

		// Verify it's valid JSON
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}
	})

	t.Run("contains expected fields", func(t *testing.T) {
		dashboard := NewTemporalDashboard("default", "worker")
		jsonStr, _ := DashboardJSON(dashboard)

		if !strings.Contains(jsonStr, "temporal-default-worker") {
			t.Error("JSON missing UID")
		}
		if !strings.Contains(jsonStr, "panels") {
			t.Error("JSON missing panels")
		}
	})

	t.Run("nil dashboard", func(t *testing.T) {
		jsonStr, err := DashboardJSON(nil)
		if err != nil {
			t.Fatalf("DashboardJSON(nil) error: %v", err)
		}
		// Should return "null" for nil
		if jsonStr != "null" {
			t.Errorf("Expected 'null', got %q", jsonStr)
		}
	})

	t.Run("empty dashboard", func(t *testing.T) {
		dashboard := &GrafanaDashboard{}
		jsonStr, err := DashboardJSON(dashboard)
		if err != nil {
			t.Fatalf("DashboardJSON error: %v", err)
		}
		if jsonStr == "" {
			t.Error("DashboardJSON returned empty string for empty dashboard")
		}
	})
}

func TestFloatPtr(t *testing.T) {
	ptr := floatPtr(42.5)
	if ptr == nil {
		t.Fatal("floatPtr returned nil")
	}
	if *ptr != 42.5 {
		t.Errorf("*ptr = %f, want 42.5", *ptr)
	}
}

func TestGrafanaStructs(t *testing.T) {
	t.Run("GrafanaPanel", func(t *testing.T) {
		panel := GrafanaPanel{
			ID:          1,
			Type:        "timeseries",
			Title:       "Test Panel",
			Description: "A test panel",
			GridPos:     GrafanaGridPos{H: 8, W: 12, X: 0, Y: 0},
			Targets: []GrafanaTarget{
				{RefID: "A", Expr: "rate(metric[5m])", LegendFormat: "{{label}}"},
			},
		}

		// Verify JSON serialization
		data, err := json.Marshal(panel)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !strings.Contains(string(data), "timeseries") {
			t.Error("Serialized JSON missing type")
		}
	})

	t.Run("GrafanaVariable", func(t *testing.T) {
		variable := GrafanaVariable{
			Name:  "namespace",
			Label: "Namespace",
			Type:  "query",
			Query: "label_values(namespace)",
			Current: GrafanaVariableCurrent{
				Text:  "default",
				Value: "default",
			},
		}

		data, err := json.Marshal(variable)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !strings.Contains(string(data), "namespace") {
			t.Error("Serialized JSON missing name")
		}
	})

	t.Run("GrafanaThresholds", func(t *testing.T) {
		thresholds := GrafanaThresholds{
			Mode: "absolute",
			Steps: []GrafanaThreshold{
				{Value: nil, Color: "red"},
				{Value: floatPtr(90), Color: "yellow"},
				{Value: floatPtr(99), Color: "green"},
			},
		}

		data, err := json.Marshal(thresholds)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !strings.Contains(string(data), "absolute") {
			t.Error("Serialized JSON missing mode")
		}
	})

	t.Run("GrafanaAnnotation", func(t *testing.T) {
		annotation := GrafanaAnnotation{
			Name:       "deployments",
			Enable:     true,
			Datasource: "prometheus",
			Expr:       "changes(deployment_timestamp[5m]) > 0",
		}

		data, err := json.Marshal(annotation)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}
		if !strings.Contains(string(data), "deployments") {
			t.Error("Serialized JSON missing name")
		}
	})
}

func TestSLODefinition(t *testing.T) {
	slo := SLODefinition{
		Name:        "Test SLO",
		Description: "A test SLO",
		Target:      99.9,
		Window:      30 * 24 * time.Hour,
		Metric:      "sum(rate(requests_total[5m]))",
		Indicator:   "availability",
	}

	data, err := json.Marshal(slo)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if !strings.Contains(string(data), "Test SLO") {
		t.Error("Serialized JSON missing name")
	}
	if !strings.Contains(string(data), "99.9") {
		t.Error("Serialized JSON missing target")
	}
}

func TestAlertRule(t *testing.T) {
	rule := AlertRule{
		Alert: "TestAlert",
		Expr:  "sum(rate(errors[5m])) > 0.05",
		For:   "5m",
		Labels: map[string]string{
			"severity": "warning",
		},
		Annotations: map[string]string{
			"summary":     "Test alert summary",
			"description": "Test alert description",
		},
	}

	if rule.Alert != "TestAlert" {
		t.Error("Alert name mismatch")
	}
	if rule.For != "5m" {
		t.Error("For duration mismatch")
	}
	if rule.Labels["severity"] != "warning" {
		t.Error("Labels mismatch")
	}
}
