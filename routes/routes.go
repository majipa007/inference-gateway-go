// Package routes has all the middleware and mux
package routes

import (
	"log"
	"net/http"
	"os"
	"time"

	"inference-gateway-go/gateway"
	"inference-gateway-go/handlers"
	"inference-gateway-go/llamacpp"
	"inference-gateway-go/middlewares"
)

const (
	cacheTTL      = 10 * time.Minute
	cacheCapacity = 10000
)

func Register() http.Handler {
	mux := http.NewServeMux()

	// Ollama predict endpoint with caching
	cacheHandler := gateway.NewCachingHandler(cacheTTL, cacheCapacity, handlers.Predict)
	var predhandler http.Handler = cacheHandler
	predhandler = middlewares.ConcurrentLimitWare(2000)(predhandler)
	mux.Handle("/predict", predhandler)

	// llama.cpp predict endpoint with caching
	llamaBaseURL, llamaTimeout := llamacpp.LoadConfig()
	llamacpp.SetDefaultClient(llamacpp.NewClient(llamaBaseURL, llamaTimeout))

	cacheHandlerLlama := gateway.NewCachingHandler(cacheTTL, cacheCapacity, llamacpp.Predict)
	var llmapredhandler http.Handler = cacheHandlerLlama
	llmapredhandler = middlewares.ConcurrentLimitWare(2000)(llmapredhandler)
	mux.Handle("/llamacpp/predict", llmapredhandler)

	// Streaming endpoint bypasses cache (by nature streaming)
	var llamaStreamHandler http.Handler = http.HandlerFunc(llamacpp.PredictStream)
	llamaStreamHandler = middlewares.ConcurrentLimitWare(2000)(llamaStreamHandler)
	mux.Handle("/llamacpp/stream", llamaStreamHandler)

	// Health and metrics endpoints
	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/metrics", handlers.Metrics)
	mux.HandleFunc("/llamacpp/health", llamacpp.Health)

	// Apply middleware stack
	handlers := middlewares.TimeoutMiddleware(gatewayTimeout())(mux)
	handlers = middlewares.MetricsWare(handlers)
	handlers = middlewares.LoggerWare(handlers)

	return handlers
}

func gatewayTimeout() time.Duration {
	const defaultTimeout = 30 * time.Second

	raw := os.Getenv("GATEWAY_TIMEOUT")
	if raw == "" {
		return defaultTimeout
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		log.Printf("WARN: invalid GATEWAY_TIMEOUT=%q, using default %s", raw, defaultTimeout)
		return defaultTimeout
	}

	return timeout
}
