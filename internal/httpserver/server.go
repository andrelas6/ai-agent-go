package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/andrelas6/ai-agent-go/internal/session"
)

type Server struct {
	reg *session.Registry
}

func New(reg *session.Registry) *Server {
	return &Server{reg: reg}
}

func (s *Server) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sse", s.handleSSE)
	mux.HandleFunc("POST /messages/session/{id}", s.handlePostMessage)
	return mux
}

type postMessageRequest struct {
	Message string `json:"message"`
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	ch, ok := s.reg.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	ch <- req.Message + "!!!"
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	id, ch := s.reg.Create()
	defer s.reg.Delete(id)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "data: /messages/session/%s\n\n", id)
	if err := rc.Flush(); err != nil {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
