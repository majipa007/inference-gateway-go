// Package routes has all the middleware and mux
package routes

import (
	"net/http"
	"time"

	"inference-gateway-go/handlers"
	"inference-gateway-go/middlewares"
)

func Register() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/metrics", handlers.Metrics)
	var predhandler http.Handler = http.HandlerFunc(handlers.Predict)
	predhandler = middlewares.ConcurrentLimitWare(2)(predhandler)
	mux.Handle("/predict", predhandler)

	handlers := middlewares.TimeoutMiddleware(25 * time.Second)(mux)
	handlers = middlewares.MetricsWare(handlers)
	handlers = middlewares.LoggerWare(handlers)

	return handlers
}
