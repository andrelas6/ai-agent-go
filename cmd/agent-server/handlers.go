package main

import (
	"encoding/json"
	"net/http"
)

func registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/tasks", handlePostTasks)
	mux.HandleFunc("POST /api/clear", handlePostClear)
}

func handlePostTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]string{"workflowId": "dummy-workflow-id"})
}

func handlePostClear(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
