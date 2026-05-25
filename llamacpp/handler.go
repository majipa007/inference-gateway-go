package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"inference-gateway-go/metrics"
)

func Predict(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		log.Printf("WARN: method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var req PredictRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		log.Printf("ERROR: failed to decode request body: %v", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Model == "" || req.Prompt == "" {
		log.Printf("WARN: missing required fields: model=%q", req.Model)
		http.Error(w, "model and prompt are required", http.StatusBadRequest)
		return
	}

	if req.Tokens <= 0 {
		req.Tokens = 256
	}
	if req.Temp <= 0 {
		req.Temp = 0.8
	}

	log.Printf("INFO: llamacpp request validated, model=%s tokens=%d", req.Model, req.Tokens)

	compReq := &CompletionRequest{
		Prompt:        req.Prompt,
		Model:         req.Model,
		NPredict:      req.Tokens,
		Temperature:   req.Temp,
		Stream:        false,
		MaxTokens:     req.Tokens,
		RepeatPenalty: 1.1,
		Stop:          []string{"\n\n--"},
	}

	ctx := r.Context()
	resp, err := defaultClient.Completion(ctx, compReq)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			metrics.IncTimedOut()
			log.Printf("ERROR: llama.cpp request timed out: %v", err)
			http.Error(w, "llama.cpp request timed out", http.StatusGatewayTimeout)

		case errors.Is(err, context.Canceled):
			log.Printf("ERROR: llama.cpp request canceled: %v", err)
			http.Error(w, "llama.cpp request canceled", http.StatusRequestTimeout)

		default:
			log.Printf("ERROR: llama.cpp request failed: %v", err)
			http.Error(w, "error communicating with llama.cpp", http.StatusBadGateway)
		}
		return
	}

	log.Printf("INFO: llama.cpp response received in %s, tokens_read=%d", time.Since(start), resp.TokensRead)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"response":         resp.CompletionText,
		"tokens_read":      resp.TokensRead,
		"tokens_evaluated": resp.TokensEvaluated,
		"stop":             resp.Stop,
	}); err != nil {
		log.Printf("ERROR: failed writing response: %v", err)
	}
}

func PredictStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("WARN: method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var req PredictRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		log.Printf("ERROR: failed to decode request body: %v", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Model == "" || req.Prompt == "" {
		log.Printf("WARN: missing required fields: model=%q", req.Model)
		http.Error(w, "model and prompt are required", http.StatusBadRequest)
		return
	}

	if req.Tokens <= 0 {
		req.Tokens = 256
	}
	if req.Temp <= 0 {
		req.Temp = 0.8
	}

	log.Printf("INFO: llamacpp stream request, model=%s tokens=%d", req.Model, req.Tokens)

	compReq := &CompletionRequest{
		Prompt:        req.Prompt,
		Model:         req.Model,
		NPredict:      req.Tokens,
		Temperature:   req.Temp,
		Stream:        true,
		MaxTokens:     req.Tokens,
		RepeatPenalty: 1.1,
		Stop:          []string{"\n\n--"},
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var fullResponse strings.Builder
	ctx := r.Context()
	errCh := make(chan error, 1)

	go func() {
		err := defaultClient.CompletionStream(ctx, compReq, func(text string) {
			fullResponse.WriteString(text)
			event := map[string]string{
				"content": text,
			}
			data, _ := json.Marshal(event)
			_, writeErr := fmt.Fprintf(w, "data:%s\n\n", data)
			if writeErr != nil {
				errCh <- fmt.Errorf("stream write error: %w", writeErr)
				return
			}
			flusher.Flush()
		})
		if err != nil {
			errCh <- err
			return
		}
		_, _ = fmt.Fprint(w, "data:[DONE]\n\n")
		flusher.Flush()
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Printf("WARN: stream canceled by client: %v", ctx.Err())
		http.Error(w, "stream canceled", http.StatusRequestTimeout)
		return
	case err := <-errCh:
		if err != nil {
			log.Printf("ERROR: stream error: %v", err)
			http.Error(w, "stream error", http.StatusBadGateway)
			return
		}
		log.Printf("INFO: llama.cpp stream completed for model=%s", req.Model)
	}
}
