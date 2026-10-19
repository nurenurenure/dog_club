// Package httpapi contains transport helpers. Services and repositories must not
// depend on it or return HTTP problems.
package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// Problem is an RFC 9457 response. Detail and field messages must be safe,
// developer-authored text, never a raw error or a value supplied by a client.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Code      string       `json:"code"`
	Detail    string       `json:"detail,omitempty"`
	RequestID string       `json:"request_id"`
	Errors    []FieldError `json:"errors,omitempty"`
}

func NewProblem(status int, code, detail string) *Problem {
	return &Problem{
		Type: "about:blank", Title: http.StatusText(status), Status: status,
		Code: code, Detail: detail,
	}
}

// WriteProblem adds the request ID without modifying the caller's problem.
// Call before committing a response; unexpected errors belong in Recover.
func WriteProblem(w http.ResponseWriter, r *http.Request, problem *Problem) {
	response := *problem
	response.RequestID = chimiddleware.GetReqID(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", response.RequestID)
	w.Header().Del("Content-Length")
	writeJSON(w, response.Status, "application/problem+json", response)
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, NewProblem(http.StatusNotFound, "not_found", "Ресурс не найден."))
}

// MethodNotAllowed computes Allow using the root router: Chi's custom 405
// handler replaces the default handler that would populate this header.
func MethodNotAllowed(routes chi.Routes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}
		var allowed []string
		// Standard methods supported by Chi. No handler is executed by Match.
		for _, method := range []string{http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPatch, http.MethodPost, http.MethodPut, "QUERY", http.MethodTrace} {
			if routes.Match(chi.NewRouteContext(), method, path) {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) != 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
		}
		WriteProblem(w, r, NewProblem(http.StatusMethodNotAllowed, "method_not_allowed", "Метод не поддерживается для этого ресурса."))
	}
}

func InternalServerError(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, NewProblem(http.StatusInternalServerError, "internal_error", "Внутренняя ошибка сервера."))
}
