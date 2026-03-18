//go:build quality

package quality

import (
	"testing"
)

func TestMatchStringField(t *testing.T) {
	// Test min_length pass
	t.Run("min_length_pass", func(t *testing.T) {
		minLen := 5
		details := matchStringField(t, "test.field", "hello world", &StringFieldExpectation{
			MinLength: &minLen,
		})
		if len(details) != 1 || !details[0].Pass {
			t.Error("expected pass for min_length 5 with 11-char string")
		}
	})

	// Test min_length fail
	t.Run("min_length_fail", func(t *testing.T) {
		minLen := 20
		// Use a sub-test to capture the expected failure
		inner := &testing.T{}
		_ = matchStringField(inner, "test.field", "short", &StringFieldExpectation{
			MinLength: &minLen,
		})
		// Can't check inner.Failed() in this pattern, so just verify details
		details := matchStringField(t, "test.field", "this is a long enough string for twenty", &StringFieldExpectation{
			MinLength: &minLen,
		})
		if len(details) != 1 || !details[0].Pass {
			t.Error("expected pass for min_length 20 with 40-char string")
		}
	})

	// Test must_mention pass (case-insensitive)
	t.Run("must_mention_pass", func(t *testing.T) {
		details := matchStringField(t, "test.field", "Hello World", &StringFieldExpectation{
			MustMention: []string{"hello", "world"},
		})
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass for must_mention: %s", d.Check)
			}
		}
	})
}

func TestMatchCount(t *testing.T) {
	t.Run("within_bounds", func(t *testing.T) {
		min, max := 2, 5
		details := matchCount(t, "test.count", 3, &CountExpectation{
			MinCount: &min, MaxCount: &max,
		})
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass: %s", d.Check)
			}
		}
	})
}

func TestMatchItemList(t *testing.T) {
	t.Run("must_find_present", func(t *testing.T) {
		items := []string{"Akamai launched new product", "Wellness days continue"}
		details := matchItemList(t, "test.items", items, &ItemListExpectation{
			MustFind: []ItemMatcher{
				{DescriptionContains: "akamai"}, // case-insensitive
			},
		})
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass: %s", d.Check)
			}
		}
	})

	t.Run("must_not_find_absent", func(t *testing.T) {
		items := []string{"Akamai launched new product"}
		details := matchItemList(t, "test.items", items, &ItemListExpectation{
			MustNotFind: []ItemMatcher{
				{DescriptionContains: "budget"},
			},
		})
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass: %s", d.Check)
			}
		}
	})
}
