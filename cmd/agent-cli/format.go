package main

import (
	"andrelas6/ai-agent-go/internal/events"
	"bytes"
	"encoding/json"
	"strings"
)

func formatEvent(e events.Event) string {
	var fields []string
	switch e.Type {
	case events.WorkflowStarted:
		fields = []string{oneLine(e.Input)}
	case events.WorkflowCompleted:
		fields = []string{oneLine(e.Output)}
	case events.WorkflowFailed:
		fields = []string{oneLine(e.Error)}
	case events.ModelDelta, events.ModelCompleted:
		fields = []string{oneLine(e.Text)}
	case events.ToolRequested:
		fields = []string{oneLine(e.Name), compactJSON(e.Args)}
	case events.ToolCompleted:
		fields = []string{oneLine(e.ToolCallID), compactJSON(e.Result)}
	case events.ToolFailed:
		fields = []string{oneLine(e.ToolCallID), oneLine(e.Error)}
	case events.Log:
		fields = []string{oneLine(e.Level), oneLine(e.Message)}
	}

	line := []string{string(e.Type)}
	for _, f := range fields {
		if f != "" {
			line = append(line, f)
		}
	}
	return strings.Join(line, " ")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return oneLine(string(raw))
	}
	return buf.String()
}
