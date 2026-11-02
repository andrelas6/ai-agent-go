package tools

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSupportListsToolsInOrder(t *testing.T) {
	want := []struct{ name, description string }{
		{"searchKnowledgeBase", "Search the support knowledge base for relevant articles."},
		{"classifyItem", "Classify a work item into a category."},
		{"draftReply", "Write a draft reply for a work item. Does not send anything."},
		{"sendReply", "Send the drafted reply to the customer. This really emails them."},
	}

	got := Support()
	if len(got) != len(want) {
		t.Fatalf("Support() returned %d tools, want %d", len(got), len(want))
	}
	for i, tool := range got {
		if tool.Name != want[i].name {
			t.Errorf("tool %d name = %q, want %q", i, tool.Name, want[i].name)
		}
		if tool.Description != want[i].description {
			t.Errorf("%s description = %q, want %q", tool.Name, tool.Description, want[i].description)
		}
		if tool.Run == nil {
			t.Errorf("%s has no Run", tool.Name)
		}
	}
}

func TestSupportToolSchemas(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "searchKnowledgeBase",
			want: `{
				"type": "object",
				"properties": {
					"query": {"type": "string", "minLength": 1, "description": "what to look up"}
				},
				"required": ["query"],
				"additionalProperties": false
			}`,
		},
		{
			name: "classifyItem",
			want: `{
				"type": "object",
				"properties": {
					"itemId": {"type": "string", "minLength": 1},
					"category": {"type": "string", "minLength": 1, "enum": ["billing", "technical", "sales", "other"]}
				},
				"required": ["itemId", "category"],
				"additionalProperties": false
			}`,
		},
		{
			name: "draftReply",
			want: `{
				"type": "object",
				"properties": {
					"itemId": {"type": "string", "minLength": 1},
					"message": {"type": "string", "minLength": 1}
				},
				"required": ["itemId", "message"],
				"additionalProperties": false
			}`,
		},
		{
			name: "sendReply",
			want: `{
				"type": "object",
				"properties": {
					"itemId": {"type": "string", "minLength": 1},
					"draftId": {"type": "string", "minLength": 1}
				},
				"required": ["itemId", "draftId"],
				"additionalProperties": false
			}`,
		},
	}

	tools := map[string]map[string]any{}
	for _, tool := range Support() {
		tools[tool.Name] = tool.Parameters
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, ok := tools[tt.name]
			if !ok {
				t.Fatalf("Support() has no %s tool", tt.name)
			}
			sent, err := json.Marshal(params)
			if err != nil {
				t.Fatalf("marshal schema: %v", err)
			}

			var got, want any
			if err := json.Unmarshal(sent, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("schema = %s, want %s", sent, tt.want)
			}
		})
	}
}
