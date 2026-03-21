package middlewares

import "net/http"

func ConcurrentLimitWare(max int) func(next http.Handler) http.Handler {
	sem := make(chan struct{}, max)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() {
					<-sem
				}()
				next.ServeHTTP(w, r)
			default:
				http.Error(w, "Server Busy", http.StatusServiceUnavailable)
			}
		})
	}
}
