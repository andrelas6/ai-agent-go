# AI AGENT GO

A small agent runtime in Go. It runs an LLM-driven agent loop and makes every step visible.

The model only picks the next step. The runtime does everything else: it runs the tools, keeps the state, and reports each step as an event. Over time it will also cover the things that make an agent hold up in real use: surviving crashes, sandboxed tools, context management, multiple agents, and human approval before risky actions.

## What's here

- **`cmd/agent-server`** is an HTTP server.
  - `POST /api/tasks` starts a task. `POST /api/clear` empties the event history.
  - `GET /api/events` streams every event over SSE. It replays the history first, then sends live events.
- **`cmd/agent-cli`** runs the same agent loop from the terminal.
- **`internal/`** holds the code both commands share: the event types (`events`) and the in-memory event bus (`bus`).
- **`docs/`** has the design notes, plans and reviews.

The agent behind `/api/tasks` is a stub for now. It emits `workflow.started`, a log line and `workflow.completed`. The real loop is next: OpenRouter for the model, plus a few tools.

## Run it

```bash
go run ./cmd/agent-server            # listens on :8002
curl -N localhost:8002/api/events    # watch the event stream
curl -X POST localhost:8002/api/tasks -d '{"input":"hello"}'
```

## Rules

- Standard library only, plus two packages: the OpenRouter SDK and `github.com/google/uuid` (see `AGENTS.md`).
- Code goes where its users are. Code only one command uses lives in that command's folder; code both commands use lives in `internal/`.
- Go code is built test first.

## Design

[Design Canvas](https://app.excalidraw.com/s/7s9vhE83N90/4Dogyck9z2F)
