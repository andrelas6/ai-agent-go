# Harness Architectures — High-Level Study

Sources studied (cloned 2025-09):
- `anomalyco/opencode` — TypeScript/Bun monorepo
- `charmbracelet/crush` — Go
- `andrelas6/ai-agent` — my Python mini agent

---

## 1. opencode (TypeScript)

**Core idea: a headless server owns all state; every UI is just a client.**

```
┌─────────┐ ┌─────────┐ ┌──────────┐ ┌─────┐
│   TUI   │ │   CLI   │ │ Desktop/ │ │ SDK │
│(OpenTUI)│ │  cmd    │ │   Web    │ │     │
└────┬────┘ └────┬────┘ └────┬─────┘ └──┬──┘
     └───────────┴──── HTTP + SSE ──────┘
                    │
            ┌───────▼────────┐
            │  server/ (Hono) │  REST routes + SSE event stream
            └───────┬────────┘
        ┌───────────┼─────────────┐
        ▼           ▼             ▼
   session/      tool/         agent/
   prompt.ts ──► processor.ts  registry
   (loop)        (stream parts) task tool = subagents
        │           │             │
        ▼           ▼             ▼
   session/llm/  bus (global    permission/ question/
   provider abs. event bus)     storage (sqlite, drizzle)
        │
   mcp/  lsp/  plugin/  snapshot/ (git undo)
```

**Key mechanics:**
- **Message = list of parts** (text, reasoning, tool-call, file...). Everything the model
  does is appended as parts; the UI renders parts. This is the single most
  important data-model decision.
- **Agent loop** lives in `session/prompt.ts` + `session/processor.ts`:
  build messages → call LLM → consume stream part-by-part → on tool-call parts,
  execute tool, append result part → loop until stop / context overflow → compact.
- **Global event bus** (`bus/`): every mutation emits an event
  (`message.part.updated`, `session.status`, ...). Server re-broadcasts over SSE.
  Clients never poll; they subscribe.
- **Tools**: one file each = zod JSON schema + `.txt` prompt description +
  `execute` function. `registry.ts` collects them; MCP tools merge in.
- **LLM abstraction** (`session/llm/`): wraps Vercel AI SDK plus native provider
  runtimes. Newer code is migrating to **Effect** (functional effect/Stream
  system) — heavy but very explicit about errors/retries/cancellation.
- Extras: `snapshot/` (git-based revert), `lsp/` (diagnostics fed to the model),
  `plugin/` (JS plugin hooks), `permission/` + `question/` (interrupt the loop to
  ask the human).

**Takeaway:** state lives server-side in one event-sourced place; the loop emits
parts + events; UI is disposable. Overkill for an MVP but the *parts + event bus*
model is worth stealing.

---

## 2. crush (Go, Charm)

**Core idea: in-process event-driven monolith. Services own state, a pub/sub
broker fans events out, the TUI is a Bubble Tea subscriber.**

```
main.go → internal/cmd (cobra)
              │
        ┌─────▼─────┐
        │  app.App   │  composition root: wires all services + brokers
        └─────┬─────┘
   ┌──────────┼──────────────┬─────────────┐
   ▼          ▼              ▼             ▼
session.   message.      permission.    agent.Coordinator
Service    Service       Service        │
(sqlite,   (sqlite,       (approve/      ▼
 sqlc)      sqlc)         deny flow)   sessionAgent.Run
                                        │  agentic loop on top of
                                        │  "fantasy" (Charm's provider
                                        │  abstraction: anthropic, openai,
                                        │  gemini, bedrock, openrouter...)
                                        ▼
                                  agent/tools/* (bash, edit, grep,
                                  glob, fetch, lsp diagnostics, mcp...)
   ┌──────────────────────────────────────────┐
   │ pubsub.Broker[tea.Msg] (+ notifications, │
   │ run-completions brokers)                 │
   └───────────────┬──────────────────────────┘
                   ▼
              ui/ (Bubble Tea: chat, dialogs, diffview)
   (also: internal/server — REST over unix socket for headless/client mode)
```

**Key mechanics:**
- **Coordinator interface** (`internal/agent/coordinator.go`): `Run(ctx, sessionID,
  prompt, attachments...)`. One implementation, `sessionAgent`, runs the loop:
  stream via `agent.Stream(...)` → token deltas update the current assistant
  message in sqlite → tool calls execute (through permission gate) → append
  results → repeat.
- **Everything is a service with an interface**: `session.Service`,
  `message.Service`, `permission.Service`, `filetracker.Service` — constructed
  once in `app.New`, easy to fake in tests.
- **pubsub broker** (`internal/pubsub`): typed, lossy for high-frequency deltas
  (drop if subscriber's 4096-buffer is full), must-deliver for terminal events.
  This is exactly the right Go-shaped design: channels + interfaces.
- **TUI = pure subscriber**: Bubble Tea Elm loop consumes `tea.Msg` from the
  broker; permissions arrive as dialog events.
- **Tools**: one Go file + a markdown template (`.md.tpl`) for the description.
  MCP tools behind `agent/tools/mcp`.
- **Persistence**: sqlite via sqlc (type-safe generated queries).
- Bonus: `internal/server` serves a REST API over a unix socket, so crush can
  run headless and the TUI/`crush run` attach as clients.

**Takeaway:** this is the closest template for your project — same language,
proven shape. Interfaces + sqlite + one broker + Bubble Tea. You don't need
fantasy; a thin `Provider` interface over the Anthropic/OpenAI Go SDKs is enough.

---

## 3. ai-agent (mine, Python)

**Core idea: the irreducible minimum — one synchronous while-loop.**

```
main.py
  │
  ▼
messages[] ──► OpenRouter (OpenAI SDK, tools=schemas)
  ▲                   │
  │         tool_calls? ──no──► print final answer, done
  │                   │yes
  │                   ▼
  │        llm/call_function.py: function_map[name](args)
  │                   │   functions/{get_files_info, get_file_content,
  │                   │    write_file, run_python_file}.py
  │                   ▼
  └──── append assistant msg + tool result (max 15 iters)
```

**What it has (all a harness fundamentally needs):**
- Agentic loop with iteration cap
- Hand-written JSON schemas (`llm/available_functions.py`)
- Dynamic dispatch via `function_map` dict
- Working-directory sandboxing inside each tool

**What it lacks (and what opencode/crush add):**
- Streaming (prints only at the end)
- A UI layer separate from the loop
- Events (UI can't observe progress)
- Persistence / sessions
- Permissions / human-in-the-loop
- Subagents, MCP, LSP, compaction

---

## Comparison

| Concern | ai-agent (mine) | crush | opencode |
|---|---|---|---|
| Language | Python | Go | TypeScript/Bun |
| Shape | script | in-process monolith | client/server |
| Agent loop | sync for-loop, max 15 | `sessionAgent.Run` + fantasy stream | `prompt.ts`/`processor.ts` Effect stream |
| Tool def | dict + JSON schema by hand | Go file + .md.tpl template | zod schema + .txt prompt |
| Events | none | typed pubsub brokers | global bus → SSE |
| UI | print | Bubble Tea subscriber | OpenTUI client of server |
| State | in-memory list | sqlite (sqlc) services | sqlite (drizzle) |
| Permissions | none | service + UI dialog | permission/question modules |
| LLM layer | OpenAI SDK direct | fantasy (multi-provider) | AI SDK + native runtimes |
| LOC-ish scale | ~300 | large, production | very large, production |

## Conclusion for the Go MVP

The natural evolution path: **ai-agent's loop, restructured crush-style**:

1. `Provider` interface (stream, tools) — one impl first (Anthropic or OpenRouter)
2. `Tool` interface: `Schema() / Execute(ctx, args) → result` — port your 4 tools
3. Agent loop: stream → dispatch tools → append parts → repeat, with ctx cancel
4. In-process event broker (channels) so UI ≠ loop from day one
5. Bubble Tea TUI as a subscriber
6. sqlite persistence + permission gate last

Skip for MVP: MCP, LSP, subagents, compaction, client/server split, plugins.
