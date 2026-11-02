package agents_test

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Scenario: the support agent runs a task and the model answers straight away.
// Outcome: the model is offered the four support tools, in the course's order.
func TestSupportAgentOffersTheSupportTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	fM := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
	agents.NewSupportAgent(fM).Run(ctx, "1234", "input", func(events.Event) {})

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

// Scenario: the support agent runs a task.
// Outcome: the model first gets the course's support prompt as the system message, and the prompt names every tool the agent offers.
func TestSupportAgentSendsTheSupportPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
	defer cancel()

	fM := newFakeModel(fakeReply{message: agents.Message{Content: "done", Role: "assistant"}})
	agents.NewSupportAgent(fM).Run(ctx, "1234", "input", func(events.Event) {})

	want := `You are a support triage agent.

For each work item the user gives you:
1. Classify it with classifyItem.
2. Search the knowledge base with searchKnowledgeBase if it helps.
3. Draft a reply with draftReply.
4. Send the reply with sendReply.

Work through every item, then briefly summarize what you did.`

	select {
	case nextCall := <-fM.nextCallsCh:
		if len(nextCall.messages) == 0 {
			t.Fatal("model got no messages")
		}
		system := nextCall.messages[0]
		if system.Role != "system" || system.Content != want {
			t.Errorf("first message = %+v, want the system prompt %q", system, want)
		}
		for _, tool := range nextCall.tools {
			if !strings.Contains(system.Content, tool.Name) {
				t.Errorf("system prompt does not name %s", tool.Name)
			}
		}
	case <-time.After(time.Second * 1):
		t.Fatal("assert timeout 1s")
	}
}
