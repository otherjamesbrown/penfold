// Package parse provides deterministic parsing utilities for the SLM/LLM pipeline Stage 0.
package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/otherjamesbrown/penfold/pkg/ingest/meeting"
)

// TranscriptParseResult holds the result of parsing transcript content for the pipeline.
type TranscriptParseResult struct {
	CleanText  string   // Speaker-attributed clean text (no timestamps)
	Speakers   []string // Normalized unique speaker names
	DurationMs int      // Total duration in milliseconds
	Format     string   // Detected format: "vtt", "srt", "txt"
	Warnings   []string
}

// SRT parsing regular expressions
var (
	// Matches SRT timestamp line: 00:00:05,579 --> 00:00:06,858
	srtTimestampRegex = regexp.MustCompile(`^(\d{2}:\d{2}:\d{2},\d{3})\s+-->\s+(\d{2}:\d{2}:\d{2},\d{3})`)

	// Matches sequence number: just digits on a line
	srtSequenceRegex = regexp.MustCompile(`^\d+$`)

	// Matches speaker prefix in text: "Speaker Name: text"
	speakerPrefixRegex = regexp.MustCompile(`^([^:]+):\s*(.*)$`)

	// Speaker normalization patterns
	pronounRegex       = regexp.MustCompile(`(?i)\s*\([^)]*(?:she|he|they|him|her|them)[^)]*\)`)
	bracketSuffixRegex = regexp.MustCompile(`\s*\[[^\]]+\]`)
)

// ParseTranscript detects format and parses transcript content.
// Accepts VTT, SRT, or TXT format content.
func ParseTranscript(content string) (*TranscriptParseResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty transcript content")
	}

	// Detect format
	format := detectFormat(content)

	var segments []transcriptSegment
	var durationMs int
	var err error

	switch format {
	case "vtt":
		segments, durationMs, err = parseVTT(content)
	case "srt":
		segments, durationMs, err = parseSRT(content)
	case "txt":
		segments, durationMs, err = parseTXT(content)
	default:
		return nil, fmt.Errorf("unknown transcript format")
	}

	if err != nil {
		return nil, err
	}

	// Build clean text and extract speakers
	cleanText, speakers := buildCleanOutput(segments)

	return &TranscriptParseResult{
		CleanText:  cleanText,
		Speakers:   speakers,
		DurationMs: durationMs,
		Format:     format,
	}, nil
}

// transcriptSegment is an internal representation for merging
type transcriptSegment struct {
	Speaker string
	Text    string
}

// detectFormat auto-detects transcript format by scanning first 10 lines
func detectFormat(content string) string {
	lines := strings.Split(content, "\n")
	maxLines := 10
	if len(lines) < maxLines {
		maxLines = len(lines)
	}

	for i := 0; i < maxLines; i++ {
		line := strings.TrimSpace(lines[i])

		// Check for VTT header
		if line == "WEBVTT" {
			return "vtt"
		}

		// Check for SRT pattern (timestamp with comma)
		if srtTimestampRegex.MatchString(line) {
			return "srt"
		}

		// Check for TXT pattern (MM:SS : Speaker : text)
		if strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			if len(parts) >= 3 {
				// Check if first part looks like timestamp (digits only)
				if matched, _ := regexp.MatchString(`^\d+$`, strings.TrimSpace(parts[0])); matched {
					return "txt"
				}
			}
		}
	}

	// Default to SRT if we find sequence numbers
	for i := 0; i < maxLines; i++ {
		line := strings.TrimSpace(lines[i])
		if srtSequenceRegex.MatchString(line) {
			return "srt"
		}
	}

	return "unknown"
}

// parseVTT delegates to existing VTT parser and normalizes output
func parseVTT(content string) ([]transcriptSegment, int, error) {
	reader := strings.NewReader(content)
	result, err := meeting.ParseVTT(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("VTT parse error: %w", err)
	}

	segments := make([]transcriptSegment, 0, len(result.Segments))
	for _, seg := range result.Segments {
		speaker := NormalizeSpeakerName(seg.Speaker)
		if speaker == "" {
			speaker = "Unknown"
		}
		segments = append(segments, transcriptSegment{
			Speaker: speaker,
			Text:    seg.Text,
		})
	}

	durationMs := result.DurationSeconds * 1000
	return segments, durationMs, nil
}

// parseTXT delegates to existing TXT parser and normalizes output
func parseTXT(content string) ([]transcriptSegment, int, error) {
	reader := strings.NewReader(content)
	result, err := meeting.ParseTXTTranscript(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("TXT parse error: %w", err)
	}

	segments := make([]transcriptSegment, 0, len(result.Segments))
	for _, seg := range result.Segments {
		speaker := NormalizeSpeakerName(seg.Speaker)
		if speaker == "" {
			speaker = "Unknown"
		}
		segments = append(segments, transcriptSegment{
			Speaker: speaker,
			Text:    seg.Text,
		})
	}

	durationMs := result.DurationSeconds * 1000
	return segments, durationMs, nil
}

// parseSRT parses SubRip (SRT) format transcripts
func parseSRT(content string) ([]transcriptSegment, int, error) {
	lines := strings.Split(content, "\n")
	segments := make([]transcriptSegment, 0)

	var currentText strings.Builder
	var currentSpeaker string
	var lastEndMs int
	inTextBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Empty line marks end of entry
		if trimmed == "" {
			if inTextBlock && currentText.Len() > 0 {
				text := currentText.String()
				speaker := currentSpeaker
				if speaker == "" {
					speaker = "Unknown"
				}

				segments = append(segments, transcriptSegment{
					Speaker: speaker,
					Text:    text,
				})

				currentText.Reset()
				currentSpeaker = ""
				inTextBlock = false
			}
			continue
		}

		// Sequence number (ignore)
		if srtSequenceRegex.MatchString(trimmed) {
			continue
		}

		// Timestamp line
		if matches := srtTimestampRegex.FindStringSubmatch(trimmed); matches != nil {
			endMs := parseSRTTimestamp(matches[2])
			if endMs > lastEndMs {
				lastEndMs = endMs
			}
			inTextBlock = true
			continue
		}

		// Text content
		if inTextBlock {
			// Check if text has speaker prefix
			if matches := speakerPrefixRegex.FindStringSubmatch(trimmed); matches != nil {
				possibleSpeaker := strings.TrimSpace(matches[1])
				possibleText := strings.TrimSpace(matches[2])

				// Only treat as speaker if name is reasonable length
				if len(possibleSpeaker) > 0 && len(possibleSpeaker) < 50 {
					currentSpeaker = possibleSpeaker
					if possibleText != "" {
						if currentText.Len() > 0 {
							currentText.WriteString(" ")
						}
						currentText.WriteString(possibleText)
					}
					continue
				}
			}

			// Regular text
			if currentText.Len() > 0 {
				currentText.WriteString(" ")
			}
			currentText.WriteString(trimmed)
		}
	}

	// Handle last segment if file doesn't end with blank line
	if inTextBlock && currentText.Len() > 0 {
		speaker := currentSpeaker
		if speaker == "" {
			speaker = "Unknown"
		}
		segments = append(segments, transcriptSegment{
			Speaker: speaker,
			Text:    currentText.String(),
		})
	}

	return segments, lastEndMs, nil
}

// parseSRTTimestamp parses SRT timestamp (HH:MM:SS,mmm) to milliseconds
func parseSRTTimestamp(ts string) int {
	// Format: 00:00:05,579
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	// Split seconds and milliseconds (comma separator)
	secParts := strings.Split(parts[2], ",")
	seconds, _ := strconv.Atoi(secParts[0])
	milliseconds := 0
	if len(secParts) > 1 {
		milliseconds, _ = strconv.Atoi(secParts[1])
	}

	return hours*3600000 + minutes*60000 + seconds*1000 + milliseconds
}

// buildCleanOutput merges consecutive same-speaker segments and builds clean text
func buildCleanOutput(segments []transcriptSegment) (string, []string) {
	if len(segments) == 0 {
		return "", []string{}
	}

	var cleanText strings.Builder
	speakerSet := make(map[string]bool)
	speakers := make([]string, 0)

	var lastSpeaker string
	var currentText strings.Builder

	for _, seg := range segments {
		speaker := NormalizeSpeakerName(seg.Speaker)
		if speaker == "" {
			speaker = "Unknown"
		}

		// Track speakers
		if !speakerSet[speaker] {
			speakerSet[speaker] = true
			speakers = append(speakers, speaker)
		}

		// Merge consecutive same-speaker segments
		if speaker == lastSpeaker {
			if currentText.Len() > 0 {
				currentText.WriteString(" ")
			}
			currentText.WriteString(seg.Text)
		} else {
			// Flush previous speaker's text
			if lastSpeaker != "" && currentText.Len() > 0 {
				if cleanText.Len() > 0 {
					cleanText.WriteString("\n")
				}
				cleanText.WriteString(lastSpeaker)
				cleanText.WriteString(": ")
				cleanText.WriteString(currentText.String())
			}

			// Start new speaker
			lastSpeaker = speaker
			currentText.Reset()
			currentText.WriteString(seg.Text)
		}
	}

	// Flush last speaker
	if lastSpeaker != "" && currentText.Len() > 0 {
		if cleanText.Len() > 0 {
			cleanText.WriteString("\n")
		}
		cleanText.WriteString(lastSpeaker)
		cleanText.WriteString(": ")
		cleanText.WriteString(currentText.String())
	}

	return cleanText.String(), speakers
}

// NormalizeSpeakerName strips pronouns, platform suffixes, and trims whitespace.
func NormalizeSpeakerName(name string) string {
	// Trim initial whitespace
	name = strings.TrimSpace(name)

	if name == "" {
		return ""
	}

	// Strip parenthetical pronouns: (she/her), (He/Him), (they/them), etc.
	name = pronounRegex.ReplaceAllString(name, "")

	// Strip square bracket suffixes: [Guest], [External], etc.
	name = bracketSuffixRegex.ReplaceAllString(name, "")

	// Final trim
	name = strings.TrimSpace(name)

	return name
}
