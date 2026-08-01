package mcp

import (
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// lenientSchema derives the JSON Schema for the args type T but relaxes two
// defaults of the SDK's reflection-based schemas:
//
//   - additionalProperties is allowed: the SDK closes structs
//     (additionalProperties: false), so intuitive LLM field names such as
//     "file", "file_pattern", or "regex" are rejected with hard validation
//     errors. A lenient schema accepts unknown fields — handlers pick up
//     known aliases via the args structs (see the alias json tags).
//   - no field is marked required: handlers validate the semantically
//     required fields themselves and produce friendlier errors (e.g. "project
//     is required — use open_project to bind one").
//
// This is the "宽进严出" (lenient in, strict out) contract: schema accepts,
// handler validates with actionable messages.
func lenientSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.ForType(reflect.TypeFor[T](), &jsonschema.ForOptions{})
	if err != nil {
		// Fall back to a fully open object schema; the handler still
		// validates required fields.
		return &jsonschema.Schema{Type: "object"}
	}
	schema.Required = nil
	schema.AdditionalProperties = &jsonschema.Schema{}
	return schema
}
