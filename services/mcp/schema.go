package main

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// schemaFromProto generates a JSON Schema compatible ToolInputSchema from a
// protobuf message descriptor. It walks fields recursively and maps proto
// types to JSON Schema types.
func schemaFromProto(md protoreflect.MessageDescriptor) mcp.ToolInputSchema {
	props, required := messageProperties(md)
	return mcp.ToolInputSchema{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

func messageProperties(md protoreflect.MessageDescriptor) (map[string]any, []string) {
	props := map[string]any{}
	var required []string

	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		name := fd.JSONName()
		prop := fieldSchema(fd)
		props[name] = prop

		// Non-optional scalar/enum fields are required.
		// Messages, repeated, and map fields are never required.
		if !fd.HasOptionalKeyword() && !fd.IsList() && !fd.IsMap() &&
			fd.Kind() != protoreflect.MessageKind && fd.Kind() != protoreflect.GroupKind {
			required = append(required, name)
		}
	}

	return props, required
}

func fieldSchema(fd protoreflect.FieldDescriptor) map[string]any {
	if fd.IsMap() {
		return mapFieldSchema(fd)
	}
	if fd.IsList() {
		item := scalarSchema(fd)
		schema := map[string]any{
			"type":  "array",
			"items": item,
		}
		addDescription(schema, fd)
		return schema
	}
	schema := scalarSchema(fd)
	addDescription(schema, fd)
	return schema
}

func mapFieldSchema(fd protoreflect.FieldDescriptor) map[string]any {
	valueSchema := scalarSchema(fd.MapValue())
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": valueSchema,
	}
	addDescription(schema, fd)
	return schema
}

func scalarSchema(fd protoreflect.FieldDescriptor) map[string]any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer"}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"type": "integer"}

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": "number"}

	case protoreflect.StringKind:
		return map[string]any{"type": "string"}

	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "format": "byte"}

	case protoreflect.EnumKind:
		ed := fd.Enum()
		vals := make([]string, ed.Values().Len())
		for i := 0; i < ed.Values().Len(); i++ {
			vals[i] = string(ed.Values().Get(i).Name())
		}
		return map[string]any{"type": "string", "enum": vals}

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageSchema(fd.Message())

	default:
		return map[string]any{"type": "string"}
	}
}

func messageSchema(md protoreflect.MessageDescriptor) map[string]any {
	switch md.FullName() {
	case "google.protobuf.Timestamp":
		return map[string]any{"type": "string", "format": "date-time"}
	case "google.protobuf.Duration":
		return map[string]any{"type": "string"}
	}

	schema := map[string]any{"type": "object"}
	props, req := messageProperties(md)
	if len(props) > 0 {
		schema["properties"] = props
	}
	if len(req) > 0 {
		schema["required"] = req
	}
	return schema
}

func addDescription(schema map[string]any, fd protoreflect.FieldDescriptor) {
	loc := fd.ParentFile().SourceLocations().ByDescriptor(fd)
	if comment := strings.TrimSpace(loc.LeadingComments); comment != "" {
		schema["description"] = comment
	}
}
