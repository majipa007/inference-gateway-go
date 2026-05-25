package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"inference-gateway-go/cache"
)

func TestCachingHandler_Integration_CacheAndMetrics(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	reqCount := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		time.Sleep(10 * time.Millisecond) // simulate work
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"integration-test"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	body := `{"model":"test","prompt":"integration-prompt"}`

	// First request - should be cache miss
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)

	if w.Header().Get("X-Cache") != "" {
		t.Fatal("expected no X-Cache header on first request")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if reqCount != 1 {
		t.Fatalf("expected next called once, got %d", reqCount)
	}

	// Second request - should be cache hit
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)

	if w.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected X-Cache=HIT, got %q", w.Header().Get("X-Cache"))
	}
	if reqCount != 1 {
		t.Fatalf("expected next still called only once, got %d", reqCount)
	}
}

func TestCachingHandler_Integration_ConcurrentCacheAccess(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(1000))

	reqCount := 0
	var mu sync.Mutex
	next := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		c := reqCount
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"concurrent","count":` + string(rune('a'+c%26)) + `}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	body := `{"model":"test","prompt":"concurrent-integration"}`
	var wg sync.WaitGroup

	// Launch many concurrent requests with the same body
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
			ch.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	// At least one request should have gone through to the backend
	if reqCount < 1 {
		t.Fatalf("expected at least 1 next call, got %d", reqCount)
	}

	// Verify no cache corruption - all responses should have been written
	// (just checking no panics or deadlocks occurred)
}

func TestCachingHandler_Integration_CacheEvictionWithMetrics(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(3))

	reqCount := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"eviction-test"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	// Fill cache with 3 entries
	for i := 0; i < 3; i++ {
		body := `{"model":"test","prompt":"evict-key-` + string(rune('a'+i)) + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
		ch.ServeHTTP(w, req)
	}

	if cacheHandler.Size() != 3 {
		t.Fatalf("expected cache size 3, got %d", cacheHandler.Size())
	}

	// Add 4th entry - should evict oldest
	body := `{"model":"test","prompt":"evict-key-d"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)

	if cacheHandler.Size() != 3 {
		t.Fatalf("expected cache size still 3, got %d", cacheHandler.Size())
	}
	if reqCount != 4 {
		t.Fatalf("expected 4 backend calls, got %d", reqCount)
	}

	// Verify oldest entry was evicted by requesting it again
	oldBody := `{"model":"test","prompt":"evict-key-a"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(oldBody))
	ch.ServeHTTP(w, req)

	if reqCount != 5 {
		t.Fatalf("expected 5 backend calls after requesting evicted key, got %d", reqCount)
	}
}

func TestCachingHandler_Integration_MultipleEngines(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	ollamaCount := 0
	llamaCount := 0

	ollamaNext := func(w http.ResponseWriter, r *http.Request) {
		ollamaCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"engine":"ollama"}`))
	}

	llamaNext := func(w http.ResponseWriter, r *http.Request) {
		llamaCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"engine":"llamacpp"}`))
	}

	ollamaHandler := &CachingHandler{
		cache: cacheHandler,
		next:  ollamaNext,
	}

	llamaHandler := &CachingHandler{
		cache: cacheHandler,
		next:  llamaNext,
	}

	// Ollama requests
	for i := 0; i < 3; i++ {
		body := `{"model":"llama3","prompt":"ollama-prompt-` + string(rune('a'+i)) + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
		ollamaHandler.ServeHTTP(w, req)
	}

	// llama.cpp requests
	for i := 0; i < 3; i++ {
		body := `{"model":"llama3","prompt":"llama-prompt-` + string(rune('a'+i)) + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/llamacpp/predict", bytes.NewBufferString(body))
		llamaHandler.ServeHTTP(w, req)
	}

	// Second round should hit cache
	ollamaBody := `{"model":"llama3","prompt":"ollama-prompt-a"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(ollamaBody))
	ollamaHandler.ServeHTTP(w, req)

	if w.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected ollama cache HIT, got %q", w.Header().Get("X-Cache"))
	}

	if ollamaCount != 3 {
		t.Fatalf("expected ollama backend called 3 times, got %d", ollamaCount)
	}
	if llamaCount != 3 {
		t.Fatalf("expected llama backend called 3 times, got %d", llamaCount)
	}
}
