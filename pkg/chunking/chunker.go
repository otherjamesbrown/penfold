// Package chunking provides utilities for splitting text into embedding-safe chunks.
package chunking

import "strings"

// Chunk represents a single chunk of text with metadata.
type Chunk struct {
	Index int    // Position of this chunk in the sequence (0-indexed)
	Text  string // The chunk content (includes overlap from previous chunk)
	Start int    // Byte offset in original text where NEW content starts
	End   int    // Byte offset in original text where NEW content ends
}

// ChunkOptions configures the chunking behavior.
type ChunkOptions struct {
	MaxTokens      int                 // Maximum tokens per chunk (default 400)
	OverlapTokens  int                 // Overlap tokens between chunks (default 50)
	TokenEstimator func(string) int    // Function to estimate tokens (default: len/4)
}

// ChunkContent splits text into chunks suitable for embedding.
//
// Requirements:
//  - Split on paragraph boundaries first (double newline), then sentence boundaries
//  - Group small consecutive paragraphs into a single chunk until approaching MaxTokens
//  - Never split mid-sentence
//  - Add OverlapTokens overlap between consecutive chunks (repeat last 1-2 sentences)
//  - Target chunk size: 350-400 tokens (headroom under 512 model limit)
//  - Short content (under MaxTokens) returns a single chunk
//  - Empty text returns empty slice
func ChunkContent(text string, opts ChunkOptions) []Chunk {
	// Handle empty text
	if text == "" {
		return []Chunk{}
	}

	// Apply defaults
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 400
	}
	if opts.OverlapTokens <= 0 {
		opts.OverlapTokens = 50
	}
	if opts.TokenEstimator == nil {
		opts.TokenEstimator = func(s string) int {
			return len(s) / 4
		}
	}

	// If entire text fits in one chunk, return it
	if opts.TokenEstimator(text) <= opts.MaxTokens {
		return []Chunk{
			{
				Index: 0,
				Text:  text,
				Start: 0,
				End:   len(text),
			},
		}
	}

	// Build chunks without overlap first
	rawChunks := buildChunksWithoutOverlap(text, opts)

	// Now add overlap between chunks
	return addOverlapToChunks(rawChunks, opts)
}

type rawChunk struct {
	text  string
	start int
	end   int
}

func buildChunksWithoutOverlap(text string, opts ChunkOptions) []rawChunk {
	// Split on double newline first
	paragraphs := strings.Split(text, "\n\n")

	var chunks []rawChunk
	var currentParts []string
	var currentStart int
	currentBytes := 0

	// Reserve space for overlap in all chunks except the first
	targetMaxTokens := opts.MaxTokens - opts.OverlapTokens

	offset := 0
	for _, para := range paragraphs {
		if para == "" {
			offset += 2 // Account for \n\n
			continue
		}

		paraTokens := opts.TokenEstimator(para)

		// For first chunk, we can use full MaxTokens (no overlap to add)
		limit := opts.MaxTokens
		if len(chunks) > 0 || len(currentParts) > 0 {
			limit = targetMaxTokens
		}

		// If paragraph exceeds limit, split it
		if paraTokens > limit {
			// Finalize current group first
			if len(currentParts) > 0 {
				content := strings.Join(currentParts, "\n\n")
				chunks = append(chunks, rawChunk{
					text:  content,
					start: currentStart,
					end:   currentStart + len(content),
				})
				currentParts = nil
				currentBytes = 0
			}

			// Split large paragraph
			paraChunks := splitLargeParagraph(para, offset, opts)
			chunks = append(chunks, paraChunks...)
			offset += len(para) + 2
			currentStart = offset
			continue
		}

		// Try adding to current group
		if len(currentParts) == 0 {
			currentParts = []string{para}
			currentStart = offset
			currentBytes = len(para)
		} else {
			combinedBytes := currentBytes + 2 + len(para) // +2 for \n\n
			combinedTokens := opts.TokenEstimator(strings.Join(append(currentParts, para), "\n\n"))

			// Use appropriate limit
			limit := opts.MaxTokens
			if len(chunks) > 0 {
				limit = targetMaxTokens
			}

			if combinedTokens <= limit {
				currentParts = append(currentParts, para)
				currentBytes = combinedBytes
			} else {
				// Finalize current group
				content := strings.Join(currentParts, "\n\n")
				chunks = append(chunks, rawChunk{
					text:  content,
					start: currentStart,
					end:   currentStart + len(content),
				})

				// Start new group
				currentParts = []string{para}
				currentStart = offset
				currentBytes = len(para)
			}
		}

		offset += len(para) + 2 // +2 for \n\n
	}

	// Finalize remaining content
	if len(currentParts) > 0 {
		content := strings.Join(currentParts, "\n\n")
		chunks = append(chunks, rawChunk{
			text:  content,
			start: currentStart,
			end:   currentStart + len(content),
		})
	}

	return chunks
}

func splitLargeParagraph(para string, paraStart int, opts ChunkOptions) []rawChunk {
	// Try sentence splitting
	sentences := splitSentences(para)

	if len(sentences) <= 1 {
		// No sentence boundaries, use word splitting
		return splitByWords(para, paraStart, opts)
	}

	var chunks []rawChunk
	var currentSents []string
	targetMaxTokens := opts.MaxTokens - opts.OverlapTokens

	offset := paraStart
	for _, sent := range sentences {
		if len(currentSents) == 0 {
			currentSents = []string{sent}
		} else {
			combined := strings.Join(append(currentSents, sent), "")
			combinedTokens := opts.TokenEstimator(combined)

			// First chunk can use full MaxTokens, others need room for overlap
			limit := opts.MaxTokens
			if len(chunks) > 0 {
				limit = targetMaxTokens
			}

			if combinedTokens <= limit {
				currentSents = append(currentSents, sent)
			} else {
				// Finalize current group
				content := strings.Join(currentSents, "")
				chunks = append(chunks, rawChunk{
					text:  content,
					start: offset,
					end:   offset + len(content),
				})
				offset += len(content)

				// Start new group
				currentSents = []string{sent}
			}
		}
	}

	// Finalize remaining
	if len(currentSents) > 0 {
		content := strings.Join(currentSents, "")
		chunks = append(chunks, rawChunk{
			text:  content,
			start: offset,
			end:   offset + len(content),
		})
	}

	return chunks
}

func splitByWords(para string, paraStart int, opts ChunkOptions) []rawChunk {
	words := strings.Fields(para)

	// If no word boundaries (single long word), split by characters
	if len(words) == 1 && opts.TokenEstimator(words[0]) > opts.MaxTokens {
		return splitByCharacters(para, paraStart, opts)
	}

	var chunks []rawChunk
	var currentWords []string
	targetMaxTokens := opts.MaxTokens - opts.OverlapTokens

	// Track actual positions in original text
	wordPositions := make([]int, len(words))
	pos := 0
	for i, word := range words {
		// Find word in remaining text
		idx := strings.Index(para[pos:], word)
		wordPositions[i] = paraStart + pos + idx
		pos += idx + len(word)
	}

	for i, word := range words {
		if len(currentWords) == 0 {
			currentWords = []string{word}
		} else {
			combined := strings.Join(append(currentWords, word), " ")
			combinedTokens := opts.TokenEstimator(combined)

			// First chunk can use full MaxTokens, others need room for overlap
			limit := opts.MaxTokens
			if len(chunks) > 0 {
				limit = targetMaxTokens
			}

			if combinedTokens <= limit {
				currentWords = append(currentWords, word)
			} else {
				// Finalize current group
				content := strings.Join(currentWords, " ")
				startPos := wordPositions[i-len(currentWords)]

				// Check if there's a space after the last word in the original text
				endPos := startPos + len(content)
				if endPos < len(para) && para[endPos-paraStart] == ' ' {
					content += " "
					endPos++
				}

				chunks = append(chunks, rawChunk{
					text:  content,
					start: startPos,
					end:   endPos,
				})

				// Start new group
				currentWords = []string{word}
			}
		}
	}

	// Finalize remaining (last chunk - no trailing space needed)
	if len(currentWords) > 0 {
		content := strings.Join(currentWords, " ")
		startPos := wordPositions[len(words)-len(currentWords)]
		endPos := startPos + len(content)
		chunks = append(chunks, rawChunk{
			text:  content,
			start: startPos,
			end:   endPos,
		})
	}

	return chunks
}

func splitByCharacters(para string, paraStart int, opts ChunkOptions) []rawChunk {
	var chunks []rawChunk
	targetMaxTokens := opts.MaxTokens - opts.OverlapTokens

	offset := paraStart
	remaining := para
	for len(remaining) > 0 {
		// Determine chunk size for this chunk
		limit := opts.MaxTokens
		if len(chunks) > 0 {
			limit = targetMaxTokens
		}

		// Binary search to find the right chunk size
		chunkSize := findChunkSize(remaining, limit, opts.TokenEstimator)

		chunk := remaining[:chunkSize]
		chunks = append(chunks, rawChunk{
			text:  chunk,
			start: offset,
			end:   offset + len(chunk),
		})
		offset += len(chunk)
		remaining = remaining[chunkSize:]
	}

	return chunks
}

func findChunkSize(text string, targetTokens int, estimator func(string) int) int {
	if estimator(text) <= targetTokens {
		return len(text)
	}

	// Binary search for the right size
	low, high := 1, len(text)
	for low < high {
		mid := (low + high + 1) / 2
		if estimator(text[:mid]) <= targetTokens {
			low = mid
		} else {
			high = mid - 1
		}
	}

	return low
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for i := 0; i < len(text); i++ {
		current.WriteByte(text[i])

		// Check for sentence-ending punctuation
		if text[i] == '.' || text[i] == '!' || text[i] == '?' {
			// Check if followed by space, newline, or end of text
			if i+1 >= len(text) || text[i+1] == ' ' || text[i+1] == '\n' {
				// Include the following space if present
				if i+1 < len(text) && text[i+1] == ' ' {
					current.WriteByte(' ')
					i++
				}
				sentences = append(sentences, current.String())
				current.Reset()
			}
		}
	}

	// Add any remaining text
	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}

	return sentences
}

func addOverlapToChunks(rawChunks []rawChunk, opts ChunkOptions) []Chunk {
	var result []Chunk

	for i, rc := range rawChunks {
		chunkText := rc.text

		// Add overlap from previous chunk if not the first chunk
		if i > 0 {
			overlap := extractOverlap(rawChunks[i-1].text, opts)
			chunkText = overlap + chunkText
		}

		result = append(result, Chunk{
			Index: i,
			Text:  chunkText,
			Start: rc.start,
			End:   rc.end,
		})
	}

	return result
}

func extractOverlap(text string, opts ChunkOptions) string {
	if opts.TokenEstimator(text) <= opts.OverlapTokens {
		return text
	}

	// Split into sentences and work backwards
	sentences := splitSentences(text)

	// If we have multiple sentences, use sentence-based overlap
	if len(sentences) > 1 {
		var result []string
		tokenCount := 0

		for i := len(sentences) - 1; i >= 0; i-- {
			sentence := sentences[i]
			sentTokens := opts.TokenEstimator(sentence)

			if tokenCount+sentTokens > opts.OverlapTokens && tokenCount > 0 {
				break
			}

			result = append([]string{sentence}, result...)
			tokenCount += sentTokens
		}

		if len(result) > 0 {
			return strings.Join(result, "")
		}
	}

	// Fallback: binary search for the right substring length
	size := findChunkSize(text, opts.OverlapTokens, opts.TokenEstimator)
	if size >= len(text) {
		return text
	}

	// Take from the end
	return text[len(text)-size:]
}
