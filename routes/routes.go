// Package routes has all the middleware and mux
package routes

import (
	"log"
	"net/http"
	"os"
	"time"

	"inference-gateway-go/handlers"
	"inference-gateway-go/llamacpp"
	"inference-gateway-go/middlewares"
)

func Register() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/metrics", handlers.Metrics)
	var predhandler http.Handler = http.HandlerFunc(handlers.Predict)
	predhandler = middlewares.ConcurrentLimitWare(2000)(predhandler)
	mux.Handle("/predict", predhandler)

	llamaBaseURL, llamaTimeout := llamacpp.LoadConfig()
	llamacpp.SetDefaultClient(llamacpp.NewClient(llamaBaseURL, llamaTimeout))

	var llmapredhandler http.Handler = http.HandlerFunc(llamacpp.Predict)
	llmapredhandler = middlewares.ConcurrentLimitWare(2000)(llmapredhandler)
	mux.Handle("/llamacpp/predict", llmapredhandler)

	var llamaStreamHandler http.Handler = http.HandlerFunc(llamacpp.PredictStream)
	llamaStreamHandler = middlewares.ConcurrentLimitWare(2000)(llamaStreamHandler)
	mux.Handle("/llamacpp/stream", llamaStreamHandler)

	mux.HandleFunc("/llamacpp/health", llamacpp.Health)

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
