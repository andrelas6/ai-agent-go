package agent

import (
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"encoding/json"
	"errors"
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
	Run         func(ctx context.Context, args json.RawMessage) (any, error)
}

type Model interface {
	Next(ctx context.Context, messages []Message, tools []Tool) (Message, error)
}

type Agent struct {
	// dependencies
	Model        Model
	MaxSteps     int
	SystemPrompt string
	Tools        []Tool
}

func NewAgent(systemPrompt string, model Model) *Agent {
	return &Agent{
		Model:        model,
		MaxSteps:     10,
		SystemPrompt: systemPrompt,
		Tools:        []Tool{},
	}
}

type ModelResponse string

const (
	ToolRequested  ModelResponse = "ToolRequested"
	ToolCompleted  ModelResponse = "ToolCompleted"
	ModelCompleted ModelResponse = "ModelCompleted"
	Unknown        ModelResponse = "Unknown"
)

func getModelResponse(message Message) ModelResponse {
	if len(message.ToolCalls) > 0 {
		return ToolRequested
	}
	if message.Content != "" {
		return ModelCompleted
	}
	return Unknown
}

func (a *Agent) runTool(ctx context.Context, call ToolCall) (json.RawMessage, error) {
	index := slices.IndexFunc(a.Tools, func(tool Tool) bool { return tool.Name == call.Name })
	if index == -1 {
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
	if len(call.Args) == 0 {
		return nil, errors.New("no arguments sent")
	}
	if !json.Valid(call.Args) {
		return nil, errors.New("invalid JSON arguments")
	}
	value, err := a.Tools[index].Run(ctx, call.Args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func jsonfyIfNeeded(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return nil
	}
	if json.Valid(args) {
		return args
	}
	quoted, _ := json.Marshal(string(args))
	return quoted
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

	for step := 0; step < a.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			emit(events.Event{
				WorkflowID: workflowID,
				Type:       events.WorkflowFailed,
				Error:      err.Error(),
			})
			return
		}

		// model run
		message, err := a.Model.Next(ctx, slices.Clone(messages), slices.Clone(a.Tools))
		if err != nil {
			emit(events.Event{
				WorkflowID: workflowID,
				Type:       events.WorkflowFailed,
				Error:      err.Error(),
			})
			return
		}

		modelResponse := getModelResponse(message)
		switch modelResponse {
		case ModelCompleted:
			messages = append(messages, message)
			emit(events.Event{
				Type:       events.ModelCompleted,
				Text:       message.Content,
				WorkflowID: workflowID,
			})
			emit(events.Event{
				Type:       events.WorkflowCompleted,
				Output:     message.Content,
				WorkflowID: workflowID,
			})
			return
		case ToolRequested:
			messages = append(messages, message)
			for _, call := range message.ToolCalls {
				// if context is cancelled, the tool call needs to stop
				if err := ctx.Err(); err != nil {
					emit(events.Event{
						WorkflowID: workflowID,
						Type:       events.WorkflowFailed,
						Error:      err.Error(),
					})
					return
				}

				emit(events.Event{
					WorkflowID: workflowID,
					Type:       events.ToolRequested,
					ToolCallID: call.ID,
					Name:       call.Name,
					// if args are invalid, they still need to be stringified to be accounted in the agent loop
					Args: jsonfyIfNeeded(call.Args),
				})
				result, err := a.runTool(ctx, call)
				if err != nil {
					emit(events.Event{
						WorkflowID: workflowID,
						Type:       events.ToolFailed,
						ToolCallID: call.ID,
						Error:      err.Error(),
					})
					messages = append(messages, Message{
						Role:       "tool",
						Content:    "error: " + err.Error(),
						ToolCallID: call.ID,
					})
					continue
				}
				emit(events.Event{
					WorkflowID: workflowID,
					Type:       events.ToolCompleted,
					ToolCallID: call.ID,
					Result:     result,
				})
				messages = append(messages, Message{
					Role:       "tool",
					Content:    string(result),
					ToolCallID: call.ID,
				})
			}
		// unknown is error
		default:
			emit(events.Event{
				WorkflowID: workflowID,
				Type:       events.WorkflowFailed,
				Error:      fmt.Sprintf("unknown message %v", message),
			})
			return
		}
	}

	// error
	// if model still has tool calls after maxSteps, it is not finished but limit is done
	emit(events.Event{
		WorkflowID: workflowID,
		Type:       events.WorkflowFailed,
		Error:      fmt.Sprintf("Hit the %d-step limit without finishing.", a.MaxSteps),
	})
}
