# ae-plan: The agent loop, under test

> **Status:** Done
> **Last updated:** 2026-11-01 (all 7 subtasks done)
> **Progress:** 7 of 7 subtasks done, merged as [andrelas6/ai-agent-go#14](https://github.com/andrelas6/ai-agent-go/pull/14). The five main ae-review fixes are in open PR [andrelas6/ai-agent-go#15](https://github.com/andrelas6/ai-agent-go/pull/15): `Agent` fields unexported and `NewAgent` takes the tools (so the Approach section is further out of date), `Message.FinishReason`, server logging of failures, and `model.delta`. The smaller tidy-ups from `ae-review/pr-14.md` come later.
> **Owner:** André builds it
> **From:** ae-init/stub-agent-loop.md, which is parent ae-plan/lesson-1-harness.md subtask 5

## What's being built

`internal/agent.Run`, the course's brittle `runAgent` loop in Go.
- It sends the conversation and the tool list to a model.
- It runs each tool call the model asks for and sends the result back.
- It stops when the model answers with no tool calls, after 10 tool turns, on a model error, or when the context is cancelled.
- It emits each step as an `events.Event`.

In this plan the model and the tools are scripted fakes in the test file, so the loop is proven by `go test` alone. `agent-server` keeps running `runStub`. `internal/runtime/` is deleted.

## Constraints

- **Dependencies:** standard library only. `internal/agent` does not import the OpenRouter SDK. It defines its own message, tool-call and tool types, and parent subtask 7's adapter converts between them and the SDK.
- **Layout:** `internal/agent` because `agent-server` and `agent-cli` will both run it. The fakes live in `agent_test.go`, the only place that uses them.
- **Events match the course** (`shared/events.ts` on `lesson-2`):
  - `tool.requested {toolCallId, name, args}`;
  - `tool.completed {toolCallId, result}`;
  - `tool.failed {toolCallId, error}`;
  - `model.completed {text}` only for the final answer, followed by `workflow.completed {output: text}`;
  - `workflow.failed {error}`.
- **Every emitted event must survive `json.Marshal`.** `Args` and `Result` are `json.RawMessage`. One bad value makes `flushEventData` fail and closes the SSE stream for that tab. The loop owns that boundary, not the tools.
- **Cancel aborts in-flight work.** The model gets the run's `ctx`, and the loop starts no model or tool call after `ctx` is done. Aborting the real HTTP call is parent subtask 7's job: it must pass `ctx` into `Chat.Send`, with a test against an `httptest` server that hangs.
- **Not touched:** `cmd/agent-server` (it still runs `runStub`), `cmd/agent-cli`, `internal/tools`.
- **Commits:** the parent plan counts subtask 5 as one commit. Like `ae-plan/reliable-bus.md` did for parent subtask 4, this plan splits it into one commit per Red/Green cycle, and André reviews each one.
- No code comments in the new code.

## What I found in the codebase

- **`internal/runtime/runtime.go`:** `package runtime` with an empty `func RunAgent() {}`. Nothing imports it, so deleting it is safe.
- **`cmd/agent-server/stub.go`:** `runStub(ctx, workflowID, input, emit)` builds `events.Event` values and calls `emit`. The loop emits the same way.
- **`cmd/agent-server/stub_test.go`:** records events into a slice through an `emit` closure, clears `ID`/`TS`, and compares with `reflect.DeepEqual`. The agent tests reuse that pattern. No testify.
- **`cmd/agent-server/handlers/handlers_test.go`:** the fakes are small structs in the test file (`newFakeRunner`, `fakeBus` with `var _ handlers.EventBus = (*fakeBus)(nil)`). The fake model follows the same style.
- **`internal/events/events.go`:** already has every type and field the loop needs, including `ToolCallID`, `Name`, `Args`, `Result` and `Error`. No change.
- **`handlers.Runner`** is `func(ctx, workflowID, input string, emit func(events.Event))` with no return value. A run reports failure through `workflow.failed`, not a Go error.
- **Course `origin/lesson-2:harness/runtime.ts`:**
  - the conversation starts as `[system, user]`;
  - the counter goes up only on tool-call turns, `while (step < 10)`;
  - the final turn emits `model.completed` then `workflow.completed`;
  - the step limit gives "Hit the 10-step limit without finishing.";
  - a stream error gives `workflow.failed` with the error text.
- **Course `origin/lesson-2:harness/tools.ts`:** tools return plain objects (`{ok, itemId, category}`). So a Go tool returns a value, and the loop marshals it.
- **`docs/openrouter-agent-loop.md` "The loop":** the same shape in SDK types: the assistant message is appended, then one `tool` message per call with `ToolCallID`. Our own types mirror it, so subtask 7's conversion is one-to-one.

## What the docs say

- **`go doc context.Context.Err`:** once `Done` is closed, `Err` returns `context.Canceled` or `DeadlineExceeded`, and keeps returning it. The loop checks `ctx.Err()` before each model call and each tool call, and reports it in `workflow.failed`.
- **`go doc net/http.NewRequestWithContext`:** the request's context controls its whole lifetime, so a cancelled `ctx` aborts an in-flight call. That only works if the model adapter passes `ctx` through, which is why `Model.Next` takes `ctx` (subtask 7 verifies it).
- **`go doc encoding/json.RawMessage`:** a raw encoded JSON value, and `json.Marshal` fails if it isn't valid JSON. The loop checks model-supplied args with `json.Valid` before putting them in an event, and builds `Result` with `json.Marshal`, so both are always valid.

## Approach

One file, `internal/agent/agent.go`:

```go
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
type Deps struct {
    Model        Model
    Tools        []Tool
    SystemPrompt string
    Emit         func(events.Event)
}

func Run(ctx context.Context, workflowID, input string, deps Deps)
```

- **Start:** emit `workflow.started {input}`. The conversation is `[system (only if SystemPrompt is set), user input]`. Parent subtask 6 fills in `SystemPrompt`; the field and its test land here so that subtask doesn't have to change the loop.
- **Each turn:**
  - if `ctx` is done, emit `workflow.failed` with the context error and stop;
  - call `Model.Next(ctx, conversation, deps.Tools)`;
  - on an error, emit `workflow.failed` with it;
  - append the assistant message;
  - with no tool calls, emit `model.completed {text}` and `workflow.completed {output}`, and stop.
- **Each tool call, in order:**
  - if `ctx` is done, stop as above;
  - emit `tool.requested`;
  - find the tool by name;
  - run it;
  - marshal the result;
  - on success, emit `tool.completed`;
  - for an unknown tool, invalid argument JSON, a tool error or a result that won't marshal, emit `tool.failed`;
  - either way, append one `tool` message with the call's `ID`, holding the result JSON or `error: <text>`, so the model sees what went wrong.
- **Step limit:** after 10 tool-call turns, emit `workflow.failed` with "Hit the 10-step limit without finishing."
- **Invalid argument JSON:** `tool.requested` still shows the call. `Args` carries the raw text as a JSON string, so the event still marshals.

**About outside-in:** the entry point is `agent.Run`, and its two dependencies, the model and the tools, are interfaces this plan defines. They stay fakes for every commit here. Parent subtask 6 swaps in the real tools and parent subtask 7 the real model. So the subtasks below are Red/Green cycles that grow one entry point, not mock swaps.

## Red / Green

**Happy path**
- A model that answers straight away gives `workflow.started {input}` → `model.completed {text}` → `workflow.completed {output: text}`.
- A model that calls one tool, then answers, gives `workflow.started` → `tool.requested {toolCallId, name, args}` → `tool.completed {toolCallId, result}` → `model.completed` → `workflow.completed`. The second model call sees `[user, assistant with the call, tool message with the matching ToolCallID and the result JSON]`.
- Several tool calls in one turn each get their own `tool.requested`/`tool.completed` pair and their own tool message, in the model's order.
- The model gets the full tool list (names, descriptions, parameters) on every call.
- With `SystemPrompt` set, the first message is the system prompt. Without it, the first message is the user input.

**Tool failures (the loop keeps going)**
- An unknown tool name gives `tool.failed` with `unknown tool "<name>"`. The tool message says so, and the next model call happens.
- A tool that returns an error gives `tool.failed` with its text, and the same goes back to the model.
- Argument JSON that isn't valid gives `tool.failed` without calling the tool.
- Every event from every test marshals with `json.Marshal`, checked by the shared test helper.

**Ending badly**
- A model that always calls a tool ends in `workflow.failed` "Hit the 10-step limit without finishing." after exactly 10 model calls, with no `model.completed`.
- A model error gives `workflow.failed` with that error's text and no further calls.
- With `ctx` cancelled while the model is thinking, the fake model returns the context error and `Run` emits `workflow.failed` with "context canceled". There's no further model or tool call, and `Run` returns.
- With `ctx` cancelled during the first of two tool calls in one turn, the second tool doesn't run and the run ends in `workflow.failed`.
- Everything passes `go test -race`.

## Most important file changes

| File | Change |
|---|---|
| `internal/agent/agent.go` | new: the types, `Model`, `Deps`, `Run` |
| `internal/agent/agent_test.go` | new: scripted fake model, fake tools, an event recorder, the tests above |
| `internal/runtime/runtime.go` | deleted |

## Open questions

- None. `Run` and `handlers.Runner` is settled: `Agent.Run` is a method with the `Runner` shape, so parent subtask 7 passes `agent.Run` to `handlers.NewMux` directly.

## Subtasks

Each item is one commit. The program builds and `go test ./...` passes after every commit. `agent-server` keeps running the stub throughout.

1. ✅ **Done (`9b862c3`, `ae880fd`, `a89eaf1`, in PR [andrelas6/ai-agent-go#14](https://github.com/andrelas6/ai-agent-go/pull/14), not merged yet).** **`agent.Run` with a model that answers straight away. Delete `internal/runtime/`.**
   - Adds the types, `Model` and `Run`, plus the test helpers: a fake model that records the messages and tools it was given, and an emitter that also checks `json.Marshal` and fails when more events arrive than expected.
   - Test: `workflow.started` → `model.completed` → `workflow.completed`, and the model received `[user input]` and the tool list.
   - `go test ./internal/agent/...` is `ok`, and `internal/runtime` is gone from `go test ./...`.
   - **Built differently from the plan:**
     - An `Agent` struct (`Model`, `MaxSteps`, `SystemPrompt`, `Tools`), built with `NewAgent(systemPrompt, model)`, with `Run(ctx, workflowID, input, emit)` as a method, instead of `Run(ctx, workflowID, input, Deps{...})`. The Approach section above still shows `Deps`.
     - Three commits instead of one.
     - The events are recorded through a channel with a timeout, not a slice.
   - **Landed early:**
     - the model-error path and its test, from subtask 5;
     - the step-limit branch, without a test, from subtask 5;
     - the `SystemPrompt` field and its code, without a test, from subtask 7.
   - **Left over, for later subtasks:**
     - `TestAgentRunBasic` has no explicit check that the model is called exactly once. A second call is only caught indirectly, because it runs out of scripted replies.
     - `messages = append(messages, message)` before the `return` in the final-answer case does nothing.
2. ✅ **Done (`3a6bda5`, `370131a`, in the same PR).** **One tool call, then an answer.**
   - `tool.requested`/`tool.completed`, plus the assistant message and the tool message with the matching `ToolCallID` on the second model call.
   - The fake model got scripted replies: each test passes them in order, and an error or a tool call is just another reply.
   - **Decided along the way:**
     - `Run` keeps the `ModelResponse` type and `getModelResponse` that André added (`616a64e`, with the `return` fix in `abd9297`).
     - A reply with no text and no tool calls is `Unknown` and fails the run. The course would treat it as an empty final answer. André chose to keep failing.
3. ✅ **Done (`bb55e9b`, in the same PR).** **Several tool calls in one turn, in order.**
   - Test only, since the loop already handled it: `TestAgentRunsSeveralToolCallsInOneTurn`. One reply with two calls gives one `tool.requested`/`tool.completed` pair per call, and the next model call sees one `tool` message per call, in the model's order.
   - The two tools return different results, so a mix-up between them fails the test.
4. ✅ **Done (`b6c750b`, in the same PR).** **Tool failures keep the loop going:** an unknown tool, a tool error, invalid argument JSON.
   - Each failure emits `tool.failed` and adds a `tool` message with `error: <text>`. The loop keeps going, including for the other calls in the same turn.
   - Tests: `TestAgentContinuesWhenToolCallErrors`, a table with the exact events per case, and `TestAgentRunsSecondToolCallWhenFirstFails`.
   - Invalid arguments appear in `tool.requested` as a JSON string, so the event always marshals.
   - **Added beyond the plan:**
     - A result that won't marshal also gets `tool.failed`, with its own table case.
     - A call with no arguments gets its own error, `no arguments sent`, and `tool.requested` has no `args`. André's decision. This means every tool needs arguments for now; a tool with no parameters would need this revisited, unless the provider sends `{}`.
   - **Built differently from the plan:** `runTool` returns the tool's own error text, with no `tool "name":` prefix.
   - **Left over:** a call with an unknown name and bad arguments reports `unknown tool`, because that check comes first. No test covers the combination.
5. ✅ **Done (`da93b4b`, in the same PR; the model error landed in subtask 1).** **Ending badly:**
   - a model error;
   - the 10-step limit, with exactly 10 model calls.
   - Test: `TestAgentStepLimit`, a table of three cases. A model that never stops, or a tool that keeps failing, ends in `workflow.failed` after exactly 10 model calls, and an answer on the 10th call completes normally.
   - The message is now the course's: "Hit the 10-step limit without finishing."
   - **Known behaviour, left as is:** on the 10th turn the tool still runs, but the model never sees its result, because the run stops there. The course's loop stops at the same limit.
6. ✅ **Done (`ca9956a`, in the same PR).** **Cancel:**
   - `ctx` cancelled during a model call;
   - `ctx` cancelled between two tool calls.
   - Run with `-race`.
   - `Run` checks `ctx` before each model call and each tool call. A cancelled run ends in `workflow.failed` with `context canceled`. André's decision.
   - Tests: `TestAgentStopsWhenContextIsCancelled`, a table of three cases (before the run starts, during the first of two tools, during the last tool of a turn), and `TestAgentStopsWhenCancelledDuringModelCall`, with a small `blockingModel`.
   - **Changed along the way:** the fake model now always records the call and replies, and ignores the context. Its random `select` could hide a missing check, so a test passed or failed from run to run.
   - **Left over:**
     - A cancel during the 10th turn reports the step-limit message, not `context canceled`, because the loop ends before the next check. The run still ends in `workflow.failed`. No test covers this.
     - Aborting a real HTTP call is still parent subtask 7's job: it must pass `ctx` into `Chat.Send`, with a test against a hanging `httptest` server.
     - On server shutdown, the final event may be dropped, because the bus's `Publish` gives up once the server context is done.
7. ✅ **Done (in the same PR).** **`SystemPrompt`:** when set, it's the first message.
   - Test only, since the code landed in subtask 1: `TestAgentSendsSystemPromptFirst`. With a prompt, the model gets it first; with an empty one, there's no system message.
   - **Size check:** about 230 lines of production code and 930 of tests, against the estimated 150 and 300–350. That's over the 500-line limit. André chose to keep one PR, because the pieces depend on each other and splitting wasn't worth the trouble. The PR description says so.
   - **For the parent plan:** this PR holds only parent subtask 5, so its PR 2 (subtasks 5–8) estimate of 550–650 lines no longer holds. The real tools, the model and the CLI come separately.
