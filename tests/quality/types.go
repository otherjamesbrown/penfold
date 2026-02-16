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
	OneOf []string `yaml:"one_of"`
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
