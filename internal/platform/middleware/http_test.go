package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dogclub.local/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestLoggingAndRecoveryDoNotLeakRequestOrPanic(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, RequestLog(logger), Recover(logger))
	router.Get("/items/{id}", func(http.ResponseWriter, *http.Request) {
		panic("private_panic_value")
	})
	request := httptest.NewRequest(http.MethodGet, "/items/private_path?token=private_query", strings.NewReader("private_body"))
	request.Header.Set("Authorization", "Bearer private_token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	var problem httpapi.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusInternalServerError || problem.Status != w.Code || problem.Code != "internal_error" || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("incorrect panic response: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-ID") == "" || problem.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatal("response has no request ID")
	}
	for _, secret := range []string{"private_path", "private_query", "private_body", "private_token", "private_panic_value"} {
		if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
			t.Fatalf("sensitive value was exposed: %s", secret)
		}
	}
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected panic and access log, got %s", logs.String())
	}
	var access struct {
		Route     string `json:"route"`
		Status    int    `json:"status"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &access); err != nil {
		t.Fatal(err)
	}
	if access.Route != "/items/{id}" || access.Status != 500 || access.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("incorrect access log: %+v", access)
	}
}

func TestRecoverPropagatesAbortHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	handler := Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if got := recover(); got != http.ErrAbortHandler {
			t.Errorf("expected http.ErrAbortHandler, got %v", got)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRecoverAbortsCommittedResponse(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID, RequestLog(logger), Recover(logger))
	router.Get("/partial", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("private_panic_value")
	})
	w := httptest.NewRecorder()
	defer func() {
		if got := recover(); got != http.ErrAbortHandler {
			t.Errorf("expected aborted connection, got %v", got)
		}
		if w.Body.String() != "partial" {
			t.Errorf("recovery appended an error to a committed response: %s", w.Body.String())
		}
	}()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/partial", nil))
}
