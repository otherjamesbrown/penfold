// Package routing provides data-driven pipeline routing for the Penfold pipeline.
// After classification, the routing table determines which pipeline(s) each content
// item enters. Items with no matching route are skipped entirely.
package routing

// Route represents a pipeline routing rule loaded from the database.
type Route struct {
	ID             int
	TenantID       string
	ContentType    string // NULL stored as "" = any content type
	ContentSubtype string // NULL stored as "" = any subtype
	Pipeline       string // pipeline identifier (e.g. "standard", "attendees_only")
	Active         bool
}

// PipelineDef defines the stages for a named pipeline.
type PipelineDef struct {
	Name   string
	Stages []string
}

// Pipeline definitions: which stages each pipeline executes.
var Pipelines = map[string]PipelineDef{
	"standard": {
		Name:   "standard",
		Stages: []string{"triage", "extract", "embed", "summarize"},
	},
	"attendees_only": {
		Name:   "attendees_only",
		Stages: []string{"extract"},
	},
}
