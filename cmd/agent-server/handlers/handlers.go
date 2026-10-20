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
	Subscribe() (history []events.Event, readStream <-chan events.Event, unsubscribe func())
	Publish(events.Event)
	Clear()
}

func NewMux(ctx context.Context, bus EventBus, run Runner) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/tasks", handlePostTasks(bus))
	mux.HandleFunc("POST /api/clear", handlePostClear(bus))
	mux.HandleFunc("GET /api/events", handleGetStreamingEvents(ctx, bus))
	mux.HandleFunc("GET /health", func(resWriter http.ResponseWriter, request *http.Request) {
		resWriter.Write([]byte("ok"))
	})

	return mux
}

func handleGetStreamingEvents(ctx context.Context, bus EventBus) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		history, eventStream, unsubscribe := bus.Subscribe()
		defer unsubscribe()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		rc := http.NewResponseController(w)

		// send response to client before first message - that tells clients are connected properly
		rc.Flush()

		// flush history first
		for _, event := range history {
			flushEventData(w, event, rc)
		}

		for {
			select {
			case <-ctx.Done():
				rc.Flush()
				return
			case <-r.Context().Done():

				rc.Flush()
				return
			case event := <-eventStream:
				flushEventData(w, event, rc)
			}
		}
	}
}

func flushEventData(w http.ResponseWriter, event events.Event, rc *http.ResponseController) error {
	data, err := json.Marshal(event)
	// TODO: improve this error handling
	if err != nil {
		// no op for now
		fmt.Printf("error marshaling %v", err)
		return err
	}

	fmt.Fprintf(w, "id: %s\n", event.ID)
	fmt.Fprintf(w, "data: %s\n", data)
	fmt.Fprintf(w, "\n")
	rc.Flush()
	return nil
}

func handlePostTasks(b EventBus) func(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Input string `json:"input"`
		// QUESTION: What's the purpose of mode?
		Mode string `json:"mode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&reqBody)
		defer r.Body.Close()
		if err != nil {
			fmt.Printf("error decoding body %v", err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}

		if reqBody.Input == "" {
			http.Error(w, "missing input", http.StatusBadRequest)
			return
		}

		workflowId := uuid.NewString()
		// mocked event
		eventStarted := events.Event{
			ID:         uuid.NewString(),
			WorkflowID: workflowId,
			Type:       events.WorkflowStarted,
			Input:      reqBody.Input,
		}
		eventLog := events.Event{
			ID:         uuid.NewString(),
			WorkflowID: workflowId,
			Type:       events.Log,
			Message:    "no model yet",
			Input:      reqBody.Input,
		}
		eventCompleted := events.Event{
			ID:         uuid.NewString(),
			WorkflowID: workflowId,
			Type:       events.WorkflowCompleted,
			Input:      reqBody.Input,
		}

		go func() {
			b.Publish(eventStarted)
			b.Publish(eventLog)
			b.Publish(eventCompleted)
		}()

		writeJSON(w, http.StatusAccepted, map[string]string{"workflowId": workflowId})
	}
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
