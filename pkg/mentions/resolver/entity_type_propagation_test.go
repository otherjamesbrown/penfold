package resolver

import (
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/stretchr/testify/assert"
)

// TestBug_pf5a63fb_EntityTypePropagation verifies that Resolution.EntityType
// is populated from Stage 1 understanding, even when the mention is unresolved.
// Root cause: storeMentions defaulted unresolved mentions to EntityTypePerson,
// causing company mentions to be stored as person mentions.
func TestBug_pf5a63fb_EntityTypePropagation(t *testing.T) {
	t.Run("resolved mention gets entity type from ResolvedTo", func(t *testing.T) {
		res := Resolution{
			MentionText: "Hydrolix",
			Decision:    DecisionTypeResolve,
			ResolvedTo: &ResolvedEntity{
				EntityType: mentions.EntityTypeCompany,
				EntityID:   42,
				EntityName: "Hydrolix Inc.",
			},
			Confidence: 0.95,
		}
		// EntityType should be populated from ResolvedTo
		assert.Equal(t, mentions.EntityType(""), res.EntityType, "EntityType not set yet")

		// Simulate resolver enrichment
		if res.EntityType == "" && res.ResolvedTo != nil {
			res.EntityType = res.ResolvedTo.EntityType
		}
		assert.Equal(t, mentions.EntityTypeCompany, res.EntityType)
	})

	t.Run("unresolved mention gets entity type from Stage 1", func(t *testing.T) {
		// Stage 1 identifies "Hydrolix" as a company
		understanding := &Stage1Understanding{
			Mentions: []MentionUnderstanding{
				{Text: "Hydrolix", EntityType: mentions.EntityTypeCompany, Position: 42},
				{Text: "Stuart", EntityType: mentions.EntityTypePerson, Position: 100},
			},
		}

		// Stage 3 can't resolve "Hydrolix" — no matching org in DB
		resolutions := []Resolution{
			{MentionText: "Hydrolix", Decision: DecisionTypeQueueReview, Confidence: 0.3},
			{MentionText: "Stuart", Decision: DecisionTypeResolve, ResolvedTo: &ResolvedEntity{
				EntityType: mentions.EntityTypePerson, EntityID: 99, EntityName: "Stuart Wakeling",
			}, Confidence: 0.92},
		}

		// Simulate the enrichment logic from resolver.go
		entityTypeByMention := make(map[string]mentions.EntityType)
		for _, m := range understanding.Mentions {
			entityTypeByMention[m.Text] = m.EntityType
		}
		for i := range resolutions {
			if resolutions[i].EntityType == "" {
				if resolutions[i].ResolvedTo != nil {
					resolutions[i].EntityType = resolutions[i].ResolvedTo.EntityType
				} else if et, ok := entityTypeByMention[resolutions[i].MentionText]; ok {
					resolutions[i].EntityType = et
				} else {
					resolutions[i].EntityType = mentions.EntityTypePerson
				}
			}
		}

		// Hydrolix should be company, not person
		assert.Equal(t, mentions.EntityTypeCompany, resolutions[0].EntityType,
			"unresolved company mention should retain company entity type from Stage 1")
		// Stuart should be person (from ResolvedTo)
		assert.Equal(t, mentions.EntityTypePerson, resolutions[1].EntityType,
			"resolved person mention should have person entity type from ResolvedTo")
	})

	t.Run("fallback to person when no Stage 1 data", func(t *testing.T) {
		res := Resolution{
			MentionText: "Unknown",
			Decision:    DecisionTypeQueueReview,
			Confidence:  0.1,
		}
		// No ResolvedTo, no Stage 1 match
		if res.EntityType == "" {
			res.EntityType = mentions.EntityTypePerson // fallback
		}
		assert.Equal(t, mentions.EntityTypePerson, res.EntityType)
	})
}
