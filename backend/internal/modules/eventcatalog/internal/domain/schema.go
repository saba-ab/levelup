package domain

import (
	"fmt"
	"slices"
)

const maxSchemaProperties = 100

var schemaPropertyTypes = []string{"string", "number", "integer", "boolean", "object", "array", "null"}

// ValidatePropertySchema checks the JSON Schema subset an event type may
// carry. nil is valid (free-form). Unknown keywords are tolerated so clients
// can annotate (title, examples, ...); the keywords below are enforced:
//
//	type:       "object" when present (activity properties are an object)
//	properties: object of property name → object; each "type" is one of the
//	            JSON Schema primitive type names when present
//	required:   array of strings, each naming a declared property
func ValidatePropertySchema(schema map[string]any) error {
	if schema == nil {
		return nil
	}
	if t, ok := schema["type"]; ok && t != "object" {
		return invalidSchema(`"type" must be "object"`)
	}

	var props map[string]any
	if raw, ok := schema["properties"]; ok {
		props, ok = raw.(map[string]any)
		if !ok {
			return invalidSchema(`"properties" must be an object`)
		}
		if len(props) > maxSchemaProperties {
			return invalidSchema(fmt.Sprintf("at most %d properties", maxSchemaProperties))
		}
		for name, def := range props {
			d, ok := def.(map[string]any)
			if !ok {
				return invalidSchema(fmt.Sprintf("property %q must be an object", name))
			}
			if t, ok := d["type"]; ok {
				ts, isString := t.(string)
				if !isString || !slices.Contains(schemaPropertyTypes, ts) {
					return invalidSchema(fmt.Sprintf("property %q has an unsupported type", name))
				}
			}
		}
	}

	if raw, ok := schema["required"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return invalidSchema(`"required" must be an array of property names`)
		}
		for _, item := range list {
			name, ok := item.(string)
			if !ok {
				return invalidSchema(`"required" must be an array of property names`)
			}
			if _, declared := props[name]; !declared {
				return invalidSchema(fmt.Sprintf("required property %q is not declared in properties", name))
			}
		}
	}
	return nil
}
