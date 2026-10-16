package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dogclub.local/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

func TestMethodNotAllowedFindsMethodsWithoutRunningHandlers(t *testing.T) {
	router := chi.NewRouter()
	router.MethodNotAllowed(httpapi.MethodNotAllowed(router))
	router.Route("/items", func(r chi.Router) {
		handler := func(http.ResponseWriter, *http.Request) { t.Fatal("405 lookup executed a handler") }
		r.Get("/{id}", handler)
		r.Put("/{id}", handler)
	})
	for _, path := range []string{"/items/12", "/items/a%2Fb"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != 405 || w.Header().Get("Allow") != "GET, PUT" {
			t.Fatalf("%s: incorrect 405: %d %v", path, w.Code, w.Header())
		}
	}
}

func TestValidationProblemDoesNotShareFieldStorage(t *testing.T) {
	if httpapi.ValidationProblem() != nil {
		t.Fatal("valid fields must not produce a problem")
	}
	fields := []httpapi.FieldError{{Field: "name", Code: "required", Message: "Укажите имя."}}
	problem := httpapi.ValidationProblem(fields...)
	fields[0].Code = "changed"
	if problem.Errors[0].Code != "required" {
		t.Fatal("problem was changed by reusing validation fields")
	}
}
