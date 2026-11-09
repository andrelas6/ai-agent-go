# ae-plan: PR #37 review fixes for agent-cli

> **Status:** Done on branch `agent-cli-review-fixes` (`2e04801`, `6ab81da`, `8f1a3af`, plus the docs commit)
> **Last updated:** 2026-11-09
> **From:** ae-review/pr-37.md, the review of andrelas6/ai-agent-go#37 (subtask 8 of ae-plan/lesson-1-harness.md)

## What's being built

The follow-ups from the PR #37 review, in one new PR on top of `main` once #37 is merged:
- the CLI exits 0 only when the run really completed;
- `tool.requested` lines carry the call id;
- the test fake and the `run` name get cleaned up;
- the docs record the decisions the review asked for.

## Constraints

- Standard library only, plus the OpenRouter SDK and `github.com/google/uuid` (`AGENTS.md`).
- Every test gets a short scenario comment above it (`AGENTS.md`).
- No comments in new code. André's comments stay.
- Everything stays in `cmd/agent-cli` (`package main`). `internal/` doesn't change.
- Red/Green: subtasks 1 and 2 start with a failing test. Subtask 3 is a refactor, so the tests stay green throughout. Subtask 4 is docs only.
- `AGENTS.md` has André's uncommitted "Implementation" section (use the ae-implementer and ae-reviewer subagents). It ships in this PR, with subtask 4.

## What I found in the codebase

- **`cmd/agent-cli/run.go`:**
  - `run(ctx, task, m agents.Model, out) int` builds `agents.NewSupportAgent(m)` itself, so a test can't make the run end with no final event. The real agent always emits one.
  - Exit code: `failed` is set on `workflow.failed`, and the default is 0.
- **`cmd/agent-server/handlers/handlers.go:15`:** `type Runner func(ctx, workflowID, input string, emit func(events.Event))`. That's the shape of `Agent.Run`. The CLI can take a function of that shape too, and `main` passes `agents.NewSupportAgent(model.New(…)).Run`.
- **`cmd/agent-cli/format.go:22`:** `tool.requested` prints `<name> <args>` and no call id.
- **`cmd/agent-cli/run_test.go:13-29`:** `scriptedModel` has parallel `replies` and `errs` slices. `internal/agents/agent_test.go:46` uses one `fakeReply{message, err}`.
- **`internal/agents/agent.go`:** `Run` calls `emit` in the caller's goroutine. The CLI's lock-free `failed` flag depends on that, and nothing in the docs says so yet.

## What the docs say

- No new APIs. Go's function types: a method value like `agent.Run` can be passed wherever a matching `func` type is expected, as the server already does with `handlers.NewMux(ctx, bus, runner)`.

## Approach

- **Fail closed:** `run` takes a runner, `func(ctx context.Context, workflowID, input string, emit func(events.Event))`, instead of a model. It returns 0 only when it saw `workflow.completed` and no `workflow.failed`, else 1. `main` passes `agents.NewSupportAgent(model.New(apiKey, modelID)).Run`. The existing tests pass `agents.NewSupportAgent(fake).Run`.
- **Call id on requests:** `tool.requested <toolCallId> <name> <args>`, so it pairs with `tool.completed <toolCallId> <result>`.
- **Clean-up:** `scriptedModel` gets one `[]scriptedReply{message, err}`, and `run` is renamed to `runTask`.
- **Docs:** the lesson-1 plan's "How subtask 8 landed" gets the squash-merge commit of #37 (the branch hashes `e1e1b6f`/`6a39df7` don't exist on `main` after a squash) and the review's decisions:
  - `emit` is synchronous, and the CLI depends on that;
  - no panic guard in the CLI: a panicking tool prints a stack trace and exits 2, which is visible enough for a terminal tool;
  - the old `main.go` comments and `readfile.go`'s doc comment were removed on purpose, as agreed in the ae-init;
  - this PR, linked.

## Red / Green

- A runner that emits only `workflow.started` and returns: exit 1. *(Red: today it exits 0.)*
- A runner that emits `workflow.started` then `workflow.completed`: exit 0.
- The existing `run` tests still pass, through `agents.NewSupportAgent(fake).Run`.
- `formatEvent` for `tool.requested` with `ToolCallID: "call-1"`: `tool.requested call-1 classifyItem {"itemId":"item-1","category":"billing"}`. *(Red: today the id is missing.)*
- The newline guard covers `tool.requested` with a multi-line call id.

## Most important file changes

| File | Change |
|---|---|
| `cmd/agent-cli/run.go` | `runTask` takes a runner, and fails closed |
| `cmd/agent-cli/run_test.go` | runner-based exit-code tests; one-slice `scriptedModel` |
| `cmd/agent-cli/main.go` | passes `agents.NewSupportAgent(...).Run` to `runTask` |
| `cmd/agent-cli/format.go`, `format_test.go` | call id on `tool.requested` |
| `docs/ae-plan/lesson-1-harness.md` | squash commit, the review decisions, this PR |
| `AGENTS.md` | André's "Implementation" section |

## Open questions

- None.

## Subtasks

Each item is one commit. The program builds and runs after every commit.

1. ✅ **Exit 0 only after `workflow.completed`.** (`2e04801`) `run` takes a runner instead of a model. `main` passes the support agent's `Run`. The existing tests move to `agents.NewSupportAgent(fake).Run`.
   - Tests: the two runner cases above. The no-final-event case is the Red.
   - **When this lands:** the sample task still exits 0, and a run with no final event exits 1.
2. ✅ **Call id on `tool.requested` lines.** (`6ab81da`)
   - Tests: update the `tool.requested` case in the table and in the compact-JSON test. Red first.
   - **When this lands:** `tool.requested <id> classifyItem {…}` and `tool.completed <id> {…}` share an id.
3. ✅ **Clean-up: one-slice `scriptedModel`, `run` → `runTask`.** (`8f1a3af`; the model-error test now also checks the error text) No behaviour change. The tests stay green before and after.
4. ✅ **Docs and `AGENTS.md`.** The lesson-1 plan update from the Approach, plus André's `AGENTS.md` section. Then add up the diff. Expect well under 500 lines.
