package httpserver

import (
	"encoding/json"
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
