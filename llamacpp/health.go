package llamacpp

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

var llamaHealthClient *Client

func init() {
	baseURL, _ := LoadConfig()
	healthClient := NewClient(baseURL, 2*time.Second)
	llamaHealthClient = healthClient
}

func Health(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: /llamacpp/health hit from %s", r.RemoteAddr)

	url := llamaHealthClient.baseURL + "/health"
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		log.Printf("ERROR: failed creating llama.cpp health request: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "unhealthy",
			"backend": "llama.cpp",
			"error":   "failed to create health probe request",
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: llama.cpp health probe failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "unhealthy",
			"backend": "llama.cpp",
			"error":   "cannot reach llama.cpp",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: llama.cpp health probe returned status=%d", resp.StatusCode)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "unhealthy",
			"backend": "llama.cpp",
			"error":   "llama.cpp returned status " + string(rune(resp.StatusCode)),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"backend": "llama.cpp",
	}); err != nil {
		log.Printf("ERROR: failed writing llama.cpp health response: %v", err)
	}
}
