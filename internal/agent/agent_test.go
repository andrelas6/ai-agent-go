package agent_test

import (
	"andrelas6/ai-agent-go/internal/agent"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"errors"
	"testing"
	"time"
)

func fakeEmitter(calls chan<- events.Event, t *testing.T) func(events.Event) {
	return func(event events.Event) {
		// capture call
		select {
		case calls <- event:
		case <-time.After(time.Second * 1):
			t.Fatal("fakeEmitter timeout - calls buffer is full")
		}
	}

}

func newFakeModel(callsBufferSize int) fakeModel {
	errCh := make(chan string, 1)
	calls := make(chan []agent.Message, callsBufferSize)
	return fakeModel{
		calls: calls,
		errCh: errCh,
	}
}

type fakeModel struct {
	calls chan []agent.Message
	errCh chan string
}

func (f fakeModel) Next(ctx context.Context, messages []agent.Message) (agent.Message, error) {
	var toolCalls []agent.ToolCall
	select {
	case <-ctx.Done():
		return agent.Message{}, ctx.Err()
	case f.calls <- messages:
		select {
		case msg := <-f.errCh:
			return agent.Message{}, errors.New(msg)
		default:
			// TODO: maybe assistant, user, system should be in a type somewhere
			return agent.Message{Content: "done", Role: "assistant", ToolCalls: toolCalls, ToolCallID: ""}, nil
		}
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
	a.Run(ctx, fakeWorkflowId, input, fakeEmitter(calls, t))

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

func TestAgentEmitErrorWhenModelErrors(t *testing.T) {
	// setup
	expectedEmitterCalls := 2
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	m := newFakeModel(3)
	errorMsg := "model is unavailable"
	input := "hi"

	a := agent.NewAgent("", m)
	workflowID := "1234"
	// expect 2 calls - if the emitter tries to write a third time, it will block and the test will fail
	calls := make(chan events.Event, expectedEmitterCalls)
	m.errCh <- errorMsg

	// execute
	a.Run(ctx, workflowID, input, fakeEmitter(calls, t))

	// assert
	assertEmittedEvents := func(it int, e events.Event) {
		if e.WorkflowID != workflowID {
			t.Fatalf("got %v. expected %v", e.WorkflowID, workflowID)
		}
		switch it {
		case 0:
			if e.Type != events.WorkflowStarted {
				t.Fatalf("WorkflowStarted:type - got %v. expected %v", e.Type, events.WorkflowStarted)
			}
			if e.Input != input {
				t.Fatalf("WorkflowStarted:input - got %v. expected %v", e.Input, input)
			}
		case 1:
			if e.Type != events.WorkflowFailed {
				t.Fatalf("WorkflowFailed:type - got %v. expected %v", e.Type, events.WorkflowFailed)
			}
			if e.Error != errorMsg {
				t.Fatalf("WorkflowFailed:error - got %v. expected %v", e.Error, errorMsg)
			}
		default:
			t.Fatal("unknown case")
		}
	}

	for i := range expectedEmitterCalls {
		select {
		case event := <-calls:
			assertEmittedEvents(i, event)
		case <-time.After(time.Second * 1):
			t.Fatal("for select timeout")
			return
		}
	}

	if got := len(m.calls); got != 1 {
		t.Fatalf("model called %d times. expected 1", got)
	}
}
