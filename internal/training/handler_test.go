package training

import (
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/auth"

	"github.com/go-chi/chi/v5"
)

func TestTrainingRoutesProtected(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, path := range []string{"/api/v1/training-types", "/api/v1/training-instructors", "/api/v1/training-sessions", "/api/v1/dogs/1/training-sessions/1", "/api/v1/dogs/1/training-sessions/1/history", "/api/v1/training-instructors/profile/history"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatalf("unprotected route %s: %d", path, w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/dogs/1/training-sessions", nil)
	req.Header.Set("Origin", "https://foreign.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("missing origin protection")
	}
}
func TestTrainingQueryStrict(t *testing.T) {
	for _, q := range []string{"limit=0", "limit=101", "limit=1&limit=2", "dog_id=0", "type_id=-1", "unknown=1", "from=2024-01-01", "from=2024-01-02T00:00:00Z&to=2024-01-01T00:00:00Z", "after=%xx", "limit="} {
		w := httptest.NewRecorder()
		if _, ok := query(w, httptest.NewRequest("GET", "/?"+q, nil), "sessions"); ok {
			t.Fatalf("accepted bad filter %s", q)
		}
	}
}
func TestTrainingJSONStrict(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":true}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fn := command[Input, Session](nil, true, nil)
	fn(w, r)
	if w.Code != 400 {
		t.Fatal("unknown JSON accepted")
	}
}
