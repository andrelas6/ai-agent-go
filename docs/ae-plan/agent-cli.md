# ae-plan: agent-cli runs the support agent from the terminal

> **Status:** Done, squash-merged in [andrelas6/ai-agent-go#37](https://github.com/andrelas6/ai-agent-go/pull/37) (`5da0c1e`). Review fixes in `ae-plan/pr-37-fixes.md`
> **Last updated:** 2026-11-09
> **From:** ae-init/agent-cli.md, which is subtask 8 of ae-plan/lesson-1-harness.md, on `main` at `b2414be`

## What's being built

`agent-cli` rewritten to run the same support agent as `agent-server`. The task is the whole of stdin, the agent is `agents.NewSupportAgent(model.New(key, modelID))`, and each event prints as one readable line. Same config as the server: `OPENROUTER_API_KEY` is required, and `OPENROUTER_MODEL` falls back to `model.DefaultModel`. Exit 0 when the run completes, 1 when it fails. This closes lesson 1: one loop, two front ends.

## Constraints

- **Dependencies:** the standard library, the OpenRouter SDK and `github.com/google/uuid` (`AGENTS.md`). Nothing new in `go.mod`.
- **Decided in the ae-init, with André:**
  - the config lines are copied into the CLI's `main`, with no shared helper in `internal/model`;
  - `cmd/agent-cli/readfile.go` and `readfile_test.go` are deleted, along with the doc comment in `readfile.go`. `testdata/claude_hooks.md` stays (`internal/metadata/metadata_test.go` names it);
  - exit 0 on `workflow.completed`, 1 on `workflow.failed` (Ctrl-C included), 1 on a missing key;
  - no `-stub` flag.
- **Layout:** everything new is used only by the CLI, so it stays in `cmd/agent-cli` as `package main`, like `newRunner` in the server. Lowercase names (`run`, `readTask`, `formatEvent`), as in `cmd/agent-server`.
- **Tests:** standard `testing`, `package main`, plain `t.Errorf`/`t.Fatalf`, and a short scenario comment above each test (`AGENTS.md`). No test needs a key or the network.
- **No comments** in new code. André's existing comments stay, except the one in `readfile.go`, which goes with the file (approved).

## What I found in the codebase

- **`cmd/agent-cli/main.go`:** the old code. A `bufio.Scanner` reads one line at a time, `ClientModel` wraps the SDK, `getModelResponseType` maps finish reasons, the model is hard-coded, and there are no tools or events. All of it goes. Nothing is reused.
- **`cmd/agent-server/main.go` and `runner.go`:** the config to copy. `OPENROUTER_MODEL` empty → `model.DefaultModel`, and a missing `OPENROUTER_API_KEY` is an error. The server logs `agent: <model>` at start. The CLI does the same on stderr, so the by-hand check can see which model ran.
- **`internal/model/openrouter.go`:** `New(apiKey, modelID string, opts ...openrouter.SDKOption) *OpenRouter`. Because of the variadic `opts` it doesn't fit a `func(string, string) agents.Model` field as is, but nothing in this plan needs that. `main` calls it directly.
- **`internal/agents/agent.go`:** `Run(ctx, workflowID, input, emit)` calls `emit` synchronously, in the caller's goroutine. So `run` can write each line straight to stdout and track the last event type with no locking. A cancelled `ctx` ends in `workflow.failed` with `context canceled`. `tool.completed` and `tool.failed` carry `ToolCallID` but no `Name`.
- **`cmd/agent-server/handlers/handlers.go`:** the server refuses an empty `input` (400), passes the input on unchanged, and makes the id with `uuid.NewString()`. The CLI does the same: it refuses blank stdin and gives each run a new UUID.
- **`cmd/agent-server/runner_test.go`:** the test pattern. Collect events through `emit` and compare them. The CLI tests need their own small scripted fake `agents.Model`, because the fakes in `internal/agents/agent_test.go` are in the `agents_test` package and can't be imported.
- **Events have no `id` or `ts` in the CLI.** The bus stamps those, and the CLI has no bus. `formatEvent` ignores both.

## What the docs say

- **Go, [`os/signal.NotifyContext`](https://pkg.go.dev/os/signal#NotifyContext):** once it's installed, `os.Interrupt` cancels the context instead of exiting the program, until `stop` is called. **This sets the order in `main`:** read stdin first, then `NotifyContext`. If the context came first, Ctrl-C at an interactive prompt (no pipe) would cancel a context nobody is watching, and `io.ReadAll` would keep blocking until Ctrl-D. Reading first keeps the default: Ctrl-C while typing just exits.
- **Go, [`io.ReadAll`](https://pkg.go.dev/io#ReadAll):** reads to EOF and returns `err == nil` at EOF. That's "the whole of stdin is the task".
- **Go, [`encoding/json.Compact`](https://pkg.go.dev/encoding/json#Compact):** removes insignificant whitespace, so pretty-printed `args` or `result` stays on one line. It returns an error on invalid JSON. Then we print the bytes as they are, with whitespace collapsed.
- **Go, [`strings.Fields`](https://pkg.go.dev/strings#Fields):** splits on any run of whitespace. `strings.Join(strings.Fields(s), " ")` turns a multi-line task or summary into one readable line.
- **`go run` and exit codes:** a non-zero exit under `go run` also prints `exit status 1` on stderr. Only the by-hand check sees this. `go build` and running the binary don't print it.

## Approach

Three small functions in `cmd/agent-cli`, and a short `main`:

- **`readTask(r io.Reader) (string, error)`:** `io.ReadAll`, then `strings.TrimSpace`. Blank input gives the error `no task on stdin`.
- **`run(ctx, task string, m agents.Model, out io.Writer) int`:** builds `agents.NewSupportAgent(m)` and calls `Run` with `uuid.NewString()` and an `emit` that writes `formatEvent(e)` plus a newline to `out` and remembers whether the run failed. It returns 1 if a `workflow.failed` was seen, else 0.
- **`formatEvent(e events.Event) string`:** one line, the event type first:

  | Event | Line |
  |---|---|
  | `workflow.started` | `workflow.started <input>` |
  | `workflow.completed` | `workflow.completed <output>` |
  | `workflow.failed` | `workflow.failed <error>` |
  | `model.delta`, `model.completed` | `<type> <text>` |
  | `tool.requested` | `tool.requested <name> <args>` |
  | `tool.completed` | `tool.completed <toolCallId> <result>` |
  | `tool.failed` | `tool.failed <toolCallId> <error>` |
  | `log` | `log <level> <message>` |
  | anything else | `<type>` |

  Text fields go through `strings.Fields`/`Join`, so newlines never split a line. `args` and `result` go through `json.Compact`, falling back to the collapsed raw text. `tool.completed`/`tool.failed` show the call id rather than the tool name. That keeps `formatEvent` pure and stateless. The id sits next to its `tool.requested` line anyway, because tools run one at a time.
- **`main`:**
  1. read `OPENROUTER_API_KEY`. If it's empty, print `OPENROUTER_API_KEY is not set` to stderr and exit 1, before reading stdin;
  2. read `OPENROUTER_MODEL`, falling back to `model.DefaultModel` (copied from the server);
  3. `readTask(os.Stdin)`. On error, print it to stderr and exit 1;
  4. `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)`;
  5. print `agent: <model>` to stderr;
  6. `code := run(ctx, task, model.New(key, modelID), os.Stdout)`, then `stop()` and `os.Exit(code)`.

  `stop` is called before `os.Exit`, because a deferred call wouldn't run.

Events go to stdout, and messages about the CLI itself (the model line, a missing key, an empty task) go to stderr. So `agent-cli < task > events.log` captures only the events.

## Red / Green

**`run` (fake model, real support agent and real tools)**
- A fake that asks for `classifyItem` with valid args, then replies with text: one line per event in order (`workflow.started`, `tool.requested`, `tool.completed`, `model.completed`, `workflow.completed`), each starting with its type, and exit code 0.
- The task reaches the model unchanged, as the user message after `SupportPrompt`.
- The fake returns an error: the last line is `workflow.failed …`, and exit code 1.
- A cancelled `ctx`: a `workflow.failed context canceled` line, and exit code 1.

**`readTask`**
- The four-line sample task comes back whole, all four lines, without the trailing newline.
- Empty stdin, and stdin that's only whitespace and newlines: the error `no task on stdin`.

**`formatEvent`** (table test)
- One case per row of the table above, with the exact line.
- A multi-line input, output or text comes out on one line.
- Pretty-printed `args`/`result` come out compacted. Invalid JSON `args` come out as the raw text.
- No result ever contains `\n`.

**By hand** (the ae-init's "How to reproduce", not automated)
- The sample task: tool lines for all three items, then `workflow.completed`, exit 0.
- Ctrl-C mid-run: `workflow.failed context canceled`, exit 1. Ctrl-C at an interactive prompt before Ctrl-D: exits at once.
- `OPENROUTER_API_KEY=`: the message on stderr, exit 1, with no wait for stdin.
- `OPENROUTER_MODEL=google/gemma-4-31b-it`: stderr says `agent: google/gemma-4-31b-it`.

## Most important file changes

| File | Change |
|---|---|
| `cmd/agent-cli/main.go` | rewritten: config, `readTask`, `NotifyContext`, `run`, exit code. `ClientModel`, `ModelResponseType` and `getModelResponseType` go |
| `cmd/agent-cli/run.go`, `run_test.go` | new: `readTask`, `run`, the scripted fake model |
| `cmd/agent-cli/format.go`, `format_test.go` | new: `formatEvent` |
| `cmd/agent-cli/readfile.go`, `readfile_test.go` | deleted |
| `testdata/sample-task.txt` | new: the course's `SAMPLE_TASK`, four lines |
| `docs/ae-plan/lesson-1-harness.md` | subtask 8 ✅ and a "How subtask 8 landed" note |

`internal/` doesn't change.

## Risks

- **Ctrl-C ordering in `main`** (see "What the docs say"). It isn't covered by a test, because `main` isn't. The by-hand check covers it.
- **`main` isn't tested.** The key check, the model default and the stdin/signal order live there. That's the price of copying the config instead of sharing a helper. Each one has a by-hand step.

## Open questions

- None. The small ones the ae-init left for this plan are settled above: events go to stdout and messages to stderr, blank stdin is refused, and tool results show the call id.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit.

1. ✅ **The CLI runs the support agent.** (`e1e1b6f`) Rewrite `main` as in the Approach, with `readTask` and `run` in `run.go`. The formatter is a stand-in: `formatEvent` returns just `string(e.Type)`. Delete the old code, `readfile.go` and `readfile_test.go`. Add `testdata/sample-task.txt`.
   - Tests: the `run` and `readTask` cases above. The `run` tests check that each line *starts with* its type, so they keep passing once the real formatter lands.
   - **When this lands:** `go run ./cmd/agent-cli < testdata/sample-task.txt` runs the real agent and prints one bare event type per line, `workflow.started` … `workflow.completed`, with exit 0. A missing key and blank stdin exit 1 with their messages, and Ctrl-C stops the run.
2. ✅ **Readable lines.** (`6a39df7`) The real `formatEvent`, replacing the stand-in.
   - Tests: the `formatEvent` table above.
   - **When this lands:** B from the ae-init exactly: `tool.requested classifyItem {"itemId":"item-1",...}`-style lines.
3. ✅ **By-hand check and docs.** Done on 2026-11-09; the results are in `lesson-1-harness.md` under "How subtask 8 landed". **Actual size:** 366 lines added and 195 deleted against `main`, so one PR. Run the ae-init's "How to reproduce", plus the "By hand" list above. Update `lesson-1-harness.md`: subtask 8 ✅, and a "How subtask 8 landed" note covering what differs from this plan and what the run showed. Then add up the diff.
   - **Size estimate:** `run.go` about 40 lines, `format.go` 50, `main.go` 35, `run_test.go` 150, `format_test.go` 130, plus 4 for the sample task. That's about 410 added lines. About 210 old lines are deleted (`main.go`'s old body, `readfile.go`, `readfile_test.go`). One PR fits under 500 added lines. If it grows past that, split after subtask 1: PR 1 is the runnable CLI with bare event types, PR 2 is readable lines plus docs.
