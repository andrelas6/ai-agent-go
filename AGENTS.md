# AGENTS.md

## Dependencies

Use the Go standard library only. Do not add third-party modules.

The one exception is the OpenRouter SDK (`github.com/OpenRouterTeam/go-sdk`), which the agent uses to talk to the model. If something seems to need a library (YAML parsing, CLI flags, test helpers), write it with the standard library instead.
