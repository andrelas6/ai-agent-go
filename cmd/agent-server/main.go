package main

import (
	"andrelas6/ai-agent-go/internal/messages"
	"context"
	"encoding/json"
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
	mux := http.NewServeMux()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	messagesService := messages.NewService()

	mux.HandleFunc("GET /health", func(resWriter http.ResponseWriter, request *http.Request) {
		resWriter.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /sse", func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		rc := http.NewResponseController(w)

		for {
			select {
			case <-ctx.Done():
				if err := rc.Flush(); err != nil {
					fmt.Printf("issue flushing: %v", err)
				}
				return
			case <-request.Context().Done():
				// repeated code here and in two other places - move to another place
				if err := rc.Flush(); err != nil {
					fmt.Printf("issue flushing: %v", err)
				}
				return
			// LLM response in the future
			case message := <-messagesService.MessagesStream():
				fmt.Fprintf(w, "data: {\"token\"}:%q\n\n", message)
			// TODO: remove tick as it will be unnecessary
			case <-time.Tick(time.Second * 3):
				if err := rc.Flush(); err != nil {
					fmt.Printf("issue flushing: %v", err)
				}
			}
		}
	})

	mux.HandleFunc("POST /messages", func(writer http.ResponseWriter, request *http.Request) {
		var chatReq ChatRequest
		if err := json.NewDecoder(request.Body).Decode(&chatReq); err != nil {
			http.Error(writer, "could not read the body", http.StatusBadRequest)
		}

		go func() {
			messagesService.AddMessage(chatReq.Message)
		}()

		writer.Header().Set("Content-Type", "application/json")
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
