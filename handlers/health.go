// Package handlers provides HTTP handlers used by the gateway.
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"inference-gateway-go/metrics"
)

const defaultOllamaBaseURL = "http://localhost:11434"

func ollamaBaseURL() string {
	base := strings.TrimSpace(os.Getenv("OLLAMA_URL"))
	if base == "" {
		return defaultOllamaBaseURL
	}

	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/api/generate") {
		base = strings.TrimSuffix(base, "/api/generate")
	}

	return base
}

// Health checks whether the gateway can reach Ollama.
func Health(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: /health hit from %s", r.RemoteAddr)

	url := fmt.Sprintf("%s/api/tags", ollamaBaseURL())
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		log.Printf("ERROR: failed creating health request: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"ollama": "down",
			"error":  "failed to create health probe request",
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: Ollama health probe failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"ollama": "down",
			"error":  "cannot reach ollama",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Ollama health probe returned status=%d", resp.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"ollama": "down",
			"error":  fmt.Sprintf("ollama returned status %d", resp.StatusCode),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"ollama": "up",
	}); err != nil {
		log.Printf("ERROR: failed writing health response: %v", err)
	}
}

// Metrics returns basic gateway counters.
func Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(metrics.Snapshot()); err != nil {
		log.Printf("ERROR: failed writing metrics response: %v", err)
	}
}
