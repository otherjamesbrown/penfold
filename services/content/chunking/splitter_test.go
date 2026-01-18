package chunking

import (
	"strings"
	"testing"
)

// =============================================================================
// TextSplitter Tests
// =============================================================================

func TestNewTextSplitter(t *testing.T) {
	splitter := NewTextSplitter()
	if splitter == nil {
		t.Fatal("expected non-nil splitter")
	}
}

// =============================================================================
// SplitBySentence Tests
// =============================================================================

func TestSplitBySentence_EmptyText(t *testing.T) {
	splitter := NewTextSplitter()
	result := splitter.SplitBySentence("")
	if result != nil {
		t.Errorf("expected nil for empty text, got %v", result)
	}
}

func TestSplitBySentence_SingleSentence(t *testing.T) {
	splitter := NewTextSplitter()
	result := splitter.SplitBySentence("This is a single sentence.")
	if len(result) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(result))
	}
	if result[0] != "This is a single sentence." {
		t.Errorf("unexpected sentence: %s", result[0])
	}
}

func TestSplitBySentence_MultipleSentences(t *testing.T) {
	splitter := NewTextSplitter()
	text := "First sentence. Second sentence! Third sentence?"
	result := splitter.SplitBySentence(text)

	if len(result) != 3 {
		t.Errorf("expected 3 sentences, got %d: %v", len(result), result)
	}
}

func TestSplitBySentence_Abbreviations(t *testing.T) {
	splitter := NewTextSplitter()
	text := "Dr. Smith met Mr. Jones. They talked."

	result := splitter.SplitBySentence(text)

	// Should handle abbreviations correctly - "Dr." and "Mr." should not split
	if len(result) != 2 {
		t.Errorf("expected 2 sentences (handling abbreviations), got %d: %v", len(result), result)
	}

	// Check that abbreviations are preserved
	if !strings.Contains(result[0], "Dr.") || !strings.Contains(result[0], "Mr.") {
		t.Errorf("abbreviations should be preserved in first sentence: %s", result[0])
	}
}

func TestSplitBySentence_NoEndPunctuation(t *testing.T) {
	splitter := NewTextSplitter()
	text := "This sentence has no ending"
	result := splitter.SplitBySentence(text)

	if len(result) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(result))
	}
}

func TestSplitBySentence_MultipleSpaces(t *testing.T) {
	splitter := NewTextSplitter()
	text := "First sentence.   Second sentence."
	result := splitter.SplitBySentence(text)

	if len(result) != 2 {
		t.Errorf("expected 2 sentences, got %d: %v", len(result), result)
	}
}

// =============================================================================
// SplitByParagraph Tests
// =============================================================================

func TestSplitByParagraph_EmptyText(t *testing.T) {
	splitter := NewTextSplitter()
	result := splitter.SplitByParagraph("")
	if result != nil {
		t.Errorf("expected nil for empty text, got %v", result)
	}
}

func TestSplitByParagraph_SingleParagraph(t *testing.T) {
	splitter := NewTextSplitter()
	text := "This is a single paragraph without any breaks."
	result := splitter.SplitByParagraph(text)

	if len(result) != 1 {
		t.Errorf("expected 1 paragraph, got %d", len(result))
	}
}

func TestSplitByParagraph_MultipleParagraphs(t *testing.T) {
	splitter := NewTextSplitter()
	text := `First paragraph here.

Second paragraph there.

Third paragraph ends.`

	result := splitter.SplitByParagraph(text)

	if len(result) != 3 {
		t.Errorf("expected 3 paragraphs, got %d: %v", len(result), result)
	}
}

func TestSplitByParagraph_SkipsEmptyLines(t *testing.T) {
	splitter := NewTextSplitter()
	text := `First paragraph.



Second paragraph.`

	result := splitter.SplitByParagraph(text)

	if len(result) != 2 {
		t.Errorf("expected 2 paragraphs, got %d: %v", len(result), result)
	}
}

// =============================================================================
// SplitByHeading Tests
// =============================================================================

func TestSplitByHeading_NoHeadings(t *testing.T) {
	splitter := NewTextSplitter()
	text := "Just regular text without any headings."
	result := splitter.SplitByHeading(text)

	if len(result) != 1 {
		t.Errorf("expected 1 section, got %d", len(result))
	}
	if result[0].Title != "" {
		t.Errorf("expected empty title, got '%s'", result[0].Title)
	}
}

func TestSplitByHeading_MarkdownHeadings(t *testing.T) {
	splitter := NewTextSplitter()
	text := `# Introduction

This is the introduction.

## Methods

This describes the methods.

### Subsection

More details here.`

	result := splitter.SplitByHeading(text)

	// Should have sections for each heading
	if len(result) < 3 {
		t.Errorf("expected at least 3 sections, got %d: %v", len(result), result)
	}

	// Check heading levels
	foundH1 := false
	foundH2 := false
	foundH3 := false
	for _, section := range result {
		switch section.Level {
		case 1:
			foundH1 = true
			if section.Title != "Introduction" {
				t.Errorf("expected title 'Introduction', got '%s'", section.Title)
			}
		case 2:
			foundH2 = true
		case 3:
			foundH3 = true
		}
	}

	if !foundH1 {
		t.Error("expected to find H1 heading")
	}
	if !foundH2 {
		t.Error("expected to find H2 heading")
	}
	if !foundH3 {
		t.Error("expected to find H3 heading")
	}
}

func TestSplitByHeading_ContentBeforeFirstHeading(t *testing.T) {
	splitter := NewTextSplitter()
	text := `Some preamble content here.

# First Heading

Heading content.`

	result := splitter.SplitByHeading(text)

	if len(result) < 2 {
		t.Errorf("expected at least 2 sections, got %d", len(result))
	}

	// First section should be preamble with no title
	if result[0].Title != "" {
		t.Errorf("expected empty title for preamble, got '%s'", result[0].Title)
	}
	if !strings.Contains(result[0].Content, "preamble") {
		t.Error("preamble content should contain 'preamble'")
	}
}

// =============================================================================
// SplitByList Tests
// =============================================================================

func TestSplitByList_NoLists(t *testing.T) {
	splitter := NewTextSplitter()
	text := "Regular text without any lists."
	result := splitter.SplitByList(text)

	if len(result) != 1 {
		t.Errorf("expected 1 block, got %d", len(result))
	}
	if result[0].IsList {
		t.Error("expected non-list block")
	}
}

func TestSplitByList_BulletList(t *testing.T) {
	splitter := NewTextSplitter()
	text := `Some intro text.

- First item
- Second item
- Third item

More text after.`

	result := splitter.SplitByList(text)

	// Should have intro, list, and outro
	listFound := false
	for _, block := range result {
		if block.IsList {
			listFound = true
			if len(block.Items) != 3 {
				t.Errorf("expected 3 list items, got %d: %v", len(block.Items), block.Items)
			}
		}
	}

	if !listFound {
		t.Error("expected to find a list block")
	}
}

func TestSplitByList_NumberedList(t *testing.T) {
	splitter := NewTextSplitter()
	text := `1. First item
2. Second item
3. Third item`

	result := splitter.SplitByList(text)

	if len(result) == 0 {
		t.Fatal("expected at least one block")
	}

	found := false
	for _, block := range result {
		if block.IsList && len(block.Items) == 3 {
			found = true
		}
	}

	if !found {
		t.Error("expected to find numbered list with 3 items")
	}
}

// =============================================================================
// SplitPreservingCodeBlocks Tests
// =============================================================================

func TestSplitPreservingCodeBlocks_NoCode(t *testing.T) {
	splitter := NewTextSplitter()
	text := "Just regular text without code."
	result := splitter.SplitPreservingCodeBlocks(text)

	if len(result) != 1 {
		t.Errorf("expected 1 block, got %d", len(result))
	}
	if result[0].IsCode {
		t.Error("expected non-code block")
	}
}

func TestSplitPreservingCodeBlocks_FencedCode(t *testing.T) {
	splitter := NewTextSplitter()
	text := "Some text before.\n\n```python\nprint('hello')\n```\n\nText after."

	result := splitter.SplitPreservingCodeBlocks(text)

	codeFound := false
	for _, block := range result {
		if block.IsCode {
			codeFound = true
			if block.Language != "python" {
				t.Errorf("expected language 'python', got '%s'", block.Language)
			}
		}
	}

	if !codeFound {
		t.Error("expected to find code block")
	}
}

func TestSplitPreservingCodeBlocks_DetectsLanguage(t *testing.T) {
	splitter := NewTextSplitter()
	text := "```javascript\nconsole.log('test');\n```"

	result := splitter.SplitPreservingCodeBlocks(text)

	if len(result) == 0 {
		t.Fatal("expected at least one block")
	}

	found := false
	for _, block := range result {
		if block.IsCode && block.Language == "javascript" {
			found = true
		}
	}

	if !found {
		t.Error("expected to find javascript code block")
	}
}

// =============================================================================
// SplitStructured Tests
// =============================================================================

func TestSplitStructured_Default(t *testing.T) {
	splitter := NewTextSplitter()
	text := `# Heading

First paragraph.

- List item 1
- List item 2

` + "```go\nfmt.Println(\"hello\")\n```"

	result := splitter.SplitStructured(text, DefaultStructuredSplitOptions())

	// Should have multiple block types
	types := make(map[BlockType]bool)
	for _, block := range result {
		types[block.Type] = true
	}

	if !types[BlockTypeHeading] {
		t.Error("expected heading block")
	}
	// Paragraph might be merged or not present depending on structure
}

func TestSplitStructured_DisableHeadings(t *testing.T) {
	splitter := NewTextSplitter()
	text := `# Heading

Paragraph content.`

	options := StructuredSplitOptions{
		PreserveHeadings: false,
	}

	result := splitter.SplitStructured(text, options)

	for _, block := range result {
		if block.Type == BlockTypeHeading {
			t.Error("expected no heading blocks when PreserveHeadings is false")
		}
	}
}

// =============================================================================
// Metadata Boundary Tests
// =============================================================================

func TestPreserveMetadataBoundaries(t *testing.T) {
	content := "Test content"
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	result := PreserveMetadataBoundaries(content, metadata)

	if !strings.Contains(result, "<!--META:") {
		t.Error("expected META start marker")
	}
	if !strings.Contains(result, "<!--/META-->") {
		t.Error("expected META end marker")
	}
	if !strings.Contains(result, "key1=value1") {
		t.Error("expected key1=value1")
	}
	if !strings.Contains(result, "key2=value2") {
		t.Error("expected key2=value2")
	}
	if !strings.Contains(result, "Test content") {
		t.Error("expected content to be preserved")
	}
}

func TestPreserveMetadataBoundaries_EmptyMetadata(t *testing.T) {
	content := "Test content"
	result := PreserveMetadataBoundaries(content, nil)

	if result != content {
		t.Errorf("expected unchanged content, got '%s'", result)
	}
}

func TestExtractMetadataBoundaries(t *testing.T) {
	original := "Test content"
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	wrapped := PreserveMetadataBoundaries(original, metadata)
	extractedContent, extractedMetadata := ExtractMetadataBoundaries(wrapped)

	if extractedContent != original {
		t.Errorf("expected content '%s', got '%s'", original, extractedContent)
	}
	if extractedMetadata["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %s", extractedMetadata["key1"])
	}
	if extractedMetadata["key2"] != "value2" {
		t.Errorf("expected key2=value2, got %s", extractedMetadata["key2"])
	}
}

func TestExtractMetadataBoundaries_NoMetadata(t *testing.T) {
	content := "Plain content without markers"
	extractedContent, extractedMetadata := ExtractMetadataBoundaries(content)

	if extractedContent != content {
		t.Errorf("expected unchanged content, got '%s'", extractedContent)
	}
	if len(extractedMetadata) != 0 {
		t.Error("expected empty metadata")
	}
}

// =============================================================================
// Section Tests
// =============================================================================

func TestSection_Fields(t *testing.T) {
	section := Section{
		Title:    "Test Title",
		Content:  "Test Content",
		Level:    2,
		StartPos: 10,
		EndPos:   50,
	}

	if section.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got '%s'", section.Title)
	}
	if section.Level != 2 {
		t.Errorf("expected level 2, got %d", section.Level)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkSplitBySentence(b *testing.B) {
	splitter := NewTextSplitter()
	text := "First sentence. Second sentence. Third sentence. Fourth sentence. Fifth sentence."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitter.SplitBySentence(text)
	}
}

func BenchmarkSplitByParagraph(b *testing.B) {
	splitter := NewTextSplitter()
	text := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n\nFourth paragraph."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitter.SplitByParagraph(text)
	}
}

func BenchmarkSplitByHeading(b *testing.B) {
	splitter := NewTextSplitter()
	text := "# Heading 1\n\nContent.\n\n## Heading 2\n\nMore content.\n\n### Heading 3\n\nEven more."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitter.SplitByHeading(text)
	}
}

func BenchmarkSplitPreservingCodeBlocks(b *testing.B) {
	splitter := NewTextSplitter()
	text := "Text before.\n\n```python\nprint('hello')\n```\n\nText after."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitter.SplitPreservingCodeBlocks(text)
	}
}
