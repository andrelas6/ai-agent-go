package main

import (
	"andrelas6/ai-agent-go/internal/events"
	"reflect"
	"strings"
	"testing"
)

func TestNewRunnerWithStubRunsTheStub(t *testing.T) {
	run, line, err := newRunner(true, "", "")
	if err != nil {
		t.Fatalf("newRunner returned an error: %v", err)
	}
	if !strings.Contains(line, "running the stub") {
		t.Errorf("log line = %q, want it to say \"running the stub\"", line)
	}

	var got []events.Event
	run(t.Context(), "wf-1", "hello", func(e events.Event) {
		got = append(got, e)
	})

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

func TestNewRunnerWithoutKeyAndWithoutStubFails(t *testing.T) {
	run, _, err := newRunner(false, "", "z-ai/glm-5.3-flash")
	if err == nil {
		t.Fatal("newRunner returned no error")
	}
	if run != nil {
		t.Error("newRunner returned a runner along with the error")
	}
	for _, want := range []string{"OPENROUTER_API_KEY", "-stub"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %s", err, want)
		}
	}
}

func TestNewRunnerWithKeyReturnsTheAgent(t *testing.T) {
	run, line, err := newRunner(false, "key", "z-ai/glm-5.3-flash")
	if err != nil {
		t.Fatalf("newRunner returned an error: %v", err)
	}
	if run == nil {
		t.Fatal("newRunner returned no runner")
	}
	if !strings.Contains(line, "z-ai/glm-5.3-flash") {
		t.Errorf("log line = %q, want it to name the model", line)
	}
}
