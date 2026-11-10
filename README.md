# AI AGENT GO

A small agent runtime in Go. It runs an LLM-driven agent loop and makes every step visible.

The model only picks the next step. The runtime does everything else: it runs the tools, keeps the state, and reports each step as an event. Over time it will also cover the things that make an agent hold up in real use: surviving crashes, sandboxed tools, context management, multiple agents, and human approval before risky actions.

## What's here

- **`cmd/agent-server`** is an HTTP server.
  - `POST /api/tasks` starts a task. `POST /api/clear` empties the event history.
  - `GET /api/events` streams every event over SSE. It replays the history first, then sends live events.
    - The history keeps the last `bus.HistoryLimit` events (see `internal/bus/bus.go`).
    - A reconnecting client that sends `Last-Event-ID` gets only the events after that one. An empty or unknown id gets the whole history.
    - A client that falls `bus.SubscriberBuffer` events behind has its stream ended. The browser reconnects and catches up.
- **`cmd/agent-cli`** runs the same agent from the terminal. It reads the task from stdin and prints one line per event.
- **`internal/`** holds the code both commands share:
  - `agents`: the agent loop, and the support agent built on it;
  - `tools`: the support agent's tools;
  - `model`: the OpenRouter model;
  - `events` and `bus`: the event types and the in-memory event bus.
- **`docs/`** has the design notes, plans and reviews.

The agent is a support triage agent. It classifies each work item, searches a small knowledge base, drafts a reply and sends it.

## Run it

Both commands need an OpenRouter key. `OPENROUTER_MODEL` is optional and defaults to `z-ai/glm-5.3-flash`.

```bash
export OPENROUTER_API_KEY=...
```

**The server:**

```bash
go run ./cmd/agent-server            # listens on :8002
curl -N localhost:8002/api/events    # watch the event stream
curl -X POST localhost:8002/api/tasks -d '{"input":"hello"}'
```

- Without a key the server won't start.
- `go run ./cmd/agent-server -stub` runs a stub agent instead, with no key and no model calls. It emits `workflow.started`, a log line and `workflow.completed`.

**The CLI:**

```bash
go run ./cmd/agent-cli < testdata/sample-task.txt
```

- The task is everything on stdin up to EOF. Events go to stdout; the model name and errors go to stderr.
- It exits 0 when the run ends in `workflow.completed`. It exits 1 when the run fails, including Ctrl-C mid-run.

## Check it

```bash
make check   # go mod tidy, gofmt, go vet, staticcheck, go build, go test -race: the same as CI's check job
make cover   # the tests with a per-function coverage report
```

`make check` needs staticcheck: `go install honnef.co/go/tools/cmd/staticcheck@2026.1`. Its config is in `staticcheck.conf`.

**CI** runs on every pull request and every push to `main`:

| Workflow | What it does |
|---|---|
| `ci.yml` | The same checks as `make check`. The coverage report goes to the job summary |
| `codeql.yml` | CodeQL security analysis. It also runs weekly |
| `govulncheck.yml` | Scans the code and its modules for known vulnerabilities, with the latest Go release. It also runs weekly |
| `dependency-review.yml` | Pull requests only: fails when a change adds a dependency with a known vulnerability of moderate severity or higher |

Dependabot opens weekly updates for Go modules and GitHub Actions.

`main` only takes pull requests, squash-merged. Pull requests use `.github/pull_request_template.md`.

## Rules

- Standard library only, plus two packages: the OpenRouter SDK and `github.com/google/uuid` (see `AGENTS.md`).
- Code goes where its users are. Code only one command uses lives in that command's folder; code both commands use lives in `internal/`.
- Go code is built test first.

## Design

[Design Canvas](https://app.excalidraw.com/s/7s9vhE83N90/4Dogyck9z2F)
