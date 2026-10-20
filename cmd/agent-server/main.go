package main

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/messages"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type Dependencies struct {
	MessagesService *messages.Service
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var runner handlers.Runner

	runner = func() {
		fmt.Println("RUNNING")
	}

	mux := handlers.NewMux(ctx, bus.New(), runner)

	mux.HandleFunc("GET /health", func(resWriter http.ResponseWriter, request *http.Request) {
		resWriter.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:              ":8002",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("listening on :8002")
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("error starting the server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}
