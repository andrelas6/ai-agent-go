# ae-init: agent-cli runs the support agent from the terminal

> **Status:** Reviewed
> **Last updated:** 2026-11-07
> **From:** subtask 8 of ae-plan/lesson-1-harness.md, on `main` at `b2414be`

## What to build

The last piece of lesson 1, and the "one loop, two front ends" point of the big picture. `agent-cli` runs the same support agent as `agent-server`: same system prompt, same four tools, same OpenRouter model and config. The difference is the front end. You pipe a task in on stdin, and each harness event comes out as one readable line in the terminal, without starting a server or opening the inspector. Today's CLI is an older experiment from before the plan, with no tools, no events and a hard-coded model. All of it is replaced.

## A -> B

- **A (now):** `go run ./cmd/agent-cli < sample-task.txt` sends only the first line (`Handle these work items:`) with `testdata/claude_hooks.md` stuck in front of it, to a hard-coded `google/gemma-4-31b-it` with no tools. It prints one `RESPONSE: [...]` JSON dump (a summary of the hooks doc), then exits 0. The three items never reach the model.
- **B (after):** the same command sends the whole of stdin as the task to `agents.NewSupportAgent(model.New(key, model))`. It prints one line per event, `workflow.started` to `workflow.completed`, with the tool calls in between. `OPENROUTER_MODEL` picks the model (default `z-ai/glm-5.3-flash`), and Ctrl-C stops the run.

Checked against the code: `cmd/agent-cli/main.go` reads stdin with `bufio.Scanner`, line by line, and `break`s after the first reply unless its type is `NOT_FINISHED` (a `stop` with tool calls, which can't happen with no tools). I ran it on 2026-11-06 with the sample task: one `RESPONSE:` dump about Claude Code hooks, exit 0.

### Visual

```text
A (now)
  $ go run ./cmd/agent-cli < sample-task.txt
      stdin line 1 only: "Handle these work items:"
      + testdata/claude_hooks.md prepended
      ──▶ google/gemma-4-31b-it, no tools, no system prompt
  RESPONSE: [
   { "finish_reason": "stop", ...
     "message": { "content": "This documentation provides a comprehensive
                  reference for **Claude Code Hooks** ..." } }
  ]                                       (no trailing newline)
  $ echo $?
  0

B (after)
  $ go run ./cmd/agent-cli < testdata/sample-task.txt              ◀ new file
      all 4 lines of stdin = the task                              ◀ changed
      ──▶ support agent: SupportPrompt + 4 tools                   ◀ new
          on OPENROUTER_MODEL (default z-ai/glm-5.3-flash)         ◀ changed
  workflow.started Handle these work items: ...                    ◀ new: one line per event
  tool.requested classifyItem {"itemId":"item-1","category":"billing"}
  tool.completed {"ok":true,"itemId":"item-1","category":"billing"}
  tool.requested searchKnowledgeBase {"query":"..."}
  tool.completed ...
  tool.requested draftReply {"itemId":"item-1",...}
  tool.completed ...
  tool.requested sendReply {"itemId":"item-1",...}
  tool.completed {"sent":true,...}
  ... same for item-2 and item-3 ...
  model.completed I classified, drafted and sent replies for ...
  workflow.completed I classified, drafted and sent replies for ...
  $ echo $?
  0                                        (1 when the run ends in workflow.failed)

  $ OPENROUTER_API_KEY= go run ./cmd/agent-cli < testdata/sample-task.txt
  OPENROUTER_API_KEY is not set ...                                ◀ new: exits before reading stdin
  $ echo $?
  1
```

The exact text of each line is for ae-plan. The idea is the event type first, then its most useful field: the tool name and args, the result, the text or the error.

## How to reproduce

**Preconditions**
- This repo on `main`, `go` on the `PATH`.
- `OPENROUTER_API_KEY` set to a real key. `OPENROUTER_MODEL` unset, so the default is used.
- **A:** the sample task in a file. `testdata/sample-task.txt` doesn't exist yet, so make it from the course's `SAMPLE_TASK` (`git show origin/lesson-2:harness/system-prompt.ts` in `~/workspace/ai/courses/harness-engineering`):

  ```text
  Handle these work items:
  - item-1 (customer_message): "I was charged twice and need help."
  - item-2 (bug_report): "The export button fails on Safari."
  - item-3 (sales_request): "Can you send pricing for 50 seats?"
  ```

- **B:** the same four lines, committed as `testdata/sample-task.txt`.
- Run from the repo root (today's CLI reads `testdata/claude_hooks.md` by a relative path).
- No server needed. The CLI doesn't touch `:8002`.

**Steps**
1. `go run ./cmd/agent-cli < testdata/sample-task.txt` (for A, point at the file you made); then `echo $?`.
2. Run it again and press Ctrl-C while the model is working; then `echo $?`.
3. Run it with `OPENROUTER_API_KEY=` (empty).
4. Run it with `OPENROUTER_MODEL=google/gemma-4-31b-it`.
5. `go test ./cmd/agent-cli/...`.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| What reaches the model (step 1) | line 1 of stdin plus `claude_hooks.md`, no tools, no system prompt | all 4 lines, `SupportPrompt`, the four tools |
| Terminal output (step 1) | one `RESPONSE: [...]` JSON dump of `Choices`, about the hooks doc | one line per event, `workflow.started` … `workflow.completed` |
| Tool calls (step 1) | none | `classifyItem`, `draftReply` and `sendReply` for each of the 3 items, plus `searchKnowledgeBase` where it helps; every one completes |
| Exit code (step 1) | 0 | 0 (1 if the run ends in `workflow.failed`) |
| Ctrl-C mid-run (step 2) | process killed by the signal | the run stops, the HTTP call in flight is cancelled, and a `workflow.failed context canceled` line prints, exit 1 |
| No key (step 3) | the request goes out with no key, and `log.Fatal` prints `{"error":{"code":401,"message":"Missing Authentication header"}}`, exit 1 | an error naming `OPENROUTER_API_KEY` before reading stdin, exit 1 |
| Model choice (step 4) | ignored, always `google/gemma-4-31b-it` | `google/gemma-4-31b-it` is used |
| Tests (step 5) | only `ReadFile`'s two tests | `FormatEvent` cases (one line per event type) and "all of stdin is the task"; no test needs a key or the network |

## Decided with André (2026-11-07)

- **Slug:** `agent-cli`. ae-plan uses the same one.
- **Configuration code: copy it into the CLI's `main`.** Read `OPENROUTER_MODEL`, fall back to `model.DefaultModel`, and fail when `OPENROUTER_API_KEY` is empty, as `cmd/agent-server/main.go` and `runner.go` do. No shared helper in `internal/model`. This keeps the subtask 7 decision that env vars are read in `main`. Move it to a helper once a third caller shows up.
- **Delete `cmd/agent-cli/readfile.go` and `readfile_test.go`.** Their only caller is the old `main` (`ReadFile("testdata/claude_hooks.md")`), which goes. André approved removing the doc comment in `readfile.go` along with the file. `testdata/claude_hooks.md` stays, because `internal/metadata/metadata_test.go` names it.
- **Exit code:** 0 when the run ends in `workflow.completed`, 1 when it ends in `workflow.failed`. Ctrl-C ends in `workflow.failed`, so it exits 1 too. A missing key exits 1 before reading stdin.
- **No `-stub` flag.** `runStub` lives in `cmd/agent-server` (`package main`), and the CLI's tests don't need it: `FormatEvent` is pure, and reading stdin can be tested without a model.

## Open questions

- **Small ones, for ae-plan to decide:**
  - Where the lines go: events to stdout, the missing-key error to stderr?
  - Empty stdin: refuse with "no task on stdin" (non-zero), or send an empty task?
  - `tool.completed` and `tool.failed` events carry only `toolCallId`, not the tool name (`internal/agents/agent.go`). Should the line show the call id, or should the CLI remember the name from `tool.requested`?
  - Events from the CLI have no `id` or `ts`, because the bus stamps those and the CLI has no bus. Fine for printing; `FormatEvent` ignores both.
  - The `workflowId` is a new UUID per run, as in the server.
