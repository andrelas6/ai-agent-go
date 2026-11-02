package tools

import (
	"strings"
	"testing"
)

func TestClassifyItemSpec(t *testing.T) {
	checkSpec(t, ClassifyItem(), "classifyItem", "Classify a work item into a category.", `{
		"type": "object",
		"properties": {
			"itemId": {"type": "string", "minLength": 1},
			"category": {"type": "string", "minLength": 1, "enum": ["billing", "technical", "sales", "other"]}
		},
		"required": ["itemId", "category"],
		"additionalProperties": false
	}`)
}

func TestClassifyItem(t *testing.T) {
	got, err := runTool(t, ClassifyItem(), `{"itemId":"item-2","category":"technical"}`)
	if err != nil {
		t.Fatalf("classifyItem: %v", err)
	}
	if want := `{"ok":true,"itemId":"item-2","category":"technical"}`; got != want {
		t.Errorf("classifyItem = %s, want %s", got, want)
	}
}

func TestClassifyItemRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{"missing itemId", `{"category":"billing"}`, `"itemId" is missing`},
		{"missing category", `{"itemId":"item-1"}`, `"category" is missing`},
		{"null itemId", `{"itemId":null,"category":"billing"}`, `"itemId" is null`},
		{"null itemId with spaces around it", `{"itemId" :  null , "category":"billing"}`, `"itemId" is null`},
		{"empty itemId", `{"itemId":"","category":"billing"}`, `"itemId" is empty`},
		{"itemId is a number", `{"itemId":5,"category":"billing"}`, `"itemId" must be a string`},
		{"unknown field", `{"itemId":"item-1","category":"billing","priority":"high"}`, `unknown field "priority"`},
		{"key in the wrong case", `{"ItemID":"item-1","category":"billing"}`, `unknown field "ItemID"`},
		{"not an object", `["item-1","billing"]`, "arguments must be a JSON object"},
		{"category not in the list", `{"itemId":"item-1","category":"refunds"}`, `category "refunds" is not one of billing, technical, sales, other`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTool(t, ClassifyItem(), tt.args)
			if err == nil {
				t.Fatalf("classifyItem(%s) = %s, want an error", tt.args, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
