package health

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

type Handler struct {
	ping     func(context.Context) error
	timeout  time.Duration
	stopping atomic.Bool
}

func New(ping func(context.Context) error, timeout time.Duration) *Handler {
	return &Handler{ping: ping, timeout: timeout}
}

func (h *Handler) Stop() {
	h.stopping.Store(true)
}

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, "ok")
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.stopping.Load() {
		respond(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.ping(ctx); err != nil || ctx.Err() != nil || h.stopping.Load() {
		respond(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	respond(w, http.StatusOK, "ready")
}

func respond(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"status":"` + status + `"}` + "\n"))
}
