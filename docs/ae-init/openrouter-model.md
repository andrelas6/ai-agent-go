# ae-init: The real OpenRouter model in agent-server

> **Status:** Draft
> **Last updated:** 2026-11-05
> **From:** ae-plan/lesson-1-harness.md, subtask 7

## What to build

`agent-server` runs the real support agent on a real OpenRouter model, in place of the course stub. A task submitted in the inspector goes to `z-ai/glm-5.3-flash` (or whatever `OPENROUTER_MODEL` says), and the four Lesson 1 tools run for real. The inspector shows every tool and model event as it happens, and the sample task runs end to end. This is the lesson-1 milestone. The stub stays: `go run ./cmd/agent-server -stub` still runs it, with no account and no network.

Decisions already made:

- **The package is `internal/model`.** It's the real thing behind `agents.Model`.
- **The default model is `z-ai/glm-5.3-flash`**, in place of `google/gemma-4-31b-it`. Why is under "Model choice" below.
- **A `-stub` flag picks the stub.** With `-stub`, the server runs `runStub`. Without it, the server runs the real agent. `runStub` and its test stay.
- **No key and no `-stub`: the server doesn't start.** It exits at once with a message saying `OPENROUTER_API_KEY` is missing, before it listens on `:8002`.
- **`max_tokens: 64000`**, sent as `max_tokens`, not `max_completion_tokens`. GLM 5.3 Flash lists only `max_tokens`.
- **`provider.require_parameters: true`.** OpenRouter uses only providers that support every parameter sent (`tools` and `max_tokens: 64000`). If none does, the call fails with an error instead of quietly dropping the tools.
- **No retries.** The SDK retries every 5XX by default, with backoff for up to an hour. That gets turned off, and a 500 fails the workflow at once.
- **The SDK's 60s per-call timeout stays.** Revisit only if the by-hand run hits it.
- **`ctx` reaches the HTTP call.** Cancelling the server's context stops a call that's in flight.
- **`OPENROUTER_API_KEY` and `OPENROUTER_MODEL` are read in `main`**, not in `internal/model`.
- **The tests use `httptest`, with no network.**

## A -> B

- **A (now):** `cmd/agent-server/main.go` passes `runStub` to `handlers.NewMux`. Every task gives the same three events: `workflow.started`, a `log` warning "No agent yet…", and `workflow.completed` with "(no agent implemented yet)". Nothing calls OpenRouter.
- **B (after):** `go run ./cmd/agent-server` runs `agents.NewSupportAgent(model).Run` on GLM 5.3 Flash. The sample task gives `tool.requested`/`tool.completed` pairs for each item, then `model.completed` and `workflow.completed` with the summary. With `-stub`, it's A.

### Visual

```text
A (now): go run ./cmd/agent-server
  log:     listening on :8002
  inspector ── POST /api/tasks {input: SAMPLE_TASK} ──▶ 202 {workflowId}
  /api/events ◀── workflow.started   {input}
              ◀── log (warn)         "No agent yet. You build the brittle agent loop…"
              ◀── workflow.completed "(no agent implemented yet)"

B (after): OPENROUTER_API_KEY=… go run ./cmd/agent-server
  log:     agent: z-ai/glm-5.3-flash                                     ◀ new
  log:     listening on :8002
  inspector ── POST /api/tasks {input: SAMPLE_TASK} ──▶ 202 {workflowId}
  /api/events ◀── workflow.started   {input}
              ◀── model.delta        (only if the model writes text with its tool calls)  ◀ new
              ◀── tool.requested     classifyItem {"itemId":"item-1",…}   ◀ new
              ◀── tool.completed     {"ok":true,"itemId":"item-1",…}      ◀ new
              ◀──   … searchKnowledgeBase, draftReply, sendReply, for item-1..3   ◀ new
              ◀── model.completed    "<summary of what it did>"           ◀ new
              ◀── workflow.completed "<same summary>"                     ◀ changed

  OpenRouter returns 500         ──▶ workflow.failed, at once, no retry   ◀ new
  no provider supports the request ──▶ workflow.failed with OpenRouter's error   ◀ new
  model hits its token limit     ──▶ workflow.failed "model reply was cut off: length"   ◀ new

B (after), stub: go run ./cmd/agent-server -stub
  log:     running the stub                                              ◀ new
  /api/events ◀── same three events as A

B (after), no key and no -stub: go run ./cmd/agent-server
  exits at once: OPENROUTER_API_KEY is missing (or pass -stub)          ◀ new
  nothing listens on :8002
```

## How to reproduce

**Preconditions**
- The course repo at `~/workspace/ai/courses/harness-engineering`, on the local `lesson-1` branch (has our SSE change), with `npm install` done.
- This repo on `main`.
- **B only:** an OpenRouter key in `OPENROUTER_API_KEY`. `OPENROUTER_MODEL` unset (so the default is used), or set to another model that supports tools and `max_tokens`.
- The sample task, from the course's `lessons/01-intro-to-harness-engineering/index.md`:

  ```text
  Handle these work items:
  - item-1 (customer_message): "I was charged twice and need help."
  - item-2 (bug_report): "The export button fails on Safari."
  - item-3 (sales_request): "Can you send pricing for 50 seats?"
  ```

**Steps**
1. `go test -race ./...` in this repo.
2. In the course repo, run `npm run dev:web`.
3. In this repo, run `go run ./cmd/agent-server`. **B:** run it with `OPENROUTER_API_KEY` set. Then run it again with `-stub`.
4. Open http://localhost:5173 with the DevTools Network tab open. Submit the sample task.
5. While it runs, in a terminal: `curl -N localhost:8002/api/events`.
6. The browser checks that were never run by hand (from subtasks 3 and 4):
   - open a second tab during a run;
   - reload one tab mid-run;
   - click Clear;
   - look at how a `model.delta` event shows in the event pane, if the model sends one.
7. Ctrl-C the server while a task is running.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| `go test ./internal/model/...` | package doesn't exist | passes, with no network: the request has the model, the messages and roles in order, the tools with their schemas, the `tool_call_id` on tool messages, `max_tokens: 64000`, `provider.require_parameters: true`, and `Authorization: Bearer <key>`. A text reply and a tool-call reply come back intact |
| the same, server returns 500 | n/a | an error at once (one request to the server), not after backoff |
| the same, server hangs, `ctx` cancelled | n/a | `context canceled` within a fraction of a second, well under the SDK's 60s client timeout |
| the same, empty `choices` | n/a | an error |
| the same, `finish_reason: "length"` | n/a | comes through on the `Message`, so the loop ends in `workflow.failed` "cut off" |
| `go test ./cmd/agent-server/...` | `runStub` test passes | `runStub` test still passes |
| server's startup log | `listening on :8002` | without `-stub`: which model the agent runs on. With `-stub`: that the stub is running |
| server started with no key, no `-stub` | starts and runs the stub | exits at once with a non-zero status and a message that `OPENROUTER_API_KEY` is missing. Nothing listens on `:8002` |
| event pane after the sample task | the stub's three events | `workflow.started`, then tool events for all three items, then `model.completed` and `workflow.completed`. No `workflow.failed` |
| `curl -N :8002/api/events` | the three stub events | `id:` / `data:` lines arriving one by one as each tool runs, not in one burst at the end |
| second tab, reload mid-run, Clear | not checked by hand yet | both tabs show the same events; the reloaded tab catches up from `Last-Event-ID` with no gaps or duplicates; Clear empties the replay |
| `model.delta` display | never sent by the stub | shows as readable text in the event pane (or note that the model never sent one) |
| a tool call with no arguments | n/a | note what the provider sends (`""`, `"{}"` or nothing). The four tools all need arguments, so it's `tool.failed` either way |
| Ctrl-C mid-run | exits cleanly | exits cleanly, and the in-flight model call stops |
| OpenRouter activity page | nothing | one request per model turn for this key, on a provider that supports tools |

## Model choice

From OpenRouter's public `/api/v1/models` and `/api/v1/models/<id>/endpoints`, checked on 2026-11-05:

| | `z-ai/glm-5.3-flash` (chosen) | `nvidia/nemotron-3.5-lightning` | `google/gemma-4-31b-it` (old default) |
|---|---|---|---|
| Size | not given | 30B, 3B active | 30.7B dense |
| Context | 1M | 262k | 262k |
| Price per M tokens (in / out) | $0.15 / $0.50 | $0.07 / $0.20 | $0.20 / $0.40 |
| Providers | about 40, all support tools | 5. Fireworks has no tools | 18. Two DeepInfra endpoints have no tools |
| Providers that take `max_tokens: 64000` | all | 2 (Io Net, CoreWeave). Darkbloom and DeepInfra cap a reply at 32768 | about half; several cap at 16384 or 8192 |
| `max_completion_tokens` listed | no | — | yes |

GLM 5.3 Flash wins on routing: with `require_parameters` on, every provider still qualifies. The listing says nothing about quality. Only the by-hand run of the sample task shows whether it classifies, drafts and sends all three items.

## Findings from the SDK source (`go-sdk` v0.9.42)

- **Retries:** `chat.go` falls back to `Strategy: "backoff"` with `MaxElapsedTime: 3600000` on `5XX` and on connection errors, unless a retry config is set. `openrouter.WithRetryConfig(retry.Config{Strategy: "none"})` makes `utils.Retry` call the request exactly once.
- **Timeout:** `openrouter.New` uses `&http.Client{Timeout: 60 * time.Second}` unless `WithClient` is passed.
- **`provider.require_parameters` exists:** `ChatRequest.Provider` is an `OptionalNullable[ProviderPreferences]`, and `ProviderPreferences.RequireParameters` is an `OptionalNullable[bool]` (`json:"require_parameters"`).
- **`max_tokens`:** `ChatRequest` has both `MaxTokens` (`max_tokens`) and `MaxCompletionTokens` (`max_completion_tokens`). Today's `agent-cli` sends `MaxCompletionTokens`, which GLM 5.3 Flash doesn't list.
- **Server URL:** `WithServerURL` points the client at an `httptest` server. `Chat.Send(ctx, …)` posts to `<base>/chat/completions` and builds the request with `http.NewRequestWithContext(ctx, …)`, so `ctx` does reach the call.
- **Env vars:** with `WithSecurity` set, the SDK doesn't read `OPENROUTER_API_KEY` by itself. It does read `OPENROUTER_HTTP_REFERER` and `OPENROUTER_X_TITLE` from the environment if they're set.

## Risks

- **Tools dropped quietly.** By default, OpenRouter may route to a provider that doesn't support a parameter, and that provider ignores it. If `tools` were ignored, the model would answer in plain text, and the inspector would show a "successful" run in which nothing was classified or sent. `require_parameters: true` turns that into a visible `workflow.failed`. It's on.
- **Fewer providers with `require_parameters`.** Fine for GLM 5.3 Flash. A model with few providers that take `max_tokens: 64000` (Nemotron 3.5 Lightning has 2) could fail when those are down.
- **A model that answers with nothing.** If the model returns no text and no tool calls, the loop ends in `workflow.failed` "model returned no text and no tool calls". That's correct, but it would show up in the by-hand run, not in the tests.

## Open questions

- None.
