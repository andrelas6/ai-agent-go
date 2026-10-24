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

	"github.com/google/uuid"
)

type fakeRunnerReply struct {
	ctx        context.Context
	workflowID string
	input      string
}

func newFakeRunner(calls chan<- fakeRunnerReply, emits ...events.Event) handlers.Runner {
	return func(ctx context.Context, workflowID string, input string, emit func(events.Event)) {
		if calls != nil {
			select {
			case <-ctx.Done():
			case calls <- fakeRunnerReply{ctx: ctx, workflowID: workflowID, input: input}:
			}
		}
		for _, e := range emits {
			e.WorkflowID = workflowID
			emit(e)
		}
	}
}

// calling gets an open connection, then posting message receives a dumb text message
func TestGetStreamingEvents(t *testing.T) {
	// ----- Setup ------ //
	fakeRunnerReplyCh := make(chan fakeRunnerReply, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(newTestMux(ctx, newFakeRunner(fakeRunnerReplyCh, events.Event{Type: events.WorkflowStarted}), bus.New(ctx)))
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

	var runnerReply fakeRunnerReply
	select {
	case runnerReply = <-fakeRunnerReplyCh:
	case <-ctx.Done():
		t.Fatal("runner was not called before the timeout")
	}
	if got.WorkflowId != runnerReply.workflowID {
		t.Errorf("%s and %s should have been equal.", got.WorkflowId, runnerReply.workflowID)
	}
	if runnerReply.input != "test input" {
		t.Errorf("runner input = %q, want %q", runnerReply.input, "test input")
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

func TestRunnerRunsOnTheServerContext(t *testing.T) {
	calls := make(chan fakeRunnerReply, 1)
	serverCtx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	server := httptest.NewServer(newTestMux(serverCtx, newFakeRunner(calls), bus.New(serverCtx)))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
	}

	var call fakeRunnerReply
	select {
	case call = <-calls:
	case <-time.After(time.Second):
		t.Fatal("runner was not called within 1s")
	}

	if err := call.ctx.Err(); err != nil {
		t.Fatalf("runner ctx ended once the request finished: %v", err)
	}

	shutdown()

	select {
	case <-call.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("runner ctx was not cancelled within 1s of the server shutting down")
	}
}

func TestPostClearReturnsOK(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newFakeRunner(nil), bus.New(t.Context())))
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
	server := httptest.NewServer(newTestMux(t.Context(), newFakeRunner(nil), bus.New(t.Context())))
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
	server := httptest.NewServer(handlers.NewMux(t.Context(), fake, newFakeRunner(nil)))
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
	server := httptest.NewServer(handlers.NewMux(serverCtx, fake, newFakeRunner(nil)))
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

func TestRunnerIsNotCalledOn400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"invalid JSON", `not json`},
		{"empty input", `{"input":""}`},
		{"no input field", `{"mode":"default"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			replyFakeRunnerCh := make(chan fakeRunnerReply, 1)
			server := httptest.NewServer(newTestMux(t.Context(), newFakeRunner(replyFakeRunnerCh), bus.New(t.Context())))
			defer server.Close()

			res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(c.body))
			if err != nil {
				t.Fatalf("POST /api/tasks %v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
			}

			select {
			case call := <-replyFakeRunnerCh:
				t.Fatalf("runner was called on a bad request: %+v", call)
			case <-time.After(time.Millisecond * 100):
			}
		})
	}
}

func TestEventsSendsHistoryBeforeLiveEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)
	server := httptest.NewServer(newTestMux(ctx, newFakeRunner(nil), b))
	t.Cleanup(server.Close)

	b.Publish(events.Event{Type: events.Log, Message: "from-history"})

	stream := openEventStream(t, ctx, server.URL)
	b.Publish(events.Event{Type: events.Log, Message: "live"})

	for _, want := range []string{"from-history", "live"} {
		if got := readEvent(t, stream).Message; got != want {
			t.Fatalf("got event %q, want %q", got, want)
		}
	}
}

func TestPostClearEmptiesTheHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)
	server := httptest.NewServer(newTestMux(ctx, newFakeRunner(nil), b))
	t.Cleanup(server.Close)

	b.Publish(events.Event{Type: events.Log, Message: "before-clear"})

	res, err := http.Post(server.URL+"/api/clear", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/clear: %v", err)
	}
	res.Body.Close()

	stream := openEventStream(t, ctx, server.URL)
	b.Publish(events.Event{Type: events.Log, Message: "after-clear"})

	if got := readEvent(t, stream).Message; got != "after-clear" {
		t.Errorf("first event after Clear = %q, want %q", got, "after-clear")
	}
}

func TestPostTasksReturnsAUUIDWorkflowID(t *testing.T) {
	server := httptest.NewServer(newTestMux(t.Context(), newFakeRunner(nil), bus.New(t.Context())))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	defer res.Body.Close()

	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, err := uuid.Parse(body.WorkflowID); err != nil {
		t.Errorf("workflowId %q is not a UUID: %v", body.WorkflowID, err)
	}
}

func openEventStream(t *testing.T, ctx context.Context, serverURL string) *bufio.Reader {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return bufio.NewReader(res.Body)
}

func readEvent(t *testing.T, stream *bufio.Reader) events.Event {
	t.Helper()
	_, data := readSSEMessage(t, stream)
	var event events.Event
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	return event
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
