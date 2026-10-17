package breeding

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
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, v := range []struct {
		method, path, origin string
		status               int
	}{{"GET", "/api/v1/breeding/dogs", "", 401}, {"GET", "/api/v1/litters/1", "", 401}, {"POST", "/api/v1/breeding-requests/1/decisions", "https://foreign.example", 403}, {"POST", "/api/v1/litters/1/puppies/2/unlink", "https://foreign.example", 403}} {
		req := httptest.NewRequest(v.method, v.path, nil)
		req.Header.Set("Origin", v.origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != v.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("breeding route missing protection")
		}
	}
}
func TestStrictPuppyJSON(t *testing.T) {
	called := false
	h := command(new(Handler), false, func(context.Context, auth.Principal, int64, PuppyInput) (Litter, error) {
		called = true
		return Litter{}, nil
	})
	for _, v := range []struct {
		body   string
		status int
	}{{`{"identifier":{"number":"001","unexpected":1}}`, 400}, {`{"date_of_birth":"2024-01-01"}`, 400}, {`{"nickname":"` + strings.Repeat("x", 16384) + `"}`, 413}} {
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
