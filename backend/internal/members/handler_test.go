package members

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestMemberRoutesRejectUnauthenticatedAndForeignOrigin(t *testing.T) {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, test := range []struct {
		method, path, origin string
		status               int
	}{
		{"GET", "/api/v1/members", "", 401}, {"GET", "/api/v1/members/me", "", 401},
		{"POST", "/api/v1/members", "https://foreign.example", 403},
		{"DELETE", "/api/v1/members/1/contacts/2?version=1", "https://foreign.example", 403},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing auth/origin/cache protection")
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["request_id"] != w.Header().Get("X-Request-ID") {
			t.Fatal("invalid problem response")
		}
	}
}
func TestQueryBoundsAndDuplicateParameters(t *testing.T) {
	for _, query := range []string{"limit=-1", "limit=101", "limit=x", "limit=1&limit=2", "limit="} {
		if _, err := queryNumber(httptest.NewRequest("GET", "/?"+query, nil), "limit", 50, 100); err == nil {
			t.Fatal("invalid page accepted")
		}
	}
}
