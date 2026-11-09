package main

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/events"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

func readTask(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read task: %w", err)
	}
	task := strings.TrimSpace(string(data))
	if task == "" {
		return "", errors.New("no task on stdin")
	}
	return task, nil
}

func run(ctx context.Context, task string, m agents.Model, out io.Writer) int {
	failed := false
	agents.NewSupportAgent(m).Run(ctx, uuid.NewString(), task, func(e events.Event) {
		if e.Type == events.WorkflowFailed {
			failed = true
		}
		fmt.Fprintln(out, formatEvent(e))
	})
	if failed {
		return 1
	}
	return 0
}
