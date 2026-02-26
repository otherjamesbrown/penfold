package activities

// ContextSection identifies a type of background context that can be injected
// into a pipeline stage prompt.
type ContextSection int

const (
	SectionGlossary  ContextSection = iota
	SectionTopics
	SectionRisks
	SectionActions
	SectionDecisions
	SectionEvents
)

// ContextConfig defines which context sections a pipeline stage receives and
// the query parameters used to fetch each section. Both builder entry points
// (BuildExtractionContext and BuildContextPackage) read from DefaultConfigs()
// instead of hard-coding section selection.
type ContextConfig struct {
	Sections    []ContextSection
	TokenBudget int
	ActionScope string // "conversation" or "project"
	QueryLimits map[ContextSection]int
	RecencyDays map[ContextSection]int
}

// HasSection reports whether cfg includes the given section.
func (cfg ContextConfig) HasSection(s ContextSection) bool {
	for _, sec := range cfg.Sections {
		if sec == s {
			return true
		}
	}
	return false
}

// QueryLimit returns the configured limit for a section, or the fallback if
// none is set.
func (cfg ContextConfig) QueryLimit(s ContextSection, fallback int) int {
	if cfg.QueryLimits != nil {
		if v, ok := cfg.QueryLimits[s]; ok {
			return v
		}
	}
	return fallback
}

// Recency returns the configured recency window (days) for a section, or the
// fallback if none is set.
func (cfg ContextConfig) Recency(s ContextSection, fallback int) int {
	if cfg.RecencyDays != nil {
		if v, ok := cfg.RecencyDays[s]; ok {
			return v
		}
	}
	return fallback
}

// DefaultConfigs returns the stage-to-config mapping for every pipeline stage
// that receives background context.
func DefaultConfigs() map[string]ContextConfig {
	return map[string]ContextConfig{
		"extract_ner": {
			Sections:    []ContextSection{SectionGlossary, SectionTopics},
			TokenBudget: 500,
			QueryLimits: map[ContextSection]int{SectionGlossary: 20, SectionTopics: 10},
		},
		"extract_semantic": {
			Sections:    []ContextSection{SectionGlossary, SectionTopics},
			TokenBudget: 500,
			QueryLimits: map[ContextSection]int{SectionGlossary: 20, SectionTopics: 10},
		},
		"deep_analyze": {
			Sections:    []ContextSection{SectionGlossary, SectionTopics, SectionRisks, SectionActions, SectionDecisions, SectionEvents},
			TokenBudget: 2000,
			ActionScope: "conversation",
			QueryLimits: map[ContextSection]int{
				SectionGlossary:  20,
				SectionTopics:    10,
				SectionRisks:     10,
				SectionActions:   10,
				SectionDecisions: 5,
				SectionEvents:    10,
			},
			RecencyDays: map[ContextSection]int{
				SectionActions:   30,
				SectionDecisions: 60,
				SectionEvents:    90,
			},
		},
	}
}
