// Package privacy provides privacy filtering and PII detection for Gmail content.
package privacy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// SensitivityLevel defines how aggressively the filter should process content.
type SensitivityLevel int

const (
	// SensitivityLow only applies explicit blocklist filtering.
	SensitivityLow SensitivityLevel = iota

	// SensitivityMedium applies PII detection and redaction.
	SensitivityMedium

	// SensitivityHigh applies full content filtering including body analysis.
	SensitivityHigh
)

// String returns the string representation of the sensitivity level.
func (s SensitivityLevel) String() string {
	switch s {
	case SensitivityLow:
		return "low"
	case SensitivityMedium:
		return "medium"
	case SensitivityHigh:
		return "high"
	default:
		return "unknown"
	}
}

// ParseSensitivityLevel converts a string to a SensitivityLevel.
func ParseSensitivityLevel(s string) SensitivityLevel {
	switch strings.ToLower(s) {
	case "low":
		return SensitivityLow
	case "medium":
		return SensitivityMedium
	case "high":
		return SensitivityHigh
	default:
		return SensitivityMedium // Default to medium.
	}
}

// Message represents an email message to be filtered.
type Message struct {
	// ID is the unique message identifier.
	ID string `json:"id"`

	// ThreadID is the conversation thread identifier.
	ThreadID string `json:"thread_id,omitempty"`

	// From is the sender's email address.
	From string `json:"from"`

	// To contains recipient email addresses.
	To []string `json:"to"`

	// Cc contains CC recipient email addresses.
	Cc []string `json:"cc,omitempty"`

	// Subject is the message subject line.
	Subject string `json:"subject"`

	// Body is the message body content.
	Body string `json:"body"`

	// Labels contains Gmail labels.
	Labels []string `json:"labels,omitempty"`

	// ReceivedAt is when the message was received.
	ReceivedAt time.Time `json:"received_at"`

	// Attachments contains attachment metadata.
	Attachments []AttachmentInfo `json:"attachments,omitempty"`
}

// AttachmentInfo contains metadata about a message attachment.
type AttachmentInfo struct {
	// ID is the attachment identifier.
	ID string `json:"id"`

	// Filename is the attachment filename.
	Filename string `json:"filename"`

	// MimeType is the attachment MIME type.
	MimeType string `json:"mime_type"`

	// Size is the attachment size in bytes.
	Size int64 `json:"size"`
}

// FilterResult contains the outcome of filtering a message.
type FilterResult struct {
	// Message is the filtered message (may have redacted content).
	Message *Message `json:"message"`

	// Excluded indicates the message should not be stored.
	Excluded bool `json:"excluded"`

	// ExclusionReason explains why the message was excluded.
	ExclusionReason string `json:"exclusion_reason,omitempty"`

	// PIIDetected contains all PII locations found in the message.
	PIIDetected []PIILocation `json:"pii_detected,omitempty"`

	// RedactionCount is the number of redactions applied.
	RedactionCount int `json:"redaction_count"`

	// ProcessingTime is how long filtering took.
	ProcessingTime time.Duration `json:"processing_time"`
}

// AuditEntry records a filtering action for audit purposes.
type AuditEntry struct {
	// ID is the unique audit entry identifier.
	ID string `json:"id"`

	// MessageID is the filtered message identifier.
	MessageID string `json:"message_id"`

	// TenantID is the tenant/account identifier.
	TenantID string `json:"tenant_id"`

	// Timestamp is when the filtering occurred.
	Timestamp time.Time `json:"timestamp"`

	// Action describes what filtering action was taken.
	Action string `json:"action"`

	// SensitivityLevel is the configured level during filtering.
	SensitivityLevel string `json:"sensitivity_level"`

	// PIITypesDetected lists the types of PII found.
	PIITypesDetected []string `json:"pii_types_detected,omitempty"`

	// RedactionCount is the number of redactions performed.
	RedactionCount int `json:"redaction_count"`

	// Excluded indicates if the message was excluded.
	Excluded bool `json:"excluded"`

	// ExclusionReason explains why the message was excluded.
	ExclusionReason string `json:"exclusion_reason,omitempty"`
}

// AuditLogger defines the interface for recording audit entries.
type AuditLogger interface {
	// Log records an audit entry.
	Log(ctx context.Context, entry *AuditEntry) error
}

// FilterConfig holds configuration for the PrivacyFilter.
type FilterConfig struct {
	// SensitivityLevel controls filtering aggressiveness.
	SensitivityLevel SensitivityLevel

	// RedactionPlaceholder is used to replace detected PII.
	RedactionPlaceholder string

	// BlockedSenders is a list of email addresses to exclude.
	BlockedSenders []string

	// BlockedDomains is a list of domains to exclude.
	BlockedDomains []string

	// BlockedSubjectPatterns is a list of regex patterns for subjects to exclude.
	BlockedSubjectPatterns []string

	// AllowedSenders is an allowlist that bypasses filtering.
	AllowedSenders []string

	// AllowedDomains is an allowlist of domains that bypass filtering.
	AllowedDomains []string

	// CustomRules allows adding rules beyond the defaults.
	CustomRules []Rule

	// SkipAttachments when true, doesn't process attachment metadata.
	SkipAttachments bool

	// AuditLogger records filtering actions.
	AuditLogger AuditLogger
}

// DefaultFilterConfig returns a FilterConfig with sensible defaults.
func DefaultFilterConfig() *FilterConfig {
	return &FilterConfig{
		SensitivityLevel:     SensitivityMedium,
		RedactionPlaceholder: "[REDACTED]",
		BlockedSenders:       []string{},
		BlockedDomains:       []string{},
		AllowedSenders:       []string{},
		AllowedDomains:       []string{},
		CustomRules:          []Rule{},
		SkipAttachments:      false,
	}
}

// PrivacyFilter processes messages to detect and redact sensitive information.
type PrivacyFilter struct {
	config  *FilterConfig
	rules   []Rule
	mu      sync.RWMutex
	metrics *FilterMetrics

	// Compiled patterns for blocklist checking.
	blockedSendersMap   map[string]bool
	blockedDomainsMap   map[string]bool
	allowedSendersMap   map[string]bool
	allowedDomainsMap   map[string]bool
}

// FilterMetrics holds Prometheus metrics for filtering operations.
type FilterMetrics struct {
	MessagesProcessed prometheus.Counter
	MessagesExcluded  prometheus.Counter
	PIIDetections     *prometheus.CounterVec
	RedactionsApplied prometheus.Counter
	ProcessingTime    prometheus.Histogram
	FilterErrors      prometheus.Counter
}

// NewFilterMetrics creates and registers filter metrics.
func NewFilterMetrics(namespace, service string) *FilterMetrics {
	m := &FilterMetrics{
		MessagesProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "privacy_messages_processed_total",
			Help:        "Total number of messages processed by privacy filter",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		MessagesExcluded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "privacy_messages_excluded_total",
			Help:        "Total number of messages excluded by privacy filter",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		PIIDetections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "privacy_pii_detections_total",
			Help:        "Total PII detections by type",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"pii_type"}),
		RedactionsApplied: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "privacy_redactions_applied_total",
			Help:        "Total number of redactions applied",
			ConstLabels: prometheus.Labels{"service": service},
		}),
		ProcessingTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   namespace,
			Name:        "privacy_processing_seconds",
			Help:        "Time spent processing messages in seconds",
			Buckets:     []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			ConstLabels: prometheus.Labels{"service": service},
		}),
		FilterErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Name:        "privacy_filter_errors_total",
			Help:        "Total number of filter errors",
			ConstLabels: prometheus.Labels{"service": service},
		}),
	}

	return m
}

// Register registers all metrics with the default Prometheus registry.
func (m *FilterMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.MessagesProcessed,
		m.MessagesExcluded,
		m.PIIDetections,
		m.RedactionsApplied,
		m.ProcessingTime,
		m.FilterErrors,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// NewPrivacyFilter creates a new PrivacyFilter with the given configuration.
func NewPrivacyFilter(config *FilterConfig) (*PrivacyFilter, error) {
	if config == nil {
		config = DefaultFilterConfig()
	}

	// Combine default rules with custom rules.
	rules := DefaultRules()
	rules = append(rules, config.CustomRules...)

	// Build lookup maps for efficient blocklist checking.
	blockedSendersMap := make(map[string]bool)
	for _, sender := range config.BlockedSenders {
		blockedSendersMap[strings.ToLower(sender)] = true
	}

	blockedDomainsMap := make(map[string]bool)
	for _, domain := range config.BlockedDomains {
		blockedDomainsMap[strings.ToLower(domain)] = true
	}

	allowedSendersMap := make(map[string]bool)
	for _, sender := range config.AllowedSenders {
		allowedSendersMap[strings.ToLower(sender)] = true
	}

	allowedDomainsMap := make(map[string]bool)
	for _, domain := range config.AllowedDomains {
		allowedDomainsMap[strings.ToLower(domain)] = true
	}

	return &PrivacyFilter{
		config:              config,
		rules:               rules,
		metrics:             NewFilterMetrics("penfold", "gmail_connector"),
		blockedSendersMap:   blockedSendersMap,
		blockedDomainsMap:   blockedDomainsMap,
		allowedSendersMap:   allowedSendersMap,
		allowedDomainsMap:   allowedDomainsMap,
	}, nil
}

// SetMetrics sets custom metrics for the filter.
func (f *PrivacyFilter) SetMetrics(metrics *FilterMetrics) {
	f.metrics = metrics
}

// FilterMessage processes a message according to the configured sensitivity level.
func (f *PrivacyFilter) FilterMessage(ctx context.Context, msg *Message, tenantID string) (*FilterResult, error) {
	start := time.Now()

	if f.metrics != nil {
		f.metrics.MessagesProcessed.Inc()
	}

	result := &FilterResult{
		Message:    copyMessage(msg),
		PIIDetected: []PIILocation{},
	}

	// Check if message should be excluded based on blocklist.
	if excluded, reason := f.ShouldExclude(msg); excluded {
		result.Excluded = true
		result.ExclusionReason = reason
		result.Message = nil
		result.ProcessingTime = time.Since(start)

		if f.metrics != nil {
			f.metrics.MessagesExcluded.Inc()
			f.metrics.ProcessingTime.Observe(result.ProcessingTime.Seconds())
		}

		// Log audit entry.
		f.logAudit(ctx, msg.ID, tenantID, "excluded", result)

		return result, nil
	}

	// Check if sender is in allowlist (bypass PII filtering).
	if f.isAllowlisted(msg) {
		result.ProcessingTime = time.Since(start)

		if f.metrics != nil {
			f.metrics.ProcessingTime.Observe(result.ProcessingTime.Seconds())
		}

		f.logAudit(ctx, msg.ID, tenantID, "allowlisted", result)
		return result, nil
	}

	// Apply filtering based on sensitivity level.
	switch f.config.SensitivityLevel {
	case SensitivityLow:
		// Only blocklist filtering (already done above).
		// No PII detection at this level.

	case SensitivityMedium:
		// Apply PII detection and redaction.
		piiLocations := f.DetectPII(result.Message.Subject + "\n" + result.Message.Body)
		result.PIIDetected = piiLocations

		if len(piiLocations) > 0 {
			f.applyRedactions(result)
		}

	case SensitivityHigh:
		// Full content filtering - detect PII in all fields.
		var allLocations []PIILocation

		// Process subject.
		subjectPII := f.DetectPII(result.Message.Subject)
		allLocations = append(allLocations, subjectPII...)

		// Process body - keep original offsets for body.
		bodyPII := f.DetectPII(result.Message.Body)

		// Create adjusted locations for PIIDetected (with offsets relative to combined text).
		for _, loc := range bodyPII {
			adjustedLoc := PIILocation{
				Start:      loc.Start + len(result.Message.Subject) + 1,
				End:        loc.End + len(result.Message.Subject) + 1,
				Type:       loc.Type,
				Value:      loc.Value,
				Confidence: loc.Confidence,
			}
			allLocations = append(allLocations, adjustedLoc)
		}

		result.PIIDetected = allLocations

		if len(subjectPII) > 0 || len(bodyPII) > 0 {
			f.applyRedactionsHigh(result, subjectPII, bodyPII)
		}
	}

	result.ProcessingTime = time.Since(start)

	if f.metrics != nil {
		f.metrics.ProcessingTime.Observe(result.ProcessingTime.Seconds())
		if result.RedactionCount > 0 {
			f.metrics.RedactionsApplied.Add(float64(result.RedactionCount))
		}
	}

	f.logAudit(ctx, msg.ID, tenantID, "filtered", result)

	return result, nil
}

// applyRedactions applies redactions for medium sensitivity level.
func (f *PrivacyFilter) applyRedactions(result *FilterResult) {
	combined := result.Message.Subject + "\n" + result.Message.Body
	redacted := f.RedactPII(combined, result.PIIDetected)
	result.RedactionCount = len(result.PIIDetected)

	// Update metrics for PII types.
	if f.metrics != nil {
		for _, loc := range result.PIIDetected {
			f.metrics.PIIDetections.WithLabelValues(loc.Type).Inc()
		}
	}

	// Split back into subject and body.
	parts := strings.SplitN(redacted, "\n", 2)
	result.Message.Subject = parts[0]
	if len(parts) > 1 {
		result.Message.Body = parts[1]
	}
}

// applyRedactionsHigh applies redactions for high sensitivity level.
// bodyPII should have original offsets (relative to body text, not combined text).
func (f *PrivacyFilter) applyRedactionsHigh(result *FilterResult, subjectPII, bodyPII []PIILocation) {
	// Redact subject separately.
	if len(subjectPII) > 0 {
		result.Message.Subject = f.RedactPII(result.Message.Subject, subjectPII)
	}

	// Redact body - bodyPII already has offsets relative to body.
	if len(bodyPII) > 0 {
		result.Message.Body = f.RedactPII(result.Message.Body, bodyPII)
	}

	result.RedactionCount = len(subjectPII) + len(bodyPII)

	// Update metrics for PII types.
	if f.metrics != nil {
		for _, loc := range result.PIIDetected {
			f.metrics.PIIDetections.WithLabelValues(loc.Type).Inc()
		}
	}
}

// DetectPII runs all enabled rules against the text and returns PII locations.
func (f *PrivacyFilter) DetectPII(text string) []PIILocation {
	f.mu.RLock()
	rules := f.rules
	f.mu.RUnlock()

	var allLocations [][]PIILocation
	for _, rule := range rules {
		if rule.Enabled() {
			locations := rule.Detect(text)
			if len(locations) > 0 {
				allLocations = append(allLocations, locations)
			}
		}
	}

	return MergeLocations(allLocations)
}

// RedactPII replaces PII at the specified locations with the configured placeholder.
func (f *PrivacyFilter) RedactPII(text string, locations []PIILocation) string {
	if len(locations) == 0 {
		return text
	}

	// Use the first rule to perform redaction (all use the same logic).
	f.mu.RLock()
	rules := f.rules
	placeholder := f.config.RedactionPlaceholder
	f.mu.RUnlock()

	if len(rules) > 0 {
		return rules[0].Redact(text, locations, placeholder)
	}

	// Fallback manual redaction if no rules exist.
	return manualRedact(text, locations, placeholder)
}

// manualRedact performs redaction without using Rule interface.
func manualRedact(text string, locations []PIILocation, placeholder string) string {
	// Sort locations by start position descending.
	sortedLocs := make([]PIILocation, len(locations))
	copy(sortedLocs, locations)

	for i := 0; i < len(sortedLocs)-1; i++ {
		for j := 0; j < len(sortedLocs)-i-1; j++ {
			if sortedLocs[j].Start < sortedLocs[j+1].Start {
				sortedLocs[j], sortedLocs[j+1] = sortedLocs[j+1], sortedLocs[j]
			}
		}
	}

	result := text
	for _, loc := range sortedLocs {
		if loc.Start >= 0 && loc.End <= len(result) && loc.Start < loc.End {
			result = result[:loc.Start] + placeholder + result[loc.End:]
		}
	}

	return result
}

// ShouldExclude checks if a message should be excluded based on blocklist rules.
func (f *PrivacyFilter) ShouldExclude(msg *Message) (bool, string) {
	if msg == nil {
		return true, "nil message"
	}

	// Check sender blocklist.
	senderLower := strings.ToLower(msg.From)
	if f.blockedSendersMap[senderLower] {
		return true, fmt.Sprintf("sender %s is in blocklist", msg.From)
	}

	// Check domain blocklist.
	senderDomain := extractDomain(msg.From)
	if senderDomain != "" && f.blockedDomainsMap[strings.ToLower(senderDomain)] {
		return true, fmt.Sprintf("sender domain %s is in blocklist", senderDomain)
	}

	return false, ""
}

// isAllowlisted checks if a message sender is in the allowlist.
func (f *PrivacyFilter) isAllowlisted(msg *Message) bool {
	senderLower := strings.ToLower(msg.From)
	if f.allowedSendersMap[senderLower] {
		return true
	}

	senderDomain := extractDomain(msg.From)
	if senderDomain != "" && f.allowedDomainsMap[strings.ToLower(senderDomain)] {
		return true
	}

	return false
}

// extractDomain extracts the domain from an email address.
func extractDomain(email string) string {
	// Handle "Name <email@domain.com>" format.
	if idx := strings.Index(email, "<"); idx != -1 {
		endIdx := strings.Index(email, ">")
		if endIdx > idx {
			email = email[idx+1 : endIdx]
		}
	}

	// Extract domain after @.
	if idx := strings.LastIndex(email, "@"); idx != -1 {
		return email[idx+1:]
	}

	return ""
}

// AddRule adds a custom rule to the filter.
func (f *PrivacyFilter) AddRule(rule Rule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = append(f.rules, rule)
}

// RemoveRule removes a rule by name.
func (f *PrivacyFilter) RemoveRule(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, rule := range f.rules {
		if rule.Name() == name {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return true
		}
	}
	return false
}

// GetRules returns a copy of the current rules.
func (f *PrivacyFilter) GetRules() []Rule {
	f.mu.RLock()
	defer f.mu.RUnlock()

	rules := make([]Rule, len(f.rules))
	copy(rules, f.rules)
	return rules
}

// UpdateBlocklist updates the sender and domain blocklists.
func (f *PrivacyFilter) UpdateBlocklist(senders, domains []string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Rebuild blocklist maps.
	f.blockedSendersMap = make(map[string]bool)
	for _, sender := range senders {
		f.blockedSendersMap[strings.ToLower(sender)] = true
	}

	f.blockedDomainsMap = make(map[string]bool)
	for _, domain := range domains {
		f.blockedDomainsMap[strings.ToLower(domain)] = true
	}

	f.config.BlockedSenders = senders
	f.config.BlockedDomains = domains
}

// UpdateAllowlist updates the sender and domain allowlists.
func (f *PrivacyFilter) UpdateAllowlist(senders, domains []string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.allowedSendersMap = make(map[string]bool)
	for _, sender := range senders {
		f.allowedSendersMap[strings.ToLower(sender)] = true
	}

	f.allowedDomainsMap = make(map[string]bool)
	for _, domain := range domains {
		f.allowedDomainsMap[strings.ToLower(domain)] = true
	}

	f.config.AllowedSenders = senders
	f.config.AllowedDomains = domains
}

// SetSensitivityLevel updates the sensitivity level.
func (f *PrivacyFilter) SetSensitivityLevel(level SensitivityLevel) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config.SensitivityLevel = level
}

// GetConfig returns a copy of the current configuration.
func (f *PrivacyFilter) GetConfig() *FilterConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Return a copy to prevent external modification.
	return &FilterConfig{
		SensitivityLevel:       f.config.SensitivityLevel,
		RedactionPlaceholder:   f.config.RedactionPlaceholder,
		BlockedSenders:         append([]string{}, f.config.BlockedSenders...),
		BlockedDomains:         append([]string{}, f.config.BlockedDomains...),
		AllowedSenders:         append([]string{}, f.config.AllowedSenders...),
		AllowedDomains:         append([]string{}, f.config.AllowedDomains...),
		BlockedSubjectPatterns: append([]string{}, f.config.BlockedSubjectPatterns...),
		SkipAttachments:        f.config.SkipAttachments,
	}
}

// logAudit records an audit entry if an AuditLogger is configured.
func (f *PrivacyFilter) logAudit(ctx context.Context, messageID, tenantID, action string, result *FilterResult) {
	if f.config.AuditLogger == nil {
		return
	}

	// Collect unique PII types.
	piiTypesMap := make(map[string]bool)
	for _, loc := range result.PIIDetected {
		piiTypesMap[loc.Type] = true
	}

	piiTypes := make([]string, 0, len(piiTypesMap))
	for t := range piiTypesMap {
		piiTypes = append(piiTypes, t)
	}

	entry := &AuditEntry{
		ID:               uuid.New().String(),
		MessageID:        messageID,
		TenantID:         tenantID,
		Timestamp:        time.Now(),
		Action:           action,
		SensitivityLevel: f.config.SensitivityLevel.String(),
		PIITypesDetected: piiTypes,
		RedactionCount:   result.RedactionCount,
		Excluded:         result.Excluded,
		ExclusionReason:  result.ExclusionReason,
	}

	// Log asynchronously to avoid blocking message processing.
	go func() {
		if err := f.config.AuditLogger.Log(ctx, entry); err != nil {
			// In production, this would go to a structured logger.
			// For now, we silently ignore audit logging errors.
			if f.metrics != nil {
				f.metrics.FilterErrors.Inc()
			}
		}
	}()
}

// copyMessage creates a deep copy of a message.
func copyMessage(msg *Message) *Message {
	if msg == nil {
		return nil
	}

	copy := &Message{
		ID:         msg.ID,
		ThreadID:   msg.ThreadID,
		From:       msg.From,
		Subject:    msg.Subject,
		Body:       msg.Body,
		ReceivedAt: msg.ReceivedAt,
	}

	if len(msg.To) > 0 {
		copy.To = make([]string, len(msg.To))
		for i, to := range msg.To {
			copy.To[i] = to
		}
	}

	if len(msg.Cc) > 0 {
		copy.Cc = make([]string, len(msg.Cc))
		for i, cc := range msg.Cc {
			copy.Cc[i] = cc
		}
	}

	if len(msg.Labels) > 0 {
		copy.Labels = make([]string, len(msg.Labels))
		for i, label := range msg.Labels {
			copy.Labels[i] = label
		}
	}

	if len(msg.Attachments) > 0 {
		copy.Attachments = make([]AttachmentInfo, len(msg.Attachments))
		for i, att := range msg.Attachments {
			copy.Attachments[i] = att
		}
	}

	return copy
}

// MemoryAuditLogger is a simple in-memory audit logger for testing.
type MemoryAuditLogger struct {
	entries []AuditEntry
	mu      sync.Mutex
}

// NewMemoryAuditLogger creates a new in-memory audit logger.
func NewMemoryAuditLogger() *MemoryAuditLogger {
	return &MemoryAuditLogger{
		entries: make([]AuditEntry, 0),
	}
}

// Log records an audit entry in memory.
func (l *MemoryAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, *entry)
	return nil
}

// GetEntries returns all recorded audit entries.
func (l *MemoryAuditLogger) GetEntries() []AuditEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]AuditEntry, len(l.entries))
	copy(entries, l.entries)
	return entries
}

// Clear removes all recorded entries.
func (l *MemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = l.entries[:0]
}
