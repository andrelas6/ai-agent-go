package main

import "andrelas6/ai-agent-go/internal/events"

func formatEvent(e events.Event) string {
	return string(e.Type)
}
