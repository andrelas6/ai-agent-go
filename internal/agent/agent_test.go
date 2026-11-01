package agent_test

import (
	"andrelas6/ai-agent-go/internal/agent"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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
	message agent.Message
	err     error
}

type nextCalls struct {
	messages []agent.Message
	tools    []agent.Tool
}
type fakeModel struct {
	nextCallsCh chan nextCalls
	repliesCh   chan fakeReply
}

func (f fakeModel) Next(ctx context.Context, messages []agent.Message, tools []agent.Tool) (agent.Message, error) {
	select {
	case <-ctx.Done():
		return agent.Message{}, ctx.Err()
	case f.nextCallsCh <- nextCalls{messages: messages, tools: tools}:
		select {
		case reply := <-f.repliesCh:
			return reply.message, reply.err
		default:
			return agent.Message{}, errors.New("fakeModel: no scripted reply left")
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
	// TODO: maybe assistant, user, system should be in a type somewhere
	fM := newFakeModel(fakeReply{message: agent.Message{Content: "done", Role: "assistant"}})
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
	errorMsg := "model is unavailable"
	m := newFakeModel(fakeReply{err: errors.New(errorMsg)})
	input := "hi"

	a := agent.NewAgent("", m)
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

func TestAgentPassTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	// setup
	workflowID := "1234"
	expectedEmits := 3
	fModel := newFakeModel(fakeReply{message: agent.Message{Content: "done", Role: "assistant"}})
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

// TODO: test will have to be refactored because response with tool calls will be handled
func TestAgentFailsOnceOnUnknownModelResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	// setup
	workflowID := "1234"
	expectedEmits := 2
	fModel := newFakeModel(fakeReply{message: agent.Message{Role: "assistant"}})
	a := agent.NewAgent("", fModel)

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
	if !strings.HasPrefix(failed.Error, "unknown message") {
		t.Fatalf("got %v. expected an unknown message error", failed.Error)
	}
	if got := len(fModel.nextCallsCh); got != 1 {
		t.Fatalf("model called %d times. expected 1", got)
	}
}

func TestAgentRunsToolCallThenAnswers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)
	assistantWithToolCall := agent.Message{
		Role:      "assistant",
		ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "dummy-tool", Args: args}},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithToolCall},
		fakeReply{message: agent.Message{Content: "done", Role: "assistant"}},
	)

	var gotArgs json.RawMessage
	a := agent.NewAgent("", fModel)
	a.Tools = []agent.Tool{{
		Name: "dummy-tool",
		Run: func(ctx context.Context, toolArgs json.RawMessage) (any, error) {
			gotArgs = toolArgs
			return map[string]bool{"ok": true}, nil
		},
	}}

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
	wantMessages := []agent.Message{
		{Role: "user", Content: input},
		assistantWithToolCall,
		{Role: "tool", Content: string(result), ToolCallID: "call-1"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}

func TestAgentRunsSeveralToolCallsInOneTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()
	// setup

	args := json.RawMessage(`{"q":"x"}`)
	result1 := json.RawMessage(`{"ok":"tool-1"}`)
	result2 := json.RawMessage(`{"ok":"tool-2"}`)
	input := "hi"
	assistantWithTwoToolCalls := agent.Message{
		Role: "assistant",
		ToolCalls: []agent.ToolCall{
			{ID: "call-1", Name: "dummy-tool", Args: args},
			{ID: "call-2", Name: "dummy-tool-2", Args: args},
		},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithTwoToolCalls},
		fakeReply{message: agent.Message{Content: "done", Role: "assistant"}},
	)

	workflowID := "1234"
	a := agent.NewAgent("", fModel)

	dummyTool := agent.Tool{
		Name: "dummy-tool",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			// no op
			return map[string]string{"ok": "tool-1"}, nil
		},
	}
	dummyTool2 := agent.Tool{
		Name: "dummy-tool-2",
		Run: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]string{"ok": "tool-2"}, nil
		},
	}
	a.Tools = []agent.Tool{dummyTool, dummyTool2}

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
	wantMessages := []agent.Message{
		{Role: "user", Content: input},
		assistantWithTwoToolCalls,
		{Role: "tool", Content: string(result1), ToolCallID: "call-1"},
		{Role: "tool", Content: string(result2), ToolCallID: "call-2"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}

func TestAgentContinuesWhenToolCallErrors(t *testing.T) {
	workflowID := "1234"
	input := "hi"
	validArgs := json.RawMessage(`{"q":"x"}`)

	tests := []struct {
		name        string
		call        agent.ToolCall
		toolResult  any
		toolErr     error
		wantToolRan bool
		wantError   string
		wantEvents  []events.Event
	}{
		{
			name:        "unknown tool name",
			call:        agent.ToolCall{ID: "call-1", Name: "missing-tool", Args: validArgs},
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
			call:        agent.ToolCall{ID: "call-1", Name: "dummy-tool", Args: validArgs},
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
			call:        agent.ToolCall{ID: "call-1", Name: "dummy-tool", Args: json.RawMessage(`{"q":`)},
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
			call:        agent.ToolCall{ID: "call-1", Name: "dummy-tool"},
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
			call:        agent.ToolCall{ID: "call-1", Name: "dummy-tool", Args: validArgs},
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

			assistantWithToolCall := agent.Message{Role: "assistant", ToolCalls: []agent.ToolCall{tt.call}}
			fModel := newFakeModel(
				fakeReply{message: assistantWithToolCall},
				fakeReply{message: agent.Message{Content: "done", Role: "assistant"}},
			)

			toolRan := false
			a := agent.NewAgent("", fModel)
			a.Tools = []agent.Tool{{
				Name: "dummy-tool",
				Run: func(ctx context.Context, args json.RawMessage) (any, error) {
					toolRan = true
					return tt.toolResult, tt.toolErr
				},
			}}

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
			wantMessages := []agent.Message{
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

func TestAgentRunsSecondToolCallWhenFirstFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	workflowID := "1234"
	input := "hi"
	args := json.RawMessage(`{"q":"x"}`)
	result := json.RawMessage(`{"ok":true}`)
	toolErrorText := "tool blew up"
	assistantWithTwoToolCalls := agent.Message{
		Role: "assistant",
		ToolCalls: []agent.ToolCall{
			{ID: "call-1", Name: "failing-tool", Args: args},
			{ID: "call-2", Name: "working-tool", Args: args},
		},
	}
	fModel := newFakeModel(
		fakeReply{message: assistantWithTwoToolCalls},
		fakeReply{message: agent.Message{Content: "done", Role: "assistant"}},
	)

	a := agent.NewAgent("", fModel)
	a.Tools = []agent.Tool{
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
	wantMessages := []agent.Message{
		{Role: "user", Content: input},
		assistantWithTwoToolCalls,
		{Role: "tool", Content: "error: " + toolErrorText, ToolCallID: "call-1"},
		{Role: "tool", Content: string(result), ToolCallID: "call-2"},
	}
	if !reflect.DeepEqual(secondCall.messages, wantMessages) {
		t.Fatalf("second model call got messages %+v. expected %+v", secondCall.messages, wantMessages)
	}
}
