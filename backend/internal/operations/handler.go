// Package operations exposes aggregate, administrator-only process diagnostics.
package operations

import (
	"context"
	"net/http"
	"time"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/notifications"
	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	access  *auth.Service
	account *auth.Handler
	http    *middleware.HTTPMetrics
	pool    *pgxpool.Pool
	notices *notifications.Service
	enabled bool
	started time.Time
}

func NewHandler(a *auth.Service, h *auth.Handler, m *middleware.HTTPMetrics, p *pgxpool.Pool, n *notifications.Service, enabled bool) *Handler {
	return &Handler{a, h, m, p, n, enabled, time.Now()}
}
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.With(httpapi.Protect, h.account.RequireAccount).Get("/operations/metrics", h.get)
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		httpapi.WriteProblem(w, r, httpapi.ValidationProblem(httpapi.FieldError{Field: "query", Code: "unknown", Message: "Параметры не поддерживаются."}))
		return
	}
	p := auth.PrincipalFrom(r)
	err := h.access.WithAccess(r.Context(), p, []string{"admin"}, func(context.Context, pgx.Tx, auth.Principal) error { return nil })
	if err != nil {
		auth.WriteError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	n, err := h.notices.Metrics(ctx)
	if err != nil {
		httpapi.WriteProblem(w, r, httpapi.NewProblem(503, "operations_unavailable", "Метрики временно недоступны."))
		return
	}
	stat := h.pool.Stat()
	out := map[string]any{
		"uptime_seconds": time.Since(h.started).Seconds(),
		"http":           h.http.Snapshot(),
		"database": map[string]any{
			"total_connections":        stat.TotalConns(),
			"acquired_connections":     stat.AcquiredConns(),
			"idle_connections":         stat.IdleConns(),
			"max_connections":          stat.MaxConns(),
			"acquire_count":            stat.AcquireCount(),
			"acquire_duration_seconds": stat.AcquireDuration().Seconds(),
			"canceled_acquire_count":   stat.CanceledAcquireCount(),
		},
		"notifications_enabled": h.enabled,
		"notifications":         n,
	}
	// Recheck access after I/O so a concurrent revocation cannot disclose metrics.
	err = h.access.WithAccess(r.Context(), p, []string{"admin"}, func(context.Context, pgx.Tx, auth.Principal) error { return nil })
	if err != nil {
		auth.WriteError(w, r, err)
		return
	}
	httpapi.WriteJSON(w, 200, out)
}
