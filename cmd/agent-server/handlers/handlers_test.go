package handlers_test

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// calling gets an open connection, then posting message receives a dumb text message
func TestGetStreamingEvents(t *testing.T) {
	// ----- Setup ------ //
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(newTestMux(ctx, newStubRunner(), bus.New(ctx)))
	defer server.Close()
	// ----- Setup ------ //

	// ----- FIRST SSE GET ---- //
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)

	if err != nil {
		t.Fatalf("new request with context %v", err)
	}

	getResponse, err := http.DefaultClient.Do(req)

	if err != nil {
		t.Fatalf("GET /api/events %v", err)
	}

	defer getResponse.Body.Close()

	if getResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected %v. got %v", http.StatusOK, getResponse.StatusCode)
	}

	if got := getResponse.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream. got %v", got)
	}
	if got := getResponse.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache. got %v", got)
	}

	if got := getResponse.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("expected keep-alive. got %v", got)
	}
	// ----- FIRST SSE GET ---- //

	// ----- POST MESSAGE ---- //
	data := struct {
		Input string `json:"input"`
		Mode  string `json:"mode"`
	}{
		Input: "test input",
		Mode:  "test mode",
	}
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal("error enconding body")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/tasks", bytes.NewReader(body))

	req.Header.Set("Content-Type", "application/json")

	postResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http post %v", err)
	}

	if postResponse.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want %d", postResponse.StatusCode, http.StatusAccepted)
	}
	if got := postResponse.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var got struct {
		WorkflowId string `json:"workflowId"`
	}
	if err := json.NewDecoder(postResponse.Body).Decode(&got); err != nil {
		t.Errorf("error decoding %v", err)
	}

	if got.WorkflowId == "" {
		t.Error("workflow id is empty", err)
	}

	// ----- POST MESSAGE ---- //

	// ----- SECOND SSE GET ---- //
	var eventData struct {
		Type       string `json:"type"`
		WorkflowId string `json:"workflowId"`
	}

	id, sseDataMessage := readSSEMessage(t, bufio.NewReader(getResponse.Body))

	if id == "" {
		t.Fatal("id is blank")
	}

	err = json.Unmarshal([]byte(sseDataMessage), &eventData)

	if err != nil {
		// QUESTION: why %q here?
		t.Fatalf("unmarshal %q: %v", sseDataMessage, err)
	}

	if eventData.Type != string(events.WorkflowStarted) {
		t.Fatalf("got %v. expected %v", eventData.Type, events.WorkflowStarted)
	}

	if eventData.WorkflowId != got.WorkflowId {
		t.Fatalf("got %v. expected %v", eventData.WorkflowId, got.WorkflowId)
	}
}

func newStubRunner() handlers.Runner {
	return func() {}
}

func TestPostClearReturnsOK(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newStubRunner(), bus.New(t.Context())))
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
	server := httptest.NewServer(newTestMux(t.Context(), newStubRunner(), bus.New(t.Context())))
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

type fakeBus struct {
	history      []events.Event
	live         chan events.Event
	unsubscribed chan struct{}
}

var _ handlers.EventBus = (*fakeBus)(nil)

func newFakeBus() *fakeBus {
	return &fakeBus{
		live:         make(chan events.Event),
		unsubscribed: make(chan struct{}),
	}
}

func (f *fakeBus) Subscribe() ([]events.Event, <-chan events.Event, func()) {
	return f.history, f.live, func() { close(f.unsubscribed) }
}

func (f *fakeBus) Publish(events.Event) {}

func (f *fakeBus) Clear() {}

func TestEventsUnsubscribesWhenClientDisconnects(t *testing.T) {
	fake := newFakeBus()
	server := httptest.NewServer(handlers.NewMux(t.Context(), fake, newStubRunner()))
	defer server.Close()

	reqCtx, disconnect := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer res.Body.Close()

	disconnect()

	select {
	case <-fake.unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("handler did not unsubscribe within 1s of the client disconnecting")
	}
}

func TestEventsUnsubscribesWhenServerShutsDown(t *testing.T) {
	fake := newFakeBus()
	serverCtx, shutdown := context.WithCancel(t.Context())
	server := httptest.NewServer(handlers.NewMux(serverCtx, fake, newStubRunner()))
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer res.Body.Close()

	shutdown()

	select {
	case <-fake.unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("handler did not unsubscribe within 1s of the server shutting down")
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
