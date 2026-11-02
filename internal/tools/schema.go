package tools

import (
	"context"
	"encoding/json"
	"errors"
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

func decodeStringArgs[T any](schema map[string]any, run func(T) (any, error)) func(context.Context, json.RawMessage) (any, error) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		panic("tool schema has no properties map")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		panic("tool schema has no required list")
	}

	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, errors.New("arguments must be a JSON object")
		}

		values := make(map[string]string, len(fields))
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			if _, ok := properties[key]; !ok {
				return nil, fmt.Errorf("unknown field %q", key)
			}
			if string(fields[key]) == "null" {
				continue
			}
			var value string
			if err := json.Unmarshal(fields[key], &value); err != nil {
				return nil, fmt.Errorf("%q must be a string", key)
			}
			values[key] = value
		}

		for _, name := range required {
			if _, sent := fields[name]; !sent {
				return nil, fmt.Errorf("%q is missing", name)
			}
			if string(fields[name]) == "null" {
				return nil, fmt.Errorf("%q is null", name)
			}
			if values[name] == "" {
				return nil, fmt.Errorf("%q is empty", name)
			}
		}

		var args T
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, err
		}
		return run(args)
	}
}
