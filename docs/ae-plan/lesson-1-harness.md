# ae-plan: Lesson 1 harness in Go (SSE inspector + agent loop)

> **Status:** Reviewed
> **Last updated:** 2026-10-24
> **From:** ae-init/lesson-1-harness.md

## What's being built

The course's Lesson 1, served by `ai-agent-go`. The course inspector streams harness events from the Go server over SSE, through a Vite proxy, and sends tasks and clear as POSTs. A brittle agent loop, `agent.Run` (the course's `runAgent`), does the sample three-item task with the four Lesson 1 tools and a configurable OpenRouter model. The same loop runs from `agent-server` and from `agent-cli`.

## Constraints

- **Dependencies:** the Go standard library, plus two allowed packages:
  - the OpenRouter SDK;
  - `github.com/google/uuid`, a new exception. Add it to `AGENTS.md` in the same commit.
  
  Still no testify and no `invopop/jsonschema`. Tool schemas are written by hand as `map[string]any`.
- **Package name:** the loop lives in `internal/agent`, not `internal/runtime`, which would shadow the standard library's `runtime`.
- **Layout follows usage:** a package used by both `agent-server` and `agent-cli` goes in `internal/` (`events`, `agent`, `tools`, `model`). Code only one command uses stays in that command's folder as `package main` (the HTTP handlers live in `cmd/agent-server/handlers`, a sub-package of the server's own folder). `bus` is the one exception: it stays in `internal/` for now, to keep it flexible.
- **No streaming:** non-streaming `Chat.Send`, so no `model.delta`. The model id comes from `OPENROUTER_MODEL`, defaulting to `google/gemma-4-31b-it`.
- **Inspector:** edited in the course repo on a local branch off `lesson-1`, not `main`. Only these change: `useHarnessSocket.ts`, two URLs in `TaskPane.tsx`, and `vite.config.ts`. **No TDD for any change in the course repo.** The web app isn't what the course is about, so it gets no tests and no Red/Green cycles. Red/Green applies only to the Go code in `ai-agent-go`. We'll know it works by following the ae-init steps (two tabs, a reload mid-run, Clear, and `curl -N` showing events arrive as they're emitted).
- **Contract:** event JSON must match `shared/events.ts` exactly: camelCase fields, `id` and `workflowId` as UUID strings, `ts` in milliseconds, `args`/`result` as raw JSON.
- **Out of scope:** no CORS (the proxy replaces it) and no `/api/approve` route (Lesson 7).
- **`agent-server` stays on `:8002`.**

## What I found in the codebase

- **`cmd/agent-server/main.go`:** one unbuffered channel in `internal/messages.Service`, so two clients take events from each other and a POST blocks with no listener. `GET /sse` writes invalid JSON (`data: {"token"}:%q`) and flushes on a 3s `time.Tick` that leaks a timer per loop. The JSON decode error doesn't `return`. `/sse`, `/messages` and `messages.Service` all get replaced.
- **`cmd/agent-cli/main.go`:**
  - reads one line per message, but the sample task is 4 lines;
  - prepends `testdata/claude_hooks.md`;
  - sends no tools;
  - quits after the first normal answer, because it breaks on anything except `NOT_FINISHED`.
  
  The whole body is replaced.
- **`internal/runtime/runtime.go`:** `func RunAgent() {}`. It moves to `internal/agent`.
- **`internal/tools/readfile.go`:** stays as it is. The Lesson 1 tools go in a new file beside it.
- **Tests:** standard `testing` with `t.TempDir()` and plain `t.Errorf` (`readfile_test.go`, `metadata_test.go`). `messages/service_test.go` is empty and gets deleted along with the package.
- **`docs/openrouter-agent-loop.md`:** already has the verified SDK v0.9.42 types for tools, assistant messages with `ToolCalls`, and one `tool` message per call. I checked them against the module cache. `openrouter.WithServerURL` exists, so the model adapter can be tested against `httptest.NewServer` with no network.
- **Uncommitted work on `main`:** `cmd/agent-cli/`, `internal/runtime/`, `internal/tools/`, `testdata/`, `docs/openrouter-agent-loop.md` and `go.sum` are untracked, and `go.mod` is modified. Commit or stash them before subtask 1.
- **Course repo:**
  - `web/src/useHarnessSocket.ts` returns `{events, connected, send}`, and `App.tsx` is its only consumer.
  - `TaskPane.tsx:217` and `:288` hard-code `:8787`.
  - `web/` is identical on every branch.
  - The `lesson-1` branch uses Vite `^6`.

## What the docs say

- **MDN, [Using server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events):**
  - Each message ends with a blank line.
  - The `id:` field sets the browser's last event ID, and `EventSource` reconnects automatically after a dropped connection, sending it back as `Last-Event-ID`.
  - Lines starting with `:` are comments, useful for keep-alives.
  - Multiple `data:` lines are joined, so one-line JSON keeps framing simple.
  - The HTTP/1.1 limit is 6 SSE connections per domain, which is enough for two tabs.
- **Vite, [server.proxy](https://vite.dev/config/server-options):** the key is a path prefix, `{ target, changeOrigin }`. The docs say nothing about SSE through the proxy, so the manual `curl -N` and browser check confirms events aren't buffered.
- **Go, [`net/http#ResponseController`](https://pkg.go.dev/net/http#ResponseController):** use `Flush()` after every event. Method-and-path patterns (`"GET /api/events"`) work on the standard mux.

## Approach

Four small Go packages in `internal/`, plus the HTTP handlers in `cmd/agent-server`, each behind a plain interface the caller defines.

- **`internal/events`:** one flat `Event` struct whose JSON matches the course.
- **`internal/bus`:** an in-memory broadcaster.
  - `Publish` stamps a UUID `id` and `ts`, keeps the last 1000 events, and sends to each subscriber's buffered channel (256).
  - **A subscriber whose buffer is full is cut off.** Its channel closes and its SSE response ends, so `Publish` never blocks the agent. The browser reconnects by itself with `Last-Event-ID` and gets the gap back from history.
  - `Subscribe(lastID)` returns the history after the event with that id plus a live channel. If the id is empty or unknown (after a Clear, or dropped past the cap), it returns the whole history.
- **`cmd/agent-server/handlers` (a sub-package of the server's folder, since only agent-server uses it):**
  - `GET /api/events` (SSE: `id: <uuid>` plus one `data: {json}` line per event, reads `Last-Event-ID`);
  - `POST /api/tasks` (`{input, mode}` → `202 {workflowId}`, starts the runner in a goroutine with the **server's** context, not the request's);
  - `POST /api/clear`.
- **`internal/agent.Run(ctx, workflowID, input, Deps{Model, Tools, Emit})`:** the course's loop from the `lesson-2` branch, step cap 10. Tool calls run through a name→func registry, and an unknown tool or tool error gives `tool.failed` with the error text sent back to the model.
- **`internal/model`:** wraps `Chat.Send` and reads `OPENROUTER_MODEL`.

`/api` is the single proxy prefix, so the inspector uses `/api/events`, `/api/tasks` and `/api/clear`.

## Red / Green

**Contract and bus**
- An `Event` marshals to the course's shape: `workflowId`, `toolCallId`, `ts` as a number, `args` passed through as raw JSON, and `id` a UUID.
- Two subscribers both get every event.
- `Publish` with no subscribers doesn't block.
- A subscriber that stops reading has its channel closed once its buffer fills. `Publish` keeps going, and other subscribers still get every event.
- `Subscribe(lastID)` returns only the events after `lastID`. An empty or unknown id returns the full history.
- History is capped at 1000.
- `Clear` empties history.
- Everything passes `go test -race`.

**HTTP**
- `GET /api/events` sends `Content-Type: text/event-stream` and writes `id: <uuid>` plus one `data: {json}` line per event.
- A history replay comes before live events.
- A `Last-Event-ID` header skips the events already seen.
- The response ends when the bus cuts the subscriber off.
- `POST /api/tasks` returns 202 with a UUID `workflowId` and calls the runner with the input. The runner keeps going after the request ends. Bad JSON or an empty input gives 400.
- `POST /api/clear` empties the replay.

**Agent loop**
- A model that calls one tool and then answers produces `workflow.started` → `tool.requested` → `tool.completed` → `model.completed` → `workflow.completed`. The second model call sees the assistant message, then a `tool` message with the matching `ToolCallID`.
- Several tool calls in one turn each get their own tool message, in order.
- An unknown tool or a tool error gives `tool.failed`, and the loop continues.
- A model that never stops ends in `workflow.failed` ("step limit") after 10 turns.
- A model error gives `workflow.failed`.

**Tools**
- `searchKnowledgeBase("pricing for 50 seats")` returns the pricing article. With no match it returns the fallback text.
- `classifyItem` echoes `{ok, itemId, category}`. `draftReply` returns `draft-<itemId>`. `sendReply` returns `{sent:true,...}`.
- Bad argument JSON gives an error.
- Every tool's schema lists its required fields.

**Model adapter**
- Against an `httptest` server, the request carries the model from `OPENROUTER_MODEL` (default `google/gemma-4-31b-it`) and the tool list, and the reply's tool calls come back intact.

**CLI**
- `FormatEvent` prints `tool.requested classifyItem {"itemId":"item-1",...}`-style lines.
- The whole of stdin up to EOF becomes the task.

## Most important file changes

**`ai-agent-go`**

| File | Change |
|---|---|
| `AGENTS.md` | allow `github.com/google/uuid` |
| `go.mod`, `go.sum` | add `github.com/google/uuid` |
| `internal/events/events.go`, `events_test.go` | new |
| `internal/bus/bus.go`, `bus_test.go` | new |
| `cmd/agent-server/handlers/handlers.go`, `handlers_test.go` | new: `NewMux(ctx, bus, run)` with `/api/events`, `/api/tasks`, `/api/clear` |
| `cmd/agent-server/stub.go`, `stub_test.go` | new: the course's stub agent, `runStub` |
| `internal/agent/agent.go`, `agent_test.go` | new; replaces `internal/runtime/` |
| `internal/tools/support.go`, `support_test.go` | new: the four Lesson 1 tools and the system prompt |
| `internal/model/openrouter.go`, `openrouter_test.go` | new |
| `cmd/agent-server/main.go` | rewritten |
| `cmd/agent-cli/main.go`, `format.go`, `format_test.go` | rewritten / new |
| `internal/messages/`, `internal/runtime/` | deleted |
| `testdata/sample-task.txt` | new |

**`harness-engineering`** (branch off `lesson-1`)

| File | Change |
|---|---|
| `web/src/useHarnessSocket.ts` | uses `EventSource('/api/events')` and `fetch('/api/tasks')`, keeping the same return shape |
| `web/src/components/TaskPane.tsx` | relative `/api/clear` and `/api/approve/...` URLs |
| `vite.config.ts` | `server.proxy['/api']` → `http://localhost:8002` |

## Open questions

- None.

## Who does what

- **Claude:**
  - every change in the course repo (subtask 1);
  - the dummy POST endpoints in `ai-agent-go` (subtask 2).
- **André:** the rest. You'll decide as you go whether to hand any later subtask over.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit.

### Status, 2026-10-27

| # | Subtask | Status | Where |
|---|---|---|---|
| 0 | Housekeeping | ✅ Done | branches `lesson-1` (course repo) and `lesson-1-harness` |
| 1 | Inspector on SSE | ✅ Done | course repo `lesson-1`, `98138c1` |
| 2 | Dummy POST endpoints | ✅ Done | `40587a2`, later folded into subtask 3 |
| 3 | Real endpoints with the course stub agent | ✅ Done | [andrelas6/ai-agent-go#7](https://github.com/andrelas6/ai-agent-go/pull/7), merged. Planned in detail in `ae-plan/sse-events-and-bus.md`; reviewed in `docs/ae-review/pr-7.md` |
| 4 | A bus you can rely on | ✅ Done | [#9](https://github.com/andrelas6/ai-agent-go/pull/9), [#10](https://github.com/andrelas6/ai-agent-go/pull/10), [#11](https://github.com/andrelas6/ai-agent-go/pull/11), [#12](https://github.com/andrelas6/ai-agent-go/pull/12), [#13](https://github.com/andrelas6/ai-agent-go/pull/13), merged (`78efcad`). Planned in detail in `ae-plan/reliable-bus.md` |
| — | PR #7 review fixes | ✅ Done | [andrelas6/ai-agent-go#8](https://github.com/andrelas6/ai-agent-go/pull/8), `e8a95f3` |
| 5–8 | Agent loop, tools, model, CLI | ⏳ Not started | |

**How subtask 3 landed, where it differs from this plan:**
- The handlers live in a sub-package, `cmd/agent-server/handlers` (`NewMux(ctx, bus, run)`), not in `package main`. It's still under the server's folder and used only by the server, so it follows the layout rule.
- `internal/bus` has **one owner goroutine** that holds the subscribers and the history. There's no mutex. It stamps each event with a UUID `id` and `ts`, and `go test -race ./...` passes.
- **When a tab's buffer (`SubscriberBuffer = 256`) is full, the bus drops that event for that tab.** That goes against decision C (cut the tab off so it reconnects and catches up). It's fixed in subtask 4.
- The stub agent is `runStub` in `cmd/agent-server/stub.go`.
- The browser check from the ae-init (two tabs, Clear, `curl -N`, a bad POST, the old routes returning 404) **hasn't been run by hand yet**.

**PR #7 review follow-ups** (`docs/ae-review/pr-7.md`):
- ✅ Fixed in [andrelas6/ai-agent-go#8](https://github.com/andrelas6/ai-agent-go/pull/8), merged as `e8a95f3`:
  - Ctrl-C no longer exits with status 1. `http.ErrServerClosed` is skipped, and the `Shutdown` error is logged.
  - A panicking runner becomes `workflow.failed` for its workflow (`runSafely`) instead of crashing the server.
  - The events stream returns on the first failed write or flush, and unsubscribes.
  - No more `Connection: keep-alive` header, which HTTP/2 forbids.
- ✅ Fixed in subtask 4: silent drops for a full tab, and `Subscribe` returning a `nil` channel after shutdown.
- Skipped on purpose: the ignored `writeJSON` encode error (headers are already sent by then), logging bad request bodies, and the placement of `internal/bus` and the `handlers` sub-package (both deliberate).

0. **Housekeeping.** *Owner: André.*
   - Commit or stash the untracked work on `ai-agent-go` `main`, then branch, e.g. `lesson-1-harness`.
   - In the course repo, branch off `lesson-1`.
1. **Inspector on SSE.** *Owner: Claude. Course repo, no TDD.*
   - `web/src/useHarnessSocket.ts`:
     - read events with `new EventSource('/api/events')`, setting `connected` from `onopen`/`onerror`;
     - `send()` POSTs `submit_task` as `{input, mode}` to `/api/tasks`;
     - keep the same `{events, connected, send}` return shape, so `App.tsx` doesn't change.
   - `web/src/components/TaskPane.tsx`: the two hard-coded `http://…:8787` URLs become relative `/api/clear` and `/api/approve/...`.
   - `vite.config.ts`: `server.proxy['/api']` → `http://localhost:8002`.
   - Check it with `npm run typecheck`, then by hand.
   - **When this lands:** `npm run dev:web` serves the inspector, and the browser's requests go to `/api/*` on `:5173`. They show as proxied failures until the Go side exists.
2. **Dummy POST endpoints.** *Owner: Claude. `ai-agent-go`, Red/Green.*
   - `cmd/agent-server/handlers.go` (package main) with `registerAPIRoutes(mux)`:
     - `POST /api/tasks` → `202 {"workflowId":"dummy-workflow-id"}`, ignoring the body;
     - `POST /api/clear` → `200 {"ok":true}`.
   - Call it from `main.go` next to the existing routes. Nothing else changes: `/sse`, `/messages` and `internal/messages` stay for subtask 3.
   - Tests use `httptest`: status code, `Content-Type: application/json`, and the body.
   - **When this lands:** submitting a task or clicking Clear in the inspector reaches the Go server and gets a dummy answer, with no proxy errors on those two calls.
3. **Real endpoints with the course stub agent.** *Owner: André.*
   - `go get github.com/google/uuid` and update `AGENTS.md`.
   - Add `internal/events`, `GET /api/events` (SSE), and a simple `internal/bus` (fan-out and history, under a mutex).
   - Fill in the two POST endpoints from subtask 2:
     - tasks decodes `{input, mode}`, gives 400 on bad input, and returns a UUID `workflowId` before starting the runner on the server's context;
     - clear empties the bus.
   - The runner is the course's own Lesson 1 stub: `workflow.started` → log "no agent yet" → `workflow.completed`.
   - Delete `/sse`, `/messages` and `internal/messages`.
   - **When this lands:** the inspector connects and shows the stub's three events, exactly like the course's starter app.
4. **A bus you can rely on.** ✅ Done (#9–#13). Fan-out, `Clear`, id and `ts` stamping and a non-blocking `Publish` came with subtask 3; the rest landed here:
     - **Resume from `Last-Event-ID` by UUID.** `Subscribe()` takes no id today, and the handler doesn't read the header. An unknown id gives a full replay.
     - **Cut off a full subscriber** instead of dropping its event (decision C). Close its channel so the SSE response ends, then let `EventSource` reconnect with `Last-Event-ID`.
     - **The 1000-event history cap.** History grows without limit today.
     - **After shutdown, `Subscribe` returns a closed channel**, not `nil`. Today the handler only survives because `ctx.Done()` is already closed.
   - **When this lands:** the two-tab, reload-mid-run and Clear checks from the ae-init pass. *These browser checks still haven't been run by hand.*
5. **`agent.Run` with a scripted fake model and fake tools.** All the agent-loop tests above. Remove `internal/runtime/`.
   - **When this lands:** `agent-server` still runs the stub; the loop is covered by tests only.
6. **The real Lesson 1 tools and system prompt**, replacing the fake tools in the default registry.
7. **The real OpenRouter model** (`OPENROUTER_MODEL`), tested against `httptest`. Wire `agent.Run` into `agent-server` in place of the stub.
   - **When this lands:** the sample task runs end to end in the inspector.
8. **`agent-cli`.** The task is all of stdin up to EOF, there's one readable line per event, and the same `agent.Run` runs underneath. Then add up the diff.
   - **Size:** about 550–650 lines with tests, so over 500. Proposed split into two PRs, each runnable on its own:
     - **PR 1, transport:** subtasks 2–3 landed as [andrelas6/ai-agent-go#7](https://github.com/andrelas6/ai-agent-go/pull/7). Subtask 1 landed separately in the course repo. The rest of subtask 4 landed in #9–#13.
     - **PR 2, agent loop:** subtasks 5–8.
