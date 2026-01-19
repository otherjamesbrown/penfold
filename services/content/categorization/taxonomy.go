// Package categorization provides content categorization with rule-based
// and ML-based classification supporting hierarchical taxonomies.
package categorization

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Common errors for taxonomy operations.
var (
	ErrCategoryNotFound     = errors.New("category not found")
	ErrCategoryExists       = errors.New("category already exists")
	ErrInvalidParent        = errors.New("invalid parent category")
	ErrCircularDependency   = errors.New("circular dependency detected")
	ErrTaxonomyNotFound     = errors.New("taxonomy not found")
	ErrEmptyTaxonomy        = errors.New("taxonomy is empty")
	ErrInvalidCategoryID    = errors.New("invalid category ID")
	ErrCategoryHasChildren  = errors.New("category has children")
)

// Category represents a single category in the taxonomy.
type Category struct {
	// ID is the unique identifier for this category.
	ID string `json:"id"`

	// Name is the human-readable name.
	Name string `json:"name"`

	// Description provides additional context about the category.
	Description string `json:"description,omitempty"`

	// ParentID references the parent category (empty for root categories).
	ParentID string `json:"parent_id,omitempty"`

	// Keywords are terms associated with this category for matching.
	Keywords []string `json:"keywords,omitempty"`

	// Patterns are regex patterns for matching content to this category.
	Patterns []string `json:"patterns,omitempty"`

	// Metadata holds additional category-specific data.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Weight affects scoring when multiple categories match.
	Weight float64 `json:"weight,omitempty"`

	// MinConfidence is the minimum confidence threshold for this category.
	MinConfidence float64 `json:"min_confidence,omitempty"`

	// Enabled indicates whether this category is active.
	Enabled bool `json:"enabled"`
}

// NewCategory creates a new category with the given ID and name.
func NewCategory(id, name string) *Category {
	return &Category{
		ID:            id,
		Name:          name,
		Keywords:      make([]string, 0),
		Patterns:      make([]string, 0),
		Metadata:      make(map[string]string),
		Weight:        1.0,
		MinConfidence: 0.5,
		Enabled:       true,
	}
}

// WithParent sets the parent category ID.
func (c *Category) WithParent(parentID string) *Category {
	c.ParentID = parentID
	return c
}

// WithKeywords adds keywords to the category.
func (c *Category) WithKeywords(keywords ...string) *Category {
	c.Keywords = append(c.Keywords, keywords...)
	return c
}

// WithPatterns adds regex patterns to the category.
func (c *Category) WithPatterns(patterns ...string) *Category {
	c.Patterns = append(c.Patterns, patterns...)
	return c
}

// WithWeight sets the category weight.
func (c *Category) WithWeight(weight float64) *Category {
	c.Weight = weight
	return c
}

// WithMinConfidence sets the minimum confidence threshold.
func (c *Category) WithMinConfidence(minConfidence float64) *Category {
	c.MinConfidence = minConfidence
	return c
}

// IsRoot returns true if this is a root category (no parent).
func (c *Category) IsRoot() bool {
	return c.ParentID == ""
}

// Clone creates a deep copy of the category.
func (c *Category) Clone() *Category {
	clone := &Category{
		ID:            c.ID,
		Name:          c.Name,
		Description:   c.Description,
		ParentID:      c.ParentID,
		Weight:        c.Weight,
		MinConfidence: c.MinConfidence,
		Enabled:       c.Enabled,
	}

	if len(c.Keywords) > 0 {
		clone.Keywords = make([]string, len(c.Keywords))
		copy(clone.Keywords, c.Keywords)
	}

	if len(c.Patterns) > 0 {
		clone.Patterns = make([]string, len(c.Patterns))
		copy(clone.Patterns, c.Patterns)
	}

	if len(c.Metadata) > 0 {
		clone.Metadata = make(map[string]string)
		for k, v := range c.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// TaxonomyTree manages a hierarchical category structure.
type TaxonomyTree struct {
	mu sync.RWMutex

	// Name identifies this taxonomy.
	Name string

	// Description provides context about the taxonomy.
	Description string

	// TenantID identifies the tenant this taxonomy belongs to (empty for global).
	TenantID string

	// categories maps category IDs to categories.
	categories map[string]*Category

	// children maps parent IDs to their child category IDs.
	children map[string][]string

	// roots contains IDs of root categories.
	roots []string
}

// NewTaxonomyTree creates a new empty taxonomy tree.
func NewTaxonomyTree(name string) *TaxonomyTree {
	return &TaxonomyTree{
		Name:       name,
		categories: make(map[string]*Category),
		children:   make(map[string][]string),
		roots:      make([]string, 0),
	}
}

// WithDescription sets the taxonomy description.
func (t *TaxonomyTree) WithDescription(description string) *TaxonomyTree {
	t.Description = description
	return t
}

// WithTenant sets the tenant ID for this taxonomy.
func (t *TaxonomyTree) WithTenant(tenantID string) *TaxonomyTree {
	t.TenantID = tenantID
	return t
}

// AddCategory adds a category to the taxonomy.
func (t *TaxonomyTree) AddCategory(category *Category) error {
	if category == nil || category.ID == "" {
		return ErrInvalidCategoryID
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if category already exists
	if _, exists := t.categories[category.ID]; exists {
		return fmt.Errorf("%w: %s", ErrCategoryExists, category.ID)
	}

	// Validate parent if specified
	if category.ParentID != "" {
		if _, exists := t.categories[category.ParentID]; !exists {
			return fmt.Errorf("%w: %s", ErrInvalidParent, category.ParentID)
		}
	}

	// Add the category
	t.categories[category.ID] = category.Clone()

	// Update tree structure
	if category.ParentID == "" {
		t.roots = append(t.roots, category.ID)
	} else {
		t.children[category.ParentID] = append(t.children[category.ParentID], category.ID)
	}

	return nil
}

// RemoveCategory removes a category from the taxonomy.
// Returns an error if the category has children.
func (t *TaxonomyTree) RemoveCategory(categoryID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	category, exists := t.categories[categoryID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	// Check for children
	if len(t.children[categoryID]) > 0 {
		return fmt.Errorf("%w: %s has %d children", ErrCategoryHasChildren, categoryID, len(t.children[categoryID]))
	}

	// Remove from parent's children list
	if category.ParentID != "" {
		children := t.children[category.ParentID]
		for i, id := range children {
			if id == categoryID {
				t.children[category.ParentID] = append(children[:i], children[i+1:]...)
				break
			}
		}
	} else {
		// Remove from roots
		for i, id := range t.roots {
			if id == categoryID {
				t.roots = append(t.roots[:i], t.roots[i+1:]...)
				break
			}
		}
	}

	// Remove the category
	delete(t.categories, categoryID)
	delete(t.children, categoryID)

	return nil
}

// GetCategory retrieves a category by ID.
func (t *TaxonomyTree) GetCategory(categoryID string) (*Category, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	category, exists := t.categories[categoryID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	return category.Clone(), nil
}

// GetAllCategories returns all categories in the taxonomy.
func (t *TaxonomyTree) GetAllCategories() []*Category {
	t.mu.RLock()
	defer t.mu.RUnlock()

	categories := make([]*Category, 0, len(t.categories))
	for _, c := range t.categories {
		categories = append(categories, c.Clone())
	}

	// Sort by ID for consistent ordering
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].ID < categories[j].ID
	})

	return categories
}

// GetEnabledCategories returns only enabled categories.
func (t *TaxonomyTree) GetEnabledCategories() []*Category {
	t.mu.RLock()
	defer t.mu.RUnlock()

	categories := make([]*Category, 0)
	for _, c := range t.categories {
		if c.Enabled {
			categories = append(categories, c.Clone())
		}
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].ID < categories[j].ID
	})

	return categories
}

// GetRootCategories returns all root categories.
func (t *TaxonomyTree) GetRootCategories() []*Category {
	t.mu.RLock()
	defer t.mu.RUnlock()

	roots := make([]*Category, 0, len(t.roots))
	for _, id := range t.roots {
		if c, exists := t.categories[id]; exists {
			roots = append(roots, c.Clone())
		}
	}

	return roots
}

// GetChildren returns the direct children of a category.
func (t *TaxonomyTree) GetChildren(categoryID string) ([]*Category, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if _, exists := t.categories[categoryID]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	childIDs := t.children[categoryID]
	children := make([]*Category, 0, len(childIDs))
	for _, id := range childIDs {
		if c, exists := t.categories[id]; exists {
			children = append(children, c.Clone())
		}
	}

	return children, nil
}

// GetParent returns the parent of a category.
func (t *TaxonomyTree) GetParent(categoryID string) (*Category, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	category, exists := t.categories[categoryID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	if category.ParentID == "" {
		return nil, nil // Root category has no parent
	}

	parent, exists := t.categories[category.ParentID]
	if !exists {
		return nil, fmt.Errorf("%w: parent %s", ErrCategoryNotFound, category.ParentID)
	}

	return parent.Clone(), nil
}

// GetAncestors returns all ancestors from the category up to the root.
func (t *TaxonomyTree) GetAncestors(categoryID string) ([]*Category, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	category, exists := t.categories[categoryID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	ancestors := make([]*Category, 0)
	current := category

	for current.ParentID != "" {
		parent, exists := t.categories[current.ParentID]
		if !exists {
			break
		}
		ancestors = append(ancestors, parent.Clone())
		current = parent
	}

	return ancestors, nil
}

// GetDescendants returns all descendants of a category.
func (t *TaxonomyTree) GetDescendants(categoryID string) ([]*Category, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if _, exists := t.categories[categoryID]; !exists {
		return nil, fmt.Errorf("%w: %s", ErrCategoryNotFound, categoryID)
	}

	descendants := make([]*Category, 0)
	t.collectDescendants(categoryID, &descendants)

	return descendants, nil
}

// collectDescendants recursively collects all descendants.
func (t *TaxonomyTree) collectDescendants(categoryID string, result *[]*Category) {
	childIDs := t.children[categoryID]
	for _, id := range childIDs {
		if c, exists := t.categories[id]; exists {
			*result = append(*result, c.Clone())
			t.collectDescendants(id, result)
		}
	}
}

// GetPath returns the path from root to the category.
func (t *TaxonomyTree) GetPath(categoryID string) ([]*Category, error) {
	ancestors, err := t.GetAncestors(categoryID)
	if err != nil {
		return nil, err
	}

	category, err := t.GetCategory(categoryID)
	if err != nil {
		return nil, err
	}

	// Reverse ancestors and append the category
	path := make([]*Category, 0, len(ancestors)+1)
	for i := len(ancestors) - 1; i >= 0; i-- {
		path = append(path, ancestors[i])
	}
	path = append(path, category)

	return path, nil
}

// GetPathString returns the path as a string (e.g., "root/parent/child").
func (t *TaxonomyTree) GetPathString(categoryID string, separator string) (string, error) {
	path, err := t.GetPath(categoryID)
	if err != nil {
		return "", err
	}

	if separator == "" {
		separator = "/"
	}

	names := make([]string, len(path))
	for i, c := range path {
		names[i] = c.Name
	}

	return strings.Join(names, separator), nil
}

// GetDepth returns the depth of a category in the tree (root = 0).
func (t *TaxonomyTree) GetDepth(categoryID string) (int, error) {
	ancestors, err := t.GetAncestors(categoryID)
	if err != nil {
		return -1, err
	}

	return len(ancestors), nil
}

// Size returns the total number of categories.
func (t *TaxonomyTree) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.categories)
}

// IsEmpty returns true if the taxonomy has no categories.
func (t *TaxonomyTree) IsEmpty() bool {
	return t.Size() == 0
}

// Clear removes all categories from the taxonomy.
func (t *TaxonomyTree) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.categories = make(map[string]*Category)
	t.children = make(map[string][]string)
	t.roots = make([]string, 0)
}

// Clone creates a deep copy of the taxonomy tree.
func (t *TaxonomyTree) Clone() *TaxonomyTree {
	t.mu.RLock()
	defer t.mu.RUnlock()

	clone := NewTaxonomyTree(t.Name)
	clone.Description = t.Description
	clone.TenantID = t.TenantID

	// Clone categories
	for id, c := range t.categories {
		clone.categories[id] = c.Clone()
	}

	// Clone children
	for parentID, childIDs := range t.children {
		clone.children[parentID] = make([]string, len(childIDs))
		copy(clone.children[parentID], childIDs)
	}

	// Clone roots
	clone.roots = make([]string, len(t.roots))
	copy(clone.roots, t.roots)

	return clone
}

// =============================================================================
// Default Taxonomies
// =============================================================================

// DefaultEmailTaxonomy creates a standard taxonomy for email categorization.
func DefaultEmailTaxonomy() *TaxonomyTree {
	taxonomy := NewTaxonomyTree("email").
		WithDescription("Standard email categorization taxonomy")

	// Root categories
	taxonomy.AddCategory(NewCategory("work", "Work").
		WithKeywords("meeting", "project", "deadline", "report", "presentation").
		WithPatterns(`(?i)(quarterly|annual)\s+report`))

	taxonomy.AddCategory(NewCategory("personal", "Personal").
		WithKeywords("family", "friend", "vacation", "personal").
		WithWeight(0.9))

	taxonomy.AddCategory(NewCategory("newsletter", "Newsletter").
		WithKeywords("unsubscribe", "newsletter", "weekly digest", "update").
		WithPatterns(`(?i)unsubscribe|view\s+in\s+browser`))

	taxonomy.AddCategory(NewCategory("transaction", "Transaction").
		WithKeywords("receipt", "invoice", "payment", "order", "confirmation").
		WithPatterns(`(?i)(order|payment)\s+(confirmed|received)`))

	taxonomy.AddCategory(NewCategory("notification", "Notification").
		WithKeywords("notification", "alert", "reminder", "update").
		WithWeight(0.8))

	// Work subcategories
	taxonomy.AddCategory(NewCategory("work-meeting", "Meeting").
		WithParent("work").
		WithKeywords("agenda", "attendees", "calendar", "invite", "reschedule"))

	taxonomy.AddCategory(NewCategory("work-project", "Project").
		WithParent("work").
		WithKeywords("milestone", "task", "sprint", "jira", "asana"))

	taxonomy.AddCategory(NewCategory("work-hr", "HR").
		WithParent("work").
		WithKeywords("benefits", "payroll", "vacation request", "time off", "onboarding"))

	// Transaction subcategories
	taxonomy.AddCategory(NewCategory("transaction-purchase", "Purchase").
		WithParent("transaction").
		WithKeywords("order", "shipping", "delivery", "tracking"))

	taxonomy.AddCategory(NewCategory("transaction-banking", "Banking").
		WithParent("transaction").
		WithKeywords("statement", "balance", "transfer", "deposit", "withdrawal"))

	return taxonomy
}

// DefaultDocumentTaxonomy creates a standard taxonomy for document categorization.
func DefaultDocumentTaxonomy() *TaxonomyTree {
	taxonomy := NewTaxonomyTree("document").
		WithDescription("Standard document categorization taxonomy")

	// Root categories
	taxonomy.AddCategory(NewCategory("technical", "Technical").
		WithKeywords("api", "documentation", "specification", "architecture", "code").
		WithPatterns(`(?i)(api|sdk)\s+(reference|documentation)`))

	taxonomy.AddCategory(NewCategory("business", "Business").
		WithKeywords("strategy", "plan", "proposal", "analysis", "report"))

	taxonomy.AddCategory(NewCategory("legal", "Legal").
		WithKeywords("contract", "agreement", "terms", "policy", "compliance").
		WithMinConfidence(0.7))

	taxonomy.AddCategory(NewCategory("financial", "Financial").
		WithKeywords("budget", "forecast", "revenue", "expense", "profit"))

	taxonomy.AddCategory(NewCategory("marketing", "Marketing").
		WithKeywords("campaign", "brand", "content", "social media", "seo"))

	// Technical subcategories
	taxonomy.AddCategory(NewCategory("technical-design", "Design Document").
		WithParent("technical").
		WithKeywords("design", "architecture", "system", "component"))

	taxonomy.AddCategory(NewCategory("technical-api", "API Documentation").
		WithParent("technical").
		WithKeywords("endpoint", "request", "response", "authentication"))

	taxonomy.AddCategory(NewCategory("technical-runbook", "Runbook").
		WithParent("technical").
		WithKeywords("procedure", "troubleshooting", "incident", "playbook"))

	// Business subcategories
	taxonomy.AddCategory(NewCategory("business-proposal", "Proposal").
		WithParent("business").
		WithKeywords("proposal", "rfp", "quote", "bid"))

	taxonomy.AddCategory(NewCategory("business-report", "Report").
		WithParent("business").
		WithKeywords("quarterly", "annual", "summary", "findings"))

	return taxonomy
}

// DefaultMeetingTaxonomy creates a standard taxonomy for meeting categorization.
func DefaultMeetingTaxonomy() *TaxonomyTree {
	taxonomy := NewTaxonomyTree("meeting").
		WithDescription("Standard meeting categorization taxonomy")

	// Root categories
	taxonomy.AddCategory(NewCategory("standup", "Standup").
		WithKeywords("standup", "daily", "scrum", "sync", "status").
		WithPatterns(`(?i)daily\s+(standup|scrum|sync)`))

	taxonomy.AddCategory(NewCategory("planning", "Planning").
		WithKeywords("planning", "sprint", "roadmap", "backlog", "grooming"))

	taxonomy.AddCategory(NewCategory("review", "Review").
		WithKeywords("review", "retrospective", "retro", "demo", "showcase"))

	taxonomy.AddCategory(NewCategory("interview", "Interview").
		WithKeywords("interview", "candidate", "hiring", "onsite", "screening"))

	taxonomy.AddCategory(NewCategory("one-on-one", "One-on-One").
		WithKeywords("1:1", "one on one", "check-in", "feedback"))

	taxonomy.AddCategory(NewCategory("training", "Training").
		WithKeywords("training", "workshop", "learning", "onboarding", "tutorial"))

	taxonomy.AddCategory(NewCategory("external", "External").
		WithKeywords("client", "vendor", "partner", "sales", "customer"))

	// External subcategories
	taxonomy.AddCategory(NewCategory("external-client", "Client Meeting").
		WithParent("external").
		WithKeywords("client", "customer", "account"))

	taxonomy.AddCategory(NewCategory("external-vendor", "Vendor Meeting").
		WithParent("external").
		WithKeywords("vendor", "supplier", "contractor"))

	return taxonomy
}

// TaxonomyRegistry manages multiple taxonomies.
type TaxonomyRegistry struct {
	mu         sync.RWMutex
	taxonomies map[string]*TaxonomyTree
}

// NewTaxonomyRegistry creates a new taxonomy registry.
func NewTaxonomyRegistry() *TaxonomyRegistry {
	return &TaxonomyRegistry{
		taxonomies: make(map[string]*TaxonomyTree),
	}
}

// Register adds a taxonomy to the registry.
func (r *TaxonomyRegistry) Register(taxonomy *TaxonomyTree) error {
	if taxonomy == nil || taxonomy.Name == "" {
		return errors.New("invalid taxonomy")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.taxonomies[taxonomy.Name] = taxonomy
	return nil
}

// Get retrieves a taxonomy by name.
func (r *TaxonomyRegistry) Get(name string) (*TaxonomyTree, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taxonomy, exists := r.taxonomies[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTaxonomyNotFound, name)
	}

	return taxonomy, nil
}

// List returns all registered taxonomy names.
func (r *TaxonomyRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.taxonomies))
	for name := range r.taxonomies {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// RegisterDefaults registers all default taxonomies.
func (r *TaxonomyRegistry) RegisterDefaults() {
	r.Register(DefaultEmailTaxonomy())
	r.Register(DefaultDocumentTaxonomy())
	r.Register(DefaultMeetingTaxonomy())
}
