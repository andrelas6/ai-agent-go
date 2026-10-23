package main

import (
	"andrelas6/ai-agent-go/internal/events"
	"reflect"
	"testing"
)

func TestStubRunnerEmitsTheCourseEvents(t *testing.T) {
	var got []events.Event
	emit := func(e events.Event) {
		got = append(got, e)
	}

	newStubRunner()(t.Context(), "wf-1", "hello", emit)

	want := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: "wf-1", Input: "hello"},
		{Type: events.Log, WorkflowID: "wf-1", Level: "warn", Message: "No agent yet. You build the brittle agent loop in Lesson 1 (internal/agent/agent.go)."},
		{Type: events.WorkflowCompleted, WorkflowID: "wf-1", Output: "(no agent implemented yet)"},
	}
	if len(got) != len(want) {
		t.Fatalf("emitted %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		got[i].ID = ""
		got[i].TS = 0
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
