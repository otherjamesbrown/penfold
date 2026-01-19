package validation

import (
	"context"
	"testing"
	"time"
)

func TestCheckCategories(t *testing.T) {
	categories := []CheckCategory{
		CategoryService,
		CategoryEndpoint,
		CategoryData,
		CategoryPerformance,
	}

	expected := []string{"service", "endpoint", "data", "performance"}

	for i, cat := range categories {
		if string(cat) != expected[i] {
			t.Errorf("Category %d: got %s, want %s", i, cat, expected[i])
		}
	}
}

func TestNewChecklist(t *testing.T) {
	cl := NewChecklist()

	if cl == nil {
		t.Fatal("NewChecklist returned nil")
	}

	// Verify default checks are registered
	checks := cl.GetAllChecks()
	if len(checks) == 0 {
		t.Error("NewChecklist should register default checks")
	}

	// Verify all categories have checks
	for _, cat := range []CheckCategory{CategoryService, CategoryEndpoint, CategoryData, CategoryPerformance} {
		catChecks := cl.GetChecksByCategory(cat)
		if len(catChecks) == 0 {
			t.Errorf("No checks registered for category %s", cat)
		}
	}
}

func TestChecklist_AddCheck(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	check := &MigrationCheck{
		ID:          "test_check",
		Name:        "Test Check",
		Description: "A test check",
		Category:    CategoryService,
		Critical:    true,
	}

	cl.AddCheck(check)

	// Verify check was added
	got, ok := cl.GetCheck("test_check")
	if !ok {
		t.Fatal("Check not found after AddCheck")
	}
	if got.ID != check.ID {
		t.Errorf("Got check ID %s, want %s", got.ID, check.ID)
	}
	if got.Name != check.Name {
		t.Errorf("Got check Name %s, want %s", got.Name, check.Name)
	}
	if got.Critical != check.Critical {
		t.Errorf("Got check Critical %v, want %v", got.Critical, check.Critical)
	}
}

func TestChecklist_RemoveCheck(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Add some checks
	cl.AddCheck(&MigrationCheck{ID: "check1", Name: "Check 1", Category: CategoryService})
	cl.AddCheck(&MigrationCheck{ID: "check2", Name: "Check 2", Category: CategoryService})
	cl.AddCheck(&MigrationCheck{ID: "check3", Name: "Check 3", Category: CategoryService})

	// Remove middle check
	cl.RemoveCheck("check2")

	// Verify check2 is removed
	_, ok := cl.GetCheck("check2")
	if ok {
		t.Error("Check2 should have been removed")
	}

	// Verify other checks still exist
	if _, ok := cl.GetCheck("check1"); !ok {
		t.Error("Check1 should still exist")
	}
	if _, ok := cl.GetCheck("check3"); !ok {
		t.Error("Check3 should still exist")
	}

	// Verify order is updated
	checks := cl.GetAllChecks()
	if len(checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(checks))
	}
}

func TestChecklist_GetCheck(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	t.Run("existing check", func(t *testing.T) {
		cl.AddCheck(&MigrationCheck{ID: "existing", Name: "Existing"})
		check, ok := cl.GetCheck("existing")
		if !ok {
			t.Error("Should find existing check")
		}
		if check.ID != "existing" {
			t.Errorf("Got ID %s, want existing", check.ID)
		}
	})

	t.Run("non-existing check", func(t *testing.T) {
		_, ok := cl.GetCheck("non_existing")
		if ok {
			t.Error("Should not find non-existing check")
		}
	})
}

func TestChecklist_GetAllChecks(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Empty checklist
	checks := cl.GetAllChecks()
	if len(checks) != 0 {
		t.Errorf("Empty checklist should return empty slice, got %d", len(checks))
	}

	// Add checks
	cl.AddCheck(&MigrationCheck{ID: "a", Name: "A"})
	cl.AddCheck(&MigrationCheck{ID: "b", Name: "B"})
	cl.AddCheck(&MigrationCheck{ID: "c", Name: "C"})

	checks = cl.GetAllChecks()
	if len(checks) != 3 {
		t.Errorf("Expected 3 checks, got %d", len(checks))
	}

	// Verify order is preserved
	if checks[0].ID != "a" || checks[1].ID != "b" || checks[2].ID != "c" {
		t.Error("Order should be preserved")
	}
}

func TestChecklist_GetChecksByCategory(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	cl.AddCheck(&MigrationCheck{ID: "svc1", Category: CategoryService})
	cl.AddCheck(&MigrationCheck{ID: "svc2", Category: CategoryService})
	cl.AddCheck(&MigrationCheck{ID: "ep1", Category: CategoryEndpoint})
	cl.AddCheck(&MigrationCheck{ID: "data1", Category: CategoryData})

	t.Run("service category", func(t *testing.T) {
		checks := cl.GetChecksByCategory(CategoryService)
		if len(checks) != 2 {
			t.Errorf("Expected 2 service checks, got %d", len(checks))
		}
	})

	t.Run("endpoint category", func(t *testing.T) {
		checks := cl.GetChecksByCategory(CategoryEndpoint)
		if len(checks) != 1 {
			t.Errorf("Expected 1 endpoint check, got %d", len(checks))
		}
	})

	t.Run("performance category (empty)", func(t *testing.T) {
		checks := cl.GetChecksByCategory(CategoryPerformance)
		if len(checks) != 0 {
			t.Errorf("Expected 0 performance checks, got %d", len(checks))
		}
	})
}

func TestChecklist_GetCriticalChecks(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	cl.AddCheck(&MigrationCheck{ID: "critical1", Critical: true})
	cl.AddCheck(&MigrationCheck{ID: "noncritical1", Critical: false})
	cl.AddCheck(&MigrationCheck{ID: "critical2", Critical: true})
	cl.AddCheck(&MigrationCheck{ID: "noncritical2", Critical: false})

	critical := cl.GetCriticalChecks()
	if len(critical) != 2 {
		t.Errorf("Expected 2 critical checks, got %d", len(critical))
	}

	for _, c := range critical {
		if !c.Critical {
			t.Errorf("Check %s should be critical", c.ID)
		}
	}
}

func TestChecklist_Run(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Add a check with custom function that passes
	cl.AddCheck(&MigrationCheck{
		ID:       "pass_check",
		Name:     "Pass Check",
		Category: CategoryService,
		Critical: false,
		CheckFunc: func(ctx context.Context) ValidationResult {
			return ValidationResult{
				Status:  StatusPass,
				Message: "Check passed",
			}
		},
	})

	// Add a check with custom function that fails
	cl.AddCheck(&MigrationCheck{
		ID:       "fail_check",
		Name:     "Fail Check",
		Category: CategoryService,
		Critical: true,
		CheckFunc: func(ctx context.Context) ValidationResult {
			return ValidationResult{
				Status: StatusFail,
				Error:  "Check failed",
			}
		},
	})

	// Add a check with custom function that warns
	cl.AddCheck(&MigrationCheck{
		ID:       "warn_check",
		Name:     "Warn Check",
		Category: CategoryEndpoint,
		Critical: false,
		CheckFunc: func(ctx context.Context) ValidationResult {
			return ValidationResult{
				Status:  StatusWarn,
				Message: "Warning",
			}
		},
	})

	// Run checks
	ctx := context.Background()
	validator := NewValidator()
	status, results := cl.Run(ctx, validator)

	// Verify status
	if status.TotalChecks != 3 {
		t.Errorf("Expected 3 total checks, got %d", status.TotalChecks)
	}
	if status.PassedChecks != 1 {
		t.Errorf("Expected 1 passed check, got %d", status.PassedChecks)
	}
	if status.FailedChecks != 1 {
		t.Errorf("Expected 1 failed check, got %d", status.FailedChecks)
	}
	if status.WarningChecks != 1 {
		t.Errorf("Expected 1 warning check, got %d", status.WarningChecks)
	}
	if status.CriticalFailed != 1 {
		t.Errorf("Expected 1 critical failed, got %d", status.CriticalFailed)
	}
	if status.CriticalPassed != 0 {
		t.Errorf("Expected 0 critical passed, got %d", status.CriticalPassed)
	}

	// Overall status should be fail due to critical failure
	if status.OverallStatus != StatusFail {
		t.Errorf("Expected overall status FAIL, got %s", status.OverallStatus)
	}

	// Verify results map
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// Verify category stats
	if status.ByCategory[CategoryService].Total != 2 {
		t.Errorf("Expected 2 service checks, got %d", status.ByCategory[CategoryService].Total)
	}
}

func TestChecklist_Run_DefaultChecks(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Add checks without custom functions to test default behavior
	cl.AddCheck(&MigrationCheck{
		ID:       "service_check",
		Name:     "Service Check",
		Category: CategoryService,
	})
	cl.AddCheck(&MigrationCheck{
		ID:       "endpoint_check",
		Name:     "Endpoint Check",
		Category: CategoryEndpoint,
	})
	cl.AddCheck(&MigrationCheck{
		ID:       "data_check",
		Name:     "Data Check",
		Category: CategoryData,
	})
	cl.AddCheck(&MigrationCheck{
		ID:       "perf_check",
		Name:     "Perf Check",
		Category: CategoryPerformance,
		TargetMs: 100,
	})

	ctx := context.Background()
	validator := NewValidator()
	status, results := cl.Run(ctx, validator)

	// All should be warnings since they use default implementation
	if status.WarningChecks != 4 {
		t.Errorf("Expected 4 warning checks, got %d", status.WarningChecks)
	}

	// Check that perf check has target_ms in details
	perfResult := results["perf_check"]
	if perfResult.Details == nil {
		t.Error("Performance check should have details")
	} else {
		details, ok := perfResult.Details.(map[string]interface{})
		if !ok {
			t.Error("Details should be a map")
		} else if details["target_ms"] != int64(100) {
			t.Errorf("Expected target_ms 100, got %v", details["target_ms"])
		}
	}
}

func TestChecklist_Run_OverallStatusLogic(t *testing.T) {
	tests := []struct {
		name           string
		checks         []*MigrationCheck
		expectedStatus CheckStatus
	}{
		{
			name: "all pass",
			checks: []*MigrationCheck{
				{ID: "1", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusPass} }},
				{ID: "2", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusPass} }},
			},
			expectedStatus: StatusPass,
		},
		{
			name: "critical fail",
			checks: []*MigrationCheck{
				{ID: "1", Category: CategoryService, Critical: true, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusFail} }},
				{ID: "2", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusPass} }},
			},
			expectedStatus: StatusFail,
		},
		{
			name: "non-critical fail",
			checks: []*MigrationCheck{
				{ID: "1", Category: CategoryService, Critical: false, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusFail} }},
				{ID: "2", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusPass} }},
			},
			expectedStatus: StatusWarn,
		},
		{
			name: "warning only",
			checks: []*MigrationCheck{
				{ID: "1", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusWarn} }},
				{ID: "2", Category: CategoryService, CheckFunc: func(ctx context.Context) ValidationResult { return ValidationResult{Status: StatusPass} }},
			},
			expectedStatus: StatusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := &Checklist{
				checks: make(map[string]*MigrationCheck),
				order:  make([]string, 0),
			}
			for _, check := range tt.checks {
				cl.AddCheck(check)
			}

			ctx := context.Background()
			validator := NewValidator()
			status, _ := cl.Run(ctx, validator)

			if status.OverallStatus != tt.expectedStatus {
				t.Errorf("Expected overall status %s, got %s", tt.expectedStatus, status.OverallStatus)
			}
		})
	}
}

func TestChecklist_ConcurrentAccess(t *testing.T) {
	cl := &Checklist{
		checks: make(map[string]*MigrationCheck),
		order:  make([]string, 0),
	}

	// Add some initial checks
	for i := 0; i < 10; i++ {
		cl.AddCheck(&MigrationCheck{ID: "init_" + string(rune('0'+i))})
	}

	// Concurrent reads and writes
	done := make(chan bool)

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = cl.GetAllChecks()
			_ = cl.GetCriticalChecks()
			_ = cl.GetChecksByCategory(CategoryService)
			_, _ = cl.GetCheck("init_0")
		}
		done <- true
	}()

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cl.AddCheck(&MigrationCheck{ID: "concurrent_" + string(rune('0'+i%10))})
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done
}

func TestChecklistStatus(t *testing.T) {
	status := &ChecklistStatus{
		TotalChecks:    10,
		PassedChecks:   7,
		FailedChecks:   1,
		WarningChecks:  1,
		SkippedChecks:  1,
		CriticalPassed: 3,
		CriticalFailed: 0,
		ByCategory: map[CheckCategory]*CategoryStatus{
			CategoryService:  {Total: 3, Passed: 2, Failed: 1},
			CategoryEndpoint: {Total: 4, Passed: 3, Warning: 1},
		},
		OverallStatus: StatusWarn,
		CompletedAt:   time.Now(),
	}

	// Verify the struct is populated correctly
	if status.TotalChecks != 10 {
		t.Errorf("Expected total 10, got %d", status.TotalChecks)
	}
	if status.ByCategory[CategoryService].Total != 3 {
		t.Errorf("Expected service total 3, got %d", status.ByCategory[CategoryService].Total)
	}
}

func TestServiceChecks(t *testing.T) {
	checks := ServiceChecks()
	if len(checks) == 0 {
		t.Error("ServiceChecks should return non-empty slice")
	}
	// Verify gateway is mentioned
	found := false
	for _, c := range checks {
		if c == "Gateway service running and accepting connections" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ServiceChecks should include gateway")
	}
}

func TestEndpointChecks(t *testing.T) {
	checks := EndpointChecks()
	if len(checks) == 0 {
		t.Error("EndpointChecks should return non-empty slice")
	}
}

func TestDataChecks(t *testing.T) {
	checks := DataChecks()
	if len(checks) == 0 {
		t.Error("DataChecks should return non-empty slice")
	}
	// Verify database connection is mentioned
	found := false
	for _, c := range checks {
		if c == "Database connection successful" {
			found = true
			break
		}
	}
	if !found {
		t.Error("DataChecks should include database connection")
	}
}

func TestPerformanceChecks(t *testing.T) {
	checks := PerformanceChecks()
	if len(checks) == 0 {
		t.Error("PerformanceChecks should return non-empty slice")
	}
}

func TestDefaultChecklist_ServiceChecks(t *testing.T) {
	cl := NewChecklist()

	// Verify service checks are registered
	serviceChecks := cl.GetChecksByCategory(CategoryService)

	expectedServices := []string{"gateway", "search", "gmail", "content", "ai", "review", "relationship"}
	for _, svc := range expectedServices {
		found := false
		for _, check := range serviceChecks {
			if check.ID == "svc_"+svc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected service check for %s", svc)
		}
	}
}

func TestDefaultChecklist_CriticalServices(t *testing.T) {
	cl := NewChecklist()

	// Verify critical services are marked correctly
	criticalServices := map[string]bool{
		"svc_gateway": true,
		"svc_search":  true,
		"svc_content": true,
		"svc_ai":      true,
		"svc_gmail":   false,
		"svc_review":  false,
	}

	for id, expectedCritical := range criticalServices {
		check, ok := cl.GetCheck(id)
		if !ok {
			t.Errorf("Check %s not found", id)
			continue
		}
		if check.Critical != expectedCritical {
			t.Errorf("Check %s: expected critical=%v, got %v", id, expectedCritical, check.Critical)
		}
	}
}
