package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func plainString() map[string]any {
	return map[string]any{"type": "string", "minLength": 1}
}

func describedString(description string) map[string]any {
	field := plainString()
	field["description"] = description
	return field
}

func oneOf(values []string) map[string]any {
	field := plainString()
	field["enum"] = values
	return field
}

func strict[T any](schema map[string]any, run func(T) (any, error)) func(context.Context, json.RawMessage) (any, error) {
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]string)

	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			if _, ok := properties[key]; !ok {
				return nil, fmt.Errorf("unknown field %q", key)
			}
		}

		var args T
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, err
		}

		for _, name := range required {
			var value string
			if field, ok := fields[name]; ok {
				if err := json.Unmarshal(field, &value); err != nil {
					return nil, err
				}
			}
			if value == "" {
				return nil, fmt.Errorf("%q is required", name)
			}
		}

		return run(args)
	}
}
