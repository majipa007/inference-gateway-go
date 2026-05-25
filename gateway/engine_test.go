package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"inference-gateway-go/cache"
)

func TestCachingHandler_Hit(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	count := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"hello"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	body := `{"model":"test","prompt":"what is 2+2?"}`
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	// First request - cache miss
	ch.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
	if w.Header().Get("X-Cache") != "" {
		t.Fatalf("expected empty X-Cache on first request, got %q", w.Header().Get("X-Cache"))
	}
	if count != 1 {
		t.Fatalf("expected next to be called 1 time, got %d", count)
	}

	// Second request - cache hit
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)

	if w.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected X-Cache=HIT, got %q", w.Header().Get("X-Cache"))
	}
	if count != 1 {
		t.Fatalf("expected next to still be called 1 time after cache hit, got %d", count)
	}
}

func TestCachingHandler_Miss(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	count := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"different"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	// Different requests should miss
	for i := 0; i < 3; i++ {
		prompt := `{"model":"test","prompt":"query-` + string(rune('a'+i)) + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(prompt))
		ch.ServeHTTP(w, req)

		if count != i+1 {
			t.Fatalf("expected next called %d times, got %d", i+1, count)
		}
	}
}

func TestCachingHandler_TTLExpiry(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(50*time.Millisecond), cache.WithCapacity(100))

	count := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"hello"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	body := `{"model":"test","prompt":"expire-me"}`

	// First request
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	// Second request (cache hit)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)
	if w.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected HIT, got %q", w.Header().Get("X-Cache"))
	}

	// Wait for TTL expiry
	time.Sleep(100 * time.Millisecond)

	// Third request (should be cache miss again)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
	ch.ServeHTTP(w, req)
	if count != 2 {
		t.Fatalf("expected count=2 after TTL expiry, got %d", count)
	}
}

func TestCachingHandler_CapacityExceed(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(2))

	count := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"hello"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	for i := 0; i < 4; i++ {
		body := `{"model":"test","prompt":"key-` + string(rune('a'+i)) + `"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
		ch.ServeHTTP(w, req)
	}

	// Only 2 entries should be cached
	if ch.cache.Size() != 2 {
		t.Fatalf("expected cache size 2, got %d", ch.cache.Size())
	}
	if count != 4 {
		t.Fatalf("expected next called 4 times, got %d", count)
	}
}

func TestCachingHandler_ConcurrentAccess(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(1000))

	count := 0
	var mu sync.Mutex
	next := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		c := count
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"hello","count":` + string(rune(c%26+'a')) + `}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	body := `{"model":"test","prompt":"concurrent-test"}`
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(body))
			ch.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	if count < 1 {
		t.Fatalf("expected next called at least once, got %d", count)
	}
}

func TestCachingHandler_MalformedBody(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"fallback":true}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	// Malformed JSON should pass through to next handler
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(`not json`))
	ch.ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("expected next to be called for malformed JSON")
	}
}

func TestCachingHandler_PreservesRequestBody(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	var receivedBody []byte
	next := func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"ok"}`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	expectedBody := `{"model":"test","prompt":"preserve-body-test"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(expectedBody))
	ch.ServeHTTP(w, req)

	if string(receivedBody) != expectedBody {
		t.Fatalf("expected body %q, got %q", expectedBody, string(receivedBody))
	}
}

func TestCachingHandler_MethodFiltering(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	// GET should not trigger cache check (falls through to next)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/predict", nil)
	ch.ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("expected next to be called for GET request")
	}
}

func TestCachingHandler_EmptyModelPrompt(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(10*time.Minute), cache.WithCapacity(100))

	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	// Empty model/prompt should fall through to next
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/predict", bytes.NewBufferString(`{"model":"","prompt":""}`))
	ch.ServeHTTP(w, req)

	if !nextCalled {
		t.Fatal("expected next to be called for empty model/prompt")
	}
}

func TestCachingHandler_CacheSizeCapacityTTL(t *testing.T) {
	cacheHandler := cache.New(cache.WithTTL(5 * time.Minute), cache.WithCapacity(500))
	next := func(w http.ResponseWriter, r *http.Request) {}

	ch := &CachingHandler{
		cache: cacheHandler,
		next:  next,
	}

	if ch.Size() != 0 {
		t.Fatalf("expected size 0, got %d", ch.Size())
	}
	if ch.Capacity() != 500 {
		t.Fatalf("expected capacity 500, got %d", ch.Capacity())
	}
	if ch.TTL() != 5*time.Minute {
		t.Fatalf("expected TTL 5m, got %v", ch.TTL())
	}
}
