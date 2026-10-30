# ae-init: Lesson 1 harness in Go (SSE inspector + agent loop)

> **Status:** Draft (open questions settled)
> **Last updated:** 2026-10-15

## What to build

Lesson 1 of the Harness Engineering course, but run by the Go server in `ai-agent-go` instead of the course's Node.js server. There are two parts, and they ship together as one milestone.

1. **Transport.** The course's prebuilt inspector talks to the Go server over SSE, not WebSocket, through a Vite dev proxy. Every harness event shows up live in it. A tab opened late, or one that reconnects, sees the timeline so far, without duplicates. Submitting a task and clearing the log reach the server as HTTP POSTs.
2. **Agent loop.** A brittle agent loop, `RunAgent`, takes a task and works it, one non-streamed turn at a time, with a configurable OpenRouter model and the four Lesson 1 tools (`searchKnowledgeBase`, `classifyItem`, `draftReply`, `sendReply`). It reports everything it does as course events. The same function runs from `agent-server` (events go to the inspector) and from `agent-cli` (one readable line per event in the terminal).

The inspector is edited in the course repo, on a local branch off `lesson-1`. Lesson 2 and later branches are reference only.

## A -> B

**Transport**

- **A (now):** The inspector (`harness-engineering/web`) opens `ws://<host>:8787/ws`, which only the Node server answers. `agent-server` listens on `:8002` and has two endpoints:
  - `POST /messages {"message"}`
  - `GET /sse`, which writes `data: {"token"}:"..."` (not valid JSON) and flushes only on a 3s tick.
  
  Events pass through one unbuffered channel. Two clients take events from each other, a POST with no listener blocks forever, and nothing is replayed.
- **B (after):** The inspector calls relative URLs, and the Vite dev server proxies them to the Go server on `:8002`. That removes the hard-coded `8787` and the need for CORS. It reads events over `EventSource`, and sends `submit_task` and clear as POSTs. Each event is one line of JSON that matches `shared/events.ts`, flushed as soon as it is emitted. Every open tab gets every event. A new or reconnecting tab gets the history once.

**Agent loop**

- **A (now):** `internal/runtime.RunAgent()` is empty. `agent-cli`:
  - reads one stdin line;
  - prepends `testdata/claude_hooks.md`;
  - calls `Chat.Send` with no tools and a hard-coded `google/gemma-4-31b-it`;
  - prints the raw choices JSON;
  - exits after the first normal answer, because the loop breaks unless the response is `NOT_FINISHED`.
  
  No tool ever runs, and no event is emitted.
- **B (after):** You submit the sample task, from the inspector or from `agent-cli`. `RunAgent` loops (at most 10 steps), calling tools until the model stops asking. It emits `workflow.started` → `tool.requested`/`tool.completed` per call → `model.completed` → `workflow.completed`, or `workflow.failed` when it hits the step cap or gets an error. The model id comes from an env var, defaulting to `google/gemma-4-31b-it`.

## How to reproduce

**Preconditions**
- `OPENROUTER_API_KEY` is set. The model env var is optional and defaults to `google/gemma-4-31b-it`.
- The course repo is on a local branch off `lesson-1`, with `npm install` done. Run only the inspector, with `npm run dev:web` (:5173). `npm run dev` also starts the Node server on :8787.
- The Go server is running: `go run ./cmd/agent-server` (:8002).
- Sample task (from the course's Lesson 1 notes):
  ```
  Handle these work items:
  - item-1 (customer_message): "I was charged twice and need help."
  - item-2 (bug_report): "The export button fails on Safari."
  - item-3 (sales_request): "Can you send pricing for 50 seats?"
  ```

**Steps**
1. Open http://localhost:5173 in two tabs. Look at the connection indicator and the DevTools Network tab.
2. Submit the sample task in tab 1.
3. Reload tab 2 halfway through the run.
4. Click Clear.
5. Run `curl -N` against the Go server's SSE endpoint on `:8002` in a terminal while step 2 runs.
6. Run `go run ./cmd/agent-cli` and paste the sample task.
7. Run step 6 again with the model env var set to another OpenRouter model.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| Inspector connection (steps 1–2) | Disconnected. The `ws://…:8787/ws` request fails. The task goes nowhere. | Connected. Network shows an `eventsource` request and a task POST, both to `localhost:5173` and proxied to `:8002`. No CORS errors. |
| Event pane, tab 1 | Empty | `workflow.started`, a `tool.requested`/`tool.completed` pair for each tool call over the three items, then `model.completed` and `workflow.completed`. No `model.delta`. |
| Tab 2, after reload (step 3) | Empty | The full timeline so far, each event once, then live events |
| Raw stream (step 5) | `data: {"token"}:"…"` lines, sent in bursts every ~3s | One `data: {...}` line of valid JSON per event, arriving as it is emitted |
| Clear (step 4) | The POST fails (no `/api/clear` on Go) | Both panes empty, and a reload shows no history |
| `agent-cli` (step 6) | Prints `RESPONSE: [ ...choices JSON... ]` once, then exits. No tools run. | One readable line per event, e.g. `tool.requested classifyItem {"itemId":"item-1",...}`, ending with the final summary |
| Model switch (step 7) | Needs a code change | The run uses the model from the env var, with no code change |
| Step cap | n/a | A task that never stops ends in `workflow.failed` with "step limit" after 10 turns |

## Decisions

- **No token streaming in Lesson 1.** Use non-streaming `Chat.Send` and emit `model.completed` once per turn. `model.delta` streaming is a later change.
- **The model is configurable** through an env var, defaulting to `google/gemma-4-31b-it`. Test the three-item task, and switch models if tool calls are unreliable.
- **A Vite dev proxy** sends the inspector's relative URLs to `:8002`. There is no hard-coded port, and CORS isn't needed. Item 5 (CORS) drops out of scope.
- **The inspector is edited on a local branch off `lesson-1`.** Lesson 2 and later branches are reference only.
- **No `/api/approve` route until Lesson 7.** The button can't appear without an `approval.requested` event.
- **`agent-cli` prints one readable line per event.**

## Open questions

- None.
