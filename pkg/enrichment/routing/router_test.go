package routing

import (
	"testing"
)

// buildSeededRoutes returns the in-memory equivalent of the seeded routing rules
// from migrations/068_pipeline_routing.sql + 093_lightweight_pipelines.sql.
func buildSeededRoutes() []Route {
	return []Route{
		{ID: 1, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "HUMAN", Pipeline: "standard", Active: true},
		{ID: 2, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "NOTIFICATION", Pipeline: "notification", Active: true},
		{ID: 3, TenantID: "tenant-a", ContentType: "CALENDAR", ContentSubtype: "INVITE", Pipeline: "attendees_only", Active: true},
		{ID: 4, TenantID: "tenant-a", ContentType: "MEETING", ContentSubtype: "TRANSCRIPT", Pipeline: "transcript", Active: true},
		{ID: 5, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "NEWSLETTER", Pipeline: "newsletter", Active: true},
		{ID: 6, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "AUTO_REPLY", Pipeline: "auto_reply", Active: true},
		{ID: 7, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "CALENDAR_RESPONSE", Pipeline: "attendees_only", Active: true},
		{ID: 8, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "CALENDAR_UPDATE", Pipeline: "attendees_only", Active: true},
		{ID: 9, TenantID: "tenant-a", ContentType: "CALENDAR", ContentSubtype: "CANCELLATION", Pipeline: "attendees_only", Active: true},
	}
}

// TestPipelineRouting_StandardEmail verifies EMAIL/HUMAN → "standard" pipeline.
func TestPipelineRouting_StandardEmail(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "HUMAN")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "standard" {
		t.Errorf("expected pipeline=standard, got %q", pipelines[0])
	}
}

// TestPipelineRouting_AutoReply verifies EMAIL/AUTO_REPLY → "auto_reply" pipeline.
func TestPipelineRouting_AutoReply(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "AUTO_REPLY")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "auto_reply" {
		t.Errorf("expected pipeline=auto_reply, got %q", pipelines[0])
	}
}

// TestPipelineRouting_CalendarInvite verifies CALENDAR/INVITE → "attendees_only".
func TestPipelineRouting_CalendarInvite(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("CALENDAR", "INVITE")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "attendees_only" {
		t.Errorf("expected pipeline=attendees_only, got %q", pipelines[0])
	}
}

// TestPipelineRouting_MultiPipeline verifies that items can route to multiple pipelines.
func TestPipelineRouting_MultiPipeline(t *testing.T) {
	routes := buildSeededRoutes()
	// Add a second route for EMAIL/HUMAN → "compliance_scan"
	routes = append(routes, Route{
		ID: 10, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "HUMAN",
		Pipeline: "compliance_scan", Active: true,
	})
	router := NewRouter(routes)

	pipelines := router.Resolve("EMAIL", "HUMAN")

	if len(pipelines) != 2 {
		t.Fatalf("expected 2 pipelines, got %d: %v", len(pipelines), pipelines)
	}
	// Should have both "standard" and "compliance_scan"
	found := map[string]bool{}
	for _, p := range pipelines {
		found[p] = true
	}
	if !found["standard"] {
		t.Error("expected standard pipeline in results")
	}
	if !found["compliance_scan"] {
		t.Error("expected compliance_scan pipeline in results")
	}
}

// TestPipelineRouting_NoRowSkips verifies items with no routing row are skipped.
func TestPipelineRouting_NoRowSkips(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	tests := []struct {
		name        string
		contentType string
		subtype     string
	}{
		{"calendar update", "CALENDAR", "UPDATE"},
		{"calendar response", "CALENDAR", "RESPONSE"},
		{"unknown type", "UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelines := router.Resolve(tt.contentType, tt.subtype)
			if len(pipelines) != 0 {
				t.Errorf("expected no pipelines for %s/%s, got %v", tt.contentType, tt.subtype, pipelines)
			}
		})
	}
}

// TestPipelineRouting_NotificationPipeline verifies EMAIL/NOTIFICATION → "notification" (not "standard").
func TestPipelineRouting_NotificationPipeline(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "NOTIFICATION")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "notification" {
		t.Errorf("expected pipeline=notification, got %q", pipelines[0])
	}
}

// TestPipelineRouting_NewsletterPipeline verifies EMAIL/NEWSLETTER → "newsletter".
func TestPipelineRouting_NewsletterPipeline(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "NEWSLETTER")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "newsletter" {
		t.Errorf("expected pipeline=newsletter, got %q", pipelines[0])
	}
}

// TestPipelineRouting_CalendarResponseAttendeesOnly verifies EMAIL/CALENDAR_RESPONSE → "attendees_only".
func TestPipelineRouting_CalendarResponseAttendeesOnly(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "CALENDAR_RESPONSE")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "attendees_only" {
		t.Errorf("expected pipeline=attendees_only, got %q", pipelines[0])
	}
}

// TestPipelineRouting_HumanRegression verifies EMAIL/HUMAN still routes to "standard".
func TestPipelineRouting_HumanRegression(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("EMAIL", "HUMAN")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "standard" {
		t.Errorf("expected pipeline=standard, got %q", pipelines[0])
	}
}

// TestPipelineRouting_PerTenant verifies that routing is per-tenant.
func TestPipelineRouting_PerTenant(t *testing.T) {
	// Tenant A: EMAIL/NEWSLETTER → "newsletter_digest"
	routesA := []Route{
		{ID: 1, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "NEWSLETTER", Pipeline: "newsletter_digest", Active: true},
	}
	// Tenant B: no routing for EMAIL/NEWSLETTER
	routesB := []Route{
		{ID: 2, TenantID: "tenant-b", ContentType: "EMAIL", ContentSubtype: "HUMAN", Pipeline: "standard", Active: true},
	}

	routerA := NewRouter(routesA)
	routerB := NewRouter(routesB)

	pipelinesA := routerA.Resolve("EMAIL", "NEWSLETTER")
	pipelinesB := routerB.Resolve("EMAIL", "NEWSLETTER")

	if len(pipelinesA) != 1 || pipelinesA[0] != "newsletter_digest" {
		t.Errorf("tenant A: expected newsletter_digest, got %v", pipelinesA)
	}
	if len(pipelinesB) != 0 {
		t.Errorf("tenant B: expected no pipelines for newsletter, got %v", pipelinesB)
	}
}

// TestPipelineRouting_NewRouteNoCodeChange verifies that adding a new route requires
// no code change — just a DB insert.
func TestPipelineRouting_NewRouteNoCodeChange(t *testing.T) {
	routes := buildSeededRoutes()
	// Simulate inserting a new route for EMAIL/BROADCAST → "standard"
	routes = append(routes, Route{
		ID: 20, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "BROADCAST",
		Pipeline: "standard", Active: true,
	})
	router := NewRouter(routes)

	pipelines := router.Resolve("EMAIL", "BROADCAST")

	if len(pipelines) != 1 || pipelines[0] != "standard" {
		t.Errorf("expected standard pipeline after inserting new route, got %v", pipelines)
	}
}

// TestPipelineRouting_InactiveRouteIgnored verifies inactive routes are not matched.
func TestPipelineRouting_InactiveRouteIgnored(t *testing.T) {
	routes := []Route{
		{ID: 1, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "HUMAN", Pipeline: "standard", Active: false},
	}
	router := NewRouter(routes)

	pipelines := router.Resolve("EMAIL", "HUMAN")

	if len(pipelines) != 0 {
		t.Errorf("expected no pipelines (route is inactive), got %v", pipelines)
	}
}

// TestPipelineRouting_WildcardSubtype verifies NULL content_subtype matches any subtype.
func TestPipelineRouting_WildcardSubtype(t *testing.T) {
	routes := []Route{
		{ID: 1, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "", Pipeline: "standard", Active: true},
	}
	router := NewRouter(routes)

	// Should match any EMAIL subtype
	for _, subtype := range []string{"HUMAN", "NOTIFICATION", "AUTO_REPLY", "NEWSLETTER"} {
		pipelines := router.Resolve("EMAIL", subtype)
		if len(pipelines) != 1 || pipelines[0] != "standard" {
			t.Errorf("expected standard for EMAIL/%s with wildcard subtype, got %v", subtype, pipelines)
		}
	}
}

// TestPipelineRouting_CaseInsensitive verifies routing is case-insensitive.
func TestPipelineRouting_CaseInsensitive(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	// Lowercase input should still match uppercase routes
	pipelines := router.Resolve("email", "human")
	if len(pipelines) != 1 || pipelines[0] != "standard" {
		t.Errorf("expected standard for lowercase email/human, got %v", pipelines)
	}
}

// TestPipelineRouting_NoDuplicates verifies same pipeline isn't returned twice.
func TestPipelineRouting_NoDuplicates(t *testing.T) {
	routes := []Route{
		{ID: 1, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "HUMAN", Pipeline: "standard", Active: true},
		{ID: 2, TenantID: "tenant-a", ContentType: "EMAIL", ContentSubtype: "", Pipeline: "standard", Active: true},
	}
	router := NewRouter(routes)

	// Both routes match EMAIL/HUMAN but both say "standard" — should not duplicate
	pipelines := router.Resolve("EMAIL", "HUMAN")
	if len(pipelines) != 1 {
		t.Errorf("expected 1 pipeline (no dupes), got %d: %v", len(pipelines), pipelines)
	}
}

// TestPipelineRouting_EmptyRoutes verifies empty route set → nothing matches.
func TestPipelineRouting_EmptyRoutes(t *testing.T) {
	router := NewRouter([]Route{})

	pipelines := router.Resolve("EMAIL", "HUMAN")
	if len(pipelines) != 0 {
		t.Errorf("expected no pipelines with empty routes, got %v", pipelines)
	}
}

// TestPipelineRouting_NewsletterVariant verifies that NEWSLETTER_INTERNAL routes to
// newsletter_internal while the generic NEWSLETTER route is unaffected.
func TestPipelineRouting_NewsletterVariant(t *testing.T) {
	routes := buildSeededRoutes()
	// Add variant route (in addition to existing EMAIL/NEWSLETTER → newsletter).
	routes = append(routes, Route{
		ID: 30, TenantID: "tenant-a", ContentType: "EMAIL",
		ContentSubtype: "NEWSLETTER_INTERNAL", Pipeline: "newsletter_internal", Active: true,
	})
	router := NewRouter(routes)

	// NEWSLETTER_INTERNAL should route to newsletter_internal.
	pipelines := router.Resolve("EMAIL", "NEWSLETTER_INTERNAL")
	if len(pipelines) != 1 || pipelines[0] != "newsletter_internal" {
		t.Errorf("expected [newsletter_internal], got %v", pipelines)
	}

	// Generic NEWSLETTER should still route to newsletter (unchanged).
	pipelines = router.Resolve("EMAIL", "NEWSLETTER")
	if len(pipelines) != 1 || pipelines[0] != "newsletter" {
		t.Errorf("expected [newsletter], got %v", pipelines)
	}
}

// TestHasStage verifies pipeline stage lookups.
func TestHasStage(t *testing.T) {
	tests := []struct {
		pipeline string
		stage    string
		want     bool
	}{
		{"standard", "triage", true},
		{"standard", "extract", true},
		{"standard", "embed", true},
		{"standard", "summarize", true},
		{"standard", "analyze", false},
		{"attendees_only", "extract", true},
		{"attendees_only", "triage", false},
		{"attendees_only", "embed", false},
		{"nonexistent", "extract", false},
	}

	for _, tt := range tests {
		t.Run(tt.pipeline+"/"+tt.stage, func(t *testing.T) {
			got := HasStage(tt.pipeline, tt.stage)
			if got != tt.want {
				t.Errorf("HasStage(%q, %q) = %v, want %v", tt.pipeline, tt.stage, got, tt.want)
			}
		})
	}
}

// TestMeetingTranscript verifies MEETING/TRANSCRIPT → "transcript" pipeline.
func TestMeetingTranscript(t *testing.T) {
	router := NewRouter(buildSeededRoutes())

	pipelines := router.Resolve("MEETING", "TRANSCRIPT")

	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d: %v", len(pipelines), pipelines)
	}
	if pipelines[0] != "transcript" {
		t.Errorf("expected pipeline=transcript, got %q", pipelines[0])
	}
}
