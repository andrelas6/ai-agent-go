# ae-review: Lesson 1 harness (lesson-1-harness, andrelas6/ai-agent-go#7)

**problem being solved** before: inspector's `EventSource` gets 404 on `/api/events`, tasks return a dummy id, nothing streams. after: Go server streams course-shaped events over SSE, with history replay, Clear, UUID workflow ids and the course's stub agent. PR #7 body is empty, so this comes from the ae-init, not the PR.

**opportunities to learn**
- Owner goroutine ("share memory by communicating"): one goroutine owns `streams` and `history`, and everyone else sends it requests. No mutex. `internal/bus/bus.go:76`
- Request/reply over a channel: `Subscribe` sends a buffered `reply` channel, and the owner registers and answers in one `case`, so history and live can't overlap. `bus.go:57`, `bus.go:97`
- Non-blocking send with `select`/`default`: a full subscriber can't stall the owner. `bus.go:92`
- Every send to the owner also selects on `<-e.done`, so callers can't hang after shutdown. `bus.go:31`, `bus.go:41`
- `http.ResponseController.Flush` right after the headers: `EventSource.onopen` fires before any event. `handlers/handlers.go:49`
- Go 1.22 method-and-path patterns (`"POST /api/tasks"`) give 405 and 404 for free. `handlers.go:27`
- Handler factories close over their dependencies; `b.Publish` is passed as a method value, so the runner never sees the bus. `handlers.go:107`
- The server context passed to a background goroutine, not `r.Context()`, and a test that proves it. `handlers_test.go` `TestRunnerRunsOnTheServerContext`
- Compile-time interface checks: `var _ handlers.EventBus = (*fakeBus)(nil)`
- `json.RawMessage` plus `omitempty` keep the JSON identical to the course's TS union. `internal/events/events.go`
- Tests worth copying:
  - `t.Context()`;
  - table tests with `t.Run`;
  - a `select` with `time.After` to show something did *not* happen;
  - `t.Cleanup(server.Close)` so the stream closes before the server.

**error handling**
- `log.Fatalf` on every Ctrl-C. `ListenAndServe` always returns `ErrServerClosed` after `Shutdown`, so the process exits with status 1 and "error starting the server", possibly before `Shutdown` finishes. Check `errors.Is(err, http.ErrServerClosed)`. `cmd/agent-server/main.go:28`
- `server.Shutdown(shutdownCtx)` error ignored. A timeout, for example a stuck handler, goes unnoticed. `main.go:37`
- Dropped events are silent. When a tab's buffer is full, `default:` does nothing, with no log and no counter, so nobody knows a tab is missing events. At least `log.Printf` with the subscriber id. `bus.go:94`
- `flushEventData` returns an error that both callers ignore, and the `fmt.Fprintf` write errors aren't checked. A dead connection keeps looping until a ctx ends. Drop the return, or stop the loop on a write error. `handlers.go:52`, `handlers.go:64`, `handlers.go:79`
- `writeJSON` ignores the `Encode` error. `handlers.go:123`
- `go runner(...)` has no `recover`. A panic in the runner kills the whole server, every tab included. That's harmless with the stub, but it matters once `agent.Run` arrives. `handlers.go:107`
- The decode error logs at the same level as real problems: every bad client request writes a log line. The 400 already tells the client; consider dropping the log. `handlers.go:96`

**null handling**
- After shutdown, `Subscribe` returns `nil, nil, func() {}`. The handler then selects on a `nil` channel and only survives because `ctx.Done()` is already closed. Fine today; it only works because of that. `bus.go:56`
- Otherwise nothing: the request body is checked once at the edge, and the events use value types.

**simplicity of the solution**
- The `Bus.eventStreams` field is never read: the owner uses its local `streams`. Remove the field and line 109. `bus.go:23`
- A uuid per subscriber only to key the map. Key it by the channel (`map[chan events.Event]struct{}`) and `subscription.id`, `channelId` and `unsubscribeCh chan string` go away. `bus.go:52`
- `rc.Flush()` before `return` on both `Done` branches does nothing. `handlers.go:58`, `handlers.go:61`
- `defer r.Body.Close()`: the server closes the request body itself. `handlers.go:94`
- `newStubRunner()` returns a closure that captures nothing. A plain `func runStub(ctx, workflowID, input, emit)` already fits `handlers.Runner`. `cmd/agent-server/stub.go:9`
- `newTestMux` only reorders `NewMux`'s arguments; call `NewMux` directly. `handlers_test.go:352`
- `Mode` is decoded and never used. That's fine until Lesson 6, but leave it out of the struct until then. `handlers.go:90`

**naming and language**
- The receiver `e *Bus` reads as "event". Use `b`. `bus.go:31`
- `channelId`, `streamId` → `channelID`, `streamID` (Go initialisms). `bus.go:52`, `bus.go:79`
- Mixed suffixes for the same kind of thing: the `subscriptionStream` field holds the `subscriptionCh` local; `publishCh`, `clearCh`, `eventStream`. Pick one, for example `subscribe`, `publish`, `clear`, `unsubscribe`. `bus.go:25`
- `readStream` (the named return in the interface and in `Subscribe`) vs `live` in the tests. `live` says what it is. `handlers.go:19`
- `handleGetStreamingEvents` → `handleEvents`, after the route. `handlers.go:37`
- `fakeRunnerReply` is a recorded call, not a reply. Use `runCall`. `handlers_test.go:19`
- `TestEventBus` says nothing about what it checks, and `TestPublishDoesNotBlock` now also checks that a healthy subscriber gets every event in order. `bus_test.go`
- The PR title "Lesson 1 harness" and the empty body: say what changes (SSE events, bus, stub agent).

**how to reproduce**
- Not in the PR. From the ae-init:
  - `npm run dev:web` in the course repo on `lesson-1`, and `go run ./cmd/agent-server`;
  - open `:5173` and `curl -N localhost:8002/api/events`, then submit `hello`;
  - expect `workflow.started` → a `log` warn → `workflow.completed`, all with the 202's `workflowId`;
  - a second tab replays them, and Clear empties them;
  - `curl -i -X POST localhost:8002/api/tasks -d '{"input":""}'` → 400;
  - `/sse` and `/messages` → 404.

**comment usage**
- nothing to change. The comments are the author's own notes, kept on purpose.

**reuse of existing code**
- None found. The repo had no bus or SSE code to reuse: `internal/messages` was its predecessor and is deleted here. `analysis/crush` is reference material, not imported.

**code placement**
- `internal/bus` is imported only by `cmd/agent-server` (`main.go` and the handler tests). That's the plan's deliberate exception, but the PR doesn't say why. Note it in the PR, or move it into `cmd/agent-server`.
- `cmd/agent-server/handlers` is a separate package imported only by `main.go`, which makes every name exported (`NewMux`, `EventBus`, `Runner`) for one caller. The plan accepted this. Fine, but `package main` would drop the exports.
- `internal/events` is shared by the bus, the handlers and the stub, and `agent-cli` will use it soon. Its place fits.

**also worth a look**
- `Connection: keep-alive` is forbidden in HTTP/2; Go drops it there. The test asserts it, so it would fail behind an HTTP/2 proxy. Drop the header and its check. `handlers.go:44`, `handlers_test.go:78`
- Size: +1365/−94 in one PR. The plan suggested splitting it after the bus work (PR 1a and PR 1b). The commits are clean, so reviewing commit by commit is the fallback.
- The bus tests that use `bus.New(t.Context())` together with a deferred `unsubscribe` hang, rather than fail, if the owner ever gets stuck. `context.WithTimeout`, as the newer tests use, avoids that. `bus_test.go` (`TestEventBus` and others)
