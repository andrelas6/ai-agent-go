package main

import (
	"log"
	"net/http"
	"os"

	"github.com/andrelas6/ai-agent-go/internal/httpserver"
	"github.com/andrelas6/ai-agent-go/internal/session"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	reg := session.NewRegistry()
	srv := httpserver.New(reg)

	addr := ":" + port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Mux()); err != nil {
		log.Fatal(err)
	}
}
