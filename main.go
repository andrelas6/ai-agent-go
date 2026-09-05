package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

type Session struct {
	Ch     chan []byte
	Cancel context.CancelFunc
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) AddSession(id string) (*Session, context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// If there's an existing session with the same ID, cancel it
	if oldSess, ok := sm.sessions[id]; ok {
		oldSess.Cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess := &Session{
		Ch:     make(chan []byte, 10), // Buffer of 10 to avoid blocking immediately
		Cancel: cancel,
	}
	sm.sessions[id] = sess
	return sess, ctx
}

func (sm *SessionManager) RemoveSession(id string, sess *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// Only remove if this specific session is still the active one
	if current, ok := sm.sessions[id]; ok && current == sess {
		current.Cancel()
		delete(sm.sessions, id)
	}
}

type InitialEvent struct {
	SessionID string `json:"session_id"`
	PostRoute string `json:"post_route"`
}

func main() {
	sm := NewSessionManager()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			sessionID = uuid.New().String()
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Flush headers so client connects immediately
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// Register session
		sess, sessionCtx := sm.AddSession(sessionID)
		defer sm.RemoveSession(sessionID, sess)

		// Send initial event
		initEvent := InitialEvent{
			SessionID: sessionID,
			PostRoute: fmt.Sprintf("/messages/session/%s", sessionID),
		}
		initBytes, err := json.Marshal(initEvent)
		if err != nil {
			log.Printf("Error marshaling initial event: %v", err)
			return
		}

		fmt.Fprintf(w, "data: %s\n\n", initBytes)
		flusher.Flush()

		// Listen for messages or client disconnect
		reqCtx := r.Context()
		for {
			select {
			case <-reqCtx.Done():
				// Client disconnected
				return
			case <-sessionCtx.Done():
				// Session replaced by a new connection
				return
			case msg := <-sess.Ch:
				// We never close sess.Ch, so no need for `ok` check.
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("POST /messages/session/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		uuidStr := r.PathValue("uuid")
		if uuidStr == "" {
			http.Error(w, "Missing uuid", http.StatusBadRequest)
			return
		}

		// Read the JSON body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		// Optional: basic validation to ensure it's JSON
		if !json.Valid(body) {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		sm.mu.RLock()
		sess, ok := sm.sessions[uuidStr]
		sm.mu.RUnlock()

		if !ok {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		// Send message to the session's channel
		select {
		case sess.Ch <- body:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		default:
			http.Error(w, "Session buffer full", http.StatusServiceUnavailable)
		}
	})

	port := ":8080"
	log.Printf("Server listening on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
