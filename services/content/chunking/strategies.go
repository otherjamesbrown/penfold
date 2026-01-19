// Package chunking provides content chunking strategies configuration.
package chunking

// OverlapConfig defines how chunks should overlap for context preservation.
type OverlapConfig struct {
	// OverlapTokens specifies a fixed number of tokens to overlap.
	// Takes precedence over OverlapPercentage if > 0.
	OverlapTokens int

	// OverlapPercentage specifies overlap as a percentage of chunk size (0-100).
	// Used when OverlapTokens is 0.
	OverlapPercentage float64

	// MinOverlap is the minimum overlap tokens regardless of calculation.
	MinOverlap int

	// MaxOverlap is the maximum overlap tokens regardless of calculation.
	MaxOverlap int
}

// DefaultOverlapConfig returns sensible defaults for overlap.
func DefaultOverlapConfig() OverlapConfig {
	return OverlapConfig{
		OverlapTokens:     0,  // Use percentage instead
		OverlapPercentage: 10, // 10% overlap
		MinOverlap:        20,
		MaxOverlap:        200,
	}
}

// GetOverlapTokens calculates the overlap tokens for a given chunk size.
func (c OverlapConfig) GetOverlapTokens(chunkSize int) int {
	var overlap int

	if c.OverlapTokens > 0 {
		overlap = c.OverlapTokens
	} else {
		overlap = int(float64(chunkSize) * c.OverlapPercentage / 100)
	}

	// Apply min/max bounds
	if c.MinOverlap > 0 && overlap < c.MinOverlap {
		overlap = c.MinOverlap
	}
	if c.MaxOverlap > 0 && overlap > c.MaxOverlap {
		overlap = c.MaxOverlap
	}

	return overlap
}

// Validate ensures the overlap config is valid.
func (c OverlapConfig) Validate() error {
	if c.OverlapPercentage < 0 || c.OverlapPercentage > 50 {
		return ErrInvalidOverlapPercentage
	}
	if c.MinOverlap < 0 {
		return ErrInvalidMinOverlap
	}
	if c.MaxOverlap > 0 && c.MaxOverlap < c.MinOverlap {
		return ErrMaxLessThanMin
	}
	return nil
}

// ChunkSizeConfig defines token size constraints for chunks.
type ChunkSizeConfig struct {
	// MinTokens is the minimum acceptable chunk size in tokens.
	// Chunks smaller than this will be merged with neighbors.
	MinTokens int

	// TargetTokens is the ideal chunk size to aim for.
	TargetTokens int

	// MaxTokens is the maximum chunk size before forcing a split.
	MaxTokens int
}

// DefaultChunkSizeConfig returns sensible defaults for chunk sizes.
func DefaultChunkSizeConfig() ChunkSizeConfig {
	return ChunkSizeConfig{
		MinTokens:    50,
		TargetTokens: 256,
		MaxTokens:    512,
	}
}

// LargeChunkSizeConfig returns configuration for larger chunks.
func LargeChunkSizeConfig() ChunkSizeConfig {
	return ChunkSizeConfig{
		MinTokens:    100,
		TargetTokens: 512,
		MaxTokens:    1024,
	}
}

// SmallChunkSizeConfig returns configuration for smaller chunks.
func SmallChunkSizeConfig() ChunkSizeConfig {
	return ChunkSizeConfig{
		MinTokens:    20,
		TargetTokens: 128,
		MaxTokens:    256,
	}
}

// Validate ensures the chunk size config is valid.
func (c ChunkSizeConfig) Validate() error {
	if c.MinTokens < 0 {
		return ErrInvalidMinTokens
	}
	if c.TargetTokens < c.MinTokens {
		return ErrTargetLessThanMin
	}
	if c.MaxTokens < c.TargetTokens {
		return ErrMaxLessThanTarget
	}
	return nil
}

// BoundaryType represents the type of boundary used for chunking.
type BoundaryType string

const (
	// BoundarySentence splits at sentence boundaries.
	BoundarySentence BoundaryType = "sentence"

	// BoundaryParagraph splits at paragraph boundaries.
	BoundaryParagraph BoundaryType = "paragraph"

	// BoundarySection splits at section/heading boundaries.
	BoundarySection BoundaryType = "section"

	// BoundaryToken splits at token boundaries (words).
	BoundaryToken BoundaryType = "token"

	// BoundarySemantic splits at semantic topic shifts.
	BoundarySemantic BoundaryType = "semantic"
)

// BoundaryDetection configures how text boundaries are detected.
type BoundaryDetection struct {
	// Primary is the preferred boundary type.
	Primary BoundaryType

	// Fallback is used when primary cannot be applied.
	Fallback BoundaryType

	// PreserveHeadings keeps markdown/HTML headings intact.
	PreserveHeadings bool

	// PreserveLists keeps list items together when possible.
	PreserveLists bool

	// PreserveCodeBlocks keeps code blocks intact when possible.
	PreserveCodeBlocks bool

	// RespectMaxTokens forces a split even at non-boundaries if max is exceeded.
	RespectMaxTokens bool
}

// DefaultBoundaryDetection returns sensible defaults.
func DefaultBoundaryDetection() BoundaryDetection {
	return BoundaryDetection{
		Primary:            BoundarySentence,
		Fallback:           BoundaryToken,
		PreserveHeadings:   true,
		PreserveLists:      true,
		PreserveCodeBlocks: true,
		RespectMaxTokens:   true,
	}
}

// ChunkingStrategy combines all configuration for a chunking approach.
type ChunkingStrategy struct {
	// Name identifies this strategy.
	Name string

	// Description explains when to use this strategy.
	Description string

	// SizeConfig defines token limits.
	SizeConfig ChunkSizeConfig

	// OverlapConfig defines overlap behavior.
	OverlapConfig OverlapConfig

	// BoundaryConfig defines boundary detection.
	BoundaryConfig BoundaryDetection

	// ChunkerType is the type of chunker to use (token, sentence, paragraph, semantic, hierarchical).
	ChunkerType string
}

// PredefinedStrategies provides common chunking configurations.
var PredefinedStrategies = map[string]ChunkingStrategy{
	"default": {
		Name:           "default",
		Description:    "Balanced chunking for general use",
		SizeConfig:     DefaultChunkSizeConfig(),
		OverlapConfig:  DefaultOverlapConfig(),
		BoundaryConfig: DefaultBoundaryDetection(),
		ChunkerType:    "sentence",
	},
	"retrieval": {
		Name:        "retrieval",
		Description: "Optimized for RAG/retrieval systems with smaller chunks",
		SizeConfig: ChunkSizeConfig{
			MinTokens:    30,
			TargetTokens: 128,
			MaxTokens:    256,
		},
		OverlapConfig: OverlapConfig{
			OverlapPercentage: 15,
			MinOverlap:        20,
			MaxOverlap:        50,
		},
		BoundaryConfig: DefaultBoundaryDetection(),
		ChunkerType:    "sentence",
	},
	"summarization": {
		Name:        "summarization",
		Description: "Larger chunks for summarization tasks",
		SizeConfig: ChunkSizeConfig{
			MinTokens:    200,
			TargetTokens: 1000,
			MaxTokens:    2000,
		},
		OverlapConfig: OverlapConfig{
			OverlapPercentage: 5,
			MinOverlap:        50,
			MaxOverlap:        100,
		},
		BoundaryConfig: BoundaryDetection{
			Primary:          BoundaryParagraph,
			Fallback:         BoundarySentence,
			PreserveHeadings: true,
			RespectMaxTokens: true,
		},
		ChunkerType: "paragraph",
	},
	"qa": {
		Name:        "qa",
		Description: "Medium chunks for question-answering",
		SizeConfig: ChunkSizeConfig{
			MinTokens:    50,
			TargetTokens: 300,
			MaxTokens:    500,
		},
		OverlapConfig: OverlapConfig{
			OverlapPercentage: 20,
			MinOverlap:        30,
			MaxOverlap:        100,
		},
		BoundaryConfig: BoundaryDetection{
			Primary:            BoundarySentence,
			Fallback:           BoundaryToken,
			PreserveHeadings:   true,
			PreserveLists:      true,
			PreserveCodeBlocks: true,
			RespectMaxTokens:   true,
		},
		ChunkerType: "sentence",
	},
	"code": {
		Name:        "code",
		Description: "Optimized for code content",
		SizeConfig: ChunkSizeConfig{
			MinTokens:    50,
			TargetTokens: 200,
			MaxTokens:    400,
		},
		OverlapConfig: OverlapConfig{
			OverlapPercentage: 10,
			MinOverlap:        20,
			MaxOverlap:        50,
		},
		BoundaryConfig: BoundaryDetection{
			Primary:            BoundarySection,
			Fallback:           BoundaryToken,
			PreserveCodeBlocks: true,
			RespectMaxTokens:   true,
		},
		ChunkerType: "paragraph",
	},
}

// GetStrategy returns a predefined strategy by name.
func GetStrategy(name string) (ChunkingStrategy, bool) {
	strategy, ok := PredefinedStrategies[name]
	return strategy, ok
}

// CreateChunkerFromStrategy creates a chunker from a strategy configuration.
func CreateChunkerFromStrategy(strategy ChunkingStrategy, tokenizer Tokenizer) Chunker {
	factory := NewChunkerFactory(strategy.SizeConfig, strategy.OverlapConfig, tokenizer)
	return factory.Create(strategy.ChunkerType)
}
