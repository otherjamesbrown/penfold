package parse

import (
	"strings"
	"testing"
)

func TestNormalizeSpeakerName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "pronoun she/her",
			input:    "Sara Weisman (she/her)",
			expected: "Sara Weisman",
		},
		{
			name:     "pronoun He/Him",
			input:    "John Smith (He/Him)",
			expected: "John Smith",
		},
		{
			name:     "pronoun they/them",
			input:    "Alex Johnson (they/them)",
			expected: "Alex Johnson",
		},
		{
			name:     "bracket guest suffix",
			input:    "John Smith [Guest]",
			expected: "John Smith",
		},
		{
			name:     "bracket external suffix",
			input:    "Jane Doe [External]",
			expected: "Jane Doe",
		},
		{
			name:     "plain name no changes",
			input:    "Alice Brown",
			expected: "Alice Brown",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "pronoun with whitespace",
			input:    "  Sara Weisman (she/her)  ",
			expected: "Sara Weisman",
		},
		{
			name:     "multiple suffixes",
			input:    "John Smith (he/him) [Presenter]",
			expected: "John Smith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSpeakerName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSpeakerName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSRT(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedFormat string
		expectSegments int
		expectSpeakers []string
		expectError    bool
	}{
		{
			name: "basic SRT",
			input: `1
00:00:05,579 --> 00:00:06,858
Hello everyone

2
00:00:07,100 --> 00:00:09,200
Let's get started
`,
			expectedFormat: "srt",
			expectSegments: 1, // Both segments have "Unknown" speaker, so they merge
			expectSpeakers: []string{"Unknown"},
		},
		{
			name: "SRT with speaker labels",
			input: `1
00:00:05,579 --> 00:00:06,858
Sara Weisman: Hello everyone

2
00:00:07,100 --> 00:00:09,200
John Smith: Let's get started

3
00:00:10,000 --> 00:00:12,000
Sara Weisman: Sounds good
`,
			expectedFormat: "srt",
			expectSegments: 3,
			expectSpeakers: []string{"Sara Weisman", "John Smith"},
		},
		{
			name: "SRT with pronouns in speaker labels",
			input: `1
00:00:05,579 --> 00:00:06,858
Sara Weisman (she/her): Hello everyone

2
00:00:07,100 --> 00:00:09,200
John Smith (he/him): Let's get started
`,
			expectedFormat: "srt",
			expectSegments: 2,
			expectSpeakers: []string{"Sara Weisman", "John Smith"},
		},
		{
			name: "single segment SRT",
			input: `1
00:00:05,579 --> 00:00:06,858
Only one segment here
`,
			expectedFormat: "srt",
			expectSegments: 1,
			expectSpeakers: []string{"Unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTranscript(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Format != tt.expectedFormat {
				t.Errorf("format = %q, want %q", result.Format, tt.expectedFormat)
			}

			// Count segments by newlines in clean text
			segments := strings.Split(strings.TrimSpace(result.CleanText), "\n")
			if len(segments) != tt.expectSegments {
				t.Errorf("got %d segments, want %d. Clean text:\n%s", len(segments), tt.expectSegments, result.CleanText)
			}

			if len(result.Speakers) != len(tt.expectSpeakers) {
				t.Errorf("got %d speakers, want %d", len(result.Speakers), len(tt.expectSpeakers))
			}

			for i, expected := range tt.expectSpeakers {
				if i >= len(result.Speakers) {
					break
				}
				if result.Speakers[i] != expected {
					t.Errorf("speaker[%d] = %q, want %q", i, result.Speakers[i], expected)
				}
			}
		})
	}
}

func TestParseVTT(t *testing.T) {
	input := `WEBVTT

1 "Sara Weisman (she/her)" (123)
00:00:05.579 --> 00:00:06.858
Hello everyone

2 "John Smith" (456)
00:00:07.100 --> 00:00:09.200
Let's get started
`

	result, err := ParseTranscript(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Format != "vtt" {
		t.Errorf("format = %q, want vtt", result.Format)
	}

	if len(result.Speakers) != 2 {
		t.Errorf("got %d speakers, want 2", len(result.Speakers))
	}

	// Check speaker normalization
	expectedSpeakers := []string{"Sara Weisman", "John Smith"}
	for i, expected := range expectedSpeakers {
		if i >= len(result.Speakers) {
			break
		}
		if result.Speakers[i] != expected {
			t.Errorf("speaker[%d] = %q, want %q", i, result.Speakers[i], expected)
		}
	}

	// Clean text should not have timestamps
	if strings.Contains(result.CleanText, "00:00") {
		t.Errorf("clean text contains timestamps: %s", result.CleanText)
	}

	// Clean text should have speaker attribution
	if !strings.Contains(result.CleanText, "Sara Weisman:") {
		t.Errorf("clean text missing speaker attribution: %s", result.CleanText)
	}
}

func TestParseTXT(t *testing.T) {
	input := `0:11 : Sara Weisman (she/her) : Hello everyone
0:23 : John Smith : Let's get started
1:45 : Sara Weisman (she/her) : Sounds good to me
`

	result, err := ParseTranscript(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Format != "txt" {
		t.Errorf("format = %q, want txt", result.Format)
	}

	if len(result.Speakers) != 2 {
		t.Errorf("got %d speakers, want 2", len(result.Speakers))
	}

	// Check speaker normalization
	expectedSpeakers := []string{"Sara Weisman", "John Smith"}
	for i, expected := range expectedSpeakers {
		if i >= len(result.Speakers) {
			break
		}
		if result.Speakers[i] != expected {
			t.Errorf("speaker[%d] = %q, want %q", i, result.Speakers[i], expected)
		}
	}

	// Clean text should not have timestamps
	if strings.Contains(result.CleanText, "0:11") {
		t.Errorf("clean text contains timestamps: %s", result.CleanText)
	}
}

func TestFormatAutoDetection(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedFormat string
	}{
		{
			name: "detect VTT by header",
			input: `WEBVTT

1 "Speaker" (123)
00:00:05.579 --> 00:00:06.858
Text here
`,
			expectedFormat: "vtt",
		},
		{
			name: "detect SRT by timestamp comma",
			input: `1
00:00:05,579 --> 00:00:06,858
Text here
`,
			expectedFormat: "srt",
		},
		{
			name: "detect TXT by pattern",
			input: `0:11 : Speaker Name : Text content
0:23 : Another Speaker : More text
`,
			expectedFormat: "txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTranscript(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Format != tt.expectedFormat {
				t.Errorf("detected format = %q, want %q", result.Format, tt.expectedFormat)
			}
		})
	}
}

func TestSameSpeakerMerging(t *testing.T) {
	input := `1
00:00:05,579 --> 00:00:06,858
Sara: First sentence.

2
00:00:07,100 --> 00:00:08,200
Sara: Second sentence.

3
00:00:09,000 --> 00:00:10,000
John: Different speaker.

4
00:00:11,000 --> 00:00:12,000
Sara: Third sentence.
`

	result, err := ParseTranscript(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 blocks: Sara (merged first two), John, Sara
	lines := strings.Split(strings.TrimSpace(result.CleanText), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 output lines, got %d:\n%s", len(lines), result.CleanText)
	}

	// First line should have both first and second sentences
	if !strings.Contains(lines[0], "First sentence") || !strings.Contains(lines[0], "Second sentence") {
		t.Errorf("first Sara line should contain both sentences: %s", lines[0])
	}

	// Second line should be John
	if !strings.Contains(lines[1], "John:") {
		t.Errorf("second line should be John: %s", lines[1])
	}

	// Third line should be Sara again (not merged with first Sara block)
	if !strings.Contains(lines[2], "Sara:") || !strings.Contains(lines[2], "Third sentence") {
		t.Errorf("third line should be Sara with third sentence: %s", lines[2])
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty content",
			input:       "",
			expectError: true,
		},
		{
			name: "single segment",
			input: `1
00:00:05,579 --> 00:00:06,858
Only one segment
`,
			expectError: false,
		},
		{
			name: "whitespace only",
			input: `


`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTranscript(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result but got nil")
			}
		})
	}
}

func TestDurationCalculation(t *testing.T) {
	input := `1
00:00:05,579 --> 00:00:06,858
First

2
00:01:30,000 --> 00:01:35,500
Last segment
`

	result, err := ParseTranscript(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duration should be based on last end time: 00:01:35,500 = 95500ms
	expectedDurationMs := 95500
	if result.DurationMs != expectedDurationMs {
		t.Errorf("duration = %d ms, want %d ms", result.DurationMs, expectedDurationMs)
	}
}

func TestCleanTextFormat(t *testing.T) {
	input := `1
00:00:05,579 --> 00:00:06,858
Sara: Hello everyone

2
00:00:07,100 --> 00:00:09,200
John: Let's get started
`

	result, err := ParseTranscript(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no timestamps in clean text
	if strings.Contains(result.CleanText, "00:00") || strings.Contains(result.CleanText, "-->") {
		t.Errorf("clean text contains timestamps or arrows: %s", result.CleanText)
	}

	// Check no sequence numbers
	if strings.HasPrefix(result.CleanText, "1") {
		t.Errorf("clean text contains sequence number: %s", result.CleanText)
	}

	// Check speaker attribution format
	if !strings.Contains(result.CleanText, "Sara: Hello everyone") {
		t.Errorf("clean text missing expected speaker attribution: %s", result.CleanText)
	}

	if !strings.Contains(result.CleanText, "John: Let's get started") {
		t.Errorf("clean text missing expected speaker attribution: %s", result.CleanText)
	}
}
