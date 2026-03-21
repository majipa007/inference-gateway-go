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
	mux.HandleFunc("/predict", handlers.Predict)

	handlers := middlewares.TimeoutMiddleware(25 * time.Second)(mux)
	handlers = middlewares.ConcurrentLimitWare(2)(handlers)
	handlers = middlewares.LoggerWare(handlers)

	return handlers
}
