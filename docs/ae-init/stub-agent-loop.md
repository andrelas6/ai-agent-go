# ae-init: The agent loop, under test

> **Status:** Reviewed
> **Last updated:** 2026-10-27
> **From:** ae-plan/lesson-1-harness.md, subtask 5, on `main` at `78efcad`

## What to build

The brittle agent loop, the course's `runAgent`, as one shared loop that both the server and the CLI will run later. It receives a task and talks to a model. It runs whatever tools the model asks for, sends each result back, and stops when the model answers with no tool calls. It gives up after 10 turns, or when the model fails. Each step is reported as an event, in the course's event shape. In this subtask the model and the tools are scripted fakes, so the loop is proven by tests alone. Nothing a user runs changes yet: the server still runs the course's stub agent. The empty placeholder package left from before the plan is deleted.

## A -> B

- **A (now):** there is no agent loop. `go test ./internal/agent/...` fails with "no such file or directory". The only placeholder is the empty `internal/runtime`. A task sent to the server runs the stub, which emits `workflow.started` → `log` (warn, "No agent yet…") → `workflow.completed`.
- **B (after):** `go test ./internal/agent/...` passes. Scripted runs emit the event sequences from the plan: one tool then an answer, several calls in one turn, an unknown or failing tool, the 10-step limit, a model error, and a cancelled context. `internal/runtime` is gone. The server still runs the stub, unchanged.

Grounded in the code on `main` (`78efcad`):
- `internal/agent/` does not exist. `go test ./internal/agent/...` gives `lstat ./internal/agent/: no such file or directory … [setup failed]`.
- `internal/runtime/runtime.go` is `package runtime` with an empty `func RunAgent() {}`, and nothing imports it. `go test ./...` lists it as `? internal/runtime [no test files]`. Deleting it breaks nothing.
- `cmd/agent-server/main.go:19` passes `runStub` to `handlers.NewMux`. `runStub` (`cmd/agent-server/stub.go`) emits exactly the three events above.
- `handlers.Runner` is `func(ctx, workflowID, input string, emit func(events.Event))`. The plan's `agent.Run(ctx, workflowID, input, Deps{Model, Tools, Emit})` has a different shape, so wiring it in waits for subtask 7.
- `internal/events` already defines every type the loop needs: `workflow.started/completed/failed`, `model.completed`, `tool.requested/completed/failed`, `log`.
- `internal/tools` has only `ReadFile`, which the old CLI uses. `cmd/agent-cli/main.go` still runs its own loop against OpenRouter. Subtask 5 changes neither.

### Visual

```text
A (now)
  $ go test ./internal/agent/...
    FAIL ./internal/agent/... [setup failed]   (no such directory)
  $ go test ./...
    ?    internal/runtime  [no test files]
  POST /api/tasks ──▶ server ──▶ workflow.started, log "No agent yet…", workflow.completed

B (after)
  $ go test ./internal/agent/...
    ok   internal/agent                                              ◀ new
  $ go test ./...
    (no internal/runtime line)                                       ◀ changed
  scripted run, one tool then an answer:                             ◀ new
    workflow.started → tool.requested → tool.completed
                     → model.completed → workflow.completed
  POST /api/tasks ──▶ server ──▶ workflow.started, log "No agent yet…", workflow.completed
                                                     (same as A: still the stub)
```

## How to reproduce

**Preconditions**
- `ai-agent-go` checkout, Go toolchain, run from the repo root.
- No `OPENROUTER_API_KEY` needed: no test calls the network.
- For the server check: `go run ./cmd/agent-server` (listens on `:8002`).

**Steps**
1. `go test -race -v ./internal/agent/...`
2. `go test ./...`
3. `ls internal/`
4. In one shell: `curl -N localhost:8002/api/events`. In another: `curl -X POST localhost:8002/api/tasks -d '{"input":"hi"}'`.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| Step 1, the agent package | `[setup failed]`, no such directory | `ok`, race-clean |
| Step 2, the package list | includes `? internal/runtime [no test files]` | no `internal/runtime` line; everything else still `ok` |
| Step 3 | `runtime` listed, no `agent` | `agent` listed, no `runtime` |
| Test: one tool, then an answer | n/a | `workflow.started → tool.requested → tool.completed → model.completed → workflow.completed`; the second model call sees the assistant message, then a `tool` message with the matching tool call id |
| Test: several tool calls in one turn | n/a | one `tool.requested`/`tool.completed` pair per call, and one tool message per call, in the model's order |
| Test: unknown tool, or a tool that errors | n/a | `tool.failed` with the error text, which goes back to the model; the loop keeps going |
| Test: model never stops calling tools | n/a | `workflow.failed` with "Hit the 10-step limit without finishing." after 10 turns, and no `model.completed` |
| Test: model returns an error | n/a | `workflow.failed` with the error |
| Test: `ctx` cancelled mid-run | n/a | the fake model sees the cancelled `ctx` (standing in for an aborted HTTP call); no further model or tool calls; `Run` returns; ends in `workflow.failed` with the context error |
| Step 4, the server (regression) | `workflow.started`, `log` (warn, "No agent yet…"), `workflow.completed` | identical: the server still runs the stub |

## Decisions

- **`model.completed` is sent once, for the final answer.** Checked in the course repo, `origin/lesson-2:harness/runtime.ts`. `model.completed {text}` is emitted only when the model's turn has no tool calls, and it's followed by `workflow.completed {output: text}`. Turns with tool calls emit only `tool.requested`/`tool.completed`. The step-limit error text there is "Hit the 10-step limit without finishing." Lesson 2 later moves `model.completed` into a per-turn durable step. That's out of scope here.
- **The `Model` interface uses our own types.** `internal/agent` defines its own small message and tool-call types and does not import the OpenRouter SDK. Subtask 7's adapter converts between the two.
- **Context cancel is handled.** When `ctx` is cancelled, as on server shutdown, the in-flight model call is aborted and the loop starts no further model or tool calls.

## Open questions

- **Run vs `handlers.Runner`.** `agent.Run` takes `Deps{Model, Tools, Emit}`, while the server wants `func(ctx, id, input, emit)`. This gets decided when `agent.Run` is implemented. Nothing to do in this doc.
- **The last event after a cancel.** The B row above guesses `workflow.failed` with the context error. ae-plan decides.
