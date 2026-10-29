# ae-init: The harness in Go (the big picture)

> **Status:** Draft
> **Last updated:** 2026-10-29

## What to build

The agent harness from the Harness Engineering course, built in Go in this repository instead of in the course's TypeScript/Node.js. The course repo (`~/workspace/ai/courses/harness-engineering`) stays where it is, and we use one part of it: its **client**, the Harness Inspector web app (`web/`).

- **The course repo is the client.**
- **This repo is the server:** the harness, its event stream, and the agent loop.

The inspector shows each harness event live, and lets you submit a task and clear the log. It's pointed at our Go server instead of the course's Node server.

We follow the course lesson by lesson, but not one to one. These are the customisations:

- **Go, standard library only**, plus two packages: the OpenRouter SDK and `github.com/google/uuid` (`AGENTS.md`). The course uses Express, `ws`, the Vercel AI SDK and zod.
- **OpenRouter instead of OpenAI.** Any OpenRouter model, chosen by an env var. The course calls OpenAI through the AI SDK.
- **SSE plus POSTs instead of a WebSocket.**
  - Events stream over `GET /api/events`.
  - Tasks and clear are `POST /api/tasks` and `POST /api/clear`.
  - A reconnecting tab resumes from `Last-Event-ID`.
- **One agent loop, two front ends.**
  - `agent-server` runs the loop for the inspector.
  - `agent-cli` runs the same loop in the terminal, printing one readable line per event.
  - The course has only the server.
- **The course's event contract is kept.** Event JSON matches the course's `shared/events.ts`, so the inspector doesn't need to know it's talking to Go.
- **Minimal client changes**, made on a local branch of the course repo with no tests:
  - `useHarnessSocket.ts` reads with `EventSource`;
  - two URLs in `TaskPane.tsx` become relative;
  - `vite.config.ts` proxies `/api` to `:8002`.

## A -> B

- **A (course as shipped):** `npm run dev` starts the Node harness on `:8787` and the inspector on `:5173`. The inspector opens a WebSocket to `:8787` and sends tasks over it. The agent runs in TypeScript on OpenAI.
- **B (what we're building):** only the inspector runs from the course repo. Vite proxies `/api/*` to the Go `agent-server` on `:8002`. Events come back over SSE, and tasks go in as POSTs. The agent runs in Go on OpenRouter, and the same loop also runs from `agent-cli`.

### Visual

```text
A (course as shipped)
  browser :5173 (course inspector)
     ├── WebSocket ws://localhost:8787/ws ──▶ course Node server (TS, AI SDK, OpenAI)
     │     ◀── events, one per message
     └── submit_task / clear over the same socket

B (what we're building)
  browser :5173 (course inspector, 3 files changed)                ◀ changed
     ├── GET  /api/events  (SSE) ─┐                                ◀ new
     ├── POST /api/tasks {input} ─┼─ Vite proxy ──▶ ai-agent-go agent-server :8002   ◀ new
     └── POST /api/clear ─────────┘                 (Go, OpenRouter)
           ◀── id: <uuid> / data: {event JSON, same shape as shared/events.ts}

  terminal
     └── go run ./cmd/agent-cli  ◀── task on stdin
           ──▶ same agent loop ──▶ one readable line per event      ◀ new
```

## How to reproduce

**Preconditions**
- The course repo at `~/workspace/ai/courses/harness-engineering` with `npm install` done.
  - **A:** check out `origin/lesson-1`, the course's own branch, and put `OPENAI_API_KEY` in `.dev.vars`.
  - **B:** the local `lesson-1` branch, which has our inspector change (`98138c1`).
- **B also needs** this repo on `main`, and `OPENROUTER_API_KEY` set once the real model is wired in (lesson 1, subtask 7).

**Steps**
1. **A:** in the course repo, run `npm run dev`.
   **B:** in the course repo, run `npm run dev:web`; in this repo, run `go run ./cmd/agent-server`.
2. Open http://localhost:5173 with the DevTools Network tab open.
3. Submit a task, for example `hello`.
4. **B only:** run `curl -N localhost:8002/api/events` in a terminal while the task runs.
5. **B only, once lesson 1 is done:** pipe the task into `go run ./cmd/agent-cli`.

**Observe**

| | A (course as shipped) | B (what we're building) |
|---|---|---|
| Network tab | one WebSocket to `:8787` | `GET /api/events` (EventSource) plus `POST /api/tasks` on `:5173` |
| Which server answers | Node on `:8787` | Go `agent-server` on `:8002`, through the proxy |
| Event pane after a task | `workflow.started` → "no agent yet" log → `workflow.completed` | the same three events today, from `runStub`. After lesson 1: the real loop's tool and model events |
| `curl -N :8002/api/events` | connection refused | `id:` / `data:` lines arriving as events are emitted |
| `agent-cli` | doesn't exist | the same events, one readable line each |

## Open questions

- **Later lessons:**
  - Lesson 2 (durable execution) and lesson 3 (sandboxing) lean on TypeScript-specific tooling in the course.
  - The Go equivalent, which has to fit the standard-library rule, is decided per lesson in that lesson's ae-init.
- **Client changes beyond lesson 1:** `web/` is the same on every course branch today. If a later lesson changes it, we redo our three-file change on that lesson's branch.
