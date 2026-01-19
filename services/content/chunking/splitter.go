// Package chunking provides text splitting utilities for chunking strategies.
package chunking

import (
	"regexp"
	"strings"
)

// TextSplitter provides utilities for splitting text at various boundaries.
type TextSplitter struct {
	// Compiled regex patterns for efficiency
	sentencePattern   *regexp.Regexp
	paragraphPattern  *regexp.Regexp
	headingPattern    *regexp.Regexp
	listItemPattern   *regexp.Regexp
	codeBlockPattern  *regexp.Regexp
	markdownHeading   *regexp.Regexp
	htmlHeading       *regexp.Regexp
}

// NewTextSplitter creates a new text splitter with compiled patterns.
func NewTextSplitter() *TextSplitter {
	return &TextSplitter{
		// Sentence endings: period, exclamation, question mark followed by space or end
		sentencePattern: regexp.MustCompile(`[.!?]+(?:\s+|$)`),

		// Paragraphs: two or more newlines
		paragraphPattern: regexp.MustCompile(`\n\s*\n`),

		// Generic section/heading pattern
		headingPattern: regexp.MustCompile(`(?m)^#+\s+.*$|^[A-Z][^.!?\n]*:?\s*$`),

		// List items: bullet points or numbered lists
		listItemPattern: regexp.MustCompile(`(?m)^[\s]*[-*+]\s+|^[\s]*\d+\.\s+`),

		// Code blocks (fenced or indented)
		codeBlockPattern: regexp.MustCompile("(?s)```.*?```|(?m)^(?: {4}|\\t).*$"),

		// Markdown headings
		markdownHeading: regexp.MustCompile(`(?m)^#{1,6}\s+(.*)$`),

		// HTML headings
		htmlHeading: regexp.MustCompile(`(?i)<h[1-6][^>]*>(.*?)</h[1-6]>`),
	}
}

// SplitBySentence splits text into sentences.
func (s *TextSplitter) SplitBySentence(text string) []string {
	if text == "" {
		return nil
	}

	// Handle common abbreviations that shouldn't be split
	text = s.protectAbbreviations(text)

	// Find sentence boundaries
	matches := s.sentencePattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []string{strings.TrimSpace(text)}
	}

	var sentences []string
	lastEnd := 0

	for _, match := range matches {
		sentence := strings.TrimSpace(text[lastEnd:match[1]])
		if sentence != "" {
			// Restore protected abbreviations
			sentence = s.restoreAbbreviations(sentence)
			sentences = append(sentences, sentence)
		}
		lastEnd = match[1]
	}

	// Add any remaining text
	if lastEnd < len(text) {
		remaining := strings.TrimSpace(text[lastEnd:])
		if remaining != "" {
			remaining = s.restoreAbbreviations(remaining)
			sentences = append(sentences, remaining)
		}
	}

	return sentences
}

// Common abbreviations that shouldn't trigger sentence splits
var abbreviations = []string{
	"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.",
	"Inc.", "Ltd.", "Corp.", "Co.",
	"vs.", "etc.", "e.g.", "i.e.",
	"Jan.", "Feb.", "Mar.", "Apr.", "Jun.", "Jul.", "Aug.", "Sep.", "Oct.", "Nov.", "Dec.",
	"St.", "Ave.", "Blvd.", "Rd.",
	"No.", "Vol.", "pp.", "pg.",
	"U.S.", "U.K.", "E.U.",
}

// protectAbbreviations replaces periods in abbreviations with a placeholder.
func (s *TextSplitter) protectAbbreviations(text string) string {
	for _, abbr := range abbreviations {
		protected := strings.ReplaceAll(abbr, ".", "\x00")
		text = strings.ReplaceAll(text, abbr, protected)
	}
	return text
}

// restoreAbbreviations restores the original abbreviations.
func (s *TextSplitter) restoreAbbreviations(text string) string {
	return strings.ReplaceAll(text, "\x00", ".")
}

// SplitByParagraph splits text into paragraphs.
func (s *TextSplitter) SplitByParagraph(text string) []string {
	if text == "" {
		return nil
	}

	// Split on double newlines (or more)
	parts := s.paragraphPattern.Split(text, -1)

	var paragraphs []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}

	return paragraphs
}

// SplitByHeading splits text at heading boundaries.
func (s *TextSplitter) SplitByHeading(text string) []Section {
	if text == "" {
		return nil
	}

	var sections []Section

	// Try markdown headings first
	mdHeadings := s.markdownHeading.FindAllStringSubmatchIndex(text, -1)
	if len(mdHeadings) > 0 {
		return s.splitByHeadingMatches(text, mdHeadings, true)
	}

	// Try HTML headings
	htmlHeadings := s.htmlHeading.FindAllStringSubmatchIndex(text, -1)
	if len(htmlHeadings) > 0 {
		return s.splitByHeadingMatches(text, htmlHeadings, false)
	}

	// No headings found, return whole text as one section
	sections = append(sections, Section{
		Title:   "",
		Content: text,
		Level:   0,
	})

	return sections
}

// Section represents a document section with title and content.
type Section struct {
	// Title is the section heading text
	Title string

	// Content is the section body text
	Content string

	// Level is the heading level (1-6 for H1-H6)
	Level int

	// StartPos is the starting position in the original text
	StartPos int

	// EndPos is the ending position in the original text
	EndPos int
}

// splitByHeadingMatches extracts sections based on heading match indices.
func (s *TextSplitter) splitByHeadingMatches(text string, matches [][]int, isMarkdown bool) []Section {
	var sections []Section

	// Add content before first heading if any
	if len(matches) > 0 && matches[0][0] > 0 {
		preamble := strings.TrimSpace(text[:matches[0][0]])
		if preamble != "" {
			sections = append(sections, Section{
				Title:    "",
				Content:  preamble,
				Level:    0,
				StartPos: 0,
				EndPos:   matches[0][0],
			})
		}
	}

	for i, match := range matches {
		// Extract heading
		headingStart := match[0]
		headingEnd := match[1]
		heading := text[headingStart:headingEnd]

		// Determine level
		level := s.extractHeadingLevel(heading, isMarkdown)

		// Extract title
		title := s.extractHeadingTitle(heading, isMarkdown)

		// Find content end (start of next heading or end of text)
		contentEnd := len(text)
		if i < len(matches)-1 {
			contentEnd = matches[i+1][0]
		}

		// Extract content after heading
		contentStart := headingEnd
		content := strings.TrimSpace(text[contentStart:contentEnd])

		sections = append(sections, Section{
			Title:    title,
			Content:  content,
			Level:    level,
			StartPos: headingStart,
			EndPos:   contentEnd,
		})
	}

	return sections
}

// extractHeadingLevel determines the heading level.
func (s *TextSplitter) extractHeadingLevel(heading string, isMarkdown bool) int {
	if isMarkdown {
		// Count # characters
		level := 0
		for _, c := range heading {
			if c == '#' {
				level++
			} else {
				break
			}
		}
		return level
	}

	// HTML heading - extract number from h1-h6
	htmlLevelPattern := regexp.MustCompile(`(?i)<h([1-6])`)
	match := htmlLevelPattern.FindStringSubmatch(heading)
	if len(match) > 1 {
		return int(match[1][0] - '0')
	}

	return 1
}

// extractHeadingTitle extracts the title text from a heading.
func (s *TextSplitter) extractHeadingTitle(heading string, isMarkdown bool) string {
	if isMarkdown {
		// Remove leading # characters and whitespace
		title := strings.TrimLeft(heading, "#")
		return strings.TrimSpace(title)
	}

	// HTML heading - extract inner text
	htmlTextPattern := regexp.MustCompile(`(?i)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	match := htmlTextPattern.FindStringSubmatch(heading)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	return strings.TrimSpace(heading)
}

// SplitByList splits text while preserving list items.
func (s *TextSplitter) SplitByList(text string) []ListBlock {
	if text == "" {
		return nil
	}

	var blocks []ListBlock
	lines := strings.Split(text, "\n")
	var currentBlock ListBlock
	var currentItems []string
	inList := false

	for _, line := range lines {
		isListItem := s.listItemPattern.MatchString(line)

		if isListItem {
			if !inList {
				// Starting a new list
				if currentBlock.Content != "" {
					blocks = append(blocks, currentBlock)
				}
				currentBlock = ListBlock{IsList: true}
				currentItems = nil
				inList = true
			}
			// Clean up the list item marker
			item := s.listItemPattern.ReplaceAllString(line, "")
			currentItems = append(currentItems, strings.TrimSpace(item))
		} else {
			if inList {
				// Ending a list
				currentBlock.Items = currentItems
				currentBlock.Content = strings.Join(currentItems, "\n")
				blocks = append(blocks, currentBlock)
				currentBlock = ListBlock{}
				currentItems = nil
				inList = false
			}
			// Regular content
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				if currentBlock.Content != "" {
					currentBlock.Content += "\n"
				}
				currentBlock.Content += trimmed
			}
		}
	}

	// Handle remaining content
	if inList {
		currentBlock.Items = currentItems
		currentBlock.Content = strings.Join(currentItems, "\n")
	}
	if currentBlock.Content != "" {
		blocks = append(blocks, currentBlock)
	}

	return blocks
}

// ListBlock represents either a list or non-list block of text.
type ListBlock struct {
	// IsList indicates if this block is a list
	IsList bool

	// Items contains individual list items (if IsList is true)
	Items []string

	// Content is the full text content
	Content string
}

// SplitPreservingCodeBlocks splits text while keeping code blocks intact.
func (s *TextSplitter) SplitPreservingCodeBlocks(text string) []CodeBlock {
	if text == "" {
		return nil
	}

	var blocks []CodeBlock

	// Find all code blocks
	matches := s.codeBlockPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []CodeBlock{{
			IsCode:  false,
			Content: text,
		}}
	}

	lastEnd := 0
	for _, match := range matches {
		// Add text before code block
		if match[0] > lastEnd {
			before := text[lastEnd:match[0]]
			if strings.TrimSpace(before) != "" {
				blocks = append(blocks, CodeBlock{
					IsCode:  false,
					Content: before,
				})
			}
		}

		// Add code block
		code := text[match[0]:match[1]]
		language := s.detectCodeLanguage(code)
		blocks = append(blocks, CodeBlock{
			IsCode:   true,
			Content:  code,
			Language: language,
		})

		lastEnd = match[1]
	}

	// Add remaining text
	if lastEnd < len(text) {
		remaining := text[lastEnd:]
		if strings.TrimSpace(remaining) != "" {
			blocks = append(blocks, CodeBlock{
				IsCode:  false,
				Content: remaining,
			})
		}
	}

	return blocks
}

// CodeBlock represents a code or non-code block of text.
type CodeBlock struct {
	// IsCode indicates if this block is code
	IsCode bool

	// Content is the block content
	Content string

	// Language is the detected or specified programming language
	Language string
}

// detectCodeLanguage attempts to detect the language from a fenced code block.
func (s *TextSplitter) detectCodeLanguage(code string) string {
	if !strings.HasPrefix(code, "```") {
		return ""
	}

	// Extract language from ```language
	firstLine := strings.SplitN(code, "\n", 2)[0]
	lang := strings.TrimPrefix(firstLine, "```")
	return strings.TrimSpace(lang)
}

// SplitStructured combines multiple splitting strategies for structured documents.
func (s *TextSplitter) SplitStructured(text string, options StructuredSplitOptions) []StructuredBlock {
	if text == "" {
		return nil
	}

	var blocks []StructuredBlock

	// First, split by headings if requested
	if options.PreserveHeadings {
		sections := s.SplitByHeading(text)
		for _, section := range sections {
			blocks = append(blocks, s.processSection(section, options)...)
		}
		return blocks
	}

	// Otherwise, process the text as a single section
	section := Section{
		Title:   "",
		Content: text,
		Level:   0,
	}
	return s.processSection(section, options)
}

// processSection processes a section into structured blocks.
func (s *TextSplitter) processSection(section Section, options StructuredSplitOptions) []StructuredBlock {
	var blocks []StructuredBlock

	// Add section header if it has a title
	if section.Title != "" {
		blocks = append(blocks, StructuredBlock{
			Type:    BlockTypeHeading,
			Content: section.Title,
			Level:   section.Level,
		})
	}

	content := section.Content
	if content == "" {
		return blocks
	}

	// Handle code blocks
	if options.PreserveCodeBlocks {
		codeBlocks := s.SplitPreservingCodeBlocks(content)
		for _, cb := range codeBlocks {
			if cb.IsCode {
				blocks = append(blocks, StructuredBlock{
					Type:     BlockTypeCode,
					Content:  cb.Content,
					Language: cb.Language,
				})
			} else {
				// Process non-code content
				blocks = append(blocks, s.processTextContent(cb.Content, options)...)
			}
		}
		return blocks
	}

	// Process as regular text
	return append(blocks, s.processTextContent(content, options)...)
}

// processTextContent processes text content into blocks.
func (s *TextSplitter) processTextContent(text string, options StructuredSplitOptions) []StructuredBlock {
	var blocks []StructuredBlock

	// Handle lists
	if options.PreserveLists {
		listBlocks := s.SplitByList(text)
		for _, lb := range listBlocks {
			if lb.IsList {
				blocks = append(blocks, StructuredBlock{
					Type:    BlockTypeList,
					Content: lb.Content,
					Items:   lb.Items,
				})
			} else {
				// Split non-list content by paragraphs
				paras := s.SplitByParagraph(lb.Content)
				for _, para := range paras {
					blocks = append(blocks, StructuredBlock{
						Type:    BlockTypeParagraph,
						Content: para,
					})
				}
			}
		}
		return blocks
	}

	// Just split by paragraphs
	paras := s.SplitByParagraph(text)
	for _, para := range paras {
		blocks = append(blocks, StructuredBlock{
			Type:    BlockTypeParagraph,
			Content: para,
		})
	}

	return blocks
}

// StructuredSplitOptions configures structured splitting behavior.
type StructuredSplitOptions struct {
	PreserveHeadings   bool
	PreserveLists      bool
	PreserveCodeBlocks bool
}

// DefaultStructuredSplitOptions returns default options.
func DefaultStructuredSplitOptions() StructuredSplitOptions {
	return StructuredSplitOptions{
		PreserveHeadings:   true,
		PreserveLists:      true,
		PreserveCodeBlocks: true,
	}
}

// BlockType identifies the type of structured block.
type BlockType string

const (
	BlockTypeHeading   BlockType = "heading"
	BlockTypeParagraph BlockType = "paragraph"
	BlockTypeList      BlockType = "list"
	BlockTypeCode      BlockType = "code"
)

// StructuredBlock represents a typed block of content.
type StructuredBlock struct {
	// Type identifies the block type
	Type BlockType

	// Content is the block text
	Content string

	// Level is the heading level (for heading blocks)
	Level int

	// Language is the code language (for code blocks)
	Language string

	// Items are the list items (for list blocks)
	Items []string
}

// PreserveMetadataBoundaries wraps content with boundary markers.
// This helps maintain metadata associations during chunking.
func PreserveMetadataBoundaries(content string, metadata map[string]string) string {
	if len(metadata) == 0 {
		return content
	}

	var builder strings.Builder

	// Start boundary
	builder.WriteString("<!--META:")
	first := true
	for k, v := range metadata {
		if !first {
			builder.WriteString(";")
		}
		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(v)
		first = false
	}
	builder.WriteString("-->\n")

	builder.WriteString(content)

	// End boundary
	builder.WriteString("\n<!--/META-->")

	return builder.String()
}

// ExtractMetadataBoundaries extracts metadata from boundary markers.
func ExtractMetadataBoundaries(content string) (string, map[string]string) {
	metadata := make(map[string]string)

	// Find start boundary
	startPattern := regexp.MustCompile(`<!--META:(.*?)-->\n?`)
	startMatch := startPattern.FindStringSubmatch(content)

	if len(startMatch) > 1 {
		// Parse metadata
		pairs := strings.Split(startMatch[1], ";")
		for _, pair := range pairs {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				metadata[kv[0]] = kv[1]
			}
		}

		// Remove boundaries
		content = startPattern.ReplaceAllString(content, "")
	}

	// Remove end boundary
	endPattern := regexp.MustCompile(`\n?<!--/META-->`)
	content = endPattern.ReplaceAllString(content, "")

	return content, metadata
}
