package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/platform/httpapi"
	"dogclub.local/backend/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// This route exists only in tests. Business DTOs and routes are added per module.
func testHandler(t *testing.T, limit int64) http.Handler {
	t.Helper()
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(slog.New(slog.NewJSONHandler(io.Discard, nil))), middleware.Recover(slog.Default()))
	router.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		var dto struct {
			Name string `json:"name"`
		}
		if problem := httpapi.DecodeJSONLimit(w, r, &dto, limit); problem != nil {
			httpapi.WriteProblem(w, r, problem)
			return
		}
		if dto.Name == "" {
			httpapi.WriteProblem(w, r, httpapi.ValidationProblem(httpapi.FieldError{
				Field: "name", Code: "required", Message: "Укажите имя.",
			}))
			return
		}
		httpapi.WriteJSON(w, http.StatusCreated, dto)
	})
	return router
}

func TestJSONRequestContract(t *testing.T) {
	handler := testHandler(t, 64)
	for _, test := range []struct {
		name, contentType, body string
		status                  int
		code                    string
	}{
		{"valid", "application/json", `{"name":"Рекс"}`, 201, ""},
		{"charset", "application/json; charset=UTF-8", `{"name":"ok"}`, 201, ""},
		{"trailing whitespace", "application/json", "{\"name\":\"ok\"} \n\t", 201, ""},
		{"empty", "application/json", "", 400, "invalid_json"},
		{"whitespace", "application/json", " \n", 400, "invalid_json"},
		{"malformed", "application/json", `{"name":`, 400, "invalid_json"},
		{"unknown field", "application/json", `{"name":"ok","private_field":"private_value"}`, 400, "invalid_json"},
		{"wrong type", "application/json", `{"name":12}`, 400, "invalid_json"},
		{"array", "application/json", `[]`, 400, "invalid_json"},
		{"scalar", "application/json", `"ok"`, 400, "invalid_json"},
		{"null", "application/json", `null`, 400, "invalid_json"},
		{"two objects", "application/json", `{"name":"ok"}{}`, 400, "invalid_json"},
		{"trailing null", "application/json", `{"name":"ok"}null`, 400, "invalid_json"},
		{"trailing garbage", "application/json", `{"name":"ok"}private_value`, 400, "invalid_json"},
		{"missing type", "", `{}`, 415, "unsupported_media_type"},
		{"wrong media type", "text/plain", `{}`, 415, "unsupported_media_type"},
		{"bad media type", "application/json; charset", `{}`, 415, "unsupported_media_type"},
		{"wrong charset", "application/json; charset=iso-8859-1", `{}`, 415, "unsupported_media_type"},
		{"field validation", "application/json", `{}`, 422, "validation_failed"},
		{"invalid UTF-8", "application/json", "{\"name\":\"\xff\"}", 400, "invalid_json"},
		{"non-JSON whitespace", "application/json", "\u00a0{\"name\":\"ok\"}", 400, "invalid_json"},
		{"too large", "application/json", `{"name":"` + strings.Repeat("a", 60) + `"}`, 413, "body_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/test?token=private_query", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.contentType)
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d, want %d: %s", w.Code, test.status, w.Body.String())
			}
			if test.code == "" {
				if w.Header().Get("Content-Type") != "application/json" {
					t.Fatal(w.Header())
				}
				var dto struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || dto.Name == "" {
					t.Fatalf("successful response lost its DTO: %s", w.Body.String())
				}
				return
			}
			var problem httpapi.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != test.code || problem.Status != w.Code || problem.Title != http.StatusText(w.Code) || problem.Type != "about:blank" {
				t.Fatalf("incorrect problem: %+v", problem)
			}
			if problem.RequestID == "" || problem.RequestID != w.Header().Get("X-Request-ID") || w.Header().Get("Content-Type") != "application/problem+json" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("incorrect problem headers: %v", w.Header())
			}
			if strings.Contains(w.Body.String(), "private_") {
				t.Fatal("problem exposed request details")
			}
			if test.status == 422 && (len(problem.Errors) != 1 || problem.Errors[0].Field != "name" || problem.Errors[0].Code != "required") {
				t.Fatalf("missing field errors: %+v", problem)
			}
		})
	}
}

func TestBodyLimitCountsAllBytes(t *testing.T) {
	const limit = 32
	handler := testHandler(t, limit)
	base := `{"name":"ok"}`
	for _, size := range []int{limit - 1, limit, limit + 1} {
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(base+strings.Repeat(" ", size-len(base))))
		r.Header.Set("Content-Type", "application/json")
		r.ContentLength = -1 // Size checks must not rely on a client-supplied header.
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := http.StatusCreated
		if size > limit {
			want = http.StatusRequestEntityTooLarge
		}
		if w.Code != want {
			t.Fatalf("%d bytes: status %d, want %d", size, w.Code, want)
		}
	}
}

func TestDefaultLimitAndReadError(t *testing.T) {
	var dto struct{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"+strings.Repeat(" ", int(httpapi.DefaultMaxBodyBytes))))
	r.Header.Set("Content-Type", "application/json")
	if problem := httpapi.DecodeJSON(httptest.NewRecorder(), r, &dto); problem == nil || problem.Status != 413 {
		t.Fatalf("default body limit was not enforced: %+v", problem)
	}
	r.Body = io.NopCloser(failingReader{})
	problem := httpapi.DecodeJSON(httptest.NewRecorder(), r, &dto)
	if problem == nil || problem.Status != 400 || strings.Contains(problem.Detail, "private_") {
		t.Fatalf("unsafe read failure: %+v", problem)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("private_read_error") }

func TestEncodingFailureBecomesSafeProblem(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, middleware.RequestLog(logger), middleware.Recover(logger))
	router.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"private_key": make(chan int)})
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != 500 || w.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(w.Body.String()+logs.String(), "private_key") {
		t.Fatalf("encoding failure was not recovered safely: %d %s", w.Code, w.Body.String())
	}
}
