package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"inference-gateway-go/cache"
	"inference-gateway-go/metrics"
)

// CachingHandler wraps an HTTP handler with LRU response caching.
// Cache key is generated from the (model, prompt) combination in the request body.
type CachingHandler struct {
	cache *cache.LRU
	mu    sync.RWMutex
	next  http.HandlerFunc
}

// NewCachingHandler creates a new caching middleware.
func NewCachingHandler(ttl time.Duration, capacity int, next http.HandlerFunc) *CachingHandler {
	return &CachingHandler{
		cache: cache.New(cache.WithTTL(ttl), cache.WithCapacity(capacity)),
		next:  next,
	}
}

// ServeHTTP intercepts requests, checks cache, and serves cached or fresh responses.
func (ch *CachingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	model, prompt, _, err := parseModelPrompt(r)
	if err != nil {
		ch.next(w, r)
		return
	}
	if model == "" || prompt == "" {
		ch.next(w, r)
		return
	}

	key := model + ":" + prompt
	ch.mu.RLock()
	cached := ch.cache.Get(key)
	ch.mu.RUnlock()

	if cached != nil {
		log.Printf("INFO: cache HIT for key=%s", key)
		metrics.IncCacheHits()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		w.Write(cached)
		return
	}

	// Cache miss
	metrics.IncCacheMisses()
	log.Printf("INFO: cache MISS for key=%s", key)

	// Capture the response for caching
	capture := &responseCapture{ResponseWriter: w, body: make([]byte, 0, 4096)}
	ch.next(capture, r)

	if capture.code == http.StatusOK && len(capture.body) > 0 {
		ch.mu.Lock()
		ch.cache.Set(key, capture.body)
		ch.mu.Unlock()
		log.Printf("INFO: cached %d bytes for key=%s", len(capture.body), key)
	}
}

// Size returns the current cache size.
func (ch *CachingHandler) Size() int {
	return ch.cache.Size()
}

// Capacity returns the cache capacity.
func (ch *CachingHandler) Capacity() int {
	return ch.cache.Capacity()
}

// TTL returns the cache TTL.
func (ch *CachingHandler) TTL() time.Duration {
	return ch.cache.TTL()
}

type responseCapture struct {
	http.ResponseWriter
	code int
	body []byte
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.code = code
	rc.ResponseWriter.WriteHeader(code)
}

func (rc *responseCapture) Write(p []byte) (int, error) {
	rc.body = append(rc.body, p...)
	return rc.ResponseWriter.Write(p)
}

// parseModelPrompt extracts model, prompt and body from the request.
func parseModelPrompt(r *http.Request) (model, prompt string, body []byte, err error) {
	if r.Method != http.MethodPost {
		return "", "", nil, nil
	}

	// Read body
	body = make([]byte, r.ContentLength+1024)
	n, _ := io.ReadFull(r.Body, body)
	body = body[:n]

	// Restore the body for downstream handlers
	r.Body = io.NopCloser(bytes.NewReader(body))

	type req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}

	var reqData req
	if err := json.Unmarshal(body, &reqData); err != nil {
		return "", "", nil, err
	}

	return reqData.Model, reqData.Prompt, body, nil
}
