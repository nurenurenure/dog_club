package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			id := chimiddleware.GetReqID(r.Context())
			wrapped.Header().Set("X-Request-ID", id)
			defer func() {
				pattern := chi.RouteContext(r.Context()).RoutePattern()
				if pattern == "" {
					pattern = "unmatched"
				}
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
				}
				// Route patterns exclude query strings and raw path parameters.
				logger.InfoContext(r.Context(), "http request",
					"request_id", id, "method", r.Method, "route", pattern,
					"status", status, "bytes", wrapped.BytesWritten(),
					"duration_ms", time.Since(started).Milliseconds())
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					// Panic values and stacks can contain secrets and personal data.
					logger.ErrorContext(r.Context(), "http request panic",
						"request_id", chimiddleware.GetReqID(r.Context()))
					if wrapped, ok := w.(chimiddleware.WrapResponseWriter); ok && wrapped.Status() != 0 {
						panic(http.ErrAbortHandler)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("{\"error\":\"internal_error\"}\n"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
