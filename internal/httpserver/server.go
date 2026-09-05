package httpserver

import (
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

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	_, ok := s.reg.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
