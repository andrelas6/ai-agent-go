package main

import (
	"andrelas6/ai-agent-go/cmd/agent-server/handlers"
	"andrelas6/ai-agent-go/internal/bus"
	"andrelas6/ai-agent-go/internal/model"
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	stub := flag.Bool("stub", false, "run the stub instead of the agent")
	flag.Parse()

	modelID := os.Getenv("OPENROUTER_MODEL")
	if modelID == "" {
		modelID = model.DefaultModel
	}
	runner, startLine, err := newRunner(*stub, os.Getenv("OPENROUTER_API_KEY"), modelID)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(startLine)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// agent := &agent.Agent{}
	//

	mux := handlers.NewMux(ctx, bus.New(ctx), runner)

	server := &http.Server{
		Addr:              ":8002",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("listening on :8002")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("error starting the server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("error shutting down the server: %v", err)
	}
}
