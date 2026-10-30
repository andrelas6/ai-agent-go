# ae-init: A bus you can rely on

> **Status:** Reviewed
> **Last updated:** 2026-10-24
> **From:** ae-plan/lesson-1-harness.md, subtask 4 ("Left" list)

## What to build

The rest of subtask 4: a tab that loses the stream gets back exactly what it missed, once. Today a reconnecting tab gets the whole history again, so the inspector shows every event twice. A tab that falls behind silently loses events and never knows. After this change, a tab that falls behind is cut off on purpose. Its browser reconnects by itself, says the last event it saw, and gets only the events after that. History stops growing without limit (it keeps the last 1000 events), and a subscribe that arrives after shutdown gets a closed stream instead of nothing.

## A -> B

- **A (now):** `GET /api/events` ignores `Last-Event-ID`: every connect replays all history, so a reconnecting tab shows duplicates. A tab 256 events behind silently loses events and its stream stays open. History is unbounded. After shutdown, `Subscribe` returns a `nil` channel.
- **B (after):** `Last-Event-ID: <uuid>` replays only the events after it; an empty or unknown id replays everything. A tab 256 behind is cut off: its stream ends, it reconnects with `Last-Event-ID`, and catches up with no gap and no duplicates. Replay holds at most 1000 events.

Grounded in the code on `main` (`0d99b08`):
- `bus.Subscribe()` takes no id, and `handleEvents` never reads the request headers (`cmd/agent-server/handlers/handlers.go:38`).
- The owner goroutine's publish case does a non-blocking send with an empty `default:`, so a full tab loses that event (`internal/bus/bus.go:87-93`, with the TODO about disconnecting).
- `history = append(history, event)` with no limit (`bus.go:86`); only `Clear` empties it.
- `Subscribe` returns `nil, nil, func() {}` once the context is done (`bus.go:51-52`). The handler only survives because it also selects on `ctx.Done()`.
- The inspector appends every message it gets (`useHarnessSocket.ts`, `setEvents((prev) => [...prev, event])`), so a full replay on reconnect shows as duplicate rows.

### Visual

```text
Resume after a reconnect (history: e1 … e6, tab last saw e2)

A (now)
  tab ── GET /api/events  Last-Event-ID: e2 ──▶ server ──▶ e1 e2 e3 e4 e5 e6, then live
                                                           └─┬─┘
                                                     duplicates in the pane

B (after)
  tab ── GET /api/events  Last-Event-ID: e2 ──▶ server ──▶ e3 e4 e5 e6, then live     ◀ changed
  tab ── GET /api/events  Last-Event-ID: ??  ──▶ server ──▶ e1 … e6, then live         (unknown id: full)

A tab that stops reading while the agent publishes

A (now)
  server ──▶ slow tab:  e1 … e250, e9000, e9001 …   stream stays open
                                  └─ gap, silently lost

B (after)
  server ──▶ slow tab:  e1 … e256  ▌ stream ends                                       ◀ new
  tab ── GET /api/events  Last-Event-ID: e256 ──▶ server ──▶ e257 … now, then live     ◀ new

Replay size after 24,006 events

A (now)   fresh GET /api/events ──▶ 24,006 events
B (after) fresh GET /api/events ──▶ last 1,000 events                                   ◀ changed
```

Subscribe after shutdown: nothing to draw. It is not visible from outside, because the handler already returns on `ctx.Done()`. Same behaviour as A, but safe without that guard.

## How to reproduce

**Preconditions**
- `main` at `0d99b08` (A) or the branch with the change (B).
- `go run ./cmd/agent-server` on `:8002`, and nothing else on that port (`lsof -i :8002`).
- For the browser checks: the inspector repo on `lesson-1`, `npm run dev:web` (`:5173`, proxies `/api` to `:8002`).
- No OpenRouter key needed: `POST /api/tasks` runs the stub, which publishes 3 events per task.

**Steps**

1. **Resume by id.** Post two tasks, read the stream, and resume from the 2nd id:
   ```bash
   for i in 1 2; do curl -s -X POST localhost:8002/api/tasks -d '{"input":"hi"}'; done
   curl -sN --max-time 1 localhost:8002/api/events > full.txt; grep -c '^id:' full.txt
   ID2=$(grep '^id:' full.txt | sed -n 2p | cut -c5-)
   curl -sN --max-time 1 -H "Last-Event-ID: $ID2" localhost:8002/api/events | grep -c '^id:'
   curl -sN --max-time 1 -H "Last-Event-ID: nope" localhost:8002/api/events | grep -c '^id:'
   ```
2. **A tab that falls behind (curl).** Start a stream, stop the curl process, flood the bus, resume it:
   ```bash
   curl -sN localhost:8002/api/events > slow.txt & CP=$!; sleep 0.5; kill -STOP $CP
   seq 1 8000 | xargs -P 16 -I{} curl -s -o /dev/null -X POST localhost:8002/api/tasks -d '{"input":"x"}'
   kill -CONT $CP; sleep 3; ps -p $CP >/dev/null && echo running || echo exited
   grep -c '^id:' slow.txt
   ```
   A small burst is not enough: on macOS, 1,500 tasks (~880 KB) fit in the loopback socket buffers and nothing was lost. 8,000 tasks (24,000 events) fill them. In B, follow up with `curl -sN -H "Last-Event-ID: <last id in slow.txt>"` to get the rest.
3. **History cap.** After step 2, count a fresh replay, with enough time for it to finish:
   ```bash
   curl -sN --max-time 8 localhost:8002/api/events | grep -c '^id:'
   ```
4. **Two inspector tabs, reconnect.** Open `http://localhost:5173` in two tabs and submit a task in tab 1. In tab 2, force a reconnect while the server keeps its history: DevTools → Network → throttling **Offline**, wait for the indicator to go disconnected, then back to **No throttling**. Submit another task. Compare the two panes, and look at the reconnect request in tab 2's Network tab.
   A reload is not this check: a fresh `EventSource` sends no `Last-Event-ID`, so it gets a full replay in both A and B. Restarting Vite is not either: Vite's client reloads the page when its server comes back.
5. **Clear, then reconnect.** Click Clear, then repeat the reconnect from step 4 in tab 2. Its last id is no longer in history.
6. **Subscribe after shutdown.** Go-level only: subscribe on a bus whose context is cancelled and read from the channel.

**Observe**

| | A (now, checked on `0d99b08`) | B (after) |
|---|---|---|
| Step 1: full replay | 6 | 6 |
| Step 1: `Last-Event-ID` = 2nd id | 6 (header ignored) | 4 |
| Step 1: `Last-Event-ID: nope` | 6 | 6 |
| Step 2: slow curl after resume | still running; 2,505 of 24,006 events, with a gap in the ids | exited (stream ended); the follow-up curl with `Last-Event-ID` returns the rest, so the two files together hold every event once |
| Step 3: fresh replay count | 24,006 | 1,000 |
| Step 4: tab 2's reconnect request | carries `Last-Event-ID: <uuid>` (proxied through Vite) | same |
| Step 4: tab 2's pane after reconnect | every earlier event appears twice | same rows as tab 1, each once |
| Step 5: tab 2 after Clear + reconnect | the post-Clear history, appended after the old rows | same: unknown id gives the full (post-Clear) history |
| Step 6: read from the channel | `nil` channel: a read blocks forever | closed channel: `_, ok := <-ch` gives `ok == false` at once |
| Server behaviour on Ctrl-C | handler exits via `ctx.Done()` | same |

## Open questions

- **Step 4 not yet run by hand.** Does DevTools Offline end an open `EventSource` and trigger a reconnect with `Last-Event-ID` through the Vite proxy? The A and B cells for step 4 are expected, not observed. If Offline doesn't drop the stream, ae-plan needs another way to force a reconnect.
- **A browser tab probably never falls 256 behind.** It drains the socket even when idle, so the cut-off itself is observed with curl (step 2). The tabs show the resume without duplicates (step 4), not the cut-off. If you want to see a cut-off in the browser, ae-plan has to find a way (a breakpoint in `onmessage` might do it; untested).
- **Should a cut-off log a line on the server?** The PR 7 review flagged silent drops. A log line would make step 2 visible in the server terminal too. That's a design choice for ae-plan.
- **Step 5 in B:** after a Clear, the reconnecting tab gets the post-Clear history appended to its old rows. The inspector's Clear button only POSTs `/api/clear` (`TaskPane.tsx:215`) and never empties its own list, so the panes keep their rows in both A and B. That matches "unknown id → full replay" and isn't a duplicate. It also means the parent ae-init's "both panes empty" after Clear doesn't hold today; that's an inspector change, outside this subtask.
