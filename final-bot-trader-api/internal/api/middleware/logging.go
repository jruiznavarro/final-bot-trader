package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Logger is a middleware that logs request details
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			log.Printf(
				"[%s] %s %s - %d (%s)",
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
				ww.Status(),
				time.Since(start),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
