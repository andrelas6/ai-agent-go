# ae-plan: async server — SSE + echo

> **Status:** Draft
> **Last updated:** 2026-09-05

## What's being built

The first Go code in this repo: a minimal async HTTP server. A client opens
`GET /sse` and keeps it open; the first event on that stream is the client's
personal message URL, `/messages/session/:uuid`. The client `POST`s JSON to
that URL and the message comes back as a new SSE event on the same stream,
with `!!!` appended. No agent loop, no LLM, no persistence — this only
proves the wiring: connect, get a URL, post, see it echoed on the stream.

## Why

This is the base the real agent loop will sit on later (per
`docs/mvp/first-architecture.mermaid` and the Excalidraw MVP sketch: SSE out,
HTTP POST in, per-session). Getting the transport shape right — and proven
with a trivial handler — before any agent logic exists means the loop can be
dropped in later without re-deriving the HTTP/SSE plumbing.

## Constraints

- Per `docs/ae-init/async-server-echo.md`: `GET /sse`, `POST
  /messages/session/:uuid`, JSON body, one session per connection (no
  reconnect support in this slice).
- `cmd/server/main.go` (the process entrypoint, real `net.Listener`) has no
  meaningful failing test to write first — `ListenAndServe` is a thin
  wrapper around code already covered by handler tests. It's built without a
  Red step; we know it works by running it and following the manual repro
  steps in the ae-init doc (`curl -N /sse`, then `curl -X POST
  .../messages/session/<uuid>`).
- Keep dependencies minimal: this is the first slice of a from-scratch repo,
  no existing Go pattern to match yet (only the `analysis/` reference clones,
  which are inspiration, not this project's own code).

## What I found in the codebase

- The repo has no Go code yet — no `go.mod`, nothing outside `README.md`,
  `analysis/` (read-only reference clones of opencode, crush, and the
  author's own Python mini-agent), and `docs/`. This slice starts the
  module.
- `docs/mvp/first-architecture.mermaid` and the Excalidraw "AI AGENT GO"
  scene both draw the same shape this plan implements: client gets an SSE
  stream, posts to an HTTP endpoint, server pushes back over the stream.
- `analysis/crush/internal/server/proto.go:270-330`
  (`handleGetWorkspaceEvents`) is the SSE pattern I'm following: set
  `Content-Type: text/event-stream`, `Cache-Control: no-cache`,
  `Connection: keep-alive`; `WriteHeader(200)` then flush immediately so the
  client doesn't block waiting for the first byte; then a `for { select {
  case <-r.Context().Done(): / case ev := <-channel: } }` loop writing
  `data: %s\n\n` and flushing per event, using
  `http.NewResponseController(w).Flush()` rather than a `w.(http.Flusher)`
  type assertion.
- `analysis/crush/internal/server/server.go:157-186` shows the router style:
  plain `http.NewServeMux()` with Go 1.22+ combined method+path patterns
  (`mux.HandleFunc("GET /v1/workspaces/{id}/events", ...)`), no external
  router library.
- `analysis/crush/internal/server/e2e_test.go` shows the test style for this
  kind of handler: `httptest.NewServer`, a small helper that reads
  `data:`-prefixed lines off the SSE body with `bufio.Scanner`/`bufio.Reader`,
  and `github.com/stretchr/testify/require` for assertions.
- Sessions need a UUID; crush uses `github.com/google/uuid`
  (`analysis/crush/internal/server/proto.go` imports it for
  `requireClientID`). I'm using the same package rather than hand-rolling
  UUID generation.

## What the docs say

- go.dev, ["Routing Enhancements for Go 1.22"](https://go.dev/blog/routing-enhancements):
  `net/http.ServeMux` patterns can combine a method and a path
  (`"GET /sse"`), and `{name}` path segments are read back with
  `r.PathValue("name")` — this is what `/messages/session/{id}` uses, no
  router library needed.
- pkg.go.dev, [`net/http#ResponseController`](https://pkg.go.dev/net/http#ResponseController):
  `http.NewResponseController(w).Flush()` is the current recommended way to
  flush a streaming response — it degrades to a typed error instead of a
  panicking type assertion when the underlying `ResponseWriter` doesn't
  support flushing, which matters for a handler that must work under
  `httptest` as well as a real listener.

## Approach

Two new packages plus an entrypoint:

- `internal/session`: a `Registry` that creates a session (UUID + a
  `chan string`) and looks it up by ID. Pure in-memory map behind a mutex,
  no HTTP.
- `internal/httpserver`: builds the `*http.ServeMux` (`GET /sse`, `POST
  /messages/session/{id}`). `GET /sse` creates a session, writes its URL as
  the first SSE event, then forwards anything sent on that session's channel
  as further SSE events until the client disconnects. `POST
  /messages/session/{id}` looks the session up, decodes `{"message":
  "..."}`, and sends `message + "!!!"` on its channel (404 if the id isn't
  known).
- `cmd/server/main.go`: wires `session.NewRegistry()` into
  `httpserver.New(...).Mux()` and calls `http.ListenAndServe`, port from the
  `PORT` env var (default `8080`).

Testing is stdlib-only (`testing` + `net/http/httptest`) — no testify. Even
though crush (the reference repo) uses testify, this project has no test
dependency yet and stdlib `testing` is enough for the assertions this slice
needs; revisit if later plans want testify's fluency.

## Red / Green

1. **Red:** `internal/session/registry_test.go` —
   `TestRegistry_CreateThenGet` (Create returns an id and a channel; Get(id)
   returns that same channel, `ok=true`) and `TestRegistry_GetUnknown`
   (Get on an unregistered id returns `ok=false`). Fails: package doesn't
   exist yet.
2. **Green:** `internal/session/registry.go` — `Registry{mu sync.Mutex,
   sessions map[string]chan string}`, `NewRegistry()`, `Create() (id
   string, ch chan string)` using `uuid.NewString()`, `Get(id string) (chan
   string, bool)`.
3. **Red:** `internal/httpserver/server_test.go` —
   `TestPostMessage_UnknownSession` — POST `/messages/session/does-not-exist`
   with a JSON body against `httptest.NewServer(New(session.NewRegistry()).Mux())`
   expects `404`. Fails: package doesn't exist yet.
4. **Green:** `internal/httpserver/server.go` — `New(reg *session.Registry)
   *Server`, `(*Server) Mux() http.Handler` registering `POST
   /messages/session/{id}`; handler does `reg.Get(r.PathValue("id"))`,
   writes 404 if missing.
5. **Red:** `TestPostMessage_KnownSession_Echoes` — create a session
   directly via `reg.Create()`, POST `{"message":"hello"}` to
   `/messages/session/<id>`, read the registry's channel, expect
   `"hello!!!"`; assert `202` on the response.
6. **Green:** decode the JSON body, send `req.Message + "!!!"` on the
   session's channel.
7. **Red:** `TestGetSSE_FirstEventIsSessionURL` — GET `/sse` with `Accept:
   text/event-stream` against the same test server; read the first `data:`
   line; assert it matches `/messages/session/<uuid>` and that
   `reg.Get(id)` finds it.
8. **Green:** `handleSSE` — `reg.Create()`, set SSE headers, `WriteHeader(200)`
   + `http.NewResponseController(w).Flush()`, write `data:
   /messages/session/<id>\n\n` + flush, then `for { select { case
   <-r.Context().Done(): return; case msg := <-ch: fmt.Fprintf(w, "data: %s\n\n",
   msg); flush } }`.
9. **Red:** `TestRoundTrip_PostThenSSEEchoes` — open `GET /sse`, read the
   session URL from the first event, `POST` `{"message":"hello"}` to it,
   read the *next* SSE event on the same open stream, assert it's
   `hello!!!`.
10. **Green:** wiring only — by this point steps 4/6/8 should already
    satisfy it; this cycle exists to catch any mismatch between the two
    handlers (e.g. header/flush timing) before it ships.

## Files to change

- `go.mod`, `go.sum` — new; module `github.com/andrelas6/ai-agent-go`, `go
  1.26`; adds `github.com/google/uuid`.
- `internal/session/registry.go` — new.
- `internal/session/registry_test.go` — new.
- `internal/httpserver/server.go` — new.
- `internal/httpserver/server_test.go` — new.
- `cmd/server/main.go` — new.

## Open questions

- Module path assumed as `github.com/andrelas6/ai-agent-go` (matching the
  GitHub user in the README's own-project link) — correct me if it should
  be something else before subtask 1.
- Session channel is unbuffered vs. small-buffered: I'll use a buffer of 1
  so a POST that arrives just as the SSE loop is mid-flush doesn't
  deadlock. Flag if you want different backpressure behavior — out of
  scope for "for now, per connection, simplicity" but worth naming.
- No test dependency (testify) added despite the reference repo using it —
  say now if you'd rather standardize on it from the start.

## Subtasks

Each item is one commit. I'll stop after each and wait for your review
before committing.

1. `go.mod` + `go get github.com/google/uuid` — module scaffold, no
   behavior, nothing to test yet.
2. `internal/session.Registry`: Create/Get, with the Red/Green from cycles
   1-2 above.
3. `internal/httpserver`: mux + POST unknown-session → 404 (cycles 3-4).
4. POST known-session echoes `message!!!` onto the channel (cycles 5-6).
5. GET `/sse` sends the session URL as the first event (cycles 7-8).
6. Full round-trip e2e test (cycles 9-10) — proves the two handlers work
   together over one open connection.
7. `cmd/server/main.go` entrypoint (`PORT` env var, default `8080`); verify
   manually with the `curl` steps from `docs/ae-init/async-server-echo.md`.
   At this point I'll total the diff and flag it if it's over ~500 lines
   (not expected — this slice should land well under that).
