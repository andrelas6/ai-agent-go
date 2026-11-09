package main

import (
	"andrelas6/ai-agent-go/internal/events"
	"encoding/json"
	"strings"
	"testing"
)

// Each event type prints as one line: its type first, then its most useful fields.
func TestFormatEventPrintsEachTypeOnOneLine(t *testing.T) {
	cases := []struct {
		name  string
		event events.Event
		want  string
	}{
		{"workflow started", events.Event{Type: events.WorkflowStarted, Input: "Handle item-1"}, "workflow.started Handle item-1"},
		{"workflow completed", events.Event{Type: events.WorkflowCompleted, Output: "All done."}, "workflow.completed All done."},
		{"workflow failed", events.Event{Type: events.WorkflowFailed, Error: "context canceled"}, "workflow.failed context canceled"},
		{"model delta", events.Event{Type: events.ModelDelta, Text: "Let me classify it."}, "model.delta Let me classify it."},
		{"model completed", events.Event{Type: events.ModelCompleted, Text: "All done."}, "model.completed All done."},
		{
			"tool requested",
			events.Event{Type: events.ToolRequested, ToolCallID: "call-1", Name: "classifyItem", Args: json.RawMessage(`{"itemId":"item-1","category":"billing"}`)},
			`tool.requested classifyItem {"itemId":"item-1","category":"billing"}`,
		},
		{
			"tool completed",
			events.Event{Type: events.ToolCompleted, ToolCallID: "call-1", Result: json.RawMessage(`{"ok":true,"itemId":"item-1","category":"billing"}`)},
			`tool.completed call-1 {"ok":true,"itemId":"item-1","category":"billing"}`,
		},
		{"tool failed", events.Event{Type: events.ToolFailed, ToolCallID: "call-1", Error: "no arguments sent"}, "tool.failed call-1 no arguments sent"},
		{"log", events.Event{Type: events.Log, Level: "warn", Message: "No agent yet."}, "log warn No agent yet."},
		{"unknown type", events.Event{Type: "something.new", Text: "ignored"}, "something.new"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatEvent(c.event); got != c.want {
				t.Errorf("formatEvent = %q, want %q", got, c.want)
			}
		})
	}
}

// A multi-line task, output or text is folded onto one line.
func TestFormatEventFoldsMultiLineText(t *testing.T) {
	cases := []struct {
		event events.Event
		want  string
	}{
		{events.Event{Type: events.WorkflowStarted, Input: "Handle these work items:\n- item-1\n- item-2\n"}, "workflow.started Handle these work items: - item-1 - item-2"},
		{events.Event{Type: events.WorkflowCompleted, Output: "Done.\n\n1. item-1\n2. item-2"}, "workflow.completed Done. 1. item-1 2. item-2"},
		{events.Event{Type: events.ModelCompleted, Text: "  line one\n\tline two  "}, "model.completed line one line two"},
	}
	for _, c := range cases {
		if got := formatEvent(c.event); got != c.want {
			t.Errorf("formatEvent(%s) = %q, want %q", c.event.Type, got, c.want)
		}
	}
}

// Pretty-printed args and results are compacted, and args that aren't valid JSON are printed as they are, folded onto one line.
func TestFormatEventCompactsJSON(t *testing.T) {
	cases := []struct {
		event events.Event
		want  string
	}{
		{
			events.Event{Type: events.ToolRequested, Name: "draftReply", Args: json.RawMessage("{\n  \"itemId\": \"item-1\",\n  \"tone\": \"friendly\"\n}")},
			`tool.requested draftReply {"itemId":"item-1","tone":"friendly"}`,
		},
		{
			events.Event{Type: events.ToolCompleted, ToolCallID: "call-2", Result: json.RawMessage("{\n  \"sent\": true\n}")},
			`tool.completed call-2 {"sent":true}`,
		},
		{
			events.Event{Type: events.ToolRequested, Name: "classifyItem", Args: json.RawMessage("{\"itemId\":\n \"item-1\"")},
			`tool.requested classifyItem {"itemId": "item-1"`,
		},
	}
	for _, c := range cases {
		if got := formatEvent(c.event); got != c.want {
			t.Errorf("formatEvent(%s) = %q, want %q", c.event.Type, got, c.want)
		}
	}
}

// No event, whatever its fields hold, prints a line with a newline in it.
func TestFormatEventNeverContainsANewline(t *testing.T) {
	multi := "a\nb\r\nc"
	for _, typ := range []events.Type{
		events.WorkflowStarted, events.WorkflowCompleted, events.WorkflowFailed,
		events.ModelDelta, events.ModelCompleted,
		events.ToolRequested, events.ToolCompleted, events.ToolFailed, events.Log,
	} {
		e := events.Event{
			Type: typ, Input: multi, Output: multi, Error: multi, Text: multi,
			ToolCallID: multi, Name: multi, Level: multi, Message: multi,
			Args: json.RawMessage(multi), Result: json.RawMessage(multi),
		}
		if got := formatEvent(e); strings.ContainsAny(got, "\r\n") {
			t.Errorf("formatEvent(%s) = %q, want no newline", typ, got)
		}
	}
}
