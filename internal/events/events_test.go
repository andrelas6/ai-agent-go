package events

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTypesMatchTheCourse(t *testing.T) {
	cases := map[Type]string{
		WorkflowStarted:   "workflow.started",
		WorkflowCompleted: "workflow.completed",
		WorkflowFailed:    "workflow.failed",
		ModelCompleted:    "model.completed",
		ToolRequested:     "tool.requested",
		ToolCompleted:     "tool.completed",
		ToolFailed:        "tool.failed",
		Log:               "log",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("type = %q, want %q", got, want)
		}
	}
}

func TestEventMarshalsToTheCourseShape(t *testing.T) {
	event := Event{
		ID:         "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f",
		TS:         1760870400123,
		Type:       ToolRequested,
		WorkflowID: "9a2e7c41-5b6d-4e8f-a0b1-c2d3e4f5a6b7",
		ToolCallID: "call-1",
		Name:       "classifyItem",
		Args:       json.RawMessage(`{"itemId":"item-1"}`),
	}

	got := marshalToMap(t, event)

	want := map[string]any{
		"id":         "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f",
		"ts":         float64(1760870400123),
		"type":       "tool.requested",
		"workflowId": "9a2e7c41-5b6d-4e8f-a0b1-c2d3e4f5a6b7",
		"toolCallId": "call-1",
		"name":       "classifyItem",
		"args":       map[string]any{"itemId": "item-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("json = %v, want %v", got, want)
	}
}

func TestResultPassesThroughAsRawJSON(t *testing.T) {
	event := Event{
		ID:         "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f",
		TS:         1,
		Type:       ToolCompleted,
		WorkflowID: "9a2e7c41-5b6d-4e8f-a0b1-c2d3e4f5a6b7",
		ToolCallID: "call-1",
		Result:     json.RawMessage(`{"ok":true,"category":"billing"}`),
	}

	got := marshalToMap(t, event)

	want := map[string]any{"ok": true, "category": "billing"}
	if !reflect.DeepEqual(got["result"], want) {
		t.Errorf("result = %v, want %v", got["result"], want)
	}
}

func TestLogWithoutWorkflowIDLeavesItOut(t *testing.T) {
	event := Event{
		ID:      "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f",
		TS:      1,
		Type:    Log,
		Level:   "warn",
		Message: "No agent yet.",
	}

	got := marshalToMap(t, event)

	want := map[string]any{
		"id":      "6f1c2b9e-1d2a-4c3b-9e8f-0a1b2c3d4e5f",
		"ts":      float64(1),
		"type":    "log",
		"level":   "warn",
		"message": "No agent yet.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("json = %v, want %v", got, want)
	}
}

func TestWorkflowEventsCarryTheirText(t *testing.T) {
	cases := []struct {
		event Event
		key   string
		want  string
	}{
		{Event{Type: WorkflowStarted, Input: "hello"}, "input", "hello"},
		{Event{Type: WorkflowCompleted, Output: "done"}, "output", "done"},
		{Event{Type: WorkflowFailed, Error: "step limit"}, "error", "step limit"},
		{Event{Type: ModelCompleted, Text: "all set"}, "text", "all set"},
	}
	for _, c := range cases {
		got := marshalToMap(t, c.event)
		if got[c.key] != c.want {
			t.Errorf("%s: %s = %v, want %q", c.event.Type, c.key, got[c.key], c.want)
		}
	}
}

func marshalToMap(t *testing.T, event Event) map[string]any {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal %s: %v", data, err)
	}
	return got
}
