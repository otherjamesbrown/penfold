package routing

import "strings"

// Router evaluates routing rules to determine which pipelines a content item enters.
type Router struct {
	routes []Route
}

// NewRouter creates a router from loaded routing rules.
func NewRouter(routes []Route) *Router {
	return &Router{routes: routes}
}

// Resolve returns the pipeline names that match the given content type and subtype.
// Matching logic:
//   - Exact match on (content_type, content_subtype) — highest priority
//   - Wildcard: route with content_subtype="" matches any subtype for that content_type
//   - Wildcard: route with content_type="" matches any content type
//   - No match → empty slice (item is skipped)
//
// Only active routes are considered. Multiple routes can match (multi-pipeline).
func (r *Router) Resolve(contentType, contentSubtype string) []string {
	ct := strings.ToUpper(contentType)
	cs := strings.ToUpper(contentSubtype)

	var pipelines []string
	seen := make(map[string]bool)

	for _, route := range r.routes {
		if !route.Active {
			continue
		}

		typeMatch := route.ContentType == "" || strings.EqualFold(route.ContentType, ct)
		subtypeMatch := route.ContentSubtype == "" || strings.EqualFold(route.ContentSubtype, cs)

		if typeMatch && subtypeMatch {
			if !seen[route.Pipeline] {
				pipelines = append(pipelines, route.Pipeline)
				seen[route.Pipeline] = true
			}
		}
	}

	return pipelines
}

// HasStage returns true if the named pipeline includes the given stage.
func HasStage(pipeline, stage string) bool {
	def, ok := Pipelines[pipeline]
	if !ok {
		return false
	}
	for _, s := range def.Stages {
		if s == stage {
			return true
		}
	}
	return false
}
