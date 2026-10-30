# ae-plan: A bus you can rely on

> **Status:** Done
> **Last updated:** 2026-10-27 (all 6 subtasks done)
> **Progress:** 6 of 6 subtasks done. PR 1 merged as [andrelas6/ai-agent-go#9](https://github.com/andrelas6/ai-agent-go/pull/9); PR 2 merged as [andrelas6/ai-agent-go#10](https://github.com/andrelas6/ai-agent-go/pull/10); PR 3 merged as [andrelas6/ai-agent-go#11](https://github.com/andrelas6/ai-agent-go/pull/11); PR 4 merged as [andrelas6/ai-agent-go#12](https://github.com/andrelas6/ai-agent-go/pull/12)
> **Owner:** André builds it
> **From:** ae-init/reliable-bus.md (parent: ae-plan/lesson-1-harness.md, subtask 4)

## What's being built

The rest of subtask 4:
- `GET /api/events` resumes from `Last-Event-ID`, so a reconnecting tab gets only what it missed.
- A tab that falls 256 events behind is cut off: its stream ends, and the browser reconnects and catches up, instead of losing events silently.
- History keeps the last 1000 events.
- `Subscribe` after shutdown returns a closed channel, not `nil`.

Why: today a reconnect shows every event twice in the inspector, a slow tab loses events without knowing, and history grows forever.

## Constraints

- Standard library plus the OpenRouter SDK and `google/uuid` only (`AGENTS.md`).
- `internal/bus` stays where it is (the deliberate exception). Handlers stay in `cmd/agent-server/handlers`.
- Keep the owner-goroutine design in `bus.go`: no mutex.
- Red/Green TDD for all Go changes. `go test -race ./...` passes after every commit.
- Keep André's comments in `bus.go`, `handlers.go` and the tests untouched. That includes `bus.go:88` ("if buffer is full, drop events") and the TODO at `bus.go:89`: after subtask 3 they describe the old behaviour, and André edits them himself if he wants.
- No inspector change. `EventSource` sends `Last-Event-ID` by itself, and the Vite proxy forwards it (checked: `curl -H 'Last-Event-ID: abc-123' localhost:5173/api/events` arrives at `:8002` with `last-event-id: abc-123`).
- Separate from the agent-loop work (subtasks 5–8 of the parent plan). The bus work and the test-helper refactor go in separate PRs (see Size under Subtasks).

## What I found in the codebase

- **`internal/bus/bus.go`:**
  - `Subscribe()` takes no arguments. It registers the channel and replies with `slices.Clone(history)` in the same `select` case, so history and live never overlap. Resume has to happen in that same case, or that guarantee breaks.
  - The publish case (`bus.go:83-94`) appends to `history` with no limit, then does a non-blocking send to each stream with an empty `default:`. That `default:` is where the cut-off goes.
  - The unsubscribe case already checks `streams[ch]` before `close(ch)`. A stream the bus has already cut off is gone from the map, so a later `unsubscribe()` won't close it twice.
  - After shutdown, `Subscribe` returns `nil, nil, func() {}` (`bus.go:51-52`).
- **`cmd/agent-server/handlers/handlers.go`:**
  - `EventBus.Subscribe()` is the interface the handler owns, and `handleEvents` calls it with no id.
  - The live loop reads `event := <-eventStream` without `ok`. Once the bus closes a channel, that read returns zero-value events forever, and the handler would write empty events in a tight loop. **The handler has to stop on a closed channel before the bus starts closing channels.** That fixes the order of the subtasks.
- **Tests:**
  - `bus_test.go`: external `bus_test` package, `context.WithTimeout` per test, `select` + `time.After` to show something didn't happen, helpers `eventMessages`/`eventTypes`. `TestSlowSubscriberDoesNotBlockOthers` already has one subscriber that never reads; it keeps passing once that subscriber is cut off.
  - `handlers_test.go`: `fakeBus` (history, live channel, `unsubscribed` signal) for the disconnect, shutdown and broken-write tests; the real bus for replay and Clear. Helpers `openEventStream`, `readEvent`, `readSSEMessage`.
  - Every `Subscribe()` call in both test files (and `fakeBus.Subscribe`) changes signature in subtask 1.
- **`README.md:11`** says the stream "replays the history first, then sends live events". It needs a line about `Last-Event-ID` and the 1000 cap.
- **Inspector `useHarnessSocket.ts`** appends every message. With resume working, that's correct as is.

## What the docs say

- **WHATWG HTML, [Server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html):**
  - When the response body ends normally, the browser **reestablishes the connection**. So ending the handler is enough to make a cut-off tab reconnect.
  - On reconnect, `Last-Event-ID` is sent only if the last event id is non-empty, with that id as its value. Every event we send has a UUID `id:`, so a tab that got at least one event always sends it.
  - A non-200 status or a wrong `Content-Type` **fails the connection for good**: no reconnect. A cut-off must end the 200 stream, never answer with an error.
  - The reconnect delay is up to the browser ("a few seconds"); the `retry:` field changes it. Not needed here.
- **MDN, [Using server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events):** "if the connection between the client and server closes, the connection is restarted", unless the page calls `.close()`.
- **Go, `net/http.Header.Get`:** case-insensitive, returns `""` when the header is missing. That gives "no header" and "empty header" the same full-replay path for free.

## Approach

**Handler.** `handleEvents` reads `r.Header.Get("Last-Event-ID")` and passes it to `bus.Subscribe(lastID)`. The live loop reads `event, ok := <-eventStream` and returns when `ok` is false. The response then ends normally, and `EventSource` reconnects.

**Bus, all inside the owner goroutine:**
- **Resume.** The subscribe case looks for `lastID` in `history`. If it finds it, it replies with the events after it. If it doesn't (empty id, after a Clear, or pushed out by the cap), it replies with the whole history. Same `select` case as the registration, so the no-overlap guarantee holds.
- **Cut-off.** In the publish loop, the `default:` branch deletes the stream from `streams` and closes it. The events already in its buffer still reach the handler before the close, so the tab's last id is the last event it got.
- **Cap.** `const HistoryLimit = 1000`. After the append, if `history` is longer than that, drop the oldest.
- **After shutdown.** `Subscribe` returns `nil`, an already-closed channel, and a no-op `unsubscribe`.

**Server log.** The bus logs one line when it cuts a subscriber off (`log.Printf`), so step 2 of the ae-init shows up in the server terminal. This answers the PR 7 review's "dropped events are silent". It isn't tested. Decided with André on 2026-10-27.

What this doesn't solve: a tab more than 1000 events behind reconnects with an id that's no longer in history and gets the full 1000 again. It may see duplicates and a gap. With the stub (3 events per task) and the Lesson 1 agent (tens of events per task), that needs hundreds of tasks while one tab is stuck. Accepted.

Also not solved: the cut-off frees the bus, not the handler. If a client never reads again, its handler goroutine stays blocked in `Write` until the OS gives up on the connection, since there's no write deadline. The curl check still works, because `kill -CONT` unblocks the write.

## Red / Green

**Handler**
- The `Last-Event-ID` request header reaches `Subscribe` unchanged, and a request without it passes `""` (fake bus records the id).
- When the bus closes the live channel, the handler returns and unsubscribes; the response body hits EOF (fake bus closes `live`).
- With the real bus: a stream opened with `Last-Event-ID: <id of event 1>` gets event 2 first, not event 1.

**Bus**
- `Subscribe(id of event 2)` with events 1–4 in history returns events 3 and 4.
- `Subscribe(id of the last event)` returns no history, and live events still arrive.
- `Subscribe("")` and `Subscribe("unknown")` return the full history.
- After a Clear, the old id is unknown and the reply is the full (new) history.
- A subscriber that stops reading gets exactly `SubscriberBuffer` events, then its channel is closed. Publish never blocks, and another subscriber still gets every event in order.
- Calling `unsubscribe()` after a cut-off doesn't panic (no double close).
- After publishing 1001 events, a new subscriber's history has 1000, starting at event 2.
- An id pushed out by the cap gives the full 1000.
- After shutdown, `Subscribe` returns a closed channel: `_, ok := <-live` gives `ok == false` straight away.
- Everything passes `go test -race ./...`.

**By hand (from the ae-init)**
- curl step 1 gives 6 / 4 / 6 (full, after the 2nd id, unknown id); the stopped curl exits and the follow-up with `Last-Event-ID` returns the rest; a fresh replay holds 1000.
- Two inspector tabs: after a forced reconnect in tab 2 (DevTools → Offline and back), both panes show the same rows, each once.

## Most important file changes

- `cmd/agent-server/handlers/handlers.go`: `EventBus.Subscribe(lastID string)`; read the header; stop on a closed channel.
- `cmd/agent-server/handlers/handlers_test.go`: `fakeBus` records `lastID` and can close `live`; new resume and closed-channel tests.
- `internal/bus/bus.go`: `Subscribe(lastID string)`; resume, cut-off, `HistoryLimit`, closed channel after shutdown.
- `internal/bus/bus_test.go`: `Subscribe("")` in existing tests; new tests for resume, cut-off, cap, shutdown.
- `README.md`: one line on `Last-Event-ID` resume and the 1000-event history.
- `cmd/agent-server/handlers/handlers_test.go` (subtask 6): shared helpers for the server, the events stream, posting tasks, a bus with events, and waiting on a channel.

## Open questions

- **Is the DevTools Offline reconnect real?** Still not checked by hand (from the ae-init). If Offline doesn't drop an open `EventSource`, the two-tab check needs another trigger.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit.

1. ✅ **Done (`142bd1d`).** **Handler reads `Last-Event-ID` and ends the stream on a closed channel.** *Entry point, bus mocked.*
   - `EventBus.Subscribe(lastID string)`. `handleEvents` passes `r.Header.Get("Last-Event-ID")`, and the live loop returns on `!ok`.
   - Red: `fakeBus` records the id it was given, and a test closes `fake.live` and expects the handler to return, unsubscribe and end the body.
   - `bus.Subscribe` takes the new `lastID` argument and ignores it for now (still the full history). Existing tests call `Subscribe("")`.
   - **When this lands:** nothing changes from outside. The handler is ready for a bus that closes channels.
2. ✅ **Done (`46e82fb`).** **The bus resumes after `lastID`.**
   - Red: bus tests for "after id 2", "after the last id", empty, unknown, and after Clear. Then a handler test with the real bus and a `Last-Event-ID` header.
   - **When this lands:** ae-init step 1 gives 6 / 4 / 6. A reconnecting tab gets only what it missed.
3. ✅ **Done (`0b1a690`).** **The bus cuts off a full subscriber.**
   - Red: a subscriber that never reads gets `SubscriberBuffer` events and then a closed channel; another subscriber gets all events in order; `unsubscribe()` after the cut-off doesn't panic.
   - Replace the empty `default:` with delete + close, and log one line.
   - **When this lands:** ae-init step 2: the stopped curl exits once resumed, and a curl with its last id gets the rest.
4. ✅ **Done (`5330383`).** **History keeps the last 1000 events.**
   - Red: 1001 events give 1000 in history, starting at event 2; an id pushed out by the cap gives a full replay.
   - **When this lands:** ae-init step 3: a fresh replay after 24,006 events holds 1000.
5. ✅ **Done (`2b3dcc8`).** **`Subscribe` after shutdown returns a closed channel.**
   - Red: cancel the bus context, `Subscribe("")`, read from `live` and expect `ok == false` within a second.
   - Update `README.md:11` with the resume and the cap.
   - Run the ae-init's manual checks (curl steps 1–3, two inspector tabs, Clear then reconnect).
   - **Manual checks, run 2026-10-27 on `2b3dcc8`:**
     - Step 1: 6 / 4 / 6, as expected.
     - Step 2: the stopped curl exits (2,502 events, then the stream ends), and the server logs one cut-off line. The follow-up with the last id got the full 1000, not "the rest": that id was event ~2,502 of 24,006, already pushed out by the cap. This is the accepted limit under Approach, so step 2's "the two files together hold every event once" only holds when the tab is fewer than 1000 events behind.
     - Step 3: a fresh replay holds 1000.
     - Two inspector tabs, and Clear then reconnect: not run (need the browser and the inspector repo). The DevTools Offline question is still open.
6. ✅ **Done** (`c56a8c1`). **Shared helpers in `handlers_test.go`.** *Refactor only, in its own PR.*
   - Today the file repeats the same setup in almost every test: `httptest.NewServer(handlers.NewMux(...))` plus `Close` 17 times, a hand-built `GET /api/events` 11 times, `http.Post` to `/api/tasks` or `/api/clear` 6 times, decoding `workflowId` 3 times, a `select` with `time.After` 8 times, and a status-code check 13 times.
   - Helpers, built from what's already there (`postTask`, `openEventStream`, `readEvent`, `readBody`):
     - `newServer(t, ctx, bus, runner)`: starts the server and closes it with `t.Cleanup`; returns the URL.
     - `getEvents(t, ctx, url, lastEventID)`: opens `GET /api/events`, sets `Last-Event-ID` when it's not empty, fails on an error or a non-200 status, and closes the body with `t.Cleanup`. `openEventStream` becomes a wrapper around it.
     - `postTask(t, ctx, url, body)`: `t` first and `t.Helper()`, like the other helpers; takes the body so the 400 cases and `{"input":"hello"}` tests can use it too; checks the `NewRequest` error it ignores today. A `postTaskID` variant returns the decoded `workflowId`.
     - `busWithEvents(t, ctx, messages...)`: publishes the messages and returns the bus plus its history, for the resume tests.
     - `waitFor(t, ch, what)`: a generic `select` on a channel with a 1s `time.After`, for the "runner was called" and "unsubscribed" checks.
   - Keep André's comments, moved with the code they describe. Don't change what any test checks.
   - No Red step: this is a refactor, so the proof is that every test still passes, unchanged, with `go test -race -count=3 ./...`, and that `gofmt`/`go vet` stay clean.
   - **When this lands:** nothing changes from outside. The file is shorter, and a new handler test is a few lines of setup.
   - **What was built:** the file is 137 lines added and 291 removed. Differences from the list above:
     - `postTaskID` became `decodeWorkflowID(t, res)`, so `TestGetStreamingEvents` can still check the status code and `Content-Type` on the same response.
     - A plain `post(t, ctx, url, body)` sits under `postTask`, and the two `/api/clear` calls use it.
     - Not shared: the inverse wait in the 400 test (it expects nothing within 100ms), and the send-`select` in `TestEventsStopsWhenWritingFails`.
   - **Checks:** `go test -race -count=3 ./...` passes; `gofmt` and `go vet` are clean; the comments are line-for-line the same set as before. In a scratch copy where the handler ignores `Last-Event-ID`, the three resume tests fail as they should.

**Size.** Subtasks 1–2 came to about 420 lines changed, so the work is split into four PRs. Each one leaves the server runnable.
- **PR 1 (merged, [andrelas6/ai-agent-go#9](https://github.com/andrelas6/ai-agent-go/pull/9)):** subtasks 1–2, the handler and the resume.
- **PR 2 (merged, [andrelas6/ai-agent-go#10](https://github.com/andrelas6/ai-agent-go/pull/10)):** subtasks 3–4, the cut-off and the history cap.
- **PR 3:** subtask 5, the closed channel after shutdown, plus the README update and the manual checks.
- **PR 4:** subtask 6, the test helpers. Keeping a refactor out of the feature PRs makes all of them easier to review.
