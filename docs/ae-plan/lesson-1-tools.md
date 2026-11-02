# ae-plan: The Lesson 1 tools and system prompt

> **Status:** Done (subtasks 1–5, layout changed after subtask 2), merged in [andrelas6/ai-agent-go#16](https://github.com/andrelas6/ai-agent-go/pull/16) as `ff7c88e`
> **Last updated:** 2026-11-02
> **From:** ae-init/lesson-1-tools.md, which is subtask 6 of ae-plan/lesson-1-harness.md, on `main` at `0958115`

## What's being built

The course's four support-triage tools (`searchKnowledgeBase`, `classifyItem`, `draftReply`, `sendReply`) and its system prompt, in Go. Each tool has its own constructor in `internal/tools`, such as `tools.ClassifyItem()`. The support agent that composes them with the prompt is `agents.NewSupportAgent(model)`, which subtask 7 will run. They're strict about input. A bad call returns an error, and the loop already turns that into `tool.failed`. Only tests prove it here. The server keeps running the stub, and the CLI is untouched.

## Constraints

- **Dependencies:** the standard library only (`AGENTS.md`). Schemas are written by hand as `map[string]any`, with no `invopop/jsonschema`.
- **Match the course:**
  - names, descriptions, the `query` description, results and knowledge-base text are copied byte for byte from `origin/lesson-2:harness/tools.ts`, including the en dashes in "3–5" and "5–10" and the em dash in "No exact match — use your judgment.";
  - the prompt is copied from `system-prompt.ts`;
  - results are camelCase JSON;
  - `sendReply` returns no `ok`.
- **Input is strict, stricter than zod.** Each of these is an error:
  - a missing or `null` field;
  - `""`;
  - a field of the wrong type;
  - an unknown field, or a known one in the wrong case (`ItemID`);
  - a category outside billing, technical, sales and other.
- **Where things go** (changed after subtask 2; see "Layout change" below):
  - `internal/tools` holds only tools, one file per tool, plus the `Tool` type and the shared schema helpers;
  - `internal/agents` holds the generic agent (the loop, formerly `internal/agent`) and the specific agents built from it. The support agent and its prompt live in `agents/support.go`.
- **Unit tests only.** No test runs the tools inside `agent.Run`.
- **Out of scope:**
  - wiring into `agent-server` (subtask 7);
  - the sample task file (subtask 8);
  - any safety check on `sendReply`.
- **No comments** in new code. André's existing comments stay.

## Layout change, 2026-11-02

After subtask 2, André pointed out that `tools/support.go` was really a whole agent hidden in the tools folder: it held the list of tools, and the prompt was going to sit next to it. So the code was restructured into one refactor commit, with no change in behaviour:
- **Generic agent and specific agents together.** `internal/agent` is renamed to `internal/agents`. `agent.go` (the loop) is the building block, and `support.go` holds `NewSupportAgent(model)`, which composes it with the support tools and, from subtask 5, the support prompt.
- **`Tool` moved into `tools/tool.go`.** Otherwise `agents` (which uses the tools) and `tools` (which used `agent.Tool`) would import each other. Now `agents` imports `tools`, and `tools` imports nothing of ours.
- **One file per tool:** `search.go`, `classify.go`, `draft.go`, `send.go`, each with an exported constructor and its own test file. The shared helpers (`objectSchema`, `strict`) are in `schema.go`, and the test helpers in `helpers_test.go`.
- **`tools.Support()` is gone.** `TestSupportAgentOffersTheSupportTools` in `agents/support_test.go` now checks the four tools and their order, by looking at what the fake model is offered.
- An alternative was kept in mind: a sub-folder per specific agent (`internal/agents/support`), with `Tool` staying in `agents`. We can move to it if specific agents grow to several files each.

## What I found in the codebase

- **`internal/agent/agent.go`**:
  - `Tool{Name, Description, Parameters map[string]any, Run func(ctx, json.RawMessage) (any, error)}`;
  - `runTool` rejects an unknown name, empty args and invalid JSON before `Run` is called, then runs `json.Marshal` on what `Run` returns. A tool can therefore return a struct with camelCase `json` tags and never sees broken JSON.
- **Imports** *(before the layout change)*: `internal/tools` imported `internal/agent` for `agent.Tool`. Since the layout change it's the other way round: `agents` imports `tools`, and `Tool` lives in `tools`.
- **`internal/tools/readfile.go`**: package `tools`, with `ReadFile` used by `cmd/agent-cli/main.go:36`. It's untouched. Its test is in-package (`package tools`), with plain `t.Errorf`/`t.Fatalf`.
- **`internal/agent/agent_test.go`**: table tests with `t.Run` and `reflect.DeepEqual`. No test helper library. Follow the same style.
- **`docs/openrouter-agent-loop.md:84`**: the schema shape this repo already expects is `{"type":"object","properties":{...},"required":[...],"additionalProperties":false}`. The hand-written schemas follow it.
- **There's no default tool registry to replace**, despite the lesson-1 plan's wording. The fake tools live only in `agent_test.go`.

## What the docs say

- **`go doc encoding/json Decoder.DisallowUnknownFields`**: makes `Decode` fail when an object key doesn't match a struct field. Matching ignores case, though, so it isn't enough on its own.
- **`go doc encoding/json Unmarshal`**:
  - `null` into a `string` "has no effect", so a missing field and a `null` field both leave `""`. One empty-string check therefore covers missing, `null` and `""`.
  - Keys match struct fields **ignoring case**, so `{"ItemID":"x"}` fills `ItemID`. That's why the helper checks exact keys in a first pass over a `map[string]json.RawMessage`. Map keys keep their case. `encoding/json/v2`, which is case-sensitive, is still behind `GOEXPERIMENT` in Go 1.26.
- **`go doc encoding/json UnmarshalTypeError`**: a wrong type (`"itemId":5`) is an `*UnmarshalTypeError` naming the field. Pass it through as the error.
- **JSON Schema** (the shape OpenRouter's `Parameters` takes, per `docs/openrouter-agent-loop.md`):
  - `required`, `enum`, `additionalProperties: false` and `minLength: 1` tell the model what the tool will reject;
  - adding the last two keeps the schema honest about the strict checks.

## Approach

`internal/tools` has one file per tool (`search.go`, `classify.go`, `draft.go`, `send.go`). Each has an exported constructor returning a `tools.Tool`, and one small private args struct with camelCase `json` tags. `schema.go` has:
- the schema builders (`objectSchema`, `plainString`, `oneOf`);
- one shared helper that decodes args strictly, in two passes:
  - **keys.** Decode into `map[string]json.RawMessage`. Any key that isn't exactly one of the schema's `properties` is an error, which catches unknown fields and `ItemID`-style case variants;
  - **values.** Decode into the args struct. Type errors are passed through. Then check that each required field is non-empty, and name it in the error.
  
  The helper reads the key list from the tool's own schema, so the schema and the check can't drift.

`search.go` keeps the knowledge base as an ordered slice of `{key, article}`. `searchKnowledgeBase` lower-cases the query and keeps every article whose key it contains, in slice order. With no hit it returns the fallback.

`classifyItem` checks the category against the same four values the schema's `enum` lists, taken from one shared list so the two can't drift.

`agents.SupportPrompt` is a const in `internal/agents/support.go`, and `NewSupportAgent(model)` passes it to `NewAgent` along with the four tools.

## Red / Green

**Tool specs**
- Each tool's constructor gives the course's name and description verbatim.
- `NewSupportAgent` offers the model `searchKnowledgeBase`, `classifyItem`, `draftReply`, `sendReply`, in that order.
- Each schema has:
  - `type: object`;
  - every field as `string` with `minLength: 1`;
  - all of them listed in `required`;
  - `additionalProperties: false`.
- `category` has `enum: [billing, technical, sales, other]`. `query` has the description "what to look up".

**Results**
- `searchKnowledgeBase {"query":"pricing for 50 seats"}` → `{"articles":["Team plans are $20/seat/mo with a volume discount at 25+ seats. For 50+ seats, send the pricing PDF."]}`.
- `{"query":"Billing and REFUND"}` → the billing article, then the refund article.
- `{"query":"I was charged twice"}` → `{"articles":["No exact match — use your judgment."]}`.
- `classifyItem {"itemId":"item-2","category":"technical"}` → `{"ok":true,"itemId":"item-2","category":"technical"}`.
- `draftReply {"itemId":"item-3","message":"…"}` → `{"ok":true,"draftId":"draft-item-3"}`.
- `sendReply {"itemId":"item-3","draftId":"draft-item-3"}` → `{"sent":true,"itemId":"item-3","draftId":"draft-item-3"}`.
- Results are compared as marshalled JSON strings, so a wrong tag or an extra `ok` fails the test.

**Strict input** (a table, run against every tool where it applies)
- A missing required field is an error naming it. This includes `draftReply` with no `message`.
- `null` is an error.
- `""` is an error. This includes `searchKnowledgeBase {"query":""}`.
- A wrong type (`"itemId":5`) is an error.
- An unknown field (`"priority":"high"`) is an error.
- A key in the wrong case (`"ItemID":"item-1"`) is an error.
- A category of `"refunds"` is an error.

**Prompt**
- `NewSupportAgent` sends `agents.SupportPrompt` as the system message. It equals the course's text and names each tool the agent offers.

## Most important file changes

| File | Change |
|---|---|
| `internal/agent/` → `internal/agents/` | renamed; `agent.go` loses `Tool` |
| `internal/agents/support.go`, `support_test.go` | new: `NewSupportAgent`, later `SupportPrompt` |
| `internal/tools/tool.go` | new: `Tool` (moved from `agent.go`) |
| `internal/tools/schema.go` | new: schema builders and `strict` |
| `internal/tools/{search,classify,draft,send}.go` and their `_test.go` | new: one file per tool |
| `internal/tools/helpers_test.go` | new: `checkSpec`, `runTool` |
| `docs/ae-plan/lesson-1-harness.md` | subtask 6 marked done, plus a "How subtask 6 landed" note |

About 150 lines of code and 260 of tests, plus the rename of `internal/agent` (about 190 changed lines, all identifiers). That's over 500 counted naively, but the rename is mechanical, so it's still one PR.

## Open questions

- None. André decided on 2026-11-02 that keys must match exactly. `ItemID` for `itemId` is an error, enforced by the helper's keys pass.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit. `agents.NewSupportAgent` is the entry point subtask 7 will call (since the layout change). The tool specs come first, with every `Run` as a placeholder, and each later subtask replaces one placeholder with the real tool.

1. ✅ **The four tool specs** (`c270361`, as `tools.Support()` before the layout change).
   - Names, descriptions and schemas are real.
   - Each `Run` is a placeholder that returns an error, "not built yet".
   - Red: the spec tests (order, descriptions, `required`, `enum`, `additionalProperties`, `minLength`, the `query` description).
   - **When this lands:** `go test ./internal/tools/...` lists the spec tests. Calling any tool gives an error.
2. ✅ **`classifyItem`, with the strict decode helper** (`0c5b662`).
   - Then the layout-change refactor, one commit, no behaviour change (`625b1f4`, docs in `b124f63`).
   - Red: the result test, then the strict-input table for `classifyItem` (missing, `null`, `""`, wrong type, unknown field, wrong-case key, bad category).
   - Green: the two-pass helper (exact keys, then values), plus the category check against the shared list.
   - **When this lands:** `classifyItem` returns the course's result and rejects bad input. The other three are still placeholders.
3. ✅ **`draftReply` and `sendReply`** (`6153656`).
   - Two tiny echoes on the same helper, in `draft.go` and `send.go`.
   - Red: their results, plus the missing-`message` and missing-`draftId` rows.
   - **When this lands:** three tools are real.
4. ✅ **`searchKnowledgeBase` and the ordered knowledge base**, in `search.go` (`6bb87f8`). The "not built yet" placeholder went with it.
   - Red: the pricing, two-match-order, fallback and empty-query tests.
   - **When this lands:** no placeholders are left.
5. ✅ **`agents.SupportPrompt`.**
   - Red: a test in `agents/support_test.go` that `NewSupportAgent` sends it as the system message, that it matches the course text, and that it names every tool the agent offers.
   - Green: the const, passed to `NewAgent` in place of `""`.
   - Then update `docs/ae-plan/lesson-1-harness.md`: subtask 6 done, a "How subtask 6 landed" note, and `internal/agent` → `internal/agents` in its file table.
   - **When this lands:** everything subtask 7 needs exists: `handlers.NewMux(ctx, bus, agents.NewSupportAgent(model).Run)`. Running `go test ./...` passes, and the server still runs the stub.
   - **Size:** about 410 lines with tests, plus the mechanical rename. One PR.
