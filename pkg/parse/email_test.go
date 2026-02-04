package parse

import (
	"strings"
	"testing"
)

// TestParseEmailBody_PlainText tests passthrough of plain text.
func TestParseEmailBody_PlainText(t *testing.T) {
	plainText := "This is a simple plain text email.\nIt has multiple lines.\nNo HTML here."

	result := ParseEmailBody(plainText, "")

	if result.CleanBody != plainText {
		t.Errorf("Expected plain text to pass through unchanged.\nGot: %s\nWant: %s", result.CleanBody, plainText)
	}

	if result.QuotedReplyDetected {
		t.Error("Expected no quoted reply detection for simple plain text")
	}
}

// TestParseEmailBody_EmptyBody tests handling of empty body.
func TestParseEmailBody_EmptyBody(t *testing.T) {
	result := ParseEmailBody("", "")

	if result.CleanBody != "" {
		t.Errorf("Expected empty string for empty body, got: %s", result.CleanBody)
	}

	if result.NewContent != "" {
		t.Errorf("Expected empty NewContent, got: %s", result.NewContent)
	}

	if result.QuotedReplyDetected {
		t.Error("Expected no quoted reply detection for empty body")
	}
}

// TestHtmlToText_BasicTags tests basic HTML tag stripping.
func TestHtmlToText_BasicTags(t *testing.T) {
	html := "<p>Hello <b>world</b>!</p><p>This is <i>italic</i> text.</p>"

	result := ParseEmailBody("", html)

	// Should contain both pieces of text
	if !strings.Contains(result.CleanBody, "Hello world!") {
		t.Error("Missing 'Hello world!'")
	}
	if !strings.Contains(result.CleanBody, "This is italic text.") {
		t.Error("Missing 'This is italic text.'")
	}
	// Should have a newline separating the paragraphs
	if !strings.Contains(result.CleanBody, "\n") {
		t.Error("Should have newline between paragraphs")
	}
	// Should not have HTML tags
	if strings.Contains(result.CleanBody, "<") || strings.Contains(result.CleanBody, ">") {
		t.Error("Should not contain HTML tags")
	}
}

// TestHtmlToText_Entities tests HTML entity decoding.
func TestHtmlToText_Entities(t *testing.T) {
	html := "<p>Entities: &amp; &lt; &gt; &quot; &#39;</p>"
	expected := "Entities: & < > \" '"

	result := ParseEmailBody("", html)

	if result.CleanBody != expected {
		t.Errorf("HTML entity decoding failed.\nGot: %s\nWant: %s", result.CleanBody, expected)
	}
}

// TestHtmlToText_StyleScript tests removal of style and script blocks.
func TestHtmlToText_StyleScript(t *testing.T) {
	html := `
		<html>
		<head>
			<style>
				body { color: red; }
			</style>
		</head>
		<body>
			<p>Visible text</p>
			<script>
				alert('This should not appear');
			</script>
			<p>More visible text</p>
		</body>
		</html>
	`
	expected := "Visible text\nMore visible text"

	result := ParseEmailBody("", html)

	// Script and style content should not appear
	if strings.Contains(result.CleanBody, "color: red") {
		t.Error("Style content should be removed")
	}
	if strings.Contains(result.CleanBody, "alert") {
		t.Error("Script content should be removed")
	}

	// Normalize whitespace for comparison
	got := strings.Join(strings.Fields(result.CleanBody), " ")
	want := strings.Join(strings.Fields(expected), " ")
	if got != want {
		t.Errorf("HTML with style/script failed.\nGot: %s\nWant: %s", got, want)
	}
}

// TestHtmlToText_Paragraphs tests paragraph boundary handling.
func TestHtmlToText_Paragraphs(t *testing.T) {
	html := "<p>First paragraph.</p><p>Second paragraph.</p><div>Third block.</div>"

	result := ParseEmailBody("", html)

	// Should have newlines between paragraphs
	lines := strings.Split(result.CleanBody, "\n")
	if len(lines) < 3 {
		t.Errorf("Expected at least 3 lines, got %d: %s", len(lines), result.CleanBody)
	}

	if !strings.Contains(result.CleanBody, "First paragraph") {
		t.Error("First paragraph missing")
	}
	if !strings.Contains(result.CleanBody, "Second paragraph") {
		t.Error("Second paragraph missing")
	}
	if !strings.Contains(result.CleanBody, "Third block") {
		t.Error("Third block missing")
	}
}

// TestHtmlToText_ComplexNested tests complex nested HTML.
func TestHtmlToText_ComplexNested(t *testing.T) {
	html := `
		<div>
			<p>Outer paragraph</p>
			<div>
				<span>Nested <strong>bold</strong> text</span>
				<ul>
					<li>Item 1</li>
					<li>Item 2</li>
				</ul>
			</div>
		</div>
	`

	result := ParseEmailBody("", html)

	// Check that content is present
	if !strings.Contains(result.CleanBody, "Outer paragraph") {
		t.Error("Outer paragraph missing")
	}
	if !strings.Contains(result.CleanBody, "Nested bold text") {
		t.Error("Nested text missing")
	}
	if !strings.Contains(result.CleanBody, "Item 1") {
		t.Error("List item 1 missing")
	}
	if !strings.Contains(result.CleanBody, "Item 2") {
		t.Error("List item 2 missing")
	}

	// Should have some line breaks for block elements
	if !strings.Contains(result.CleanBody, "\n") {
		t.Error("Expected newlines for block elements")
	}
}

// TestSeparateQuotedReply_Gmail tests Gmail-style "On ... wrote:" detection.
func TestSeparateQuotedReply_Gmail(t *testing.T) {
	email := `Thanks for the update!

On Mon, Jan 29, 2024 at 10:30 AM John Doe <john@example.com> wrote:
> This is the original message.
> It has multiple lines.
> All quoted.`

	result := ParseEmailBody(email, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "Thanks for the update") {
		t.Errorf("New content missing expected text.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "On Mon") {
		t.Error("New content should not contain the attribution line")
	}

	if !strings.Contains(result.QuotedContent, "On Mon") {
		t.Error("Quoted content should start with attribution line")
	}

	if !strings.Contains(result.QuotedContent, "original message") {
		t.Error("Quoted content should contain the original message")
	}
}

// TestSeparateQuotedReply_AngleBracket tests standard ">" quoting.
func TestSeparateQuotedReply_AngleBracket(t *testing.T) {
	email := `I agree with your points.

> This is a quoted line.
> Another quoted line.
> And a third one.
> Fourth line here.`

	result := ParseEmailBody(email, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "I agree with your points") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "quoted line") {
		t.Error("New content should not contain quoted text")
	}

	// Quoted content should have ">" characters stripped
	if strings.Contains(result.QuotedContent, ">") {
		t.Errorf("Quoted content should have '>' stripped.\nGot: %s", result.QuotedContent)
	}

	if !strings.Contains(result.QuotedContent, "This is a quoted line") {
		t.Error("Quoted content missing expected text")
	}
}

// TestSeparateQuotedReply_OutlookOriginalMessage tests "-----Original Message-----".
func TestSeparateQuotedReply_OutlookOriginalMessage(t *testing.T) {
	email := `Got it, thanks!

-----Original Message-----
From: Jane Smith
Sent: Monday, January 29, 2024 2:15 PM
To: Bob Jones
Subject: RE: Project Update

Here is the original message content.`

	result := ParseEmailBody(email, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "Got it, thanks") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "Original Message") {
		t.Error("New content should not contain separator line")
	}

	if !strings.Contains(result.QuotedContent, "Original Message") {
		t.Error("Quoted content should contain separator line")
	}

	if !strings.Contains(result.QuotedContent, "From: Jane Smith") {
		t.Error("Quoted content should contain Outlook headers")
	}
}

// TestSeparateQuotedReply_OutlookBlock tests Outlook "From: ... Sent: ..." block.
func TestSeparateQuotedReply_OutlookBlock(t *testing.T) {
	email := `Sounds good to me.

From: alice@example.com
Sent: Tuesday, January 30, 2024 9:00 AM
To: bob@example.com
Subject: Re: Meeting notes

This was the previous email in the thread.`

	result := ParseEmailBody(email, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "Sounds good to me") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "From: alice") {
		t.Error("New content should not contain Outlook block")
	}

	if !strings.Contains(result.QuotedContent, "From: alice@example.com") {
		t.Error("Quoted content should contain From header")
	}

	if !strings.Contains(result.QuotedContent, "Sent: Tuesday") {
		t.Error("Quoted content should contain Sent header")
	}
}

// TestSeparateQuotedReply_NoQuotes tests email with no quoted reply.
func TestSeparateQuotedReply_NoQuotes(t *testing.T) {
	email := `This is a fresh email with no replies.

It has multiple paragraphs.

But no quoted content.`

	result := ParseEmailBody(email, "")

	if result.QuotedReplyDetected {
		t.Error("Should not detect quoted reply when none exists")
	}

	// When no quotes detected, NewContent should equal CleanBody
	if result.NewContent != result.CleanBody {
		t.Error("NewContent should equal CleanBody when no quotes detected")
	}

	if result.QuotedContent != "" {
		t.Errorf("QuotedContent should be empty, got: %s", result.QuotedContent)
	}
}

// TestSeparateQuotedReply_NestedQuotes tests nested/complex quoting.
func TestSeparateQuotedReply_NestedQuotes(t *testing.T) {
	email := `I'll handle this.

On Wed, Jan 31, 2024 at 3:00 PM Alice wrote:
> Thanks for the info.
>
> On Wed, Jan 31, 2024 at 2:00 PM Bob wrote:
> > Here's the original message.
> > With multiple lines.`

	result := ParseEmailBody(email, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "I'll handle this") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "On Wed") {
		t.Error("New content should not contain attribution lines")
	}

	// The entire nested quote should be in QuotedContent
	if !strings.Contains(result.QuotedContent, "Thanks for the info") {
		t.Error("Quoted content should contain Alice's message")
	}

	if !strings.Contains(result.QuotedContent, "original message") {
		t.Error("Quoted content should contain Bob's original message")
	}
}

// TestParseEmailBody_HtmlWithQuotes tests HTML email with quoted reply.
func TestParseEmailBody_HtmlWithQuotes(t *testing.T) {
	html := `<div>
		<p>This is my reply.</p>
		<div class="gmail_quote">
			<p>On Mon, Jan 29, 2024, John wrote:</p>
			<blockquote>
				<p>Original message here.</p>
			</blockquote>
		</div>
	</div>`

	result := ParseEmailBody("", html)

	// HTML should be stripped
	if strings.Contains(result.CleanBody, "<p>") {
		t.Error("HTML tags should be stripped")
	}

	if strings.Contains(result.CleanBody, "blockquote") {
		t.Error("HTML tags should be stripped")
	}

	// Quoted reply should be detected
	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	if !strings.Contains(result.NewContent, "This is my reply") {
		t.Error("New content missing")
	}

	if !strings.Contains(result.QuotedContent, "Original message") {
		t.Error("Quoted content missing")
	}
}

// TestNormalizeWhitespace tests whitespace normalization.
func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "Hello    world",
			expected: "Hello world",
		},
		{
			name:     "windows line endings",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "excessive newlines",
			input:    "Para 1\n\n\n\nPara 2",
			expected: "Para 1\n\nPara 2",
		},
		{
			name:     "trailing whitespace",
			input:    "  Text with spaces  \n\n",
			expected: "Text with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWhitespace(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeWhitespace() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestParseEmailBody_RealWorldGmail tests a realistic Gmail format.
func TestParseEmailBody_RealWorldGmail(t *testing.T) {
	html := `<div dir="ltr">Thanks for sending this over. I&#39;ll review and get back to you.<br></div>
<div class="gmail_quote">
<div dir="ltr" class="gmail_attr">On Mon, Jan 29, 2024 at 2:30 PM John Doe &lt;<a href="mailto:john@example.com">john@example.com</a>&gt; wrote:<br></div>
<blockquote class="gmail_quote" style="margin:0px 0px 0px 0.8ex;border-left:1px solid rgb(204,204,204);padding-left:1ex">
<div dir="ltr">Please review the attached proposal.<br><br>Let me know if you have questions.</div>
</blockquote>
</div>`

	result := ParseEmailBody("", html)

	// HTML should be stripped
	if strings.Contains(result.CleanBody, "<div") || strings.Contains(result.CleanBody, "<br>") {
		t.Error("HTML tags should be stripped")
	}

	// HTML entities should be decoded
	if strings.Contains(result.CleanBody, "&#39;") {
		t.Error("HTML entities should be decoded")
	}
	if !strings.Contains(result.CleanBody, "I'll review") {
		t.Error("HTML entity for apostrophe should be decoded to '")
	}

	// Quoted reply should be detected
	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	// New content should contain the reply
	if !strings.Contains(result.NewContent, "Thanks for sending") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	// Quoted content should contain the original
	if !strings.Contains(result.QuotedContent, "review the attached proposal") {
		t.Errorf("Quoted content missing.\nGot: %s", result.QuotedContent)
	}

	// New content should not contain "On Mon" attribution
	if strings.Contains(result.NewContent, "On Mon") {
		t.Error("New content should not contain attribution line")
	}
}

// TestParseEmailBody_RealWorldOutlook tests a realistic Outlook format.
func TestParseEmailBody_RealWorldOutlook(t *testing.T) {
	text := `Perfect, let's schedule for next week.

-----Original Message-----
From: Smith, Jane [mailto:jane.smith@company.com]
Sent: Monday, January 29, 2024 3:45 PM
To: Jones, Bob
Cc: Williams, Carol
Subject: RE: Q1 Planning Meeting

Bob,

Can we move the planning meeting to next week? I have a conflict on Thursday.

Thanks,
Jane`

	result := ParseEmailBody(text, "")

	if !result.QuotedReplyDetected {
		t.Error("Expected quoted reply to be detected")
	}

	// New content should be just the reply
	if !strings.Contains(result.NewContent, "Perfect") {
		t.Errorf("New content missing.\nGot: %s", result.NewContent)
	}

	if strings.Contains(result.NewContent, "Original Message") {
		t.Error("New content should not contain the separator")
	}

	// Quoted content should have the full original
	if !strings.Contains(result.QuotedContent, "Original Message") {
		t.Error("Quoted content should contain separator")
	}

	if !strings.Contains(result.QuotedContent, "From: Smith, Jane") {
		t.Error("Quoted content should contain From header")
	}

	if !strings.Contains(result.QuotedContent, "move the planning meeting") {
		t.Error("Quoted content should contain original message body")
	}
}

// TestSeparateQuotedReply_FewAngleBrackets tests that we don't trigger on 1-2 ">" lines.
func TestSeparateQuotedReply_FewAngleBrackets(t *testing.T) {
	email := `Here's my code example:

> function foo() {
>   return bar;
}

What do you think?`

	result := ParseEmailBody(email, "")

	// Should NOT detect this as a quoted reply (only 2 lines with >)
	// The threshold is 3+ consecutive lines
	if result.QuotedReplyDetected {
		t.Error("Should not detect quoted reply for code examples with few > lines")
	}
}
