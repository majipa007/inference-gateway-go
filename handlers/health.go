// Package handlers provides helpers for parsing config files.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// Health returns a simple Health check response from handlers.
func Health(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: /health hit from %s", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	}); err != nil {
		log.Printf("ERROR: failed writing health response: %v", err)
	}
}
