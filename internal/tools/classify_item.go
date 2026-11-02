package tools

import (
	"fmt"
	"slices"
	"strings"
)

var categories = []string{"billing", "technical", "sales", "other"}

type classifyArgs struct {
	ItemID   string `json:"itemId"`
	Category string `json:"category"`
}

type classifyResult struct {
	OK       bool   `json:"ok"`
	ItemID   string `json:"itemId"`
	Category string `json:"category"`
}

func ClassifyItem() Tool {
	schema := objectSchema(map[string]any{
		"itemId":   nonEmptyString(),
		"category": oneOf(categories),
	}, "itemId", "category")

	return Tool{
		Name:        "classifyItem",
		Description: "Classify a work item into a category.",
		Parameters:  schema,
		Run:         decodeStringArgs(schema, runClassifyItem),
	}
}

func runClassifyItem(args classifyArgs) (any, error) {
	if !slices.Contains(categories, args.Category) {
		return nil, fmt.Errorf("category %q is not one of %s", args.Category, strings.Join(categories, ", "))
	}
	return classifyResult{OK: true, ItemID: args.ItemID, Category: args.Category}, nil
}
