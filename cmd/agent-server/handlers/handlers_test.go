package handlers_test

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// calling gets an open connection, then posting message receives a dumb text message
func TestGetStreamingEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(newTestMux(ctx, newStubRunner(), bus.New()))
	defer server.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)

	if err != nil {
		t.Fatalf("new request with context %v", err)
	}

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		t.Fatalf("GET /api/events %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected %v. got %v", http.StatusOK, res.StatusCode)
	}

	id, data := readSSEMessage(t, bufio.NewReader(res.Body))

	if id == "" {
		t.Fatalf("expected non blank value")
	}

	if data == "" {
		t.Fatalf("expected non blank value")
	}
}

func newStubRunner() handlers.Runner {
	return func() {}
}

type StubEventBus struct {
	eventStream chan events.Event
}

func TestPostTasksReturnsDummyWorkflowID(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newStubRunner(), bus.New()))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(`{"input":"hi","mode":"default"}`))
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got, want := readBody(t, res), `{"workflowId":"dummy-workflow-id"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestPostClearReturnsOK(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newStubRunner(), bus.New()))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/clear", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/clear: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if got, want := readBody(t, res), `{"ok":true}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestGetTasksIsNotAllowed(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newStubRunner(), bus.New()))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/tasks")
	if err != nil {
		t.Fatalf("GET /api/tasks: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusMethodNotAllowed)
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return strings.TrimSpace(string(body))
}

func newTestMux(ctx context.Context, runner handlers.Runner, bus handlers.EventBus) *http.ServeMux {
	mux := handlers.NewMux(ctx, bus, runner)
	return mux
}

// bufio.Reader helps reading line by line of a *file reader" like res.Body
func readSSEMessage(t *testing.T, r *bufio.Reader) (id, data string) {
	t.Helper()
	for {
		// read each line
		line, err := r.ReadString('\n')

		if err != nil {
			// why nto use %w here instead? isn't %w used for error printing?
			t.Fatalf("read stream %v", err)
		}

		line = strings.TrimSuffix(line, "\n")
		switch {
		case line == "":
			return id, data
		case strings.HasPrefix(line, "id: "):
			id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}
