package agent

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Run         func(ctx context.Context, args json.RawMessage) any
}

type Model interface {
	Next(ctx context.Context, messages []Message) (Message, error)
}

type Agent struct {
	// dependencies
	Model        Model
	MaxSteps     int
	SystemPrompt string
}

func NewAgent(systemPrompt string, model Model) *Agent {
	return &Agent{
		Model:        model,
		MaxSteps:     10,
		SystemPrompt: systemPrompt,
	}
}

func (a *Agent) Run(ctx context.Context, workflowID string, input string, emit func(events.Event)) {
	messages := make([]Message, 0)

	eventStarted := events.Event{
		WorkflowID: workflowID,
		Type:       events.WorkflowStarted,
		Input:      input,
	}
	emit(eventStarted)

	if len(a.SystemPrompt) > 0 {
		messages = append(messages, Message{
			Content: a.SystemPrompt,
			Role:    "system",
		})
	}

	messages = append(messages, Message{
		Content: input,
		Role:    "user",
	})

	var step int
	for ; step < a.MaxSteps; step++ {
		message, err := a.Model.Next(ctx, slices.Clone(messages))
		if err != nil {
			emit(events.Event{
				WorkflowID: workflowID,
				Type:       events.WorkflowFailed,
				Error:      err.Error(),
			})
			return
		}
		// TODO: handle these cases (and possibly a bit more)
		// 1 - model completed
		// 2 - tool call
		// 3 - tool result
		// 4 - err (already handled above and in some other places)

		// for now only one case - model completed
		if len(message.ToolCalls) == 0 {
			messages = append(messages, message)
			emit(events.Event{
				Type:       events.ModelCompleted,
				Text:       message.Content,
				WorkflowID: workflowID,
			})
			break
		}
	}

	// error
	if step == a.MaxSteps {
		emit(events.Event{
			WorkflowID: workflowID,
			Type:       events.WorkflowFailed,
			Error:      fmt.Sprintf("agent loop reached max steps (%v) without answer", a.MaxSteps),
		})
		return
	}

	eventCompleted := events.Event{
		Output:     messages[len(messages)-1].Content,
		WorkflowID: workflowID,
		Type:       events.WorkflowCompleted,
	}
	emit(eventCompleted)
}
