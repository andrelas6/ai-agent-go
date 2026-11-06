package main

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/model"
	"errors"
)

func newRunner(stub bool, apiKey, modelID string) (handlers.Runner, string, error) {
	if stub {
		return runStub, "running the stub", nil
	}
	if apiKey == "" {
		return nil, "", errors.New("OPENROUTER_API_KEY is not set: set it, or start the server with -stub")
	}
	agent := agents.NewSupportAgent(model.New(apiKey, modelID))
	return agent.Run, "agent: " + modelID, nil
}
