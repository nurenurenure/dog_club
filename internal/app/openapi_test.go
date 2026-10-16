package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"dogclub.local/backend/internal/auth"
	"dogclub.local/backend/internal/members"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/go-chi/chi/v5"
)

func TestRoutesAndResponsesMatchOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	available := true
	a := newApp(testConfig(), testLogger(), func(context.Context) error {
		if !available {
			return errors.New("private_database_error")
		}
		return nil
	}, func() {}, auth.NewHandler(nil, false), members.NewHandler(nil, auth.NewHandler(nil, false)))
	mux := a.server.Handler.(*chi.Mux)
	var routes int
	if err := chi.Walk(mux, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		item := doc.Paths.Value(path)
		if item == nil || item.GetOperation(method) == nil {
			t.Errorf("route %s %s is missing from OpenAPI", method, path)
		}
		routes++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var documented int
	for _, item := range doc.Paths.Map() {
		documented += len(item.Operations())
	}
	if documented != routes {
		t.Fatalf("OpenAPI has %d operations, router has %d", documented, routes)
	}

	for _, test := range []struct {
		path      string
		available bool
		status    int
	}{
		{"/health/live", true, 200},
		{"/health/live", false, 200},
		{"/health/ready", true, 200},
		{"/health/ready", false, 503},
	} {
		available = test.available
		r := httptest.NewRequest(http.MethodGet, test.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("%s: status %d, want %d", test.path, w.Code, test.status)
		}
		item := doc.Paths.Value(test.path)
		input := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request: r, Route: &routers.Route{Spec: doc, Path: test.path, PathItem: item, Method: r.Method, Operation: item.Get},
			},
			Status: w.Code, Header: w.Header(), Options: &openapi3filter.Options{IncludeResponseStatus: true},
		}
		if err := openapi3filter.ValidateResponse(context.Background(), input.SetBodyBytes(w.Body.Bytes())); err != nil {
			t.Fatalf("%s %d violates OpenAPI: %v", test.path, w.Code, err)
		}
	}

	// Panic endpoint is a test fixture, never a production route.
	mux.Get("/test/panic", func(http.ResponseWriter, *http.Request) { panic("private_panic") })
	for _, test := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/api/v1/absent?secret=private_query", "not_found", 404},
		{http.MethodPost, "/health/live", "method_not_allowed", 405},
		{http.MethodGet, "/test/panic", "internal_error", 500},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if err := doc.Components.Schemas["Problem"].Value.VisitJSON(body, openapi3.EnableJSONSchema2020()); err != nil {
			t.Fatalf("problem violates OpenAPI: %v", err)
		}
		if w.Code != test.status || body["status"] != float64(test.status) || body["code"] != test.code || body["request_id"] != w.Header().Get("X-Request-ID") {
			t.Fatalf("incorrect error contract: %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Content-Type") != "application/problem+json" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("incorrect headers: %v", w.Header())
		}
		if test.status == 405 && w.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("405 must preserve Allow: GET, got %v", w.Header())
		}
	}
}
