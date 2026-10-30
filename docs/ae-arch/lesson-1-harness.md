# ae-arch: Lesson 1 harness in Go (SSE inspector + agent loop)

> **Status:** Draft
> **Last updated:** 2026-10-16
> **From:** ae-plan/lesson-1-harness.md

The developer sends a task from the course's inspector or from `agent-cli`. The Go agent loop, `agent.Run`, works the task by calling OpenRouter and four fake support tools, and reports every step as a course-shaped event. In the server, events go to an in-memory bus. The bus streams them over SSE to every open inspector tab and replays its history when a tab reconnects. In the CLI, the same loop prints one line per event. Nothing is stored on disk yet; that's Lesson 2.

## 1. System context

```mermaid
flowchart LR
    dev["<b>Developer</b><br/>[Person]<br/>Follows the course; submits tasks and watches the agent work"]
    harness["<b>ai-agent-go harness</b><br/>[Software System]<br/>Runs the Lesson 1 agent loop and shows every step it takes"]
    openrouter["<b>OpenRouter API</b><br/>[Software System, external]<br/>Hosted LLM gateway that picks the agent's next step"]

    dev -- "submits tasks to and watches events from" --> harness
    harness -- "sends the conversation + tool list to" --> openrouter
    openrouter -- "returns replies and tool calls to" --> harness

    classDef person fill:#08427b,color:#fff,stroke:#052e56
    classDef system fill:#1168bd,color:#fff,stroke:#0b4884
    classDef external fill:#999,color:#fff,stroke:#6b6b6b
    class dev person
    class harness system
    class openrouter external
```

```
System context diagram for the ai-agent-go harness

+----------------------+                              +--------------------------+
| Developer            |  submits tasks to and        | ai-agent-go harness      |
| [Person]             |  watches events from         | [Software System]        |
| Follows the course;  |----------------------------->| Runs the Lesson 1 agent  |
| submits tasks and    |                              | loop and shows every     |
| watches the agent    |                              | step it takes            |
+----------------------+                              +--------------------------+
                                                         |                   ^
                         sends the conversation +        |                   | returns replies and
                         tool list to                    v                   | tool calls to
                                                      +--------------------------+
                                                      | OpenRouter API           |
                                                      | [Software System, ext.]  |
                                                      | Hosted LLM gateway that  |
                                                      | picks the agent's next   |
                                                      | step                     |
                                                      +--------------------------+

Key: [Person] dark blue · [Software System] blue · external system grey · arrows are one-way
```

## 2. Containers

```mermaid
flowchart TB
    dev["<b>Developer</b><br/>[Person]"]
    openrouter["<b>OpenRouter API</b><br/>[Software System, external]<br/>HTTPS chat completions"]

    subgraph harness["ai-agent-go harness [Software System]"]
        inspector["<b>Inspector</b><br/>[Container: React SPA in the browser]<br/>Shows the event timeline; sends tasks and Clear.<br/><i>Lives in the harness-engineering repo, branch off lesson-1</i>"]
        vite["<b>Vite dev server</b><br/>[Container: Node.js, Vite 6, :5173]<br/>Serves the inspector and proxies /api to the Go server"]
        server["<b>agent-server</b><br/>[Container: Go, net/http, :8002]<br/>Runs agent.Run for each task; streams events over SSE"]
        cli["<b>agent-cli</b><br/>[Container: Go binary]<br/>Reads a task from stdin; runs agent.Run; prints one line per event"]
    end

    dev -- "opens in a browser tab" --> inspector
    dev -- "pipes a task into (stdin)" --> cli
    inspector -- "loads its JS/CSS from [HTTP]" --> vite
    inspector -- "opens /api/events, POSTs /api/tasks and /api/clear to [SSE + JSON/HTTP]" --> vite
    vite -- "forwards /api/* requests to [HTTP proxy]" --> server
    server -- "streams events back through the proxy to [SSE: id + data lines]" --> vite
    server -- "calls Chat.Send on [JSON/HTTPS]" --> openrouter
    cli -- "calls Chat.Send on [JSON/HTTPS]" --> openrouter

    classDef person fill:#08427b,color:#fff,stroke:#052e56
    classDef container fill:#438dd5,color:#fff,stroke:#2e6295
    classDef external fill:#999,color:#fff,stroke:#6b6b6b
    class dev person
    class inspector,vite,server,cli container
    class openrouter external
```

```
Container diagram for the ai-agent-go harness

                         +------------------+
                         | Developer        |
                         | [Person]         |
                         +------------------+
                    opens in a |        | pipes a task into (stdin)
                   browser tab v        v
+- ai-agent-go harness [Software System] -------------------------------------------+
|                                                                                   |
|  +---------------------------+                       +--------------------------+ |
|  | Inspector                 |                       | agent-cli                | |
|  | [Container: React SPA]    |                       | [Container: Go binary]   | |
|  | Event timeline; sends     |                       | stdin task -> agent.Run  | |
|  | tasks and Clear           |                       | -> one line per event    | |
|  | (harness-engineering repo)|                       +--------------------------+ |
|  +---------------------------+                                    |               |
|     | loads JS/CSS [HTTP]                                         |               |
|     | /api/events, /api/tasks, /api/clear [SSE + JSON/HTTP]       |               |
|     v                                                             |               |
|  +---------------------------+                                    |               |
|  | Vite dev server           |                                    |               |
|  | [Container: Node, :5173]  |                                    |               |
|  | Serves the SPA; proxies   |                                    |               |
|  | /api to the Go server     |                                    |               |
|  +---------------------------+                                    |               |
|     | forwards /api/*    ^ streams events back                    |               |
|     | [HTTP proxy]       | [SSE: id + data lines]                 |               |
|     v                    |                                        |               |
|  +---------------------------+                                    |               |
|  | agent-server              |                                    |               |
|  | [Container: Go, :8002]    |                                    |               |
|  | agent.Run per task;       |                                    |               |
|  | SSE event stream          |                                    |               |
|  +---------------------------+                                    |               |
+-----|-------------------------------------------------------------|---------------+
      | calls Chat.Send [JSON/HTTPS]                                | calls Chat.Send [JSON/HTTPS]
      v                                                             v
   +---------------------------------------------------------------------+
   | OpenRouter API  [Software System, external]                         |
   +---------------------------------------------------------------------+

Key: [Person] dark blue · [Container] light blue · external grey · [..] on arrows = protocol
```

## 3. Components inside agent-server

```mermaid
flowchart TB
    vite["<b>Vite dev server</b><br/>[Container]"]
    openrouter["<b>OpenRouter API</b><br/>[Software System, external]"]

    subgraph server["agent-server [Container: Go]"]
        main["<b>cmd/agent-server</b><br/>[Component: Go main]<br/>Wires the pieces together; listens on :8002; shuts down on Ctrl-C"]
        http["<b>handlers</b><br/>[Component: cmd/agent-server/handlers.go, package main]<br/>GET /api/events (SSE), POST /api/tasks, POST /api/clear"]
        bus["<b>bus</b><br/>[Component: Go, mutex + channels]<br/>Stamps UUID + ts; keeps the last 1000 events; one buffered channel (256) per tab; cuts off a full tab"]
        agent["<b>agent</b><br/>[Component: Go]<br/>agent.Run: the brittle loop, max 10 steps; runs tool calls by name"]
        tools["<b>tools</b><br/>[Component: Go]<br/>searchKnowledgeBase, classifyItem, draftReply, sendReply + hand-written JSON schemas"]
        model["<b>model</b><br/>[Component: OpenRouter Go SDK]<br/>Wraps Chat.Send; model id from OPENROUTER_MODEL"]
        events["<b>events</b><br/>[Component: Go struct]<br/>The Event shape, matching the course's shared/events.ts"]
    end

    vite -- "forwards /api/* to [HTTP]" --> http
    http -- "streams event JSON back to [SSE]" --> vite
    main -- "builds and starts" --> http
    http -- "subscribes with Last-Event-ID / clears history in" --> bus
    http -- "starts a goroutine running" --> agent
    agent -- "publishes each step as an Event to" --> bus
    agent -- "asks for the next step from" --> model
    agent -- "runs tool calls on" --> tools
    model -- "sends the request to [JSON/HTTPS]" --> openrouter
    bus -. "carries" .-> events

    classDef container fill:#438dd5,color:#fff,stroke:#2e6295
    classDef component fill:#85bbf0,color:#000,stroke:#5d82a8
    classDef external fill:#999,color:#fff,stroke:#6b6b6b
    class vite container
    class main,http,bus,agent,tools,model,events component
    class openrouter external
```

```
Component diagram for agent-server

     +----------------------+
     | Vite dev server      |
     | [Container]          |
     +----------------------+
  forwards /api/* | ^ streams event JSON back
           [HTTP] v | [SSE]
+- agent-server [Container: Go] ----------------------------------------------------+
|                                                                                   |
|  +------------------+  builds and    +----------------------------------------+   |
|  | cmd/agent-server |  starts        | handlers  [Component: package main]    |   |
|  | [Component: main]|--------------->| GET /api/events (SSE)                  |   |
|  | :8002, Ctrl-C    |                | POST /api/tasks, POST /api/clear       |   |
|  +------------------+                +----------------------------------------+   |
|                    subscribes with Last-Event-ID |              | starts a         |
|                         / clears history in     v              v goroutine running|
|  +------------------------------------+       +--------------------------------+  |
|  | bus  [Component: mutex + channels] |<------| agent  [Component: Go]         |  |
|  | UUID + ts; last 1000 events;       | pub-  | agent.Run, max 10 steps;       |  |
|  | 1 channel (256) per tab;           | lishes| runs tool calls by name        |  |
|  | cuts off a full tab                | each  +--------------------------------+  |
|  +------------------------------------+ step     | asks for the       | runs tool  |
|          : carries                                | next step from     | calls on   |
|          v                                        v                    v            |
|  +----------------------+   +---------------------------+  +--------------------+  |
|  | events [Component]   |   | model  [Component: SDK]   |  | tools [Component]  |  |
|  | Event struct, same   |   | Chat.Send;                |  | 4 fake support     |  |
|  | shape as events.ts   |   | OPENROUTER_MODEL          |  | tools + schemas    |  |
|  +----------------------+   +---------------------------+  +--------------------+  |
+-------------------------------------------|---------------------------------------+
                                            | sends the request to [JSON/HTTPS]
                                            v
                              +-----------------------------+
                              | OpenRouter API [external]   |
                              +-----------------------------+

Key: [Component] pale blue · [Container] light blue · external grey · dotted = data shape, not a call
```

## 4. Dynamic: one task, and a tab that falls behind

```mermaid
sequenceDiagram
    autonumber
    participant tab as Inspector tab [SPA]
    participant vite as Vite dev server
    participant http as handlers
    participant bus as bus
    participant agent as agent.Run
    participant or as OpenRouter API

    tab->>vite: GET /api/events (EventSource)
    vite->>http: proxy GET /api/events
    http->>bus: Subscribe(lastID = "")
    bus-->>http: full history + live channel
    http-->>tab: id: {uuid} / data: {...} per past event
    tab->>vite: POST /api/tasks {input, mode}
    vite->>http: proxy POST
    http->>agent: go agent.Run(serverCtx, workflowId, input)
    http-->>tab: 202 {workflowId}
    agent->>bus: Publish workflow.started
    loop until no tool calls, max 10 steps
        agent->>or: Chat.Send(conversation, tools)
        or-->>agent: assistant message (+ tool calls)
        agent->>bus: Publish tool.requested / tool.completed per call
        bus-->>http: event on each tab's channel
        http-->>tab: id: {uuid} / data: {...} (flushed)
    end
    agent->>bus: Publish model.completed, workflow.completed
    Note over bus,tab: Tab stops reading, its 256 buffer fills
    bus->>http: close that tab's channel (cut off)
    http-->>tab: SSE response ends
    tab->>vite: EventSource reconnects, Last-Event-ID: {uuid}
    vite->>http: proxy GET /api/events
    http->>bus: Subscribe(lastID = {uuid})
    bus-->>http: events after {uuid} + live channel
    http-->>tab: missed events, then live
```

```
Dynamic diagram: one task, and a tab that falls behind

 1. Inspector tab  -> Vite -> handlers     GET /api/events (EventSource)
 2. handlers       -> bus                  Subscribe(lastID = "")
 3. bus            -> handlers             full history + live channel
 4. handlers       -> tab                  "id: {uuid}\ndata: {...}\n\n" per past event
 5. tab            -> Vite -> handlers     POST /api/tasks {input, mode}
 6. handlers       -> agent.Run            go agent.Run(serverCtx, workflowId, input)
 7. handlers       -> tab                  202 {workflowId}
 8. agent.Run      -> bus                  Publish workflow.started
 9. loop (until no tool calls, max 10 steps):
      agent.Run -> OpenRouter   Chat.Send(conversation, tools)
      OpenRouter -> agent.Run   assistant message (+ tool calls)
      agent.Run -> bus          Publish tool.requested / tool.completed per call
      bus -> handlers -> tab    one SSE message per event, flushed
10. agent.Run      -> bus                  Publish model.completed, workflow.completed
11. tab stops reading; its 256-event buffer fills
12. bus            -> handlers             close that tab's channel (cut off)
13. handlers       -> tab                  SSE response ends
14. tab            -> Vite -> handlers     EventSource reconnects with Last-Event-ID: {uuid}
15. handlers       -> bus                  Subscribe(lastID = {uuid})
16. bus            -> handlers   -> tab    missed events, then live

Key: -> = request/call · numbered in time order
```

## Components

| Name | Type | Job |
|---|---|---|
| Inspector | Container (React SPA) | Shows the event stream and sends tasks and Clear. Only `useHarnessSocket.ts`, `TaskPane.tsx` and `vite.config.ts` change. |
| Vite dev server | Container (Node, Vite 6) | Serves the inspector, and proxies `/api/*` to `:8002` so the browser stays on one origin and needs no CORS. |
| agent-server | Container (Go) | Hosts the HTTP endpoints, the bus, and one `agent.Run` goroutine per task. |
| agent-cli | Container (Go) | Runs the same `agent.Run` in-process with a print-to-terminal `emit`. It doesn't talk to agent-server. |
| handlers | Component (`cmd/agent-server`, package main) | Turns HTTP into bus subscriptions and agent runs. Owns SSE framing and `Last-Event-ID`. |
| bus | Component | Fans events out to tabs, keeps the last 1000, never blocks the agent, and cuts off a tab whose buffer is full. |
| agent | Component | The brittle Lesson 1 loop: ask the model, run tools, repeat; emit each step. |
| tools | Component | The four fake support tools and their JSON schemas. |
| model | Component | The only code that talks to OpenRouter. |
| events | Component | The single Event struct, kept identical to the course's `shared/events.ts`. |

## Assumptions and open questions

- **Layout follows usage.** `events`, `agent`, `tools` and `model` are used by both commands, so they live in `internal/`. The handlers are only used by agent-server, so they live in `cmd/agent-server` as `package main`. `bus` is also server-only but stays in `internal/` for now, to keep it flexible.
- **Assumption: the inspector is drawn inside the system,** although its code lives in the course repo. It's a container of this system at runtime.
- **Assumption: agent-cli and agent-server share code, not a connection.** Both compile in `agent`, `tools` and `model`. The CLI never calls the server's HTTP API.
- **Assumption: the Vite proxy streams SSE without buffering.** The docs are silent on this. Check it with the `curl -N` versus browser timing from the ae-init.
- **Assumption: everything lives in memory.** A server restart loses history and kills running tasks. That's the Lesson 1 failure Lesson 2 fixes.
- **Assumption: tasks run on the server's context,** not the request's. Ctrl-C cancels every running `agent.Run`, which emits `workflow.failed`, and SSE handlers end on shutdown.
- **Assumption: `mode` in `POST /api/tasks` is accepted and ignored** until Lesson 6.
- **Open question: should a cut-off tab get an SSE `retry:` hint** (for example 1000 ms) so it reconnects quickly? Browsers default to about 3 s.
