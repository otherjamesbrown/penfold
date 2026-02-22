package contentv1_test

import (
	"encoding/json"
	"testing"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"google.golang.org/protobuf/proto"
)

// TestProtoCompiles verifies that the generated package imports and enum
// constants are accessible without errors.
func TestProtoCompiles(t *testing.T) {
	// Access each enum type to confirm they compiled correctly.
	_ = contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED
	_ = contentv1.ContentSubtype_CONTENT_SUBTYPE_UNSPECIFIED
	_ = contentv1.ContentStructure_CONTENT_STRUCTURE_UNSPECIFIED
	_ = contentv1.TriageCategory_TRIAGE_CATEGORY_UNSPECIFIED
	_ = contentv1.TriageImportance_TRIAGE_IMPORTANCE_UNSPECIFIED
}

// TestEnumRoundTrip verifies that enum values survive JSON and binary proto
// marshal/unmarshal cycles.
func TestEnumRoundTrip(t *testing.T) {
	original := &contentv1.ContentItem{
		Id:                "test-id",
		ContentTypeEnum:   contentv1.ContentType_CONTENT_TYPE_EMAIL,
		ContentSubtypeEnum: contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN,
		ContentStructure:  contentv1.ContentStructure_CONTENT_STRUCTURE_STANDALONE,
		NotificationSource: "jira",
	}

	// Binary proto round-trip.
	t.Run("BinaryProto", func(t *testing.T) {
		data, err := proto.Marshal(original)
		if err != nil {
			t.Fatalf("proto.Marshal failed: %v", err)
		}
		decoded := &contentv1.ContentItem{}
		if err := proto.Unmarshal(data, decoded); err != nil {
			t.Fatalf("proto.Unmarshal failed: %v", err)
		}
		if decoded.ContentTypeEnum != original.ContentTypeEnum {
			t.Errorf("ContentTypeEnum: got %v, want %v", decoded.ContentTypeEnum, original.ContentTypeEnum)
		}
		if decoded.ContentSubtypeEnum != original.ContentSubtypeEnum {
			t.Errorf("ContentSubtypeEnum: got %v, want %v", decoded.ContentSubtypeEnum, original.ContentSubtypeEnum)
		}
		if decoded.ContentStructure != original.ContentStructure {
			t.Errorf("ContentStructure: got %v, want %v", decoded.ContentStructure, original.ContentStructure)
		}
		if decoded.NotificationSource != original.NotificationSource {
			t.Errorf("NotificationSource: got %q, want %q", decoded.NotificationSource, original.NotificationSource)
		}
	})

	// JSON round-trip for TriageCategory and TriageImportance via ProcessingStatus.
	t.Run("JSONProcessingStatus", func(t *testing.T) {
		ps := &contentv1.ProcessingStatus{
			ContentId:           "test-content",
			TriageCategoryEnum:  contentv1.TriageCategory_TRIAGE_CATEGORY_ACTION_REQUEST,
			TriageImportanceEnum: contentv1.TriageImportance_TRIAGE_IMPORTANCE_HIGH,
		}
		data, err := proto.Marshal(ps)
		if err != nil {
			t.Fatalf("proto.Marshal failed: %v", err)
		}
		decoded := &contentv1.ProcessingStatus{}
		if err := proto.Unmarshal(data, decoded); err != nil {
			t.Fatalf("proto.Unmarshal failed: %v", err)
		}
		if decoded.TriageCategoryEnum != ps.TriageCategoryEnum {
			t.Errorf("TriageCategoryEnum: got %v, want %v", decoded.TriageCategoryEnum, ps.TriageCategoryEnum)
		}
		if decoded.TriageImportanceEnum != ps.TriageImportanceEnum {
			t.Errorf("TriageImportanceEnum: got %v, want %v", decoded.TriageImportanceEnum, ps.TriageImportanceEnum)
		}
	})

	// JSON marshal to confirm numeric values survive.
	t.Run("JSONValues", func(t *testing.T) {
		vals := map[string]int32{
			"email":    int32(contentv1.ContentType_CONTENT_TYPE_EMAIL),
			"meeting":  int32(contentv1.ContentType_CONTENT_TYPE_MEETING),
			"calendar": int32(contentv1.ContentType_CONTENT_TYPE_CALENDAR),
			"document": int32(contentv1.ContentType_CONTENT_TYPE_DOCUMENT),
		}
		data, err := json.Marshal(vals)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var decoded map[string]int32
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded["email"] != 1 {
			t.Errorf("email enum value: got %d, want 1", decoded["email"])
		}
		if decoded["meeting"] != 2 {
			t.Errorf("meeting enum value: got %d, want 2", decoded["meeting"])
		}
	})
}

// TestEnumRanges verifies that subtype enum values fall within their expected ranges.
func TestEnumRanges(t *testing.T) {
	// Email subtypes should be in 1-9.
	emailSubtypes := []contentv1.ContentSubtype{
		contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_AUTO_REPLY,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_NEWSLETTER,
	}
	for _, st := range emailSubtypes {
		v := int32(st)
		if v < 1 || v > 9 {
			t.Errorf("email subtype %v has value %d, want 1-9", st, v)
		}
	}

	// Calendar subtypes should be in 10-19.
	calendarSubtypes := []contentv1.ContentSubtype{
		contentv1.ContentSubtype_CONTENT_SUBTYPE_INVITE,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_CANCELLATION,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_UPDATE,
		contentv1.ContentSubtype_CONTENT_SUBTYPE_RESPONSE,
	}
	for _, st := range calendarSubtypes {
		v := int32(st)
		if v < 10 || v > 19 {
			t.Errorf("calendar subtype %v has value %d, want 10-19", st, v)
		}
	}

	// Meeting subtypes should be in 20-29.
	meetingSubtypes := []contentv1.ContentSubtype{
		contentv1.ContentSubtype_CONTENT_SUBTYPE_TRANSCRIPT,
	}
	for _, st := range meetingSubtypes {
		v := int32(st)
		if v < 20 || v > 29 {
			t.Errorf("meeting subtype %v has value %d, want 20-29", st, v)
		}
	}
}

// TestListContentItemsRequestFilter verifies the ContentTypeFilter field exists
// and can be set on ListContentItemsRequest.
func TestListContentItemsRequestFilter(t *testing.T) {
	ct := contentv1.ContentType_CONTENT_TYPE_EMAIL
	req := &contentv1.ListContentItemsRequest{
		TenantId:          "tenant-123",
		ContentTypeFilter: &ct,
	}
	if req.ContentTypeFilter == nil {
		t.Fatal("ContentTypeFilter should not be nil after assignment")
	}
	if *req.ContentTypeFilter != contentv1.ContentType_CONTENT_TYPE_EMAIL {
		t.Errorf("ContentTypeFilter: got %v, want CONTENT_TYPE_EMAIL", *req.ContentTypeFilter)
	}
}
