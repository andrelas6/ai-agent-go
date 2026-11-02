package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func checkSpec(t *testing.T, tool Tool, name, description, schema string) {
	t.Helper()
	if tool.Name != name {
		t.Errorf("name = %q, want %q", tool.Name, name)
	}
	if tool.Description != description {
		t.Errorf("description = %q, want %q", tool.Description, description)
	}
	if tool.Run == nil {
		t.Errorf("%s has no Run", tool.Name)
	}

	sent, err := json.Marshal(tool.Parameters)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(sent, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(schema), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("schema = %s, want %s", sent, schema)
	}
}

func runTool(t *testing.T, tool Tool, args string) (string, error) {
	t.Helper()
	result, err := tool.Run(context.Background(), json.RawMessage(args))
	if err != nil {
		return "", err
	}
	sent, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return string(sent), nil
}
