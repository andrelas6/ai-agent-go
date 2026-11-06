# ae-plan: The real OpenRouter model in agent-server

> **Status:** Done, merged in [andrelas6/ai-agent-go#24](https://github.com/andrelas6/ai-agent-go/pull/24) and [andrelas6/ai-agent-go#25](https://github.com/andrelas6/ai-agent-go/pull/25) (`9619991`)
> **Last updated:** 2026-11-07
> **From:** ae-init/openrouter-model.md, which is subtask 7 of ae-plan/lesson-1-harness.md, on `main` at `f04b4cb`

## What's being built

`internal/model`, the real `agents.Model`: it turns our `[]agents.Message` and `[]tools.Tool` into one OpenRouter `Chat.Send` call and turns the reply back into an `agents.Message`. `agent-server` runs `agents.NewSupportAgent(model).Run` on it by default, and the stub with `-stub`. With no key and no `-stub`, the server doesn't start. When this lands, the course's sample task runs end to end in the inspector. This is the lesson-1 milestone.

## Constraints

- **Dependencies:** the standard library, the OpenRouter SDK (`go-sdk` v0.9.42, already in `go.mod`) and `github.com/google/uuid` (`AGENTS.md`). The flag uses the standard `flag` package.
- **Decided in the ae-init:**
  - default model `z-ai/glm-5.3-flash`, overridden by `OPENROUTER_MODEL`;
  - `max_tokens: 64000`, sent as `max_tokens`, never `max_completion_tokens`;
  - `provider.require_parameters: true`;
  - no retries: a 500 fails at once;
  - the SDK's 60s per-call timeout stays;
  - `ctx` goes into `Chat.Send`;
  - `OPENROUTER_API_KEY` and `OPENROUTER_MODEL` are read in `main`, not in `internal/model`;
  - `-stub` runs `runStub`; `runStub` and its test stay;
  - no key and no `-stub`: exit non-zero before listening.
- **Tests use `httptest.NewServer` plus `openrouter.WithServerURL`.** No test touches the network.
- **No streaming.** `Stream` stays false, as in the lesson-1 plan.
- **Out of scope:** `agent-cli` (subtask 8, which reuses `internal/model`), the sample task file, sending back the model's `reasoning`.
- **No comments** in new code. André's existing comments stay.

## What I found in the codebase

- **`internal/agents/agent.go`** defines the interface the caller owns: `Model.Next(ctx, []Message, []tools.Tool) (Message, error)`. `Message` has `Role` (`system`, `user`, `assistant`, `tool`), `Content`, `ToolCalls []ToolCall{ID, Name, Args json.RawMessage}`, `ToolCallID` and `FinishReason` (a plain string, `""` when unknown). The loop already handles:
  - `length`, `content_filter` and `error` → `workflow.failed` "model reply was cut off";
  - text sent with tool calls → `model.delta`;
  - empty `Args` → `tool.failed` "no arguments sent";
  - no text and no tool calls → `workflow.failed`.

  So the adapter only translates. It decides nothing.
- **`internal/agents/support.go`:** `NewSupportAgent(model)` passes `SupportPrompt` and the four tools to `NewAgent`. Its `Run` has the `handlers.Runner` shape.
- **`internal/tools/tool.go`:** `Tool{Name, Description, Parameters map[string]any, Run}`. `Parameters` drops straight into `ChatFunctionToolFunctionFunction.Parameters`.
- **`cmd/agent-server/main.go`:** `handlers.NewMux(ctx, bus.New(ctx), runStub)`, with André's commented-out `// agent := &agent.Agent{}` line above it (it stays). No flags, no env vars yet.
- **`cmd/agent-server/stub_test.go`:** calls `runStub` with a slice-backed `emit` and compares events after zeroing `ID` and `TS`. The runner-choice test can reuse that pattern.
- **`cmd/agent-cli/main.go`:** builds a client with `openrouter.WithSecurity`, and sends `MaxCompletionTokens: 64000` with no tools. Nothing to reuse except the constructor call. Subtask 8 replaces it.
- **Test style:** standard `testing`, `t.Context()`, plain `t.Errorf`/`t.Fatalf`, black-box `_test` packages (`agents_test`). No helpers library.

## What the SDK and the docs say

- **SDK source (`go-sdk` v0.9.42 in the module cache):**
  - `chat.go`: with no retry config, `Send` retries `5XX` and connection errors with backoff for up to 3600000 ms. `openrouter.WithRetryConfig(retry.Config{Strategy: "none"})` (package `github.com/OpenRouterTeam/go-sdk/retry`) makes it send once.
  - `Send` builds the request with `http.NewRequestWithContext(ctx, …)` and wraps transport errors with `%w`, so a cancelled `ctx` should come back as `context.Canceled`.
  - A 200 reply is parsed only when its `Content-Type` is `application/json`. The fake server must set it.
  - Request: `ChatRequest{Model *string, Messages, Tools []ChatFunctionTool, MaxTokens OptionalNullable[int64], Provider OptionalNullable[ProviderPreferences]}`, where `ProviderPreferences.RequireParameters` is an `OptionalNullable[bool]`.
  - Reply: `res.ChatResult.Choices[i]` has `FinishReason *ChatFinishReasonEnum` (may be nil) and `Message ChatAssistantMessage`. `Content` is optional and nullable (read `.Str`). `ToolCalls[i].Function.Arguments` is a JSON string. `Reasoning` and `ReasoningDetails` are there too, and we ignore them.
  - Message constructors are in `docs/openrouter-agent-loop.md`. An assistant message we send back is rebuilt from our `Message`: `Role: assistant`, `Content` set only when non-empty, and each tool call as `{ID, Type: "function", Function{Name, Arguments: string(Args)}}`.
- **OpenRouter, [provider selection](https://openrouter.ai/docs/guides/routing/provider-selection):**
  - `require_parameters` (default `false`): "Only use providers that support all parameters in your request."
  - With the default, providers that don't support a parameter "can still receive the request, but will ignore unknown parameters". `tools` is only a soft preference ("best effort"), so `require_parameters: true` is what makes it strict.
  - With `max_tokens` set, OpenRouter "will only route to providers that support a response of that length".
  - What OpenRouter returns when no provider qualifies isn't documented. Whatever it is comes back as an SDK error and becomes `workflow.failed`. The by-hand run notes what it looks like if it ever happens.
- **Go, [`net/http/httptest`](https://pkg.go.dev/net/http/httptest):** `Server.Close` blocks until handlers return, so the hanging handler must wait on `r.Context().Done()`, not on a channel the test never closes.

## Approach

- **`internal/model/openrouter.go`:**
  - `DefaultModel = "z-ai/glm-5.3-flash"` and `MaxTokens = 64000` as constants;
  - `New(apiKey, modelID string, opts ...openrouter.SDKOption) *OpenRouter`, which builds the client with `WithSecurity(apiKey)`, `WithRetryConfig(retry.Config{Strategy: "none"})`, then the caller's `opts`. The tests pass `WithServerURL(srv.URL)`; `main` passes none.
  - `Next(ctx, messages, tools)`:
    - maps each `Message` by `Role` to a `components.ChatMessages` (an unknown role is an error);
    - maps each `Tool` to a `ChatFunctionTool`;
    - sends one `ChatRequest` with the model, `MaxTokens: 64000` and `Provider{RequireParameters: true}`;
    - maps `Choices[0]` back: `Content` from `.Str`, each tool call's `Arguments` as `json.RawMessage` (`""` becomes `nil`, so the loop says "no arguments sent"), `FinishReason` as a string or `""`.

    A nil `ChatResult` or empty `Choices` is an error.
- **`cmd/agent-server`:**
  - `runner.go` has `newRunner(stub bool, apiKey, modelID string) (handlers.Runner, string, error)`. It returns `runStub` with "running the stub", or the support agent's `Run` with "agent: <model>". With no key and no stub, it returns an error that names `OPENROUTER_API_KEY` and `-stub`.
  - `main` parses `-stub` with `flag`, reads the two env vars (`OPENROUTER_MODEL` empty → `model.DefaultModel`), calls `newRunner`, logs the line, and `log.Fatal`s on the error before `ListenAndServe`.

## Red / Green

**Runner choice (`cmd/agent-server`)**
- `newRunner(true, "", "")` returns a runner that emits the stub's three events, and the log line "running the stub".
- `newRunner(false, "", m)` returns an error that mentions `OPENROUTER_API_KEY` and `-stub`.
- `newRunner(false, "key", "z-ai/glm-5.3-flash")` returns a runner and a log line naming the model. The runner isn't called, so there's no network.
- `runStub`'s existing test still passes.

**Request shape (`internal/model`, against `httptest`)**
- `POST /chat/completions` with `Authorization: Bearer test-key`.
- The body has `"model"`, `"max_tokens": 64000` and `"provider": {"require_parameters": true}`, and no `max_completion_tokens`.
- `messages` keep the order and roles: system, user, assistant with `tool_calls` (`id`, `type: "function"`, `function.name`, `function.arguments` as a string), then `tool` with the matching `tool_call_id`.
- An assistant message with tool calls and no text sends no text content.
- `tools` lists each tool as `{type: "function", function: {name, description, parameters}}`, with the schema passed through unchanged.
- An unknown role is an error, and nothing is sent.

**Reply mapping**
- A text reply comes back as `Message{Role: "assistant", Content: "…", FinishReason: "stop"}`.
- A tool-call reply comes back with `ToolCalls[i].ID`, `.Name` and `.Args` intact, in order, and the `Args` bytes equal to the `arguments` string.
- A tool call with `"arguments": ""` comes back with nil `Args`.
- `finish_reason: "length"` comes through as `FinishReason: "length"`. A null `finish_reason` comes through as `""`.
- Empty `choices` is an error.

**Failure and cancellation**
- The server hangs, and `ctx` is cancelled after 50ms: `Next` returns an error that `errors.Is(err, context.Canceled)`, well under a second.
- The server answers 500: `Next` returns an error, and the server saw exactly one request.
- Every `internal/model` test runs under `context.WithTimeout(t.Context(), 2*time.Second)`, the pattern in `agent_test.go`. Without that, a fake server that returns 5XX would make the SDK retry for up to an hour, and `go test` would panic at its 10-minute default.

**End to end, by hand** (the ae-init's "How to reproduce", not automated)
- The sample task runs in the inspector with tool events for all three items and ends in `workflow.completed`.
- Plus the browser checks never run so far: two tabs, a reload mid-run, Clear, `curl -N`, the `model.delta` display, Ctrl-C mid-run, and what the provider sends for a call with no arguments.

## Most important file changes

| File | Change |
|---|---|
| `internal/model/openrouter.go` | new: `DefaultModel`, `MaxTokens`, `New`, `Next` and the message, tool and reply mapping |
| `internal/model/openrouter_test.go` | new: the `httptest` tests above |
| `cmd/agent-server/runner.go`, `runner_test.go` | new: `newRunner` |
| `cmd/agent-server/main.go` | `-stub` flag, env vars, `newRunner`, startup log, exit before listening |
| `docs/ae-plan/lesson-1-harness.md` | subtask 7 status and a "How subtask 7 landed" note |

`cmd/agent-server/stub.go`, `stub_test.go`, `internal/agents/` and `internal/tools/` don't change.

## Risks

- **Reasoning models.** GLM 5.3 Flash can return `reasoning` and `reasoning_details`, which we drop. If a provider expects them back across tool turns, later turns could get worse or fail. The by-hand run shows it. If it breaks the sample task, subtask 5 isn't done, because it blocks the milestone. First rerun with another `OPENROUTER_MODEL` to confirm the cause, then add a subtask that sends them back.
- **The unmerged `tools-broken-schema-panics` branch** (`69213ca`, `745b2c9`) only changes `internal/tools/schema.go` and the tool tests. No constructor or `NewSupportAgent` signature changes, so this plan works on top of it or without it.

## Open questions

- None.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit.

1. ✅ **The server picks its runner.** (andrelas6/ai-agent-go#24) `cmd/agent-server/runner.go` with `newRunner`, `-stub`, the env vars, the startup log, and the exit with no key. The model behind it is a placeholder: `internal/model.New` returns an `*OpenRouter` whose `Next` returns `errors.New("model not built yet")`, plus `DefaultModel`.
   - Tests: the three runner-choice cases.
   - **When this lands:** `-stub` gives A exactly. With a key, a task gives `workflow.started` → `workflow.failed` "model not built yet". With neither, the server exits with the message.
2. ✅ **`Next` sends the real request and reads a text reply.** (andrelas6/ai-agent-go#24) Also covers empty `choices`, a null `finish_reason` and a stream reply, so subtask 3's real Reds are only the tool-call reply and empty `arguments`. It replaces the placeholder. Message, tool and request mapping, `max_tokens`, `require_parameters`, auth, the text reply, and `ctx` passed into `Send`.
   - Tests: request shape, the text reply, and the hanging-server cancel test. That one is a real Red here, because the placeholder returns "model not built yet", not `context.Canceled`. Retries are still on, so check that it returns in under a second anyway.
   - **When this lands:** with a key, the model gets the sample task and the tools. A plain question ("hi") gets a text answer and ends in `model.completed` → `workflow.completed`. Tool calls in the reply aren't read yet, so the sample task ends in `workflow.failed` "model returned no text and no tool calls". Ctrl-C stops an in-flight call.
3. ✅ **Tool-call replies, finish reason and empty choices.**
   - Tests: the tool-call reply, empty `arguments`, `length` and null `finish_reason`, empty `choices`.
   - **When this lands:** the sample task runs end to end in the inspector.
4. ✅ **No retries.** `WithRetryConfig(retry.Config{Strategy: "none"})` in `New`.
   - Test: the 500 test. Red: without the option, the SDK retries until the test's 2s deadline, so the server sees more than one request and the error is a deadline error.
   - **When this lands:** an OpenRouter 5XX shows as `workflow.failed` at once.
5. ✅ **By-hand check and docs.** Done on 2026-11-06. Everything in the ae-init checked out, and the OpenRouter activity page shows the calls. The model never sent a tool call with no arguments, and it sent `model.delta` with each round of tool calls. **Actual size:** 805 lines of code and tests against `main` (`openrouter_test.go` alone is 530), over the limit, so it was split: #24 has subtasks 1–2 (616 lines), and #25 has subtasks 3–4 (191 lines) plus this docs update. Run the ae-init's "How to reproduce" end to end, including the browser checks never run so far. Write down what the provider sent for a call with no arguments, and whether `model.delta` showed up. Update `lesson-1-harness.md`: subtask 7 ✅, a "How subtask 7 landed" note, and the two places that still name gemma (the "No streaming" constraint and the "Model adapter" Red/Green bullet) now say `z-ai/glm-5.3-flash`. Then add up the diff.
   - **Size estimate:** about 120 lines in `openrouter.go`, 300 in its tests, 40 in `runner.go` and `main.go`, and 50 in `runner_test.go`. That's roughly 500 with tests, at the limit. If it ends up over, split into two PRs, each runnable on its own:
     - **PR 1:** subtasks 1–2. The server picks its runner, and the model answers in text.
     - **PR 2:** subtasks 3–5. Tool calls, no retries, and the milestone check.
