package chunking

import (
	"testing"
)

// =============================================================================
// OverlapConfig Tests
// =============================================================================

func TestOverlapConfig_GetOverlapTokens_FixedTokens(t *testing.T) {
	config := OverlapConfig{
		OverlapTokens: 50,
	}

	result := config.GetOverlapTokens(200)
	if result != 50 {
		t.Errorf("expected 50 overlap tokens, got %d", result)
	}
}

func TestOverlapConfig_GetOverlapTokens_Percentage(t *testing.T) {
	config := OverlapConfig{
		OverlapPercentage: 10,
	}

	result := config.GetOverlapTokens(200)
	if result != 20 {
		t.Errorf("expected 20 overlap tokens (10%% of 200), got %d", result)
	}
}

func TestOverlapConfig_GetOverlapTokens_MinBound(t *testing.T) {
	config := OverlapConfig{
		OverlapPercentage: 5,
		MinOverlap:        50,
	}

	// 5% of 200 = 10, but min is 50
	result := config.GetOverlapTokens(200)
	if result != 50 {
		t.Errorf("expected 50 overlap tokens (min bound), got %d", result)
	}
}

func TestOverlapConfig_GetOverlapTokens_MaxBound(t *testing.T) {
	config := OverlapConfig{
		OverlapPercentage: 50,
		MaxOverlap:        30,
	}

	// 50% of 200 = 100, but max is 30
	result := config.GetOverlapTokens(200)
	if result != 30 {
		t.Errorf("expected 30 overlap tokens (max bound), got %d", result)
	}
}

func TestOverlapConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  OverlapConfig
		wantErr error
	}{
		{
			name:    "valid config",
			config:  DefaultOverlapConfig(),
			wantErr: nil,
		},
		{
			name: "invalid percentage - too high",
			config: OverlapConfig{
				OverlapPercentage: 60,
			},
			wantErr: ErrInvalidOverlapPercentage,
		},
		{
			name: "invalid percentage - negative",
			config: OverlapConfig{
				OverlapPercentage: -5,
			},
			wantErr: ErrInvalidOverlapPercentage,
		},
		{
			name: "invalid min overlap - negative",
			config: OverlapConfig{
				MinOverlap: -10,
			},
			wantErr: ErrInvalidMinOverlap,
		},
		{
			name: "max less than min",
			config: OverlapConfig{
				MinOverlap: 100,
				MaxOverlap: 50,
			},
			wantErr: ErrMaxLessThanMin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// =============================================================================
// ChunkSizeConfig Tests
// =============================================================================

func TestChunkSizeConfig_Defaults(t *testing.T) {
	config := DefaultChunkSizeConfig()

	if config.MinTokens <= 0 {
		t.Error("default min tokens should be positive")
	}
	if config.TargetTokens <= config.MinTokens {
		t.Error("default target should be greater than min")
	}
	if config.MaxTokens <= config.TargetTokens {
		t.Error("default max should be greater than target")
	}
}

func TestChunkSizeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ChunkSizeConfig
		wantErr error
	}{
		{
			name:    "valid config",
			config:  DefaultChunkSizeConfig(),
			wantErr: nil,
		},
		{
			name: "invalid min tokens - negative",
			config: ChunkSizeConfig{
				MinTokens:    -10,
				TargetTokens: 100,
				MaxTokens:    200,
			},
			wantErr: ErrInvalidMinTokens,
		},
		{
			name: "target less than min",
			config: ChunkSizeConfig{
				MinTokens:    100,
				TargetTokens: 50,
				MaxTokens:    200,
			},
			wantErr: ErrTargetLessThanMin,
		},
		{
			name: "max less than target",
			config: ChunkSizeConfig{
				MinTokens:    50,
				TargetTokens: 200,
				MaxTokens:    100,
			},
			wantErr: ErrMaxLessThanTarget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestChunkSizeConfig_Presets(t *testing.T) {
	tests := []struct {
		name   string
		config ChunkSizeConfig
	}{
		{"default", DefaultChunkSizeConfig()},
		{"large", LargeChunkSizeConfig()},
		{"small", SmallChunkSizeConfig()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); err != nil {
				t.Errorf("preset config '%s' failed validation: %v", tt.name, err)
			}
		})
	}
}

// =============================================================================
// BoundaryDetection Tests
// =============================================================================

func TestBoundaryDetection_Defaults(t *testing.T) {
	config := DefaultBoundaryDetection()

	if config.Primary != BoundarySentence {
		t.Errorf("expected primary boundary to be sentence, got %s", config.Primary)
	}
	if config.Fallback != BoundaryToken {
		t.Errorf("expected fallback boundary to be token, got %s", config.Fallback)
	}
}

// =============================================================================
// ChunkingStrategy Tests
// =============================================================================

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"default", true},
		{"retrieval", true},
		{"summarization", true},
		{"qa", true},
		{"code", true},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, found := GetStrategy(tt.name)
			if found != tt.found {
				t.Errorf("expected found=%v for strategy '%s'", tt.found, tt.name)
			}
			if found {
				if strategy.Name != tt.name {
					t.Errorf("expected strategy name '%s', got '%s'", tt.name, strategy.Name)
				}
				// Validate the strategy configuration
				if err := strategy.SizeConfig.Validate(); err != nil {
					t.Errorf("strategy '%s' has invalid size config: %v", tt.name, err)
				}
				if err := strategy.OverlapConfig.Validate(); err != nil {
					t.Errorf("strategy '%s' has invalid overlap config: %v", tt.name, err)
				}
			}
		})
	}
}

func TestCreateChunkerFromStrategy(t *testing.T) {
	strategies := []string{"default", "retrieval", "summarization", "qa", "code"}

	for _, name := range strategies {
		t.Run(name, func(t *testing.T) {
			strategy, ok := GetStrategy(name)
			if !ok {
				t.Fatalf("strategy '%s' not found", name)
			}

			chunker := CreateChunkerFromStrategy(strategy, nil)
			if chunker == nil {
				t.Fatal("expected non-nil chunker")
			}

			// Verify the chunker can be used
			config := chunker.GetConfig()
			if config.MinTokens <= 0 {
				t.Error("chunker should have valid min tokens")
			}
		})
	}
}

func TestPredefinedStrategies_Complete(t *testing.T) {
	// Ensure all predefined strategies have required fields
	for name, strategy := range PredefinedStrategies {
		t.Run(name, func(t *testing.T) {
			if strategy.Name == "" {
				t.Error("strategy should have a name")
			}
			if strategy.Description == "" {
				t.Error("strategy should have a description")
			}
			if strategy.ChunkerType == "" {
				t.Error("strategy should have a chunker type")
			}
			if err := strategy.SizeConfig.Validate(); err != nil {
				t.Errorf("invalid size config: %v", err)
			}
			if err := strategy.OverlapConfig.Validate(); err != nil {
				t.Errorf("invalid overlap config: %v", err)
			}
		})
	}
}

// =============================================================================
// Boundary Types Tests
// =============================================================================

func TestBoundaryTypes(t *testing.T) {
	types := []BoundaryType{
		BoundarySentence,
		BoundaryParagraph,
		BoundarySection,
		BoundaryToken,
		BoundarySemantic,
	}

	for _, bt := range types {
		if bt == "" {
			t.Error("boundary type should not be empty")
		}
	}
}
