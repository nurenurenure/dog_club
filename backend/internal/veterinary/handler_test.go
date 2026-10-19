package veterinary

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestMedicalRoutesRequireAccountAndOrigin(t *testing.T) {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, v := range []struct {
		method, path, origin string
		status               int
	}{{"GET", "/api/v1/veterinary/dogs", "", 401}, {"GET", "/api/v1/dogs/1/medical-status", "", 401}, {"POST", "/api/v1/dogs/1/examinations", "https://foreign.example", 403}, {"POST", "/api/v1/dogs/1/vaccinations/1/void", "https://foreign.example", 403}} {
		req := httptest.NewRequest(v.method, v.path, nil)
		req.Header.Set("Origin", v.origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != v.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("medical route lacks auth/origin/cache/request ID protection")
		}
	}
}
func TestMedicalCommandsStrictJSON(t *testing.T) {
	called := false
	handler := command(new(Handler), "", true, func(context.Context, auth.Principal, int32, int64, VaccineTypeInput) (VaccineType, error) {
		called = true
		return VaccineType{ID: 1}, nil
	})
	for _, v := range []struct {
		body   string
		status int
	}{
		{`{"name":"Test","validity":{"value":12,"unit":"months","unexpected":1}}`, 400},
		{`{"name":"Test","valid_until":"2027-01-01"}`, 400},
		{`{"name":"` + strings.Repeat("x", 16384) + `"}`, 413},
	} {
		called = false
		req := httptest.NewRequest("POST", "/api/v1/vaccine-types", strings.NewReader(v.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != v.status || called {
			t.Fatalf("status %d, want %d", w.Code, v.status)
		}
	}
}
