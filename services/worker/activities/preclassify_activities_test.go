package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/routing"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockRoutingRepo is a mock for RoutingRepository.
type mockRoutingRepo struct {
	loadRoutesFn func(ctx context.Context, tenantID string) ([]routing.Route, error)
}

func (m *mockRoutingRepo) LoadRoutes(ctx context.Context, tenantID string) ([]routing.Route, error) {
	if m.loadRoutesFn != nil {
		return m.loadRoutesFn(ctx, tenantID)
	}
	return nil, nil
}

func TestPreClassify_NotificationSourcesMatch(t *testing.T) {
	// Verify all 7 existing notification sources produce correct classification.
	sources := []struct {
		name        string
		senderEmail string
		ruleName    string
		wantSubtype string
	}{
		{"jira", "gsd-jira@akamai.com", "jira", "NOTIFICATION"},
		{"aha", "aha-notifications@mailer.aha.io", "aha", "NOTIFICATION"},
		{"google_docs", "noreply@google.com", "google_docs", "NOTIFICATION"},
		{"google_security", "no-reply@accounts.google.com", "google_security", "NOTIFICATION"},
		{"webex", "webex@webex.com", "webex", "NOTIFICATION"},
		{"smartsheet", "notification@smartsheet.com", "smartsheet", "NOTIFICATION"},
		{"it_notification", "oracle-hcm-prod@akamai.com", "it_notification", "NOTIFICATION"},
	}

	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			rules := []classification.ClassificationRule{
				notificationRule(src.ruleName, src.senderEmail),
			}

			repo := &mockClassificationRepo{
				loadRulesFn: func(_ context.Context, _ string) ([]classification.ClassificationRule, error) {
					return rules, nil
				},
			}

			act := NewPreClassifyActivities(logging.NewNopLogger(), repo)
			out, err := act.PreClassify(context.Background(), workflows.PreClassifyInput{
				TenantID:    "test-tenant",
				ContentType: "email",
				SenderEmail: src.senderEmail,
				Subject:     "Test subject",
			})

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !out.Matched {
				t.Errorf("expected rule match for %s, got no match", src.name)
			}
			if out.ContentSubtype != src.wantSubtype {
				t.Errorf("subtype: got %q, want %q", out.ContentSubtype, src.wantSubtype)
			}
			if out.RuleName != src.ruleName {
				t.Errorf("rule name: got %q, want %q", out.RuleName, src.ruleName)
			}
		})
	}
}

func TestPreClassify_NoRuleMatch(t *testing.T) {
	repo := &mockClassificationRepo{
		loadRulesFn: func(_ context.Context, _ string) ([]classification.ClassificationRule, error) {
			return []classification.ClassificationRule{
				notificationRule("jira", "gsd-jira@akamai.com"),
			}, nil
		},
	}

	act := NewPreClassifyActivities(logging.NewNopLogger(), repo)
	out, err := act.PreClassify(context.Background(), workflows.PreClassifyInput{
		TenantID:    "test-tenant",
		ContentType: "email",
		SenderEmail: "human@example.com",
		Subject:     "Hello",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Matched {
		t.Errorf("expected no match for human sender, got match with rule %q", out.RuleName)
	}
	if out.ContentSubtype != "HUMAN" {
		t.Errorf("expected HUMAN subtype for unmatched, got %q", out.ContentSubtype)
	}
}

func TestPreClassify_DBErrorReturnsErrorField(t *testing.T) {
	repo := &mockClassificationRepo{
		loadRulesFn: func(_ context.Context, _ string) ([]classification.ClassificationRule, error) {
			return nil, errors.New("connection refused")
		},
	}

	act := NewPreClassifyActivities(logging.NewNopLogger(), repo)
	out, err := act.PreClassify(context.Background(), workflows.PreClassifyInput{
		TenantID:    "test-tenant",
		ContentType: "email",
		SenderEmail: "gsd-jira@akamai.com",
	})

	if err != nil {
		t.Fatalf("activity should not return error on DB failure, got: %v", err)
	}
	if out.Error == "" {
		t.Error("expected error field to be populated on DB failure")
	}
	if out.Matched {
		t.Error("should not report a match when rule loading fails")
	}
}

func TestPreClassify_WithRouting(t *testing.T) {
	repo := &mockClassificationRepo{
		loadRulesFn: func(_ context.Context, _ string) ([]classification.ClassificationRule, error) {
			return []classification.ClassificationRule{
				notificationRule("jira", "gsd-jira@akamai.com"),
			}, nil
		},
	}

	routeRepo := &mockRoutingRepo{
		loadRoutesFn: func(_ context.Context, _ string) ([]routing.Route, error) {
			return []routing.Route{
				{ContentType: "EMAIL", ContentSubtype: "NOTIFICATION", Pipeline: "notification", Active: true},
			}, nil
		},
	}

	act := NewPreClassifyActivities(logging.NewNopLogger(), repo)
	act.WithRoutingRepo(routeRepo)

	out, err := act.PreClassify(context.Background(), workflows.PreClassifyInput{
		TenantID:    "test-tenant",
		ContentType: "email",
		SenderEmail: "gsd-jira@akamai.com",
		Subject:     "JIRA Update",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.PipelineName != "notification" {
		t.Errorf("pipeline: got %q, want %q", out.PipelineName, "notification")
	}
}

// notificationRule creates a classification rule that matches on from_address contains.
func notificationRule(name, fromPattern string) classification.ClassificationRule {
	return classification.ClassificationRule{
		Name:               name,
		Priority:           10,
		ContentTypeScope:   "EMAIL",
		ContentType:        "EMAIL",
		ContentSubtype:     "NOTIFICATION",
		NotificationSource: name,
		Active:             true,
		Conditions: []classification.MatchCondition{
			{
				Field:     "from_address",
				MatchType: "contains",
				Value:     fromPattern,
			},
		},
	}
}
