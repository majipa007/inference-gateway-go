package middlewares

import (
	"context"
	"net/http"
	"time"
)

func TimeoutWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		r = r.WithContext(ctxWithTimeout)

		next.ServeHTTP(w, r)
	})
}
