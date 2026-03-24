package middlewares

import (
	"net/http"

	"inference-gateway-go/metrics"
)

func ConcurrentLimitWare(max int) func(next http.Handler) http.Handler {
	sem := make(chan struct{}, max)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				metrics.IncInFlight()
				defer func() {
					metrics.DecInFlight()
					<-sem
				}()
				next.ServeHTTP(w, r)
			default:
				metrics.IncRejected()
				http.Error(w, "Server Busy", http.StatusServiceUnavailable)
			}
		})
	}
}
