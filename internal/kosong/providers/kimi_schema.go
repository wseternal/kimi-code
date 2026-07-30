// Package providers implements LLM provider adapters.
// kimi_schema.go provides JSON Schema normalization for Kimi/Moonshot APIs.
// It handles $ref dereferencing, type completion, and Moonshot-specific quirks.
package providers

import (
	"encoding/json"
	"strings"
)

// NormalizeKimiToolSchema normalizes a tool parameter schema for Kimi APIs.
// It resolves $ref references, completes missing types, and applies
// Moonshot-specific transformations.
func NormalizeKimiToolSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	// Deep copy to avoid mutating the original
	result := deepCopyMap(schema)

	// Resolve $defs / definitions references
	defs := extractDefs(result)
	if defs != nil {
		if resolved, ok := resolveRefs(result, defs, 0).(map[string]interface{}); ok {
			result = resolved
		}
	}

	// Complete types and apply Moonshot normalizations
	if completed, ok := completeTypes(result).(map[string]interface{}); ok {
		result = completed
	}
	if normalized, ok := normalizeForMoonshot(result).(map[string]interface{}); ok {
		result = normalized
	}

	return result
}

// NormalizeKimiTools normalizes schemas for a slice of tools.
func NormalizeKimiTools(tools []map[string]interface{}) []map[string]interface{} {
	if len(tools) == 0 {
		return tools
	}
	result := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		result[i] = NormalizeKimiToolSchema(t)
	}
	return result
}

// extractDefs extracts $defs or definitions from a schema root.
func extractDefs(schema map[string]interface{}) map[string]interface{} {
	if d, ok := schema["$defs"]; ok {
		if m, ok := d.(map[string]interface{}); ok {
			return m
		}
	}
	if d, ok := schema["definitions"]; ok {
		if m, ok := d.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// resolveRefs recursively resolves $ref references in a schema.
// maxDepth prevents infinite recursion on circular references.
func resolveRefs(node interface{}, defs map[string]interface{}, depth int) interface{} {
	if depth > 20 {
		return node
	}

	switch v := node.(type) {
	case map[string]interface{}:
		// Check if this node is a $ref
		if ref, ok := v["$ref"]; ok {
			if refStr, ok := ref.(string); ok {
				resolved := resolveRef(refStr, defs)
				if resolved != nil {
					// Merge any sibling properties with the resolved ref
					merged := deepCopyMap(resolved)
					for k, val := range v {
						if k == "$ref" {
							continue
						}
						merged[k] = resolveRefs(val, defs, depth+1)
					}
					return merged
				}
			}
		}

		// Recurse into all properties
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = resolveRefs(val, defs, depth+1)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = resolveRefs(item, defs, depth+1)
		}
		return result

	default:
		return node
	}
}

// resolveRef resolves a JSON Pointer $ref string like "#/$defs/MyType".
func resolveRef(ref string, defs map[string]interface{}) map[string]interface{} {
	// Handle "#/$defs/Name" or "#/definitions/Name"
	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return nil
	}

	// The definition name is the last segment
	name := parts[len(parts)-1]
	if def, ok := defs[name]; ok {
		if m, ok := def.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// completeTypes fills in missing "type" fields based on structural clues.
// For example, if a schema has "properties" but no "type", it's an object.
func completeTypes(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = completeTypes(val)
		}

		// Infer type from structure if missing
		if _, hasType := result["type"]; !hasType {
			if _, hasProps := result["properties"]; hasProps {
				result["type"] = "object"
			} else if _, hasItems := result["items"]; hasItems {
				result["type"] = "array"
			} else if enum, hasEnum := result["enum"]; hasEnum {
				// Infer type from enum values
				if arr, ok := enum.([]interface{}); ok && len(arr) > 0 {
					result["type"] = inferTypeFromValue(arr[0])
				}
			}
		}

		// Recurse into properties
		if props, ok := result["properties"].(map[string]interface{}); ok {
			for pk, pv := range props {
				props[pk] = completeTypes(pv)
			}
		}

		// Recurse into items
		if items, ok := result["items"]; ok {
			result["items"] = completeTypes(items)
		}

		// Recurse into additionalProperties if it's a schema
		if ap, ok := result["additionalProperties"].(map[string]interface{}); ok {
			result["additionalProperties"] = completeTypes(ap)
		}

		// Recurse into anyOf, oneOf, allOf
		for _, key := range []string{"anyOf", "oneOf", "allOf"} {
			if arr, ok := result[key].([]interface{}); ok {
				for i, item := range arr {
					arr[i] = completeTypes(item)
				}
			}
		}

		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = completeTypes(item)
		}
		return result

	default:
		return node
	}
}

// inferTypeFromValue returns the JSON Schema type name for a Go value.
func inferTypeFromValue(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case float64, float32, json.Number:
		return "number"
	case bool:
		return "boolean"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	default:
		return "string"
	}
}

// normalizeForMoonshot applies Moonshot/Kimi-specific schema transformations:
// - Strips $schema and $id fields that confuse the API
// - Removes unsupported keywords (e.g., "additionalProperties" set to false
//   when there are no properties, which some Moonshot models reject)
// - Normalizes "nullable" to anyOf with null type
func normalizeForMoonshot(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			// Strip meta-schema fields that confuse Moonshot
			if k == "$schema" || k == "$id" {
				continue
			}
			result[k] = normalizeForMoonshot(val)
		}

		// Normalize "nullable: true" to anyOf with null
		if nullable, ok := result["nullable"]; ok {
			if b, ok := nullable.(bool); ok && b {
				delete(result, "nullable")
				existingType, _ := result["type"].(string)
				if existingType != "" {
					result["anyOf"] = []interface{}{
						map[string]interface{}{"type": existingType},
						map[string]interface{}{"type": "null"},
					}
					delete(result, "type")
				}
			}
		}

		// Remove empty required arrays
		if req, ok := result["required"]; ok {
			if arr, ok := req.([]interface{}); ok && len(arr) == 0 {
				delete(result, "required")
			}
		}

		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = normalizeForMoonshot(item)
		}
		return result

	default:
		return node
	}
}

// deepCopyMap creates a deep copy of a map[string]interface{}.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	// Use JSON round-trip for simplicity and correctness
	data, err := json.Marshal(m)
	if err != nil {
		// Fallback: shallow copy
		result := make(map[string]interface{}, len(m))
		for k, v := range m {
			result[k] = v
		}
		return result
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return m
	}
	return result
}

// FlattenSchemaRefs resolves all $ref references in a JSON schema and returns
// a flat schema suitable for providers that don't support $ref.
// This is a convenience wrapper around NormalizeKimiToolSchema that focuses
// specifically on ref resolution without Moonshot-specific transforms.
func FlattenSchemaRefs(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}
	result := deepCopyMap(schema)
	defs := extractDefs(result)
	if defs != nil {
		if resolved, ok := resolveRefs(result, defs, 0).(map[string]interface{}); ok {
			result = resolved
		}
		// Remove the defs/definitions from the output
		delete(result, "$defs")
		delete(result, "definitions")
	}
	return result
}
