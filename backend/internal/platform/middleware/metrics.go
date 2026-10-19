package middleware

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type HTTPMetric struct {
	Method             string  `json:"method"`
	Route              string  `json:"route"`
	Requests           uint64  `json:"requests"`
	ClientErrors       uint64  `json:"client_errors"`
	ServerErrors       uint64  `json:"server_errors"`
	DurationSeconds    float64 `json:"duration_seconds"`
	MaxDurationSeconds float64 `json:"max_duration_seconds"`
}
type HTTPMetrics struct {
	mu     sync.Mutex
	routes map[string]HTTPMetric
}

func (m *HTTPMetrics) Observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			method := r.Method
			switch method {
			case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
			default:
				method = "OTHER"
			}
			key := method + " " + route
			elapsed := time.Since(start).Seconds()
			status := wrapped.Status()
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.routes == nil {
				m.routes = map[string]HTTPMetric{}
			}
			v := m.routes[key]
			v.Method = method
			v.Route = route
			v.Requests++
			v.DurationSeconds += elapsed
			v.MaxDurationSeconds = max(v.MaxDurationSeconds, elapsed)
			if status >= 400 && status < 500 {
				v.ClientErrors++
			}
			if status >= 500 {
				v.ServerErrors++
			}
			m.routes[key] = v
		}()
		next.ServeHTTP(wrapped, r)
	})
}
func (m *HTTPMetrics) Snapshot() []HTTPMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]HTTPMetric, 0, len(m.routes))
	for _, v := range m.routes {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b HTTPMetric) int { return strings.Compare(a.Method+" "+a.Route, b.Method+" "+b.Route) })
	return out
}
