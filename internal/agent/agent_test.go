package agent_test

import (
	"andrelas6/ai-agent-go/internal/agent"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func fakeEmitter(backChannel chan<- events.Event, t *testing.T) func(events.Event) {
	return func(event events.Event) {
		if _, err := json.Marshal(event); err != nil {
			t.Fatalf("error unmasharling event %v", err)
		}

		// capture call
		select {
		case backChannel <- event:
		case <-time.After(time.Second * 1):
			t.Fatal("fakeEmitter timeout - calls buffer is full")
		}
	}

}

func newFakeModel(callsBufferSize int) fakeModel {
	errCh := make(chan string, 1)
	calls := make(chan nextCalls, callsBufferSize)
	return fakeModel{
		nextCallsCh: calls,
		errCh:       errCh,
	}
}

type nextCalls struct {
	messages []agent.Message
	tools    []agent.Tool
}
type fakeModel struct {
	nextCallsCh chan nextCalls
	errCh       chan string
}

func (f fakeModel) Next(ctx context.Context, messages []agent.Message, tools []agent.Tool) (agent.Message, error) {
	var toolCalls []agent.ToolCall
	select {
	case <-ctx.Done():
		return agent.Message{}, ctx.Err()
	case f.nextCallsCh <- nextCalls{messages: messages, tools: tools}:
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

	expectedEmitterCalls := 3
	calls := make(chan events.Event, expectedEmitterCalls)

	fakeWorkflowId := "1234"
	input := "input"
	fM := newFakeModel(expectedEmitterCalls)
	a := agent.NewAgent("", fM)

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

	for i := range expectedEmitterCalls {
		select {
		case emittedEvent := <-calls:
			assertEmittedEvents(i, emittedEvent)
		case <-time.After(time.Second * 1):
			t.Fatal("TestAgentRunBasic timeout 1s")
			return
		}
	}

	select {
	case nextCall := <-fM.nextCallsCh:
		want := []agent.Message{{Role: "user", Content: "input"}}
		if !reflect.DeepEqual(nextCall.messages, want) {
			t.Fatalf("model got messages %+v. expected %+v", nextCall.messages, want)
		}
	case <-time.After(time.Second * 1):
		t.Fatal("assert timeout 1s")
	}

}

func TestAgentEmitErrorWhenModelErrors(t *testing.T) {
	// setup
	expectedEmitterCalls := 2
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	m := newFakeModel(1)
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

	if got := len(m.nextCallsCh); got != 1 {
		t.Fatalf("model called %d times. expected 1", got)
	}
}

func TestAgentPassTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	// setup
	workflowID := "1234"
	expectedEmits := 3
	fModel := newFakeModel(1)
	a := agent.NewAgent("", fModel)
	dummyTool := agent.Tool{
		Name: "dummy-tool",
	}
	a.Tools = []agent.Tool{dummyTool}

	// execute
	backChannel := make(chan events.Event, expectedEmits)
	a.Run(ctx, workflowID, "hi", fakeEmitter(backChannel, t))

	// assert
	select {
	case nextCall := <-fModel.nextCallsCh:
		if len(nextCall.tools) != 1 || nextCall.tools[0].Name != dummyTool.Name {
			t.Fatalf("got %v. expected only one tool %v", nextCall.tools, dummyTool.Name)
		}
	case <-time.After(time.Second * 1):
		t.Fatal("assert timeout 1s")
	}
}
