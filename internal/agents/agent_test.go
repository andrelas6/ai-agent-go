package agents_test

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/events"
	"andrelas6/ai-agent-go/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"
)

func fakeEmitter(backChannel chan<- events.Event, t *testing.T) func(events.Event) {
	return func(event events.Event) {
		if _, err := json.Marshal(event); err != nil {
			t.Fatalf("error marshalling event %v", err)
		}

		// capture call
		select {
		case backChannel <- event:
		case <-time.After(time.Second * 1):
			t.Fatal("fakeEmitter timeout - calls buffer is full")
		}
	}

}

func newFakeModel(replies ...fakeReply) fakeModel {
	repliesCh := make(chan fakeReply, len(replies))
	for _, reply := range replies {
		repliesCh <- reply
	}
	return fakeModel{
		// +1 because if the test calls without replies (or there aer no replies left), the Next function records that with the "no scripted reply left"
		nextCallsCh: make(chan nextCalls, len(replies)+1),
		repliesCh:   repliesCh,
	}
}

type fakeReply struct {
	message agents.Message
	err     error
}

type nextCalls struct {
	messages []agents.Message
	tools    []tools.Tool
}
type fakeModel struct {
	nextCallsCh chan nextCalls
	repliesCh   chan fakeReply
}

func (f fakeModel) Next(ctx context.Context, messages []agents.Message, tools []tools.Tool) (agents.Message, error) {
	select {
	case f.nextCallsCh <- nextCalls{messages: messages, tools: tools}:
	default:
		return agents.Message{}, errors.New("fakeModel: called too many times")
	}
	select {
	case reply := <-f.repliesCh:
		return reply.message, reply.err
	default:
		return agents.Message{}, errors.New("fakeModel: no scripted reply left")
	}
}

// Scenario: the model answers the task straight away with text, no tools.
// Outcome: started, model.completed, workflow.completed with that text; the model got only the user message.
func TestAgentRunBasic(t *testing.T) {
	// setup
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	expectedEmitterCalls := 3
	calls := make(chan events.Event, expectedEmitterCalls)

	fakeWorkflowId := "1234"
	input := "input"
	// TODO: maybe assistant, user, system should be in a type somewhere
	fM := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
	a := agents.NewAgent("", fM)

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
		want := []agents.Message{{Role: "user", Content: "input"}}
		if !reflect.DeepEqual(nextCall.messages, want) {
			t.Fatalf("model got messages %+v. expected %+v", nextCall.messages, want)
		}
	case <-time.After(time.Second * 1):
		t.Fatal("assert timeout 1s")
	}

}

// Scenario: the model call returns an error.
// Outcome: started, then workflow.failed with the error text; the model is not called again.
func TestAgentEmitErrorWhenModelErrors(t *testing.T) {
	// setup
	expectedEmitterCalls := 2
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	errorMsg := "model is unavailable"
	m := newFakeModel(fakeReply{err: errors.New(errorMsg)})
	input := "hi"

	a := agents.NewAgent("", m)
	workflowID := "1234"
	// expect 2 calls - if the emitter tries to write a third time, it will block and the test will fail
	calls := make(chan events.Event, expectedEmitterCalls)

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

// Scenario: the agent has a tool and the model answers straight away.
// Outcome: the model receives exactly the agent's tool list.
func TestAgentPassTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	// setup
	workflowID := "1234"
	expectedEmits := 3
	fModel := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
	dummyTool := tools.Tool{
		Name: "dummy-tool",
	}
	tools := []tools.Tool{dummyTool}
	a := agents.NewAgent("", fModel, tools...)

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

// TODO: test will have to be refactored because response with tool calls will be handled
// Scenario: the model replies with no text and no tool calls.
// Outcome: one workflow.failed saying so, and the model is called once.
func TestAgentFailsOnceOnUnknownModelResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	// setup
	workflowID := "1234"
	expectedEmits := 2
	fModel := newFakeModel(fakeReply{message: agents.Message{Role: "assistant"}})
	a := agents.NewAgent("", fModel)

	// execute
	backChannel := make(chan events.Event, expectedEmits)
	a.Run(ctx, workflowID, "hi", fakeEmitter(backChannel, t))

	// assert
	if started := <-backChannel; started.Type != events.WorkflowStarted {
		t.Fatalf("got %v. expected %v", started.Type, events.WorkflowStarted)
	}
	failed := <-backChannel
	if failed.Type != events.WorkflowFailed {
		t.Fatalf("got %v. expected %v", failed.Type, events.WorkflowFailed)
	}
	if failed.WorkflowID != workflowID {
		t.Fatalf("got %v. expected %v", failed.WorkflowID, workflowID)
	}
	wantError := "model returned no text and no tool calls"
	if failed.Error != wantError {
		t.Fatalf("got %v. expected %v", failed.Error, wantError)
	}
	if got := len(fModel.nextCallsCh); got != 1 {
		t.Fatalf("model called %d times. expected 1", got)
	}
}

// Scenario: the model calls one tool, then answers with text.
// Outcome: requested and completed events for the call, and the second model call sees the call and its result.
func TestAgentRunsToolCallThenAnswers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)
	assistantWithToolCall := agents.Message{
		Role:      "assistant",
		ToolCalls: []agents.ToolCall{{ID: "call-1", Name: "dummy-tool", Args: args}},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithToolCall},
		fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
	)

	var gotArgs json.RawMessage
	tools := []tools.Tool{{
		Name: "dummy-tool",
		Run: func(ctx context.Context, toolArgs json.RawMessage) (any, error) {
			gotArgs = toolArgs
			return map[string]bool{"ok": true}, nil
		},
	}}
	a := agents.NewAgent("", fModel, tools...)

	backChannel := make(chan events.Event, 5)
	a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}
	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-1", Result: result},
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
	}

	if !reflect.DeepEqual(gotArgs, args) {
		t.Fatalf("tool got args %s. expected %s", gotArgs, args)
	}

	if got := len(fModel.nextCallsCh); got != 2 {
		t.Fatalf("model called %d times. expected 2", got)
	}
	<-fModel.nextCallsCh
	secondCall := <-fModel.nextCallsCh
	wantMessages := []agents.Message{
		{Role: "user", Content: input},
		assistantWithToolCall,
		{Role: "tool", Content: string(result), ToolCallID: "call-1"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}

// Scenario: the model asks for two different tools in one reply, then answers.
// Outcome: each call runs in order with its own result, and the next model call sees one tool message per call.
func TestAgentRunsSeveralToolCallsInOneTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	// setup

	args := json.RawMessage(`{"q":"x"}`)
	result1 := json.RawMessage(`{"ok":"tool-1"}`)
	result2 := json.RawMessage(`{"ok":"tool-2"}`)
	input := "hi"
	assistantWithTwoToolCalls := agents.Message{
		Role: "assistant",
		ToolCalls: []agents.ToolCall{
			{ID: "call-1", Name: "dummy-tool", Args: args},
			{ID: "call-2", Name: "dummy-tool-2", Args: args},
		},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithTwoToolCalls},
		fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
	)

	workflowID := "1234"

	dummyTool := tools.Tool{
		Name: "dummy-tool",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			// no op
			return map[string]string{"ok": "tool-1"}, nil
		},
	}
	dummyTool2 := tools.Tool{
		Name: "dummy-tool-2",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]string{"ok": "tool-2"}, nil
		},
	}
	tools := []tools.Tool{dummyTool, dummyTool2}
	a := agents.NewAgent("", fModel, tools...)

	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-1", Result: result1},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-2", Name: "dummy-tool-2", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-2", Result: result2},
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}

	backChannel := make(chan events.Event, len(wantEvents))

	// execute
	a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	// assert

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}

	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got %v. expected %v", gotEvents, wantEvents)
	}

	<-fModel.nextCallsCh
	secondCall := <-fModel.nextCallsCh
	wantMessages := []agents.Message{
		{Role: "user", Content: input},
		assistantWithTwoToolCalls,
		{Role: "tool", Content: string(result1), ToolCallID: "call-1"},
		{Role: "tool", Content: string(result2), ToolCallID: "call-2"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}

// Scenario: a tool call fails: unknown tool, tool error, no arguments, bad JSON arguments or an unmarshalable result.
// Outcome: tool.failed with the reason, the error goes back to the model, and the run continues to the answer.
func TestAgentContinuesWhenToolCallErrors(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	validArgs := json.RawMessage(`{"q":"x"}`)

	tests := []struct {
		name        string
		call        agents.ToolCall
		toolResult  any
		toolErr     error
		wantToolRan bool
		wantError   string
		wantEvents  []events.Event
	}{
		{
			name:        "unknown tool name",
			call:        agents.ToolCall{ID: "call-1", Name: "missing-tool", Args: validArgs},
			wantToolRan: false,
			wantError:   `unknown tool "missing-tool"`,
			wantEvents: []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "missing-tool", Args: validArgs},
				{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: `unknown tool "missing-tool"`},
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
			},
		},
		{
			name:        "tool returns an error",
			call:        agents.ToolCall{ID: "call-1", Name: "dummy-tool", Args: validArgs},
			toolErr:     errors.New("tool blew up"),
			wantToolRan: true,
			wantError:   "tool blew up",
			wantEvents: []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: validArgs},
				{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: "tool blew up"},
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
			},
		},
		{
			name:        "arguments are not valid JSON",
			call:        agents.ToolCall{ID: "call-1", Name: "dummy-tool", Args: json.RawMessage(`{"q":`)},
			wantToolRan: false,
			wantError:   "invalid JSON arguments",
			wantEvents: []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: json.RawMessage(`"{\"q\":"`)},
				{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: "invalid JSON arguments"},
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
			},
		},
		{
			name:        "no arguments sent",
			call:        agents.ToolCall{ID: "call-1", Name: "dummy-tool"},
			wantToolRan: false,
			wantError:   "no arguments sent",
			wantEvents: []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool"},
				{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: "no arguments sent"},
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
			},
		},
		{
			name:        "result won't marshal",
			call:        agents.ToolCall{ID: "call-1", Name: "dummy-tool", Args: validArgs},
			toolResult:  make(chan int),
			wantToolRan: true,
			wantError:   "json: unsupported type: chan int",
			wantEvents: []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: validArgs},
				{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: "json: unsupported type: chan int"},
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
			defer cancel()

			assistantWithToolCall := agents.Message{Role: "assistant", ToolCalls: []agents.ToolCall{tt.call}}
			fModel := newFakeModel(
				fakeReply{message: assistantWithToolCall},
				fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
			)

			toolRan := false
			tools := []tools.Tool{{
				Name: "dummy-tool",
				Run: func(ctx context.Context, args json.RawMessage) (any, error) {
					toolRan = true
					return tt.toolResult, tt.toolErr
				},
			}}
			a := agents.NewAgent("", fModel, tools...)

			backChannel := make(chan events.Event, len(tt.wantEvents))

			// execute
			a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
			close(backChannel)

			// assert
			var gotEvents []events.Event
			for event := range backChannel {
				gotEvents = append(gotEvents, event)
			}
			if !reflect.DeepEqual(gotEvents, tt.wantEvents) {
				t.Fatalf("got events %+v. expected %+v", gotEvents, tt.wantEvents)
			}

			if toolRan != tt.wantToolRan {
				t.Fatalf("tool ran: got %v. expected %v", toolRan, tt.wantToolRan)
			}

			if got := len(fModel.nextCallsCh); got != 2 {
				t.Fatalf("model called %d times. expected 2", got)
			}
			<-fModel.nextCallsCh
			secondCall := <-fModel.nextCallsCh
			wantMessages := []agents.Message{
				{Role: "user", Content: input},
				assistantWithToolCall,
				{Role: "tool", Content: "error: " + tt.wantError, ToolCallID: "call-1"},
			}
			if !reflect.DeepEqual(secondCall.messages, wantMessages) {
				t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
			}
		})
	}
}

// Scenario: the model asks for two tools in one reply and the first one fails.
// Outcome: the second still runs, and the model gets an error message for the first and a result for the second.
func TestAgentRunsSecondToolCallWhenFirstFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)
	toolErrorText := "tool blew up"
	assistantWithTwoToolCalls := agents.Message{
		Role: "assistant",
		ToolCalls: []agents.ToolCall{
			{ID: "call-1", Name: "failing-tool", Args: args},
			{ID: "call-2", Name: "working-tool", Args: args},
		},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithTwoToolCalls},
		fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
	)

	tools := []tools.Tool{
		{
			Name: "failing-tool",
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				return nil, errors.New(toolErrorText)
			},
		},
		{
			Name: "working-tool",
			Run: func(ctx context.Context, args json.RawMessage) (any, error) {
				return map[string]bool{"ok": true}, nil
			},
		},
	}
	a := agents.NewAgent("", fModel, tools...)

	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "failing-tool", Args: args},
		{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: "call-1", Error: toolErrorText},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-2", Name: "working-tool", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-2", Result: result},
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}
	backChannel := make(chan events.Event, len(wantEvents))

	a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
	}

	if got := len(fModel.nextCallsCh); got != 2 {
		t.Fatalf("model called %d times. expected 2", got)
	}
	<-fModel.nextCallsCh
	secondCall := <-fModel.nextCallsCh
	wantMessages := []agents.Message{
		{Role: "user", Content: input},
		assistantWithTwoToolCalls,
		{Role: "tool", Content: "error: " + toolErrorText, ToolCallID: "call-1"},
		{Role: "tool", Content: string(result), ToolCallID: "call-2"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}

// Scenario: the model keeps calling tools for 10 turns, with tools that work or keep failing, or answers on turn 10.
// Outcome: workflow.failed with the step-limit message after exactly 10 model calls, or a normal finish on turn 10.
func TestAgentStepLimit(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)
	toolErrorText := "tool blew up"
	stepLimitError := "Hit the 10-step limit without finishing."

	started := []events.Event{{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input}}
	hitTheLimit := []events.Event{{Type: events.WorkflowFailed, WorkflowID: workflowID, Error: stepLimitError}}
	answered := []events.Event{
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}

	toolCallReplies := func(count int) []fakeReply {
		var replies []fakeReply
		for i := range count {
			replies = append(replies, fakeReply{message: agents.Message{
				Role:      "assistant",
				ToolCalls: []agents.ToolCall{{ID: fmt.Sprintf("call-%d", i+1), Name: "dummy-tool", Args: args}},
			}})
		}
		return replies
	}
	toolTurns := func(count int, outcome func(callID string) events.Event) []events.Event {
		var turns []events.Event
		for i := range count {
			callID := fmt.Sprintf("call-%d", i+1)
			turns = append(turns,
				events.Event{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: callID, Name: "dummy-tool", Args: args},
				outcome(callID),
			)
		}
		return turns
	}
	toolCompleted := func(callID string) events.Event {
		return events.Event{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: callID, Result: result}
	}
	toolFailed := func(callID string) events.Event {
		return events.Event{Type: events.ToolFailed, WorkflowID: workflowID, ToolCallID: callID, Error: toolErrorText}
	}

	tests := []struct {
		name           string
		replies        []fakeReply
		toolErr        error
		wantModelCalls int
		wantEvents     []events.Event
	}{
		{
			name:           "model never stops calling tools",
			replies:        toolCallReplies(10),
			wantModelCalls: 10,
			wantEvents:     slices.Concat(started, toolTurns(10, toolCompleted), hitTheLimit),
		},
		{
			name:           "model answers on the last step",
			replies:        append(toolCallReplies(9), fakeReply{message: agents.Message{Content: "done", Role: "assistant"}}),
			wantModelCalls: 10,
			wantEvents:     slices.Concat(started, toolTurns(9, toolCompleted), answered),
		},
		{
			name:           "tool keeps failing",
			replies:        toolCallReplies(10),
			toolErr:        errors.New(toolErrorText),
			wantModelCalls: 10,
			wantEvents:     slices.Concat(started, toolTurns(10, toolFailed), hitTheLimit),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
			defer cancel()

			fModel := newFakeModel(tt.replies...)
			tools := []tools.Tool{{
				Name: "dummy-tool",
				Run: func(ctx context.Context, args json.RawMessage) (any, error) {
					return map[string]bool{"ok": true}, tt.toolErr
				},
			}}
			a := agents.NewAgent("", fModel, tools...)

			backChannel := make(chan events.Event, len(tt.wantEvents))

			a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
			close(backChannel)

			var gotEvents []events.Event
			for event := range backChannel {
				gotEvents = append(gotEvents, event)
			}
			if !reflect.DeepEqual(gotEvents, tt.wantEvents) {
				t.Fatalf("got events %+v. expected %+v", gotEvents, tt.wantEvents)
			}

			if got := len(fModel.nextCallsCh); got != tt.wantModelCalls {
				t.Fatalf("model called %d times. expected %d", got, tt.wantModelCalls)
			}
		})
	}
}

// Scenario: the context is cancelled before the run, during the first of two tools, or during the last tool of a turn.
// Outcome: workflow.failed with "context canceled", and no tool or model call starts after the cancel.
func TestAgentStopsWhenContextIsCancelled(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)

	started := events.Event{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input}
	cancelled := events.Event{Type: events.WorkflowFailed, WorkflowID: workflowID, Error: "context canceled"}
	firstToolRequested := events.Event{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "first-tool", Args: args}
	firstToolCompleted := events.Event{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-1", Result: result}
	secondToolRequested := events.Event{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-2", Name: "second-tool", Args: args}
	secondToolCompleted := events.Event{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-2", Result: result}
	done := fakeReply{message: agents.Message{Content: "done", Role: "assistant"}}

	tests := []struct {
		name            string
		cancelBeforeRun bool
		replies         []fakeReply
		cancellingTool  string
		wantToolsRan    []string
		wantModelCalls  int
		wantEvents      []events.Event
	}{
		{
			name:            "cancelled before the run starts",
			cancelBeforeRun: true,
			replies:         []fakeReply{done},
			wantToolsRan:    nil,
			wantModelCalls:  0,
			wantEvents:      []events.Event{started, cancelled},
		},
		{
			name: "cancelled during the first of two tools",
			replies: []fakeReply{
				{message: agents.Message{Role: "assistant", ToolCalls: []agents.ToolCall{
					{ID: "call-1", Name: "first-tool", Args: args},
					{ID: "call-2", Name: "second-tool", Args: args},
				}}},
				done,
			},
			cancellingTool: "first-tool",
			wantToolsRan:   []string{"first-tool"},
			wantModelCalls: 1,
			wantEvents:     []events.Event{started, firstToolRequested, firstToolCompleted, cancelled},
		},
		{
			name: "cancelled during the last tool of a turn",
			replies: []fakeReply{
				{message: agents.Message{Role: "assistant", ToolCalls: []agents.ToolCall{
					{ID: "call-1", Name: "first-tool", Args: args},
					{ID: "call-2", Name: "second-tool", Args: args},
				}}},
				done,
			},
			cancellingTool: "second-tool",
			wantToolsRan:   []string{"first-tool", "second-tool"},
			wantModelCalls: 1,
			wantEvents:     []events.Event{started, firstToolRequested, firstToolCompleted, secondToolRequested, secondToolCompleted, cancelled},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
			defer cancel()

			fModel := newFakeModel(tt.replies...)

			var toolsRan []string
			newTool := func(name string) tools.Tool {
				return tools.Tool{
					Name: name,
					Run: func(ctx context.Context, args json.RawMessage) (any, error) {
						toolsRan = append(toolsRan, name)
						if name == tt.cancellingTool {
							cancel()
						}
						return map[string]bool{"ok": true}, nil
					},
				}
			}
			tools := []tools.Tool{newTool("first-tool"), newTool("second-tool")}
			a := agents.NewAgent("", fModel, tools...)

			backChannel := make(chan events.Event, len(tt.wantEvents))

			if tt.cancelBeforeRun {
				cancel()
			}
			a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
			close(backChannel)

			var gotEvents []events.Event
			for event := range backChannel {
				gotEvents = append(gotEvents, event)
			}
			if !reflect.DeepEqual(gotEvents, tt.wantEvents) {
				t.Fatalf("got events %+v. expected %+v", gotEvents, tt.wantEvents)
			}

			if !reflect.DeepEqual(toolsRan, tt.wantToolsRan) {
				t.Fatalf("tools ran %v. expected %v", toolsRan, tt.wantToolsRan)
			}

			if got := len(fModel.nextCallsCh); got != tt.wantModelCalls {
				t.Fatalf("model called %d times. expected %d", got, tt.wantModelCalls)
			}
		})
	}
}

type blockingModel struct {
	started chan struct{}
}

// don't process any messages - just "waits to be cancelled"
// so that the cancelling of a model while it's still in transit can be tested
func (m blockingModel) Next(ctx context.Context, messages []agents.Message, tools []tools.Tool) (agents.Message, error) {
	close(m.started)
	select {
	case <-ctx.Done():
		return agents.Message{}, ctx.Err()
	case <-time.After(time.Second):
		return agents.Message{}, errors.New("blockingModel: context never cancelled")
	}
}

// Scenario: the context is cancelled while the model call is still waiting.
// Outcome: the model call returns the context error and the run ends in workflow.failed.
func TestAgentStopsWhenCancelledDuringModelCall(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	workflowID := "1234"
	input := "hi"
	model := blockingModel{started: make(chan struct{})}
	a := agents.NewAgent("", model)

	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.WorkflowFailed, WorkflowID: workflowID, Error: "context canceled"},
	}
	backChannel := make(chan events.Event, len(wantEvents))

	go func() {
		<-model.started
		cancel()
	}()
	a.Run(ctx, workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
	}
}

// Scenario: the agent has a system prompt, or an empty one.
// Outcome: the model gets the system prompt before the user message, or no system message at all.
func TestAgentSendsSystemPromptFirst(t *testing.T) {
	input := "hi"

	tests := []struct {
		name         string
		systemPrompt string
		wantMessages []agents.Message
	}{
		{
			name:         "prompt is set",
			systemPrompt: "be helpful",
			wantMessages: []agents.Message{
				{Role: "system", Content: "be helpful"},
				{Role: "user", Content: input},
			},
		},
		{
			name:         "prompt is empty",
			systemPrompt: "",
			wantMessages: []agents.Message{
				{Role: "user", Content: input},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
			defer cancel()

			fModel := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
			a := agents.NewAgent(tt.systemPrompt, fModel)

			backChannel := make(chan events.Event, 3)
			a.Run(ctx, "1234", input, fakeEmitter(backChannel, t))

			if got := len(fModel.nextCallsCh); got != 1 {
				t.Fatalf("model called %d times. expected 1", got)
			}
			firstCall := <-fModel.nextCallsCh
			if !reflect.DeepEqual(firstCall.messages, tt.wantMessages) {
				t.Fatalf("model got messages %+v. expected %+v", firstCall.messages, tt.wantMessages)
			}
		})
	}
}

// Scenario: the agent has no model: a zero-value Agent, or NewAgent with nil.
// Outcome: started, then workflow.failed "agent has no model", with no panic.
func TestAgentWithoutAModelFailsCleanly(t *testing.T) {
	workflowID := "1234"
	input := "hi"

	tests := []struct {
		name  string
		agent *agents.Agent
	}{
		{name: "zero value", agent: &agents.Agent{}},
		{name: "nil model", agent: agents.NewAgent("", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantEvents := []events.Event{
				{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
				{Type: events.WorkflowFailed, WorkflowID: workflowID, Error: "agent has no model"},
			}
			backChannel := make(chan events.Event, len(wantEvents))

			tt.agent.Run(t.Context(), workflowID, input, fakeEmitter(backChannel, t))
			close(backChannel)

			var gotEvents []events.Event
			for event := range backChannel {
				gotEvents = append(gotEvents, event)
			}
			if !reflect.DeepEqual(gotEvents, wantEvents) {
				t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
			}
		})
	}
}

// Scenario: the model answers with text but stopped on length, content_filter, error, or a normal stop.
// Outcome: workflow.failed "model reply was cut off" for the first three, a normal finish for stop.
func TestAgentHandlesTheModelFinishReason(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	started := events.Event{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input}

	cutOff := func(reason string) []events.Event {
		return []events.Event{
			started,
			{Type: events.WorkflowFailed, WorkflowID: workflowID, Error: "model reply was cut off: " + reason},
		}
	}

	tests := []struct {
		name         string
		finishReason string
		wantEvents   []events.Event
	}{
		{name: "token limit", finishReason: "length", wantEvents: cutOff("length")},
		{name: "content filter", finishReason: "content_filter", wantEvents: cutOff("content_filter")},
		{name: "provider error", finishReason: "error", wantEvents: cutOff("error")},
		{
			name:         "normal stop",
			finishReason: "stop",
			wantEvents: []events.Event{
				started,
				{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "partial answ"},
				{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "partial answ"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fModel := newFakeModel(fakeReply{message: agents.Message{
				Role:         "assistant",
				Content:      "partial answ",
				FinishReason: tt.finishReason,
			}})
			a := agents.NewAgent("", fModel)

			backChannel := make(chan events.Event, len(tt.wantEvents))
			a.Run(t.Context(), workflowID, input, fakeEmitter(backChannel, t))
			close(backChannel)

			var gotEvents []events.Event
			for event := range backChannel {
				gotEvents = append(gotEvents, event)
			}
			if !reflect.DeepEqual(gotEvents, tt.wantEvents) {
				t.Fatalf("got events %+v. expected %+v", gotEvents, tt.wantEvents)
			}
		})
	}
}

// Scenario: the model asks for a tool but its reply stopped on length.
// Outcome: the tool still runs and the run carries on, because the tool calls take priority over the finish reason.
func TestAgentRunsToolCallsWhateverTheFinishReason(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	fModel := newFakeModel(
		fakeReply{message: agents.Message{
			Role:         "assistant",
			FinishReason: "length",
			ToolCalls:    []agents.ToolCall{{ID: "call-1", Name: "dummy-tool", Args: args}},
		}},
		fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
	)
	tool := tools.Tool{
		Name: "dummy-tool",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]bool{"ok": true}, nil
		},
	}
	a := agents.NewAgent("", fModel, tool)

	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-1", Result: json.RawMessage(`{"ok":true}`)},
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}
	backChannel := make(chan events.Event, len(wantEvents))
	a.Run(t.Context(), workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
	}
}

// Scenario: the model sends text together with a tool call.
// Outcome: model.delta with that text comes before the tool events; the final answer is still model.completed.
func TestAgentShowsTextSentWithToolCalls(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	fModel := newFakeModel(
		fakeReply{message: agents.Message{
			Role:      "assistant",
			Content:   "let me check that",
			ToolCalls: []agents.ToolCall{{ID: "call-1", Name: "dummy-tool", Args: args}},
		}},
		fakeReply{message: agents.Message{Content: "done", Role: "assistant"}},
	)
	tool := tools.Tool{
		Name: "dummy-tool",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]bool{"ok": true}, nil
		},
	}
	a := agents.NewAgent("", fModel, tool)

	wantEvents := []events.Event{
		{Type: events.WorkflowStarted, WorkflowID: workflowID, Input: input},
		{Type: events.ModelDelta, WorkflowID: workflowID, Text: "let me check that"},
		{Type: events.ToolRequested, WorkflowID: workflowID, ToolCallID: "call-1", Name: "dummy-tool", Args: args},
		{Type: events.ToolCompleted, WorkflowID: workflowID, ToolCallID: "call-1", Result: json.RawMessage(`{"ok":true}`)},
		{Type: events.ModelCompleted, WorkflowID: workflowID, Text: "done"},
		{Type: events.WorkflowCompleted, WorkflowID: workflowID, Output: "done"},
	}
	backChannel := make(chan events.Event, len(wantEvents))
	a.Run(t.Context(), workflowID, input, fakeEmitter(backChannel, t))
	close(backChannel)

	var gotEvents []events.Event
	for event := range backChannel {
		gotEvents = append(gotEvents, event)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("got events %+v. expected %+v", gotEvents, wantEvents)
	}
}
