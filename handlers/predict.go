// Package handlers provides helpers for parsing config files.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"
)

// PredictRequest is the payload your API expects from the client.
type PredictRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// OllamaRequest is the payload sent to the Ollama server.
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse is the response returned by Ollama.
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
}

func Predict(w http.ResponseWriter, r *http.Request) {
	// handler accepts a predict request, forwards it to Ollama,
	// and returns the Ollama response back to the client.

	start := time.Now()

	// Only allow POST for this endpoint.
	if r.Method != http.MethodPost {
		log.Printf("WARN: method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var req PredictRequest
	var ollamaReq OllamaRequest
	var ollamaResponse OllamaResponse

	// Decode incoming JSON request body into PredictRequest.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	baseCtx := context.Background()
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	if err := dec.Decode(&req); err != nil {
		log.Printf("ERROR: failed to decode request body: %v", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Basic validation.
	if req.Model == "" || req.Prompt == "" {
		log.Printf("WARN: missing required fields: model=%q prompt_present=%t", req.Model, req.Prompt != "")
		http.Error(w, "model and prompt are required", http.StatusBadRequest)
		return
	}

	log.Printf("INFO: request validated successfully, model=%s", req.Model)

	// Build request payload for Ollama.
	ollamaReq.Model = req.Model
	ollamaReq.Prompt = req.Prompt
	ollamaReq.Stream = false

	// Convert Go struct into JSON bytes for outbound HTTP request.
	reqBytes, err := json.Marshal(ollamaReq)
	if err != nil {
		log.Printf("ERROR: failed to marshal Ollama request: %v", err)
		http.Error(w, "error converting JSON to bytes", http.StatusInternalServerError)
		return
	}

	// Read Ollama URL from environment.
	// url := os.Getenv("OLLAMA_URL")
	url := "http://localhost:11434/api/generate"
	if url == "" {
		log.Printf("ERROR: OLLAMA_URL environment variable is not set")
		http.Error(w, "server configuration error", http.StatusInternalServerError)
		return
	}

	log.Printf("INFO: sending request to Ollama at %s", url)

	// Send request to Ollama server.
	// resp, err := http.Post(url, "application/json", bytes.NewReader(reqBytes))
	reqOllama, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		log.Printf("ERROR: failed communicating with Ollama server: %v", err)
		http.Error(w, "Error Building request for OLLAMA", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(reqOllama)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			log.Printf("ERROR: Ollama request timed out after 5s: %v", err)
			http.Error(w, "Ollama request timed out", http.StatusGatewayTimeout)

		case errors.Is(err, context.Canceled):
			log.Printf("ERROR: Ollama request canceled: %v", err)
			http.Error(w, "Ollama request canceled", http.StatusRequestTimeout)

		default:
			log.Printf("ERROR: failed communicating with Ollama server: %v", err)
			http.Error(w, "Error communicating with OLLAMA", http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	// If Ollama returns a non-200 response, read the body for debugging.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("ERROR: Ollama returned status=%d body=%s", resp.StatusCode, string(body))
		http.Error(w, "ollama server returned an error", http.StatusBadGateway)
		return
	}

	// Decode Ollama JSON response into struct.
	ollamaResponseDecoder := json.NewDecoder(resp.Body)
	if err := ollamaResponseDecoder.Decode(&ollamaResponse); err != nil {
		log.Printf("ERROR: failed to decode Ollama response: %v", err)
		http.Error(w, "error decoding Ollama response", http.StatusInternalServerError)
		return
	}

	log.Printf("INFO: Ollama response received successfully for model=%s in %s", ollamaResponse.Model, time.Since(start))

	// Return the Ollama response back to the client.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(ollamaResponse.Response); err != nil {
		log.Printf("ERROR: failed writing response to client: %v", err)
		return
	}
}
