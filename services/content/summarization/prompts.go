package summarization

import (
	"fmt"
	"strings"
)

// PromptTemplate represents a reusable prompt template.
type PromptTemplate struct {
	// Name identifies the template.
	Name string

	// Template is the prompt template with placeholders.
	Template string

	// Description explains the template's purpose.
	Description string
}

// Predefined prompt templates for various summarization tasks.
var (
	// SummaryPrompt is the default summarization prompt.
	SummaryPrompt = PromptTemplate{
		Name: "summary",
		Template: `Summarize the following content in a clear, concise manner.

Content:
{{.Content}}

Requirements:
- Target length: approximately {{.TargetWords}} words
- Style: {{.Style}}
- Preserve key facts and important details
- Maintain the original meaning and tone
- Do not add information not present in the original

Summary:`,
		Description: "General-purpose summarization prompt",
	}

	// BulletPointPrompt generates bullet-point summaries.
	BulletPointPrompt = PromptTemplate{
		Name: "bullet_points",
		Template: `Extract the key points from the following content as a bullet-point list.

Content:
{{.Content}}

Requirements:
- Extract {{.NumPoints}} key points
- Each point should be a complete, standalone statement
- Order points by importance
- Use clear, concise language
- Start each point with an action verb when possible

Key Points:`,
		Description: "Extracts content as bullet points",
	}

	// TLDRPrompt generates very brief summaries.
	TLDRPrompt = PromptTemplate{
		Name: "tldr",
		Template: `Provide a TL;DR (Too Long; Didn't Read) summary of the following content in one to two sentences.

Content:
{{.Content}}

TL;DR:`,
		Description: "One-line TL;DR summary",
	}

	// ExecutiveSummaryPrompt generates executive-style summaries.
	ExecutiveSummaryPrompt = PromptTemplate{
		Name: "executive_summary",
		Template: `Write an executive summary of the following content for a business audience.

Content:
{{.Content}}

Requirements:
- Begin with the main conclusion or recommendation
- Highlight key findings, decisions, or action items
- Include relevant metrics or data points
- Use professional, clear language
- Target length: {{.TargetWords}} words
- Structure for quick scanning

Executive Summary:`,
		Description: "Business-focused executive summary",
	}

	// KeyPointsPrompt extracts key facts and takeaways.
	KeyPointsPrompt = PromptTemplate{
		Name: "key_points",
		Template: `Analyze the following content and extract the most important facts, findings, and takeaways.

Content:
{{.Content}}

Extract:
1. Main Topic: What is the primary subject?
2. Key Facts: List 3-5 most important facts or findings
3. Conclusions: What are the main conclusions or outcomes?
4. Action Items: Any recommended actions or next steps?

Analysis:`,
		Description: "Structured key points extraction",
	}

	// TechnicalSummaryPrompt generates technical summaries.
	TechnicalSummaryPrompt = PromptTemplate{
		Name: "technical_summary",
		Template: `Summarize the following technical content, preserving important technical details.

Content:
{{.Content}}

Requirements:
- Preserve technical terminology and specifications
- Include relevant numbers, metrics, and measurements
- Maintain accuracy of technical claims
- Target length: {{.TargetWords}} words
- Suitable for a technical audience

Technical Summary:`,
		Description: "Technical content summarization",
	}

	// HierarchicalPrompt generates multi-level summaries.
	HierarchicalPrompt = PromptTemplate{
		Name: "hierarchical",
		Template: `Create a multi-level summary of the following content.

Content:
{{.Content}}

Key Points for context:
{{.KeyPoints}}

Provide summaries at three levels:

One-Line: A single sentence capturing the essence (max 25 words)

Paragraph: A brief paragraph summary (50-75 words)

Full: A comprehensive summary (150-200 words)

Format your response exactly as:
One-Line: [your one-line summary]

Paragraph: [your paragraph summary]

Full: [your full summary]`,
		Description: "Multi-level hierarchical summaries",
	}

	// HybridPrompt for hybrid summarization approach.
	HybridPrompt = PromptTemplate{
		Name: "hybrid",
		Template: `Based on the following extracted content and key points, create a polished summary.

Extracted Content:
{{.ExtractedContent}}

Key Points:
{{.KeyPoints}}

Requirements:
- Synthesize the extracted content into a coherent summary
- Ensure all key points are represented
- Target length: {{.TargetWords}} words
- Style: {{.Style}}
- Improve flow and readability while preserving accuracy

Summary:`,
		Description: "Hybrid summarization with extracted input",
	}

	// EntityPreservingPrompt ensures entities are preserved.
	EntityPreservingPrompt = PromptTemplate{
		Name: "entity_preserving",
		Template: `Summarize the following content while preserving all named entities (people, organizations, locations, dates, etc.).

Content:
{{.Content}}

Requirements:
- Preserve all names of people, organizations, and places exactly as written
- Keep all specific dates, times, and numbers
- Maintain accuracy of any quoted statements
- Target length: {{.TargetWords}} words

Summary:`,
		Description: "Summary preserving named entities",
	}

	// ComparativeSummaryPrompt for comparing multiple documents.
	ComparativeSummaryPrompt = PromptTemplate{
		Name: "comparative",
		Template: `Compare and summarize the following documents, highlighting similarities and differences.

Document 1:
{{.Document1}}

Document 2:
{{.Document2}}

Provide:
1. Common Themes: What do both documents share?
2. Key Differences: Where do they diverge?
3. Combined Summary: A unified summary of both

Analysis:`,
		Description: "Comparative summary of multiple documents",
	}
)

// PromptParams holds parameters for prompt template filling.
type PromptParams struct {
	Content          string
	ExtractedContent string
	KeyPoints        []string
	TargetWords      int
	NumPoints        int
	Style            string
	Document1        string
	Document2        string
}

// FillTemplate fills a prompt template with the given parameters.
func FillTemplate(template PromptTemplate, params PromptParams) string {
	result := template.Template

	// Replace placeholders
	result = strings.ReplaceAll(result, "{{.Content}}", params.Content)
	result = strings.ReplaceAll(result, "{{.ExtractedContent}}", params.ExtractedContent)
	result = strings.ReplaceAll(result, "{{.TargetWords}}", fmt.Sprintf("%d", params.TargetWords))
	result = strings.ReplaceAll(result, "{{.NumPoints}}", fmt.Sprintf("%d", params.NumPoints))
	result = strings.ReplaceAll(result, "{{.Style}}", params.Style)
	result = strings.ReplaceAll(result, "{{.Document1}}", params.Document1)
	result = strings.ReplaceAll(result, "{{.Document2}}", params.Document2)

	// Format key points as bullet list
	if len(params.KeyPoints) > 0 {
		keyPointsStr := formatKeyPointsList(params.KeyPoints)
		result = strings.ReplaceAll(result, "{{.KeyPoints}}", keyPointsStr)
	} else {
		result = strings.ReplaceAll(result, "{{.KeyPoints}}", "(none provided)")
	}

	return result
}

// formatKeyPointsList formats key points as a bullet list.
func formatKeyPointsList(points []string) string {
	var builder strings.Builder
	for i, point := range points {
		builder.WriteString(fmt.Sprintf("- %s", point))
		if i < len(points)-1 {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

// BuildSummaryPrompt constructs a prompt for the given summary request.
func BuildSummaryPrompt(req *SummaryRequest) string {
	params := PromptParams{
		Content:     truncateForPrompt(req.Content, 8000), // Limit content for prompt
		TargetWords: tokensToWords(req.TargetTokens()),
		Style:       formatStyleDescription(req.Format),
	}

	// Select appropriate template based on format
	var template PromptTemplate
	switch req.Format {
	case SummaryFormatBulletPoints:
		params.NumPoints = 5
		template = BulletPointPrompt
	case SummaryFormatKeyPoints:
		template = KeyPointsPrompt
	case SummaryFormatExecutive:
		template = ExecutiveSummaryPrompt
	default:
		template = SummaryPrompt
	}

	// Use TL;DR for one-line summaries
	if req.Length == SummaryLengthOneLine {
		template = TLDRPrompt
	}

	// Use entity-preserving if requested
	if req.PreserveEntities || req.PreserveFacts {
		template = EntityPreservingPrompt
	}

	return FillTemplate(template, params)
}

// BuildHybridPrompt constructs a prompt for hybrid summarization.
func BuildHybridPrompt(req *SummaryRequest, extractedContent string, keyPoints []string) string {
	params := PromptParams{
		Content:          req.Content,
		ExtractedContent: extractedContent,
		KeyPoints:        keyPoints,
		TargetWords:      tokensToWords(req.TargetTokens()),
		Style:            formatStyleDescription(req.Format),
	}

	return FillTemplate(HybridPrompt, params)
}

// BuildHierarchicalPrompt constructs a prompt for hierarchical summarization.
func BuildHierarchicalPrompt(content string, keyPoints []string) string {
	params := PromptParams{
		Content:   truncateForPrompt(content, 6000),
		KeyPoints: keyPoints,
	}

	return FillTemplate(HierarchicalPrompt, params)
}

// BuildComparativePrompt constructs a prompt for comparing documents.
func BuildComparativePrompt(doc1, doc2 string) string {
	params := PromptParams{
		Document1: truncateForPrompt(doc1, 4000),
		Document2: truncateForPrompt(doc2, 4000),
	}

	return FillTemplate(ComparativeSummaryPrompt, params)
}

// Helper functions

// truncateForPrompt truncates content to fit within prompt limits.
func truncateForPrompt(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	// Truncate at a sentence boundary if possible
	truncated := content[:maxChars]
	lastPeriod := strings.LastIndex(truncated, ". ")
	if lastPeriod > maxChars/2 {
		truncated = truncated[:lastPeriod+1]
	}

	return truncated + " [...]"
}

// tokensToWords converts token count to approximate word count.
func tokensToWords(tokens int) int {
	// Roughly 0.75 words per token
	return int(float64(tokens) * 0.75)
}

// formatStyleDescription converts format enum to description.
func formatStyleDescription(format SummaryFormat) string {
	switch format {
	case SummaryFormatProse:
		return "clear, flowing prose"
	case SummaryFormatBulletPoints:
		return "bullet points"
	case SummaryFormatKeyPoints:
		return "key facts and takeaways"
	case SummaryFormatExecutive:
		return "executive briefing style"
	default:
		return "clear and concise"
	}
}

// PromptBuilder provides a fluent interface for building prompts.
type PromptBuilder struct {
	parts []string
}

// NewPromptBuilder creates a new prompt builder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{parts: make([]string, 0)}
}

// WithSystemInstruction adds a system instruction.
func (b *PromptBuilder) WithSystemInstruction(instruction string) *PromptBuilder {
	b.parts = append(b.parts, fmt.Sprintf("System: %s\n", instruction))
	return b
}

// WithContext adds context information.
func (b *PromptBuilder) WithContext(context string) *PromptBuilder {
	b.parts = append(b.parts, fmt.Sprintf("Context:\n%s\n", context))
	return b
}

// WithContent adds the main content.
func (b *PromptBuilder) WithContent(content string) *PromptBuilder {
	b.parts = append(b.parts, fmt.Sprintf("Content:\n%s\n", content))
	return b
}

// WithRequirements adds requirements section.
func (b *PromptBuilder) WithRequirements(requirements []string) *PromptBuilder {
	b.parts = append(b.parts, "Requirements:")
	for _, req := range requirements {
		b.parts = append(b.parts, fmt.Sprintf("- %s", req))
	}
	b.parts = append(b.parts, "")
	return b
}

// WithOutputLabel adds an output label.
func (b *PromptBuilder) WithOutputLabel(label string) *PromptBuilder {
	b.parts = append(b.parts, fmt.Sprintf("%s:", label))
	return b
}

// Build constructs the final prompt.
func (b *PromptBuilder) Build() string {
	return strings.Join(b.parts, "\n")
}

// QuickSummaryPrompt generates a simple summary prompt.
func QuickSummaryPrompt(content string, targetWords int) string {
	return NewPromptBuilder().
		WithSystemInstruction("You are a helpful assistant that creates clear, accurate summaries.").
		WithContent(content).
		WithRequirements([]string{
			fmt.Sprintf("Target length: %d words", targetWords),
			"Preserve key information",
			"Use clear, concise language",
		}).
		WithOutputLabel("Summary").
		Build()
}

// QuickBulletPointsPrompt generates a bullet points prompt.
func QuickBulletPointsPrompt(content string, numPoints int) string {
	return NewPromptBuilder().
		WithSystemInstruction("Extract key points as a bullet-point list.").
		WithContent(content).
		WithRequirements([]string{
			fmt.Sprintf("Extract exactly %d key points", numPoints),
			"Order by importance",
			"Each point should be self-contained",
		}).
		WithOutputLabel("Key Points").
		Build()
}

// QuickTLDRPrompt generates a TL;DR prompt.
func QuickTLDRPrompt(content string) string {
	return fmt.Sprintf(`Summarize this in one sentence:

%s

TL;DR:`, truncateForPrompt(content, 4000))
}
