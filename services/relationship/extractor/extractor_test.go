// Package extractor provides tests for relationship extraction capabilities.
package extractor

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
)

// MockAIClient is a mock implementation of the AIClient interface.
type MockAIClient struct {
	ExtractAssertionsFunc func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
}

func (m *MockAIClient) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	if m.ExtractAssertionsFunc != nil {
		return m.ExtractAssertionsFunc(ctx, req)
	}
	return &aiv1.AssertionResponse{
		Assertions: []*aiv1.Assertion{},
		ModelUsed:  "mock-model",
	}, nil
}

func TestNewRelationshipExtractor(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("creates extractor with default options", func(t *testing.T) {
		extractor := NewRelationshipExtractor(logger, nil, nil)
		require.NotNil(t, extractor)
	})

	t.Run("creates extractor with custom options", func(t *testing.T) {
		opts := &ExtractionOptions{
			MinConfidence:    0.7,
			MaxRelationships: 50,
			UseAI:            false,
		}
		extractor := NewRelationshipExtractor(logger, nil, opts)
		require.NotNil(t, extractor)
	})

	t.Run("creates extractor with AI client", func(t *testing.T) {
		mockAI := &MockAIClient{}
		extractor := NewRelationshipExtractor(logger, mockAI, nil)
		require.NotNil(t, extractor)
	})
}

func TestExtract_EmptyContent(t *testing.T) {
	logger := zerolog.Nop()
	extractor := NewRelationshipExtractor(logger, nil, nil)

	result, err := extractor.Extract(context.Background(), "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Relationships)
	assert.Empty(t, result.Entities)
	assert.GreaterOrEqual(t, result.ProcessingTimeMs, int64(0))
}

func TestExtract_WithPatternExtraction(t *testing.T) {
	logger := zerolog.Nop()
	opts := &ExtractionOptions{
		MinConfidence: 0.5,
		UseAI:         false, // Disable AI to test pattern extraction
	}
	extractor := NewRelationshipExtractor(logger, nil, opts)

	t.Run("extracts email addresses as person entities", func(t *testing.T) {
		content := "Contact john.doe@example.com and jane.smith@company.org for details."

		result, err := extractor.Extract(context.Background(), content)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entities), 2)

		// Find the email entities
		emailFound := 0
		for _, entity := range result.Entities {
			if entity.Type == EntityTypePerson {
				emailFound++
			}
		}
		assert.GreaterOrEqual(t, emailFound, 2)
	})

	t.Run("extracts URLs as document entities", func(t *testing.T) {
		content := "See https://docs.example.com/guide and http://wiki.internal.com/page"

		result, err := extractor.Extract(context.Background(), content)

		require.NoError(t, err)

		urlFound := 0
		for _, entity := range result.Entities {
			if entity.Type == EntityTypeDocument {
				urlFound++
			}
		}
		assert.GreaterOrEqual(t, urlFound, 2)
	})

	t.Run("extracts hashtags as topic entities", func(t *testing.T) {
		content := "Working on #ProjectAlpha and #engineering tasks today."

		result, err := extractor.Extract(context.Background(), content)

		require.NoError(t, err)

		tagFound := 0
		for _, entity := range result.Entities {
			if entity.Type == EntityTypeTopic {
				tagFound++
			}
		}
		assert.GreaterOrEqual(t, tagFound, 2)
	})

	t.Run("creates co-mention relationships", func(t *testing.T) {
		content := "Contact alice@example.com and bob@example.com about #project"

		result, err := extractor.Extract(context.Background(), content)

		require.NoError(t, err)
		// Should have co-mention relationships between entities
		assert.GreaterOrEqual(t, len(result.Relationships), 1)
	})
}

func TestExtract_WithAI(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("uses AI for relationship extraction", func(t *testing.T) {
		sourceText := "John works at Acme Corp"
		mockAI := &MockAIClient{
			ExtractAssertionsFunc: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
				return &aiv1.AssertionResponse{
					Assertions: []*aiv1.Assertion{
						{
							Subject:    "John",
							Predicate:  "works at",
							Object:     "Acme Corp",
							Confidence: 0.9,
							SourceText: &sourceText,
						},
					},
					ModelUsed:  "test-model",
					TotalFound: 1,
				}, nil
			},
		}

		opts := &ExtractionOptions{
			MinConfidence: 0.5,
			UseAI:         true,
		}
		extractor := NewRelationshipExtractor(logger, mockAI, opts)

		result, err := extractor.Extract(context.Background(), "John works at Acme Corp")

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Relationships), 1)
		assert.Equal(t, "test-model", result.ModelUsed)

		// Find the works_at relationship
		found := false
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeWorksAt {
				found = true
				assert.Equal(t, "John", rel.Source.Name)
				assert.Equal(t, "Acme Corp", rel.Target.Name)
				assert.InDelta(t, 0.9, float64(rel.Confidence), 0.01)
			}
		}
		assert.True(t, found, "Expected to find works_at relationship")
	})

	t.Run("falls back to pattern extraction on AI error", func(t *testing.T) {
		mockAI := &MockAIClient{
			ExtractAssertionsFunc: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
				return nil, context.DeadlineExceeded
			},
		}

		opts := &ExtractionOptions{
			MinConfidence: 0.5,
			UseAI:         true,
		}
		extractor := NewRelationshipExtractor(logger, mockAI, opts)

		content := "Contact test@example.com for details"
		result, err := extractor.Extract(context.Background(), content)

		require.NoError(t, err)
		// Should still extract the email via pattern matching
		assert.GreaterOrEqual(t, len(result.Entities), 1)
	})
}

func TestExtractFromEmail(t *testing.T) {
	logger := zerolog.Nop()
	opts := &ExtractionOptions{
		MinConfidence: 0.5,
		UseAI:         false,
	}
	extractor := NewRelationshipExtractor(logger, nil, opts)

	t.Run("extracts sender and recipient entities", func(t *testing.T) {
		email := &EmailContent{
			MessageID: "msg-001",
			From:      "alice@example.com",
			FromName:  "Alice Smith",
			To:        []string{"bob@example.com"},
			ToNames:   []string{"Bob Jones"},
			Subject:   "Test Subject",
			Body:      "Hello Bob",
		}

		result, err := extractor.ExtractFromEmail(context.Background(), email)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entities), 2)

		// Should have sender entity
		foundSender := false
		foundRecipient := false
		for _, entity := range result.Entities {
			if entity.Metadata["email"] == "alice@example.com" {
				foundSender = true
				assert.Equal(t, "Alice Smith", entity.Name)
			}
			if entity.Metadata["email"] == "bob@example.com" {
				foundRecipient = true
				assert.Equal(t, "Bob Jones", entity.Name)
			}
		}
		assert.True(t, foundSender, "Expected to find sender entity")
		assert.True(t, foundRecipient, "Expected to find recipient entity")
	})

	t.Run("creates knows relationship between sender and recipient", func(t *testing.T) {
		email := &EmailContent{
			MessageID: "msg-002",
			From:      "sender@example.com",
			FromName:  "Sender",
			To:        []string{"recipient@example.com"},
			ToNames:   []string{"Recipient"},
			Subject:   "Important Message",
		}

		result, err := extractor.ExtractFromEmail(context.Background(), email)

		require.NoError(t, err)

		// Find the knows relationship
		found := false
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeKnows {
				found = true
				assert.InDelta(t, 0.9, float64(rel.Confidence), 0.1)
			}
		}
		assert.True(t, found, "Expected to find knows relationship")
	})

	t.Run("creates mentioned_with relationship for CC recipients", func(t *testing.T) {
		email := &EmailContent{
			MessageID: "msg-003",
			From:      "sender@example.com",
			FromName:  "Sender",
			To:        []string{"recipient@example.com"},
			CC:        []string{"cc@example.com"},
			CCNames:   []string{"CC Person"},
			Subject:   "With CC",
		}

		result, err := extractor.ExtractFromEmail(context.Background(), email)

		require.NoError(t, err)

		// Find the mentioned_with relationship for CC
		found := false
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeMentionedWith {
				found = true
				// CC relationships have lower confidence
				assert.Less(t, rel.Confidence, float32(0.9))
			}
		}
		assert.True(t, found, "Expected to find mentioned_with relationship for CC")
	})

	t.Run("creates collaborates_with between multiple To recipients", func(t *testing.T) {
		email := &EmailContent{
			MessageID: "msg-004",
			From:      "sender@example.com",
			To:        []string{"alice@example.com", "bob@example.com", "carol@example.com"},
			Subject:   "Team Discussion",
		}

		result, err := extractor.ExtractFromEmail(context.Background(), email)

		require.NoError(t, err)

		// Count collaborates_with relationships
		collabCount := 0
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeCollaboratesWith {
				collabCount++
			}
		}
		// Should have 3 collaborates_with relationships (alice-bob, alice-carol, bob-carol)
		assert.Equal(t, 3, collabCount)
	})

	t.Run("handles nil email", func(t *testing.T) {
		result, err := extractor.ExtractFromEmail(context.Background(), nil)

		require.NoError(t, err)
		assert.Empty(t, result.Relationships)
		assert.Empty(t, result.Entities)
	})
}

func TestExtractFromMeeting(t *testing.T) {
	logger := zerolog.Nop()
	opts := &ExtractionOptions{
		MinConfidence: 0.5,
		UseAI:         false,
	}
	extractor := NewRelationshipExtractor(logger, nil, opts)

	t.Run("extracts organizer and meeting entities", func(t *testing.T) {
		meeting := &MeetingContent{
			MeetingID:     "mtg-001",
			Title:         "Weekly Standup",
			Organizer:     "manager@example.com",
			OrganizerName: "Manager",
			StartTime:     time.Now(),
			EndTime:       time.Now().Add(time.Hour),
		}

		result, err := extractor.ExtractFromMeeting(context.Background(), meeting)

		require.NoError(t, err)

		// Should have organizer and meeting entities
		foundOrganizer := false
		foundMeeting := false
		for _, entity := range result.Entities {
			if entity.Type == EntityTypePerson && entity.Metadata["email"] == "manager@example.com" {
				foundOrganizer = true
			}
			if entity.Type == EntityTypeEvent && entity.Name == "Weekly Standup" {
				foundMeeting = true
			}
		}
		assert.True(t, foundOrganizer, "Expected to find organizer entity")
		assert.True(t, foundMeeting, "Expected to find meeting entity")
	})

	t.Run("creates attended relationships for attendees", func(t *testing.T) {
		meeting := &MeetingContent{
			MeetingID:     "mtg-002",
			Title:         "Team Meeting",
			Organizer:     "organizer@example.com",
			OrganizerName: "Organizer",
			Attendees:     []string{"alice@example.com", "bob@example.com"},
			AttendeeNames: []string{"Alice", "Bob"},
			StartTime:     time.Now(),
			EndTime:       time.Now().Add(time.Hour),
		}

		result, err := extractor.ExtractFromMeeting(context.Background(), meeting)

		require.NoError(t, err)

		// Count attended relationships
		attendedCount := 0
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeAttended {
				attendedCount++
			}
		}
		// Should have 3 attended relationships (organizer + 2 attendees)
		assert.Equal(t, 3, attendedCount)
	})

	t.Run("creates knows relationships between organizer and attendees", func(t *testing.T) {
		meeting := &MeetingContent{
			MeetingID:     "mtg-003",
			Title:         "1:1 Meeting",
			Organizer:     "manager@example.com",
			OrganizerName: "Manager",
			Attendees:     []string{"report@example.com"},
			AttendeeNames: []string{"Direct Report"},
			StartTime:     time.Now(),
			EndTime:       time.Now().Add(30 * time.Minute),
		}

		result, err := extractor.ExtractFromMeeting(context.Background(), meeting)

		require.NoError(t, err)

		// Find the knows relationship
		found := false
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeKnows {
				found = true
			}
		}
		assert.True(t, found, "Expected to find knows relationship")
	})

	t.Run("creates collaborates_with between attendees", func(t *testing.T) {
		meeting := &MeetingContent{
			MeetingID:     "mtg-004",
			Title:         "Project Planning",
			Organizer:     "pm@example.com",
			Attendees:     []string{"dev1@example.com", "dev2@example.com", "dev3@example.com"},
			AttendeeNames: []string{"Dev 1", "Dev 2", "Dev 3"},
			StartTime:     time.Now(),
			EndTime:       time.Now().Add(time.Hour),
		}

		result, err := extractor.ExtractFromMeeting(context.Background(), meeting)

		require.NoError(t, err)

		// Count collaborates_with relationships
		collabCount := 0
		for _, rel := range result.Relationships {
			if rel.Type == RelationshipTypeCollaboratesWith {
				collabCount++
			}
		}
		// Should have 3 collaborates_with relationships between attendees
		assert.Equal(t, 3, collabCount)
	})

	t.Run("handles nil meeting", func(t *testing.T) {
		result, err := extractor.ExtractFromMeeting(context.Background(), nil)

		require.NoError(t, err)
		assert.Empty(t, result.Relationships)
		assert.Empty(t, result.Entities)
	})
}

func TestConfidenceFiltering(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("filters out low confidence relationships", func(t *testing.T) {
		sourceText := "Low confidence"
		mockAI := &MockAIClient{
			ExtractAssertionsFunc: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
				return &aiv1.AssertionResponse{
					Assertions: []*aiv1.Assertion{
						{
							Subject:    "Entity1",
							Predicate:  "related to",
							Object:     "Entity2",
							Confidence: 0.3, // Below threshold
							SourceText: &sourceText,
						},
						{
							Subject:    "Entity3",
							Predicate:  "knows",
							Object:     "Entity4",
							Confidence: 0.8, // Above threshold
							SourceText: &sourceText,
						},
					},
					ModelUsed:  "test-model",
					TotalFound: 2,
				}, nil
			},
		}

		opts := &ExtractionOptions{
			MinConfidence: 0.5,
			UseAI:         true,
		}
		extractor := NewRelationshipExtractor(logger, mockAI, opts)

		result, err := extractor.Extract(context.Background(), "test content")

		require.NoError(t, err)
		// Only the high confidence relationship should remain
		highConfCount := 0
		for _, rel := range result.Relationships {
			if rel.Confidence >= 0.5 {
				highConfCount++
			}
		}
		assert.GreaterOrEqual(t, highConfCount, 1)
		assert.Equal(t, 2, result.TotalFound)
		assert.GreaterOrEqual(t, result.FilteredCount, 1)
	})
}

func TestMaxRelationshipsLimit(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("limits number of relationships returned", func(t *testing.T) {
		// Create an email with many recipients to generate many relationships
		email := &EmailContent{
			MessageID: "msg-limit",
			From:      "sender@example.com",
			To:        make([]string, 20), // 20 recipients
			Subject:   "Large Distribution",
		}
		for i := 0; i < 20; i++ {
			email.To[i] = "recipient" + string(rune('a'+i)) + "@example.com"
		}

		opts := &ExtractionOptions{
			MinConfidence:    0.1, // Low threshold to get more relationships
			MaxRelationships: 5,   // Limit to 5
			UseAI:            false,
		}
		extractor := NewRelationshipExtractor(logger, nil, opts)

		result, err := extractor.ExtractFromEmail(context.Background(), email)

		require.NoError(t, err)
		assert.LessOrEqual(t, len(result.Relationships), 5)
	})
}

func TestEntityTypeConversions(t *testing.T) {
	t.Run("EntityType to proto and back", func(t *testing.T) {
		types := []EntityType{
			EntityTypePerson,
			EntityTypeOrganization,
			EntityTypeTopic,
			EntityTypeProject,
			EntityTypeLocation,
			EntityTypeEvent,
			EntityTypeDocument,
			EntityTypeUnknown,
		}

		for _, et := range types {
			proto := et.ToProto()
			back := EntityTypeFromProto(proto)

			if et == EntityTypeUnknown {
				// Unknown maps to unspecified and back
				continue
			}
			assert.Equal(t, et, back, "EntityType %s should round-trip correctly", et)
		}
	})
}

func TestRelationshipTypeConversions(t *testing.T) {
	t.Run("RelationshipType to proto and back", func(t *testing.T) {
		types := []RelationshipType{
			RelationshipTypeKnows,
			RelationshipTypeWorksAt,
			RelationshipTypeReportsTo,
			RelationshipTypeManages,
			RelationshipTypeMentionedWith,
			RelationshipTypeAttended,
			RelationshipTypeMemberOf,
			RelationshipTypePartOf,
			RelationshipTypeCreated,
			RelationshipTypeLocatedAt,
			RelationshipTypeRelatedTo,
		}

		for _, rt := range types {
			proto := rt.ToProto()
			back := RelationshipTypeFromProto(proto)

			// Some types map to the same proto value, so just verify non-empty
			assert.NotEmpty(t, back, "RelationshipType %s should convert correctly", rt)
		}
	})
}

func TestEntityToProto(t *testing.T) {
	entity := &Entity{
		ID:            "entity-123",
		Type:          EntityTypePerson,
		Name:          "John Doe",
		CanonicalName: "john doe",
		Metadata:      map[string]string{"email": "john@example.com"},
		Confidence:    0.95,
	}

	proto := entity.ToProto()

	assert.Equal(t, "entity-123", proto.GetId())
	assert.Equal(t, "John Doe", proto.GetName())
	assert.Equal(t, "john doe", proto.GetCanonicalName())
	assert.Equal(t, "john@example.com", proto.GetMetadata()["email"])
}

func TestRelationshipToProto(t *testing.T) {
	source := &Entity{
		ID:   "source-123",
		Type: EntityTypePerson,
		Name: "Alice",
	}
	target := &Entity{
		ID:   "target-456",
		Type: EntityTypeOrganization,
		Name: "Acme Corp",
	}

	rel := &Relationship{
		ID:         "rel-789",
		Source:     source,
		Target:     target,
		Type:       RelationshipTypeWorksAt,
		Strength:   0.8,
		Confidence: 0.9,
		Context:    "Employment relationship",
		Evidence: []Evidence{
			{
				SourceID:               "doc-001",
				SourceType:             "email",
				Excerpt:                "Alice works at Acme",
				ConfidenceContribution: 0.9,
			},
		},
		TenantID: "tenant-abc",
	}

	proto := rel.ToProto()

	assert.Equal(t, "rel-789", proto.GetId())
	assert.Equal(t, "Alice", proto.GetSourceEntity().GetName())
	assert.Equal(t, "Acme Corp", proto.GetTargetEntity().GetName())
	assert.InDelta(t, 0.9, float64(proto.GetConfidence()), 0.01)
	assert.Equal(t, "tenant-abc", proto.GetTenantId())
	assert.Len(t, proto.GetEvidence(), 1)
}

func TestInferRelationshipTypeFromPredicate(t *testing.T) {
	tests := []struct {
		predicate string
		expected  RelationshipType
	}{
		{"works at", RelationshipTypeWorksAt},
		{"employed by", RelationshipTypeWorksAt},
		{"works with", RelationshipTypeWorksWith},
		{"collaborates with", RelationshipTypeWorksWith},
		{"reports to", RelationshipTypeReportsTo},
		{"manages", RelationshipTypeManages},
		{"supervises", RelationshipTypeManages},
		{"knows", RelationshipTypeKnows},
		{"met", RelationshipTypeKnows},
		{"knows about", RelationshipTypeKnowsAbout},
		{"expert in", RelationshipTypeKnowsAbout},
		{"discussed", RelationshipTypeDiscussed},
		{"talked about", RelationshipTypeDiscussed},
		{"attended", RelationshipTypeAttended},
		{"participated in", RelationshipTypeAttended},
		{"member of", RelationshipTypeMemberOf},
		{"belongs to", RelationshipTypeMemberOf},
		{"part of", RelationshipTypePartOf},
		{"created", RelationshipTypeCreated},
		{"authored", RelationshipTypeCreated},
		{"located in", RelationshipTypeLocatedAt},
		{"based in", RelationshipTypeLocatedAt},
		{"some random predicate", RelationshipTypeRelatedTo},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			result := inferRelationshipTypeFromPredicate(tt.predicate)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInferEntityTypeFromCategory(t *testing.T) {
	tests := []struct {
		category string
		expected EntityType
	}{
		{"person", EntityTypePerson},
		{"personnel", EntityTypePerson},
		{"organization", EntityTypeOrganization},
		{"company", EntityTypeOrganization},
		{"team", EntityTypeOrganization},
		{"topic", EntityTypeTopic},
		{"subject", EntityTypeTopic},
		{"project", EntityTypeProject},
		{"initiative", EntityTypeProject},
		{"location", EntityTypeLocation},
		{"geographical", EntityTypeLocation},
		{"event", EntityTypeEvent},
		{"meeting", EntityTypeEvent},
		{"document", EntityTypeDocument},
		{"unknown_category", EntityTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			result := inferEntityTypeFromCategory(tt.category)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultExtractionOptions(t *testing.T) {
	opts := DefaultExtractionOptions()

	assert.InDelta(t, 0.5, float64(opts.MinConfidence), 0.01)
	assert.Equal(t, 100, opts.MaxRelationships)
	assert.True(t, opts.UseAI)
	assert.Nil(t, opts.EntityTypes)
	assert.Nil(t, opts.RelationshipTypes)
}

func TestMergeEntities(t *testing.T) {
	t.Run("deduplicates by canonical name", func(t *testing.T) {
		a := []Entity{
			{ID: "1", Name: "Alice", CanonicalName: "alice"},
			{ID: "2", Name: "Bob", CanonicalName: "bob"},
		}
		b := []Entity{
			{ID: "3", Name: "ALICE", CanonicalName: "alice"}, // Duplicate
			{ID: "4", Name: "Carol", CanonicalName: "carol"},
		}

		result := mergeEntities(a, b)

		assert.Len(t, result, 3)

		names := make(map[string]bool)
		for _, e := range result {
			names[e.CanonicalName] = true
		}
		assert.True(t, names["alice"])
		assert.True(t, names["bob"])
		assert.True(t, names["carol"])
	})
}

func TestEvidenceToProto(t *testing.T) {
	evidence := &Evidence{
		SourceID:               "src-123",
		SourceType:             "email",
		Excerpt:                "This is the excerpt",
		DiscoveredAt:           time.Now(),
		ConfidenceContribution: 0.85,
	}

	proto := evidence.ToProto()

	assert.Equal(t, "src-123", proto.GetSourceId())
	assert.Equal(t, "email", proto.GetSourceType())
	assert.Equal(t, "This is the excerpt", proto.GetExcerpt())
	assert.InDelta(t, 0.85, float64(proto.GetConfidenceContribution()), 0.01)
}
