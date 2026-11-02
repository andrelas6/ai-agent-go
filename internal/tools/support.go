package tools

import (
	"andrelas6/ai-agent-go/internal/agent"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var categories = []string{"billing", "technical", "sales", "other"}

var errNotBuiltYet = errors.New("not built yet")

type classifyArgs struct {
	ItemID   string `json:"itemId"`
	Category string `json:"category"`
}

type classifyResult struct {
	OK       bool   `json:"ok"`
	ItemID   string `json:"itemId"`
	Category string `json:"category"`
}

func Support() []agent.Tool {
	classifySchema := objectSchema(map[string]any{
		"itemId":   plainString(),
		"category": oneOf(categories),
	}, "itemId", "category")

	return []agent.Tool{
		{
			Name:        "searchKnowledgeBase",
			Description: "Search the support knowledge base for relevant articles.",
			Parameters: objectSchema(map[string]any{
				"query": describedString("what to look up"),
			}, "query"),
			Run: notBuiltYet,
		},
		{
			Name:        "classifyItem",
			Description: "Classify a work item into a category.",
			Parameters:  classifySchema,
			Run:         strict(classifySchema, classifyItem),
		},
		{
			Name:        "draftReply",
			Description: "Write a draft reply for a work item. Does not send anything.",
			Parameters: objectSchema(map[string]any{
				"itemId":  plainString(),
				"message": plainString(),
			}, "itemId", "message"),
			Run: notBuiltYet,
		},
		{
			Name:        "sendReply",
			Description: "Send the drafted reply to the customer. This really emails them.",
			Parameters: objectSchema(map[string]any{
				"itemId":  plainString(),
				"draftId": plainString(),
			}, "itemId", "draftId"),
			Run: notBuiltYet,
		},
	}
}

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

func notBuiltYet(ctx context.Context, args json.RawMessage) (any, error) {
	return nil, errNotBuiltYet
}

func classifyItem(args classifyArgs) (any, error) {
	if !slices.Contains(categories, args.Category) {
		return nil, fmt.Errorf("category %q is not one of %s", args.Category, strings.Join(categories, ", "))
	}
	return classifyResult{OK: true, ItemID: args.ItemID, Category: args.Category}, nil
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
