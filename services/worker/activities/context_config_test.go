package activities

import (
	"testing"
)

func TestDefaultConfigs_ReturnsAllStages(t *testing.T) {
	configs := DefaultConfigs()

	for _, stage := range []string{"extract_ner", "extract_semantic", "deep_analyze"} {
		if _, ok := configs[stage]; !ok {
			t.Errorf("DefaultConfigs() missing stage %q", stage)
		}
	}
}

func TestDefaultConfigs_ExtractNER(t *testing.T) {
	cfg := DefaultConfigs()["extract_ner"]

	if cfg.TokenBudget != 500 {
		t.Errorf("extract_ner TokenBudget = %d, want 500", cfg.TokenBudget)
	}
	if !cfg.HasSection(SectionGlossary) {
		t.Error("extract_ner missing SectionGlossary")
	}
	if !cfg.HasSection(SectionTopics) {
		t.Error("extract_ner missing SectionTopics")
	}
	if cfg.HasSection(SectionRisks) {
		t.Error("extract_ner should not have SectionRisks")
	}
	if cfg.HasSection(SectionActions) {
		t.Error("extract_ner should not have SectionActions")
	}
	if cfg.QueryLimit(SectionGlossary, 0) != 20 {
		t.Errorf("extract_ner glossary limit = %d, want 20", cfg.QueryLimit(SectionGlossary, 0))
	}
	if cfg.QueryLimit(SectionTopics, 0) != 10 {
		t.Errorf("extract_ner topics limit = %d, want 10", cfg.QueryLimit(SectionTopics, 0))
	}
}

func TestDefaultConfigs_ExtractSemantic(t *testing.T) {
	cfg := DefaultConfigs()["extract_semantic"]

	if cfg.TokenBudget != 500 {
		t.Errorf("extract_semantic TokenBudget = %d, want 500", cfg.TokenBudget)
	}
	if !cfg.HasSection(SectionGlossary) || !cfg.HasSection(SectionTopics) {
		t.Error("extract_semantic missing expected sections")
	}
	if cfg.HasSection(SectionRisks) || cfg.HasSection(SectionActions) || cfg.HasSection(SectionDecisions) || cfg.HasSection(SectionEvents) {
		t.Error("extract_semantic should only have Glossary and Topics")
	}
}

func TestDefaultConfigs_DeepAnalyze(t *testing.T) {
	cfg := DefaultConfigs()["deep_analyze"]

	if cfg.TokenBudget != 2000 {
		t.Errorf("deep_analyze TokenBudget = %d, want 2000", cfg.TokenBudget)
	}
	if cfg.ActionScope != "conversation" {
		t.Errorf("deep_analyze ActionScope = %q, want %q", cfg.ActionScope, "conversation")
	}

	// All sections present
	for _, s := range []ContextSection{SectionGlossary, SectionTopics, SectionRisks, SectionActions, SectionDecisions, SectionEvents} {
		if !cfg.HasSection(s) {
			t.Errorf("deep_analyze missing section %d", s)
		}
	}

	// Query limits
	limits := map[ContextSection]int{
		SectionGlossary:  20,
		SectionTopics:    10,
		SectionRisks:     10,
		SectionActions:   10,
		SectionDecisions: 5,
		SectionEvents:    10,
	}
	for section, want := range limits {
		if got := cfg.QueryLimit(section, 0); got != want {
			t.Errorf("deep_analyze QueryLimit(%d) = %d, want %d", section, got, want)
		}
	}

	// Recency days
	recency := map[ContextSection]int{
		SectionActions:   30,
		SectionDecisions: 60,
		SectionEvents:    90,
	}
	for section, want := range recency {
		if got := cfg.Recency(section, 0); got != want {
			t.Errorf("deep_analyze Recency(%d) = %d, want %d", section, got, want)
		}
	}
}

func TestContextConfig_HasSection(t *testing.T) {
	cfg := ContextConfig{
		Sections: []ContextSection{SectionGlossary, SectionTopics},
	}

	if !cfg.HasSection(SectionGlossary) {
		t.Error("HasSection(SectionGlossary) = false, want true")
	}
	if cfg.HasSection(SectionRisks) {
		t.Error("HasSection(SectionRisks) = true, want false")
	}
}

func TestContextConfig_QueryLimit_Fallback(t *testing.T) {
	cfg := ContextConfig{}

	if got := cfg.QueryLimit(SectionGlossary, 42); got != 42 {
		t.Errorf("QueryLimit with nil map = %d, want 42", got)
	}

	cfg.QueryLimits = map[ContextSection]int{SectionGlossary: 10}
	if got := cfg.QueryLimit(SectionTopics, 99); got != 99 {
		t.Errorf("QueryLimit for missing key = %d, want 99", got)
	}
}

func TestContextConfig_Recency_Fallback(t *testing.T) {
	cfg := ContextConfig{}

	if got := cfg.Recency(SectionActions, 30); got != 30 {
		t.Errorf("Recency with nil map = %d, want 30", got)
	}

	cfg.RecencyDays = map[ContextSection]int{SectionDecisions: 60}
	if got := cfg.Recency(SectionActions, 7); got != 7 {
		t.Errorf("Recency for missing key = %d, want 7", got)
	}
}
