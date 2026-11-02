package agents_test

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"reflect"
	"testing"
	"time"
)

// Scenario: the support agent runs a task and the model answers straight away.
// Outcome: the model is offered the four support tools, in the course's order.
func TestSupportAgentOffersTheSupportTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	fM := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
	calls := make(chan events.Event, 3)

	agents.NewSupportAgent(fM).Run(ctx, "1234", "input", fakeEmitter(calls, t))

	select {
	case nextCall := <-fM.nextCallsCh:
		var got []string
		for _, tool := range nextCall.tools {
			got = append(got, tool.Name)
		}
		want := []string{"searchKnowledgeBase", "classifyItem", "draftReply", "sendReply"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("model got tools %v, want %v", got, want)
		}
	case <-time.After(time.Second * 1):
		t.Fatal("assert timeout 1s")
	}
}
