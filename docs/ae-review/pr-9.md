# ae-review: PR #9 — feat: events stream resumes from Last-Event-ID

**problem being solved** before: a reconnecting `EventSource` gets the whole history again and shows duplicates; a closed bus channel would make the handler loop on zero values. after: `Last-Event-ID` resumes right after that event; a closed channel ends the response so the browser reconnects.

**opportunities to learn**
- comma-ok receive `event, ok := <-eventStream`: tells a closed channel apart from a real event. `cmd/agent-server/handlers/handlers.go:63`
- `slices.IndexFunc` + `slices.Clone`: find by predicate, then hand the subscriber a copy so the owner goroutine's slice never leaks. `internal/bus/bus.go:111-116`
- `r.Header.Get` canonicalises names, so `last-event-id` from any client still matches. `handlers.go:38`
- fake bus that records its argument (`fakeBus.lastEventID`) seeded with `"Subscribe not called"`: proves the call happened, not just the value. `handlers_test.go:563`

**error handling**
- non-empty id not found in history → silent full replay. client sees duplicates, on-call sees nothing. log once when `lastEventID != ""` and index is -1. `internal/bus/bus.go:112-116`
- handler returns on closed channel with no log; the server side can't tell "bus cut me off" from "client left". `handlers.go:64-66`

**null handling**
- nothing

**simplicity of the solution**
- `if lastEventIndex != -1 {…} else {…}` collapses to one line: `slices.Clone(history[lastEventIndex+1:])`, since -1+1 = 0. `internal/bus/bus.go:112-116`

**naming and language**
- `TestBusChannelCloseHandlerReturns` reads backwards; `TestEventsEndsWhenBusClosesStream` matches the `TestEvents…` tests around it. `handlers_test.go:532`
- `TestGetHandler…` prefix (3 tests) vs existing `TestEvents…` for the same route; pick one. `handlers_test.go:563,593,650`
- typos in comments: "throaway" `bus.go:49`, "lsatevent" `handlers_test.go`, "requset" `handlers_test.go`

**how to reproduce**
- `go run ./cmd/agent-server`; post 2 tasks; `curl -sN --max-time 1 localhost:8002/api/events` → note 2nd `id:`; `curl -sN --max-time 1 -H "Last-Event-ID: <id>" localhost:8002/api/events | grep -c '^id:'` → before 6, after 4

**comment usage**
- only your comments in this PR; not flagged (keep-your-comments rule). fix the typos above

**reuse of existing code**
- `postTask` added, but 5 other tests still hand-roll `http.Post` to `/api/tasks`; fixed later in #12

**code placement**
- `internal/bus` imported only by `cmd/agent-server` (`main.go`, handler tests); `agent-cli` doesn't use it. README says `internal/` = shared by both commands. keep only if the CLI will publish to the bus; else move to `cmd/agent-server/bus`

**also worth a look**
- `Subscribe(lastEventID string)` on the `EventBus` interface: every caller that doesn't resume passes `""`; fine for now, worth a second look if a third parameter shows up
