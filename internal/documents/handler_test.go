package documents

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/auth"

	"github.com/go-chi/chi/v5"
)

func TestDocumentRoutesRequireAuthenticationAndOrigin(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/v1", NewHandler(nil, auth.NewHandler(nil, false)).RegisterRoutes)
	for _, path := range []string{"/api/v1/documents", "/api/v1/document-types", "/api/v1/documents/1/files/1"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal("unprotected document route")
		}
	}
	req := httptest.NewRequest("POST", "/api/v1/documents", nil)
	req.Header.Set("Origin", "https://foreign.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("origin not checked")
	}
}
func TestDocumentJSONIsStrict(t *testing.T) {
	called := false
	h := command[Input](nil, true, func(context.Context, auth.Principal, int32, Input) (Document, error) {
		called = true
		return Document{}, nil
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":true}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != 400 || called {
		t.Fatal("unknown field accepted")
	}
}
