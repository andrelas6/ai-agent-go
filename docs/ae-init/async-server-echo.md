# ae-init: async server — SSE + echo

> **Status:** Draft
> **Last updated:** 2026-09-04 (routes, envelope, and scope confirmed)

## What to build

The first slice of the MVP server: an async HTTP server a client can talk to
over SSE. The client opens `GET /sse` and, as the first event on that
stream, receives the session's message URL: `/messages/session/:uuid`. From
then on, anything the client POSTs (JSON) to that URL comes back to it as a
new event on the still-open SSE stream — the message text with `!!!`
appended. There is no agent loop, no LLM call, no permissions and no storage
yet; this slice only proves the connect -> get URL -> post ->
receive-on-stream round trip works.

## A -> B

- **A (now):** There is no server in this repo. No `go.mod`, no server
  code — the repo is `README.md` plus `analysis/` (reference clones of
  opencode, crush, and the author's own mini agent) and `docs/` (inspiration
  notes and an MVP architecture sketch in
  `docs/mvp/first-architecture.mermaid`). There is nothing to call.
- **B (after):** A client can `GET /sse` and keep the connection open. The
  first event delivered on that stream contains the session's message URL,
  `/messages/session/:uuid` (a fresh UUID per connection — no reconnect
  support yet). The client `POST`s a JSON message to that URL and, within
  the same open SSE stream, receives a new event whose body is
  `<message>!!!`.

## How to reproduce

**Preconditions**
- Server built and running locally (exact run command is TBD — decided in
  ae-plan, since no `go.mod`/`cmd` layout exists yet).
- No auth, no flags, no env vars for this slice.

**Steps**
1. Start the server.
2. Open the SSE connection: `curl -N http://localhost:PORT/sse`.
3. Read the first event on the stream — its data is
   `/messages/session/:uuid`.
4. `POST` a JSON message to that URL, e.g.
   `curl -X POST http://localhost:PORT/messages/session/:uuid -d '{"message":"hello"}'`.
5. Watch the still-open SSE stream from step 2 for the next event.

**Observe**

| | A (now) | B (after) |
|---|---|---|
| `GET /sse` | connection refused — nothing listens | `200`, stream stays open; first event's data is `/messages/session/:uuid` |
| `POST /messages/session/:uuid` with JSON body | n/a — no such endpoint exists | `200`/`202` accepted, and the open SSE stream (from step 2) emits a new event with body `hello!!!` |

## Open questions

None — routes, envelope, and connection scope confirmed:
- `GET /sse`, `POST /messages/session/:uuid`.
- JSON message body.
- Per-connection session for now; no reconnect support in this slice.
