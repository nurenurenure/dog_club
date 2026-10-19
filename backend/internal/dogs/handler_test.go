package dogs

import (
	"net/http/httptest"
	"testing"

	"dogclub.local/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestDogRoutesRequireAccountAndOrigin(t *testing.T) {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, test := range []struct {
		method, path, origin string
		status               int
	}{{"GET", "/api/v1/dogs", "", 401}, {"GET", "/api/v1/dog-dictionaries", "", 401}, {"PUT", "/api/v1/dogs/1/pedigree", "https://foreign.example", 403}, {"DELETE", "/api/v1/dogs/1/titles/1", "https://foreign.example", 403}} {
		request := httptest.NewRequest(test.method, test.path, nil)
		request.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("dog route lacks auth/origin/cache/request ID protection")
		}
	}
}
