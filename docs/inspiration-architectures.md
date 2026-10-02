# Harness architectures — inspiration for ai-agent-go

Source clones live in `analysis/` (`ai-agent`, `crush`, `opencode`). This is a
read of the code as of 2026-09-04, focused on the shape of each harness, not on
feature lists.

Three harnesses at three scales:

| | ai-agent (mine) | crush (Charm) | opencode (anomalyco) |
|---|---|---|---|
| Language | Python | Go | TypeScript (Bun + Effect) |
| Size | ~400 LOC | ~100k LOC Go in `internal/` | monorepo, 27 packages |
| Shape | single script, one loop | one binary, client/server over a Unix socket | HTTP server + many clients (TUI, desktop, web, IDE, SDKs) |
| LLM abstraction | OpenAI SDK direct | `charm.land/fantasy` | Vercel AI SDK + own `provider/transform` |
| Persistence | none (in-memory list) | SQLite via sqlc | SQLite (Drizzle) + snapshot/storage layer |
| UI | stdout | Bubble Tea v2 TUI | OpenTUI/Solid TUI, web, desktop |
| Extensibility | none | MCP, skills, hooks (shell), `crushrc` | MCP, plugins (JS), skills, commands, ACP |

---

## 1. ai-agent (my own mini agent)

**What it is.** A CLI chatbot that gives an OpenRouter-hosted model four
sandboxed tools over a hardcoded working directory (`./calculator`). Built to
learn how function calling and dispatch actually work.

**Architecture (whole thing):**

```
main.py                 CLI args, OpenAI client, the agent loop (max 15 iterations)
llm/available_functions.py   JSON-schema tool declarations (list sent to the model)
llm/call_function.py         function_map: name -> python callable; wraps result as {role:"tool"}
functions/*.py               the 4 tools: get_files_info, get_file_content, write_file, run_python_file
test_data/prompts.py         system prompt
```

**The loop** (`main.py:call_model`):

```
messages = [system, user]
repeat ≤15:
  resp = chat.completions.create(model, messages, tools)
  if resp has tool_calls:
     for each call: result = function_map[name](working_dir, **args)
     append assistant msg; append ONE tool result   # <- bug: only the last result is appended
  else:
     print final text; stop
```

**Tool contract.** Each tool is `(working_directory, **kwargs) -> str`. Errors
are returned as `"Error: ..."` strings, never raised — the model sees the error
and can retry. Every tool goes through `is_inside_dir()` path-jail check
(`os.path.commonpath`). `run_python_file` has a 30s timeout and `.py`-only
filter. `get_file_content` truncates at 10k chars.

**What's already right (keep these ideas):**
- Tools return strings, errors included — the model self-corrects.
- A working-directory sandbox on every tool.
- Output truncation and an iteration cap.
- Schema declarations separated from implementations, joined by a name→func map.

**What's missing (these are exactly what crush/opencode add):**
- Multi-tool-call turns: only one `tool` message is appended per iteration
  (`result` is overwritten in the loop), so parallel tool calls break the
  message history.
- No streaming, no cancellation, no persistence, no permissions, no session
  concept, no provider abstraction, no UI events.
- Working dir is hardcoded; config is env-var only.

---

## 2. crush (charmbracelet)

**What it is.** A terminal AI coding assistant in Go. One binary that runs as
a detached **server** (Unix socket / named pipe, JSON over HTTP) with the TUI
as a **client**; can also run in-process ("local mode"). This is the closest
model for a Go harness.

**Layering** (`internal/`), from outside in:

```
cmd/            cobra CLI: root (TUI), run (non-interactive), server, login, models, sessions...
ui/             Bubble Tea v2 TUI. Pure consumer of pubsub events + client calls.
client/ proto/  Client SDK + wire types for the server API (/v1/...). TUI talks only via these.
server/         net/http mux over unix socket. Routes -> backend. Swagger-documented.
backend/        Multi-workspace host: Workspace = one project dir = one App. Client attach/detach,
                idle shutdown, event fan-out to clients.
app/            Wiring for ONE workspace: opens DB, builds services, Coordinator, LSP, MCP, brokers.
agent/          Coordinator (named agents "coder"/"task") + SessionAgent (the loop) + tools/
config/         Config service (crushrc bash-builtins or crush.json), provider/model catalog.
session/ message/ history/ filetracker/   Persistence services, SQLite via sqlc.
permission/ question/ hooks/              Human-in-the-loop + user shell hooks (PreToolUse).
lsp/ shell/ skills/ agent/tools/mcp/      Capability providers the tools sit on.
pubsub/         Generic Broker[T] — the nervous system between agent, services and UI.
```

**Dependency direction:** `ui -> client -> (proto) <- server -> backend -> app -> agent -> {services}`.
Everything below `app` communicates *upward* only through `pubsub.Broker[T]`
events; nothing below `app` imports `ui`.

**The core object graph** (`app.App`):

```go
type App struct {
    Sessions    session.Service      // CRUD, SQLite
    Messages    message.Service      // parts-based messages, debounced flush
    History     history.Service
    Permissions permission.Service   // Request(ctx, ...) blocks until UI grants/denies
    Questions   question.Service     // tool asks user a question
    FileTracker filetracker.Service
    AgentCoordinator agent.Coordinator
    LSPManager  *lsp.Manager
    Skills      *skills.Manager
    events      *pubsub.Broker[tea.Msg]   // everything the TUI needs to redraw
    ...
}
```

**Agent layer, two levels:**

- `Coordinator` (`agent/coordinator.go`) — owns provider construction
  (`buildProvider`: anthropic/openai/openrouter/bedrock/vertex/azure/...), model
  selection (large + small), tool construction (`buildTools`), prompt
  templates, and holds one `SessionAgent` per named agent (`coder`, `task`).
  `Run(ctx, sessionID, prompt)` is the public entry.
- `SessionAgent` (`agent/agent.go`) — the loop. Per-session mutex + queue:
  a prompt arriving while a run is active is **queued and folded into the next
  step**, not rejected. Cancellation is per session (`activeRequests`).
  Each run: load session messages from DB → prepend system prompt (+ MCP
  instructions) → `fantasy.NewAgent(model, tools...)` → stream → persist each
  delta into `message.Service` (which publishes `UpdatedEvent`s) → on finish
  publish `RunComplete`. Summarize/compaction and title generation use the
  small model.

**Tool contract** (`charm.land/fantasy`):

```go
type AgentTool interface {
    Info() ToolInfo                                   // name, description (rendered from .md.tpl), JSON schema
    Run(ctx, ToolCall) (ToolResponse, error)
    ProviderOptions() / SetProviderOptions()          // e.g. Anthropic cache_control on last tool
}
// Built with a generic helper: fantasy.NewAgentTool(name, desc, func(ctx, Params) (ToolResponse, error))
```

Tool implementations (`agent/tools/*.go`) each have a sibling `.md`/`.md.tpl`
description. Context values carry `sessionID`, `messageID`, `modelName` into
tools (`tools.GetSessionFromContext`). Tools are wrapped with decorators at
coordinator level: `hookedTool` (runs user PreToolUse shell hooks) → then the
tool itself calls `permissions.Request(...)` before doing anything dangerous.

**Permissions.** `permission.Service.Request(ctx, {SessionID, ToolName,
Action, Path, Params})` publishes a `PermissionRequest` event, blocks on a
channel until the UI calls `Grant/GrantPersistent/Deny`. Allow-lists from
config, per-session auto-approve for `crush run`. Sub-agent (`task`) sessions
get a derived session id (`CreateAgentToolSessionID`).

**Sub-agents.** The `agent` tool (`agent/agent_tool.go`) creates a child
session and runs the `task` SessionAgent inside it with a narrower toolset —
same loop, different config.

**Persistence.** `db/sql/*.sql` → sqlc → typed Go. Migrations shipped in the
binary. Sessions, messages (with parts serialized as JSON), file history.

**Client/server.** `server/` is a plain `net/http` mux; `proto/` is the shared
types; `client/` is a thin HTTP client. Events stream to the TUI over the
connection (`server/events.go`). `backend/` multiplexes workspaces (project
dirs) and clients on one server process, with idle shutdown. `cmd/root.go`
auto-starts a detached server if the socket is missing.

**Extensibility.** MCP servers (tools + resources), skills (markdown dirs),
hooks (shell commands on tool events, Claude Code-compatible payload), context
files (`AGENTS.md`, `CRUSH.md`, `CLAUDE.md`), `crushrc` bash config.

**Takeaways for a Go harness:**
- Small interfaces defined in the consuming package (`session.Service`,
  `permission.Service`) — easy to mock, easy to swap.
- `pubsub.Broker[T]` generic event bus is the decoupling point between agent
  and UI. Bubble Tea's `Update` just switches on `pubsub.Event[T]` types.
- Tool = `Info()` + `Run(ctx, call)`; descriptions live in markdown next to
  the code; decorators (hooks, permissions) wrap without the tool knowing.
- Prompt queuing + per-session cancel + persist-every-delta is what makes it
  feel robust.
- Client/server was added later; the `app.App` wiring is still the heart and
  works in-process.

---

## 3. opencode (anomalyco)

**What it is.** A server-first AI coding platform in TypeScript on Bun, built
on **Effect** (typed effects, DI via `Context.Service` / `Layer`). The agent
runtime is an HTTP API; TUI, desktop app, web UI, IDE extensions, Slack, and
GitHub actions are all just clients of that API.

**Package layering** (`packages/`), enforced by `AGENTS.md`:

```
schema      ──► core ──► server ──► sdk-next (composes client+core+server)
            ──► protocol ─┘             │
client (generated from HttpApi; may depend on schema+protocol, NEVER core/server)
opencode    the CLI binary: wires core+server, plus session/tool/provider/mcp/plugin/permission/...
tui         OpenTUI + SolidJS terminal client. Talks to the server via the SDK (in-process worker or remote URL)
app / desktop / web / console / slack / ide   more clients
plugin      public plugin API (JS hooks like command.execute.before, chat.messages.transform)
codemode    "confined code execution over schema-described tools"
```

Runtime dependency rule: Schema → Core/Protocol → Server. Clients never import
Core.

**Vocabulary** (from `CONTEXT.md`, worth stealing): *System Context* (built
from ordered **Context Sources**), *Session History* (projection after
compaction cutoffs), *Context Epoch* (immutable prompt baseline for provider
caching), *Admitted Prompt → Prompt Promotion* (durable inbox before
history), *Provider Turn* (one request), *Session Drain* (run turns until
nothing continues), *Model Tool Output* (bounded projection; oversized output
goes to a *Managed Tool Output File*).

**The core loop** (`packages/opencode/src/session/prompt.ts` `runLoop`):

```
ensureRunning(sessionID)            # one drain per session, others attach
loop:
  promote admitted prompts into history
  if pending compaction task: compaction.process(); if "stop" break; continue
  if isOverflow(lastFinished.tokens, model): compaction.create(auto); continue
  build system context: [sys.skills(agent), sys.environment(model), sys.mcp(agent, perms), instructions...]
  plugin.trigger("experimental.chat.messages.transform", messages)
  resolve tools for agent (ToolRegistry, MCP tools, permission ruleset)
  result = processor.process({ model, system, messages, tools })   # streams
  match result: "stop" -> break | "compact" -> compaction.create(); continue | "continue" -> continue
compaction.prune() in background
return lastAssistant(sessionID)
```

`processor.ts` consumes the AI SDK stream and switches on event types
(`reasoning-*`, `tool-input-*`, `tool-call`, `tool-result`, `tool-error`,
`step-start/finish`, `text-*`, `finish`), persisting **parts** to the message
and publishing bus events for each. Tool execution happens inside the
`tool-call` case, with permission `ask()` and abort signal.

**Tool contract** (`tool/tool.ts`):

```ts
interface Def<Params, M> {
  id: string
  description: string
  parameters: Schema                      // Effect Schema -> JSON schema for the model
  execute(args, ctx: Context): Effect<ExecuteResult<M>>
}
type Context = { sessionID, messageID, agent, abort: AbortSignal, messages,
                 metadata(...)  /* live progress to UI */,
                 ask(...)       /* permission request */ }
type ExecuteResult = { title, metadata, output: string, attachments? }
```

`wrap()` decorates every tool: decode/validate args (typed
`InvalidArgumentsError` fed back to the model), tracing attributes, output
truncation (`Truncate`), registry-enforced size limit. Tool descriptions are
sibling `.txt` files. `ToolRegistry.Service` resolves the toolset per agent and
provider (e.g. web search enabled per provider).

**Agents.** `agent/agent.ts` — named agents (build, plan, subagents) with
mode, model, permission ruleset, prompt. The `task` tool spawns a child
session running another agent (`parts: [{type:"subtask", agent, prompt}]`).
`command`s are markdown templates with `$1..$N`/`$ARGUMENTS` and inline
`!\`shell\`` expansion that resolve to a prompt (or subtask).

**Permissions** are a **ruleset**: `[{permission, action: allow|deny|ask,
pattern}]` merged from agent config and session (`Permission.merge`,
`Permission.evaluate(tool, name, rules)`). `ask` resolves through the bus to
whichever client is attached.

**Persistence and events.** SQLite through Drizzle (`effect-drizzle-sqlite`),
`storage/` abstraction, `snapshot/` (git-based file snapshots for revert),
`bus/` typed events → SSE/WebSocket to clients (`server/routes/instance/...`).
Sessions can be shared (`share/`) and synced (`sync/`).

**Extensibility.** MCP (tools, resources, OAuth), JS plugins with lifecycle
hooks, skills, commands, ACP (agent-client protocol for editors), custom
providers via `provider/transform.ts`.

**Takeaways:**
- Server-first pays off when you want >1 frontend; the TUI is *just* a client.
- Message = list of typed **parts** (text, reasoning, tool, file, subtask,
  step-start/finish). The UI renders parts; the provider adapter serializes
  them. This is the single most reusable data model decision.
- A processor that maps *stream events → persisted parts + bus events* is the
  seam between "LLM protocol" and "everything else".
- Compaction is a first-class loop outcome (`"compact"`), not an afterthought.
- Permission as a mergeable ruleset (agent-level + session-level) with
  `allow|deny|ask` and glob patterns.
- Named agents + subtask parts make sub-agents cheap.
- Structured vocabulary (`CONTEXT.md`) — write the glossary before the code.

---

## Side-by-side of the pieces every harness has

| Concern | ai-agent | crush | opencode |
|---|---|---|---|
| Loop | `for _ in range(15)` | `SessionAgent.Run` → `fantasy.Agent` stream | `runLoop` drain + `processor` |
| Message model | raw OpenAI dicts | `message.Message` with parts JSON, SQLite | `MessageV2` with typed parts, SQLite |
| Tool contract | `fn(workdir, **kw) -> str` | `AgentTool{Info, Run}` + `.md` desc | `Def{parameters, execute(args, ctx)}` + `.txt` desc |
| Tool → user I/O | none | `permission.Service.Request`, `question.Service` | `ctx.ask()`, `ctx.metadata()` |
| Provider abstraction | none | `fantasy.Provider` (many builders) | AI SDK + `provider/transform` |
| Cancel / queue | none | per-session ctx cancel, prompt queue folding | abort signal, `ensureRunning`, admitted-prompt inbox |
| Compaction | none | `Summarize` with small model | `compaction` service, auto on overflow |
| Sub-agents | none | `agent` tool → `task` agent, child session | `task` tool → subtask part, child session |
| Events to UI | print | `pubsub.Broker[T]` → Bubble Tea msgs | bus → SSE/WS → clients |
| Config | env var | `crushrc` / `crush.json`, config.Service | `opencode.json` + markdown agents/commands |
| Sandbox | path jail per tool | permissions + allow-lists + hooks | permission ruleset + external-directory guard |
| Transport | — | unix socket HTTP, `proto` types | HTTP API (`HttpApi`), generated client |

---

## What this suggests for the ai-agent-go MVP

Not a design yet — just the shape the three point at, ordered by how early
each decision locks you in:

1. **Message-as-parts data model first** (opencode). Text / reasoning / tool
   call / tool result / file. Everything else (DB, UI, provider adapters)
   hangs off it.
2. **Tool interface = `Info() + Run(ctx, call)`** (crush), with decorators for
   permissions/hooks, description in a sibling `.md`, and the path-jail +
   string-error conventions already in `ai-agent`.
3. **A generic `Broker[T]` pubsub** (crush) so the loop never imports the UI.
4. **Services as small interfaces** (`session.Service`, `message.Service`,
   `permission.Service`) backed by SQLite; in-memory impls are fine for MVP.
5. **Loop returns an outcome** (`stop | continue | compact`) and supports
   per-session cancel. Fix the "one tool result per turn" bug from `ai-agent`
   by appending every tool result.
6. **Provider abstraction** — start with one (Anthropic or OpenAI-compatible)
   behind an interface that streams typed events; add more later.
7. Start **in-process** (crush "local mode"); keep the `App` wiring so a
   server front can be added later without touching the agent.
