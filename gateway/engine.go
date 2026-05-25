package gateway

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"inference-gateway-go/cache"
)

// CachingHandler wraps any HTTP handler with an LRU response cache.
// Cache key is the (model, prompt) combination from the JSON request body.
type CachingHandler struct {
	cache *cache.LRU
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
	model, prompt := parseModelPrompt(r)
	if model == "" || prompt == "" {
		ch.next(w, r)
		return
	}

	key := model + ":" + prompt

	// Check cache
	cached := ch.cache.Get(key)
	if cached != nil {
		log.Printf("INFO: cache HIT for key=%s", key)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		w.Write(cached)
		return
	}

	// Cache miss - capture and cache the response
	capture := &responseCapture{ResponseWriter: w, body: make([]byte, 0, 4096)}
	ch.next(capture, r)

	if capture.written && capture.code == 200 && len(capture.body) > 0 {
		ch.cache.Set(key, capture.body)
		log.Printf("INFO: cache MISS for key=%s, cached %d bytes", key, len(capture.body))
	}
}

type responseCapture struct {
	http.ResponseWriter
	written bool
	code    int
	body    []byte
}

func (rc *responseCapture) WriteHeader(code int) {
	if !rc.written {
		rc.code = code
	}
	rc.ResponseWriter.WriteHeader(code)
	rc.written = true
}

func (rc *responseCapture) Write(p []byte) (int, error) {
	if !rc.written {
		rc.code = 200
		rc.written = true
	}
	rc.body = append(rc.body, p...)
	return rc.ResponseWriter.Write(p)
}

// parseModelPrompt extracts model and prompt from the request body.
func parseModelPrompt(r *http.Request) (model, prompt string) {
	if r.Method != http.MethodPost {
		return "", ""
	}

	type req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}

	// Read body
	body := make([]byte, r.ContentLength+1024)
	n, _ := r.Body.Read(body)
	body = body[:n]

	var reqData req
	if err := json.Unmarshal(body, &reqData); err != nil {
		return "", ""
	}

	return reqData.Model, reqData.Prompt
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
