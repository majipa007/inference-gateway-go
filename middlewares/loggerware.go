// Package middlewares contains all the middlewares
package middlewares

import (
	"log"
	"net/http"
)

func LoggerWare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("INFO: %s hit from %s", r.URL, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("INFO: %s hit from %s", r.URL, r.RemoteAddr)
	})
}
