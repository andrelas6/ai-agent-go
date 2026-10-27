package handlers_test

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/events"
	"bufio"
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
	serverURL := newServer(t, ctx, bus.New(ctx), newFakeRunner(calls, events.Event{Type: events.WorkflowStarted}))
	// ----- Setup ------ //

	// ----- FIRST SSE GET ---- //
	getResponse := getEvents(t, ctx, serverURL, "")

	if got := getResponse.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream. got %v", got)
	}
	if got := getResponse.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache. got %v", got)
	}

	// ----- FIRST SSE GET ---- //

	// ----- POST MESSAGE ---- //
	postResponse := postTask(t, ctx, serverURL, `{"input":"test input","mode":"test mode"}`)

	if postResponse.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want %d", postResponse.StatusCode, http.StatusAccepted)
	}
	if got := postResponse.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	workflowID := decodeWorkflowID(t, postResponse)
	if workflowID == "" {
		t.Error("workflow id is empty")
	}

	call := waitFor(t, calls, "runner was not called")
	if workflowID != call.workflowID {
		t.Errorf("%s and %s should have been equal.", workflowID, call.workflowID)
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

	err := json.Unmarshal([]byte(sseDataMessage), &eventData)

	if err != nil {
		// QUESTION: why %q here?
		t.Fatalf("unmarshal %q: %v", sseDataMessage, err)
	}

	if eventData.Type != string(events.WorkflowStarted) {
		t.Fatalf("got %v. expected %v", eventData.Type, events.WorkflowStarted)
	}

	if eventData.WorkflowId != workflowID {
		t.Fatalf("got %v. expected %v", eventData.WorkflowId, workflowID)
	}
}

func TestRunnerRunsOnTheServerContext(t *testing.T) {
	calls := make(chan runCall, 1)
	serverCtx, shutdown := context.WithCancel(t.Context())
	defer shutdown()
	serverURL := newServer(t, serverCtx, bus.New(serverCtx), newFakeRunner(calls))

	res := postTask(t, t.Context(), serverURL, `{"input":"hello"}`)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
	}

	call := waitFor(t, calls, "runner was not called")

	if err := call.ctx.Err(); err != nil {
		t.Fatalf("runner ctx ended once the request finished: %v", err)
	}

	shutdown()

	waitFor(t, call.ctx.Done(), "runner ctx was not cancelled after the server shut down")
}

func TestPostClearReturnsOK(t *testing.T) {
	serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(nil))

	res := post(t, t.Context(), serverURL+"/api/clear", "")

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
	serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(nil))

	res := get(t, t.Context(), serverURL+"/api/tasks")

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
	serverURL := newServer(t, t.Context(), fake, newFakeRunner(nil))

	reqCtx, disconnect := context.WithCancel(t.Context())
	getEvents(t, reqCtx, serverURL, "")

	disconnect()

	waitFor(t, fake.unsubscribed, "handler did not unsubscribe after the client disconnected")
}

func TestEventsUnsubscribesWhenServerShutsDown(t *testing.T) {
	fake := newFakeBus()
	serverCtx, shutdown := context.WithCancel(t.Context())
	serverURL := newServer(t, serverCtx, fake, newFakeRunner(nil))

	getEvents(t, t.Context(), serverURL, "")

	shutdown()

	waitFor(t, fake.unsubscribed, "handler did not unsubscribe after the server shut down")
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
			serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(calls))

			res := postTask(t, t.Context(), serverURL, c.body)

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
	b, _ := busWithEvents(t, ctx, "from-history")
	serverURL := newServer(t, ctx, b, newFakeRunner(nil))

	stream := openEventStream(t, ctx, serverURL)
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
	b, _ := busWithEvents(t, ctx, "before-clear")
	serverURL := newServer(t, ctx, b, newFakeRunner(nil))

	post(t, ctx, serverURL+"/api/clear", "")

	stream := openEventStream(t, ctx, serverURL)
	b.Publish(events.Event{Type: events.Log, Message: "after-clear"})

	if got := readEvent(t, stream).Message; got != "after-clear" {
		t.Errorf("first event after Clear = %q, want %q", got, "after-clear")
	}
}

func TestPostTasksReturnsAUUIDWorkflowID(t *testing.T) {
	serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(nil))

	workflowID := decodeWorkflowID(t, postTask(t, t.Context(), serverURL, `{"input":"hello"}`))

	if _, err := uuid.Parse(workflowID); err != nil {
		t.Errorf("workflowId %q is not a UUID: %v", workflowID, err)
	}
}

func TestHealthReturnsOK(t *testing.T) {
	serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(nil))

	res := get(t, t.Context(), serverURL+"/health")

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := readBody(t, res); got != "ok" {
		t.Errorf("body = %q, want %q", got, "ok")
	}
}

func TestOldRoutesAreGone(t *testing.T) {
	serverURL := newServer(t, t.Context(), bus.New(t.Context()), newFakeRunner(nil))

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/sse"},
		{http.MethodPost, "/messages"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), c.method, serverURL+c.path, strings.NewReader(`{"message":"hi"}`))
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

func TestRunnerPanicBecomesWorkflowFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	panickingRunner := func(ctx context.Context, workflowID string, input string, emit func(events.Event)) {
		panic("boom")
	}
	serverURL := newServer(t, ctx, bus.New(ctx), panickingRunner)

	stream := openEventStream(t, ctx, serverURL)

	workflowID := decodeWorkflowID(t, postTask(t, ctx, serverURL, `{"input":"hello"}`))

	event := readEvent(t, stream)
	if event.Type != events.WorkflowFailed {
		t.Fatalf("event type = %q, want %q", event.Type, events.WorkflowFailed)
	}
	if event.WorkflowID != workflowID {
		t.Errorf("workflowId = %q, want %q", event.WorkflowID, workflowID)
	}
	if !strings.Contains(event.Error, "boom") {
		t.Errorf("error = %q, want it to mention the panic", event.Error)
	}

	health := get(t, ctx, serverURL+"/health")
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

	waitFor(t, done, "handler kept running after a failed write")
	select {
	case <-fake.unsubscribed:
	default:
		t.Error("handler returned without unsubscribing")
	}
}

func TestEventsEndsWhenBusClosesStream(t *testing.T) {
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

	serverURL := newServer(t, ctx, b, newFakeRunner(nil, events.Event{Type: events.WorkflowStarted}))
	// ----- Setup ------ //

	// ----- FIRST SSE GET ---- //
	getResponse := getEvents(t, ctx, serverURL, "")

	close(liveCh)

	gotEofCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, getResponse.Body)
		gotEofCh <- err
	}()

	if err := waitFor(t, gotEofCh, "stream did not end"); err != nil {
		t.Fatalf("stream ended with error %v", err)
	}
}

func TestEventsReadsLastEventID(t *testing.T) {
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

			serverURL := newServer(t, ctx, b, newFakeRunner(nil, events.Event{Type: events.WorkflowStarted}))
			// create request
			// fire request
			getEvents(t, ctx, serverURL, c.header)

			if b.lastEventID != c.want {
				t.Fatalf("got %q. expected %q", b.lastEventID, c.want)
			}
		})
	}
}

func TestEventsStartsAfterLastEventID(t *testing.T) {
	// setup
	// create server
	b, history := busWithEvents(t, t.Context(), "1", "2", "3", "4", "5")

	serverURL := newServer(t, t.Context(), b, newFakeRunner(nil))
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

	// check that events are resumed from last event id
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
			defer cancel()
			resp := getEvents(t, ctx, serverURL, c.lastEventID)

			// reads one message - if id is after lastEventID, all good
			id, _ := readSSEMessage(t, bufio.NewReader(resp.Body))

			if id != c.expectedFirstIncomingEventID {
				t.Fatalf("got %v. expected %v", id, c.expectedFirstIncomingEventID)
			}
		})
	}
}

func TestEventsSendsNothingAfterTheLastEvent(t *testing.T) {
	b, history := busWithEvents(t, t.Context(), "1", "2")

	serverURL := newServer(t, t.Context(), b, newFakeRunner(nil))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	resp := getEvents(t, ctx, serverURL, history[1].ID)

	// add a deadline and read everything
	got, err := io.ReadAll(resp.Body)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got error reading the body %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected 0. got %v", len(got))
	}
}

func newServer(t *testing.T, ctx context.Context, b handlers.EventBus, run handlers.Runner) string {
	t.Helper()
	server := httptest.NewServer(handlers.NewMux(ctx, b, run))
	t.Cleanup(server.Close)
	return server.URL
}

func getEvents(t *testing.T, ctx context.Context, serverURL, lastEventID string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	// needs to close the response otherwise it stays open
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	return res
}

func openEventStream(t *testing.T, ctx context.Context, serverURL string) *bufio.Reader {
	t.Helper()
	return bufio.NewReader(getEvents(t, ctx, serverURL, "").Body)
}

func post(t *testing.T, ctx context.Context, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func get(t *testing.T, ctx context.Context, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func postTask(t *testing.T, ctx context.Context, serverURL, body string) *http.Response {
	t.Helper()
	return post(t, ctx, serverURL+"/api/tasks", body)
}

func decodeWorkflowID(t *testing.T, res *http.Response) string {
	t.Helper()
	var body struct {
		WorkflowID string `json:"workflowId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body.WorkflowID
}

func busWithEvents(t *testing.T, ctx context.Context, messages ...string) (*bus.Bus, []events.Event) {
	t.Helper()
	b := bus.New(ctx)
	for _, m := range messages {
		b.Publish(events.Event{Type: events.Log, Message: m})
	}
	history, _, unsubscribe := b.Subscribe("")
	unsubscribe()
	return b, history
}

func waitFor[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatalf("%s within 1s", what)
		var zero T
		return zero
	}
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
