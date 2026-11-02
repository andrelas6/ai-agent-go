# ae-plan: The Lesson 1 tools and system prompt

> **Status:** Draft
> **Last updated:** 2026-11-02
> **From:** ae-init/lesson-1-tools.md, which is subtask 6 of ae-plan/lesson-1-harness.md, on `main` at `0958115`

## What's being built

The course's four support-triage tools (`searchKnowledgeBase`, `classifyItem`, `draftReply`, `sendReply`) and its system prompt, in Go. The tools come from one constructor, `tools.Support()`, which subtask 7 will hand to `agent.NewAgent`. They're strict about input. A bad call returns an error, and the loop already turns that into `tool.failed`. Only tests prove it here. The server keeps running the stub, and the CLI is untouched.

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
- **Where things go:**
  - the tools go in `internal/tools`;
  - the prompt goes in `internal/agent`, as an exported const.
- **Unit tests only.** No test runs the tools inside `agent.Run`.
- **Out of scope:**
  - wiring into `agent-server` (subtask 7);
  - the sample task file (subtask 8);
  - any safety check on `sendReply`.
- **No comments** in new code. André's existing comments stay.

## What I found in the codebase

- **`internal/agent/agent.go`**:
  - `Tool{Name, Description, Parameters map[string]any, Run func(ctx, json.RawMessage) (any, error)}`;
  - `runTool` rejects an unknown name, empty args and invalid JSON before `Run` is called, then runs `json.Marshal` on what `Run` returns. A tool can therefore return a struct with camelCase `json` tags and never sees broken JSON.
- **Imports:** `internal/tools` will import `internal/agent` for `agent.Tool`. `agent` doesn't import `tools`, so there's no cycle. The prompt test can live in `tools` and check that `agent.SystemPrompt` names every tool `Support()` returns.
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

`internal/tools/support.go` has:
- `Support() []agent.Tool`, returning the four tools in the course's order;
- one small private args struct per tool, with camelCase `json` tags;
- one shared helper that decodes args strictly, in two passes:
  - **keys.** Decode into `map[string]json.RawMessage`. Any key that isn't exactly one of the schema's `properties` is an error, which catches unknown fields and `ItemID`-style case variants;
  - **values.** Decode into the args struct. Type errors are passed through. Then check that each required field is non-empty, and name it in the error.
  
  The helper reads the key list from the tool's own schema, so the schema and the check can't drift.
- the knowledge base as an ordered slice of `{key, article}`.

`searchKnowledgeBase` lower-cases the query and keeps every article whose key it contains, in slice order. With no hit it returns the fallback.

`classifyItem` checks the category against the same four values the schema's `enum` lists, taken from one shared list so the two can't drift.

`agent.SystemPrompt` is a const in a new `internal/agent/prompt.go`.

## Red / Green

**Tool specs**
- `Support()` returns `searchKnowledgeBase`, `classifyItem`, `draftReply`, `sendReply` in that order, each with the course's description verbatim.
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
- `agent.SystemPrompt` equals the course's text, and it names each tool `Support()` returns.

## Most important file changes

| File | Change |
|---|---|
| `internal/tools/support.go` | new: `Support()`, the four tools, the strict decode helper, the knowledge base |
| `internal/tools/support_test.go` | new |
| `internal/agent/prompt.go` | new: `SystemPrompt` |
| `docs/ae-plan/lesson-1-harness.md` | subtask 6 marked done, plus a "How subtask 6 landed" note |

About 150 lines of code and 260 of tests: one PR, under 500.

## Open questions

- None. André decided on 2026-11-02 that keys must match exactly. `ItemID` for `itemId` is an error, enforced by the helper's keys pass.

## Subtasks

Each item is one commit, ordered outside-in. The program builds and runs after every commit. `Support()` is the entry point subtask 7 will call. It comes first, with every `Run` as a placeholder, and each later subtask replaces one placeholder with the real tool.

1. **`tools.Support()` with the four specs.**
   - Names, descriptions and schemas are real.
   - Each `Run` is a placeholder that returns an error, "not built yet".
   - Red: the spec tests (order, descriptions, `required`, `enum`, `additionalProperties`, `minLength`, the `query` description).
   - **When this lands:** `go test ./internal/tools/...` lists the spec tests. Calling any tool gives an error.
2. **`classifyItem`, with the strict decode helper.**
   - Red: the result test, then the strict-input table for `classifyItem` (missing, `null`, `""`, wrong type, unknown field, wrong-case key, bad category).
   - Green: the two-pass helper (exact keys, then values), plus the category check against the shared list.
   - **When this lands:** `classifyItem` returns the course's result and rejects bad input. The other three are still placeholders.
3. **`draftReply` and `sendReply`.**
   - Two tiny echoes on the same helper.
   - Red: their results, plus the missing-`message` and missing-`draftId` rows.
   - **When this lands:** three tools are real.
4. **`searchKnowledgeBase` and the ordered knowledge base.**
   - Red: the pricing, two-match-order, fallback and empty-query tests.
   - **When this lands:** no placeholders are left.
5. **`agent.SystemPrompt`.**
   - Red: the test in `support_test.go` that it matches the course text and names every tool in `Support()`.
   - Then update `docs/ae-plan/lesson-1-harness.md` (subtask 6 done, "How subtask 6 landed").
   - **When this lands:** everything subtask 7 needs exists: `agent.NewAgent(agent.SystemPrompt, model, tools.Support()...)`. Running `go test ./...` passes, and the server still runs the stub.
   - **Size:** about 410 lines with tests, under 500, so it's one PR.
