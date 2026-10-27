package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"andrelas6/ai-agent-go/internal/events"

	"github.com/google/uuid"
)

type Runner func(ctx context.Context, workflowID string, input string, emit func(events.Event))

type EventBus interface {
	Subscribe(lastEventID string) (history []events.Event, liveEventCh <-chan events.Event, unsubscribe func())
	Publish(events.Event)
	Clear()
}

func NewMux(ctx context.Context, bus EventBus, run Runner) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/tasks", handlePostTasks(ctx, bus, run))
	mux.HandleFunc("POST /api/clear", handlePostClear(bus))
	mux.HandleFunc("GET /api/events", handleEvents(ctx, bus))
	mux.HandleFunc("GET /health", func(resWriter http.ResponseWriter, request *http.Request) {
		resWriter.Write([]byte("ok"))
	})

	return mux
}

func handleEvents(ctx context.Context, bus EventBus) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		lastEventID := r.Header.Get("Last-Event-ID")
		history, eventStream, unsubscribe := bus.Subscribe(lastEventID)
		defer unsubscribe()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		// no "Connection: keep-alive": it's forbidden in HTTP/2, and HTTP/1.1 keeps the stream open without it

		rc := http.NewResponseController(w)

		// send response to client before first message - that tells clients are connected properly
		rc.Flush()

		for _, event := range history {
			if err := flushEventData(w, event, rc); err != nil {
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-r.Context().Done():
				return
			case event, ok := <-eventStream:
				// channel closed
				if !ok {
					return
				}
				if err := flushEventData(w, event, rc); err != nil {
					return
				}
			}
		}
	}
}

func flushEventData(w http.ResponseWriter, event events.Event, rc *http.ResponseController) error {
	data, err := json.Marshal(event)
	// TODO: improve this error handling
	if err != nil {
		// no op for now
		log.Printf("error marshaling %v", err)
		return err
	}

	// return write and flush errors so the caller stops looping (and unsubscribes) on a dead connection
	if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", event.ID, data); err != nil {
		return err
	}
	return rc.Flush()
}

func handlePostTasks(ctx context.Context, b EventBus, runner Runner) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var reqBody struct {
			Input string `json:"input"`
		}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&reqBody)
		if err != nil {
			log.Printf("error decoding body %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		if reqBody.Input == "" {
			http.Error(w, "missing input", http.StatusBadRequest)
			return
		}

		workflowID := uuid.NewString()
		go runSafely(ctx, runner, workflowID, reqBody.Input, b.Publish)

		writeJSON(w, http.StatusAccepted, map[string]string{"workflowId": workflowID})
	}
}

func runSafely(ctx context.Context, runner Runner, workflowID string, input string, emit func(events.Event)) {
	// a panic in a goroutine kills the whole server, so turn it into workflow.failed for this workflow only
	defer func() {
		if r := recover(); r != nil {
			log.Printf("runner panicked for workflow %s: %v", workflowID, r)
			emit(events.Event{
				Type:       events.WorkflowFailed,
				WorkflowID: workflowID,
				Error:      fmt.Sprintf("runner panicked: %v", r),
			})
		}
	}()
	runner(ctx, workflowID, input, emit)
}

func handlePostClear(bus EventBus) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		bus.Clear()
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
