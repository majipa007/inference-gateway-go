package gateway

import (
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"inference-gateway-go/batcher"
	"inference-gateway-go/metrics"
)

// BatchHandler wraps an HTTP handler with request batching support.
// Requests are collected and processed in groups for throughput optimization.
type BatchHandler struct {
	batcher *batcher.Batcher
	next    http.HandlerFunc
}

// NewBatchHandler creates a new batched HTTP handler.
func NewBatchHandler(handler func([]*batcher.Request) []batcher.Response, next http.HandlerFunc, batchOpts ...batcher.Option) *BatchHandler {
	b := batcher.New(handler, batchOpts...)
	return &BatchHandler{
		batcher: b,
		next:    next,
	}
}

// Start launches the batcher's processing goroutine.
func (bh *BatchHandler) Start() {
	bh.batcher.Start()
}

// Stop gracefully shuts down the batch handler.
func (bh *BatchHandler) Stop() {
	bh.batcher.Stop()
}

// Stats returns current batcher statistics.
func (bh *BatchHandler) Stats() batcher.Stats {
	return bh.batcher.Stats()
}

// MetricsHandler wraps a handler and records latency and batch metrics.
func MetricsHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		metrics.RecordLatency(float64(time.Since(start).Microseconds()) / 1000.0)
	}
}

// BatchMetricsHandler wraps a handler with both batching and metrics.
func BatchMetricsHandler(next http.HandlerFunc) http.HandlerFunc {
	return MetricsHandler(next)
}

// WrapBatch adds batch metrics tracking to a handler.
func WrapBatch(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&totalBatchedRequests, 1)
		next(w, r)
	}
}

// IncBatchedRequests increments the batched request counter.
func IncBatchedRequests(n uint64) {
	atomic.AddUint64(&totalBatchedRequests, n)
}

// GetTotalBatchedRequests returns the total number of batched requests.
func GetTotalBatchedRequests() uint64 {
	return atomic.LoadUint64(&totalBatchedRequests)
}

var totalBatchedRequests uint64

func handleBatchedRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("WARN: batch handler not initialized, using passthrough mode")
	http.Error(w, "batch handler not configured", http.StatusServiceUnavailable)
}
