package middlewares

import (
	"net/http"

	"inference-gateway-go/metrics"
)

func MetricsWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			metrics.IncTotalRequests()
		}
		next.ServeHTTP(w, r)
	})
}
