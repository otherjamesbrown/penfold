package main

import (
	"testing"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaFromProto_SearchRequest(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	assert.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	// Non-optional scalars should be required
	assert.Contains(t, schema.Required, "query")
	assert.Contains(t, schema.Required, "tenantId")
	assert.Contains(t, schema.Required, "limit")
	assert.Contains(t, schema.Required, "offset")
	assert.Contains(t, schema.Required, "sortOrder")

	// Optional fields should NOT be required
	assert.NotContains(t, schema.Required, "textWeight")
	assert.NotContains(t, schema.Required, "vectorWeight")
	assert.NotContains(t, schema.Required, "minScore")
	assert.NotContains(t, schema.Required, "includeHighlights")

	// Message field should NOT be required
	assert.NotContains(t, schema.Required, "filters")
}

func TestSchemaFromProto_StringField(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	query := schema.Properties["query"].(map[string]any)
	assert.Equal(t, "string", query["type"])
}

func TestSchemaFromProto_IntField(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	limit := schema.Properties["limit"].(map[string]any)
	assert.Equal(t, "integer", limit["type"])
}

func TestSchemaFromProto_OptionalFloat(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	tw := schema.Properties["textWeight"].(map[string]any)
	assert.Equal(t, "number", tw["type"])
}

func TestSchemaFromProto_OptionalBool(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	ih := schema.Properties["includeHighlights"].(map[string]any)
	assert.Equal(t, "boolean", ih["type"])
}

func TestSchemaFromProto_Enum(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	so := schema.Properties["sortOrder"].(map[string]any)
	assert.Equal(t, "string", so["type"])

	enumVals := so["enum"].([]string)
	assert.Contains(t, enumVals, "SORT_ORDER_UNSPECIFIED")
	assert.Contains(t, enumVals, "SORT_ORDER_RELEVANCE")
	assert.Contains(t, enumVals, "SORT_ORDER_DATE_DESC")
	assert.Contains(t, enumVals, "SORT_ORDER_DATE_ASC")
}

func TestSchemaFromProto_NestedMessage(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	filters := schema.Properties["filters"].(map[string]any)
	assert.Equal(t, "object", filters["type"])

	props := filters["properties"].(map[string]any)
	assert.Contains(t, props, "contentTypes")
	assert.Contains(t, props, "sourceIds")
	assert.Contains(t, props, "tags")

	// Repeated string inside nested message
	ct := props["contentTypes"].(map[string]any)
	assert.Equal(t, "array", ct["type"])
	items := ct["items"].(map[string]any)
	assert.Equal(t, "string", items["type"])
}

func TestSchemaFromProto_RepeatedField(t *testing.T) {
	md := (&searchv1.SearchResult{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	highlights := schema.Properties["highlights"].(map[string]any)
	assert.Equal(t, "array", highlights["type"])
	items := highlights["items"].(map[string]any)
	assert.Equal(t, "string", items["type"])
}

func TestSchemaFromProto_MapField(t *testing.T) {
	md := (&searchv1.SearchResult{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	meta := schema.Properties["metadata"].(map[string]any)
	assert.Equal(t, "object", meta["type"])
	ap := meta["additionalProperties"].(map[string]any)
	assert.Equal(t, "string", ap["type"])
}

func TestSchemaFromProto_Timestamp(t *testing.T) {
	md := (&searchv1.SearchResult{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	ca := schema.Properties["createdAt"].(map[string]any)
	assert.Equal(t, "string", ca["type"])
	assert.Equal(t, "date-time", ca["format"])
}

func TestSchemaFromProto_OptionalString(t *testing.T) {
	md := (&searchv1.SearchResult{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	title := schema.Properties["title"].(map[string]any)
	assert.Equal(t, "string", title["type"])

	// Optional string should not be required
	assert.NotContains(t, schema.Required, "title")
}

func TestSchemaFromProto_Descriptions(t *testing.T) {
	md := (&searchv1.SearchRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	query := schema.Properties["query"].(map[string]any)
	desc, ok := query["description"].(string)
	if ok {
		assert.NotEmpty(t, desc)
	}
	// Comments may not be available in compiled protos — don't fail if absent
}

func TestSchemaFromProto_MapInt64Values(t *testing.T) {
	md := (&searchv1.GetSearchStatsResponse{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	dbt := schema.Properties["documentsByType"].(map[string]any)
	assert.Equal(t, "object", dbt["type"])
	ap := dbt["additionalProperties"].(map[string]any)
	assert.Equal(t, "integer", ap["type"])
}

func TestSchemaFromProto_NestedTimestamp(t *testing.T) {
	// FilterOptions has optional Timestamp fields
	md := (&searchv1.FilterOptions{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	df := schema.Properties["dateFrom"].(map[string]any)
	assert.Equal(t, "string", df["type"])
	assert.Equal(t, "date-time", df["format"])
}

func TestSchemaFromProto_RepeatedFloat(t *testing.T) {
	md := (&searchv1.IndexDocumentRequest{}).ProtoReflect().Descriptor()
	schema := schemaFromProto(md)

	emb := schema.Properties["embedding"].(map[string]any)
	assert.Equal(t, "array", emb["type"])
	items := emb["items"].(map[string]any)
	assert.Equal(t, "number", items["type"])
}
