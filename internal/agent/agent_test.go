package agent_test

import (
	"andrelas6/ai-agent-go/internal/agent"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"testing"
	"time"
)

func fakeEmitter(calls chan<- events.Event) func(events.Event) {
	return func(event events.Event) {
		// capture call
		calls <- event
	}

}

func newFakeModel(callsBufferSize int) fakeModel {
	calls := make(chan []agent.Message, callsBufferSize)
	return fakeModel{
		calls: calls,
	}
}

type fakeModel struct {
	calls chan []agent.Message
}

func (f fakeModel) Next(ctx context.Context, messages []agent.Message) (agent.Message, error) {
	var toolCalls []agent.ToolCall
	select {
	case <-ctx.Done():
		return agent.Message{}, nil
	case f.calls <- messages:
		// TODO: maybe assistant, user, system should be in a type somewhere
		return agent.Message{Content: "done", Role: "assistant", ToolCalls: toolCalls, ToolCallID: ""}, nil
	}
}

func TestAgentRunBasic(t *testing.T) {
	// setup
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	calls := make(chan events.Event, 3)

	fakeWorkflowId := "1234"
	input := "input"
	fM := newFakeModel(1)
	a := agent.NewAgent("", fM)

	indexToEventType := make(map[int]events.Type)
	indexToEventType[0] = events.WorkflowStarted
	indexToEventType[1] = events.ModelCompleted
	indexToEventType[2] = events.WorkflowCompleted

	// execute
	a.Run(ctx, fakeWorkflowId, input, fakeEmitter(calls))

	// assert
	// check sequence - first is started, second is model-completed and third is completed
	assertEmittedEvents := func(it int, e events.Event) {
		if e.WorkflowID != "1234" {
			t.Fatalf("got %v. expected %v", e.WorkflowID, fakeWorkflowId)
		}
		switch it {
		case 0:
			if e.Type != events.WorkflowStarted {
				t.Fatalf("WorkflowStarted:type - got %v. expected %v", e.Type, events.WorkflowStarted)
			}
			if e.Input != "input" {
				t.Fatalf("WorkflowStarted:input - got %v. expected %v", e.Input, input)
			}
		case 1:
			if e.Type != events.ModelCompleted {
				t.Fatalf("ModelCompleted:type - got %v. expected %v", e.Type, events.ModelCompleted)
			}
			if e.Text != "done" {
				t.Fatalf("ModelCompleted:text - got %v. expected %v", e.Text, "done")
			}
		case 2:
			if e.Type != events.WorkflowCompleted {
				t.Fatalf("WorkflowCompleted:type - got %v. expected %v", e.Type, events.WorkflowCompleted)
			}
			if e.Output != "done" {
				t.Fatalf("ModelCompleted:message - got %v. expected %v", e.Output, "done")
			}
		default:
			t.Fatal("unknown case")
		}
	}

	for i := range 3 {
		select {
		case emittedEvent := <-calls:
			assertEmittedEvents(i, emittedEvent)
			if i == 2 {
				return
			}
		case <-time.After(time.Second * 1):
			t.Fatal("timeout 1s")
			return
		}
	}

}
