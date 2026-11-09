package main

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/model"
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is not set")
		os.Exit(1)
	}
	modelID := os.Getenv("OPENROUTER_MODEL")
	if modelID == "" {
		modelID = model.DefaultModel
	}

	task, err := readTask(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	fmt.Fprintln(os.Stderr, "agent: "+modelID)
	code := run(ctx, task, agents.NewSupportAgent(model.New(apiKey, modelID)).Run, os.Stdout)
	stop()
	os.Exit(code)
}
