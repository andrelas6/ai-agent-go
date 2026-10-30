# ae-init: SSE event stream and bus, with the course stub agent

> **Status:** Draft
> **Last updated:** 2026-10-17
> **From:** ae-plan/lesson-1-harness.md, subtask 3

## What to build

The Go server sends real harness events to the course inspector. Submitting a task starts the course's Lesson 1 stub agent. It doesn't call a model yet; it announces the workflow, logs "no agent yet", and finishes. Every step is an event, and every open inspector tab sees it live. A tab that opens later first gets the timeline so far. Clear empties that timeline. The old echo endpoints, `/sse` and `/messages`, are removed. This is the course's starter app, rebuilt on the Go server, with SSE in place of WebSocket.

Out of scope (subtask 4): resuming from `Last-Event-ID`, the 1000-event cap, cutting off a slow tab, and race testing.

## A -> B

- **A (now):** The inspector's `EventSource` asks for `/api/events`, but `agent-server` has no such route, so it gets a 404 and shows disconnected. A task POST gets `202 {"workflowId":"dummy-workflow-id"}` and Clear gets `200 {"ok":true}`, but no event ever appears. The old `GET /sse` (invalid JSON, 3s flush) and `POST /messages` are still registered. `github.com/google/uuid` is in `go.mod` but unused.
- **B (after):** The inspector connects to `GET /api/events`. A task POST gets `202` with a real UUID `workflowId`, and every tab shows `workflow.started` → `log` ("no agent yet") → `workflow.completed` for that id. A new tab replays the history, and Clear empties it. Bad input gets a 400. `/sse` and `/messages` return 404.

## How to reproduce

**Preconditions**
- Course repo on `lesson-1` (with commit `98138c1`). Run `npm run dev:web` (inspector on :5173, `/api` proxied to :8002).
- `ai-agent-go` on `lesson-1-harness`. Run `go run ./cmd/agent-server` (:8002).
- No `OPENROUTER_API_KEY` needed. The stub agent doesn't call a model.

**Steps**
1. Open http://localhost:5173. Look at the connection indicator and the DevTools Network tab.
2. In a terminal, run `curl -N localhost:8002/api/events` and leave it open.
3. Submit `hello` in the inspector.
4. Open a second tab on http://localhost:5173.
5. `curl -i -X POST localhost:8002/api/tasks -d 'not json'`, then `curl -i -X POST localhost:8002/api/tasks -d '{"input":""}'`.
6. Click Clear, which reloads the page.
7. `curl -i localhost:8002/sse` and `curl -i -X POST localhost:8002/messages -d '{"message":"hi"}'`.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| Connection (step 1) | Disconnected. `/api/events` returns 404 and `EventSource` keeps retrying. | Connected. `/api/events` returns 200, `text/event-stream`, and stays open. |
| `curl -N` (steps 2–3) | 404 | Three blocks, each `id: <uuid>` + one `data: {...}` line of valid JSON, arriving right away. Types are `workflow.started`, `log`, `workflow.completed`, all with the same `workflowId`. |
| Task POST response (step 3) | `202 {"workflowId":"dummy-workflow-id"}` | `202 {"workflowId":"<uuid>"}`, the same id as the events |
| Event pane, tab 1 | Empty | The three events. The chat pane shows the "no agent yet" warning and the "(no agent implemented yet)" output, like the course's starter. |
| Tab 2 (step 4) | Empty | The same three events, replayed from history |
| Bad task input (step 5) | `202` dummy for both | `400` for invalid JSON and for an empty `input`. No events are emitted. |
| Clear (step 6) | `200 ok`; panes stay empty | `200 ok`. After the reload both panes are empty, and `curl -N` gets no history. |
| Old routes (step 7) | `/sse` holds the connection open; `/messages` returns 200 with an empty body | `404` for both |
| Two tabs open, submit again | Nothing | Both tabs show the new run's three events |

## Decisions

- **Duplicates on reconnect are accepted until subtask 4.** `EventSource` reconnects on its own, for example when the Go server restarts. Without `Last-Event-ID` handling, a reconnecting tab gets the whole history again and may show duplicates. Subtask 4 fixes this.

## Open questions

- None.
