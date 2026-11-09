# AGENTS.md

## Dependencies

Use the Go standard library only. Do not add third-party modules.

There are two exceptions:

- the OpenRouter SDK (`github.com/OpenRouterTeam/go-sdk`), which the agent uses to talk to the model;
- `github.com/google/uuid`, for event and workflow ids.

If something seems to need a library (YAML parsing, CLI flags, test helpers), write it with the standard library instead.

## Tests

Every test gets a short comment right above it that says which scenario it checks. A test name can't hold the whole case, and the comment helps whoever reviews the test.

```go
// With -stub set, the server runs the course stub and never needs an API key.
func TestNewRunnerWithStubRunsTheStub(t *testing.T) {
```

This holds even where other instructions say not to add comments.


## Implementation

When you implement, use the ae-implementer and ae-reviewer subagents.
