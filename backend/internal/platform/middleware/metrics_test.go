package middleware

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMetricsAggregateRoutePatternsConcurrently(t *testing.T) {
	m := &HTTPMetrics{}
	r := chi.NewRouter()
	r.Use(m.Observe)
	r.Get("/dogs/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/dogs/private-id?email=private", nil))
		})
	}
	wg.Wait()
	rows := m.Snapshot()
	if len(rows) != 1 || rows[0].Requests != 100 || rows[0].ServerErrors != 100 || rows[0].Route != "/dogs/{id}" || rows[0].DurationSeconds <= 0 {
		t.Fatalf("incorrect metrics: %+v", rows)
	}
	rows[0].Route = "mutation"
	if m.Snapshot()[0].Route != "/dogs/{id}" {
		t.Fatal("snapshot shares mutable state")
	}
}
