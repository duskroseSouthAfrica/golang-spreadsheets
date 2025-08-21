// api.go
package main

import (
	"encoding/json"
	"net/http"
	"time"
)

func validateFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Error: "Method not allowed"})
		return
	}
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: map[string]string{"status": "File valid"}})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}