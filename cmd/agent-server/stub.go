package main

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"
)

func runStub(ctx context.Context, workflowID string, input string, emit func(events.Event)) {

	eventStarted := events.Event{
		WorkflowID: workflowID,
		Type:       events.WorkflowStarted,
		Input:      input,
	}
	eventLog := events.Event{
		WorkflowID: workflowID,
		Type:       events.Log,
		Level:      "warn",
		Message:    "No agent yet. You build the brittle agent loop in Lesson 1 (internal/agent/agent.go).",
	}
	eventCompleted := events.Event{
		Output:     "(no agent implemented yet)",
		WorkflowID: workflowID,
		Type:       events.WorkflowCompleted,
	}

	emit(eventStarted)
	emit(eventLog)
	emit(eventCompleted)
}
