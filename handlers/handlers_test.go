package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	// Ollama is unlikely to be running, so this will likely fail health check
	// but we verify the handler itself doesn't panic
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	// Response should be valid JSON
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := result["status"]; !ok {
		t.Error("expected 'status' field in response")
	}
	if _, ok := result["ollama"]; !ok {
		t.Error("expected 'ollama' field in response")
	}
}

func TestMetrics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	Metrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedFields := []string{"in_flight", "rejected", "timed_out", "total_requests"}
	for _, field := range expectedFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field '%s' in metrics response", field)
		}
	}
}

func TestPredictMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/predict", nil)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d for GET, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestPredictInvalidJSON(t *testing.T) {
	body := strings.NewReader("not valid json{")
	req := httptest.NewRequest(http.MethodPost, "/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid JSON, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPredictMissingFields(t *testing.T) {
	body := strings.NewReader(`{"model": "llama3.2"}`)
	req := httptest.NewRequest(http.MethodPost, "/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing prompt, got %d", http.StatusBadRequest, rec.Code)
	}

	body = strings.NewReader(`{"prompt": "hello"}`)
	req = httptest.NewRequest(http.MethodPost, "/predict", body)
	rec = httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing model, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPredictUnknownFields(t *testing.T) {
	body := strings.NewReader(`{"model": "llama3.2", "prompt": "hello", "unknown": "field"}`)
	req := httptest.NewRequest(http.MethodPost, "/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for unknown fields, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPredictEmptyBody(t *testing.T) {
	body := strings.NewReader("")
	req := httptest.NewRequest(http.MethodPost, "/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for empty body, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestOllamaBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "default",
			envValue: "",
			expected: "http://localhost:11434",
		},
		{
			name:     "custom url",
			envValue: "http://my-ollama:8080",
			expected: "http://my-ollama:8080",
		},
		{
			name:     "with trailing slash",
			envValue: "http://my-ollama:8080/",
			expected: "http://my-ollama:8080",
		},
		{
			name:     "with generate path",
			envValue: "http://my-ollama:8080/api/generate",
			expected: "http://my-ollama:8080",
		},
		{
			name:     "trimmed whitespace",
			envValue: "  http://my-ollama:8080  ",
			expected: "http://my-ollama:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OLLAMA_URL", tt.envValue)
			got := ollamaBaseURL()
			if got != tt.expected {
				t.Errorf("ollamaBaseURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
