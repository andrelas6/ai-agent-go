package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andrelas6/ai-agent-go/internal/session"
)

func newTestServer(t *testing.T) (*httptest.Server, *session.Registry) {
	t.Helper()
	reg := session.NewRegistry()
	ts := httptest.NewServer(New(reg).Mux())
	t.Cleanup(ts.Close)
	return ts, reg
}

func TestPostMessage_UnknownSession(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Post(ts.URL+"/messages/session/does-not-exist", "application/json", strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestPostMessage_MalformedJSON(t *testing.T) {
	ts, reg := newTestServer(t)
	id, _ := reg.Create()
	resp, err := http.Post(ts.URL+"/messages/session/"+id, "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestPostMessage_KnownSession_Echoes(t *testing.T) {
	ts, reg := newTestServer(t)
	id, ch := reg.Create()

	resp, err := http.Post(ts.URL+"/messages/session/"+id, "application/json", strings.NewReader(`{"message":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}

	select {
	case got := <-ch:
		if got != "hello!!!" {
			t.Fatalf("got %q, want %q", got, "hello!!!")
		}
	default:
		t.Fatal("expected message on session channel")
	}
}
