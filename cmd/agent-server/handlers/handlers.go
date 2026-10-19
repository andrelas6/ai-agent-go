package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"andrelas6/ai-agent-go/internal/events"

	"github.com/google/uuid"
)

type Runner func()

// pub/sub
type EventBus interface {
	Subscribe() (readStream <-chan events.Event, unsubscribe func())
	Publish(events.Event)
	Clear()
}

func NewMux(ctx context.Context, bus EventBus, run Runner) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/tasks", handlePostTasks())
	mux.HandleFunc("POST /api/clear", handlePostClear())
	mux.HandleFunc("GET /api/events", handleGetStreamingEvents(ctx, bus))

	return mux
}

func handleGetStreamingEvents(ctx context.Context, bus EventBus) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		rc := http.NewResponseController(w)

		fmt.Fprintf(w, "id: %s\n", uuid.New().String())
		fmt.Fprintf(w, "data: %s\n", "log(no model yet)")
		fmt.Fprintf(w, "\n")
		rc.Flush()
	}
}

func handlePostTasks() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, map[string]string{"workflowId": "dummy-workflow-id"})
	}
}

func handlePostClear() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
