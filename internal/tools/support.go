package tools

import (
	"andrelas6/ai-agent-go/internal/agent"
	"context"
	"encoding/json"
	"errors"
)

var categories = []string{"billing", "technical", "sales", "other"}

var errNotBuiltYet = errors.New("not built yet")

func Support() []agent.Tool {
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
			Parameters: objectSchema(map[string]any{
				"itemId":   plainString(),
				"category": oneOf(categories),
			}, "itemId", "category"),
			Run: notBuiltYet,
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
