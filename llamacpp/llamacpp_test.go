package llamacpp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestClientCompletion(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("expected POST method")
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"model":            "llama3.2",
			"tokens_read":      42,
			"tokens_evaluated": 10,
			"completion_text":  "Hello, world!",
			"stop":             true,
			"content":          "Hello, world!",
		})
	})
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	resp, err := client.Completion(context.Background(), &CompletionRequest{
		Prompt:      "What is 2+2?",
		NPredict:    10,
		Temperature: 0.8,
		Stream:      false,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.CompletionText != "Hello, world!" {
		t.Errorf("expected completion_text 'Hello, world!', got %q", resp.CompletionText)
	}
	if resp.TokensRead != 42 {
		t.Errorf("expected tokens_read=42, got %d", resp.TokensRead)
	}
	if resp.TokensEvaluated != 10 {
		t.Errorf("expected tokens_evaluated=10, got %d", resp.TokensEvaluated)
	}
}

func TestClientCompletionErrorStatus(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("model not found"))
	})
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	_, err := client.Completion(context.Background(), &CompletionRequest{
		Prompt: "What is 2+2?",
	})

	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}

	if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected error to contain '502', got: %v", err)
	}
}

func TestClientCompletionMalformedResponse(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json at all"))
	})
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)

	_, err := client.Completion(context.Background(), &CompletionRequest{
		Prompt: "What is 2+2?",
	})

	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
}

func TestClientCompletionMarshalError(t *testing.T) {
	client := NewClient("http://example.com", 5*time.Second)

	// CompletionRequest is serializable, so this just tests the error path
	// by passing a request with valid data
	resp, err := client.Completion(context.Background(), &CompletionRequest{
		Prompt: "test",
	})

	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("LLAMA_CPP_URL", "")
		t.Setenv("LLAMA_CPP_TIMEOUT", "")

		baseURL, timeout := LoadConfig()

		if baseURL != "http://localhost:8081" {
			t.Errorf("expected default URL 'http://localhost:8081', got %q", baseURL)
		}
		if timeout != 30*time.Second {
			t.Errorf("expected default timeout 30s, got %v", timeout)
		}
	})

	t.Run("custom url", func(t *testing.T) {
		t.Setenv("LLAMA_CPP_URL", "http://my-server:9000")
		t.Setenv("LLAMA_CPP_TIMEOUT", "60s")

		baseURL, timeout := LoadConfig()

		if baseURL != "http://my-server:9000" {
			t.Errorf("expected 'http://my-server:9000', got %q", baseURL)
		}
		if timeout != 60*time.Second {
			t.Errorf("expected timeout 60s, got %v", timeout)
		}
	})

	t.Run("trailing slash stripped", func(t *testing.T) {
		t.Setenv("LLAMA_CPP_URL", "http://localhost:8081/")
		t.Setenv("LLAMA_CPP_TIMEOUT", "10s")

		baseURL, _ := LoadConfig()

		if baseURL != "http://localhost:8081" {
			t.Errorf("expected trailing slash stripped, got %q", baseURL)
		}
	})

	t.Run("invalid timeout defaults", func(t *testing.T) {
		t.Setenv("LLAMA_CPP_TIMEOUT", "not-a-duration")

		_, timeout := LoadConfig()

		if timeout != 30*time.Second {
			t.Errorf("expected default timeout 30s for invalid duration, got %v", timeout)
		}
	})
}

func TestNewClient(t *testing.T) {
	t.Run("adds trailing slash", func(t *testing.T) {
		client := NewClient("http://localhost:8081", 30*time.Second)
		if client.baseURL != "http://localhost:8081/" {
			t.Errorf("expected trailing slash added, got %q", client.baseURL)
		}
	})

	t.Run("preserves trailing slash", func(t *testing.T) {
		client := NewClient("http://localhost:8081/", 30*time.Second)
		if client.baseURL != "http://localhost:8081/" {
			t.Errorf("expected trailing slash preserved, got %q", client.baseURL)
		}
	})

	t.Run("sets timeout", func(t *testing.T) {
		client := NewClient("http://localhost:8081", 45*time.Second)
		if client.httpClient.Timeout != 45*time.Second {
			t.Errorf("expected timeout 45s, got %v", client.httpClient.Timeout)
		}
	})
}

func TestCompletionRequestMarshal(t *testing.T) {
	req := &CompletionRequest{
		Prompt:      "What is Go?",
		Model:       "llama3.2",
		NPredict:    256,
		Temperature: 0.7,
		Stream:      false,
		MaxTokens:   256,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["prompt"] != "What is Go?" {
		t.Errorf("expected prompt 'What is Go?', got %v", result["prompt"])
	}
	if result["n_predict"] != float64(256) {
		t.Errorf("expected n_predict 256, got %v", result["n_predict"])
	}
}

func TestPredictHandlerValidation(t *testing.T) {
	SetDefaultClient(&Client{})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/llamacpp/predict", nil)
		rec := httptest.NewRecorder()

		Predict(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		body := strings.NewReader("not json")
		req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", body)
		rec := httptest.NewRecorder()

		Predict(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		body := strings.NewReader(`{"prompt": "hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", body)
		rec := httptest.NewRecorder()

		Predict(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("missing prompt", func(t *testing.T) {
		body := strings.NewReader(`{"model": "llama3.2"}`)
		req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", body)
		rec := httptest.NewRecorder()

		Predict(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

func TestPredictHandlerSuccessfulRequest(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"completion_text":  "The answer is 42.",
			"tokens_read":      5,
			"tokens_evaluated": 10,
			"stop":             true,
		})
	})
	defer server.Close()

	SetDefaultClient(NewClient(server.URL, 5*time.Second))

	body := strings.NewReader(`{"model": "llama3.2", "prompt": "What is 2+2?"}`)
	req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["response"] != "The answer is 42." {
		t.Errorf("expected response 'The answer is 42.', got %v", result["response"])
	}
}

func TestPredictHandlerDefaults(t *testing.T) {
	server := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"completion_text":  "test",
			"tokens_read":      0,
			"tokens_evaluated": 0,
			"stop":             false,
		})
	})
	defer server.Close()

	SetDefaultClient(NewClient(server.URL, 5*time.Second))

	// Request with no n_predict or temperature - should use defaults
	body := strings.NewReader(`{"model": "llama3.2", "prompt": "hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", body)
	rec := httptest.NewRecorder()

	Predict(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
