package model

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

const (
	DefaultModel = "z-ai/glm-5.3-flash"
	MaxTokens    = 64000
)

type OpenRouter struct {
	client  *openrouter.OpenRouter
	modelID string
}

func New(apiKey, modelID string, opts ...openrouter.SDKOption) *OpenRouter {
	options := append([]openrouter.SDKOption{openrouter.WithSecurity(apiKey)}, opts...)
	return &OpenRouter{
		client:  openrouter.New(options...),
		modelID: modelID,
	}
}

func (m *OpenRouter) Next(ctx context.Context, messages []agents.Message, toolList []tools.Tool) (agents.Message, error) {
	chatMessages, err := toChatMessages(messages)
	if err != nil {
		return agents.Message{}, err
	}

	request := components.ChatRequest{
		Model:     &m.modelID,
		Messages:  chatMessages,
		Tools:     toChatTools(toolList),
		MaxTokens: optionalnullable.From(ptr(int64(MaxTokens))),
		Provider: optionalnullable.From(&components.ProviderPreferences{
			RequireParameters: optionalnullable.From(ptr(true)),
		}),
	}

	response, err := m.client.Chat.Send(ctx, request, nil)
	if err != nil {
		return agents.Message{}, fmt.Errorf("openrouter request failed: %w", err)
	}
	if response.ChatResult == nil {
		// streaming mode is disabled so this a case we don't expect
		if response.EventStream != nil {
			_ = response.EventStream.Close()
		}
		return agents.Message{}, errors.New("openrouter reply had no result")
	}
	// successful model replies should contain an element in Choices
	if len(response.ChatResult.Choices) == 0 {
		return agents.Message{}, errors.New("openrouter returned no choices")
	}
	return fromChatChoice(response.ChatResult.Choices[0]), nil
}

func ptr[T any](value T) *T {
	return &value
}

func toChatMessages(messages []agents.Message) ([]components.ChatMessages, error) {
	chatMessages := make([]components.ChatMessages, 0, len(messages))
	for i, message := range messages {
		switch message.Role {
		case "system":
			chatMessages = append(chatMessages, components.CreateChatMessagesSystem(components.ChatSystemMessage{
				Role:    components.ChatSystemMessageRoleSystem,
				Content: components.CreateChatSystemMessageContentStr(message.Content),
			}))
		case "user":
			chatMessages = append(chatMessages, components.CreateChatMessagesUser(components.ChatUserMessage{
				Role:    components.ChatUserMessageRoleUser,
				Content: components.CreateChatUserMessageContentStr(message.Content),
			}))
		case "assistant":
			chatMessages = append(chatMessages, components.CreateChatMessagesAssistant(toAssistantMessage(message)))
		case "tool":
			chatMessages = append(chatMessages, components.CreateChatMessagesTool(components.ChatToolMessage{
				Role:       components.ChatToolMessageRoleTool,
				ToolCallID: message.ToolCallID,
				Content:    components.CreateChatToolMessageContentStr(message.Content),
			}))
		default:
			return nil, fmt.Errorf("message %d has unknown role %q", i, message.Role)
		}
	}
	return chatMessages, nil
}

func toAssistantMessage(message agents.Message) components.ChatAssistantMessage {
	assistant := components.ChatAssistantMessage{Role: components.ChatAssistantMessageRoleAssistant}
	if message.Content != "" {
		assistant.Content = optionalnullable.From(ptr(components.CreateChatAssistantMessageContentStr(message.Content)))
	}
	for _, call := range message.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, components.ChatToolCall{
			ID:   call.ID,
			Type: components.ChatToolCallTypeFunction,
			Function: components.ChatToolCallFunction{
				Name:      call.Name,
				Arguments: string(call.Args),
			},
		})
	}
	return assistant
}

func toChatTools(toolList []tools.Tool) []components.ChatFunctionTool {
	var chatTools []components.ChatFunctionTool
	for _, tool := range toolList {
		chatTools = append(chatTools, components.CreateChatFunctionToolChatFunctionToolFunction(
			components.ChatFunctionToolFunction{
				Type: components.ChatFunctionToolTypeFunction,
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        tool.Name,
					Description: ptr(tool.Description),
					Parameters:  tool.Parameters,
				},
			}))
	}
	return chatTools
}

func fromChatChoice(choice components.ChatChoice) agents.Message {
	reply := agents.Message{Role: "assistant"}
	if content, ok := choice.Message.Content.GetOrZero(); ok && content.Str != nil {
		reply.Content = *content.Str
	}
	// finish reason tells the "nature" of the model reply
	// stop -> final answer
	// tool_calls -> asks for tools to be run
	// length, content_filter or error -> errors
	if choice.FinishReason != nil {
		reply.FinishReason = string(*choice.FinishReason)
	}
	for _, call := range choice.Message.ToolCalls {
		toolCall := agents.ToolCall{ID: call.ID, Name: call.Function.Name}
		if call.Function.Arguments != "" {
			toolCall.Args = json.RawMessage(call.Function.Arguments)
		}
		reply.ToolCalls = append(reply.ToolCalls, toolCall)
	}
	return reply
}
