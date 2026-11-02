# ae-init: The Lesson 1 tools and system prompt

> **Status:** Draft
> **Last updated:** 2026-11-02
> **From:** ae-plan/lesson-1-harness.md, subtask 6, on `main` at `0958115`

## What to build

The four support-triage tools from the course's Lesson 1, and the system prompt that tells the agent to use them, written in Go. They are the tools the real agent will get in subtask 7. A knowledge-base search returns canned articles. Classify, draft and send each echo back a small result. Send "emails the customer" the moment it is called, with no checks, because the course adds safety later. The tools are strict about their input: a missing, empty, wrongly typed or unknown field is an error that goes back to the model. In this subtask they are proven by tests only. Nothing a user runs changes: the server still runs the course's stub agent, and the CLI is untouched.

## A -> B

- **A (now):** no Lesson 1 tools and no system prompt exist in Go. `go test ./internal/tools/...` runs only the two `ReadFile` tests. The only tools are the fakes inside the agent tests.
- **B (after):** `go test ./internal/tools/...` also covers the four tools. Each returns the course's JSON for a sample input and rejects bad input. The system prompt is a Go constant with the course's wording. The server and the CLI behave exactly as before.

Grounded in the code on `main` (`0958115`):
- `internal/tools/` holds only `readfile.go` (`ReadFile`, `ReadFileToolSpec`). `go test -v ./internal/tools/...` runs `TestReadFile` and `TestReadFileMissing`, then `ok`.
- `cmd/agent-cli/main.go:36` imports `tools.ReadFile`. It must keep working.
- `internal/agent/agent.go` defines `Tool{Name, Description, Parameters map[string]any, Run func(ctx, json.RawMessage) (any, error)}` and `NewAgent(systemPrompt, model, tools ...Tool)`. The loop marshals whatever `Run` returns into the `tool.completed` result.
- `runTool` already rejects an unknown tool name, empty arguments ("no arguments sent") and invalid JSON before any tool runs. At the tool level, "bad input" therefore means a wrong shape: a missing, `null`, empty, wrongly typed or unknown field, or a bad category.
- The plan says subtask 6 "replaces the fake tools in the default registry". There is no default registry. The fake tools live only in `internal/agent/agent_test.go`, and `cmd/agent-server/main.go:21` passes `runStub` to `handlers.NewMux`. Nothing is replaced; the four tools are new, and subtask 7 hands them to `NewAgent`.
- The course originals are `origin/lesson-2:harness/tools.ts` and `origin/lesson-2:harness/system-prompt.ts`. They are identical to the code in the Lesson 1 text.

### Visual

```text
A (now)
  $ go test -v ./internal/tools/...
    --- PASS: TestReadFile
    --- PASS: TestReadFileMissing
    ok   internal/tools
  POST /api/tasks ──▶ server ──▶ workflow.started, log "No agent yet…", workflow.completed

B (after)
  $ go test -v ./internal/tools/...
    --- PASS: TestReadFile
    --- PASS: TestReadFileMissing
    --- PASS: tests for the four tools and their schemas                    ◀ new
    ok   internal/tools

  searchKnowledgeBase {"query":"pricing for 50 seats"}                      ◀ new
    ──▶ {"articles":["Team plans are $20/seat/mo … send the pricing PDF."]}
  classifyItem {"itemId":"item-1","category":"billing"}                     ◀ new
    ──▶ {"ok":true,"itemId":"item-1","category":"billing"}
  draftReply {"itemId":"item-1","message":"Sorry about that…"}              ◀ new
    ──▶ {"ok":true,"draftId":"draft-item-1"}
  sendReply {"itemId":"item-1","draftId":"draft-item-1"}                    ◀ new
    ──▶ {"sent":true,"itemId":"item-1","draftId":"draft-item-1"}
  classifyItem {"itemId":"item-1","category":"refunds"}                     ◀ new
    ──▶ error  (would be tool.failed in the loop)

  POST /api/tasks ──▶ server ──▶ workflow.started, log "No agent yet…", workflow.completed
                                                     (same as A: still the stub)
```

## How to reproduce

**Preconditions**
- `ai-agent-go` checkout, Go toolchain, run from the repo root.
- No `OPENROUTER_API_KEY` needed: nothing calls the network.
- For the server check: `go run ./cmd/agent-server` (listens on `:8002`).

**Steps**
1. `go test -race -v ./internal/tools/... ./internal/agent/...`
2. `go test ./...`
3. In one shell: `curl -N localhost:8002/api/events`. In another: `curl -X POST localhost:8002/api/tasks -d '{"input":"hi"}'`.

**Observe**

Strings below are copied from `tools.ts`, including the en dashes in "3–5" and "5–10" and the em dash in the fallback. The knowledge base is checked in the course's order: billing, refund, export, pricing.

| | A (now) | B (after) |
|---|---|---|
| Step 1, test list | only `TestReadFile`, `TestReadFileMissing` | those two, plus tests for the four tools; `ok`, race-clean |
| `searchKnowledgeBase {"query":"pricing for 50 seats"}` | n/a | `{"articles":["Team plans are $20/seat/mo with a volume discount at 25+ seats. For 50+ seats, send the pricing PDF."]}` |
| `searchKnowledgeBase {"query":"Billing and REFUND"}` (case, several matches) | n/a | two articles, billing then refund, in that order every run |
| `searchKnowledgeBase {"query":"I was charged twice"}` | n/a | `{"articles":["No exact match — use your judgment."]}`. No key matches, so item-1 relies on the model choosing a better query |
| `classifyItem {"itemId":"item-2","category":"technical"}` | n/a | `{"ok":true,"itemId":"item-2","category":"technical"}` |
| `classifyItem` with `category` not in billing, technical, sales, other | n/a | error |
| `draftReply {"itemId":"item-3","message":"…"}` | n/a | `{"ok":true,"draftId":"draft-item-3"}`. `message` is required even though it isn't used |
| `sendReply {"itemId":"item-3","draftId":"draft-item-3"}` | n/a | `{"sent":true,"itemId":"item-3","draftId":"draft-item-3"}`. No `ok` field, as in the course. No checks |
| A required field missing, e.g. `draftReply {"itemId":"item-1"}` | n/a | error naming the field |
| A field set to `null` | n/a | error |
| A field set to `""`, e.g. `searchKnowledgeBase {"query":""}` | n/a | error (stricter than zod, which accepts `""`) |
| A field of the wrong type, e.g. `{"itemId":5,...}` | n/a | error |
| An unknown field, e.g. `classifyItem {...,"priority":"high"}` | n/a | error (stricter than zod, which strips it) |
| Each tool's schema | n/a | `type: object`, its fields as `string`, all listed in `required`; `category` has the four-value `enum`; `query` is described "what to look up" |
| Each tool's name and description | n/a | the course's, verbatim (e.g. `sendReply`: "Send the drafted reply to the customer. This really emails them.") |
| The system prompt constant | n/a | the course's `SYSTEM_PROMPT` text, verbatim |
| Step 2 | everything `ok` | everything `ok`; `agent-cli` still builds and uses `ReadFile` |
| Step 3, the server (regression) | `workflow.started`, `log` (warn, "No agent yet…"), `workflow.completed` | identical: the server still runs the stub |

## Decisions

Agreed before this doc:
- Four tools with the course's names, descriptions, inputs and results. Results are JSON in camelCase, as in the course.
- JSON schemas are written by hand as `map[string]any`, with `required` and an `enum` for `category`. They are a hand-written equivalent of the course's zod schemas, not a byte-for-byte copy of what the AI SDK emits.
- The knowledge base is ordered (a slice, not a map), so a query that matches several keys always returns the same articles in the same order.
- The tools live in `internal/tools`, which both commands will use, behind one constructor such as `tools.Support() []agent.Tool`.
- No safety checks on `sendReply`: the course adds them in later lessons.
- Out of scope: wiring the tools and prompt into `agent-server` (subtask 7) and the sample task file (subtask 8). `agent-server` keeps `runStub`.

Settled with André on 2026-11-02:
- **Slug:** `lesson-1-tools`.
- **Input is strict.** Any of these is an error, which the loop sends back to the model as `tool.failed`:
  - a missing or `null` field;
  - an empty string;
  - a field of the wrong type;
  - an unknown field;
  - a category outside the four values.
  
  This is stricter than zod, which accepts `""` and strips unknown fields. In Go, a plain `json.Unmarshal` into `string` fields reads a missing or `null` field as `""`, so the empty-string check also covers those, and unknown fields need `DisallowUnknownFields`.
- **The system prompt lives in `internal/agent`**, as an exported constant with the course's wording, not in `internal/tools`.
- **Unit tests only.** No extra test runs the four tools inside `agent.Run`; subtask 5 already covers the loop, and subtask 7 runs them for real.

## Open questions

- None.
