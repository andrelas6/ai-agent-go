package main

import (
	"andrelas6/ai-agent-go/internal/tools"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type ModelResponseType string

const (
	FINISHED       ModelResponseType = "finished"
	CALL_TOOL      ModelResponseType = "call_tool"
	NOT_FINISHED   ModelResponseType = "not_finished"
	CUT_OFF        ModelResponseType = "cut_off"
	CONTENT_FILTER ModelResponseType = "content_filter"
	ERROR          ModelResponseType = "error"
	UNKNOWN        ModelResponseType = "unknown"
)

func main() {
	ctx := context.Background()
	clientModel := ClientModel{
		instance: openrouter.New(openrouter.WithSecurity(os.Getenv("OPENROUTER_API_KEY"))),
	}
	scanner := bufio.NewScanner(os.Stdin)

	claudeDocs, _ := tools.ReadFile("testdata/claude_hooks.md")

	var conversation []components.ChatMessages
	for scanner.Scan() {
		text := scanner.Text()

		// set system prompt

		// userMessage, _ := clientModel.getMessage("user", text)
		userMessageAndPrompt, _ := clientModel.getMessage("user", fmt.Sprintf("%s\n%s", claudeDocs, text))
		conversation = append(conversation, *userMessageAndPrompt)

		chatResponse := clientModel.chatOpenRouter(ctx, conversation)

		if chatResponse.ResponseType != NOT_FINISHED {
			break
		}

		assistantMessage, _ := clientModel.getMessage("assistant", chatResponse.Message)
		conversation = append(conversation, *assistantMessage)
	}

}

type ClientModel struct {
	instance *openrouter.OpenRouter
}

func (c *ClientModel) getMessage(userType string, message string) (*components.ChatMessages, bool) {
	if userType == "system" {
		chatMessages := components.CreateChatMessagesSystem(
			components.ChatSystemMessage{
				Content: components.CreateChatSystemMessageContentStr(message),
				Role:    components.ChatSystemMessageRoleSystem,
			},
		)
		return &chatMessages, true
	}
	if userType == "user" {
		chatMessages := components.CreateChatMessagesUser(
			components.ChatUserMessage{
				Content: components.CreateChatUserMessageContentStr(message),
				Role:    components.ChatUserMessageRoleUser,
			},
		)
		return &chatMessages, true
	}

	if userType == "assistant" {
		content := components.CreateChatAssistantMessageContentStr(message)
		chatMessages := components.CreateChatMessagesAssistant(
			components.ChatAssistantMessage{
				Content: optionalnullable.From(&content),
				Role:    components.ChatAssistantMessageRoleAssistant,
			},
		)
		return &chatMessages, true
	}

	return nil, false
}

type ChatOpenRouterResponse struct {
	Message      string
	Err          bool
	ResponseType ModelResponseType
}

func (c *ClientModel) chatOpenRouter(ctx context.Context, conversation []components.ChatMessages) ChatOpenRouterResponse {
	var model string = "google/gemma-4-31b-it"
	var maxTokens int64 = int64(64000)
	res, err := c.instance.Chat.Send(ctx, components.ChatRequest{
		Model:               &model,
		MaxCompletionTokens: optionalnullable.From(&maxTokens),
		Messages:            conversation,
	}, nil)
	if err != nil {
		log.Fatal(err)
		return ChatOpenRouterResponse{
			Err:          true,
			Message:      "",
			ResponseType: "",
		}
	}
	if res != nil {
		pretty, err := json.MarshalIndent(res.ChatResult.Choices, "", " ")
		if err != nil {
			fmt.Errorf("issue marshalling %w", err)
			return ChatOpenRouterResponse{
				Err:          true,
				Message:      "",
				ResponseType: "",
			}
		}
		fmt.Printf("RESPONSE: %v", string(pretty))
		return ChatOpenRouterResponse{
			Err:          false,
			Message:      string(pretty),
			ResponseType: getModelResponseType(res.ChatResult.Choices[0].FinishReason, res.ChatResult.Choices[0].Message.ToolCalls),
		} // if c, ok := res.ChatResult.Choices[0].Message.Content.GetOrZero(); ok && c.Str != nil {
		// fmt.Printf("MODEL RESPONDS: %v", *c.Str)
		// }
		// handle response
	}
	return ChatOpenRouterResponse{
		Err:          true,
		Message:      "",
		ResponseType: "",
	}
}

func getModelResponseType(finishReason *components.ChatFinishReasonEnum, toolCalls []components.ChatToolCall) ModelResponseType {
	if finishReason == nil {
		return UNKNOWN
	}

	switch *finishReason {
	case components.ChatFinishReasonEnumStop:
		if len(toolCalls) == 0 {
			return FINISHED
		}
		return NOT_FINISHED
	case components.ChatFinishReasonEnumToolCalls:
		return CALL_TOOL
	case components.ChatFinishReasonEnumLength:
		return CUT_OFF
	case components.ChatFinishReasonEnumContentFilter:
		return CONTENT_FILTER
	case components.ChatFinishReasonEnumError:
		return ERROR
	}

	return UNKNOWN
}
