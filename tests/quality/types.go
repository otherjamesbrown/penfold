//go:build quality

// Package quality provides extraction quality tests for the Penfold pipeline.
// These tests run real emails through the real pipeline (actual LLM calls) and
// compare extraction output against golden YAML files with semantic matching.
package quality

// GoldenExpectation represents the expected extraction output for a test email.
// Loaded from golden/*.yaml files.
type GoldenExpectation struct {
	Email       string `yaml:"email"`
	Description string `yaml:"description"`

	LastVerified          string `yaml:"last_verified"`
	ModelAtVerification   string `yaml:"model_at_verification"`

	Pipeline   PipelineExpectation    `yaml:"pipeline"`
	Triage     *TriageExpectation     `yaml:"triage"`
	People     *PeopleExpectation     `yaml:"people"`
	Assertions *AssertionExpectation  `yaml:"assertions"`
	Projects   *ProjectsExpectation   `yaml:"projects"`

	// --- New fields for category evals ---
	Category          string                        `yaml:"category"`
	Routing           *RoutingExpectation           `yaml:"routing"`
	NewsletterExtract *NewsletterExtractExpectation `yaml:"newsletter_extract"`
	Digest            *DigestExpectation            `yaml:"digest"`
	Intent            *IntentExpectation            `yaml:"intent"`
}

// PipelineExpectation defines which pipeline stages must complete.
type PipelineExpectation struct {
	MustComplete []string `yaml:"must_complete"`
}

// TriageExpectation defines expected triage classification.
type TriageExpectation struct {
	Importance *OneOfMatcher `yaml:"importance"`
	Category   *OneOfMatcher `yaml:"category"`
}

// OneOfMatcher matches a value against a list of acceptable options.
type OneOfMatcher struct {
	OneOf     []string `yaml:"one_of"`
	MustNotBe []string `yaml:"must_not_be"`
}

// PeopleExpectation defines expected person extraction results.
type PeopleExpectation struct {
	MinCount    *int            `yaml:"min_count"`
	MaxCount    *int            `yaml:"max_count"`
	MustFind    []PersonMatcher `yaml:"must_find"`
	MustNotFind []PersonMatcher `yaml:"must_not_find"`
}

// PersonMatcher matches an extracted person by name and optional role.
type PersonMatcher struct {
	NameContains string  `yaml:"name_contains"`
	RoleContains string  `yaml:"role_contains"`
	ConfidenceMin *float64 `yaml:"confidence_min"`
}

// AssertionExpectation defines expected assertion extraction results.
type AssertionExpectation struct {
	MinCount    *int               `yaml:"min_count"`
	MaxCount    *int               `yaml:"max_count"`
	MustFind    []AssertionMatcher `yaml:"must_find"`
	MustNotFind []AssertionMatcher `yaml:"must_not_find"`
}

// AssertionMatcher matches an extracted assertion by type and description.
type AssertionMatcher struct {
	Type                string   `yaml:"type"`
	DescriptionContains string   `yaml:"description_contains"`
	ConfidenceMin       *float64 `yaml:"confidence_min"`
}

// ProjectsExpectation defines expected project extraction results.
type ProjectsExpectation struct {
	MinCount    *int             `yaml:"min_count"`
	MaxCount    *int             `yaml:"max_count"`
	MustFind    []ProjectMatcher `yaml:"must_find"`
	MustNotFind []ProjectMatcher `yaml:"must_not_find"`
}

// ProjectMatcher matches an extracted project by name.
type ProjectMatcher struct {
	NameContains string `yaml:"name_contains"`
}

// --- Actual extraction data structures (queried from DB) ---

// ActualPerson represents a person record from the database.
type ActualPerson struct {
	ID            int64
	CanonicalName string
	Title         string
	AutoCreated   bool
}

// ActualAssertion represents an assertion record from the database.
type ActualAssertion struct {
	ID            int64
	AssertionType string
	Description   string
	Confidence    float64
	IsCurrent     bool
}

// ActualProject represents a project reference found in extraction.
type ActualProject struct {
	ID   int64
	Name string
}

// ActualTriageResult represents parsed triage output from pipeline_runs.
type ActualTriageResult struct {
	Importance string `json:"importance"`
	Category   string `json:"category"`
}

// RoutingExpectation validates L1: classification and pipeline routing.
type RoutingExpectation struct {
	ContentSubtype     string   `yaml:"content_subtype"`
	NotificationSource string   `yaml:"notification_source"`
	Pipeline           string   `yaml:"pipeline"`
	MustComplete       []string `yaml:"must_complete"`
	MustNotRun         []string `yaml:"must_not_run"`
}

// NewsletterExtractExpectation validates L2: newsletter_extract stage output.
type NewsletterExtractExpectation struct {
	Summary              *StringFieldExpectation `yaml:"summary"`
	Sections             *CountExpectation       `yaml:"sections"`
	ActionItems          *ItemListExpectation    `yaml:"action_items"`
	KeyAnnouncements     *ItemListExpectation    `yaml:"key_announcements"`
	Risks                *ItemListExpectation    `yaml:"risks"`
	PeopleMentioned      *CountExpectation       `yaml:"people_mentioned"`
	QualityGateTriggered *bool                   `yaml:"quality_gate_triggered"`
}

// StringFieldExpectation validates a string field.
type StringFieldExpectation struct {
	MinLength   *int     `yaml:"min_length"`
	MustMention []string `yaml:"must_mention"`
}

// CountExpectation validates array length.
type CountExpectation struct {
	MinCount *int `yaml:"min_count"`
	MaxCount *int `yaml:"max_count"`
}

// ItemListExpectation validates an array of items with count bounds and content checks.
type ItemListExpectation struct {
	MinCount    *int          `yaml:"min_count"`
	MaxCount    *int          `yaml:"max_count"`
	MustFind    []ItemMatcher `yaml:"must_find"`
	MustNotFind []ItemMatcher `yaml:"must_not_find"`
}

// ItemMatcher matches an item in a list by substring.
type ItemMatcher struct {
	DescriptionContains string `yaml:"description_contains"`
}

// DigestExpectation validates L3: how content contributes to digest output.
type DigestExpectation struct {
	Handling         string   `yaml:"handling"`
	FieldsInRollup   []string `yaml:"fields_in_rollup"`
	SummaryMinLength *int     `yaml:"summary_min_length"`
}

// IntentExpectation captures James's stated purpose for this content.
type IntentExpectation struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

// --- Eval Results (used by matchers to report structured results for Langfuse scoring) ---

// MatchDetail records a single check outcome.
type MatchDetail struct {
	Check    string `json:"check"`
	Pass     bool   `json:"pass"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

// EvalResults aggregates all eval checks for Langfuse score recording.
type EvalResults struct {
	L1Routing []MatchDetail `json:"l1_routing"`
	L2Quality []MatchDetail `json:"l2_quality"`
	L3Digest  []MatchDetail `json:"l3_digest"`
}

// L1Pass returns true if all routing checks passed.
func (r *EvalResults) L1Pass() bool {
	for _, d := range r.L1Routing {
		if !d.Pass {
			return false
		}
	}
	return true
}

// L2Score returns the fraction of quality checks that passed (0.0–1.0).
func (r *EvalResults) L2Score() float64 {
	if len(r.L2Quality) == 0 {
		return 1.0
	}
	pass := 0
	for _, d := range r.L2Quality {
		if d.Pass {
			pass++
		}
	}
	return float64(pass) / float64(len(r.L2Quality))
}

// L3Pass returns true if all digest contribution checks passed.
func (r *EvalResults) L3Pass() bool {
	for _, d := range r.L3Digest {
		if !d.Pass {
			return false
		}
	}
	return true
}

// --- Actual data structs for newsletter extraction ---

// ActualNewsletterExtraction represents parsed newsletter_extract output from content_enrichment.
type ActualNewsletterExtraction struct {
	NewsletterName       string                    `json:"newsletter_name"`
	Edition              string                    `json:"edition"`
	Summary              string                    `json:"summary"`
	Sections             []ActualNewsletterSection `json:"sections"`
	ActionItems          []ActualNewsletterAction  `json:"action_items"`
	KeyAnnouncements     []string                  `json:"key_announcements"`
	Risks                []string                  `json:"risks"`
	PeopleMentioned      []ActualNewsletterPerson  `json:"people_mentioned"`
	QualityGateTriggered bool                      `json:"quality_gate_triggered"`
}

type ActualNewsletterSection struct {
	Heading  string `json:"heading"`
	Summary  string `json:"summary"`
	Category string `json:"category"`
}

type ActualNewsletterAction struct {
	Assignee  string `json:"assignee"`
	Action    string `json:"action"`
	Due       string `json:"due"`
	Mandatory bool   `json:"mandatory"`
}

type ActualNewsletterPerson struct {
	Name    string `json:"name"`
	Context string `json:"context"`
}
