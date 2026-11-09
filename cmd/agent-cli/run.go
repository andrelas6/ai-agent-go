package main

import (
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

type runner func(ctx context.Context, workflowID, input string, emit func(events.Event))

func runTask(ctx context.Context, task string, runAgent runner, out io.Writer) int {
	completed := false
	failed := false
	runAgent(ctx, uuid.NewString(), task, func(e events.Event) {
		if e.Type == events.WorkflowCompleted {
			completed = true
		}
		if e.Type == events.WorkflowFailed {
			failed = true
		}
		fmt.Fprintln(out, formatEvent(e))
	})
	if failed || !completed {
		return 1
	}
	return 0
}
