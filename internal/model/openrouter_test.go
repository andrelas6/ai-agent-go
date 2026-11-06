package model_test

import (
	"andrelas6/ai-agent-go/internal/agents"
	"andrelas6/ai-agent-go/internal/model"
	"andrelas6/ai-agent-go/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
)

type capturedRequest struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

func textReplyServer(t *testing.T, content string, requests chan<- capturedRequest) *httptest.Server {
	t.Helper()
	return choicesServer(t, []any{map[string]any{
		"index":         0,
		"finish_reason": "stop",
		"message":       map[string]any{"role": "assistant", "content": content},
	}}, requests)
}

func choicesServer(t *testing.T, choices []any, requests chan<- capturedRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request body is not json: %v", err)
		}
		requests <- capturedRequest{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		w.Header().Set("Content-Type", "application/json")
		reply := map[string]any{
			"id":      "gen-1",
			"object":  "chat.completion",
			"created": 1,
			"model":   "test/model",
			"choices": choices,
		}
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			t.Errorf("writing reply: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%v is not a json object", value)
	}
	return m
}

func newTestModel(srv *httptest.Server) *model.OpenRouter {
	return model.New("test-key", "test/model", openrouter.WithServerURL(srv.URL))
}

func testContext(t *testing.T) (context.Context, context.CancelFunc) {
	return context.WithTimeout(t.Context(), 2*time.Second)
}

// Scenario: Next sends one POST to /chat/completions with the bearer key, the model id, max_tokens, and require_parameters.
func TestNextSendsAuthorizedRequest(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "hello", requests)

	_, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(requests))
	}
	got := <-requests
	if got.method != http.MethodPost || got.path != "/chat/completions" {
		t.Errorf("request was %s %s, want POST /chat/completions", got.method, got.path)
	}
	if got.auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", got.auth, "Bearer test-key")
	}
	if got.body["model"] != "test/model" {
		t.Errorf("model = %v, want test/model", got.body["model"])
	}
	if got.body["max_tokens"] != float64(64000) {
		t.Errorf("max_tokens = %v, want 64000", got.body["max_tokens"])
	}
	if _, ok := got.body["max_completion_tokens"]; ok {
		t.Errorf("max_completion_tokens must not be sent")
	}
	wantProvider := map[string]any{"require_parameters": true}
	if !reflect.DeepEqual(got.body["provider"], wantProvider) {
		t.Errorf("provider = %v, want %v", got.body["provider"], wantProvider)
	}
}

// Scenario: messages keep their order and roles, and an assistant message with tool calls and no text sends no content.
func TestNextSendsMessagesInOrder(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "done", requests)

	messages := []agents.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "list files"},
		{Role: "assistant", ToolCalls: []agents.ToolCall{{ID: "call-1", Name: "list_files", Args: json.RawMessage(`{"path":"."}`)}}},
		{Role: "tool", ToolCallID: "call-1", Content: "a.txt"},
	}
	if _, err := newTestModel(srv).Next(ctx, messages, nil); err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	got := <-requests
	sent, ok := got.body["messages"].([]any)
	if !ok || len(sent) != 4 {
		t.Fatalf("messages = %v, want 4 messages", got.body["messages"])
	}
	system := asMap(t, sent[0])
	if system["role"] != "system" || system["content"] != "be brief" {
		t.Errorf("message 0 = %v", system)
	}
	user := asMap(t, sent[1])
	if user["role"] != "user" || user["content"] != "list files" {
		t.Errorf("message 1 = %v", user)
	}
	assistant := asMap(t, sent[2])
	if assistant["role"] != "assistant" {
		t.Errorf("message 2 role = %v, want assistant", assistant["role"])
	}
	if content, ok := assistant["content"]; ok && content != nil {
		t.Errorf("assistant content = %v, want none", content)
	}
	wantCalls := []any{map[string]any{
		"id":       "call-1",
		"type":     "function",
		"function": map[string]any{"name": "list_files", "arguments": `{"path":"."}`},
	}}
	if !reflect.DeepEqual(assistant["tool_calls"], wantCalls) {
		t.Errorf("tool_calls = %v, want %v", assistant["tool_calls"], wantCalls)
	}
	tool := asMap(t, sent[3])
	if tool["role"] != "tool" || tool["tool_call_id"] != "call-1" || tool["content"] != "a.txt" {
		t.Errorf("message 3 = %v", tool)
	}
}

// Scenario: an assistant message with text keeps its text content.
func TestNextSendsAssistantText(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "done", requests)

	messages := []agents.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "bye"},
	}
	if _, err := newTestModel(srv).Next(ctx, messages, nil); err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	sent, ok := (<-requests).body["messages"].([]any)
	if !ok || len(sent) != 3 {
		t.Fatalf("messages = %v, want 3 messages", sent)
	}
	assistant := asMap(t, sent[1])
	if assistant["role"] != "assistant" || assistant["content"] != "hello" {
		t.Errorf("message 1 = %v", assistant)
	}
}

// Scenario: each tool is sent as a function with its name, description, and schema unchanged.
func TestNextSendsTools(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "ok", requests)

	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"path": map[string]any{"type": "string"}},
		"required":   []any{"path"},
	}
	toolList := []tools.Tool{{Name: "read_file", Description: "reads a file", Parameters: schema}}
	if _, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, toolList); err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	want := []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "reads a file",
			"parameters":  schema,
		},
	}}
	got := (<-requests).body["tools"]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

// Scenario: an unknown role is an error and nothing is sent.
func TestNextRejectsUnknownRole(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "ok", requests)

	_, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "wizard", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("Next returned no error for an unknown role")
	}
	if len(requests) != 0 {
		t.Errorf("server saw %d requests, want 0", len(requests))
	}
}

// Scenario: a text reply comes back as an assistant message with its text and finish reason.
func TestNextReadsTextReply(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "hello there", requests)

	got, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	want := agents.Message{Role: "assistant", Content: "hello there", FinishReason: "stop"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reply = %+v, want %+v", got, want)
	}
}

// Scenario: cancelling ctx while the server hangs makes Next return context.Canceled quickly.
func TestNextStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	arrived := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(arrived)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	callCtx, cancelCall := context.WithCancel(ctx)
	go func() {
		select {
		case <-arrived:
			cancelCall()
		case <-ctx.Done():
		}
	}()

	start := time.Now()
	_, err := newTestModel(srv).Next(callCtx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Next took %v after cancel, want under 1s", elapsed)
	}
}

// Scenario: a reply with an empty choices list is an error.
func TestNextFailsOnEmptyChoices(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := choicesServer(t, []any{}, requests)

	_, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("Next returned no error for empty choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("err = %v, want it to say no choices", err)
	}
}

// Scenario: a null finish_reason comes back as an empty FinishReason.
func TestNextReadsNullFinishReason(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := choicesServer(t, []any{map[string]any{
		"index":         0,
		"finish_reason": nil,
		"message":       map[string]any{"role": "assistant", "content": "partial"},
	}}, requests)

	got, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if got.FinishReason != "" || got.Content != "partial" {
		t.Errorf("reply = %+v, want content partial and empty finish reason", got)
	}
}

// Scenario: an empty tool list sends no tools field.
func TestNextOmitsToolsWhenThereAreNone(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	requests := make(chan capturedRequest, 1)
	srv := textReplyServer(t, "ok", requests)

	if _, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	if tools, ok := (<-requests).body["tools"]; ok {
		t.Errorf("tools = %v, want no tools field", tools)
	}
}

// Scenario: a reply sent as an event stream, which has no chat result, is an error.
func TestNextFailsWhenReplyIsAStream(t *testing.T) {
	ctx, cancel := testContext(t)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	_, err := newTestModel(srv).Next(ctx, []agents.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("Next returned no error for an event stream reply")
	}
}
