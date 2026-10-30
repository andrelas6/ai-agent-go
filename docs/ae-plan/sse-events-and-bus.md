# ae-plan: SSE event stream and bus, with the course stub agent

> **Status:** Done (subtasks 1–15). Manual browser check pending
> **Last updated:** 2026-10-24
> **From:** ae-init/sse-events-and-bus.md (parent: ae-plan/lesson-1-harness.md, subtask 3)

## What's being built

`agent-server` sends real harness events to the course inspector. `GET /api/events` streams them over SSE. An in-memory bus fans every event out to each open tab and keeps a history, so a tab that opens late first gets the timeline so far. `POST /api/tasks` gets a real UUID `workflowId`, rejects bad input with a 400, and starts the course's Lesson 1 stub agent on the server's context: `workflow.started`, then a "no agent yet" `log`, then `workflow.completed`. `POST /api/clear` empties the history. The old `/sse`, `/messages` and `internal/messages` go away. It's the course's starter app, rebuilt in Go, with SSE in place of WebSocket.

## Constraints

- **Dependencies:** the standard library, the OpenRouter SDK and `github.com/google/uuid`. The uuid change to `AGENTS.md`, `go.mod` and `go.sum` is already in the working tree, uncommitted. It goes into the first commit that imports uuid. Run `go mod tidy` there so uuid moves from `// indirect` to a direct require.
- **Contract:** event JSON matches the course's `shared/events.ts`. That means camelCase fields, `id` and `workflowId` as UUID strings, `ts` in milliseconds, and `args`/`result` as raw JSON.
- **Layout follows usage:**
  - `internal/events` is shared with `agent-cli` later.
  - `internal/bus` is the deliberate exception: server-only, but in `internal/`.
  - The handlers and the stub agent are server-only, so they live in `cmd/agent-server` as `package main`.
- **Tasks run on the server's context,** not the request's.
- **Red/Green TDD, outside-in,** using standard `testing` and `httptest`, no testify. Claude adds no code comments, but the comments André writes stay. One subtask per commit, committed only after review.
- **Out of scope (parent subtask 4):**
  - `Last-Event-ID` resume;
  - the 1000-event cap;
  - cutting off a slow tab;
  - `go test -race`.

  A reconnecting tab gets the whole history again, so it may show duplicates. That's accepted until then.

## What I found in the codebase

- **`cmd/agent-server/handlers.go`** (commit `40587a2`):
  - `registerAPIRoutes(mux)` registers dummy `POST /api/tasks` (`202 {"workflowId":"dummy-workflow-id"}`) and `POST /api/clear` (`200 {"ok":true}`);
  - `writeJSON` sets `Content-Type`, the status and the body. We keep it.
- **`cmd/agent-server/handlers_test.go`:** `httptest.NewServer(newTestMux())`, a `readBody` helper and plain `t.Errorf`. `TestPostTasksReturnsDummyWorkflowID` gets replaced, not kept. `TestPostClearReturnsOK` and `TestGetTasksIsNotAllowed` stay.
- **`cmd/agent-server/main.go`:**
  - `signal.NotifyContext` gives the server context, already the one to hand to tasks.
  - The `GET /sse` handler, the `POST /messages` handler, `ChatRequest`, `Dependencies` and the `messages.Service` wiring all get deleted.
  - `http.Server` has `ReadHeaderTimeout` and no `WriteTimeout`. Keep it that way: a `WriteTimeout` would kill SSE streams.
- **`internal/messages/`:** a single unbuffered channel and an empty test. Deleted.
- **Course `server/bus.ts`:** `emit` stamps `id: randomUUID()` and `ts: Date.now()`, pushes to history and calls every listener. `history()` returns a copy. On connect, `server/index.ts` replays the history, then sends live events. Our bus does the same.
- **Course `harness/runtime.ts`:** the stub. It emits `workflow.started {input}`, then `log {level:"warn", message:"No agent yet. You build the brittle agent loop in Lesson 1 (harness/runtime.ts)."}`, then `workflow.completed {output:"(no agent implemented yet)"}`. In the course the stub makes its own `workflowId`. Here the handler makes it and passes it in, so the 202 and the events share the same id.
- **Course `shared/events.ts`:** `log` has an optional `workflowId` and a `level`. Every other event carries `workflowId`.
- **Course `web/src/useHarnessSocket.ts`:**
  - `EventSource('/api/events')`, with `connected` set from `onopen`/`onerror`;
  - each `onmessage` does `JSON.parse(e.data)`, so the inspector needs one JSON object per message;
  - the POST body is `{input, mode}`, and the reply is ignored.
- **Course `TaskPane.tsx:188`:** renders `log` events from `level` and `message`, which is how the "no agent yet" warning shows up.

## What the docs say

- **Go `net/http` (go1.26.4, `go doc`):**
  - `http.NewResponseController(w).Flush()` sends buffered data to the client. Call it after the headers and after every event.
  - `Server.Shutdown` waits for active connections to go idle, so a long-lived SSE handler has to return by itself when the server context ends. Otherwise Ctrl-C waits out the full 10 s timeout.
  - Method-and-path patterns (`"GET /api/events"`) work on the standard mux, which already returns 405 for a wrong method and 404 for an unknown path.
- **MDN, [Using server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events):**
  - each message is `id: …` plus `data: …` lines, ending with a blank line;
  - multiple `data:` lines are joined, so one-line JSON keeps framing simple;
  - `id:` sets the last event ID that a later `Last-Event-ID` resume will use.

## Approach

Three seams, all defined by the caller in `cmd/agent-server`:

- **`eventBus` interface:**
  - `Publish(events.Event)`;
  - `Subscribe() (history []events.Event, live <-chan events.Event, unsubscribe func())`;
  - `Clear()`.
- **`runner` func:** `func(ctx context.Context, workflowID, input string, emit func(events.Event))`.
- **`newMux(ctx, bus, run) *http.ServeMux`:** used by both `main` and the tests. It registers `/health` and the three `/api` routes, and nothing else, so `/sse` and `/messages` can be tested as 404s.

How the pieces behave:

- **`GET /api/events`:**
  - sets `Content-Type: text/event-stream` and `Cache-Control: no-cache`, then **flushes right away**. Without that flush, an empty history means the client never gets headers: `EventSource.onopen` never fires and the inspector shows disconnected;
  - calls `Subscribe`, writes each history event as `id: <id>\ndata: <json>\n\n`, then loops over `live`, flushing after each event;
  - returns when the request context ends (tab closed) or the server context ends (Ctrl-C), and calls `unsubscribe` on the way out.
- **`POST /api/tasks`:**
  - decodes `{input, mode}`. Invalid JSON or an empty `input` gives 400, and nothing is emitted. `mode` is accepted and ignored;
  - makes `workflowId := uuid.NewString()`, starts `go run(serverCtx, workflowId, input, bus.Publish)`, and returns `202 {"workflowId": …}`;
  - uses the server context passed to `newMux`, not `r.Context()` and not `context.WithoutCancel`, so Ctrl-C still reaches the task.
- **`POST /api/clear`:** calls `bus.Clear()` and returns `200 {"ok":true}`.
- **`internal/events`:**
  - the `Type` constants for the Lesson 1 event types: `workflow.started/completed/failed`, `model.completed`, `tool.requested/completed/failed` and `log`;
  - one flat `Event` struct: `id`, `ts` (`int64` ms), `type`, `workflowId`, `input`, `output`, `error`, `text`, `toolCallId`, `name`, `args`/`result` (`json.RawMessage`), `level`, `message`. Every field except `id`, `ts` and `type` is `omitempty`;
  - known trap for Lesson 7: `approved:false` would vanish under `omitempty`, so that field stays out until then.
- **`internal/bus`:**
  - `New()` returns a `*Bus` with a mutex, a history slice and a set of subscriber channels;
  - `Publish` stamps `id = uuid.NewString()` and `ts = time.Now().UnixMilli()`, like the course's `emit`. It then appends to history and does a **non-blocking** send to each subscriber's buffered channel (256), dropping the event for that tab if its buffer is full;
  - `Subscribe` takes the history snapshot and registers the channel **under the same lock**, so nothing is lost or sent twice between the two;
  - `unsubscribe` removes the channel. `Clear` empties the history.
- **Stub runner, `cmd/agent-server/stub.go`:** `runStub` emits the course's three events with the given `workflowId`. It's deleted when `agent.Run` takes its place (parent subtask 7).

## Decisions

- **Non-blocking send with a drop when a tab's buffer is full.**
  - Why: `Publish` must never block the agent, and a blocking send under the lock would deadlock against a handler that is unsubscribing.
  - The gap: a tab that falls more than 256 events behind silently misses events. The stub sends 3 per task, so this won't happen in practice.
  - Parent subtask 4 replaces the drop with cutting the tab off and resuming from `Last-Event-ID`.
- **Duplicates on reconnect are accepted** until parent subtask 4.
- **The stub lives in `cmd/agent-server`.** It's server-only and temporary.
- **`log` gets the stub's `workflowId`,** like the course does, even though the field is optional.
- **The stub's log text points at the Go file.** It reads "No agent yet. You build the brittle agent loop in Lesson 1 (internal/agent/agent.go)." That's where `agent.Run` gets built (parent subtask 5). Everything else in the text matches the course.

## Red / Green

**Contract (`internal/events`)**
- An `Event` marshals to the course's shape. The test unmarshals into `map[string]any` and checks keys and values, instead of comparing strings, since Go writes keys in struct order:
  - `workflowId` and `toolCallId` are camelCase;
  - `ts` is a number;
  - `args` and `result` pass through as raw JSON objects, not strings;
  - unset optional fields are absent.
- A `log` event without a `workflowId` has no `workflowId` key.

**Bus (`internal/bus`)**
- `Publish` stamps a UUID `id` (it parses with `uuid.Parse`) and a non-zero millisecond `ts`.
- Two subscribers both get every event, in order.
- `Publish` with no subscribers doesn't block.
- `Subscribe` returns the earlier events as history, then later events on `live`.
- After `unsubscribe`, `Publish` still works and the channel gets nothing more.
- `Clear` empties history: a new `Subscribe` gets none.
- A subscriber that never reads doesn't block `Publish`, and other subscribers still get every event.

**HTTP (`cmd/agent-server`, fake bus and fake runner)**
- `GET /api/events` returns 200 with `Content-Type: text/event-stream`, and the headers arrive with an empty history. The client's `Get` returns without waiting for an event.
- History events come first, each as `id: <id>` plus one `data: <json>` line and a blank line. Then a live event follows.
- The handler returns, and calls `unsubscribe`, when the client disconnects, and also when the server context is cancelled.
- `POST /api/tasks` with `{"input":"hello"}` returns 202, `Content-Type: application/json` and a UUID `workflowId`. The fake runner is called with that id and `hello`.
- The runner's context is still live after the 202, and is cancelled when the test cancels the server context.
- Invalid JSON gives 400, and `{"input":""}` gives 400. The runner isn't called in either case.
- `POST /api/clear` calls `Clear` and returns `200 {"ok":true}`.
- `GET /sse` and `POST /messages` return 404. `GET /api/tasks` still returns 405.

**Stub (`cmd/agent-server`)**
- `runStub` emits `workflow.started {input}`, then `log {level:"warn", message:"No agent yet. You build the brittle agent loop in Lesson 1 (internal/agent/agent.go)."}`, then `workflow.completed {output:"(no agent implemented yet)"}`, all with the given `workflowId`.

**By hand:** the ae-init steps 1–7. This includes `curl -N` showing the three blocks right away through the server, and the inspector on `:5173` through the Vite proxy.

## Most important file changes

| File | Change |
|---|---|
| `AGENTS.md`, `go.mod`, `go.sum` | the uuid allowance, already in the working tree; `go mod tidy` makes it a direct require |
| `internal/events/events.go`, `events_test.go` | new |
| `cmd/agent-server/handlers.go`, `handlers_test.go` | the seams, `newMux`, SSE, real tasks and clear |
| `internal/bus/bus.go`, `bus_test.go` | new |
| `cmd/agent-server/stub.go`, `stub_test.go` | new |
| `cmd/agent-server/main.go` | serves `newMux(ctx, bus.New(), runStub)`; the old routes and types go |
| `internal/messages/` | deleted |

## Open questions

- None.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit.

> **Progress, 2026-10-20.** Subtask 1 landed as `e5addc2`. Subtasks 2–4 were built partly, across `29f8b2a` and `89529f6` (both pushed, so they stay as they are). A review against this plan turned up bugs and differences. Each one is now its own subtask, 5–15 below. A subtask 16 that removed code comments was dropped on 2026-10-23: the comments in the code are kept on purpose.
>
> **Kept on purpose:** the handlers live in their own package, `cmd/agent-server/handlers`, not in `package main`. It's still server-only, so it fits "layout follows usage". The exported names (`NewMux`, `EventBus`, `Runner`) are fine.
>
> **Scope change:** subtask 5 adds one `go test -race` check for the bus. Race testing as a whole stays in parent subtask 4. This one is here because the race it catches can crash the server.

### Status, 2026-10-24: all subtasks done

Checked against `89aa1b7`, pushed to `origin/lesson-1-harness`. The working tree is clean. `go build`, `go vet` and `go test -race ./...` all pass.

**Still to do by hand:** the ae-init's B column in the real browser (the inspector on `:5173` through the Vite proxy, two tabs, Clear, `curl -N`, a bad task POST, the old routes returning 404).

| # | Subtask | Status | Where |
|---|---|---|---|
| 1 | The event contract | ✅ Done | `e5addc2` |
| 2 | Handlers on their real interfaces | ✅ Done | Through 10–14 |
| 3 | The real bus | ✅ Done | Through 5–9 |
| 4 | The stub agent | ✅ Done | Through 12–13, in `6cc5db2` |
| 5 | No races in the bus | ✅ Done | `212593b`. One owner goroutine holds the map and the history, instead of a mutex |
| 6 | `Publish` never blocks | ✅ Done | `7592f00`. A full buffer drops the event for that tab. The buffer is a `SubscriberBuffer = 256` constant. `Clear` gives up once the bus has shut down |
| 7 | `unsubscribe` removes, then closes | ✅ Done | `212593b` |
| 8 | `Subscribe` returns a copy of the history | ✅ Done | `212593b` |
| 9 | The plan's bus tests | ✅ Done | `6b48d1b`: `Clear` leaves a new subscriber with no history, events after `Clear` start a new history, `Publish` with no subscribers doesn't block. `TestConcurrentPublishSubscribe` removed |
| 10 | One request body per request | ✅ Done | `6cc5db2` |
| 11 | Handler tests | ✅ Done | `3d3bd7a` (fake bus for disconnect and shutdown), `6cc5db2` (400 table test), `eecf687` (history before live, Clear through the API, UUID `workflowId`). The other handler tests use the real bus |
| 12 | The runner seam | ✅ Done | `6cc5db2` |
| 13 | `runStub` with the course's events | ✅ Done | `6cc5db2` |
| 14 | `main` registers no routes; old code goes | ✅ Done | `89aa1b7`: `ChatRequest` removed, `/health` 200 and `/sse` / `/messages` 404 tests |
| 15 | `go mod tidy` | ✅ Done | uuid is a direct require |
| — | The bus stamps `id` and `ts` | ✅ Done | `5e9dcf0` |

1. **The event contract.** ✅ Done in `e5addc2`.
   - `internal/events` with the `Type` constants and the flat `Event`, plus the contract tests.
   - It's the vocabulary the handler seams are written in, so it comes first. It depends on nothing, so nothing needs a mock.
   - **When this lands:** `go test ./...` passes. The server behaves as before.
2. **The handlers on their real interfaces, with fakes behind them.** ✅ Done through subtasks 10–14.
   - In `handlers.go`, add the `eventBus` interface and the `runner` type, and replace `registerAPIRoutes` with `newMux(ctx, bus, run)`.
   - Build `GET /api/events` (SSE), real `POST /api/tasks` (uuid, 400s, server context) and `POST /api/clear` on `bus.Clear`.
   - Tests use a fake bus and a fake runner.
   - `main` serves `newMux` with placeholders: `nopBus` (in package main: empty history, a live channel that never sends, `Clear` does nothing) and a runner that does nothing.
   - Since `main` now serves only `newMux`, delete `/sse`, `/messages`, `ChatRequest`, `Dependencies` and `internal/messages` here. The 404 test for the old routes is part of this subtask.
   - Commit the uuid change to `AGENTS.md`, `go.mod` and `go.sum` here, after `go mod tidy`.
   - **When this lands:** the inspector shows connected. A task POST returns 202 with a UUID, bad input returns 400, and `/sse` and `/messages` return 404. No events appear yet.
3. **The real bus.** ✅ Done through subtasks 5–9.
   - `internal/bus` with its tests.
   - `main` swaps `nopBus` for `bus.New()`, and `nopBus` is deleted.
   - **When this lands:** the inspector stays connected and Clear works, but nothing publishes yet.
4. **The stub agent.** ✅ Done through subtasks 12–13, in `6cc5db2`.
   - `runStub` with its test, wired into `main` in place of the runner that does nothing.
   - **When this lands:** every row of the ae-init B column holds.

**Bus fixes (`internal/bus`)**

5. **Guard the bus with a mutex.** *Bug.* ✅ Done in `212593b`, with an owner goroutine instead of a mutex.
   - **Why:** `Publish` runs in the tasks goroutine while `Subscribe` and `unsubscribe` run in SSE handlers, and all three touch the same map and slice. `go test -race` already fails on `TestGetStreamingEvents` (`bus.go:21` against `handlers.go:130`). A concurrent map write stops the whole server with `fatal error: concurrent map writes`.
   - **Red:** a bus test where one goroutine publishes while another subscribes and unsubscribes in a loop. Run it with `go test -race ./internal/bus/`.
   - **Green:** add `mu sync.Mutex` to `Bus`, and hold it in `Publish`, `Subscribe`, `unsubscribe` and `Clear`.
   - **When this lands:** `go test -race ./internal/bus/ ./cmd/agent-server/...` passes.
6. **`Publish` never blocks.** *Bug.* ✅ Done in `7592f00`.
   - **Why:** `stream <- event` blocks once a tab's buffer of 256 is full. One stuck tab stops the runner, and with the lock from subtask 5, it stops every other tab too.
   - **Red:** one subscriber never reads, and a second one reads in a goroutine. Publish 300 events: `Publish` returns within 1 s, and the reading subscriber gets all 300.
   - **Green:** `select { case stream <- event: default: }`, plus a named `subscriberBuffer = 256` constant.
   - **When this lands:** a stuck tab can't stall the agent. It loses events instead, as the plan's Decisions accept until parent subtask 4.
7. **`unsubscribe` removes the channel, then closes it.** *Bug.* ✅ Done in `212593b`.
   - **Why:** it closes before deleting, without the lock, so a `Publish` running at the same moment sends on a closed channel and panics.
   - **Red:** after `unsubscribe`, `Publish` doesn't panic, and a receive on the old channel returns `ok == false`.
   - **Green:** lock, `delete`, unlock, then `close`.
8. **`Subscribe` returns a copy of the history.** *Bug.* ✅ Done in `212593b`.
   - **Why:** it returns `e.history` itself, so the handler reads it while `Publish` appends to it.
   - **Red:** change an element of the returned history. A new `Subscribe` still sees the original event.
   - **Green:** under the lock, `append([]events.Event(nil), b.history...)`.
9. **The bus tests from the plan's Red/Green list.** *Different from the plan.* ✅ Done in `6b48d1b`.
   - Replace `TestEventBusTwice`, which tests two separate buses, with **two subscribers on the same bus both get every event, in order**.
   - Add these tests:
     - earlier events come back as history and later ones arrive on `live`;
     - `Publish` with no subscribers doesn't block;
     - `Clear` leaves a new subscriber with no history.
   - **When this lands:** the bus part of the Red/Green list is covered.

**Handler fixes (`cmd/agent-server/handlers`)**

10. **One request body per request in `POST /api/tasks`.** *Bug.* ✅ Done in `6cc5db2`.
    - **Why:** `reqBody` is declared outside the returned function, so all requests share it. Two POSTs at once overwrite each other, and a field one request leaves out keeps the previous request's value.
    - **Red:** POST `{"input":"a"}` (202), then POST `{}`. The second one must be 400. Today it returns 202 with the old input.
    - **Green:** move `var reqBody struct{…}` inside `func(w, r)`.
11. **Handler tests use a fake bus, and the missing handler tests.** *Different from the plan.* ✅ Done in `3d3bd7a`, `6cc5db2` and `eecf687`.
    - **Why:** the handler tests use `bus.New()`, so a bus bug shows up as a handler failure, which is what the race in subtask 5 did. `StubEventBus` is declared and never used.
    - Turn `StubEventBus` into a real fake for `EventBus` (with `var _ handlers.EventBus = (*StubEventBus)(nil)`) and use it in the handler tests. Then add:
      - history events are written before live ones;
      - the handler returns, and calls `unsubscribe`, when the client disconnects, and also when the server context is cancelled;
      - `POST /api/clear` calls `Clear`;
      - the 202's `workflowId` parses with `uuid.Parse`;
      - invalid JSON gives 400, and so does `{"input":""}`.
12. **The runner seam.** *Different from the plan.* ✅ Done in `6cc5db2`.
    - **Why:**
      - `NewMux` takes `run` and never uses it, and `Runner` is `func()`;
      - the stub's events are published from inside `handlePostTasks`, so nothing checks that tasks run on the server's context;
      - parent subtask 7 would have to edit the handler instead of passing `agent.Run` in `main`.
    - Change `Runner` to `func(ctx context.Context, workflowID, input string, emit func(events.Event))`. The handler calls `go run(ctx, workflowID, reqBody.Input, b.Publish)` with the server context from `NewMux`.
    - **Red (fake runner):**
      - it gets the same `workflowId` as the 202, and the input;
      - its ctx is still live after the 202, and is cancelled when the test cancels the server context;
      - it isn't called on either 400.
    - To keep the program running, `main` passes a temporary runner that publishes today's three events, moved out of the handler unchanged.
    - **When this lands:** the handler no longer knows what a task does.
13. **`runStub` with the course's events.** *Different from the plan.* ✅ Done in `6cc5db2`.
    - **Why:** today the log has no `level` and says "no model yet", `workflow.completed` has no `output`, and every event carries `Input`.
    - **Red:** `runStub` emits, all with the given `workflowId`, in this order:
      - `workflow.started {input}`;
      - `log {level:"warn", message:"No agent yet. You build the brittle agent loop in Lesson 1 (internal/agent/agent.go)."}`;
      - `workflow.completed {output:"(no agent implemented yet)"}`.
    - **Green:** `cmd/agent-server/stub.go` (`package main`), wired into `main` in place of the temporary runner from subtask 12.
    - **When this lands:** the inspector shows the "no agent yet" warning and the "(no agent implemented yet)" output, like the course's starter.

**Cleanup**

14. **`main` registers no routes, and the old code goes.** *Different from the plan.* ✅ Done in `89aa1b7`.
    - **Why:**
      - `/health` is added in `main`, outside `NewMux`, so no test sees every route the server serves;
      - `ChatRequest`, `Dependencies`, the `messages` import, `internal/messages/` and the `RUNNING` print are left over.
    - **Red:** `GET /health` returns 200 `ok`. `GET /sse` and `POST /messages` return 404. Both go through `NewMux`.
    - **Green:** move `/health` into `NewMux`, then delete the leftovers.
15. **`go mod tidy`.** *Different from the plan.* ✅ Done.
    - uuid is still `// indirect` in `go.mod`, though it's imported directly now.
    - No test. Check that `go build ./...` passes and the `// indirect` is gone.
    - **When this lands:** every row of the ae-init B column holds, and `go test -race ./cmd/agent-server/... ./internal/...` passes.

**Size check:** subtasks 5–15 add roughly 120 lines of production changes and 300 lines of tests, on top of about 450 already committed. That's over 500 for the whole of PR 1 (transport). If it's too much to review at once, split it, keeping each PR runnable:
- **PR 1a:** subtasks 1–9 (the contract, and the bus made safe);
- **PR 1b:** subtasks 10–15 (handler fixes, the runner seam, the stub, cleanup).
