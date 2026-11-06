package model

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/tools"
	"context"
	"errors"

	openrouter "github.com/OpenRouterTeam/go-sdk"
)

const DefaultModel = "z-ai/glm-5.3-flash"

type OpenRouter struct {
	client  *openrouter.OpenRouter
	modelID string
}

func New(apiKey, modelID string, opts ...openrouter.SDKOption) *OpenRouter {
	options := append([]openrouter.SDKOption{openrouter.WithSecurity(apiKey)}, opts...)
	return &OpenRouter{
		client:  openrouter.New(options...),
		modelID: modelID,
	}
}

func (m *OpenRouter) Next(ctx context.Context, messages []agents.Message, toolList []tools.Tool) (agents.Message, error) {
	return agents.Message{}, errors.New("model not built yet")
}
