package handlers_test

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type runCall struct {
	ctx        context.Context
	workflowID string
	input      string
}

func newFakeRunner(calls chan<- runCall, emits ...events.Event) handlers.Runner {
	return func(ctx context.Context, workflowID string, input string, emit func(events.Event)) {
		if calls != nil {
			select {
			case <-ctx.Done():
			case calls <- runCall{ctx: ctx, workflowID: workflowID, input: input}:
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
	calls := make(chan runCall, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(handlers.NewMux(ctx, bus.New(ctx), newFakeRunner(calls, events.Event{Type: events.WorkflowStarted})))
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

	// ----- FIRST SSE GET ---- //

	// ----- POST MESSAGE ---- //
	postResponse := postTask(ctx, server.URL+"/api/tasks", t)

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

	var call runCall
	select {
	case call = <-calls:
	case <-ctx.Done():
		t.Fatal("runner was not called before the timeout")
	}
	if got.WorkflowId != call.workflowID {
		t.Errorf("%s and %s should have been equal.", got.WorkflowId, call.workflowID)
	}
	if call.input != "test input" {
		t.Errorf("runner input = %q, want %q", call.input, "test input")
	}

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

func postTask(ctx context.Context, apiURL string, t *testing.T) *http.Response {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))

	req.Header.Set("Content-Type", "application/json")

	postResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http post %v", err)
	}

	return postResponse
}

func TestRunnerRunsOnTheServerContext(t *testing.T) {
	calls := make(chan runCall, 1)
	serverCtx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	server := httptest.NewServer(handlers.NewMux(serverCtx, bus.New(serverCtx), newFakeRunner(calls)))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
	}

	var call runCall
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
	server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(nil)))
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
	server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(nil)))
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
	lastEventID  string
}

var _ handlers.EventBus = (*fakeBus)(nil)

func newFakeBus() *fakeBus {
	return &fakeBus{
		live:         make(chan events.Event),
		unsubscribed: make(chan struct{}),
	}
}

func (f *fakeBus) Subscribe(lastEventID string) ([]events.Event, <-chan events.Event, func()) {
	f.lastEventID = lastEventID
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
			calls := make(chan runCall, 1)
			server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(calls)))
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
			case call := <-calls:
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
	server := httptest.NewServer(handlers.NewMux(ctx, b, newFakeRunner(nil)))
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
	server := httptest.NewServer(handlers.NewMux(ctx, b, newFakeRunner(nil)))
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
	server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(nil)))
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

func TestHealthReturnsOK(t *testing.T) {
	server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(nil)))
	defer server.Close()

	res, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := readBody(t, res); got != "ok" {
		t.Errorf("body = %q, want %q", got, "ok")
	}
}

func TestOldRoutesAreGone(t *testing.T) {
	server := httptest.NewServer(handlers.NewMux(t.Context(), bus.New(t.Context()), newFakeRunner(nil)))
	defer server.Close()

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/sse"},
		{http.MethodPost, "/messages"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), c.method, server.URL+c.path, strings.NewReader(`{"message":"hi"}`))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", c.method, c.path, err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusNotFound {
				t.Errorf("status = %d, want %d", res.StatusCode, http.StatusNotFound)
			}
		})
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

func TestRunnerPanicBecomesWorkflowFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	b := bus.New(ctx)
	panickingRunner := func(ctx context.Context, workflowID string, input string, emit func(events.Event)) {
		panic("boom")
	}
	server := httptest.NewServer(handlers.NewMux(ctx, b, panickingRunner))
	t.Cleanup(server.Close)

	stream := openEventStream(t, ctx, server.URL)

	res, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	res.Body.Close()

	event := readEvent(t, stream)
	if event.Type != events.WorkflowFailed {
		t.Fatalf("event type = %q, want %q", event.Type, events.WorkflowFailed)
	}
	if event.WorkflowID != body.WorkflowID {
		t.Errorf("workflowId = %q, want %q", event.WorkflowID, body.WorkflowID)
	}
	if !strings.Contains(event.Error, "boom") {
		t.Errorf("error = %q, want it to mention the panic", event.Error)
	}

	health, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health after the panic: %v", err)
	}
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", health.StatusCode, http.StatusOK)
	}
}

type brokenWriter struct {
	header http.Header
}

func (w *brokenWriter) Header() http.Header        { return w.header }
func (w *brokenWriter) WriteHeader(statusCode int) {}
func (w *brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection is gone")
}
func (w *brokenWriter) FlushError() error { return nil }

func TestEventsStopsWhenWritingFails(t *testing.T) {
	fake := newFakeBus()
	mux := handlers.NewMux(t.Context(), fake, newFakeRunner(nil))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/events", nil)

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(&brokenWriter{header: http.Header{}}, req)
		close(done)
	}()

	select {
	case fake.live <- events.Event{Type: events.Log, Message: "lost"}:
	case <-time.After(time.Second):
		t.Fatal("handler never read the live event")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler kept running after a failed write")
	}
	select {
	case <-fake.unsubscribed:
	default:
		t.Error("handler returned without unsubscribing")
	}
}

func TestBusChannelCloseHandlerReturns(t *testing.T) {
	// ----- Setup ------ //
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	liveCh := make(chan events.Event)
	unsubscribedCh := make(chan struct{})
	b := &fakeBus{
		history:      []events.Event{},
		live:         liveCh,
		unsubscribed: unsubscribedCh,
	}

	server := httptest.NewServer(handlers.NewMux(ctx, b, newFakeRunner(nil, events.Event{Type: events.WorkflowStarted})))
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

	close(liveCh)

	gotEofCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, getResponse.Body)
		gotEofCh <- err
	}()

	defer getResponse.Body.Close()

	if getResponse.StatusCode != http.StatusOK {
		t.Fatalf("got %v. expected %v", getResponse.StatusCode, http.StatusOK)
	}

	select {
	case err := <-gotEofCh:
		if err != nil {
			t.Fatalf("stream ended with error %v", err)
		}
	case <-time.After(time.Second * 1):
		t.Fatalf("timeout 1s")
	}
}

func TestGetHandlerReadsLastEventID(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"header set", "1234", "1234"},
		{"no header", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// create server
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*2)
			defer cancel()

			b := newFakeBus()
			b.lastEventID = "Subscribe not called"

			server := httptest.NewServer(handlers.NewMux(ctx, b, newFakeRunner(nil, events.Event{Type: events.WorkflowStarted})))
			defer server.Close()
			// create request
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
			if err != nil {
				t.Fatalf("got error creating request %v", err)
			}
			if c.header != "" {
				req.Header.Set("Last-Event-ID", c.header)
			}

			// fire requset
			resp, err := http.DefaultClient.Do(req)
			// needs to close the response otherwise it stays open
			if err != nil {
				t.Fatalf("got error making the request %v", err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got %v. expected %v", resp.StatusCode, http.StatusOK)
			}

			if b.lastEventID != c.want {
				t.Fatalf("got %q. expected %q", b.lastEventID, c.want)
			}
		})
	}
}

func TestGetHandlerGetsNextEventAfterLastEvent(t *testing.T) {
	// setup
	// create server
	b := bus.New(t.Context())
	b.Publish(events.Event{Message: "1"})
	b.Publish(events.Event{Message: "2"})
	b.Publish(events.Event{Message: "3"})
	b.Publish(events.Event{Message: "4"})
	b.Publish(events.Event{Message: "5"})

	history, _, unsubscribe := b.Subscribe("")
	unsubscribe()

	server := httptest.NewServer(handlers.NewMux(t.Context(), b, newFakeRunner(nil)))
	defer server.Close()
	// make request with lastEvent ID
	//
	cases := []struct {
		name                         string
		lastEventID                  string
		expectedFirstIncomingEventID string
	}{
		{
			name:                         "no last event id",
			lastEventID:                  "",
			expectedFirstIncomingEventID: history[0].ID,
		},
		{
			name:                         "first event id",
			lastEventID:                  history[0].ID,
			expectedFirstIncomingEventID: history[1].ID,
		},
		{
			name:                         "middle event id",
			lastEventID:                  history[2].ID,
			expectedFirstIncomingEventID: history[3].ID,
		},
		{
			name:                         "second to last event id",
			lastEventID:                  history[3].ID,
			expectedFirstIncomingEventID: history[4].ID,
		},
		{
			name:                         "unknown event id",
			lastEventID:                  "not-an-event-id",
			expectedFirstIncomingEventID: history[0].ID,
		},
	}

	// check that events are resumed from lsatevent id
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
			if err != nil {
				t.Fatalf("error creating request %v", err)
			}

			req.Header.Set("Last-Event-ID", c.lastEventID)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("response error %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status code %v not %v", resp.StatusCode, http.StatusOK)
			}

			// reads one message - if id is after lastEventID, all good
			id, _ := readSSEMessage(t, bufio.NewReader(resp.Body))

			if id != c.expectedFirstIncomingEventID {
				t.Fatalf("got %v. expected %v", id, c.expectedFirstIncomingEventID)
			}
		})
	}
}

func TestGetHandlerSendsNothingAfterTheLastEvent(t *testing.T) {
	b := bus.New(t.Context())
	b.Publish(events.Event{Message: "1"})
	b.Publish(events.Event{Message: "2"})

	history, _, unsubscribe := b.Subscribe("")
	unsubscribe()

	server := httptest.NewServer(handlers.NewMux(t.Context(), b, newFakeRunner(nil)))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("error creating request %v", err)
	}
	req.Header.Set("Last-Event-ID", history[1].ID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code %v not %v", resp.StatusCode, http.StatusOK)
	}

	// add a deadline and read everything
	got, err := io.ReadAll(resp.Body)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got error reading the body %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0. got %v", len(got))
	}
}
