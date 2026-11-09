package main

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/events"
	"andrelas6/ai-agent-go/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type scriptedModel struct {
	replies []agents.Message
	errs    []error
	calls   [][]agents.Message
}

func (m *scriptedModel) Next(ctx context.Context, messages []agents.Message, _ []tools.Tool) (agents.Message, error) {
	m.calls = append(m.calls, messages)
	i := len(m.calls) - 1
	if i < len(m.errs) && m.errs[i] != nil {
		return agents.Message{}, m.errs[i]
	}
	if i >= len(m.replies) {
		return agents.Message{}, errors.New("scriptedModel: no reply left")
	}
	return m.replies[i], nil
}

func outputLines(out string) []string {
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

// The model asks for classifyItem, then answers in text: one line per event, each starting with its type, and exit code 0.
func TestRunPrintsOneLinePerEventAndExitsZeroOnCompleted(t *testing.T) {
	m := &scriptedModel{replies: []agents.Message{
		{Role: "assistant", ToolCalls: []agents.ToolCall{{
			ID:   "call-1",
			Name: "classifyItem",
			Args: json.RawMessage(`{"itemId":"item-1","category":"billing"}`),
		}}, FinishReason: "tool_calls"},
		{Role: "assistant", Content: "done", FinishReason: "stop"},
	}}
	var out strings.Builder

	code := run(t.Context(), "Handle item-1", agents.NewSupportAgent(m).Run, &out)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	wantTypes := []string{"workflow.started", "tool.requested", "tool.completed", "model.completed", "workflow.completed"}
	lines := outputLines(out.String())
	if len(lines) != len(wantTypes) {
		t.Fatalf("printed %d lines, want %d:\n%s", len(lines), len(wantTypes), out.String())
	}
	for i, want := range wantTypes {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want it to start with %q", i, lines[i], want)
		}
	}
}

// A runner that emits only workflow.started and then returns never completed, so the exit code is 1, and the started line is still printed.
func TestRunExitsOneWhenNoFinalEvent(t *testing.T) {
	runAgent := func(_ context.Context, workflowID, _ string, emit func(events.Event)) {
		emit(events.Event{Type: events.WorkflowStarted, WorkflowID: workflowID})
	}
	var out strings.Builder

	code := run(t.Context(), "Handle item-1", runAgent, &out)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	lines := outputLines(out.String())
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "workflow.started") {
		t.Errorf("output = %q, want one workflow.started line", out.String())
	}
}

// A runner that emits workflow.started then workflow.completed exits 0.
func TestRunExitsZeroWhenCompleted(t *testing.T) {
	runAgent := func(_ context.Context, workflowID, _ string, emit func(events.Event)) {
		emit(events.Event{Type: events.WorkflowStarted, WorkflowID: workflowID})
		emit(events.Event{Type: events.WorkflowCompleted, WorkflowID: workflowID})
	}

	code := run(t.Context(), "Handle item-1", runAgent, &strings.Builder{})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// The task reaches the model unchanged, as the user message right after the support prompt.
func TestRunSendsTheTaskAsTheUserMessage(t *testing.T) {
	task := "Handle these work items:\n- item-1 (customer_message): \"I was charged twice and need help.\""
	m := &scriptedModel{replies: []agents.Message{{Role: "assistant", Content: "done", FinishReason: "stop"}}}

	run(t.Context(), task, agents.NewSupportAgent(m).Run, &strings.Builder{})

	if len(m.calls) == 0 {
		t.Fatal("the model was never called")
	}
	got := m.calls[0]
	if len(got) != 2 {
		t.Fatalf("first call sent %d messages, want 2: %+v", len(got), got)
	}
	if got[0].Role != "system" || got[0].Content != agents.SupportPrompt {
		t.Errorf("message 0 = %+v, want the support prompt as the system message", got[0])
	}
	if got[1].Role != "user" || got[1].Content != task {
		t.Errorf("message 1 = %+v, want the task as the user message", got[1])
	}
}

// The model returns an error: the last line is workflow.failed and the exit code is 1.
func TestRunExitsOneWhenTheModelFails(t *testing.T) {
	m := &scriptedModel{errs: []error{errors.New("boom")}}
	var out strings.Builder

	code := run(t.Context(), "Handle item-1", agents.NewSupportAgent(m).Run, &out)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	lines := outputLines(out.String())
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "workflow.failed") {
		t.Errorf("last line = %q, want it to start with workflow.failed", last)
	}
}

// A cancelled context, as after Ctrl-C, ends the run in workflow.failed and exit code 1.
func TestRunExitsOneWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m := &scriptedModel{replies: []agents.Message{{Role: "assistant", Content: "done", FinishReason: "stop"}}}
	var out strings.Builder

	code := run(ctx, "Handle item-1", agents.NewSupportAgent(m).Run, &out)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	lines := outputLines(out.String())
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "workflow.failed") {
		t.Errorf("last line = %q, want it to start with workflow.failed", last)
	}
	if len(m.calls) != 0 {
		t.Errorf("the model was called %d times after cancel, want 0", len(m.calls))
	}
}

// The whole four-line sample task is the task, without the trailing newline.
func TestReadTaskReadsAllOfStdin(t *testing.T) {
	sample := "Handle these work items:\n" +
		"- item-1 (customer_message): \"I was charged twice and need help.\"\n" +
		"- item-2 (bug_report): \"The export button fails on Safari.\"\n" +
		"- item-3 (sales_request): \"Can you send pricing for 50 seats?\"\n"

	got, err := readTask(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("readTask returned an error: %v", err)
	}
	if want := strings.TrimSuffix(sample, "\n"); got != want {
		t.Errorf("readTask = %q, want %q", got, want)
	}
}

// Empty stdin, or stdin with only whitespace, is refused with "no task on stdin".
func TestReadTaskRefusesBlankStdin(t *testing.T) {
	for _, in := range []string{"", "  \n\t\n"} {
		_, err := readTask(strings.NewReader(in))
		if err == nil {
			t.Errorf("readTask(%q) returned no error", in)
			continue
		}
		if !strings.Contains(err.Error(), "no task on stdin") {
			t.Errorf("readTask(%q) error = %q, want it to say \"no task on stdin\"", in, err)
		}
	}
}
