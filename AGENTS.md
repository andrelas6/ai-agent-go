# AGENTS.md

## Dependencies

Use the Go standard library only. Do not add third-party modules.

There are two exceptions:

- the OpenRouter SDK (`github.com/OpenRouterTeam/go-sdk`), which the agent uses to talk to the model;
- `github.com/google/uuid`, for event and workflow ids.

If something seems to need a library (YAML parsing, CLI flags, test helpers), write it with the standard library instead.
