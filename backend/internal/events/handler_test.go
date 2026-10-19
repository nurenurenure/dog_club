package events

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/auth"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRoutesRequireAccountAndOrigin(t *testing.T) {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, v := range []struct {
		method, path, origin string
		status               int
	}{{"GET", "/api/v1/events", "", 401}, {"GET", "/api/v1/event-venues", "", 401}, {"GET", "/api/v1/event-experts", "", 401}, {"POST", "/api/v1/events/1/registrations/1/attend", "https://foreign.example", 403}, {"PUT", "/api/v1/events/1/results/1", "https://foreign.example", 403}} {
		r := httptest.NewRequest(v.method, v.path, nil)
		r.Header.Set("Origin", v.origin)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != v.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("route lacks auth/origin/cache/request ID protection")
		}
	}
}
func TestStrictEventJSON(t *testing.T) {
	called := false
	h := command(new(Handler), "", true, func(context.Context, auth.Principal, int32, int64, EventInput) (Event, error) {
		called = true
		return Event{}, nil
	})
	for _, v := range []struct {
		body   string
		status int
	}{{`{"starts_at":"2025-01-01T10:00:00"}`, 400}, {`{"status":"completed"}`, 400}, {`{"name":"` + strings.Repeat("x", 16384) + `"}`, 413}} {
		called = false
		r := httptest.NewRequest("POST", "/", strings.NewReader(v.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != v.status || called {
			t.Fatalf("status %d want %d", w.Code, v.status)
		}
	}
}
