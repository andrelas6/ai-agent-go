package tools

import (
	"andrelas6/ai-agent-go/internal/agent"
	"context"
	"encoding/json"
	"reflect"
	"strings"
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

func findTool(t *testing.T, name string) agent.Tool {
	t.Helper()
	for _, tool := range Support() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("Support() has no %s tool", name)
	return agent.Tool{}
}

func runTool(t *testing.T, name, args string) (string, error) {
	t.Helper()
	result, err := findTool(t, name).Run(context.Background(), json.RawMessage(args))
	if err != nil {
		return "", err
	}
	sent, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return string(sent), nil
}

func TestClassifyItem(t *testing.T) {
	got, err := runTool(t, "classifyItem", `{"itemId":"item-2","category":"technical"}`)
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
		{"missing itemId", `{"category":"billing"}`, `"itemId" is required`},
		{"missing category", `{"itemId":"item-1"}`, `"category" is required`},
		{"null itemId", `{"itemId":null,"category":"billing"}`, `"itemId" is required`},
		{"empty itemId", `{"itemId":"","category":"billing"}`, `"itemId" is required`},
		{"itemId is a number", `{"itemId":5,"category":"billing"}`, "cannot unmarshal number"},
		{"unknown field", `{"itemId":"item-1","category":"billing","priority":"high"}`, `unknown field "priority"`},
		{"key in the wrong case", `{"ItemID":"item-1","category":"billing"}`, `unknown field "ItemID"`},
		{"not an object", `["item-1","billing"]`, "cannot unmarshal array"},
		{"category not in the list", `{"itemId":"item-1","category":"refunds"}`, `category "refunds" is not one of billing, technical, sales, other`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTool(t, "classifyItem", tt.args)
			if err == nil {
				t.Fatalf("classifyItem(%s) = %s, want an error", tt.args, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
