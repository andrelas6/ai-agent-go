# Agent loop with the OpenRouter Go SDK

How to build the agent from [How to Build an Agent](https://ampcode.com/notes/how-to-build-an-agent) with the OpenRouter Go SDK (`github.com/OpenRouterTeam/go-sdk` v0.9.42, the version in `go.mod`) in place of the Anthropic SDK.

The loop has to do three things:

1. Send the conversation and the tool specs to the model.
2. Read the tool name and arguments the model wants to call, run the tool, and send the result back.
3. Decide when the model's reply is final, so the loop stops and waits for the user.

The type names below come from the SDK source in the module cache, and the full example at the bottom was compiled against it. Some examples in the online docs are out of date. For instance, the Chat page reads `res.ChatStreamingResponse`, which does not exist in v0.9.42. Trust the code here, or the source, over the web examples.

## Setup

```bash
go get github.com/OpenRouterTeam/go-sdk@v0.9.42 github.com/invopop/jsonschema
```

```go
import (
	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
	"github.com/invopop/jsonschema"
)

client := openrouter.New(openrouter.WithSecurity(os.Getenv("OPENROUTER_API_KEY")))
```

`client` is a `*openrouter.OpenRouter`.

## Anthropic → OpenRouter mapping

OpenRouter uses the OpenAI chat-completions format, not Anthropic's content blocks. This is the main thing that changes in the article's code:

| Concept | Anthropic SDK (article) | OpenRouter Go SDK |
| - | - | - |
| Call the model | `client.Messages.New(ctx, anthropic.MessageNewParams{...})` | `client.Chat.Send(ctx, components.ChatRequest{...}, nil)` |
| Conversation type | `[]anthropic.MessageParam` | `[]components.ChatMessages` |
| Model | `anthropic.ModelClaude3_7SonnetLatest` | `Model: &modelID` (a string like `"anthropic/claude-sonnet-4.5"`) |
| Max tokens | `MaxTokens: 1024` | `MaxTokens: optionalnullable.From(&n)` (`n` is an `int64`) |
| Tool spec | `anthropic.ToolParam{Name, Description, InputSchema}` | `components.ChatFunctionTool` wrapping `ChatFunctionToolFunctionFunction{Name, Description, Parameters}` |
| Schema type | `anthropic.ToolInputSchemaParam` | `map[string]any` (a plain JSON Schema object) |
| Model's reply | `message.Content[]` blocks (`text`, `tool_use`) | `res.ChatResult.Choices[0].Message` (`ChatAssistantMessage`) |
| Text in the reply | `content.Text` where `Type == "text"` | `Message.Content` (OptionalNullable, read `.Str`) |
| Tool call | `content.ID`, `content.Name`, `content.Input` (a `json.RawMessage`) | `ToolCalls[i].ID`, `.Function.Name`, `.Function.Arguments` (a JSON **string**) |
| Tool result | `anthropic.NewToolResultBlock(id, out, isErr)` inside **one user message** | **One `tool` message per call**: `CreateChatMessagesTool(ChatToolMessage{ToolCallID, Content})` |
| "Is it final?" | no `tool_use` blocks | `len(Message.ToolCalls) == 0` (or `FinishReason != "tool_calls"`) |

## 1. Send text and tool specs to the model

### Tool definition

Keep the article's `ToolDefinition`, but store the schema as `map[string]any`, because that is what `ChatFunctionToolFunctionFunction.Parameters` takes:

```go
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Function    func(input json.RawMessage) (string, error)
}
```

### Generating the schema

Use the same `invopop/jsonschema` reflector as the article. Round-trip it through JSON to get a `map[string]any`:

```go
func GenerateSchema[T any]() map[string]any {
	reflector := jsonschema.Reflector{AllowAdditionalProperties: false, DoNotReference: true}
	var v T
	raw, _ := json.Marshal(reflector.Reflect(v))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	delete(out, "$schema") // some providers reject the meta-schema key
	return out
}
```

For `ReadFileInput{Path string \`json:"path" jsonschema_description:"..."\`}` this produces:

```json
{"type":"object","properties":{"path":{"type":"string","description":"..."}},"required":["path"],"additionalProperties":false}
```

### Request

```go
func (a *Agent) runInference(ctx context.Context, conversation []components.ChatMessages) (*components.ChatChoice, error) {
	tools := make([]components.ChatFunctionTool, 0, len(a.tools))
	for _, t := range a.tools {
		desc := t.Description
		tools = append(tools, components.CreateChatFunctionToolChatFunctionToolFunction(
			components.ChatFunctionToolFunction{
				Type: components.ChatFunctionToolTypeFunction,
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        t.Name,
					Description: &desc,
					Parameters:  t.Parameters,
				},
			}))
	}

	maxTokens := int64(1024)
	res, err := a.client.Chat.Send(ctx, components.ChatRequest{
		Model:     &a.model,
		Messages:  conversation,
		Tools:     tools,
		MaxTokens: optionalnullable.From(&maxTokens),
	}, nil)
	if err != nil {
		return nil, err
	}
	if res.ChatResult == nil || len(res.ChatResult.Choices) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}
	return &res.ChatResult.Choices[0], nil
}
```

Notes:

- `Stream` defaults to `false`, so the response is in `res.ChatResult`. `res.EventStream` is only set when you stream.
- As with Anthropic, the API is stateless: send the whole `conversation` on every call.
- Pick a model that supports tool calling. Check the model's page on [openrouter.ai/models](https://openrouter.ai/models).

### Message constructors

| Role | Constructor |
| - | - |
| system | `components.CreateChatMessagesSystem(components.ChatSystemMessage{Content: components.CreateChatSystemMessageContentStr(s), Role: components.ChatSystemMessageRoleSystem})` |
| user | `components.CreateChatMessagesUser(components.ChatUserMessage{Content: components.CreateChatUserMessageContentStr(s), Role: components.ChatUserMessageRoleUser})` |
| assistant | `components.CreateChatMessagesAssistant(choice.Message)`: append the model's reply as-is |
| tool | `components.CreateChatMessagesTool(components.ChatToolMessage{Content: components.CreateChatToolMessageContentStr(out), Role: components.ChatToolMessageRoleTool, ToolCallID: id})` |

## 2. Read tool calls and execute them

Tool calls are on the assistant message:

```go
type ChatToolCall struct {
	Function ChatToolCallFunction // .Name string, .Arguments string (JSON text)
	ID       string
	Type     ChatToolCallType     // "function"
}
```

`Arguments` is a JSON string, so cast it to `json.RawMessage` and the article's tool functions work unchanged:

```go
func (a *Agent) executeTool(call components.ChatToolCall) string {
	for _, t := range a.tools {
		if t.Name == call.Function.Name {
			fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", call.Function.Name, call.Function.Arguments)
			out, err := t.Function(json.RawMessage(call.Function.Arguments))
			if err != nil {
				return "error: " + err.Error()
			}
			return out
		}
	}
	return "error: tool not found"
}
```

There is no `is_error` flag in the chat-completions format. Put the error text in the tool message content instead. The model reads it and can recover.

Two rules when sending results back:

1. **Append the assistant message, with its `ToolCalls`, before the tool messages.** Each tool message's `ToolCallID` must match a call in the assistant message right before it.
2. **Send one `tool` message per tool call.** This is different from Anthropic, where all results go in one user message.

## 3. Decide whether the reply is final

```go
type ChatChoice struct {
	FinishReason *ChatFinishReasonEnum // "tool_calls" | "stop" | "length" | "content_filter" | "error"
	Message      ChatAssistantMessage
}
```

- `len(choice.Message.ToolCalls) > 0`: run the tools, append the results, and **call the model again without asking the user**.
- No tool calls: the turn is over. Print the text and wait for user input.
- `FinishReason` is a pointer and may be nil. Checking `ToolCalls` is the more reliable signal, because some providers report `"stop"` even when tool calls are present. You can use `"length"`, `"content_filter"` and `"error"` to show a warning.

The text of the reply:

```go
if c, ok := msg.Content.GetOrZero(); ok && c.Str != nil && *c.Str != "" {
	fmt.Printf("\u001b[93mModel\u001b[0m: %s\n", *c.Str)
}
```

`Content` can be null when the model only returns tool calls, so check `ok` and `Str`.

## The loop

```go
type Agent struct {
	client         *openrouter.OpenRouter
	model          string
	getUserMessage func() (string, bool)
	tools          []ToolDefinition
}

func (a *Agent) Run(ctx context.Context) error {
	var conversation []components.ChatMessages
	readUserInput := true

	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			text, ok := a.getUserMessage()
			if !ok {
				return nil
			}
			conversation = append(conversation, components.CreateChatMessagesUser(components.ChatUserMessage{
				Content: components.CreateChatUserMessageContentStr(text),
				Role:    components.ChatUserMessageRoleUser,
			}))
		}

		// 1. send conversation + tools
		choice, err := a.runInference(ctx, conversation)
		if err != nil {
			return err
		}
		msg := choice.Message
		conversation = append(conversation, components.CreateChatMessagesAssistant(msg))

		if c, ok := msg.Content.GetOrZero(); ok && c.Str != nil && *c.Str != "" {
			fmt.Printf("\u001b[93mModel\u001b[0m: %s\n", *c.Str)
		}

		// 3. final answer? -> back to the user
		if len(msg.ToolCalls) == 0 {
			readUserInput = true
			continue
		}

		// 2. run each tool call and send its result back
		for _, call := range msg.ToolCalls {
			result := a.executeTool(call)
			conversation = append(conversation, components.CreateChatMessagesTool(components.ChatToolMessage{
				Content:    components.CreateChatToolMessageContentStr(result),
				Role:       components.ChatToolMessageRoleTool,
				ToolCallID: call.ID,
			}))
		}
		readUserInput = false
	}
}
```

## main

```go
func main() {
	client := openrouter.New(openrouter.WithSecurity(os.Getenv("OPENROUTER_API_KEY")))

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		return scanner.Text(), true
	}

	agent := &Agent{
		client:         client,
		model:          "anthropic/claude-sonnet-4.5",
		getUserMessage: getUserMessage,
		tools:          []ToolDefinition{ReadFileDefinition, ListFilesDefinition, EditFileDefinition},
	}
	if err := agent.Run(context.TODO()); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}
```

Tools from the article, with the `Parameters` field in place of `InputSchema`:

```go
type ReadFileInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

var ReadFileDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	Parameters:  GenerateSchema[ReadFileInput](),
	Function: func(input json.RawMessage) (string, error) {
		var in ReadFileInput
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
		b, err := os.ReadFile(in.Path)
		return string(b), err
	},
}
```

`list_files` (`Path string \`json:"path,omitempty"\``) and `edit_file` (`Path`, `OldStr` `json:"old_str"`, `NewStr` `json:"new_str"`) port the same way. Their `Function` bodies are identical to the article's.

## References

- Go SDK overview: https://openrouter.ai/docs/client-sdks/go/overview
- Chat.Send reference: https://openrouter.ai/docs/client-sdks/go/sdks/chat/README.md
- Tool calling guide (request/response format): https://openrouter.ai/docs/guides/features/tool-calling
- SDK source for the exact types: `$(go env GOMODCACHE)/github.com/!open!router!team/go-sdk@v0.9.42/models/components/` (`chatrequest.go`, `chatfunctiontool.go`, `chattoolcall.go`, `chattoolmessage.go`, `chatchoice.go`, `chatfinishreasonenum.go`)
