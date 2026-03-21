// Package routes has all the middleware and mux
package routes

import (
	"net/http"

	"inference-gateway-go/handlers"
	"inference-gateway-go/middlewares"
)

func Register() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.Health)
	mux.HandleFunc("/predict", handlers.Predict)

	return middlewares.LoggerWare(middlewares.TimeoutWare(mux))
}
